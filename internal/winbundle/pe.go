package winbundle

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type PEReport struct {
	Imports     []string `json:"imports"`
	ExportCount int      `json:"exportCount"`
}

func InspectPE(path string, dll bool, requiredExports []string) (PEReport, error) {
	result := PEReport{}
	file, err := pe.Open(path)
	if err != nil {
		return result, errors.New("invalid PE image")
	}
	defer file.Close()
	if file.Machine != pe.IMAGE_FILE_MACHINE_AMD64 || ((file.Characteristics&pe.IMAGE_FILE_DLL) != 0) != dll {
		return result, errors.New("PE image is not the expected Windows x64 type")
	}
	header, ok := file.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		return result, errors.New("64-bit PE optional header missing")
	}
	stat, err := os.Stat(path)
	if err != nil || stat.Size() > 256<<20 {
		return result, errors.New("PE image too large or unreadable")
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 256<<20 {
		return result, errors.New("PE image too large or unreadable")
	}
	read := func(rva uint32, length uint32) ([]byte, error) {
		for _, section := range file.Sections {
			if rva < section.VirtualAddress {
				continue
			}
			relative := uint64(rva - section.VirtualAddress)
			if relative+uint64(length) > uint64(section.Size) {
				continue
			}
			start := uint64(section.Offset) + relative
			if start+uint64(length) > uint64(len(data)) {
				break
			}
			return data[start : start+uint64(length)], nil
		}
		return nil, errors.New("PE address outside image")
	}
	text := func(rva uint32) (string, error) {
		for n := uint32(1); n <= 256; n++ {
			raw, err := read(rva, n)
			if err != nil {
				return "", err
			}
			if raw[n-1] == 0 {
				return string(raw[:n-1]), nil
			}
		}
		return "", errors.New("unterminated PE name")
	}
	libraries := map[string]bool{}
	for _, table := range []struct{ index, size, nameOffset uint32 }{{1, 20, 12}, {13, 32, 4}} {
		directory := header.DataDirectory[table.index]
		if directory.VirtualAddress == 0 {
			continue
		}
		ended := false
		for index := uint32(0); index < 256; index++ {
			row, err := read(directory.VirtualAddress+index*table.size, table.size)
			if err != nil {
				return result, err
			}
			if bytes.Equal(row, make([]byte, table.size)) {
				ended = true
				break
			}
			if table.index == 13 && binary.LittleEndian.Uint32(row[:4])&1 == 0 {
				return result, errors.New("unsupported legacy delay import addressing")
			}
			name, err := text(binary.LittleEndian.Uint32(row[table.nameOffset : table.nameOffset+4]))
			if err != nil {
				return result, err
			}
			name = strings.ToLower(name)
			if filepath.Base(name) != name || !strings.HasSuffix(name, ".dll") || strings.ContainsAny(name, "/\\:") {
				return result, errors.New("invalid imported DLL name")
			}
			libraries[name] = true
		}
		if !ended {
			return result, errors.New("import directory exceeds bound")
		}
	}
	exports := map[string]bool{}
	directory := header.DataDirectory[0]
	if directory.VirtualAddress != 0 {
		row, err := read(directory.VirtualAddress, 40)
		if err != nil {
			return result, err
		}
		count := binary.LittleEndian.Uint32(row[24:28])
		names := binary.LittleEndian.Uint32(row[32:36])
		if count > 65536 {
			return result, errors.New("export directory exceeds bound")
		}
		for i := uint32(0); i < count; i++ {
			entry, err := read(names+i*4, 4)
			if err != nil {
				return result, err
			}
			name, err := text(binary.LittleEndian.Uint32(entry))
			if err != nil {
				return result, err
			}
			exports[name] = true
		}
	}
	for _, name := range requiredExports {
		if !exports[name] {
			return result, fmt.Errorf("required native export missing: %s", name)
		}
	}
	for name := range libraries {
		result.Imports = append(result.Imports, name)
	}
	sort.Strings(result.Imports)
	result.ExportCount = len(exports)
	return result, nil
}

var mpvExports = []string{"mpv_client_api_version", "mpv_create", "mpv_initialize", "mpv_set_option_string", "mpv_command", "mpv_command_async", "mpv_get_property_string", "mpv_wait_event", "mpv_free", "mpv_terminate_destroy"}

func Verify(directory string, lock Lock) (map[string]PEReport, error) {
	if err := CheckHash(filepath.Join(directory, "VulkanRT-License.txt"), lock.Vulkan.LicenseSHA256); err != nil {
		return nil, err
	}
	result := map[string]PEReport{}
	allowed := map[string]bool{"libmpv-2.dll": true, "vulkan-1.dll": true}
	for _, name := range lock.SystemDLLs {
		allowed[strings.ToLower(name)] = true
	}
	for _, item := range []struct {
		name, hash string
		exports    []string
	}{{"libmpv-2.dll", lock.Libmpv.DLLSHA256, mpvExports}, {"vulkan-1.dll", lock.Vulkan.DLLSHA256, []string{"vkGetInstanceProcAddr"}}} {
		path := filepath.Join(directory, item.name)
		if err := CheckHash(path, item.hash); err != nil {
			return nil, fmt.Errorf("%s: %w", item.name, err)
		}
		report, err := InspectPE(path, true, item.exports)
		if err != nil {
			return nil, err
		}
		for _, library := range report.Imports {
			if !allowed[library] {
				return nil, fmt.Errorf("unbundled non-system dependency: %s", library)
			}
		}
		result[item.name] = report
	}
	return result, nil
}
