package sourceruntime

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var profileMu sync.Mutex

func containerProfile() (string, *windows.SID, error) {
	profileMu.Lock()
	defer profileMu.Unlock()
	name, _ := windows.UTF16PtrFromString(containerName)
	create, folder := userenv.NewProc("CreateAppContainerProfile"), userenv.NewProc("GetAppContainerFolderPath")
	if create.Find() != nil || folder.Find() != nil {
		return "", nil, errWindowsSandbox
	}
	var created *windows.SID
	result, _, _ := create.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(name)), 0, 0, uintptr(unsafe.Pointer(&created)))
	if created != nil {
		windows.FreeSid(created)
	}
	if int32(result) < 0 && uint32(result) != 0x800700b7 {
		return "", nil, errWindowsSandbox
	}
	sid, err := deriveContainerSID()
	if err != nil {
		return "", nil, err
	}
	sidText, _ := windows.UTF16PtrFromString(sid.String())
	var path *uint16
	result, _, _ = folder.Call(uintptr(unsafe.Pointer(sidText)), uintptr(unsafe.Pointer(&path)))
	if int32(result) < 0 || path == nil {
		return "", nil, errWindowsSandbox
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(path))
	location := windows.UTF16PtrToString(path)
	if !filepath.IsAbs(location) {
		return "", nil, errWindowsSandbox
	}
	if err = protectContainerStorage(location, sid); err != nil {
		return "", nil, err
	}
	return location, sid, nil
}

// The dedicated profile's package directory and AC directory are read-only to
// its SID. This also prevents a peer worker from replacing staging ancestors.
// Never change permissions on the user's AppData or other package directories.
func protectContainerStorage(folder string, sid *windows.SID) error {
	local, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return errWindowsSandbox
	}
	packages := filepath.Join(local, "Packages")
	relative, err := filepath.Rel(packages, folder)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return sandboxFailure("profile layout")
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) < 1 || len(parts) > 2 || (len(parts) == 2 && !strings.EqualFold(parts[1], "AC")) {
		return sandboxFailure("profile layout")
	}
	if !strings.EqualFold(parts[0], containerName) && !strings.EqualFold(parts[0], sid.String()) {
		return sandboxFailure("profile identity path")
	}

	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return errWindowsSandbox
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;GRGX;;;" + sid.String() + ")(A;OICI;RC;;;OW)")
	if err != nil {
		return errWindowsSandbox
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		return errWindowsSandbox
	}
	paths := []string{filepath.Join(packages, parts[0])}
	if len(parts) == 2 {
		paths = append(paths, folder)
	}
	for _, path := range paths {
		name, _ := windows.UTF16PtrFromString(path)
		handle, openErr := windows.CreateFile(name, windows.READ_CONTROL|windows.WRITE_DAC|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if openErr != nil {
			return sandboxFailure("profile ACL")
		}
		var info windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
			windows.CloseHandle(handle)
			return sandboxFailure("profile reparse point")
		}
		setErr := windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
		windows.CloseHandle(handle)
		if setErr != nil {
			return sandboxFailure("profile ACL")
		}
	}
	runtime.KeepAlive(descriptor)
	return nil
}

type stagedExecutable struct {
	path       string
	sid        *windows.SID
	nativeName string
	cleanup    func()
}

// CreateProcess resolves a pathname, not our pinned file handle. Before resume,
// compare the actual image section's native name with the pinned, normalized
// file name. A container-storage junction swap must not select another image.
func (s *stagedExecutable) verifyImage(process windows.Handle) error {
	name := make([]uint16, 32768)
	size := uint32(len(name))
	if windows.QueryFullProcessImageName(process, 1, &name[0], &size) != nil || windows.UTF16ToString(name[:size]) != s.nativeName {
		return sandboxFailure("image verification")
	}
	return nil
}

