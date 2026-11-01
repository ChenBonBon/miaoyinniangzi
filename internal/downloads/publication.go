package downloads

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type fileIdentity struct {
	Volume   uint64 `json:"volume"`
	File     uint64 `json:"file"`
	Created  int64  `json:"created,omitempty"`
	Modified int64  `json:"modified"`
}

func validPublicationName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsAny(name, ":/\\\x00")
}
func publish(root *os.Root, source, target string) error {
	if !validPublicationName(source) || !validPublicationName(target) || source == target {
		return ErrDownload
	}
	file, err := regular(root, source, true, false)
	if err != nil {
		return err
	}
	defer file.Close()
	identity, err := identityOf(file)
	if err != nil {
		return err
	}
	return publishFile(root, file, identity, source, target)
}
func publishFile(root *os.Root, file *os.File, identity fileIdentity, source, target string) error {
	if !validPublicationName(source) || !validPublicationName(target) || source == target {
		return ErrDownload
	}
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() {
		return ErrDownload
	}
	named, err := root.Lstat(source)
	if err != nil || !named.Mode().IsRegular() || !os.SameFile(before, named) {
		return ErrDownload
	}
	dir, err := root.Open(".")
	if err != nil {
		return storageFailure(err)
	}
	defer dir.Close()
	if err = renameNoReplace(dir, source, target, identity); err != nil {
		if errors.Is(err, os.ErrExist) {
			return os.ErrExist
		}
		return storageFailure(err)
	}
	targetFile, err := regular(root, target, false, false)
	if err != nil {
		return ErrPublishedChanged
	}
	defer targetFile.Close()
	actual, err := identityOf(targetFile)
	if err != nil || actual != identity {
		return ErrPublishedChanged
	}
	// The caller's original open file remains valid after the rename.
	if err = file.Sync(); err != nil {
		return storageFailure(err)
	}
	return syncPublishedDirectory(dir)
}
