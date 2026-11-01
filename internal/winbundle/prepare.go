package winbundle

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

func cacheDownload(path string, artifact Artifact) error {
	if _, err := os.Lstat(path); err == nil {
		return CheckHash(path, artifact.SHA256)
	} else if !os.IsNotExist(err) {
		return err
	}
	allowed := map[string]bool{"github.com": true, "release-assets.githubusercontent.com": true, "objects.githubusercontent.com": true, "www.7-zip.org": true, "7-zip.org": true, "sdk.lunarg.com": true}
	validURL := func(value string) bool {
		parsed, err := url.Parse(value)
		return err == nil && parsed.Scheme == "https" && allowed[parsed.Hostname()] && parsed.User == nil && (parsed.Port() == "" || parsed.Port() == "443")
	}
	if !validURL(artifact.URL) {
		return errors.New("dependency origin is not permitted")
	}
	client := &http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) > 8 || !validURL(request.URL.String()) {
			return errors.New("dependency redirect refused")
		}
		return nil
	}}
	response, err := client.Get(artifact.URL)
	if err != nil {
		return errors.New("dependency download failed; check access to the documented upstream")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.ContentLength > artifact.MaxBytes {
		return errors.New("dependency response rejected")
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".download-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	count, copyErr := io.Copy(temporary, io.LimitReader(response.Body, artifact.MaxBytes+1))
	syncErr := temporary.Sync()
	closeErr := temporary.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || count > artifact.MaxBytes {
		return errors.New("dependency download was incomplete or exceeded its limit")
	}
	if err = CheckHash(name, artifact.SHA256); err != nil {
		return err
	}
	return os.Rename(name, path)
}
func copyExclusive(source, target string, limit int64) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	return writeExclusive(input, target, limit)
}
func writeExclusive(input io.Reader, target string, limit int64) error {
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(output, io.LimitReader(input, limit+1))
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || n > limit {
		_ = os.Remove(target)
		return errors.New("bundle file copy failed or exceeded limit")
	}
	return nil
}
func extractZipEntry(archive, entry, target string, max int64) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	found := false
	for _, file := range reader.File {
		if file.Name != entry {
			continue
		}
		if found || file.UncompressedSize64 > uint64(max) || !file.Mode().IsRegular() {
			return errors.New("unexpected runtime archive entry")
		}
		found = true
		input, err := file.Open()
		if err != nil {
			return err
		}
		err = writeExclusive(input, target, max)
		input.Close()
		if err != nil {
			return err
		}
	}
	if !found {
		return errors.New("pinned archive entry missing")
	}
	return nil
}
func Prepare(repo string, lock Lock, localMPV string) (string, error) {
	cache := filepath.Join(repo, "build", "deps", "windows-x64")
	if err := os.MkdirAll(cache, 0700); err != nil {
		return "", err
	}
	dll := filepath.Join(cache, "libmpv-2.dll")
	if _, err := os.Lstat(dll); os.IsNotExist(err) {
		stage, err := os.MkdirTemp(cache, ".extract-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(stage)
		if localMPV != "" {
			if err = CheckHash(localMPV, lock.Libmpv.DLLSHA256); err != nil {
				return "", err
			}
			if err = copyExclusive(localMPV, filepath.Join(stage, "libmpv-2.dll"), 256<<20); err != nil {
				return "", err
			}
		} else {
			if runtime.GOOS != "windows" {
				return "", errors.New("7z extraction runs on Windows; use --libmpv with a separately extracted, pinned DLL for cross-host verification")
			}
			archive := filepath.Join(cache, "libmpv-"+lock.Libmpv.Version+".7z")
			extractor := filepath.Join(cache, "7zr-26.04.exe")
			if err = cacheDownload(archive, lock.Libmpv); err != nil {
				return "", err
			}
			if err = cacheDownload(extractor, lock.Extractor); err != nil {
				return "", err
			}
			command := exec.Command(extractor, "x", archive, "-o"+stage, "-y", "libmpv-2.dll")
			command.Stdout = io.Discard
			command.Stderr = io.Discard
			if err = command.Run(); err != nil {
				return "", errors.New("verified 7z extractor failed")
			}
		}
		extracted := filepath.Join(stage, "libmpv-2.dll")
		if err = CheckHash(extracted, lock.Libmpv.DLLSHA256); err != nil {
			return "", err
		}
		if err = os.Rename(extracted, dll); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	if err := CheckHash(dll, lock.Libmpv.DLLSHA256); err != nil {
		return "", err
	}
	archive := filepath.Join(cache, "VulkanRT-"+lock.Vulkan.Version+".zip")
	if err := cacheDownload(archive, lock.Vulkan); err != nil {
		return "", err
	}
	for _, item := range []struct {
		entry, target string
		max           int64
	}{{lock.Vulkan.DLLEntry, "vulkan-1.dll", 16 << 20}, {lock.Vulkan.LicenseEntry, "VulkanRT-License.txt", 65536}} {
		target := filepath.Join(cache, item.target)
		if _, err := os.Lstat(target); os.IsNotExist(err) {
			if err = extractZipEntry(archive, item.entry, target, item.max); err != nil {
				return "", err
			}
		} else if err != nil {
			return "", err
		}
	}
	if err := CheckHash(filepath.Join(cache, "VulkanRT-License.txt"), lock.Vulkan.LicenseSHA256); err != nil {
		return "", err
	}
	if _, err := Verify(cache, lock); err != nil {
		return "", fmt.Errorf("native dependency verification failed: %w", err)
	}
	return cache, nil
}
