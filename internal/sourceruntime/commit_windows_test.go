package sourceruntime

import (
	"encoding/json"
	"os"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

const commitProbeChunk = 16 << 20

type nativeCommitResult struct {
	NativeBytes, PrivateCommit, PeakProcess, PeakJob, ProcessLimit, JobLimit uint64
	AllocationCode                                                           uint32
	Denied                                                                   bool
}

func validCommitResult(result *nativeCommitResult) bool {
	if result == nil {
		return false
	}
	knownFailure := result.AllocationCode == uint32(windows.ERROR_NOT_ENOUGH_MEMORY) || result.AllocationCode == uint32(windows.ERROR_COMMITMENT_LIMIT) || result.AllocationCode == uint32(windows.ERROR_NOT_ENOUGH_QUOTA)
	near := func(value uint64) bool { return value >= jobMemoryLimit-2*commitProbeChunk && value <= jobMemoryLimit }
	return result.Denied && knownFailure && result.ProcessLimit == jobMemoryLimit && result.JobLimit == jobMemoryLimit && result.NativeBytes > 0 && result.NativeBytes <= jobMemoryLimit && result.PrivateCommit >= result.NativeBytes && result.PrivateCommit <= jobMemoryLimit && near(result.PeakProcess) && near(result.PeakJob)
}

func TestWindowsCommitProbeValidation(t *testing.T) {
	valid := nativeCommitResult{NativeBytes: 224 << 20, PrivateCommit: 240 << 20, PeakProcess: 240 << 20, PeakJob: 240 << 20, ProcessLimit: jobMemoryLimit, JobLimit: jobMemoryLimit, AllocationCode: uint32(windows.ERROR_COMMITMENT_LIMIT), Denied: true}
	if !validCommitResult(&valid) {
		t.Fatal("valid near-limit rejection not recognized")
	}
	if validCommitResult(nil) {
		t.Fatal("missing native result accepted")
	}
	for _, change := range []func(*nativeCommitResult){func(r *nativeCommitResult) { r.Denied = false }, func(r *nativeCommitResult) { r.AllocationCode = 5 }, func(r *nativeCommitResult) { r.NativeBytes = 0 }, func(r *nativeCommitResult) { r.NativeBytes = jobMemoryLimit + 1 }, func(r *nativeCommitResult) { r.PeakProcess = 1 }, func(r *nativeCommitResult) { r.PeakJob = jobMemoryLimit + 1 }, func(r *nativeCommitResult) { r.ProcessLimit = jobMemoryLimit * 2 }, func(r *nativeCommitResult) { r.PrivateCommit = 1 }} {
		broken := valid
		change(&broken)
		if validCommitResult(&broken) {
			t.Fatal("unproven native limit accepted")
		}
	}
}
// All allocation is outside the Go heap: GOMEMLIMIT/GC must not pace this test.
// Job memory limits deny new commitment; they need not terminate the process.
func nativeCommitProbeMain() int {
	result := windowsProbeResult{}
	fill := windows.NewLazySystemDLL("ntdll.dll").NewProc("RtlFillMemory")
	memory := kernel.NewProc("K32GetProcessMemoryInfo")
	if fill.Find() != nil || memory.Find() != nil {
		result.Failure = sandboxFailure("native commit APIs").Error()
		_ = json.NewEncoder(os.Stdout).Encode(result)
		return 0
	}
	counters := struct {
		Size, Faults                                                                                          uint32
		PeakWorkingSet, WorkingSet, PeakPaged, Paged, PeakNonPaged, NonPaged, PageFile, PeakPageFile, Private uintptr
	}{}
	counters.Size = uint32(unsafe.Sizeof(counters))
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	var pinned runtime.Pinner
	pinned.Pin(&counters)
	pinned.Pin(&limits)
	defer pinned.Unpin()
	var allocations [jobMemoryLimit/commitProbeChunk + 1]uintptr
	count := 0
	var allocationErr error
	var nativeBytes uint64
	for count < len(allocations) {
		address, err := windows.VirtualAlloc(0, commitProbeChunk, windows.MEM_RESERVE|windows.MEM_COMMIT, windows.PAGE_READWRITE)
		if address == 0 {
			allocationErr = err
			break
		}
		allocations[count] = address
		count++
		nativeBytes += commitProbeChunk
		// Touch every committed byte using the native API, not a Go heap slice.
		fill.Call(address, commitProbeChunk, 0xa5)
	}
	success, _, _ := memory.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&counters)), uintptr(counters.Size))
	queryErr := windows.QueryInformationJobObject(0, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil)
	freeFailed := false
	for _, address := range allocations[:count] {
		if windows.VirtualFree(address, 0, windows.MEM_RELEASE) != nil {
			freeFailed = true
		}
	}
	// Report only after releasing the native commitment, leaving Go headroom to
	// encode the bounded numeric result. Peak Job accounting survives release.
	if success == 0 || queryErr != nil || freeFailed {
		result.Failure = sandboxFailure("native commit accounting").Error()
	} else {
		result.Commit = &nativeCommitResult{NativeBytes: nativeBytes, PrivateCommit: uint64(counters.Private), PeakProcess: uint64(limits.PeakProcessMemoryUsed), PeakJob: uint64(limits.PeakJobMemoryUsed), ProcessLimit: uint64(limits.ProcessMemoryLimit), JobLimit: uint64(limits.JobMemoryLimit), AllocationCode: bootstrapErrorCode(allocationErr), Denied: count < len(allocations) && allocationErr != nil}
	}
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		return 5
	}
	return 0
}
