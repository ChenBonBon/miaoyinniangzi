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
	_, err := publishFileIdentity(root, file, identity, source, target)
	return err
}

// Returns only an identity proven through the live exclusively-renamed source,
// not one inferred from an unrelated target with matching content.
func publishFileIdentity(root *os.Root, file *os.File, identity fileIdentity, source, target string) (fileIdentity, error) {
	if !validPublicationName(source) || !validPublicationName(target) || source == target {
		return fileIdentity{}, ErrDownload
	}
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() {
		return fileIdentity{}, ErrDownload
	}
	named, err := root.Lstat(source)
	if err != nil || !named.Mode().IsRegular() || !os.SameFile(before, named) {
		return fileIdentity{}, ErrDownload
	}
	dir, err := root.Open(".")
	if err != nil {
		return fileIdentity{}, storageFailure(err)
	}
	defer dir.Close()
	published, err := renameNoReplaceIdentity(dir, source, target, identity)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fileIdentity{}, os.ErrExist
		}
		return fileIdentity{}, storageFailure(err)
	}
	targetFile, err := regular(root, target, false, false)
	if err != nil {
		return fileIdentity{}, ErrPublishedChanged
	}
	defer targetFile.Close()
	actual, err := identityOf(targetFile)
	if err != nil || actual != published {
		return fileIdentity{}, ErrPublishedChanged
	}
	if err = file.Sync(); err != nil {
		return fileIdentity{}, storageFailure(err)
	}
	if err = syncPublishedDirectory(dir); err != nil {
		return fileIdentity{}, err
	}
	return published, nil
}
