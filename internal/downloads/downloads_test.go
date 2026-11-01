package downloads

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"miaoyinniangzi/internal/testfs"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"miaoyinniangzi/internal/mediastream"
	"miaoyinniangzi/internal/storage"
)

type fixtureMedia struct {
	mu      sync.Mutex
	data    []byte
	tag     string
	starts  []int64
	block   bool
	blocked chan struct{}
	once    sync.Once
}

func (f *fixtureMedia) OpenMedia(ctx context.Context, _, _, value string) (*http.Response, error) {
	f.mu.Lock()
	start, end := int64(0), int64(len(f.data)-1)
	status := 200
	if value != "" {
		if _, err := fmt.Sscanf(value, "bytes=%d-%d", &start, &end); err != nil {
			f.mu.Unlock()
			return nil, err
		}
		status = 206
	}
	f.starts = append(f.starts, start)
	block := f.block && start >= chunkBytes
	data := append([]byte(nil), f.data[start:end+1]...)
	tag := f.tag
	f.mu.Unlock()
	if block {
		f.once.Do(func() { close(f.blocked) })
		<-ctx.Done()
		return nil, ctx.Err()
	}
	header := make(http.Header)
	header.Set("ETag", tag)
	if status == 206 {
		header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(f.data)))
	}
	return &http.Response{StatusCode: status, Header: header, ContentLength: int64(len(data)), Body: io.NopCloser(bytes.NewReader(data))}, nil
}
func (f *fixtureMedia) load(context.Context, storage.Song, string) (Media, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return Media{URL: "https://media.example/audio?signature=private", Source: "source", Quality: "320k", Info: mediastream.Info{Codec: "mp3", Size: int64(len(f.data)), Seekable: true, EntityTag: f.tag}}, nil
}
func mediaFixture() *fixtureMedia {
	frame := make([]byte, 1044)
	copy(frame, []byte{255, 251, 224, 0})
	data := bytes.Repeat(frame, int(3*chunkBytes/1044)+1)
	return &fixtureMedia{data: data, tag: `"one"`, blocked: make(chan struct{})}
}
func awaitTask(t *testing.T, m *Manager, id, state string) Task {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, task := range m.List() {
			if task.ID == id && task.State == state {
				return task
			}
			if task.ID == id && task.State == "failed" && state != "failed" {
				t.Fatalf("task failed: %s", task.Message)
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("download state timed out")
	return Task{}
}
func setup(t *testing.T) (*storage.Store, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := storage.Open(t.TempDir(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store, dir
}
func oneSong() []storage.Song {
	return []storage.Song{{ID: "Song1", Source: "tx", Title: "Title", Artist: "Artist", Duration: 60.125}}
}
func TestDownloadCollisionDedupMissingFileAndSafeDeletion(t *testing.T) {
	store, dir := setup(t)
	source := mediaFixture()
	m, err := New(store, source, source.load)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	name := "Artist - Title [320K].mp3"
	if err = os.WriteFile(filepath.Join(dir, name), []byte("unknown existing file"), 0600); err != nil {
		t.Fatal(err)
	}
	added, err := m.Add(oneSong(), dir, "320k")
	if err != nil {
		t.Fatal(err)
	}
	done := awaitTask(t, m, added[0].ID, "success")
	if done.File == name {
		t.Fatal("unknown file overwritten")
	}
	data, _ := os.ReadFile(filepath.Join(dir, name))
	if string(data) != "unknown existing file" {
		t.Fatal("unknown file modified")
	}
	data, _ = os.ReadFile(filepath.Join(dir, done.File))
	if !bytes.Equal(data, source.data) {
		t.Fatal("download bytes differ")
	}
	before := len(m.List())
	duplicate, err := m.Enqueue(oneSong(), dir, "128k")
	if err != nil || duplicate.Existing != 1 || len(duplicate.Tasks) != 0 || len(m.List()) != before {
		t.Fatal("existing download added a history record")
	}
	if err = m.Remove(done.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, done.File)); err != nil {
		t.Fatal("record removal deleted file")
	}
	added, _ = m.Add(oneSong(), dir, "320k")
	second := awaitTask(t, m, added[0].ID, "success")
	if err = os.WriteFile(filepath.Join(dir, second.File), []byte("external replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if m.Remove(second.ID, true) == nil {
		t.Fatal("modified file deleted")
	}
	if err = os.Remove(filepath.Join(dir, second.File)); err != nil {
		t.Fatal(err)
	}
	added, _ = m.Add(oneSong(), dir, "320k")
	awaitTask(t, m, added[0].ID, "success")
}
func TestRestartWaitsForUserAndResumesOnlyMatchingEntity(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprint(changed), func(t *testing.T) {
			store, dir := setup(t)
			source := mediaFixture()
			source.block = true
			m, err := New(store, source, source.load)
			if err != nil {
				t.Fatal(err)
			}
			added, _ := m.Add(oneSong(), dir, "320k")
			id := added[0].ID
			select {
			case <-source.blocked:
			case <-time.After(5 * time.Second):
				t.Fatal("checkpoint not reached")
			}
			m.Close()
			source.mu.Lock()
			source.block = false
			source.starts = nil
			if changed {
				source.tag = `"two"`
				source.data[100] = 99
			}
			source.mu.Unlock()
			m, err = New(store, source, source.load)
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			awaitTask(t, m, id, "paused")
			source.mu.Lock()
			count := len(source.starts)
			source.mu.Unlock()
			if count != 0 {
				t.Fatal("restart automatically contacted media")
			}
			if err = m.Continue(id); err != nil {
				t.Fatal(err)
			}
			done := awaitTask(t, m, id, "success")
			source.mu.Lock()
			first := source.starts[0]
			source.mu.Unlock()
			if changed && first != 0 || !changed && first != chunkBytes {
				t.Fatalf("incorrect resume boundary %d", first)
			}
			data, _ := os.ReadFile(filepath.Join(dir, done.File))
			if !bytes.Equal(data, source.data) {
				t.Fatal("different entities were spliced")
			}
		})
	}
}
func TestDownloadMetadataDoesNotPersistURLAndSymlinkIsRejected(t *testing.T) {
	store, dir := setup(t)
	source := mediaFixture()
	m, _ := New(store, source, source.load)
	defer m.Close()
	added, _ := m.Add(oneSong(), dir, "320k")
	awaitTask(t, m, added[0].ID, "success")
	records, err := store.DownloadRecords(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if bytes.Contains(record, []byte("signature")) || bytes.Contains(record, []byte("https://")) {
			t.Fatal("media URL persisted")
		}
	}
	outside := filepath.Join(t.TempDir(), "outside")
	_ = os.WriteFile(outside, []byte("untouched"), 0600)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	t.Run("symlink", func(t *testing.T) {
		testfs.Symlink(t, outside, filepath.Join(dir, "bad.part"))
		if _, err := regular(root, "bad.part", true, true); err == nil {
			t.Fatal("symlink accepted")
		}
	})
}
