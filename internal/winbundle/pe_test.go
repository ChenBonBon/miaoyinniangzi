package winbundle

import (
	"archive/zip"
	"bytes"
	"debug/pe"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func syntheticPE(t *testing.T, dll bool, library string) string {
	t.Helper()
	data := make([]byte, 2048)
	copy(data, "MZ")
	binary.LittleEndian.PutUint32(data[0x3c:], 0x80)
	copy(data[0x80:], "PE\x00\x00")
	header := pe.FileHeader{Machine: pe.IMAGE_FILE_MACHINE_AMD64, NumberOfSections: 1, SizeOfOptionalHeader: 240, Characteristics: pe.IMAGE_FILE_EXECUTABLE_IMAGE}
	if dll {
		header.Characteristics |= pe.IMAGE_FILE_DLL
	}
	var buffer bytes.Buffer
	_ = binary.Write(&buffer, binary.LittleEndian, header)
	copy(data[0x84:], buffer.Bytes())
	optional := pe.OptionalHeader64{Magic: 0x20b, SectionAlignment: 4096, FileAlignment: 512, SizeOfImage: 8192, SizeOfHeaders: 512, NumberOfRvaAndSizes: 16}
	optional.DataDirectory[1] = pe.DataDirectory{VirtualAddress: 4096, Size: 40}
	optional.DataDirectory[0] = pe.DataDirectory{VirtualAddress: 4352, Size: 40}
	buffer.Reset()
	_ = binary.Write(&buffer, binary.LittleEndian, optional)
	copy(data[0x98:], buffer.Bytes())
	section := pe.SectionHeader32{VirtualSize: 1536, VirtualAddress: 4096, SizeOfRawData: 1536, PointerToRawData: 512}
	copy(section.Name[:], ".rdata")
	buffer.Reset()
	_ = binary.Write(&buffer, binary.LittleEndian, section)
	copy(data[0x188:], buffer.Bytes())
	binary.LittleEndian.PutUint32(data[512+12:], 4192)
	copy(data[608:], library+"\x00")
	binary.LittleEndian.PutUint32(data[768+24:], 1)
	binary.LittleEndian.PutUint32(data[768+32:], 4416)
	binary.LittleEndian.PutUint32(data[832:], 4432)
	copy(data[848:], "mpv_create\x00")
	path := filepath.Join(t.TempDir(), "test.dll")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestPEInspectionRejectsWrongArchitectureTypeAndMissingExports(t *testing.T) {
	path := syntheticPE(t, true, "KERNEL32.dll")
	report, err := InspectPE(path, true, []string{"mpv_create"})
	if err != nil || len(report.Imports) != 1 || report.Imports[0] != "kernel32.dll" {
		t.Fatal("valid PE import/export inspection failed", err)
	}
	if _, err = InspectPE(path, false, nil); err == nil {
		t.Fatal("DLL accepted as executable")
	}
	if _, err = InspectPE(path, true, []string{"missing_export"}); err == nil {
		t.Fatal("missing export accepted")
	}
	data, _ := os.ReadFile(path)
	binary.LittleEndian.PutUint16(data[0x84:], pe.IMAGE_FILE_MACHINE_I386)
	_ = os.WriteFile(path, data, 0600)
	if _, err = InspectPE(path, true, nil); err == nil {
		t.Fatal("32-bit dependency accepted")
	}
}
func TestPEInspectionRejectsMalformedAddressesAndImportPaths(t *testing.T) {
	path := syntheticPE(t, true, "../bad.dll")
	if _, err := InspectPE(path, true, nil); err == nil {
		t.Fatal("imported path accepted")
	}
	path = syntheticPE(t, true, "kernel32.dll")
	data, _ := os.ReadFile(path)
	binary.LittleEndian.PutUint32(data[512+12:], 0xffffff00)
	_ = os.WriteFile(path, data, 0600)
	if _, err := InspectPE(path, true, nil); err == nil {
		t.Fatal("out-of-image import accepted")
	}
}
func TestPEInspectionIncludesDelayLoadedDependencies(t *testing.T) {
	path := syntheticPE(t, true, "kernel32.dll")
	data, _ := os.ReadFile(path)
	binary.LittleEndian.PutUint32(data[0x98+112+13*8:], 0x1400)
	binary.LittleEndian.PutUint32(data[0x98+112+13*8+4:], 64)
	binary.LittleEndian.PutUint32(data[1536:], 1)
	binary.LittleEndian.PutUint32(data[1540:], 0x1450)
	copy(data[1616:], "missing-helper.dll\x00")
	_ = os.WriteFile(path, data, 0600)
	report, err := InspectPE(path, true, nil)
	if err != nil || len(report.Imports) != 2 || report.Imports[1] != "missing-helper.dll" {
		t.Fatal("delay-loaded dependency omitted", err)
	}
}

func TestChecksumAndExclusiveCopyFailClosed(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	target := filepath.Join(dir, "target")
	_ = os.WriteFile(source, []byte("one"), 0600)
	hash, _ := FileHash(source)
	if err := CheckHash(source, hash); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(source, []byte("changed"), 0600)
	if CheckHash(source, hash) == nil {
		t.Fatal("changed native dependency accepted")
	}
	_ = os.WriteFile(target, []byte("keep"), 0600)
	if copyExclusive(source, target, 100) == nil {
		t.Fatal("existing package file replaced")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "keep" {
		t.Fatal("existing output changed")
	}
}
func TestPackageRefusesOutsideOrExistingOutputWithoutDeletingFiles(t *testing.T) {
	repo := t.TempDir()
	if _, err := Package(repo, "missing", filepath.Join(repo, "outside"), Lock{}); err == nil {
		t.Fatal("outside build output accepted")
	}
	target := filepath.Join(repo, "build", "bin", "windows-x64-existing")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(target, "keep")
	_ = os.WriteFile(marker, []byte("original"), 0600)
	if _, err := Package(repo, "missing", target, Lock{}); err == nil {
		t.Fatal("existing output accepted")
	}
	if data, _ := os.ReadFile(marker); string(data) != "original" {
		t.Fatal("old package was removed")
	}
}

func TestZipExtractionSelectsOnlyThePinnedEntry(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "runtime.zip")
	file, _ := os.Create(archive)
	writer := zip.NewWriter(file)
	for name, value := range map[string]string{"../escape": "bad", "runtime/x64/vulkan-1.dll": "dll"} {
		entry, _ := writer.Create(name)
		_, _ = entry.Write([]byte(value))
	}
	_ = writer.Close()
	_ = file.Close()
	target := filepath.Join(dir, "vulkan-1.dll")
	if err := extractZipEntry(archive, "runtime/x64/vulkan-1.dll", target, 10); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "dll" {
		t.Fatal("wrong archive entry")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape")); !os.IsNotExist(err) {
		t.Fatal("archive traversal occurred")
	}
}
