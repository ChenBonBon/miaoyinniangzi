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
	var body windowsProbeResult
	if stage, err := readWindowsProbeResult(json.NewDecoder(strings.NewReader("{\"Stage\":\"file_checks\"}\n{\"Stage\":\"child_check\"}\n{\"Stage\":\"memory_check\"}\n{\"Failure\":\"Windows source sandbox: native probe panic stage=memory_check\"}\n")), &body); err != nil || stage != "memory_check" || body.Failure == "" {
		t.Fatal("test-body failure stage lost")
	}

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
