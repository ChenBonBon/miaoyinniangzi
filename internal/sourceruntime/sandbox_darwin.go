//go:build darwin

package sourceruntime

/*
#include <stdlib.h>
#include <sandbox.h>
#include <libproc.h>
#include <sys/resource.h>

static int isolate_source_worker(void) {
  char *error = NULL;
  // Goja has no native file/process/network bindings. Seatbelt adds a second
  // boundary should a native interpreter bug ever bypass that restriction.
  const char *profile = "(version 1)(deny default)(allow sysctl-read)";
  int result = sandbox_init(profile, 0, &error);
  if (error != NULL) sandbox_free_error(error);
  return result;
}
static int source_worker_memory(int pid, unsigned long long *bytes) {
  struct rusage_info_v2 usage;
  int result = proc_pid_rusage(pid, RUSAGE_INFO_V2, (rusage_info_t *)&usage);
  if (result == 0) *bytes = usage.ri_resident_size;
  return result;
}
*/
import "C"

import (
	"errors"
	"golang.org/x/sys/unix"
)

func sandboxWorker() error {
	for _, limit := range []struct {
		resource int
		value    uint64
	}{{unix.RLIMIT_CORE, 0}, {unix.RLIMIT_CPU, 2}, {unix.RLIMIT_FSIZE, 0}, {unix.RLIMIT_NOFILE, 64}} {
		if err := unix.Setrlimit(limit.resource, &unix.Rlimit{Cur: limit.value, Max: limit.value}); err != nil {
			return errors.New("worker resource limits unavailable")
		}
	}
	if C.isolate_source_worker() != 0 {
		return errors.New("worker sandbox unavailable")
	}
	return nil
}
func residentBytes(pid int) (uint64, error) {
	var value C.ulonglong
	if C.source_worker_memory(C.int(pid), &value) != 0 {
		return 0, errors.New("cannot monitor worker memory")
	}
	return uint64(value), nil
}
