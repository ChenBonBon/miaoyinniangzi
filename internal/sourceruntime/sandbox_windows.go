package sourceruntime

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const EngineVersion = "lx-inspect-win-lpac-v1"
const containerName = "com.miaoyinniangzi.source.v1"
const jobMemoryLimit = 256 * 1024 * 1024
const jobCPUTime = 2 * 10000000 // Windows user CPU time, 100ns units.
const jobFlags = windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS | windows.JOB_OBJECT_LIMIT_PROCESS_TIME | windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY | windows.JOB_OBJECT_LIMIT_JOB_MEMORY | windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION

var errWindowsSandbox = errors.New("Windows source sandbox unavailable")
var userenv = windows.NewLazySystemDLL("userenv.dll")
var kernel = windows.NewLazySystemDLL("kernel32.dll")

func deriveContainerSID() (*windows.SID, error) {
	proc := userenv.NewProc("DeriveAppContainerSidFromAppContainerName")
	if proc.Find() != nil {
		return nil, errWindowsSandbox
	}
	name, _ := windows.UTF16PtrFromString(containerName)
	var sid *windows.SID
	result, _, _ := proc.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&sid)))
	if int32(result) < 0 || sid == nil {
		return nil, errWindowsSandbox
	}
	defer windows.FreeSid(sid)
	return sid.Copy()
}
func tokenValue(token windows.Token, class uint32) (uint32, error) {
	var value, size uint32
	if windows.GetTokenInformation(token, class, (*byte)(unsafe.Pointer(&value)), 4, &size) != nil || size != 4 {
		return 0, errWindowsSandbox
	}
	return value, nil
}
func validateContainerToken(process windows.Handle) error {
	var token windows.Token
	if windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token) != nil {
		return errWindowsSandbox
	}
	defer token.Close()
	if elevated, err := tokenValue(token, windows.TokenElevation); err != nil || elevated != 0 {
		return errWindowsSandbox
	}
	var integrity [256]byte
	var integritySize uint32
	if windows.GetTokenInformation(token, windows.TokenIntegrityLevel, &integrity[0], uint32(len(integrity)), &integritySize) != nil {
		return errWindowsSandbox
	}
	label := *(**windows.SID)(unsafe.Pointer(&integrity[0]))
	if label == nil || (label.String() != "S-1-16-4096" && label.String() != "S-1-16-0") {
		return errWindowsSandbox
	}
	runtime.KeepAlive(integrity)
	// TOKEN_INFORMATION_CLASS values from the Windows SDK.
	for _, class := range []uint32{29, 46} {
		value, err := tokenValue(token, class)
		if err != nil || value != 1 {
			return errWindowsSandbox
		}
	}
	var caps [4096]byte
	var size uint32
	if windows.GetTokenInformation(token, 30, &caps[0], uint32(len(caps)), &size) != nil || size < 4 || *(*uint32)(unsafe.Pointer(&caps[0])) != 0 {
		return errWindowsSandbox
	}
	var data [4096]byte
	if windows.GetTokenInformation(token, 31, &data[0], uint32(len(data)), &size) != nil || size < uint32(unsafe.Sizeof(uintptr(0))) {
		return errWindowsSandbox
	}
	sid := *(**windows.SID)(unsafe.Pointer(&data[0]))
	expected, err := deriveContainerSID()
	if err != nil || sid == nil || !sid.Equals(expected) {
		return errWindowsSandbox
	}
	runtime.KeepAlive(data)
	return nil
}
func sandboxWorker() error {
	if validateContainerToken(windows.CurrentProcess()) != nil {
		return sandboxFailure("worker token")
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if windows.QueryInformationJobObject(0, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil) != nil {
		return sandboxFailure("worker job query")
	}
	basic := limits.BasicLimitInformation
	if basic.LimitFlags&jobFlags != jobFlags || basic.LimitFlags&(windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK|windows.JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK) != 0 || basic.ActiveProcessLimit != 1 || basic.PerProcessUserTimeLimit != jobCPUTime || limits.ProcessMemoryLimit != jobMemoryLimit || limits.JobMemoryLimit != jobMemoryLimit {
		return sandboxFailure("worker job policy")
	}
	var ui windows.JOBOBJECT_BASIC_UI_RESTRICTIONS
	if windows.QueryInformationJobObject(0, windows.JobObjectBasicUIRestrictions, uintptr(unsafe.Pointer(&ui)), uint32(unsafe.Sizeof(ui)), nil) != nil || ui.UIRestrictionsClass != 0xff {
		return sandboxFailure("worker UI policy")
	}
	proc := kernel.NewProc("GetProcessMitigationPolicy")
	if proc.Find() != nil {
		return sandboxFailure("worker child policy API")
	}
	var policy uint32
	result, _, _ := proc.Call(uintptr(windows.CurrentProcess()), 13, uintptr(unsafe.Pointer(&policy)), 4)
	if result == 0 || policy&1 == 0 {
		return sandboxFailure("worker child policy")
	}
	executable, pathErr := os.Executable()
	if pathErr != nil {
		return sandboxFailure("worker image directory")
	}
	directory, _ := windows.UTF16PtrFromString(filepath.Dir(executable))
	// FILE_WRITE_DATA is FILE_ADD_FILE when opening a directory.
	for _, access := range []uint32{windows.WRITE_DAC, windows.FILE_WRITE_DATA} {
		handle, openErr := windows.CreateFile(directory, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if openErr == nil {
			windows.CloseHandle(handle)
		}
		if !errors.Is(openErr, windows.ERROR_ACCESS_DENIED) {
			return sandboxFailure("worker staging ACL")
		}
	}
	// Verify actual WFP denial before accepting any script. An offline route or
	// timeout is not evidence of isolation. No HTTP or source data is transmitted.
	for _, address := range []string{"127.0.0.1:9", "1.1.1.1:443"} {
		connection, networkErr := net.DialTimeout("tcp4", address, 500*time.Millisecond)
		if connection != nil {
			connection.Close()
		}
		if !errors.Is(networkErr, syscall.Errno(10013)) {
			return sandboxFailure("worker network policy")
		}
	}
	return nil
}
func residentBytes(pid int) (uint64, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		return 0, errWindowsSandbox
	}
	defer windows.CloseHandle(handle)
	counters := struct {
		Size, Faults                                                                                          uint32
		PeakWorkingSet, WorkingSet, PeakPaged, Paged, PeakNonPaged, NonPaged, PageFile, PeakPageFile, Private uintptr
	}{}
	counters.Size = uint32(unsafe.Sizeof(counters))
	proc := kernel.NewProc("K32GetProcessMemoryInfo")
	if proc.Find() != nil {
		return 0, errWindowsSandbox
	}
	result, _, _ := proc.Call(uintptr(handle), uintptr(unsafe.Pointer(&counters)), uintptr(counters.Size))
	if result == 0 {
		return 0, errWindowsSandbox
	}
	return uint64(counters.WorkingSet), nil
}
func noLoopbackExemption(sid *windows.SID) error {
	proc := windows.NewLazySystemDLL("FirewallAPI.dll").NewProc("NetworkIsolationGetAppContainerConfig")
	if proc.Find() != nil {
		return errWindowsSandbox
	}
	var count uint32
	var entries *windows.SIDAndAttributes
	result, _, _ := proc.Call(uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&entries)))
	if result != 0 {
		return errWindowsSandbox
	}
	heapProc, freeProc := kernel.NewProc("GetProcessHeap"), kernel.NewProc("HeapFree")
	if heapProc.Find() != nil || freeProc.Find() != nil {
		return errWindowsSandbox
	}
	heap, _, _ := heapProc.Call()
	if entries == nil {
		if count != 0 {
			return errWindowsSandbox
		}
		return nil
	}
	if count > 16384 {
		freeProc.Call(heap, 0, uintptr(unsafe.Pointer(entries)))
		return errWindowsSandbox
	}
	defer freeProc.Call(heap, 0, uintptr(unsafe.Pointer(entries)))
	denied := false
	for _, entry := range unsafe.Slice(entries, int(count)) {
		if entry.Sid == nil {
			denied = true
			continue
		}
		if entry.Sid.Equals(sid) || entry.Sid.String() == "S-1-15-2-1" || entry.Sid.String() == "S-1-15-2-2" {
			denied = true
		}
		freeProc.Call(heap, 0, uintptr(unsafe.Pointer(entry.Sid)))
	}
	if denied {
		return errWindowsSandbox
	}
	return nil
}

// The user-facing error deliberately does not include executable paths, scripts,
// token details, URLs or raw Windows loader messages.
func sandboxFailure(stage string, causes ...error) error {
	message := "Windows source sandbox: " + stage
	// Preserve only the machine-readable code, never cause.Error(): Windows
	// errors can be wrapped in path/command strings and must stay out of logs.
	if len(causes) > 0 {
		var code syscall.Errno
		if errors.As(causes[0], &code) && code != 0 {
			message += fmt.Sprintf(" (win32=%d)", uint32(code))
		}
	}
	return errors.New(message)
}
