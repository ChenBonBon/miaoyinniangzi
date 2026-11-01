package sourceruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"miaoyinniangzi/internal/privatefs"
)

type windowsProbeRequest struct {
	Read, Write, Loopback string
	Registry              string
	Parent                uint32
	ExtraHandle           uintptr
	Hang                  bool
	Memory                bool
	CPU                   bool
}
type windowsProbeResult struct {
	Failure                                                                                                                                                                                                     string
	FileReadDenied, FileWriteDenied, LoopbackDenied, InternetDenied, ChildDenied, ParentDenied, EnvironmentClean, JobMemoryEnforced, StagingWriteDenied, ACLChangeDenied, RegistryIsolated, ImpersonationDenied bool
}

func windowsProbeMain() int {
	if err := sandboxWorker(); err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(windowsProbeResult{Failure: err.Error()})
		return 0
	}
	var request windowsProbeRequest
	if json.NewDecoder(io.LimitReader(os.Stdin, 8192)).Decode(&request) != nil {
		return 3
	}
	if request.Hang {
		if json.NewEncoder(os.Stdout).Encode(struct{ Ready bool }{true}) != nil {
			return 3
		}
		for {
			time.Sleep(time.Second)
		}
	}
	if request.CPU {
		for {
		}
	}
	if request.Memory {
		// Touch committed pages, rather than just reserve address space.
		var chunks [][]byte
		for {
			chunk := make([]byte, 16<<20)
			for i := 0; i < len(chunk); i += 4096 {
				chunk[i] = 1
			}
			chunks = append(chunks, chunk)
			if len(chunks) > 64 {
				return 4
			}
		}
	}
	_, readErr := os.ReadFile(request.Read)
	writeErr := os.WriteFile(request.Write, []byte("must-not-write"), 0600)
	denied := func(address string) bool {
		connection, err := net.DialTimeout("tcp4", address, 600*time.Millisecond)
		if connection != nil {
			connection.Close()
		}
		return errors.Is(err, syscall.Errno(10013)) // WSAEACCES, not a timeout/offline false positive.
	}
	system, _ := windows.GetWindowsDirectory()
	childErr := exec.Command(filepath.Join(system, "System32", "cmd.exe"), "/c", "exit", "0").Run()
	handle, parentErr := windows.OpenProcess(windows.PROCESS_VM_READ|windows.PROCESS_DUP_HANDLE, false, request.Parent)
	if parentErr == nil {
		windows.CloseHandle(handle)
	}
	// If an unrelated inheritable event leaked, this signals the parent's object.
	if request.ExtraHandle != 0 {
		_ = windows.SetEvent(windows.Handle(request.ExtraHandle))
	}
	executable, _ := os.Executable()
	stagingWriteErr := os.WriteFile(filepath.Join(filepath.Dir(executable), "must-not-write"), []byte("fixture"), 0600)
	directoryName, _ := windows.UTF16PtrFromString(filepath.Dir(executable))
	aclHandle, aclErr := windows.CreateFile(directoryName, windows.WRITE_DAC, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if aclErr == nil {
		windows.CloseHandle(aclHandle)
	}
	registryIsolated := true
	key, keyErr := registry.OpenKey(registry.CURRENT_USER, request.Registry, registry.QUERY_VALUE|registry.SET_VALUE)
	if keyErr == nil {
		value, _, readErr := key.GetStringValue("probe")
		if readErr == nil && value == "synthetic-secret" {
			registryIsolated = false
		}
		_ = key.SetStringValue("probe", "tampered")
		key.Close()
	}
	clean := true
	for _, variable := range os.Environ() {
		name := strings.SplitN(variable, "=", 2)[0]
		switch strings.ToUpper(name) {
		case "GOGC", "GOMEMLIMIT", "GOMAXPROCS", "GOTRACEBACK", "SYSTEMROOT", "TZ", "WINDIR":
		default:
			clean = false
		}
	}
	allocation, allocationErr := windows.VirtualAlloc(0, jobMemoryLimit+64*1024*1024, windows.MEM_RESERVE|windows.MEM_COMMIT, windows.PAGE_READWRITE)
	memoryDenied := allocation == 0 && allocationErr != nil
	if allocation != 0 {
		windows.VirtualFree(allocation, 0, windows.MEM_RELEASE)
	}
	impersonationDenied := probePipeImpersonationDenied()
	result := windowsProbeResult{FileReadDenied: errors.Is(readErr, os.ErrPermission), FileWriteDenied: errors.Is(writeErr, os.ErrPermission), LoopbackDenied: denied(request.Loopback), InternetDenied: denied("1.1.1.1:443"), ChildDenied: childErr != nil, ParentDenied: errors.Is(parentErr, windows.ERROR_ACCESS_DENIED), EnvironmentClean: clean, JobMemoryEnforced: memoryDenied, StagingWriteDenied: errors.Is(stagingWriteErr, os.ErrPermission), ACLChangeDenied: errors.Is(aclErr, windows.ERROR_ACCESS_DENIED), RegistryIsolated: registryIsolated, ImpersonationDenied: impersonationDenied}
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		return 5
	}
	return 0
}
func probePipeImpersonationDenied() bool {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	advapi := windows.NewLazySystemDLL("advapi32.dll")
	impersonate, revert := advapi.NewProc("ImpersonateNamedPipeClient"), advapi.NewProc("RevertToSelf")
	if impersonate.Find() != nil || revert.Find() != nil {
		return false
	}
	result, _, _ := impersonate.Call(os.Stdin.Fd())
	if result == 0 {
		return true
	}
	defer revert.Call()
	var token windows.Token
	if windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &token) != nil {
		return false
	}
	defer token.Close()
	container, err := tokenValue(token, 29)
	if err != nil {
		return false
	}
	level, err := tokenValue(token, windows.TokenImpersonationLevel)
	if err != nil {
		return false
	}
	return container == 1 || level <= 1 // Identification cannot authorize access as the broker.
}

