package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"miaoyinniangzi/internal/sourcenetwork"
)

func TestMinimalWorkerOfflineAndBrokerSmoke(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("implemented sandbox required")
	}
	helper := filepath.Join(t.TempDir(), "miaoyin-source-worker.exe")
	command := exec.Command("go", "build", "-o", helper, "../source_worker")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("minimal helper build failed: %s", output)
	}
	if err := runSandboxSmoke(helper); err != nil {
		t.Fatal(err)
	}
}

func TestSandboxSmokeProxyNeverUsesRealNetwork(t *testing.T) {
	ctx := context.Background()
	proxy := &smokeProxy{}
	if _, err := proxy.Do(ctx, sourcenetwork.Request{URL: "https://unexpected.invalid/", Method: "GET"}); err == nil {
		t.Fatal("unexpected request accepted")
	}
	response, err := proxy.Do(ctx, sourcenetwork.Request{URL: "https://sandbox-probe.invalid/resolve", Method: "GET"})
	if err != nil || response.Status != 200 || proxy.calls != 1 {
		t.Fatal("fixture response failed")
	}
	if _, err = proxy.Do(ctx, sourcenetwork.Request{URL: "https://sandbox-probe.invalid/resolve", Method: "GET"}); err == nil {
		t.Fatal("repeated request accepted")
	}
	if proxy.ValidateMedia(ctx, "https://media.sandbox-probe.invalid/audio.flac") != nil || proxy.ValidateMedia(ctx, "https://unexpected.invalid/") == nil {
		t.Fatal("media fixture not restricted")
	}
}
