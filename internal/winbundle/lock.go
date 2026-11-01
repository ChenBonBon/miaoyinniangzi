// Package winbundle prepares a pinned, app-local Windows development bundle.
package winbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
)

type Artifact struct {
	Version       string `json:"version"`
	URL           string `json:"url"`
	SHA256        string `json:"sha256"`
	MaxBytes      int64  `json:"maxBytes"`
	DLLSHA256     string `json:"dllSha256,omitempty"`
	ClientAPI     string `json:"clientApi,omitempty"`
	DLLEntry      string `json:"dllEntry,omitempty"`
	LicenseEntry  string `json:"licenseEntry,omitempty"`
	LicenseSHA256 string `json:"licenseSha256,omitempty"`
	Source        string `json:"source,omitempty"`
}
type Lock struct {
	Architecture string   `json:"architecture"`
	Libmpv       Artifact `json:"libmpv"`
	Extractor    Artifact `json:"extractor"`
	Vulkan       Artifact `json:"vulkan"`
	SystemDLLs   []string `json:"systemDlls"`
}

func ReadLock(path string) (Lock, error) {
	var value Lock
	file, err := os.Open(path)
	if err != nil {
		return value, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 65536))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&value); err != nil {
		return value, err
	}
	if value.Architecture != "amd64" {
		return value, errors.New("unsupported dependency architecture")
	}
	for _, item := range []Artifact{value.Libmpv, value.Extractor, value.Vulkan} {
		if len(item.SHA256) != 64 || item.MaxBytes <= 0 || item.MaxBytes > 256<<20 {
			return value, errors.New("invalid dependency lock")
		}
		if _, err = hex.DecodeString(item.SHA256); err != nil {
			return value, err
		}
	}
	return value, nil
}
func FileHash(path string) (string, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return "", errors.New("bundle input is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", errors.New("bundle input changed during verification")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func CheckHash(path, expected string) error {
	value, err := FileHash(path)
	if err != nil {
		return err
	}
	if value != expected {
		return errors.New("dependency checksum mismatch; refuse to use cached or supplied file")
	}
	return nil
}
