package sourceruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const stagePrefix = "miaoyin-worker-"
const stageOwnerFile = "owner.json"
const stageManifestFormat = "miaoyin-source-stage"
const stageFileLimit = 256 << 20
const stageReapBytes = 256 << 20
const stageReapTime = 300 * time.Millisecond

type stageIdentity struct {
	Volume  uint32 `json:"volume"`
	File    uint64 `json:"file"`
	Created uint64 `json:"created"`
}
type stageManifest struct {
	Format     string        `json:"format"`
	Version    int           `json:"version"`
	Name       string        `json:"name"`
	Directory  stageIdentity `json:"directory"`
	Executable stageIdentity `json:"executable"`
	Size       int64         `json:"size"`
	SHA256     string        `json:"sha256"`
}
type stageReapReport struct {
	Removed, Busy, Unproven int
	Bounded                 bool
}

func stageSecurityDescriptor(sid *windows.SID) (*windows.SECURITY_DESCRIPTOR, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, errWindowsSandbox
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;GRGX;;;" + sid.String() + ")(A;OICI;RC;;;OW)")
	if err != nil {
		return nil, errWindowsSandbox
	}
	return descriptor, nil
}
func validStageName(name string) bool {
	if len(name) != len(stagePrefix)+32 || !strings.HasPrefix(name, stagePrefix) {
		return false
	}
	for _, ch := range name[len(stagePrefix):] {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	return true
}
func stageIdentityOf(handle windows.Handle, directory bool) (stageIdentity, error) {
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || !directory && info.NumberOfLinks != 1 {
		return stageIdentity{}, sandboxFailure("staging object identity")
	}
	id := uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)
	if id == 0 {
		return stageIdentity{}, sandboxFailure("staging object identity")
	}
	return stageIdentity{info.VolumeSerialNumber, id, uint64(info.CreationTime.HighDateTime)<<32 | uint64(info.CreationTime.LowDateTime)}, nil
}
func mappedStageAccess(mask uint32) uint32 {
	if mask&windows.GENERIC_ALL != 0 {
		mask = mask&^windows.GENERIC_ALL | 0x1f01ff
	}
	if mask&windows.GENERIC_READ != 0 {
		mask = mask&^windows.GENERIC_READ | windows.FILE_GENERIC_READ
	}
	if mask&windows.GENERIC_WRITE != 0 {
		mask = mask&^windows.GENERIC_WRITE | windows.FILE_GENERIC_WRITE
	}
	if mask&windows.GENERIC_EXECUTE != 0 {
		mask = mask&^windows.GENERIC_EXECUTE | windows.FILE_GENERIC_EXECUTE
	}
	return mask
}

