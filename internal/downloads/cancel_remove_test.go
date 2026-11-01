package downloads

import (
	"context"
	"miaoyinniangzi/internal/storage"
	"testing"
	"time"
)

func TestCancelRunningTaskDisappearsAndDoesNotReturnAfterRestart(t *testing.T) {
	store, dir := setup(t)
	source := mediaFixture()
	entered := make(chan struct{})
	m, err := New(store, source, func(ctx context.Context, _ storage.Song, _ string) (Media, error) {
		close(entered)
		<-ctx.Done()
		return Media{}, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	tasks, err := m.Add(oneSong(), dir, "320k")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("worker not started")
	}
	if err = m.Control([]string{tasks[0].ID}, "cancel", false); err != nil {
		t.Fatal(err)
	}
	if len(m.List()) != 0 {
		t.Fatal("canceled task is still visible")
	}
	if m.Continue(tasks[0].ID) == nil {
		t.Fatal("canceled task could be revived")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		records, err := store.DownloadRecords(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(records) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("released worker record was not removed")
		}
		time.Sleep(time.Millisecond * 5)
	}
	m.Close()
	restored, err := New(store, source, func(context.Context, storage.Song, string) (Media, error) {
		t.Error("cancellation restore performed networking")
		return Media{}, context.Canceled
	})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if len(restored.List()) != 0 {
		t.Fatal("canceled task returned after restart")
	}
}
func TestStartupPrunesLegacyCanceledRecords(t *testing.T) {
	store, dir := setup(t)
	source := mediaFixture()
	m, err := New(store, source, source.load)
	if err != nil {
		t.Fatal(err)
	}
	// Use a real, validated record without ever waking the worker.
	id, _ := newID()
	r := &record{Task: Task{ID: id, Batch: id, Song: oneSong()[0], Directory: dir, Requested: "320k", State: "canceled", Created: 1}}
	m.mu.Lock()
	err = m.saveLocked(r)
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	m.Close()
	restored, err := New(store, source, source.load)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	records, err := store.DownloadRecords(context.Background())
	if err != nil || len(records) != 0 || len(restored.List()) != 0 {
		t.Fatal("legacy cancellation was retained")
	}
}
