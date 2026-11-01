package downloads

import (
	"bytes"
	"miaoyinniangzi/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

// Explicit opt-in only: never touches existing files in the selected volume.
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
