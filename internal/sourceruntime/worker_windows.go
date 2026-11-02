package sourceruntime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	attributeSecurityCapabilities = 0x20009
	attributeJobList              = 0x2000d
	attributeChildPolicy          = 0x2000e
	attributeAllPackagesPolicy    = 0x2000f
)

func workerExecutable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "miaoyin-source-worker.exe"), nil
}

type securityCapabilities struct {
	SID             *windows.SID
	Capabilities    *windows.SIDAndAttributes
	Count, Reserved uint32
}

func startWorker(ctx context.Context, source, flag string, extra ...string) (*workerProcess, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	stage, err := stageWorker(ctx, source)
	if err != nil {
		return nil, err
	}
	executable, sid, stageCleanup := stage.path, stage.sid, stage.cleanup
	success := false
	var job windows.Handle
	var process windows.ProcessInformation
	var input, output *os.File
	defer func() {
		if !success {
			if job != 0 {
				windows.TerminateJobObject(job, 1)
			}
			if process.Process != 0 {
				windows.TerminateProcess(process.Process, 1)
				windows.WaitForSingleObject(process.Process, windows.INFINITE)
				windows.CloseHandle(process.Process)
			}
			if process.Thread != 0 {
				windows.CloseHandle(process.Thread)
			}
			if job != 0 {
				windows.CloseHandle(job)
			}
			if input != nil {
				input.Close()
			}
			if output != nil {
				output.Close()
			}
			stageCleanup()
		}
	}()
	job, err = windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, sandboxFailure("job creation", err)
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{ProcessMemoryLimit: jobMemoryLimit, JobMemoryLimit: jobMemoryLimit}
	limits.BasicLimitInformation.LimitFlags = jobFlags
	limits.BasicLimitInformation.ActiveProcessLimit = 1
	limits.BasicLimitInformation.PerProcessUserTimeLimit = jobCPUTime
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, sandboxFailure("job limits", err)
	}
	ui := windows.JOBOBJECT_BASIC_UI_RESTRICTIONS{UIRestrictionsClass: 0xff}
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectBasicUIRestrictions, uintptr(unsafe.Pointer(&ui)), uint32(unsafe.Sizeof(ui))); err != nil {
		return nil, sandboxFailure("job UI limits", err)
	}
	childIn, parentIn, err := os.Pipe()
	if err != nil {
		return nil, sandboxFailure("stdin pipe", err)
	}
	input = parentIn
	defer childIn.Close()
	parentOut, childOut, err := os.Pipe()
	if err != nil {
		return nil, sandboxFailure("stdout pipe", err)
	}
	output = parentOut
	defer childOut.Close()
	nullName, _ := windows.UTF16PtrFromString("NUL")
	null, err := windows.CreateFile(nullName, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, sandboxFailure("stderr", err)
	}
	defer windows.CloseHandle(null)
	original := []windows.Handle{windows.Handle(childIn.Fd()), windows.Handle(childOut.Fd()), null}
	handles := make([]windows.Handle, 0, 3)
	defer func() {
		for _, handle := range handles {
			windows.CloseHandle(handle)
		}
	}()
	for _, handle := range original {
		var duplicate windows.Handle
		if err = windows.DuplicateHandle(windows.CurrentProcess(), handle, windows.CurrentProcess(), &duplicate, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
			return nil, sandboxFailure("handle duplication", err)
		}
		handles = append(handles, duplicate)
	}
	attributes, err := windows.NewProcThreadAttributeList(5)
	if err != nil {
		return nil, sandboxFailure("process attributes", err)
	}
	defer attributes.Delete()
	capabilities := securityCapabilities{SID: sid}
	jobs := []windows.Handle{job}
	childPolicy, allPackagesPolicy := uint32(1), uint32(1)
	for _, attribute := range []struct {
		key   uintptr
		value unsafe.Pointer
		size  uintptr
	}{
		{attributeSecurityCapabilities, unsafe.Pointer(&capabilities), unsafe.Sizeof(capabilities)},
		{attributeJobList, unsafe.Pointer(&jobs[0]), unsafe.Sizeof(jobs[0])},
		{attributeChildPolicy, unsafe.Pointer(&childPolicy), 4},
		{attributeAllPackagesPolicy, unsafe.Pointer(&allPackagesPolicy), 4},
		{windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles)) * unsafe.Sizeof(handles[0])},
	} {
		if err = attributes.Update(attribute.key, attribute.value, attribute.size); err != nil {
			return nil, sandboxFailure("required process attribute", err)
		}
	}
	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES
	startup.StdInput = handles[0]
	startup.StdOutput = handles[1]
	startup.StdErr = handles[2]
	app, _ := windows.UTF16PtrFromString(executable)
	command, _ := windows.UTF16FromString(windows.ComposeCommandLine(append([]string{executable, flag}, extra...)))
	systemRoot, err := windows.GetWindowsDirectory()
	if err != nil {
		return nil, sandboxFailure("system directory")
	}
	profile, profileErr := windows.KnownFolderPath(windows.FOLDERID_Profile, 0)
	local, localErr := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	roaming, roamingErr := windows.KnownFolderPath(windows.FOLDERID_RoamingAppData, 0)
	if profileErr != nil || localErr != nil || roamingErr != nil {
		return nil, sandboxFailure("known environment directories")
	}
	environment := workerEnvironment(systemRoot, profile, local, roaming)
	current, _ := windows.UTF16PtrFromString(systemRoot + "\\System32")
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_SUSPENDED | windows.CREATE_NO_WINDOW)
	err = windows.CreateProcess(app, &command[0], nil, nil, true, flags, &environment[0], current, &startup.StartupInfo, &process)
	runtime.KeepAlive(capabilities)
	runtime.KeepAlive(jobs)
	runtime.KeepAlive(handles)
	runtime.KeepAlive(attributes)
	if err != nil {
		return nil, sandboxFailure("lowbox creation", err)
	}
	if err = stage.verifyImage(process.Process); err != nil {
		return nil, err
	}
	if err = validateContainerToken(process.Process); err != nil {
		return nil, err
	}
	inJob := uint32(0)
	isInJob := kernel.NewProc("IsProcessInJob")
	if isInJob.Find() != nil {
		return nil, sandboxFailure("job verification")
	}
	result, _, _ := isInJob.Call(uintptr(process.Process), uintptr(job), uintptr(unsafe.Pointer(&inJob)))
	if result == 0 || inJob != 1 {
		return nil, sandboxFailure("job verification")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if _, err = windows.ResumeThread(process.Thread); err != nil {
		return nil, sandboxFailure("resume", err)
	}
	windows.CloseHandle(process.Thread)
	process.Thread = 0
	kill := func() { _ = windows.TerminateJobObject(job, 1) }
	wait := func() error {
		state, err := windows.WaitForSingleObject(process.Process, windows.INFINITE)
		if err != nil || state != windows.WAIT_OBJECT_0 {
			return errWindowsSandbox
		}
		var code uint32
		if err = windows.GetExitCodeProcess(process.Process, &code); err != nil {
			return sandboxFailure("worker exit query", err)
		}
		if code != 0 {
			return &windowsWorkerExitError{code: code}
		}
		return nil
	}
	success = true
	return &workerProcess{input: input, output: output, pid: int(process.ProcessId), wait: wait, kill: kill, close: func() {
		kill()
		input.Close()
		output.Close()
		windows.CloseHandle(process.Process)
		windows.CloseHandle(job)
		stageCleanup()
	}}, nil
}

type windowsWorkerExitError struct{ code uint32 }

func (err *windowsWorkerExitError) Error() string {
	return fmt.Sprintf("Windows source sandbox: worker exit code=0x%08x", err.code)
}

func workerEnvironment(root, profile, local, roaming string) []uint16 {
	// Windows lowbox/profile initialization expands these paths. Supply values
	// from OS known folders, not the parent's arbitrary environment. Knowing a
	// path does not grant the lowbox access to it; all ACL/token checks remain.
	temp := filepath.Join(local, "Temp")
	values := []string{"GOGC=50", "GOMAXPROCS=2", "GOMEMLIMIT=64MiB", "GOTRACEBACK=none", "SystemRoot=" + root, "TZ=UTC", "WINDIR=" + root,
		"SystemDrive=" + filepath.VolumeName(root), "USERPROFILE=" + profile, "LOCALAPPDATA=" + local, "APPDATA=" + roaming, "TEMP=" + temp, "TMP=" + temp}
	sort.Slice(values, func(i, j int) bool { return strings.ToUpper(values[i]) < strings.ToUpper(values[j]) })
	var result []uint16
	for _, value := range values {
		encoded, _ := windows.UTF16FromString(value)
		result = append(result, encoded...)
	}
	return append(result, 0)
}
