package downloads

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func publicationProbeCode(err error) uint32 {
	if err == nil {
		return 0
	}
	var code syscall.Errno
	if errors.As(err, &code) {
		return uint32(code)
	}
	return ^uint32(0)
}
func publicationProbePath(file *os.File) (string, error) {
	buffer := make([]uint16, 32768)
	count, err := windows.GetFinalPathNameByHandle(windows.Handle(file.Fd()), &buffer[0], uint32(len(buffer)), 0)
	if err != nil {
		return "", err
	}
	if count == 0 || count >= uint32(len(buffer)) {
		return "", ErrDownload
	}
	return windows.UTF16ToString(buffer[:count]), nil
}

// Opt-in metadata diagnostic only. Creates exclusively in a new random folder;
// never reads user music, changes volume settings, formats, or contacts services.
func TestExternalWindowsPublicationIdentity(t *testing.T) {
	parent := os.Getenv("MIAOYIN_DOWNLOAD_TEST_DIRECTORY")
	if parent == "" {
		t.Skip("explicit external-directory opt-in required")
	}
	if !filepath.IsAbs(parent) {
		t.Fatal("test directory must be absolute")
	}
	dir, err := os.MkdirTemp(parent, ".miaoyin-identity-")
	if err != nil {
		t.Fatal("cannot create isolated metadata fixture directory")
	}
	defer os.Remove(dir) // Remove only our directory if empty; never recurse.
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal("cannot pin fixture directory")
	}
	defer root.Close()
	sourceName := ".miaoyin-synthetic-00000000000000000000000000000000.part"
	targetName := "合成测试-" + strings.Repeat("示例", 32) + " [320K].mp3"
	defer func() { _ = root.Remove(sourceName); _ = root.Remove(targetName) }()
	file, err := root.OpenFile(sourceName, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal("cannot create exclusive fixture")
	}
	defer file.Close()
	fixture := []byte("synthetic-publication-fixture")
	if _, err = file.Write(fixture); err != nil {
		t.Fatal("cannot write fixture")
	}
	if file.Sync() != nil {
		t.Fatal("cannot flush fixture")
	}
	before, err := identityOf(file)
	if err != nil {
		t.Fatal("cannot query fixture identity")
	}
	var fs [64]uint16
	fsErr := windows.GetVolumeInformationByHandle(windows.Handle(file.Fd()), nil, 0, nil, nil, nil, &fs[0], uint32(len(fs)))
	fsType := "other"
	switch windows.UTF16ToString(fs[:]) {
	case "FAT":
		fsType = "FAT"
	case "FAT32":
		fsType = "FAT32"
	case "exFAT":
		fsType = "exFAT"
	case "NTFS":
		fsType = "NTFS"
	}
	publicationErr := publishFile(root, file, before, sourceName, targetName)
	category := "other"
	switch {
	case publicationErr == nil:
		category = "success"
	case errors.Is(publicationErr, ErrPublishedChanged):
		category = "identity_changed"
	case errors.Is(publicationErr, ErrPublicationUnsupported):
		category = "unsupported"
	}
	live, liveErr := identityOf(file)
	t.Logf("publication metadata: fs=%s fs_query_code=%d result=%s live_query_ok=%t live_volume_same=%t live_file_same=%t live_created_same=%t", fsType, publicationProbeCode(fsErr), category, liveErr == nil, live.Volume == before.Volume, live.File == before.File, live.Created == before.Created)
	target, err := root.OpenFile(targetName, os.O_RDONLY, 0)
	if err != nil {
		t.Fatal("synthetic final fixture unavailable; no user files touched")
	}
	defer target.Close()
	final, finalErr := identityOf(target)
	sourcePath, pathErr := publicationProbePath(file)
	targetPath, targetPathErr := publicationProbePath(target)
	got, readErr := os.ReadFile(filepath.Join(dir, targetName))
	t.Logf("publication final metadata: query_ok=%t volume_same=%t file_same=%t created_same=%t live_and_final_file_same=%t live_and_final_created_same=%t handle_paths_query_ok=%t handle_paths_same=%t exact_bytes=%t", finalErr == nil, final.Volume == before.Volume, final.File == before.File, final.Created == before.Created, final.File == live.File, final.Created == live.Created, pathErr == nil && targetPathErr == nil, pathErr == nil && targetPathErr == nil && sourcePath == targetPath, readErr == nil && bytes.Equal(got, fixture))
	created := windows.NsecToFiletime(before.Created)
	setErr := windows.SetFileTime(windows.Handle(file.Fd()), &created, nil, nil)
	after, afterErr := identityOf(file)
	t.Logf("creation metadata roundtrip: representable=%t set_code=%d query_ok=%t restored=%t", created.Nanoseconds() == before.Created, publicationProbeCode(setErr), afterErr == nil, afterErr == nil && after.Created == before.Created)
	fresh, freshErr := root.OpenFile(targetName, os.O_RDONLY, 0)
	if freshErr == nil {
		defer fresh.Close()
		freshIdentity, idErr := identityOf(fresh)
		t.Logf("creation metadata fresh target: query_ok=%t restored=%t", idErr == nil, idErr == nil && freshIdentity.Created == before.Created)
	} else {
		t.Log("creation metadata fresh target: query_ok=false restored=false")
	}
	if readErr != nil || !bytes.Equal(got, fixture) {
		t.Fatal("synthetic fixture bytes changed")
	}
	if publicationErr != nil {
		t.Fatal("synthetic publication rejected; metadata above is diagnostic only, not filesystem acceptance")
	}
}
