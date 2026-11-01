package downloads

import (
	"testing"
	"time"
)

func TestLateQualityDowngradeDuplicateLeavesNoRecord(t *testing.T) {
	store, dir := setup(t)
	source := mediaFixture()
	manager, err := New(store, source, source.load)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	added, err := manager.Add(oneSong(), dir, "320k")
	if err != nil {
		t.Fatal(err)
	}
	awaitTask(t, manager, added[0].ID, "success")
	noticed := make(chan struct{}, 1)
	manager.SetDuplicateNotice(func() { noticed <- struct{}{} })
	// Highest cannot preflight-match a 320K file until the resolver confirms the
	// actual downgraded target. This check must also discard its provisional job.
	result, err := manager.Enqueue(oneSong(), dir, "highest")
	if err != nil || len(result.Tasks) != 1 {
		t.Fatal("expected provisional resolution task")
	}
	select {
	case <-noticed:
	case <-time.After(5 * time.Second):
		t.Fatal("duplicate notice not emitted")
	}
	if len(manager.List()) != 1 {
		t.Fatal("duplicate left a skipped history record")
	}
	records, err := store.DownloadRecords(manager.ctx)
	if err != nil || len(records) != 1 {
		t.Fatal("duplicate record persisted")
	}
}
