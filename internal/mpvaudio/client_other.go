//go:build !darwin && !windows

package mpvaudio

func loadLibrary(string) (uintptr, error) { return 0, ErrLibrary }
func closeLibrary(uintptr)                {}
