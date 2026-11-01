package sourceruntime

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsTokenInformationBuffers(t *testing.T) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		t.Fatal(sandboxFailure("test token open", err))
	}
	defer token.Close()
	for i := 0; i < 32; i++ {
		for _, class := range []uint32{windows.TokenUser, windows.TokenIntegrityLevel} {
			info, err := queryTokenInfo(token, class)
			if err != nil {
				t.Fatal(err)
			}
			if uintptr(unsafe.Pointer(&info.words[0]))%unsafe.Alignof(uintptr(0)) != 0 {
				t.Fatal("native token buffer is not pointer aligned")
			}
			sid, err := info.sid()
			if err != nil || sid == nil {
				t.Fatal("native token SID buffer invalid")
			}
			if class == windows.TokenIntegrityLevel && sid.SubAuthorityCount() == 0 {
				t.Fatal("integrity label missing")
			}
			runtime.KeepAlive(info)
		}
	}
}
func TestWindowsTokenSIDRejectsOutOfBufferPointers(t *testing.T) {
	info := new(tokenInfoBuffer)
	info.size = 64
	base := uintptr(unsafe.Pointer(&info.words[0]))
	for _, pointer := range []uintptr{base - 1, base, base + 9, base + 63, base + unsafe.Sizeof(info.words)} {
		info.words[0] = pointer
		if _, err := info.sid(); err == nil {
			t.Fatal("invalid native SID pointer accepted")
		}
	}
	info.size = uint32(unsafe.Sizeof(info.words)) + 1
	if _, err := info.sid(); err == nil {
		t.Fatal("oversized native token buffer accepted")
	}
	info.size = uint32(unsafe.Sizeof(uintptr(0)))
	info.words[0] = 0
	if sid, err := info.sid(); err != nil || sid != nil {
		t.Fatal("null non-container SID not represented safely")
	}
}
