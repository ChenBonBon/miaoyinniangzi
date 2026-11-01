package winbundle

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type Manifest struct {
	DevelopmentOnly bool                `json:"developmentOnly"`
	Architecture    string              `json:"architecture"`
	GoVersion       string              `json:"goVersion"`
	Files           map[string]string   `json:"sha256"`
	PE              map[string]PEReport `json:"pe"`
}

func Package(repo, exe, destination string, lock Lock) (string, error) {
	parent := filepath.Join(repo, "build", "bin")
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(parent, absolute)
	if err != nil || relative == "." || strings.ContainsAny(relative, "/\\") || !strings.HasPrefix(relative, "windows-x64-") {
		return "", errors.New("output must be a new windows-x64-* folder under build/bin")
	}
	if _, err = os.Lstat(absolute); !os.IsNotExist(err) {
		return "", errors.New("output already exists or cannot be inspected")
	}
	dependencies := filepath.Join(repo, "build", "deps", "windows-x64")
	reports, err := Verify(dependencies, lock)
	if err != nil {
		return "", err
	}
	report, err := InspectPE(exe, false, nil)
	if err != nil {
		return "", err
	}
	allowed := map[string]bool{"libmpv-2.dll": true, "vulkan-1.dll": true}
	for _, name := range lock.SystemDLLs {
		allowed[strings.ToLower(name)] = true
	}
	for _, name := range report.Imports {
		if !allowed[name] {
			return "", fmt.Errorf("unbundled application import: %s", name)
		}
	}
	reports["miaoyinniangzi.exe"] = report
	if err = os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(parent, ".windows-package-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	copies := map[string]string{
		"miaoyinniangzi.exe": exe, "libmpv-2.dll": filepath.Join(dependencies, "libmpv-2.dll"), "vulkan-1.dll": filepath.Join(dependencies, "vulkan-1.dll"),
		"VulkanRT-License.txt": filepath.Join(dependencies, "VulkanRT-License.txt"), "windows-dependencies.json": filepath.Join(repo, "tools", "windows-dependencies.json"),
		"mpv-Copyright.txt": filepath.Join(repo, "docs", "third-party", "windows", "mpv-Copyright.txt"), "mpv-LICENSE-GPL.txt": filepath.Join(repo, "docs", "third-party", "windows", "mpv-LICENSE-GPL.txt"),
		"WINDOWS-TESTING.md": filepath.Join(repo, "docs", "windows-testing.md"), "THIRD-PARTY-WINDOWS.md": filepath.Join(repo, "docs", "third-party", "windows.md"),
	}
	for target, source := range copies {
		if err = copyExclusive(source, filepath.Join(stage, target), 256<<20); err != nil {
			return "", fmt.Errorf("cannot package %s: %w", target, err)
		}
	}
	if _, err = Verify(stage, lock); err != nil {
		return "", err
	}
	manifest := Manifest{DevelopmentOnly: true, Architecture: "windows/amd64", GoVersion: runtime.Version(), Files: map[string]string{}, PE: reports}
	for name := range copies {
		hash, err := FileHash(filepath.Join(stage, name))
		if err != nil {
			return "", err
		}
		manifest.Files[name] = hash
	}
	encoded, _ := json.MarshalIndent(manifest, "", "  ")
	if err = os.WriteFile(filepath.Join(stage, "manifest.json"), encoded, 0600); err != nil {
		return "", err
	}
	if err = os.Rename(stage, absolute); err != nil {
		return "", err
	}
	archivePath := absolute + ".zip"
	output, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	writer := zip.NewWriter(output)
	names := []string{"manifest.json"}
	for name := range copies {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry, createErr := writer.Create(name)
		if createErr != nil {
			err = createErr
			break
		}
		data, readErr := os.ReadFile(filepath.Join(absolute, name))
		if readErr != nil {
			err = readErr
			break
		}
		if _, err = entry.Write(data); err != nil {
			break
		}
	}
	zipErr := writer.Close()
	syncErr := output.Sync()
	closeErr := output.Close()
	if err != nil || zipErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(archivePath)
		return "", errors.New("portable archive creation failed; unpacked build remains available")
	}
	return archivePath, nil
}
