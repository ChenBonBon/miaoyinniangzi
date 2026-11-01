//go:build darwin

package mpvaudio

import "github.com/ebitengine/purego"

func loadLibrary(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
}
func closeLibrary(handle uintptr) { _ = purego.Dlclose(handle) }
