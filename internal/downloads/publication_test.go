package downloads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"miaoyinniangzi/internal/storage"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestExclusivePublicationDoesNotOverwriteAnyExistingEntry(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = os.WriteFile(filepath.Join(dir, "source.part"), []byte("new bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "target.mp3"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if !os.IsExist(publish(root, "source.part", "target.mp3")) {
		t.Fatal("existing target was not rejected")
	}
	got, _ := os.ReadFile(filepath.Join(dir, "target.mp3"))
	if string(got) != "original" {
		t.Fatal("unknown target overwritten")
	}
	if err = publish(root, "source.part", "fresh.mp3"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(filepath.Join(dir, "source.part")); !os.IsNotExist(err) {
		t.Fatal("publication still relies on a second hard link")
	}
	got, _ = os.ReadFile(filepath.Join(dir, "fresh.mp3"))
	if string(got) != "new bytes" {
		t.Fatal("published content changed")
	}
	if err = os.Mkdir(filepath.Join(dir, "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "other.part"), []byte("x"), 0600)
	if !os.IsExist(publish(root, "other.part", "directory")) {
		t.Fatal("existing directory not protected")
	}
}
func TestConcurrentNoReplacePublicationHasExactlyOneWinner(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	var group sync.WaitGroup
	var winners atomic.Int32
	for i := 0; i < 12; i++ {
		name := partName(string(rune('a' + i)))
		if err = os.WriteFile(filepath.Join(dir, name), []byte{byte(i)}, 0600); err != nil {
			t.Fatal(err)
		}
		group.Add(1)
		go func() {
			defer group.Done()
			err := publish(root, name, "winner.mp3")
			if err == nil {
				winners.Add(1)
			} else if !os.IsExist(err) {
				t.Errorf("unexpected publication failure: %v", err)
			}
		}()
	}
	group.Wait()
	if winners.Load() != 1 {
		t.Fatal("no-replace race had multiple winners")
	}
}

type failFinishStore struct {
	Store
	fail atomic.Bool
}

func (s *failFinishStore) SaveDownload(ctx context.Context, id string, data json.RawMessage) error {
	var r record
	_ = json.Unmarshal(data, &r)
	if r.Task.State == "success" && s.fail.Swap(false) {
		return errors.New("injected final write failure")
	}
	return s.Store.SaveDownload(ctx, id, data)
}
func TestRenamePublicationRecoversAfterFinalDatabaseFailureWithoutNetworking(t *testing.T) {
	store, dir := setup(t)
	failing := &failFinishStore{Store: store}
	failing.fail.Store(true)
	source := mediaFixture()
	first, err := New(failing, source, source.load)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := first.Add(oneSong(), dir, "320k")
	if err != nil {
		t.Fatal(err)
	}
	failed := awaitTask(t, first, tasks[0].ID, "failed")
	first.Close()
	if _, err = os.Lstat(filepath.Join(dir, partName(failed.ID))); !os.IsNotExist(err) {
		t.Fatal("test did not reach rename publication")
	}
	got, err := os.ReadFile(filepath.Join(dir, failed.File))
	if err != nil || !bytes.Equal(got, source.data) {
		t.Fatal("published file missing after database failure")
	}
	var resolved atomic.Int32
	restored, err := New(failing, source, func(context.Context, storage.Song, string) (Media, error) {
		resolved.Add(1)
		return Media{}, ErrDownload
	})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if resolved.Load() != 0 {
		t.Fatal("restart resolved media automatically")
	}
	if err = restored.Continue(failed.ID); err != nil {
		t.Fatal(err)
	}
	awaitTask(t, restored, failed.ID, "success")
	if resolved.Load() != 0 {
		t.Fatal("recovering a proven local publication used the network")
	}
}
