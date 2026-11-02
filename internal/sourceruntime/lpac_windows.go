package sourceruntime

import (
	"fmt"
	"runtime"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const lpacAttributeName = "WIN://NOALLAPPPKG"

// TOKEN_SECURITY_ATTRIBUTES_INFORMATION and TOKEN_SECURITY_ATTRIBUTE_V1.
// Unlike CLAIM_SECURITY_ATTRIBUTE_V1, the native Name is a UNICODE_STRING.
type nativeSecurityAttributes struct {
	Version, Reserved uint16
	Count             uint32
	Attributes        uintptr
}
type nativeSecurityAttribute struct {
	Name                windows.NTUnicodeString
	ValueType, Reserved uint16
	Flags, Count        uint32
	Values              uintptr
}

func (info *tokenInfoBuffer) region(address, length, alignment uintptr) (unsafe.Pointer, error) {
	base := unsafe.Pointer(&info.words[0])
	start := uintptr(base)
	if info.size > uint32(unsafe.Sizeof(info.words)) || address < start || alignment == 0 || address%alignment != 0 {
		return nil, sandboxFailure("token attribute bounds")
	}
	offset := address - start
	if offset < unsafe.Sizeof(nativeSecurityAttributes{}) || offset > uintptr(info.size) || length > uintptr(info.size)-offset {
		return nil, sandboxFailure("token attribute bounds")
	}
	return unsafe.Add(base, offset), nil
}
func tokenLPACFlag(token windows.Token) (uint32, error) {
	app, err := tokenValue(token, 29)
	if err != nil {
		return 0, err
	}
	if app != 1 {
		return 0, nil
	}
	info, err := queryNativeTokenInfo(token, 39) // TokenSecurityAttributes, not unsupported class 46.
	if err != nil {
		return 0, err
	}
	flag, err := lpacAttributeFlag(info)
	runtime.KeepAlive(info)
	return flag, err
}
func lpacAttributeFlag(info *tokenInfoBuffer) (uint32, error) {
	if info.size < uint32(unsafe.Sizeof(nativeSecurityAttributes{})) || info.size > uint32(unsafe.Sizeof(info.words)) {
		return 0, sandboxFailure("token attribute header")
	}
	header := (*nativeSecurityAttributes)(unsafe.Pointer(&info.words[0]))
	// An empty attribute set cannot prove LPAC, regardless of its version.
	if header.Count == 0 {
		return 0, nil
	}
	if header.Version != 1 || header.Reserved != 0 || header.Count > 64 {
		return 0, sandboxFailure(fmt.Sprintf("token attribute header version=%d count=%d", header.Version, header.Count))
	}
	array, err := info.region(header.Attributes, uintptr(header.Count)*unsafe.Sizeof(nativeSecurityAttribute{}), unsafe.Alignof(nativeSecurityAttribute{}))
	if err != nil {
		return 0, err
	}
	found := false
	var flag uint32
	for _, attribute := range unsafe.Slice((*nativeSecurityAttribute)(array), int(header.Count)) {
		if attribute.Name.Length == 0 || attribute.Name.Length%2 != 0 || attribute.Name.Length > 512 || attribute.Name.Length > attribute.Name.MaximumLength {
			return 0, sandboxFailure("token attribute name")
		}
		name, err := info.region(uintptr(unsafe.Pointer(attribute.Name.Buffer)), uintptr(attribute.Name.Length), 2)
		if err != nil {
			return 0, err
		}
		text := string(utf16.Decode(unsafe.Slice((*uint16)(name), int(attribute.Name.Length)/2)))
		if !strings.EqualFold(text, lpacAttributeName) {
			continue
		}
		// The system-managed UInt64 marker is the effective ALL_APPLICATION_PACKAGES
		// opt-out used by LPAC. Missing/disabled/ill-typed/duplicate markers never pass.
		if found || attribute.Reserved != 0 || attribute.ValueType != 2 || attribute.Count != 1 || attribute.Flags&(0x4|0x10) != 0 {
			return 0, sandboxFailure(fmt.Sprintf("token LPAC attribute policy type=%d count=%d flags=0x%x duplicate=%t", attribute.ValueType, attribute.Count, attribute.Flags, found))
		}
		value, err := info.region(attribute.Values, 8, 8)
		if err != nil {
			return 0, err
		}
		number := *(*uint64)(value)
		if number > 1 {
			return 0, sandboxFailure("token LPAC attribute value")
		}
		flag = uint32(number)
		found = true
	}
	return flag, nil
}
