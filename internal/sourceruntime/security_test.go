package sourceruntime

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestOSGuardBlocksFilesNetworkAndProcesses(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS Seatbelt required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := exec.CommandContext(ctx, path, WorkerFlag, "--sandbox-probe").Output()
	if err != nil {
		t.Fatal(err)
	}
	var blocked []bool
	if json.Unmarshal(data, &blocked) != nil || len(blocked) != 3 {
		t.Fatal("invalid sandbox probe")
	}
	for _, value := range blocked {
		if !value {
			t.Fatalf("OS guard did not block all operations: %v", blocked)
		}
	}
}

// Optional real-script smoke test: its contents never enter test output.
func TestOptInUserScript(t *testing.T) {
	path := os.Getenv("MIAOYIN_SOURCE_TEST_PATH")
	if path == "" {
		t.Skip("no user script supplied")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("cannot read supplied source fixture")
	}
	defer file.Close()
	script, err := io.ReadAll(io.LimitReader(file, maxScriptSize+1))
	if err != nil || len(script) > maxScriptSize {
		t.Fatal("invalid source fixture")
	}
	defer clear(script)
	inspector, err := NewInspector()
	if err != nil {
		t.Fatal(err)
	}
	report, err := inspector.Inspect(context.Background(), script)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Ready {
		t.Fatalf("source check: %s", report.Error)
	}
	t.Logf("QQ qualities: %v", report.Qualities)
}

func TestMemoryHungryWorkerIsBounded(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS worker required")
	}
	inspector, err := NewInspector()
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	report, err := inspector.Inspect(context.Background(), []byte(`const values=[];for(;;){values.push(new Array(200000).fill(Math.random()))}`))
	if err == nil && report.Ready {
		t.Fatal("accepted unbounded allocation")
	}
	if time.Since(start) > 4*time.Second {
		t.Fatal("worker did not terminate promptly")
	}
}
