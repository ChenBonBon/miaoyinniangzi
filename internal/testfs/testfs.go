// Package testfs contains filesystem test helpers, never application policy.
package testfs

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"testing"
)

// Symlink skips only callers' symlink-specific subtests when an ordinary Windows
// user cannot create symlinks. Other errors remain failures. No elevation needed.
func Symlink(t testing.TB, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) {
			t.Skip("Windows symlink privilege unavailable; enable Developer Mode to run this subtest")
		}
		t.Fatal(err)
	}
}

// RenamePinnedDenied distinguishes Windows' stronger OS directory pinning from
// arbitrary errors, which must not be hidden by a platform skip.
func RenamePinnedDenied(err error) bool {
	return runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(32))
}
