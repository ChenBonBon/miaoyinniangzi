package downloads

import (
	"bytes"
	"context"
	"errors"
	"miaoyinniangzi/internal/storage"
	"miaoyinniangzi/internal/testfs"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func failedPublication(t *testing.T, cause error) (*storage.Store, string, *fixtureMedia, Task) {
	t.Helper()
	store, dir := setup(t)
	source := mediaFixture()
	manager, err := New(store, source, source.load)
	if err != nil {
		t.Fatal(err)
	}
	manager.publish = func(*os.Root, *os.File, fileIdentity, string, string) error { return cause }
	tasks, err := manager.Add(oneSong(), dir, "320k")
	if err != nil {
		t.Fatal(err)
	}
	task := awaitTask(t, manager, tasks[0].ID, "failed")
	manager.Close()
	if task.Message != cause.Error() {
		t.Fatal("storage failure message lost")
	}
	if task.Bytes != int64(len(source.data)) {
		t.Fatal("complete checkpoint not retained")
	}
	return store, dir, source, task
}
func TestCompleteDownloadRetriesPublicationOfflineAfterStorageFailures(t *testing.T) {
	for _, cause := range []error{ErrDiskFull, ErrDirectoryUnavailable, ErrPublicationUnsupported} {
		t.Run(cause.Error(), func(t *testing.T) {
			store, dir, source, task := failedPublication(t, cause)
			var network atomic.Int32
			manager, err := New(store, source, func(context.Context, storage.Song, string) (Media, error) {
				network.Add(1)
				return Media{}, ErrDownload
			})
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			if err = manager.Continue(task.ID); err != nil {
				t.Fatal(err)
			}
			completed := awaitTask(t, manager, task.ID, "success")
			data, err := os.ReadFile(filepath.Join(dir, completed.File))
			if err != nil || !bytes.Equal(data, source.data) || network.Load() != 0 {
				t.Fatal("local retry lost bytes or used network")
			}
		})
	}
}
func TestRecoveryDoesNotClaimAnUnknownIdenticalTargetByHash(t *testing.T) {
	store, dir, source, task := failedPublication(t, ErrDiskFull)
	if err := os.WriteFile(filepath.Join(dir, task.File), source.data, 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := New(store, source, source.load)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err = manager.Continue(task.ID); err != nil {
		t.Fatal(err)
	}
	completed := awaitTask(t, manager, task.ID, "success")
	if completed.File == task.File {
		t.Fatal("unknown identical file inferred as owned")
	}
	data, _ := os.ReadFile(filepath.Join(dir, task.File))
	if !bytes.Equal(data, source.data) {
		t.Fatal("existing file changed")
	}
}
func TestMissingCheckpointOrReplacedDirectoryNeverCreatesFilesOnRetry(t *testing.T) {
	store, dir, source, task := failedPublication(t, ErrDirectoryUnavailable)
	moved := dir + "-detached"
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(moved)
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	var network atomic.Int32
	manager, err := New(store, source, func(context.Context, storage.Song, string) (Media, error) {
		network.Add(1)
		return Media{}, ErrDownload
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err = manager.Continue(task.ID); err != nil {
		t.Fatal(err)
	}
	failed := awaitTask(t, manager, task.ID, "failed")
	if failed.Message != ErrPublishedChanged.Error() || network.Load() != 0 {
		t.Fatal("missing checkpoint was silently restarted")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("replacement directory was written")
	}
}
func TestExternalModificationOfCompletedCheckpointDoesNotPublish(t *testing.T) {
	store, dir, source, task := failedPublication(t, ErrDiskFull)
	path := filepath.Join(dir, partName(task.ID))
	file, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.WriteAt([]byte("changed"), 0)
	file.Close()
	manager, err := New(store, source, source.load)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err = manager.Continue(task.ID); err != nil {
		t.Fatal(err)
	}
	failed := awaitTask(t, manager, task.ID, "failed")
	if failed.Message != ErrPublishedChanged.Error() {
		t.Fatal("changed checkpoint not rejected")
	}
	if _, err = os.Stat(filepath.Join(dir, task.File)); !os.IsNotExist(err) {
		t.Fatal("modified file published")
	}
}
func TestPinnedDirectoryReplacementIsRejected(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	moved := dir + "-old"
	if err = os.Rename(dir, moved); err != nil {
		if testfs.RenamePinnedDenied(err) && directoryStillMounted(root, dir) {
			t.Log("Windows prevents renaming an open pinned directory")
			return
		}
		t.Fatal(err)
	}
	defer os.RemoveAll(moved)
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if directoryStillMounted(root, dir) {
		t.Fatal("replacement directory accepted")
	}
	if !errors.Is(storageFailure(ErrDiskFull), ErrDiskFull) {
		t.Fatal("typed storage error changed")
	}
}
