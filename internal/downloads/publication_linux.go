package downloads

import (
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

func creationTime(*syscall.Stat_t) int64 { return 0 }
func renameNoReplace(dir *os.File, source, target string, _ fileIdentity) error {
	return unix.Renameat2(int(dir.Fd()), source, int(dir.Fd()), target, unix.RENAME_NOREPLACE)
}