// Require the exact creator policy, never repair or broaden an unexpected ACL.
func stageACLMatches(handle windows.Handle, sid *windows.SID, directory bool) bool {
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	defer runtime.KeepAlive(descriptor)
	control, _, err := descriptor.Control()
	if err != nil || directory && control&windows.SE_DACL_PROTECTED == 0 {
		return false
	}
	acl, _, err := descriptor.DACL()
	if err != nil || acl == nil || acl.AceCount != 4 {
		return false
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return false
	}
	expected := map[string]uint32{"S-1-5-18": 0x1f01ff, user.User.Sid.String(): 0x1f01ff, sid.String(): windows.FILE_GENERIC_READ | windows.FILE_GENERIC_EXECUTE, "S-1-3-4": windows.READ_CONTROL}
	seen := map[string]bool{}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			return false
		}
		principal := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
		mask, ok := expected[principal]
		if !ok || seen[principal] || mappedStageAccess(uint32(ace.Mask)) != mask {
			return false
		}
		seen[principal] = true
	}
	return len(seen) == 4
}
func openStageObject(parent windows.Handle, name string, directory bool) (*os.File, error) {
	if directory && !validStageName(name) || !directory && name != "worker.exe" && name != stageOwnerFile {
		return nil, sandboxFailure("staging relative name")
	}
	value, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, sandboxFailure("staging relative name")
	}
	attributes := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: parent, ObjectName: value, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	options := uint32(windows.FILE_NON_DIRECTORY_FILE)
	if directory {
		options = windows.FILE_DIRECTORY_FILE
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, windows.FILE_GENERIC_READ|windows.DELETE, &attributes, &status, nil, 0, windows.FILE_SHARE_READ, windows.FILE_OPEN, options|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	runtime.KeepAlive(value)
	if err != nil {
		return nil, err
	}
	if _, err = stageIdentityOf(handle, directory); err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	return os.NewFile(uintptr(handle), "source staging"), nil
}
func stageEntries(handle windows.Handle) ([]os.DirEntry, error) {
	var duplicate windows.Handle
	if err := windows.DuplicateHandle(windows.CurrentProcess(), handle, windows.CurrentProcess(), &duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(duplicate), "source staging")
	defer file.Close()
	entries, err := file.ReadDir(3)
	if err != nil && err != io.EOF || len(entries) > 2 {
		return nil, sandboxFailure("staging contents")
	}
	for _, entry := range entries {
		if entry.Name() != "worker.exe" && entry.Name() != stageOwnerFile || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, sandboxFailure("staging contents")
		}
	}
	return entries, nil
}
func deleteStageHandle(handle windows.Handle) error {
	var status windows.IO_STATUS_BLOCK
	remove := byte(1)
	return windows.NtSetInformationFile(handle, &status, &remove, 1, windows.FileDispositionInformation)
}
func stageFileGone(directory windows.Handle, name string) bool {
	file, err := openStageObject(directory, name, false)
	if err == nil {
		file.Close()
		return false
	}
	return errors.Is(err, windows.STATUS_OBJECT_NAME_NOT_FOUND)
}
func readStageManifest(file *os.File) (stageManifest, error) {
	var value stageManifest
	info, err := file.Stat()
	if err != nil || info.Size() <= 0 || info.Size() > 4096 {
		return value, sandboxFailure("staging marker size")
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(data) > 4096 || json.Unmarshal(data, &value) != nil {
		return stageManifest{}, sandboxFailure("staging marker")
	}
	canonical, err := json.Marshal(value)
	// Only our exact v1 encoding is authority: reject duplicate/unknown fields,
	// trailing values and partially written or externally rewritten markers.
	if err != nil || !bytes.Equal(data, canonical) {
		return stageManifest{}, sandboxFailure("staging marker")
	}
	hash, err := hex.DecodeString(value.SHA256)
	if value.Format != stageManifestFormat || value.Version != 1 || !validStageName(value.Name) || value.Directory.File == 0 || value.Executable.File == 0 || value.Size <= 0 || value.Size > stageFileLimit || err != nil || len(hash) != sha256.Size || hex.EncodeToString(hash) != value.SHA256 {
		return stageManifest{}, sandboxFailure("staging marker")
	}
	return value, nil
}
func writeStageManifest(root *os.Root, name string, value stageManifest, known map[string]stageIdentity) error {
	if !validStageName(name) || value.Name != name || value.Format != stageManifestFormat || value.Version != 1 {
		return sandboxFailure("staging marker encoding")
	}
	data, err := json.Marshal(value)
	if err != nil || len(data) > 4096 {
		return sandboxFailure("staging marker encoding")
	}
	file, err := root.OpenFile(name+string(os.PathSeparator)+stageOwnerFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return sandboxFailure("staging marker creation")
	}
	defer file.Close()
	identity, err := stageIdentityOf(windows.Handle(file.Fd()), false)
	if err != nil {
		return err
	}
	known[stageOwnerFile] = identity
	if n, err := file.Write(data); err != nil || n != len(data) {
		return sandboxFailure("staging marker write")
	}
	if file.Sync() != nil {
		return sandboxFailure("staging marker sync")
	}
	return nil
}
func reapStage(ctx context.Context, parent windows.Handle, name string, sid *windows.SID, remaining *int64) (removed, busy bool) {
	if !validStageName(name) || ctx.Err() != nil {
		return false, false
	}
	directory, err := openStageObject(parent, name, true)
	if err != nil {
		return false, errors.Is(err, windows.STATUS_SHARING_VIOLATION) // active delete-sharing pin
	}
	defer directory.Close()
	handle := windows.Handle(directory.Fd())
	if !stageACLMatches(handle, sid, true) {
		return false, false
	}
	identity, err := stageIdentityOf(handle, true)
	if err != nil {
		return false, false
	}
	entries, err := stageEntries(handle)
	if err != nil {
		return false, false
	}
	marker, err := openStageObject(handle, stageOwnerFile, false)
	if err != nil {
		return false, false
	}
	defer marker.Close()
	if !stageACLMatches(windows.Handle(marker.Fd()), sid, false) {
		return false, false
	}
	manifest, err := readStageManifest(marker)
	if err != nil || manifest.Name != name || manifest.Directory != identity {
		return false, false
	}
	hasExe := false
	for _, entry := range entries {
		if entry.Name() == "worker.exe" {
			hasExe = true
		}
	}
	if hasExe {
		if manifest.Size > *remaining {
			return false, false
		}
		executable, err := openStageObject(handle, "worker.exe", false)
		if err != nil {
			return false, true
		}
		defer executable.Close()
		info, statErr := executable.Stat()
		fileID, idErr := stageIdentityOf(windows.Handle(executable.Fd()), false)
		if statErr != nil || idErr != nil || info.Size() != manifest.Size || fileID != manifest.Executable || !stageACLMatches(windows.Handle(executable.Fd()), sid, false) {
			return false, false
		}
		*remaining -= manifest.Size
		hash := sha256.New()
		// The open handle excludes writers/replacement. Bound actual I/O by the
		// charged size and confirm the whole file still has that exact length.
		n, copyErr := io.Copy(hash, &workerContextReader{ctx, io.LimitReader(executable, manifest.Size)})
		after, afterErr := executable.Stat()
		if copyErr != nil || n != manifest.Size || afterErr != nil || after.Size() != manifest.Size || hex.EncodeToString(hash.Sum(nil)) != manifest.SHA256 || ctx.Err() != nil {
			return false, false
		}
		if deleteStageHandle(windows.Handle(executable.Fd())) != nil {
			return false, true
		}
		executable.Close()
		if !stageFileGone(handle, "worker.exe") {
			return false, true
		}
	}
	if ctx.Err() != nil {
		return false, false
	}
	if deleteStageHandle(windows.Handle(marker.Fd())) != nil {
		return false, false
	}
	marker.Close()
	if deleteStageHandle(handle) != nil {
		return false, false
	}
	return true, false
}
func reapStagedWorkers(ctx context.Context, root *os.Root, sid *windows.SID) stageReapReport {
	var report stageReapReport
	if ctx.Err() != nil {
		return stageReapReport{Bounded: true}
	}
	limited, cancel := context.WithTimeout(ctx, stageReapTime)
	defer cancel()
	scan, err := root.Open(".")
	if err != nil {
		return report
	}
	entries, readErr := scan.ReadDir(64)
	scan.Close()
	if readErr != nil && readErr != io.EOF {
		return report
	}
	parent, err := root.Open(".")
	if err != nil {
		return report
	}
	defer parent.Close()
	remaining := int64(stageReapBytes)
	for _, entry := range entries {
		if limited.Err() != nil || remaining <= 0 {
			report.Bounded = true
			break
		}
		if !strings.HasPrefix(entry.Name(), stagePrefix) {
			continue
		}
		removed, busy := reapStage(limited, windows.Handle(parent.Fd()), entry.Name(), sid, &remaining)
		switch {
		case removed:
			report.Removed++
		case busy:
			report.Busy++
		default:
			report.Unproven++
		}
	}
	return report
}

// The creator can remove partial copies by their captured file identities while
// still owning the directory pin. Never recurse or remove an unknown entry.
func cleanupOwnedStage(directory windows.Handle, expected stageIdentity, known map[string]stageIdentity) bool {
	identity, err := stageIdentityOf(directory, true)
	if err != nil || identity != expected {
		return false
	}
	entries, err := stageEntries(directory)
	if err != nil {
		return false
	}
	files := make(map[string]*os.File, len(entries))
	defer func() {
		for _, file := range files {
			file.Close()
		}
	}()
	for _, entry := range entries {
		want, ok := known[entry.Name()]
		if !ok {
			return false
		}
		file, err := openStageObject(directory, entry.Name(), false)
		if err != nil {
			return false
		}
		files[entry.Name()] = file
		actual, err := stageIdentityOf(windows.Handle(file.Fd()), false)
		if err != nil || actual != want {
			return false
		}
	}
	// Preserve recovery authority if an exiting image still prevents deletion.
	for _, name := range []string{"worker.exe", stageOwnerFile} {
		file := files[name]
		if file == nil {
			continue
		}
		if deleteStageHandle(windows.Handle(file.Fd())) != nil {
			return false
		}
		file.Close()
		if name == "worker.exe" && !stageFileGone(directory, name) {
			return false
		}
	}
	return true
}

// Keep the original read-only live pin unchanged. After releasing it, reopen
// only this proven empty directory for deletion and check its identity again.
func deleteEmptyOwnedStage(root *os.Root, name string, expected stageIdentity) {
	parent, err := root.Open(".")
	if err != nil {
		return
	}
	defer parent.Close()
	directory, err := openStageObject(windows.Handle(parent.Fd()), name, true)
	if err != nil {
		return
	}
	defer directory.Close()
	identity, err := stageIdentityOf(windows.Handle(directory.Fd()), true)
	if err != nil || identity != expected {
		return
	}
	entries, err := stageEntries(windows.Handle(directory.Fd()))
	if err != nil || len(entries) != 0 {
		return
	}
	_ = deleteStageHandle(windows.Handle(directory.Fd()))
}
