package privatefs

import (
	"os"
	"path/filepath"
	"testing"

	"miaoyinniangzi/internal/testfs"
)

func TestPrivateDirectoryFilesAndReopen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Protect(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Check(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "data")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err = Check(path, 0600); err != nil {
		t.Fatal("new file did not inherit private permissions:", err)
	}
	if err = ProtectFile(file, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write([]byte("fixture")); err != nil {
		t.Fatal(err)
	}
	if err = Check(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err = Protect(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err = Check(path, 0600); err != nil {
		t.Fatal(err)
	}
	t.Run("symlink-is-not-private", func(t *testing.T) {
		link := filepath.Join(dir, "link")
		testfs.Symlink(t, path, link)
		if Check(link, 0600) == nil {
			t.Fatal("symlink accepted as private file")
		}
	})
}
