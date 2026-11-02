package sourceruntime

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Native token security attributes use UNICODE_STRING names and pointer-rich
// structures, not the Win32 CLAIM_SECURITY_ATTRIBUTE representation.
func queryNativeTokenInfo(token windows.Token, class uint32) (*tokenInfoBuffer, error) {
	proc := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryInformationToken")
	if proc.Find() != nil {
		return nil, sandboxFailure("native token query API")
	}
	info := new(tokenInfoBuffer)
	capacity := uint32(unsafe.Sizeof(info.words))
	result, _, _ := proc.Call(uintptr(token), uintptr(class), uintptr(unsafe.Pointer(&info.words[0])), uintptr(capacity), uintptr(unsafe.Pointer(&info.size)))
	status := windows.NTStatus(uint32(result))
	if int32(status) < 0 {
		return nil, sandboxFailure(fmt.Sprintf("native token query class=%d ntstatus=0x%08x", class, uint32(status)), status.Errno())
	}
	if info.size > capacity {
		return nil, sandboxFailure("native token information bounds")
	}
	return info, nil
}

// Token information contains native pointers. A byte array has alignment 1 and
// is not a suitable ABI buffer on every Go stack layout. uintptr storage forces
// pointer alignment, including when the compiler keeps the object on the stack.
type tokenInfoBuffer struct {
	words [8192]uintptr
	size  uint32
}

func queryTokenInfo(token windows.Token, class uint32) (*tokenInfoBuffer, error) {
	info := new(tokenInfoBuffer)
	capacity := uint32(unsafe.Sizeof(info.words))
	err := windows.GetTokenInformation(token, class, (*byte)(unsafe.Pointer(&info.words[0])), capacity, &info.size)
	if err != nil {
		return nil, sandboxFailure(fmt.Sprintf("token query class=%d", class), err)
	}
	if info.size > capacity {
		return nil, sandboxFailure("token information bounds")
	}
	return info, nil
}

// TOKEN_MANDATORY_LABEL and TOKEN_APPCONTAINER_INFORMATION both start with a
// SID pointer. Require the complete SID to remain within the returned buffer
// before handing it to an OS SID function; never dereference a stale pointer.
func (info *tokenInfoBuffer) sid() (*windows.SID, error) {
	if info.size > uint32(unsafe.Sizeof(info.words)) || info.size < uint32(unsafe.Sizeof(uintptr(0))) {
		return nil, sandboxFailure("token SID header")
	}
	pointer := info.words[0]
	if pointer == 0 {
		return nil, nil
	}
	base := unsafe.Pointer(&info.words[0])
	address := uintptr(base)
	if pointer < address || pointer%unsafe.Alignof(uint32(0)) != 0 {
		return nil, sandboxFailure("token SID bounds")
	}
	offset := pointer - address
	if offset < unsafe.Sizeof(uintptr(0)) || offset > uintptr(info.size) || uintptr(info.size)-offset < 8 {
		return nil, sandboxFailure("token SID bounds")
	}
	count := *(*byte)(unsafe.Add(base, offset+1))
	length := uintptr(8) + uintptr(count)*4
	if count > 15 || length > uintptr(info.size)-offset {
		return nil, sandboxFailure("token SID bounds")
	}
	sid := (*windows.SID)(unsafe.Add(base, offset))
	if !sid.IsValid() {
		return nil, sandboxFailure("token SID validity")
	}
	return sid, nil
}
