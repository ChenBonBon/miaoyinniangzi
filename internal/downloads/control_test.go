package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"miaoyinniangzi/internal/storage"
)

func seedControlRecords(t *testing.T, m *Manager, store *storage.Store, dir string) map[string]string {
	t.Helper()
	ids := map[string]string{}
	m.mu.Lock()
	defer m.mu.Unlock()
	for index, state := range []string{"queued", "downloading", "paused", "failed", "canceled", "success", "skipped"} {
		id, err := newID()
		if err != nil {
			t.Fatal(err)
		}
		r := record{Task: Task{ID: id, Batch: id, Song: oneSong()[0], Directory: dir, Requested: "320k", State: state, Bytes: 12, Total: 30, Created: int64(index)}}
		data, _ := json.Marshal(r)
		if err = store.SaveDownload(context.Background(), id, data); err != nil {
			t.Fatal(err)
		}
		m.records[id] = &r
		ids[state] = id
	}
	return ids
}
func TestAllControlRespectsEligibilityAndPreservesMetadata(t *testing.T) {
	store, dir := setup(t)
	source := mediaFixture()
	m, _ := New(store, source, func(ctx context.Context, _ storage.Song, _ string) (Media, error) {
		<-ctx.Done()
		return Media{}, ctx.Err()
	})
	defer m.Close()
	ids := seedControlRecords(t, m, store, dir)
	if err := m.Control(nil, "pause", true); err != nil {
		t.Fatal(err)
	}
	paused := map[string]string{}
	for _, task := range m.List() {
		paused[task.ID] = task.State
	}
	for _, state := range []string{"queued", "downloading", "paused"} {
		if paused[ids[state]] != "paused" {
			t.Fatal("active item not paused")
		}
	}
	if err := m.Control(nil, "start", true); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, state := range []string{"queued", "downloading", "paused", "failed"} {
		r := m.records[ids[state]]
		if r.Task.State != "queued" || r.Task.Bytes != 12 || r.Task.Directory != dir {
			t.Fatal("bulk start altered checkpoint or omitted eligible state")
		}
	}
	if m.records[ids["success"]].Task.State != "success" || m.records[ids["skipped"]].Task.State != "skipped" {
		t.Fatal("completed item restarted")
	}
}
func TestSelectedCancelRemovesRecordsWithoutAffectingUnselected(t *testing.T) {
	store, dir := setup(t)
	source := mediaFixture()
	m, _ := New(store, source, source.load)
	defer m.Close()
	ids := seedControlRecords(t, m, store, dir)
	if err := m.Control([]string{ids["paused"], ids["queued"], ids["success"]}, "cancel", false); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.records[ids["paused"]] != nil || m.records[ids["queued"]] != nil || m.records[ids["downloading"]].Task.State != "downloading" || m.records[ids["success"]].Task.State != "success" || len(m.records) != 4 {
		t.Fatal("selection affected other tasks or removed data")
	}
}
func TestInvalidOrFailedControlIsAtomic(t *testing.T) {
	store, dir := setup(t)
	source := mediaFixture()
	m, _ := New(failingBatchStore{store}, source, source.load)
	defer m.Close()
	ids := seedControlRecords(t, m, store, dir)
	for _, request := range []struct {
		ids    []string
		action string
		all    bool
	}{{nil, "pause", false}, {nil, "cancel", true}, {[]string{"../bad"}, "pause", false}, {nil, "unknown", true}} {
		if m.Control(request.ids, request.action, request.all) == nil {
			t.Fatal("invalid bulk request accepted")
		}
	}
	if m.Control([]string{ids["queued"]}, "pause", false) == nil {
		t.Fatal("failed write reported success")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.records[ids["queued"]].Task.State != "queued" {
		t.Fatal("uncommitted transition changed task")
	}
}
func TestQuickPauseAndStartDoesNotTurnRestartIntoFailure(t *testing.T) {
	store, dir := setup(t)
	source := mediaFixture()
	entered := make(chan int32, 2)
	release := make(chan struct{})
	var calls atomic.Int32
	m, _ := New(store, source, func(ctx context.Context, _ storage.Song, _ string) (Media, error) {
		count := calls.Add(1)
		entered <- count
		<-ctx.Done()
		if count == 1 {
			<-release
		}
		return Media{}, errors.New("canceled")
	})
	defer m.Close()
	_, err := m.Add(oneSong(), dir, "320k")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	if err = m.Control(nil, "pause", true); err != nil {
		t.Fatal(err)
	}
	if err = m.Control(nil, "start", true); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case count := <-entered:
		if count != 2 {
			t.Fatal("wrong restart")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("restart was overwritten by old failure")
	}
}
