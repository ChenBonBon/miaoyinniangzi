//go:build !source_live

package sourceruntime

import (
	"os/exec"
	"strings"
	"testing"
)

// A Windows lowbox is active before package init, unlike macOS Seatbelt. Live
// playback/download test dependencies must not initialize in ordinary workers.
func TestWorkerTestBinaryHasNoApplicationDependencies(t *testing.T) {
	output, err := exec.Command("go", "list", "-test", "-deps", ".").Output()
	if err != nil {
		t.Fatal("cannot inspect worker test dependencies")
	}
	for _, path := range strings.Fields(string(output)) {
		if strings.HasPrefix(path, "modernc.org/") || strings.Contains(path, "wailsapp/") || path == "miaoyinniangzi/internal/storage" || path == "miaoyinniangzi/internal/sources" || path == "miaoyinniangzi/internal/mpvaudio" {
			t.Fatalf("application initialization dependency in sandbox test binary: %s", path)
		}
	}
}
