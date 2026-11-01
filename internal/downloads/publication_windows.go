package downloads

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"runtime"
	"unsafe"
)

func identityOf(file *os.File) (fileIdentity, error) {
	return identityOfHandle(windows.Handle(file.Fd()))
}
func identityOfHandle(handle windows.Handle) (fileIdentity, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return fileIdentity{}, storageFailure(err)
	}
	index := uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || index == 0 {
		return fileIdentity{}, ErrPublicationUnsupported
	}
	return fileIdentity{Volume: uint64(info.VolumeSerialNumber), File: index, Created: info.CreationTime.Nanoseconds()}, nil
}
func ntError(err error) error {
	if status, ok := err.(windows.NTStatus); ok {
		return status.Errno()
	}
	return err
}
func renameNoReplace(dir *os.File, source, target string, expected fileIdentity) error {
	name, err := windows.NewNTUnicodeString(source)
	if err != nil {
		return err
	}
	attributes := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: windows.Handle(dir.Fd()), ObjectName: name, Attributes: windows.OBJ_CASE_INSENSITIVE}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, windows.DELETE|windows.SYNCHRONIZE|windows.FILE_READ_ATTRIBUTES, &attributes, &status, nil, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN, windows.FILE_OPEN_REPARSE_POINT|windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_FOR_BACKUP_INTENT, 0, 0)
	if err != nil {
		return ntError(err)
	}
	defer windows.CloseHandle(handle)
	identity, err := identityOfHandle(handle)
	if err != nil || identity != expected {
		return ErrPublishedChanged
	}
	encoded, err := windows.UTF16FromString(target)
	if err != nil {
		return err
	}
	// Same handle-relative layout as Go's Root.Rename, but ReplaceIfExists is
	// deliberately zero and no POSIX/replacing fallback is permitted.
	info := struct {
		ReplaceIfExists byte
		RootDirectory   windows.Handle
		FileNameLength  uint32
		FileName        [260]uint16
	}{RootDirectory: windows.Handle(dir.Fd()), FileNameLength: uint32((len(encoded) - 1) * 2)}
	if len(encoded) > len(info.FileName) {
		return ErrDownload
	}
	copy(info.FileName[:], encoded)
	err = windows.NtSetInformationFile(handle, &status, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), windows.FileRenameInformation)
	runtime.KeepAlive(name)
	runtime.KeepAlive(dir)
	return ntError(err)
}
func syncPublishedDirectory(*os.File) error { return nil }
func platformStorageFailure(err error) error {
	switch {
	case errors.Is(err, windows.ERROR_DISK_FULL), errors.Is(err, windows.ERROR_HANDLE_DISK_FULL):
		return ErrDiskFull
	case errors.Is(err, windows.ERROR_NOT_READY), errors.Is(err, windows.ERROR_DEVICE_NOT_CONNECTED), errors.Is(err, windows.ERROR_PATH_NOT_FOUND), errors.Is(err, windows.ERROR_DEV_NOT_EXIST), errors.Is(err, windows.ERROR_IO_DEVICE):
		return ErrDirectoryUnavailable
	case errors.Is(err, windows.ERROR_ACCESS_DENIED), errors.Is(err, windows.ERROR_WRITE_PROTECT):
		return ErrDirectoryPermission
	case errors.Is(err, windows.ERROR_NOT_SUPPORTED), errors.Is(err, windows.ERROR_INVALID_FUNCTION), errors.Is(err, windows.ERROR_INVALID_PARAMETER):
		return ErrPublicationUnsupported
	default:
		return ErrDownload
	}
}
