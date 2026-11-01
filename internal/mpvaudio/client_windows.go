//go:build windows

package mpvaudio

import "golang.org/x/sys/windows"

func loadLibrary(path string) (uintptr, error) {
	// Do not search the current directory for transitive DLL dependencies.
	handle, err := windows.LoadLibraryEx(path, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	return uintptr(handle), err
}
func closeLibrary(handle uintptr) { _ = windows.FreeLibrary(windows.Handle(handle)) }