func TestWindowsSandboxNativeBoundaries(t *testing.T) {
	version := windows.RtlGetVersion()
	t.Logf("Windows kernel %d.%d build %d; arch=%s", version.MajorVersion, version.MinorVersion, version.BuildNumber, runtime.GOARCH)
	var parentInJob uint32
	membership := kernel.NewProc("IsProcessInJob")
	if membership.Find() != nil {
		t.Fatal("cannot query parent job membership")
	}
	ok, _, _ := membership.Call(uintptr(windows.CurrentProcess()), 0, uintptr(unsafe.Pointer(&parentInJob)))
	if ok == 0 {
		t.Fatal("cannot query parent job membership")
	}
	t.Logf("parent_in_job=%t", parentInJob != 0)
	dir := t.TempDir()
	if err := privatefs.Protect(dir, 0700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "fake-source-key")
	if err := os.WriteFile(secret, []byte("synthetic-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	security := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	event, err := windows.CreateEvent(&security, 1, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(event)
	registryPath := `Software\MiaoyinSandboxTest\` + time.Now().Format("20060102150405.000000000")
	key, _, err := registry.CreateKey(registry.CURRENT_USER, registryPath, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	if err = key.SetStringValue("probe", "synthetic-secret"); err != nil {
		key.Close()
		t.Fatal(err)
	}
	key.Close()
	defer registry.DeleteKey(registry.CURRENT_USER, registryPath)
	executable, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	worker, err := startWorker(ctx, executable, WorkerFlag, "--native-windows-probe")
	if err != nil {
		t.Fatal(err)
	}
	defer worker.close()
	if _, err := residentBytes(worker.pid); err != nil {
		worker.kill()
		worker.wait()
		t.Fatal("cannot monitor contained worker RSS")
	}
	writeErr := json.NewEncoder(worker.input).Encode(windowsProbeRequest{Read: secret, Write: filepath.Join(dir, "must-not-exist"), Loopback: listener.Addr().String(), Registry: registryPath, Parent: windows.GetCurrentProcessId(), ExtraHandle: uintptr(event)})
	worker.input.Close()
	var result windowsProbeResult
	done := make(chan error, 1)
	go func() {
		err := json.NewDecoder(io.LimitReader(worker.output, 4096)).Decode(&result)
		if err != nil {
			worker.kill()
		}
		waitErr := worker.wait()
		if err == nil {
			err = waitErr
		}
		done <- err
	}()
	select {
	case err = <-done:
	case <-ctx.Done():
		worker.kill()
		<-done
		t.Fatal("native sandbox probe timed out")
	}
	if strings.HasPrefix(result.Failure, "Windows source sandbox: worker ") {
		t.Fatal(result.Failure)
	}
	if err != nil || writeErr != nil {
		t.Fatal("native sandbox probe failed")
	}
	if !result.FileReadDenied || !result.FileWriteDenied || !result.LoopbackDenied || !result.InternetDenied || !result.ChildDenied || !result.ParentDenied || !result.EnvironmentClean || !result.JobMemoryEnforced || !result.StagingWriteDenied || !result.ACLChangeDenied || !result.RegistryIsolated || !result.ImpersonationDenied {
		t.Fatalf("OS sandbox boundary failure: %+v", result)
	}
	if state, _ := windows.WaitForSingleObject(event, 0); state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatal("unrelated inheritable handle leaked")
	}
	if _, err = os.Stat(filepath.Join(dir, "must-not-exist")); !os.IsNotExist(err) {
		t.Fatal("worker wrote outside container")
	}
	key, err = registry.OpenKey(registry.CURRENT_USER, registryPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	value, _, readErr := key.GetStringValue("probe")
	key.Close()
	if readErr != nil || value != "synthetic-secret" {
		t.Fatal("worker changed the parent registry")
	}
	if rss, err := residentBytes(os.Getpid()); err != nil || rss == 0 {
		t.Fatal("Windows RSS monitor unavailable")
	}
}
func TestWindowsUncontainedWorkerRefusesScript(t *testing.T) {
	executable, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	payload, _ := json.Marshal(workerRequest{Script: []byte(validScript)})
	command := exec.CommandContext(ctx, executable, WorkerFlag)
	command.Stdin = bytes.NewReader(payload)
	output, err := command.Output()
	if err != nil {
		t.Fatal("uncontained worker protocol failed")
	}
	var report Report
	if json.Unmarshal(output, &report) != nil || report.Ready || report.Error != errSandbox {
		t.Fatal("ordinary process accepted a source script")
	}
}
func TestWindowsSandboxHardCommitLimit(t *testing.T) {
	executable, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	worker, err := startWorker(ctx, executable, WorkerFlag, "--native-windows-probe")
	if err != nil {
		t.Fatal(err)
	}
	defer worker.close()
	if err = json.NewEncoder(worker.input).Encode(windowsProbeRequest{Memory: true}); err != nil {
		worker.kill()
		t.Fatal("memory probe input failed")
	}
	worker.input.Close()
	done := make(chan error, 1)
	go func() { _, _ = io.Copy(io.Discard, worker.output); done <- worker.wait() }()
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("unbounded native memory allocation survived")
		}
	case <-ctx.Done():
		worker.kill()
		<-done
		t.Fatal("native allocation was not stopped by job limits")
	}
}

func TestWindowsSandboxHardCPULimit(t *testing.T) {
	executable, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	worker, err := startWorker(ctx, executable, WorkerFlag, "--native-windows-probe")
	if err != nil {
		t.Fatal(err)
	}
	defer worker.close()
	if err = json.NewEncoder(worker.input).Encode(windowsProbeRequest{CPU: true}); err != nil {
		worker.kill()
		t.Fatal("CPU probe input failed")
	}
	worker.input.Close()
	done := make(chan error, 1)
	go func() { _, _ = io.Copy(io.Discard, worker.output); done <- worker.wait() }()
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("native busy loop exited without job termination")
		}
	case <-ctx.Done():
		worker.kill()
		<-done
		t.Fatal("job CPU budget was not enforced")
	}
}

// The broker below is deliberately a separate trusted test process. Killing it
// proves the job handle is not held by the child and OS cleanup needs no defer.
type brokerChild struct {
	PID        uint32
	Executable string
}

func windowsBrokerMain() int {
	executable, _ := os.Executable()
	worker, err := startWorker(context.Background(), executable, WorkerFlag, "--native-windows-probe")
	if err != nil {
		return 2
	}
	defer worker.close()
	if json.NewEncoder(worker.input).Encode(windowsProbeRequest{Hang: true}) != nil {
		return 3
	}
	worker.input.Close()
	var ready struct{ Ready bool }
	if json.NewDecoder(io.LimitReader(worker.output, 1024)).Decode(&ready) != nil || !ready.Ready {
		return 3
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(worker.pid))
	if err != nil {
		return 4
	}
	path := make([]uint16, 32768)
	size := uint32(len(path))
	err = windows.QueryFullProcessImageName(handle, 0, &path[0], &size)
	windows.CloseHandle(handle)
	if err != nil {
		return 5
	}
	if json.NewEncoder(os.Stdout).Encode(brokerChild{uint32(worker.pid), windows.UTF16ToString(path[:size])}) != nil {
		return 6
	}
	for {
		time.Sleep(time.Second)
	}
}
func TestWindowsSandboxParentDeathKillsWorker(t *testing.T) {
	executable, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "--miaoyin-sandbox-broker-test")
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			command.Process.Kill()
			command.Wait()
		}
	}()
	var child brokerChild
	decoded := make(chan error, 1)
	go func() { decoded <- json.NewDecoder(io.LimitReader(output, 65536)).Decode(&child) }()
	select {
	case err = <-decoded:
		if err != nil {
			t.Fatal("broker did not create a contained worker")
		}
	case <-ctx.Done():
		t.Fatal("broker timed out")
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, child.PID)
	if err != nil {
		t.Fatal("cannot observe contained worker")
	}
	defer windows.CloseHandle(handle)
	if err = command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	waited = true
	state, err := windows.WaitForSingleObject(handle, 5000)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatal("contained worker survived broker termination")
	}
	profile, _, err := containerProfile()
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(child.Executable)
	relative, err := filepath.Rel(profile, directory)
	if err != nil || strings.ContainsAny(relative, "/\\") || !strings.HasPrefix(relative, "miaoyin-worker-") || filepath.Base(child.Executable) != "worker.exe" {
		t.Fatal("unexpected staged worker location")
	}
	// Broker termination releases its pinned handles; only this proven test copy
	// is removed, never a guessed directory or user file.
	if err = os.RemoveAll(directory); err != nil {
		t.Fatal("test executable staging cleanup failed")
	}
}
func TestWindowsSandboxDiagnosticsKeepOnlyNumericErrorCodes(t *testing.T) {
	wrapped := &os.PathError{Op: "SECRET_OPERATION", Path: "SECRET_PATH", Err: syscall.Errno(5)}
	if message := sandboxFailure("lowbox creation", wrapped).Error(); message != "Windows source sandbox: lowbox creation (win32=5)" {
		t.Fatal("numeric diagnostic missing or raw data leaked")
	}
	for _, cause := range []error{nil, errors.New("SECRET_RAW_ERROR")} {
		if sandboxFailure("lowbox creation", cause).Error() != "Windows source sandbox: lowbox creation" {
			t.Fatal("raw error text leaked")
		}
	}
}

func TestWindowsWorkerEnvironmentDoesNotInheritCredentials(t *testing.T) {
	block := workerEnvironment("C:\\Windows")
	values := strings.Split(string(utf16.Decode(block)), "\x00")
	if len(values) != 9 || values[len(values)-1] != "" || values[len(values)-2] != "" {
		t.Fatal("environment block is not double-NUL terminated")
	}
	for _, value := range values[:7] {
		if strings.Contains(value, "USER") || strings.Contains(value, "PATH=") || strings.Contains(value, "MIAOYIN_") {
			t.Fatal("credentials inherited")
		}
	}
}