// Staging contains public executable bytes only, never scripts or credentials.
func stageWorker(ctx context.Context, source string) (stage *stagedExecutable, err error) {
	folder, sid, err := containerProfile()
	if err != nil {
		return nil, sandboxFailure("profile")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if noLoopbackExemption(sid) != nil {
		return nil, sandboxFailure("loopback exemption")
	}
	info, err := os.Lstat(folder)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, sandboxFailure("profile directory")
	}
	root, err := os.OpenRoot(folder)
	if err != nil {
		return nil, sandboxFailure("profile directory")
	}
	var directory windows.Handle
	var pinned *os.File
	name := ""
	cleanup := func() {
		if pinned != nil {
			pinned.Close()
		}
		if directory != 0 {
			windows.CloseHandle(directory)
		}
		if name != "" {
			_ = root.RemoveAll(name)
		}
		root.Close()
	}
	defer func() {
		if err != nil {
			cleanup()
		}
	}()
	scan, err := root.Open(".")
	if err != nil {
		return nil, sandboxFailure("profile enumeration")
	}
	entries, readErr := scan.ReadDir(64)
	scan.Close()
	if readErr != nil && readErr != io.EOF || len(entries) >= 64 {
		return nil, sandboxFailure("staging directory limit")
	}
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "miaoyin-worker-") {
			count++
		}
	}
	// Abrupt parent termination can leave a public binary copy. Bound those copies
	// and fail closed, rather than deleting another application's live worker.
	if count >= 16 {
		return nil, sandboxFailure("staging directory limit")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, errWindowsSandbox
	}
	// OWNER RIGHTS prevents an implicit owner WRITE_DAC grant in the lowbox.
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;GRGX;;;" + sid.String() + ")(A;OICI;RC;;;OW)")
	if err != nil {
		return nil, errWindowsSandbox
	}
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		return nil, errWindowsSandbox
	}
	candidate := "miaoyin-worker-" + hex.EncodeToString(nonce)
	parent, err := root.Open(".")
	if err != nil {
		return nil, errWindowsSandbox
	}
	ntName, _ := windows.NewNTUnicodeString(candidate)
	attributes := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: windows.Handle(parent.Fd()), ObjectName: ntName, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE, SecurityDescriptor: descriptor}
	var status windows.IO_STATUS_BLOCK
	// Atomic protected creation + a non-delete-sharing handle closes the
	// mkdir/chmod/rename race with other workers using the same package SID.
	err = windows.NtCreateFile(&directory, windows.FILE_GENERIC_READ, &attributes, &status, nil, windows.FILE_ATTRIBUTE_DIRECTORY, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, windows.FILE_CREATE, windows.FILE_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT, 0, 0)
	runtime.KeepAlive(descriptor)
	runtime.KeepAlive(ntName)
	parent.Close()
	if err != nil {
		return nil, sandboxFailure("protected staging")
	}
	name = candidate
	sourceName, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return nil, errWindowsSandbox
	}
	sourceHandle, err := windows.CreateFile(sourceName, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, sandboxFailure("source executable")
	}
	input := os.NewFile(uintptr(sourceHandle), source)
	defer input.Close()
	image, imageErr := pe.NewFile(input)
	if imageErr != nil {
		return nil, sandboxFailure("worker executable PE")
	}
	header, ok := image.OptionalHeader.(*pe.OptionalHeader64)
	if !ok || header.Subsystem != pe.IMAGE_SUBSYSTEM_WINDOWS_GUI {
		return nil, sandboxFailure("worker executable requires GUI subsystem (-H=windowsgui)")
	}

	var sourceInfo windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(sourceHandle, &sourceInfo) != nil || sourceInfo.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return nil, errWindowsSandbox
	}
	info, err = input.Stat()
	if err != nil || info.Size() <= 0 || info.Size() > 256<<20 {
		return nil, errWindowsSandbox
	}
	output, err := root.OpenFile(filepath.Join(name, "worker.exe"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, sandboxFailure("staged executable")
	}
	digest := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(output, digest), &workerContextReader{ctx, io.LimitReader(input, 256<<20+1)})
	syncErr, closeErr := output.Sync(), output.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || written != info.Size() || ctx.Err() != nil {
		return nil, sandboxFailure("executable copy")
	}
	executable := filepath.Join(folder, name, "worker.exe")
	targetName, _ := windows.UTF16PtrFromString(executable)
	targetHandle, err := windows.CreateFile(targetName, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, sandboxFailure("executable pin")
	}
	pinned = os.NewFile(uintptr(targetHandle), executable)
	check := sha256.New()
	if _, err = io.Copy(check, &workerContextReader{ctx, pinned}); err != nil || !bytes.Equal(check.Sum(nil), digest.Sum(nil)) {
		return nil, sandboxFailure("executable hash")
	}
	path := make([]uint16, 32768)
	size, pathErr := windows.GetFinalPathNameByHandle(targetHandle, &path[0], uint32(len(path)), 2) // VOLUME_NAME_NT, normalized.
	if pathErr != nil || size == 0 || int(size) >= len(path) {
		return nil, sandboxFailure("pinned image name")
	}
	return &stagedExecutable{path: executable, sid: sid, nativeName: windows.UTF16ToString(path[:size]), cleanup: cleanup}, nil
}

type workerContextReader struct {
	ctx   context.Context
	input io.Reader
}

func (r *workerContextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.input.Read(data)
}
