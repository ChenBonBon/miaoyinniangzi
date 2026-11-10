package sourceruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"miaoyinniangzi/internal/testfs"
)

type stageFixture struct {
	name     string
	pin      windows.Handle
	manifest stageManifest
	known    map[string]stageIdentity
	data     []byte
}

func makeStageFixture(t *testing.T, root *os.Root, sid *windows.SID, nonce int, marker bool) *stageFixture {
	t.Helper()
	name := fmt.Sprintf("%s%032x", stagePrefix, nonce)
	descriptor, err := stageSecurityDescriptor(sid)
	if err != nil {
		t.Fatal("fixture descriptor unavailable")
	}
	parent, err := root.Open(".")
	if err != nil {
		t.Fatal("fixture parent unavailable")
	}
	defer parent.Close()
	value, _ := windows.NewNTUnicodeString(name)
	attributes := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: windows.Handle(parent.Fd()), ObjectName: value, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE, SecurityDescriptor: descriptor}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, windows.FILE_GENERIC_READ, &attributes, &status, nil, windows.FILE_ATTRIBUTE_DIRECTORY, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, windows.FILE_CREATE, windows.FILE_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT, 0, 0)
	runtime.KeepAlive(descriptor)
	runtime.KeepAlive(value)
	if err != nil {
		t.Fatal("protected fixture creation failed")
	}
	var fixture *stageFixture
	t.Cleanup(func() {
		if fixture != nil {
			closeFixturePin(fixture)
		} else {
			windows.CloseHandle(handle)
		}
	})
	dirID, err := stageIdentityOf(handle, true)
	if err != nil || !stageACLMatches(handle, sid, true) {
		t.Fatal("protected fixture ACL/identity mismatch")
	}
	data := []byte("synthetic public helper bytes")
	file, err := root.OpenFile(filepath.Join(name, "worker.exe"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("fixture file creation failed")
	}
	if _, err = file.Write(data); err != nil {
		file.Close()
		t.Fatal("fixture write failed")
	}
	fileID, err := stageIdentityOf(windows.Handle(file.Fd()), false)
	file.Close()
	if err != nil {
		t.Fatal("fixture identity failed")
	}
	hash := sha256.Sum256(data)
	fixture = &stageFixture{name: name, pin: handle, data: data, known: map[string]stageIdentity{"worker.exe": fileID}, manifest: stageManifest{Format: stageManifestFormat, Version: 1, Name: name, Directory: dirID, Executable: fileID, Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:])}}
	if marker {
		if err = writeStageManifest(root, name, fixture.manifest, fixture.known); err != nil {
			t.Fatal("fixture manifest failed")
		}
	}
	return fixture
}
func closeFixturePin(fixture *stageFixture) {
	if fixture.pin != 0 {
		windows.CloseHandle(fixture.pin)
		fixture.pin = 0
	}
}
func stageFixtureRoot(t *testing.T) (*os.Root, *windows.SID) {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal("fixture root failed")
	}
	t.Cleanup(func() { root.Close() })
	sid, err := deriveContainerSID()
	if err != nil {
		t.Fatal("fixture SID unavailable")
	}
	return root, sid
}
func TestWindowsStageReaperPreservesActiveAndCollectsIdle(t *testing.T) {
	root, sid := stageFixtureRoot(t)
	fixture := makeStageFixture(t, root, sid, 1, true)
	if report := reapStagedWorkers(context.Background(), root, sid); report.Removed != 0 || report.Busy != 1 {
		t.Fatalf("active stage was not protected: %+v", report)
	}
	closeFixturePin(fixture)
	report := reapStagedWorkers(context.Background(), root, sid)
	if report.Removed != 1 {
		t.Fatalf("proven idle stage not collected: %+v", report)
	}
	if _, err := root.Lstat(fixture.name); !os.IsNotExist(err) {
		t.Fatal("collected stage still named")
	}
}
func TestWindowsStageReaperRefusesUnknownOrChangedContents(t *testing.T) {
	for _, mode := range []string{"legacy", "unknown-file", "changed-exe", "replaced-exe", "wrong-directory", "wrong-executable", "wrong-hash", "wrong-format", "unknown-field", "duplicate-field", "weakened-acl"} {
		t.Run(mode, func(t *testing.T) {
			root, sid := stageFixtureRoot(t)
			fixture := makeStageFixture(t, root, sid, 2, mode != "legacy")
			switch mode {
			case "unknown-file":
				if err := os.WriteFile(filepath.Join(root.Name(), fixture.name, "keep.txt"), []byte("must stay"), 0600); err != nil {
					t.Fatal("fixture failed")
				}
			case "changed-exe":
				if err := os.WriteFile(filepath.Join(root.Name(), fixture.name, "worker.exe"), bytes.Repeat([]byte("x"), len(fixture.data)), 0600); err != nil {
					t.Fatal("fixture mutation failed")
				}
			case "replaced-exe":
				if root.Remove(filepath.Join(fixture.name, "worker.exe")) != nil {
					t.Fatal("replacement removal failed")
				}
				file, err := root.OpenFile(filepath.Join(fixture.name, "worker.exe"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal("replacement creation failed")
				}
				_, writeErr := file.Write(fixture.data)
				identity, idErr := stageIdentityOf(windows.Handle(file.Fd()), false)
				file.Close()
				if writeErr != nil || idErr != nil || identity == fixture.manifest.Executable {
					t.Fatal("replacement identity control failed")
				}
			case "wrong-directory", "wrong-executable", "wrong-hash", "wrong-format", "unknown-field", "duplicate-field":
				value := fixture.manifest
				if mode == "wrong-directory" {
					value.Directory.File++
				}
				if mode == "wrong-executable" {
					value.Executable.File++
				}
				if mode == "wrong-hash" {
					value.SHA256 = strings.Repeat("0", 64)
				}
				if mode == "wrong-format" {
					value.Format = "unknown"
				}
				raw, _ := json.Marshal(value)
				if mode == "unknown-field" {
					raw = append(raw[:len(raw)-1], []byte(`,"unknown":true}`)...)
				}
				if mode == "duplicate-field" {
					raw = append(raw[:len(raw)-1], []byte(`,"version":1}`)...)
				}
				if err := os.WriteFile(filepath.Join(root.Name(), fixture.name, stageOwnerFile), raw, 0600); err != nil {
					t.Fatal("fixture marker mutation failed")
				}
			case "weakened-acl":
				sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;WD)")
				if err != nil {
					t.Fatal("fixture descriptor failed")
				}
				acl, _, _ := sd.DACL()
				if windows.SetNamedSecurityInfo(filepath.Join(root.Name(), fixture.name), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil) != nil {
					t.Fatal("fixture ACL mutation failed")
				}
				runtime.KeepAlive(sd)
			}
			closeFixturePin(fixture)
			if report := reapStagedWorkers(context.Background(), root, sid); report.Removed != 0 {
				t.Fatal("unproven stage was collected")
			}
			if _, err := root.Lstat(filepath.Join(fixture.name, "worker.exe")); err != nil {
				t.Fatal("unproven file was deleted")
			}
		})
	}
}
func TestWindowsStageReaperRefusesReparseAndHardLinks(t *testing.T) {
	t.Run("reparse", func(t *testing.T) {
		root, sid := stageFixtureRoot(t)
		outside := t.TempDir()
		sentinel := filepath.Join(outside, "keep")
		os.WriteFile(sentinel, []byte("sentinel"), 0600)
		name := fmt.Sprintf("%s%032x", stagePrefix, 3)
		testfs.Symlink(t, outside, filepath.Join(root.Name(), name))
		if reapStagedWorkers(context.Background(), root, sid).Removed != 0 {
			t.Fatal("reparse directory collected")
		}
		data, _ := os.ReadFile(sentinel)
		if string(data) != "sentinel" {
			t.Fatal("outside sentinel changed")
		}
	})
	t.Run("hardlink", func(t *testing.T) {
		root, sid := stageFixtureRoot(t)
		fixture := makeStageFixture(t, root, sid, 4, true)
		closeFixturePin(fixture)
		if err := os.Link(filepath.Join(root.Name(), fixture.name, "worker.exe"), filepath.Join(root.Name(), "outside-copy")); err != nil {
			t.Fatal("hardlink fixture failed")
		}
		if reapStagedWorkers(context.Background(), root, sid).Removed != 0 {
			t.Fatal("multiply linked file collected")
		}
	})
}
func TestWindowsOwnedStageCleanupIsNonrecursiveAndHandlesPartialCopies(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprintf("unknown-%t", unknown), func(t *testing.T) {
			root, sid := stageFixtureRoot(t)
			fixture := makeStageFixture(t, root, sid, 5, false)
			if unknown {
				if err := os.WriteFile(filepath.Join(root.Name(), fixture.name, "keep.txt"), []byte("must stay"), 0600); err != nil {
					t.Fatal("unknown fixture failed")
				}
			}
			if !unknown {
				if os.Truncate(filepath.Join(root.Name(), fixture.name, "worker.exe"), 3) != nil {
					t.Fatal("partial copy fixture failed")
				}
			}
			cleaned := cleanupOwnedStage(fixture.pin, fixture.manifest.Directory, fixture.known)
			closeFixturePin(fixture)
			if cleaned {
				deleteEmptyOwnedStage(root, fixture.name, fixture.manifest.Directory)
			}
			_, err := root.Lstat(fixture.name)
			if unknown && err != nil || !unknown && !os.IsNotExist(err) {
				t.Fatal("owned cleanup violated contents policy")
			}
		})
	}
}
func TestWindowsStageReaperFinishesMarkerOnlyCleanup(t *testing.T) {
	root, sid := stageFixtureRoot(t)
	fixture := makeStageFixture(t, root, sid, 9, true)
	closeFixturePin(fixture)
	if root.Remove(filepath.Join(fixture.name, "worker.exe")) != nil {
		t.Fatal("interrupted cleanup fixture failed")
	}
	if report := reapStagedWorkers(context.Background(), root, sid); report.Removed != 1 {
		t.Fatal("valid marker-only cleanup was not resumed")
	}
}
func TestWindowsStageReaperBudgetsAndManifestBinding(t *testing.T) {
	root, sid := stageFixtureRoot(t)
	fixture := makeStageFixture(t, root, sid, 6, true)
	closeFixturePin(fixture)
	parent, _ := root.Open(".")
	defer parent.Close()
	remaining := int64(0)
	if removed, _ := reapStage(context.Background(), windows.Handle(parent.Fd()), fixture.name, sid, &remaining); removed {
		t.Fatal("hash-byte budget ignored")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if reapStagedWorkers(canceled, root, sid).Removed != 0 {
		t.Fatal("canceled collection deleted a stage")
	}
	for _, name := range []string{"miaoyin-worker-..", stagePrefix + strings.Repeat("g", 32), stagePrefix + strings.Repeat("a", 32) + "/child"} {
		if validStageName(name) {
			t.Fatal("invalid staging name accepted")
		}
	}
	if report := reapStagedWorkers(context.Background(), root, sid); report.Removed != 1 {
		t.Fatal("budget refusal destroyed a recoverable stage")
	}
}
func windowsStageControlMain() int {
	if len(os.Args) == 2 && os.Args[1] == "--miaoyin-stage-image-test" {
		if json.NewEncoder(os.Stdout).Encode(struct{ Ready bool }{true}) != nil {
			return 5
		}
		_, _ = io.Copy(io.Discard, os.Stdin)
		return 0
	}
	if len(os.Args) != 4 || !validStageName(os.Args[3]) {
		return 2
	}
	root, err := os.OpenRoot(os.Args[2])
	if err != nil {
		return 3
	}
	defer root.Close()
	parent, err := root.Open(".")
	if err != nil {
		return 3
	}
	defer parent.Close()
	file, err := openStageObject(windows.Handle(parent.Fd()), os.Args[3], true)
	if err != nil {
		return 4
	}
	defer file.Close()
	if json.NewEncoder(os.Stdout).Encode(struct{ Ready bool }{true}) != nil {
		return 5
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	return 0
}
func TestWindowsStageReaperPreservesMappedImageAndRecoveryMarker(t *testing.T) {
	root, sid := stageFixtureRoot(t)
	fixture := makeStageFixture(t, root, sid, 8, true)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("control executable unavailable")
	}
	input, err := os.Open(executable)
	if err != nil {
		t.Fatal("control image open failed")
	}
	defer input.Close()
	output, err := root.OpenFile(filepath.Join(fixture.name, "worker.exe"), os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal("control copy failed")
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, stageFileLimit+1))
	id, idErr := stageIdentityOf(windows.Handle(output.Fd()), false)
	output.Close()
	if copyErr != nil || idErr != nil || size > stageFileLimit {
		t.Fatal("control image copy failed")
	}
	fixture.known["worker.exe"] = id
	fixture.manifest.Executable = id
	fixture.manifest.Size = size
	fixture.manifest.SHA256 = hex.EncodeToString(hash.Sum(nil))
	raw, _ := json.Marshal(fixture.manifest)
	if os.WriteFile(filepath.Join(root.Name(), fixture.name, stageOwnerFile), raw, 0600) != nil {
		t.Fatal("control marker update failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(root.Name(), fixture.name, "worker.exe"), "--miaoyin-stage-image-test")
	pipe, _ := command.StdinPipe()
	readyPipe, _ := command.StdoutPipe()
	if command.Start() != nil {
		t.Fatal("mapped image control failed")
	}
	defer func() { pipe.Close(); command.Process.Kill(); command.Wait() }()
	var ready struct{ Ready bool }
	if json.NewDecoder(io.LimitReader(readyPipe, 1024)).Decode(&ready) != nil || !ready.Ready {
		t.Fatal("mapped image readiness failed")
	}
	cleanupOwnedStage(fixture.pin, fixture.manifest.Directory, fixture.known)
	if _, err = root.Lstat(filepath.Join(fixture.name, stageOwnerFile)); err != nil {
		t.Fatal("creator deleted recovery marker before busy image")
	}
	closeFixturePin(fixture)
	parent, err := root.Open(".")
	if err != nil {
		t.Fatal("control parent failed")
	}
	defer parent.Close()
	remaining := int64(stageReapBytes)
	if removed, _ := reapStage(ctx, windows.Handle(parent.Fd()), fixture.name, sid, &remaining); removed {
		t.Fatal("running mapped image was collected")
	}
	if _, err = root.Lstat(filepath.Join(fixture.name, stageOwnerFile)); err != nil {
		t.Fatal("collector deleted recovery marker before busy image")
	}
	if command.Process.Kill() != nil {
		t.Fatal("mapped image control kill failed")
	}
	command.Wait()
	remaining = stageReapBytes
	if removed, _ := reapStage(ctx, windows.Handle(parent.Fd()), fixture.name, sid, &remaining); !removed {
		t.Fatal("exited image remained unrecoverable")
	}
}
func TestWindowsStageReaperWaitsForCrashedOwnerHandleRelease(t *testing.T) {
	root, sid := stageFixtureRoot(t)
	fixture := makeStageFixture(t, root, sid, 7, true)
	closeFixturePin(fixture)
	executable, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "--miaoyin-stage-lease-test", root.Name(), fixture.name)
	input, _ := command.StdinPipe()
	output, _ := command.StdoutPipe()
	if command.Start() != nil {
		t.Fatal("lease control child failed")
	}
	defer func() { input.Close(); command.Process.Kill(); command.Wait() }()
	var ready struct{ Ready bool }
	if json.NewDecoder(io.LimitReader(output, 1024)).Decode(&ready) != nil || !ready.Ready {
		t.Fatal("lease control readiness failed")
	}
	if report := reapStagedWorkers(context.Background(), root, sid); report.Removed != 0 || report.Busy != 1 {
		t.Fatal("live peer process was reclaimed")
	}
	if command.Process.Kill() != nil {
		t.Fatal("lease owner crash failed")
	}
	command.Wait()
	if report := reapStagedWorkers(context.Background(), root, sid); report.Removed != 1 {
		t.Fatalf("crash-left public copy not reclaimed: %+v", report)
	}
}
