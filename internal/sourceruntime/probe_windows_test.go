package sourceruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	Hang                  bool
	Memory                bool
	CPU                   bool
}
type windowsProbeResult struct {
	Bootstrap                                                                                                                                                                                                   *winsockBootstrapDiagnostics `json:",omitempty"`
	Stage                                                                                                                                                                                                       string
	Ready                                                                                                                                                                                                       bool
	Failure                                                                                                                                                                                                     string
	FileReadDenied, FileWriteDenied, LoopbackDenied, InternetDenied, ChildDenied, ParentDenied, EnvironmentClean, JobMemoryEnforced, StagingWriteDenied, ACLChangeDenied, RegistryIsolated, ImpersonationDenied bool
}

func windowsProbeStage(stage string) error {
	return json.NewEncoder(os.Stdout).Encode(struct{ Stage string }{stage})
}
func windowsProbeMain() (exit int) {
	lastStage := "probe_entry"
	defer func() {
		if recover() != nil {
			_ = json.NewEncoder(os.Stdout).Encode(windowsProbeResult{Failure: "Windows source sandbox: native probe panic stage=" + lastStage})
			exit = 102
		}
	}()
	stage := func(name string) {
		lastStage = name
		if windowsProbeStage(name) != nil {
			panic(struct{}{})
		}
	}
	if windowsProbeStage("probe_entry") != nil {
		return 101
	}
	if err := sandboxWorker(); err != nil {
		result := windowsProbeResult{Failure: err.Error()}
		if strings.HasPrefix(result.Failure, "Windows source sandbox: worker network policy") {
			diagnostics := inspectWinsockBootstrap()
			result.Bootstrap = &diagnostics
		}
		_ = json.NewEncoder(os.Stdout).Encode(result)
		return 0
	}
	if windowsProbeStage("sandbox_ready") != nil {
		return 101
	}
	var request windowsProbeRequest
	if json.NewDecoder(io.LimitReader(os.Stdin, 8192)).Decode(&request) != nil {
		return 3
	}
	if windowsProbeStage("request_read") != nil {
		return 101
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
	stage("file_checks")
	_, readErr := os.ReadFile(request.Read)
	writeErr := os.WriteFile(request.Write, []byte("must-not-write"), 0600)
	denied := func(address string) bool {
		connected, err := nativeTCPProbe(address)
		return !connected && networkPolicyDenied(err) // Kernel access denial, not initialization failure or offline status.
	}
	stage("network_checks")
	loopbackDenied, internetDenied := denied(request.Loopback), denied("1.1.1.1:443")
	stage("child_check")
	childDenied := probeNativeChildDenied()
	stage("parent_handle_check")
	handle, parentErr := windows.OpenProcess(windows.PROCESS_VM_READ|windows.PROCESS_DUP_HANDLE, false, request.Parent)
	if parentErr == nil {
		windows.CloseHandle(handle)
	}
	stage("staging_checks")
	executable, _ := os.Executable()
	stagingWriteErr := os.WriteFile(filepath.Join(filepath.Dir(executable), "must-not-write"), []byte("fixture"), 0600)
	directoryName, _ := windows.UTF16PtrFromString(filepath.Dir(executable))
	aclHandle, aclErr := windows.CreateFile(directoryName, windows.WRITE_DAC, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if aclErr == nil {
		windows.CloseHandle(aclHandle)
	}
	stage("registry_check")
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
		case "GOGC", "GOMEMLIMIT", "GOMAXPROCS", "GOTRACEBACK", "SYSTEMROOT", "TZ", "WINDIR", "SYSTEMDRIVE", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "TEMP", "TMP", loopbackProbeEnvironment:
		default:
			clean = false
		}
	}
	stage("memory_check")
	allocation, allocationErr := windows.VirtualAlloc(0, jobMemoryLimit+64*1024*1024, windows.MEM_RESERVE|windows.MEM_COMMIT, windows.PAGE_READWRITE)
	memoryDenied := allocation == 0 && allocationErr != nil
	if allocation != 0 {
		windows.VirtualFree(allocation, 0, windows.MEM_RELEASE)
	}
	stage("impersonation_check")
	impersonationDenied := probePipeImpersonationDenied()
	stage("result_write")
	result := windowsProbeResult{FileReadDenied: errors.Is(readErr, os.ErrPermission), FileWriteDenied: errors.Is(writeErr, os.ErrPermission), LoopbackDenied: loopbackDenied, InternetDenied: internetDenied, ChildDenied: childDenied, ParentDenied: errors.Is(parentErr, windows.ERROR_ACCESS_DENIED), EnvironmentClean: clean, JobMemoryEnforced: memoryDenied, StagingWriteDenied: errors.Is(stagingWriteErr, os.ErrPermission), ACLChangeDenied: errors.Is(aclErr, windows.ERROR_ACCESS_DENIED), RegistryIsolated: registryIsolated, ImpersonationDenied: impersonationDenied}
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		return 5
	}
	return 0
}
func readWindowsProbeResult(decoder *json.Decoder, target *windowsProbeResult) (string, error) {
	stage := "before_entry"
	for count := 0; count < 16; count++ {
		var frame windowsProbeResult
		if err := decoder.Decode(&frame); err != nil {
			return stage, err
		}
		if frame.Stage == "" {
			*target = frame
			return stage, nil
		}
		switch frame.Stage {
		case "probe_entry", "sandbox_ready", "request_read", "file_checks", "network_checks", "child_check", "parent_handle_check", "staging_checks", "registry_check", "memory_check", "impersonation_check", "result_write":
			stage = frame.Stage
		default:
			return stage, errors.New("invalid probe stage")
		}
	}
	return stage, errors.New("probe frame limit")
}
func windowsProbeErrorKind(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, io.EOF):
		return "eof"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "truncated"
	}
	var code syscall.Errno
	if errors.As(err, &code) {
		return fmt.Sprintf("win32=%d", uint32(code))
	}
	var exit *windowsWorkerExitError
	if errors.As(err, &exit) {
		return "worker_exit"
	}
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		return "invalid_json"
	}
	return "other"
}
func probeNativeChildDenied() bool {
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	// The pinned staged image is already known readable/executable by this token;
	// a system command could fail for unrelated image/loader permissions.
	image, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return false
	}
	command, err := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{executable, "--miaoyin-child-native-control"}))
	if err != nil {
		return false
	}
	startup := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var process windows.ProcessInformation
	err = windows.CreateProcess(image, command, nil, nil, false, windows.CREATE_NO_WINDOW, nil, nil, &startup, &process)
	if err == nil {
		windows.TerminateProcess(process.Process, 1)
		windows.WaitForSingleObject(process.Process, windows.INFINITE)
		windows.CloseHandle(process.Thread)
		windows.CloseHandle(process.Process)
		return false
	}
	return errors.Is(err, windows.ERROR_CHILD_PROCESS_BLOCKED) || errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_NOT_ENOUGH_QUOTA)
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
	requireNativeAFDControl(t)
	requireHandleSnapshotControl(t)
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
	query, queryErr := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION, false, uint32(worker.pid))
	if queryErr != nil {
		worker.kill()
		worker.wait()
		t.Fatal("cannot open lowbox for read-only handle audit")
	}
	extraPresent, snapshotErr := processInheritedHandlePresent(query, event)
	windows.CloseHandle(query)
	if snapshotErr != nil || extraPresent {
		worker.kill()
		worker.wait()
		if snapshotErr != nil {
			t.Fatal(snapshotErr)
		}
		t.Fatal("extra inheritable event leaked into lowbox handle snapshot")
	}
	t.Log("lowbox handle snapshot: unrelated inheritable event absent")
	if _, err := residentBytes(worker.pid); err != nil {
		worker.kill()
		worker.wait()
		t.Fatal("cannot monitor contained worker RSS")
	}
	writeErr := json.NewEncoder(worker.input).Encode(windowsProbeRequest{Read: secret, Write: filepath.Join(dir, "must-not-exist"), Loopback: listener.Addr().String(), Registry: registryPath, Parent: windows.GetCurrentProcessId()})
	worker.input.Close()
	var result windowsProbeResult
	var readErr, waitErr error
	lastStage := "before_entry"
	done := make(chan struct{}, 1)
	go func() {
		lastStage, readErr = readWindowsProbeResult(json.NewDecoder(io.LimitReader(worker.output, 4096)), &result)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			worker.kill()
		}
		waitErr = worker.wait()
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-ctx.Done():
		worker.kill()
		<-done
		t.Fatal("native sandbox probe timed out")
	}
	if strings.HasPrefix(result.Failure, "Windows source sandbox: ") {
		if result.Bootstrap != nil {
			t.Logf("lowbox OS bootstrap: %+v", *result.Bootstrap)
		}
		t.Fatal(result.Failure)
	}
	if readErr != nil || waitErr != nil || writeErr != nil {
		var exit *windowsWorkerExitError
		var code uint32
		if errors.As(waitErr, &exit) {
			code = exit.code
		}
		t.Fatalf("native sandbox probe failed: stage=%s write=%s read=%s wait=%s exit=0x%08x", lastStage, windowsProbeErrorKind(writeErr), windowsProbeErrorKind(readErr), windowsProbeErrorKind(waitErr), code)
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
	var ready windowsProbeResult
	if _, err := readWindowsProbeResult(json.NewDecoder(io.LimitReader(worker.output, 4096)), &ready); err != nil || !ready.Ready {
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
func TestWindowsContainerTokenPolicyRemainsStrict(t *testing.T) {
	good := containerTokenState{integrityRID: 4096, integrityAllowed: true, appContainer: 1, lpac: 1, sidMatches: true}
	if good.validate() != nil {
		t.Fatal("valid token properties rejected")
	}
	cases := []struct {
		name   string
		change func(*containerTokenState)
	}{
		{"elevated", func(s *containerTokenState) { s.elevated = 1 }},
		{"integrity", func(s *containerTokenState) { s.integrityAllowed = false; s.integrityRID = 8192 }},
		{"appcontainer", func(s *containerTokenState) { s.appContainer = 0 }},
		{"lpac", func(s *containerTokenState) { s.lpac = 0 }},
		{"capabilities", func(s *containerTokenState) { s.capabilities = 1 }},
		{"sid", func(s *containerTokenState) { s.sidMatches = false }},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			state := good
			item.change(&state)
			if state.validate() == nil {
				t.Fatal("unsafe token accepted")
			}
		})
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
	t.Setenv("MIAOYIN_SECRET", "SYNTHETIC_SECRET")
	t.Setenv(loopbackProbeEnvironment, "1")
	t.Setenv("USERPROFILE", "WRONG_PARENT_PATH")
	t.Setenv("LOCALAPPDATA", "WRONG_PARENT_PATH")
	block := workerEnvironment(`C:\Windows`, `C:\Users\Fixture`, `C:\Users\Fixture\AppData\Local`, `C:\Users\Fixture\AppData\Roaming`, "43210")
	values := strings.Split(string(utf16.Decode(block)), "\x00")
	if len(values) != 16 || values[len(values)-1] != "" || values[len(values)-2] != "" {
		t.Fatal("environment block is not double-NUL terminated")
	}
	found := map[string]string{}
	previous := ""
	for _, value := range values[:14] {
		pair := strings.SplitN(value, "=", 2)
		if len(pair) != 2 {
			t.Fatal("invalid environment entry")
		}
		key := strings.ToUpper(pair[0])
		if previous > key {
			t.Fatal("environment block not case-insensitively sorted")
		}
		previous = key
		found[key] = pair[1]
		if strings.Contains(value, "SYNTHETIC_SECRET") || strings.Contains(value, "WRONG_PARENT_PATH") {
			t.Fatal("parent environment inherited")
		}
	}
	for _, key := range []string{"USERNAME", "HOME", "PATH", "MIAOYIN_SECRET", "GODEBUG"} {
		if _, ok := found[key]; ok {
			t.Fatal("unapproved environment variable")
		}
	}
	if found["USERPROFILE"] != `C:\Users\Fixture` || found["SYSTEMDRIVE"] != "C:" || found["TEMP"] != `C:\Users\Fixture\AppData\Local\Temp` || found["TMP"] != found["TEMP"] || found[loopbackProbeEnvironment] != "43210" {
		t.Fatal("required Windows paths incorrect")
	}

}
