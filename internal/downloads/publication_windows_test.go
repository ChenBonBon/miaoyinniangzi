package downloads

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsPublicationRestoresTunneledCreationTimeWithoutRelaxingIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "part")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	expected, err := identityOf(file)
	if err != nil {
		t.Fatal(err)
	}
	changed := windows.NsecToFiletime(time.Now().Add(-24 * time.Hour).UnixNano())
	handle := windows.Handle(file.Fd())
	if err = windows.SetFileTime(handle, &changed, nil, nil); err != nil {
		t.Fatal(err)
	}
	if actual, _ := identityOf(file); actual == expected {
		t.Fatal("creation-time mutation did not occur")
	}
	wrong := expected
	wrong.File++
	if restorePublicationIdentity(handle, wrong) == nil {
		t.Fatal("different file identity accepted")
	}
	if err = restorePublicationIdentity(handle, expected); err != nil {
		t.Fatal(err)
	}
	if actual, _ := identityOf(file); actual != expected {
		t.Fatal("original identity not preserved")
	}
}
func TestWindowsPublicationIntoRecentlyDeletedNamePreservesIdentity(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	target, err := root.OpenFile("target", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	old := windows.NsecToFiletime(time.Now().Add(-48 * time.Hour).UnixNano())
	if err = windows.SetFileTime(windows.Handle(target.Fd()), &old, nil, nil); err != nil {
		target.Close()
		t.Fatal(err)
	}
	target.Close()
	if err = root.Remove("target"); err != nil {
		t.Fatal(err)
	}
	file, err := root.OpenFile("source", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	expected, err := identityOf(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = publishFile(root, file, expected, "source", "target"); err != nil {
		t.Fatal(err)
	}
	published, err := regular(root, "target", false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer published.Close()
	if actual, _ := identityOf(published); actual != expected {
		t.Fatal("rename into recycled filename changed identity")
	}
}
