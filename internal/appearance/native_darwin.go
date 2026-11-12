package appearance

/*
#cgo LDFLAGS: -framework Cocoa
void miaoyinAppearance(int mode);
*/
import "C"

// Apply changes only this application's AppKit appearance, on its main queue.
func Apply(mode string) {
	value := 0
	switch mode {
	case "dark":
		value = 1
	case "light":
		value = 2
	case "system":
	default:
		return
	}
	C.miaoyinAppearance(C.int(value))
}
