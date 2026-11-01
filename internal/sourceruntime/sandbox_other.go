//go:build !darwin && !windows

package sourceruntime

import "errors"

func sandboxWorker() error { return errors.New("source workers require an implemented OS sandbox") }
func residentBytes(int) (uint64, error) {
	return 0, errors.New("memory monitoring requires an implemented OS backend")
}
