package downloads

import (
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

func creationTime(info *syscall.Stat_t) int64 {
	return info.Birthtimespec.Sec*1e9 + info.Birthtimespec.Nsec
}
func renameNoReplace(dir *os.File, source, target string, _ fileIdentity) error {
	return unix.RenameatxNp(int(dir.Fd()), source, int(dir.Fd()), target, unix.RENAME_EXCL)
}
