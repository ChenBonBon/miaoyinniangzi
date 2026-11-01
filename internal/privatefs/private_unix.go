//go:build !windows

// Package privatefs enforces owner-private permissions using the host OS model.
package privatefs

import (
	"errors"
	"os"
)

func Protect(path string, mode os.FileMode) error       { return os.Chmod(path, mode) }
func ProtectFile(file *os.File, mode os.FileMode) error { return file.Chmod(mode) }
func Check(path string, mode os.FileMode) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != mode {
		return errors.New("object is not private")
	}
	return nil
}
