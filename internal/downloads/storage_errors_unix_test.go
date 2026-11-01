//go:build darwin || linux

package downloads

import (
	"errors"
	"fmt"
	"syscall"
	"testing"
)

func TestStorageFailuresAreActionableAndNeverExposeRawPaths(t *testing.T) {
	for _, sample := range []struct{ cause, errorType error }{{syscall.ENOSPC, ErrDiskFull}, {syscall.EDQUOT, ErrDiskFull}, {syscall.ENODEV, ErrDirectoryUnavailable}, {syscall.EIO, ErrDirectoryUnavailable}, {syscall.EACCES, ErrDirectoryPermission}, {syscall.EROFS, ErrDirectoryPermission}, {syscall.ENOTSUP, ErrPublicationUnsupported}} {
		classified := storageFailure(fmt.Errorf("private-path: %w", sample.cause))
		if !errors.Is(classified, sample.errorType) || failureMessage(classified) != sample.errorType.Error() {
			t.Fatal("storage failure lost safe classification")
		}
	}
	if failureMessage(fmt.Errorf("private path and signed URL")) != ErrDownload.Error() {
		t.Fatal("raw filesystem details leaked")
	}
}
