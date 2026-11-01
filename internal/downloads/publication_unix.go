//go:build darwin || linux

package downloads

import (
	"errors"
	"os"
	"syscall"
)

func identityOf(file *os.File) (fileIdentity, error) {
	stat, err := file.Stat()
	if err != nil {
		return fileIdentity{}, storageFailure(err)
	}
	info, ok := stat.Sys().(*syscall.Stat_t)
	if !ok || !stat.Mode().IsRegular() || info.Ino == 0 {
		return fileIdentity{}, ErrPublicationUnsupported
	}
	return fileIdentity{Volume: uint64(info.Dev), File: uint64(info.Ino), Created: creationTime(info), Modified: stat.ModTime().UnixNano()}, nil
}
func syncPublishedDirectory(dir *os.File) error {
	err := dir.Sync()
	// Some removable filesystems cannot fsync directories. File data was synced
	// before publication; never replace with a non-exclusive rename or copy.
	if err == nil || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) {
		return nil
	}
	return storageFailure(err)
}
func platformStorageFailure(err error) error {
	switch {
	case errors.Is(err, syscall.ENOSPC), errors.Is(err, syscall.EDQUOT):
		return ErrDiskFull
	case errors.Is(err, syscall.ENODEV), errors.Is(err, syscall.ENXIO), errors.Is(err, syscall.EIO), errors.Is(err, os.ErrNotExist):
		return ErrDirectoryUnavailable
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM), errors.Is(err, syscall.EROFS):
		return ErrDirectoryPermission
	case errors.Is(err, syscall.ENOTSUP), errors.Is(err, syscall.ENOSYS), errors.Is(err, syscall.EINVAL):
		return ErrPublicationUnsupported
	default:
		return ErrDownload
	}
}
