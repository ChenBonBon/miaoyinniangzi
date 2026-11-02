package sourceruntime

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"syscall"
	"testing"
)

func TestWindowsProbeProtocolDiagnostics(t *testing.T) {
	var result windowsProbeResult
	stage, err := readWindowsProbeResult(json.NewDecoder(strings.NewReader("{\"Stage\":\"probe_entry\"}\n{\"Stage\":\"sandbox_ready\"}\n{\"Ready\":true}\n")), &result)
	if err != nil || stage != "sandbox_ready" || !result.Ready {
		t.Fatal("stage protocol failed")
	}
	stage, err = readWindowsProbeResult(json.NewDecoder(strings.NewReader("{\"Stage\":\"probe_entry\"}\n")), &result)
	if !errors.Is(err, io.EOF) || stage != "probe_entry" {
		t.Fatal("early exit lost its last startup stage")
	}
	if _, err = readWindowsProbeResult(json.NewDecoder(strings.NewReader("{\"Stage\":\"UNTRUSTED_VALUE\"}\n")), &result); err == nil {
		t.Fatal("unrecognized stage accepted")
	}
	if windowsProbeErrorKind(errors.New("SECRET_RAW_ERROR")) != "other" || windowsProbeErrorKind(syscall.Errno(109)) != "win32=109" {
		t.Fatal("unsafe probe diagnostic")
	}
	if (&windowsWorkerExitError{code: 0xc0000142}).Error() != "Windows source sandbox: worker exit code=0xc0000142" {
		t.Fatal("worker exit code lost")
	}
}
