package songcache

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"miaoyinniangzi/internal/mediastream"
	"miaoyinniangzi/internal/mpvaudio"
)

func fakeMP3(seconds int) []byte {
	var data []byte
	for count := 0; float64(count)*1152/44100 < float64(seconds); count++ {
		frame := make([]byte, 417)
		copy(frame, []byte{255, 251, 144, 0})
		data = append(data, frame...)
	}
	return data
}

type memorySource struct {
	data  []byte
	calls atomic.Int32
	etag  string
	byURL sync.Map
}

func (s *memorySource) OpenMedia(ctx context.Context, remote, method, rangeValue string) (*http.Response, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	s.calls.Add(1)
	value, _ := s.byURL.LoadOrStore(remote, &atomic.Int32{})
	value.(*atomic.Int32).Add(1)
	start, end := 0, len(s.data)-1
	if _, err := fmt.Sscanf(rangeValue, "bytes=%d-%d", &start, &end); err != nil {
		return nil, err
	}
	if start < 0 || end >= len(s.data) || end < start {
		return nil, ErrCache
	}
	h := make(http.Header)
	h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(s.data)))
	h.Set("ETag", s.etag)
	return &http.Response{StatusCode: 206, Header: h, ContentLength: int64(end - start + 1), Body: io.NopCloser(bytes.NewReader(s.data[start : end+1]))}, nil
}
func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("cache condition timed out")
}
func TestCompleteCurrentCacheAndFrameAlignedDemotion(t *testing.T) {
	source := &memorySource{data: fakeMP3(30), etag: `"one"`}
	info := mediastream.Info{Codec: "mp3", Size: int64(len(source.data)), Seekable: true}
	disk, err := newDisk(context.Background(), source, "http://public.example/audio", info, newBudget())
	if err != nil {
		t.Fatal(err)
	}
	defer disk.close()
	disk.setFull(true)
	waitUntil(t, func() bool {
		disk.mu.Lock()
		defer disk.mu.Unlock()
		return disk.available(0) == info.Size && disk.prefix > 0
	})
	calls := source.calls.Load()
	for _, offset := range []int64{0, info.Size / 2, info.Size - 100} {
		output := make([]byte, 100)
		if _, err = disk.readAt(context.Background(), output, offset); err != nil || !bytes.Equal(output, source.data[offset:offset+100]) {
			t.Fatal("cached seek bytes differ")
		}
	}
	if source.calls.Load() != calls {
		t.Fatal("complete cache still accessed source")
	}
	disk.setFull(false)
	disk.mu.Lock()
	prefix := disk.prefix
	disk.mu.Unlock()
	if prefix%417 != 0 || float64(prefix/417)*1152/44100 < 5 || prefix >= info.Size {
		t.Fatal("not a five-second frame boundary")
	}
	stat, err := disk.file.Stat()
	if err != nil || stat.Size() != prefix {
		t.Fatal("full tail retained after demotion")
	}
	fileData, err := os.ReadFile(filepath.Join(disk.directory, "audio.cache"))
	if err != nil || !bytes.Equal(fileData, source.data[:prefix]) {
		t.Fatal("prefix was rewritten or corrupted")
	}
	if _, err = disk.readAt(context.Background(), make([]byte, 100), info.Size-100); err != nil {
		t.Fatal(err)
	}
	stat, _ = disk.file.Stat()
	if stat.Size() != prefix {
		t.Fatal("neighbor metadata read regrew discarded tail")
	}
	if runtimeMode, err := os.Stat(disk.directory); err != nil || runtimeMode.Mode().Perm() != 0700 {
		t.Fatal("cache directory not private")
	}
}
func TestFLACPrefixHasCompleteFrames(t *testing.T) {
	data, err := os.ReadFile("../../frontend/e2e/fixtures/tone.flac")
	if err != nil {
		t.Fatal(err)
	}
	cut, err := prefixEnd(data, "flac", 5)
	if err != nil || cut >= len(data) || cut < 42 {
		t.Fatalf("FLAC boundary failed: %d %v", cut, err)
	}
	if data[cut] != 255 || data[cut+1]&254 != 248 {
		t.Fatal("FLAC cut is not a frame boundary")
	}
}
func TestSourceEntityChangesAreNotMerged(t *testing.T) {
	source := &memorySource{data: fakeMP3(40), etag: `"one"`}
	disk, err := newDisk(context.Background(), source, "http://public.example/audio", mediastream.Info{Codec: "mp3", Size: int64(len(source.data)), Seekable: true}, newBudget())
	if err != nil {
		t.Fatal(err)
	}
	defer disk.close()
	if _, err = disk.readAt(context.Background(), make([]byte, 4), 0); err != nil {
		t.Fatal(err)
	}
	source.etag = `"two"`
	if _, err = disk.readAt(context.Background(), make([]byte, 4), chunkSize); err == nil {
		t.Fatal("changed entity merged into previous cache")
	}
}
func TestThreeSlotsPromoteDemoteReuseAndRevoke(t *testing.T) {
	source := &memorySource{data: fakeMP3(30), etag: `"one"`}
	m := New(context.Background(), source)
	defer m.Close()
	m.open = func(context.Context, string, string) (*mpvaudio.Player, error) { return &mpvaudio.Player{}, nil }
	var loadCalls atomic.Int32
	request := func(id string) Request {
		return Request{Key: Key{Source: "source", Quality: "128k", Song: id}, Load: func(context.Context) (Spec, error) {
			loadCalls.Add(1)
			return Spec{Remote: "http://public.example/" + id, Info: mediastream.Info{Codec: "mp3", Size: int64(len(source.data)), Seekable: true}, Quality: "128k"}, nil
		}}
	}
	one, two, three := request("one"), request("two"), request("three")
	player, _, _, err := m.Use(context.Background(), 1, one, []Request{two, three})
	if err != nil {
		t.Fatal(err)
	}
	player.Close()
	waitUntil(t, func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return len(m.entries) == 3 && m.entries[two.Key] != nil && m.entries[two.Key].finished && m.entries[three.Key] != nil && m.entries[three.Key].finished
	})
	m.mu.Lock()
	old := m.entries[one.Key]
	m.mu.Unlock()
	waitUntil(t, func() bool {
		old.disk.mu.Lock()
		defer old.disk.mu.Unlock()
		return old.disk.available(0) == old.spec.Info.Size && old.disk.prefix > 0
	})
	player, _, hit, err := m.Use(context.Background(), 2, two, []Request{one, three})
	if err != nil || !hit {
		t.Fatal("prepared neighbor not reused")
	}
	player.Close()
	if loadCalls.Load() != 3 {
		t.Fatal("cached neighbor re-resolved")
	}
	old.disk.mu.Lock()
	prefix := old.disk.prefix
	stat, err := old.disk.file.Stat()
	old.disk.mu.Unlock()
	if err != nil || stat.Size() != prefix {
		t.Fatal("old current did not demote")
	}
	m.mu.Lock()
	directory := old.disk.directory
	m.mu.Unlock()
	m.Invalidate(3)
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("revocation retained audio file")
	}
	if m.apply(2, 0, one.Key, nil) {
		t.Fatal("stale topology resurrected cache")
	}
}
func TestConcurrentChunkReadsShareOneRequest(t *testing.T) {
	source := &memorySource{data: fakeMP3(30), etag: `"one"`}
	disk, err := newDisk(context.Background(), source, "http://public.example/audio", mediastream.Info{Codec: "mp3", Size: int64(len(source.data)), Seekable: true}, newBudget())
	if err != nil {
		t.Fatal(err)
	}
	defer disk.close()
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := disk.readAt(context.Background(), make([]byte, 64), 0); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if source.calls.Load() != 1 {
		t.Fatal("duplicate range fetch")
	}
}
