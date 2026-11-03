package sourceruntime

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Native ProcessHandleInformation (51), not a query against the candidate
// handle itself. Invalid references can raise a fatal strict-handle exception.
type processHandleEntry struct {
	Value                                   windows.Handle
	HandleCount, PointerCount               uintptr
	Access, TypeIndex, Attributes, Reserved uint32
}

func inheritedHandlePresent(words []uintptr, size uint32, target windows.Handle) (bool, error) {
	header := 2 * unsafe.Sizeof(uintptr(0))
	entrySize := unsafe.Sizeof(processHandleEntry{})
	capacity := uintptr(len(words)) * unsafe.Sizeof(uintptr(0))
	if uintptr(size) > capacity || uintptr(size) < header {
		return false, sandboxFailure("handle snapshot size")
	}
	count := words[0]
	if count > (uintptr(size)-header)/entrySize {
		return false, sandboxFailure("handle snapshot count")
	}
	base := unsafe.Pointer(&words[0])
	for index := uintptr(0); index < count; index++ {
		entry := (*processHandleEntry)(unsafe.Add(base, header+index*entrySize))
		if entry.Value == target && entry.Attributes&windows.OBJ_INHERIT != 0 {
			return true, nil
		}
	}
	return false, nil
}
func currentInheritedHandlePresent(target windows.Handle) (bool, error) {
	return processInheritedHandlePresent(windows.CurrentProcess(), target)
}
func processInheritedHandlePresent(process windows.Handle, target windows.Handle) (bool, error) {
	proc := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryInformationProcess")
	if proc.Find() != nil {
		return false, sandboxFailure("handle snapshot API")
	}
	// Bounded, pointer-aligned snapshot; no object names or kernel pointers.
	for length := 8192; length <= 131072; length *= 2 {
		words := make([]uintptr, length)
		var returned uint32
		status, _, _ := proc.Call(uintptr(process), 51, uintptr(unsafe.Pointer(&words[0])), uintptr(len(words))*unsafe.Sizeof(uintptr(0)), uintptr(unsafe.Pointer(&returned)))
		runtime.KeepAlive(words)
		if uint32(status) == 0xc0000004 || uint32(status) == 0xc0000023 {
			continue
		}
		if uint32(status) != 0 {
			return false, sandboxFailure("handle snapshot query", windows.NTStatus(uint32(status)))
		}
		return inheritedHandlePresent(words, returned, target)
	}
	return false, sandboxFailure("handle snapshot limit")
}
func TestWindowsHandleSnapshotControl(t *testing.T) { requireHandleSnapshotControl(t) }
func requireHandleSnapshotControl(t *testing.T) {
	t.Helper()
	if unsafe.Sizeof(processHandleEntry{}) != 40 {
		t.Fatal("handle snapshot ABI mismatch")
	}
	security := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	event, err := windows.CreateEvent(&security, 1, 0, nil)
	if err != nil {
		t.Fatal("handle control event creation failed")
	}
	defer windows.CloseHandle(event)
	present, err := currentInheritedHandlePresent(event)
	if err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Fatal("inheritable event missing from snapshot")
	}
	if err = windows.SetHandleInformation(event, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		t.Fatal("handle control inheritance update failed")
	}
	present, err = currentInheritedHandlePresent(event)
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Fatal("non-inheritable event misclassified")
	}
	// This arbitrary invalid value is searched only in a snapshot, never passed
	// to SetEvent, CloseHandle, GetHandleInformation or another handle operation.
	present, err = currentInheritedHandlePresent(windows.Handle(^uintptr(0)))
	if err != nil || present {
		t.Fatal("invalid-value read-only search failed")
	}
	words := make([]uintptr, 2)
	words[0] = 1
	if _, err = inheritedHandlePresent(words, 16, event); err == nil {
		t.Fatal("truncated handle snapshot accepted")
	}
	t.Log("handle snapshot control: inheritable flag detected; cleared flag and invalid-value search verified")
}
