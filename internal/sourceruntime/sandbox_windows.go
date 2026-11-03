package sourceruntime

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const EngineVersion = "lx-inspect-win-lpac-afd-v2"
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
	if err := windows.GetTokenInformation(token, class, (*byte)(unsafe.Pointer(&value)), 4, &size); err != nil {
		return 0, sandboxFailure(fmt.Sprintf("token query class=%d", class), err)
	}
	if size != 4 {
		return 0, sandboxFailure(fmt.Sprintf("token query class=%d bytes=%d", class, size))
	}
	return value, nil
}

type containerTokenState struct {
	elevated, integrityRID, appContainer, lpac, capabilities uint32
	integrityAllowed, sidMatches                             bool
}

func (state containerTokenState) validate() error {
	if state.elevated == 0 && state.integrityAllowed && (state.integrityRID == 0 || state.integrityRID == 4096) && state.appContainer == 1 && state.lpac == 1 && state.capabilities == 0 && state.sidMatches {
		return nil
	}
	// Numeric security properties only; never include user/package SID strings.
	return sandboxFailure(fmt.Sprintf("token policy elevated=%d integrity_rid=%d integrity_allowed=%t appcontainer=%d lpac=%d capabilities=%d sid_match=%t", state.elevated, state.integrityRID, state.integrityAllowed, state.appContainer, state.lpac, state.capabilities, state.sidMatches))
}
func validateContainerToken(process windows.Handle) error {
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return sandboxFailure("token open", err)
	}
	defer token.Close()
	var state containerTokenState
	var err error
	if state.elevated, err = tokenValue(token, windows.TokenElevation); err != nil {
		return err
	}
	integrity, err := queryTokenInfo(token, windows.TokenIntegrityLevel)
	if err != nil {
		return err
	}
	label, err := integrity.sid()
	if err != nil || label == nil || label.SubAuthorityCount() == 0 {
		return sandboxFailure("token integrity SID")
	}
	state.integrityRID = label.SubAuthority(uint32(label.SubAuthorityCount() - 1))
	state.integrityAllowed = label.String() == "S-1-16-4096" || label.String() == "S-1-16-0"
	runtime.KeepAlive(integrity)
	// TOKEN_INFORMATION_CLASS values from the Microsoft Windows SDK.
	if state.appContainer, err = tokenValue(token, 29); err != nil {
		return err
	}
	if state.appContainer != 1 {
		return state.validate()
	}
	if state.lpac, err = tokenLPACFlag(token); err != nil {
		return err
	}
	caps, err := queryTokenInfo(token, 30)
	if err != nil {
		return err
	}
	if caps.size < 4 {
		return sandboxFailure("token capabilities size")
	}
	state.capabilities = *(*uint32)(unsafe.Pointer(&caps.words[0]))
	data, err := queryTokenInfo(token, 31)
	if err != nil {
		return err
	}
	sid, err := data.sid()
	if err != nil {
		return err
	}
	expected, err := deriveContainerSID()
	if err != nil {
		return sandboxFailure("token expected SID", err)
	}
	state.sidMatches = sid != nil && sid.Equals(expected)
	runtime.KeepAlive(data)
	return state.validate()
}
func sandboxWorker() error {
	if err := validateContainerToken(windows.CurrentProcess()); err != nil {
		return err
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
	// Probe AFD directly, without granting registryRead just to bootstrap Winsock.
	// Require explicit kernel access denial, not initialization failure, an invalid
	// ABI, an offline route or a timeout. No source data or HTTP is transmitted.
	port, portErr := strconv.Atoi(os.Getenv(loopbackProbeEnvironment))
	if portErr != nil || port < 1 || port > 65535 {
		return sandboxFailure("worker network probe port")
	}
	for _, target := range []struct{ name, address string }{{"loopback", net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}, {"public", "1.1.1.1:443"}} {
		connected, networkErr := nativeTCPProbe(target.address)
		if connected || !networkPolicyDenied(networkErr) {
			return sandboxFailure(fmt.Sprintf("worker network policy target=%s connected=%t phase=%s", target.name, connected, afdFailurePhase(networkErr)), networkErr)
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
		var status windows.NTStatus
		if errors.As(causes[0], &status) {
			message += fmt.Sprintf(" (ntstatus=0x%08x)", uint32(status))
			return errors.New(message)
		}
		var code syscall.Errno
		if errors.As(causes[0], &code) && code != 0 {
			message += fmt.Sprintf(" (win32=%d)", uint32(code))
		}
	}
	return errors.New(message)
}
