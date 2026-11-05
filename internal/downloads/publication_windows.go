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
func openPublicationHandle(dir *os.File, source string, expected fileIdentity) (windows.Handle, error) {
	name, err := windows.NewNTUnicodeString(source)
	if err != nil {
		return 0, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: windows.Handle(dir.Fd()), ObjectName: name, Attributes: windows.OBJ_CASE_INSENSITIVE}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, windows.DELETE|windows.SYNCHRONIZE|windows.FILE_READ_ATTRIBUTES|windows.FILE_WRITE_ATTRIBUTES, &attributes, &status, nil, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, windows.FILE_OPEN, windows.FILE_OPEN_REPARSE_POINT|windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_FOR_BACKUP_INTENT, 0, 0)
	runtime.KeepAlive(name)
	runtime.KeepAlive(dir)
	if err != nil {
		return 0, ntError(err)
	}
	// Deny delete sharing for the whole transition: no other handle can rename,
	// unlink or replace the proven object between its exclusive rename and query.
	identity, err := identityOfHandle(handle)
	if err != nil || identity != expected {
		windows.CloseHandle(handle)
		return 0, ErrPublishedChanged
	}
	return handle, nil
}
func renameNoReplaceIdentity(dir *os.File, source, target string, expected fileIdentity) (fileIdentity, error) {
	handle, err := openPublicationHandle(dir, source, expected)
	if err != nil {
		return fileIdentity{}, err
	}
	defer windows.CloseHandle(handle)
	var status windows.IO_STATUS_BLOCK
	encoded, err := windows.UTF16FromString(target)
	if err != nil {
		return fileIdentity{}, err
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
		return fileIdentity{}, ErrDownload
	}
	copy(info.FileName[:], encoded)
	err = windows.NtSetInformationFile(handle, &status, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), windows.FileRenameInformation)
	runtime.KeepAlive(dir)
	if err != nil {
		return fileIdentity{}, ntError(err)
	}
	return identityAfterExclusiveRename(handle, expected)
}

func identityAfterExclusiveRename(handle windows.Handle, expected fileIdentity) (fileIdentity, error) {
	actual, err := identityOfHandle(handle)
	if err != nil || actual.Volume != expected.Volume {
		return fileIdentity{}, ErrPublishedChanged
	}
	if actual.File == expected.File {
		if err = restorePublicationIdentity(handle, expected); err != nil {
			return fileIdentity{}, err
		}
		return expected, nil
	}
	// FAT32 FileIndex identifies a directory entry, which a long-name rename can
	// move. This exception is valid only here, after a successful no-replace rename
	// through the prevalidated still-open handle with delete sharing denied.
	var fs [64]uint16
	if windows.GetVolumeInformationByHandle(handle, nil, 0, nil, nil, nil, &fs[0], uint32(len(fs))) != nil || windows.UTF16ToString(fs[:]) != "FAT32" {
		return fileIdentity{}, ErrPublishedChanged
	}
	created := windows.NsecToFiletime(expected.Created)
	if windows.SetFileTime(handle, &created, nil, nil) != nil {
		return fileIdentity{}, ErrPublishedChanged
	}
	after, err := identityOfHandle(handle)
	if err != nil || after.Volume != actual.Volume || after.File != actual.File || after.Created != expected.Created {
		return fileIdentity{}, ErrPublishedChanged
	}
	return after, nil
}

// NTFS filename tunneling can replace the creation time during an exclusive
// rename into a recently deleted name. Restore only that metadata through the
// still-open, proven source handle; never relax the persisted identity check.
func restorePublicationIdentity(handle windows.Handle, expected fileIdentity) error {
	actual, err := identityOfHandle(handle)
	if err != nil || actual.Volume != expected.Volume || actual.File != expected.File {
		return ErrPublishedChanged
	}
	// Set explicitly even if this handle still reports its pre-rename metadata;
	// a newly opened target handle must observe exactly the persisted birth time.
	created := windows.NsecToFiletime(expected.Created)
	if windows.SetFileTime(handle, &created, nil, nil) != nil {
		return ErrPublishedChanged
	}
	actual, err = identityOfHandle(handle)
	if err != nil || actual != expected {
		return ErrPublishedChanged
	}
	return nil
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
