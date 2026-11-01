package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"miaoyinniangzi/internal/storage"
)

type gatedReader struct {
	io.ReadCloser
	ctx       context.Context
	remaining int64
	notify    func()
}

func (r *gatedReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		r.notify()
		<-r.ctx.Done()
		return 0, r.ctx.Err()
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.ReadCloser.Read(p)
	r.remaining -= int64(n)
	return n, err
}

type wholeMedia struct{ *fixtureMedia }

func (f wholeMedia) OpenMedia(ctx context.Context, url, method, rangeValue string) (*http.Response, error) {
	response, err := f.fixtureMedia.OpenMedia(ctx, url, method, rangeValue)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	block := f.block
	f.mu.Unlock()
	if block && rangeValue == "" {
		response.Body = &gatedReader{ReadCloser: response.Body, ctx: ctx, remaining: chunkBytes, notify: func() { f.once.Do(func() { close(f.blocked) }) }}
	}
	return response, nil
}
func TestMissingValidatorRestartsRatherThanAppending(t *testing.T) {
	store, dir := setup(t)
	source := mediaFixture()
	source.tag = ""
	source.block = true
	raw := wholeMedia{source}
	manager, _ := New(store, raw, source.load)
	added, _ := manager.Add(oneSong(), dir, "320k")
	id := added[0].ID
	select {
	case <-source.blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("whole stream did not reach pause point")
	}
	manager.Close()
	source.mu.Lock()
	source.block = false
	source.starts = nil
	source.mu.Unlock()
	manager, _ = New(store, raw, source.load)
	defer manager.Close()
	if err := manager.Continue(id); err != nil {
		t.Fatal(err)
	}
	done := awaitTask(t, manager, id, "success")
	if source.starts[0] != 0 {
		t.Fatal("unvalidated entity was appended")
	}
	if done.Message != "无法确认同一媒体实体，已从头下载" {
		t.Fatal("restart was not explained")
	}
}
func TestChangedPartialIsNotOverwritten(t *testing.T) {
	store, dir := setup(t)
	source := mediaFixture()
	source.block = true
	manager, _ := New(store, source, source.load)
	added, _ := manager.Add(oneSong(), dir, "320k")
	id := added[0].ID
	<-source.blocked
	manager.Close()
	file, err := os.OpenFile(filepath.Join(dir, partName(id)), os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteAt([]byte("changed"), 100)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	source.block = false
	manager, _ = New(store, source, source.load)
	defer manager.Close()
	_ = manager.Continue(id)
	awaitTask(t, manager, id, "failed")
	data, err := os.ReadFile(filepath.Join(dir, partName(id)))
	if err != nil || string(data[100:107]) != "changed" {
		t.Fatal("changed checkpoint overwritten")
	}
}

type failingBatchStore struct{ *storage.Store }

func (s failingBatchStore) SaveDownloads(context.Context, map[string]json.RawMessage) error {
	return errors.New("write failed")
}
func TestBatchPersistenceFailureDoesNotStartPartialWork(t *testing.T) {
	store, dir := setup(t)
	source := mediaFixture()
	manager, _ := New(failingBatchStore{store}, source, source.load)
	defer manager.Close()
	if _, err := manager.Add(oneSong(), dir, "320k"); err == nil {
		t.Fatal("failed persistence reported success")
	}
	if len(manager.List()) != 0 || len(source.starts) != 0 {
		t.Fatal("uncommitted tasks started")
	}
}
func TestResourceIdentityIncludesQueryAndOrigin(t *testing.T) {
	a := resourceKey("https://a.example/audio?file=one")
	if a == resourceKey("https://a.example/audio?file=two") || a == resourceKey("https://b.example/audio?file=one") {
		t.Fatal("unrelated resource treated as identical")
	}
}
