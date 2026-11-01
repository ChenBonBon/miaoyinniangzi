//go:build !darwin

package sourceruntime

import "errors"

func sandboxWorker() error { return errors.New("source workers currently require macOS") }
func residentBytes(int) (uint64, error) {
	return 0, errors.New("memory monitoring currently requires macOS")
}
