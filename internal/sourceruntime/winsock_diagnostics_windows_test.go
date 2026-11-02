package sourceruntime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type winsockBootstrapDiagnostics struct {
	Parameters, ProtocolCatalog, NamespaceCatalog uint32
	MSWSock, NSI                                  uint32
}

func bootstrapErrorCode(err error) uint32 {
	if err == nil {
		return 0
	}
	var code syscall.Errno
	if errors.As(err, &code) {
		return uint32(code)
	}
	return ^uint32(0)
}

// Inspect only fixed OS resources; no values, provider names, paths or user
// registry data are read or emitted. This test-only diagnostic grants nothing.
func inspectWinsockBootstrap() winsockBootstrapDiagnostics {
	codes := [3]uint32{}
	for index, path := range []string{`SYSTEM\CurrentControlSet\Services\WinSock2\Parameters`, `SYSTEM\CurrentControlSet\Services\WinSock2\Parameters\Protocol_Catalog9`, `SYSTEM\CurrentControlSet\Services\WinSock2\Parameters\NameSpace_Catalog5`} {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
		codes[index] = bootstrapErrorCode(err)
		if err == nil {
			key.Close()
		}
	}
	modules := [2]uint32{}
	for index, name := range []string{"mswsock.dll", "nsi.dll"} {
		handle, err := windows.LoadLibraryEx(name, 0, windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
		modules[index] = bootstrapErrorCode(err)
		if err == nil {
			windows.FreeLibrary(handle)
		}
	}
	return winsockBootstrapDiagnostics{Parameters: codes[0], ProtocolCatalog: codes[1], NamespaceCatalog: codes[2], MSWSock: modules[0], NSI: modules[1]}
}
func windowsWinsockControlMain() int {
	result := windowsProbeResult{}
	if err := initializeWorkerWinsock(); err != nil {
		result.Failure = err.Error()
	}
	diagnostics := inspectWinsockBootstrap()
	result.Bootstrap = &diagnostics
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		return 3
	}
	return 0
}

// A trusted, input-free ordinary child receives precisely the lowbox's curated
// environment and cwd. It never accepts script bytes or performs source RPC.
func TestWindowsWinsockCuratedEnvironmentControl(t *testing.T) {
	root, err := windows.GetWindowsDirectory()
	if err != nil {
		t.Fatal("system directory unavailable")
	}
	profile, e1 := windows.KnownFolderPath(windows.FOLDERID_Profile, 0)
	local, e2 := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	roaming, e3 := windows.KnownFolderPath(windows.FOLDERID_RoamingAppData, 0)
	if e1 != nil || e2 != nil || e3 != nil {
		t.Fatal("known folders unavailable")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("test executable unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "--miaoyin-winsock-control-test")
	command.Dir = root + `\System32`
	values := strings.Split(string(utf16.Decode(workerEnvironment(root, profile, local, roaming, "1"))), "\x00")
	command.Env = values[:len(values)-2]
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal("control pipe unavailable")
	}
	if command.Start() != nil {
		t.Fatal("control child failed to start")
	}
	var result windowsProbeResult
	readErr := json.NewDecoder(io.LimitReader(output, 4096)).Decode(&result)
	waitErr := command.Wait()
	if readErr != nil || waitErr != nil {
		t.Fatalf("control protocol failed: read=%s wait=%s", windowsProbeErrorKind(readErr), windowsProbeErrorKind(waitErr))
	}
	if result.Bootstrap == nil {
		t.Fatal("missing bootstrap diagnostics")
	}
	t.Logf("curated ordinary child OS bootstrap: %+v", *result.Bootstrap)
	if result.Failure != "" {
		if strings.HasPrefix(result.Failure, "Windows source sandbox: worker Winsock") {
			t.Fatal(result.Failure)
		}
		t.Fatal("unexpected control failure")
	}
}
