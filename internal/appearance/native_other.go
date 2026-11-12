//go:build !darwin

package appearance

// Windows native window appearance is applied through Wails' frontend runtime.
func Apply(string) {}
