package downloads

import (
	"bytes"
	"context"
	"miaoyinniangzi/internal/storage"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestPublishedIdentityMustMatchReportedLiveRename(t *testing.T) {
	store, dir := setup(t)
	source := mediaFixture()
	manager, err := New(store, source, source.load)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	manager.publish = func(root *os.Root, file *os.File, before fileIdentity, part, target string) (fileIdentity, error) {
		after, err := publishFileIdentity(root, file, before, part, target)
		if err != nil {
			return fileIdentity{}, err
		}
		after.File++ // Identical bytes cannot authorize a fabricated final identity.
		return after, nil
	}
	tasks, err := manager.Add(oneSong(), dir, "320k")
	if err != nil {
		t.Fatal(err)
	}
	failed := awaitTask(t, manager, tasks[0].ID, "failed")
	if failed.Message != ErrPublishedChanged.Error() {
		t.Fatal("reported identity mismatch was accepted")
	}
	data, err := os.ReadFile(filepath.Join(dir, failed.File))
	if err != nil || !bytes.Equal(data, source.data) {
		t.Fatal("rejected publication changed verified bytes")
	}
}

// Models a crash after FAT32 renamed but before the final identity was saved.
// Even exact final bytes cannot recover ownership from a stale part identity.
func TestRecoveryRejectsFinalFileWithUncommittedIdentityTransition(t *testing.T) {
	store, dir := setup(t)
	source := mediaFixture()
	first, err := New(store, source, source.load)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := first.Add(oneSong(), dir, "320k")
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	completed := awaitTask(t, first, tasks[0].ID, "success")
	err = first.update(completed.ID, func(r *record) {
		stale := *r.Publication
		stale.File++
		r.Publication = &stale
		r.Task.State = "failed"
	})
	first.Close()
	if err != nil {
		t.Fatal(err)
	}
	var network atomic.Int32
	second, err := New(store, source, func(context.Context, storage.Song, string) (Media, error) {
		network.Add(1)
		return Media{}, ErrDownload
	})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err = second.Continue(completed.ID); err != nil {
		t.Fatal(err)
	}
	failed := awaitTask(t, second, completed.ID, "failed")
	if failed.Message != ErrPublishedChanged.Error() || network.Load() != 0 {
		t.Fatal("uncommitted identity was silently adopted or resolved")
	}
	bytesOnDisk, err := os.ReadFile(filepath.Join(dir, completed.File))
	if err != nil || !bytes.Equal(bytesOnDisk, source.data) {
		t.Fatal("unrecognized final file was changed")
	}
}
func TestVerifiedPublicationIdentityRejectsUnknownIdenticalFile(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	first, err := root.OpenFile("owned", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err = first.Write([]byte("same fixture bytes")); err != nil {
		t.Fatal(err)
	}
	expected, err := identityOf(first)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := digest(first, 18)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "unknown"), []byte("same fixture bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if !verifiedIdentity(root, "owned", 18, sum, expected) || verifiedIdentity(root, "unknown", 18, sum, expected) {
		t.Fatal("identity and content ownership checks diverged")
	}
}
