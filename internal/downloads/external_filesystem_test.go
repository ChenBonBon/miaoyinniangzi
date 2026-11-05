package downloads

import (
	"bytes"
	"context"
	"miaoyinniangzi/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Explicit opt-in only: never touches existing files in the selected volume.
func TestExternalFilesystemPublicationRecovery(t *testing.T) {
	parent := os.Getenv("MIAOYIN_DOWNLOAD_TEST_DIRECTORY")
	if parent == "" {
		t.Skip("explicit external-directory opt-in required")
	}
	if !filepath.IsAbs(parent) {
		t.Fatal("external test directory must be absolute")
	}
	dir, err := os.MkdirTemp(parent, ".miaoyin-recovery-")
	if err != nil {
		t.Fatal("cannot create isolated recovery fixture")
	}
	defer os.RemoveAll(dir)
	store, err := storage.Open(t.TempDir(), dir)
	if err != nil {
		t.Fatal("cannot create isolated test database")
	}
	defer store.Close()
	source := mediaFixture()
	failing := &failFinishStore{Store: store}
	failing.fail.Store(true)
	first, err := New(failing, source, source.load)
	if err != nil {
		t.Fatal("cannot initialize fixture manager")
	}
	var transitioned atomic.Bool
	first.publish = func(root *os.Root, file *os.File, before fileIdentity, part, target string) (fileIdentity, error) {
		after, err := publishFileIdentity(root, file, before, part, target)
		if err == nil {
			transitioned.Store(before != after)
		}
		return after, err
	}
	songs := oneSong()
	songs[0].Title = strings.Repeat("合成", 32)
	tasks, err := first.Add(songs, dir, "320k")
	if err != nil {
		first.Close()
		t.Fatal("fixture enqueue failed")
	}
	failed := awaitTask(t, first, tasks[0].ID, "failed")
	first.Close()
	if failed.Message != ErrDownload.Error() || failing.fail.Load() {
		t.Fatal("fixture did not reach injected final database failure")
	}
	var network atomic.Int32
	second, err := New(store, source, func(context.Context, storage.Song, string) (Media, error) {
		network.Add(1)
		return Media{}, ErrDownload
	})
	if err != nil {
		t.Fatal("cannot reopen fixture manager")
	}
	defer second.Close()
	if err = second.Continue(failed.ID); err != nil {
		t.Fatal("manual fixture continuation failed")
	}
	complete := awaitTask(t, second, failed.ID, "success")
	got, err := os.ReadFile(filepath.Join(dir, complete.File))
	if err != nil || !bytes.Equal(got, source.data) || network.Load() != 0 {
		t.Fatal("persisted publication recovery lost bytes or resolved a source")
	}
	t.Logf("external published identity recovery: identity_changed=%t exact_bytes=true source_resolutions=0", transitioned.Load())
}

func TestExternalFilesystemDownload(t *testing.T) {
	parent := os.Getenv("MIAOYIN_DOWNLOAD_TEST_DIRECTORY")
	if parent == "" {
		t.Skip("set an explicit mounted external directory to test")
	}
	if !filepath.IsAbs(parent) {
		t.Fatal("external test directory must be absolute")
	}
	dir, err := os.MkdirTemp(parent, ".miaoyin-compat-")
	if err != nil {
		t.Fatal("cannot create isolated filesystem test directory")
	}
	defer os.RemoveAll(dir)
	store, err := storage.Open(t.TempDir(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	source := mediaFixture()
	manager, err := New(store, source, source.load)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	name := fileName(Task{Song: oneSong()[0], Quality: "320k", Codec: "mp3"})
	sentinel := []byte("existing file must never be replaced")
	if err = os.WriteFile(filepath.Join(dir, name), sentinel, 0600); err != nil {
		t.Fatal(err)
	}
	tasks, err := manager.Add(oneSong(), dir, "320k")
	if err != nil {
		t.Fatal(err)
	}
	done := awaitTask(t, manager, tasks[0].ID, "success")
	got, err := os.ReadFile(filepath.Join(dir, done.File))
	if err != nil || !bytes.Equal(got, source.data) || done.File == name {
		t.Fatal("external publication or collision handling failed")
	}
	got, _ = os.ReadFile(filepath.Join(dir, name))
	if !bytes.Equal(got, sentinel) {
		t.Fatal("existing file was overwritten")
	}
	result, err := manager.Enqueue(oneSong(), dir, "320k")
	if err != nil || result.Existing != 1 || len(result.Tasks) != 0 {
		t.Fatal("verified external file did not deduplicate")
	}
	if _, err = os.Stat(filepath.Join(dir, partName(done.ID))); !os.IsNotExist(err) {
		t.Fatal("private checkpoint name remained after completion")
	}
	t.Log("isolated external filesystem download, exact bytes, no-overwrite collision and dedup passed; no paths or music metadata logged")
}
