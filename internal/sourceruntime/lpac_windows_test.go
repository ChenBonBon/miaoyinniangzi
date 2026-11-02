package sourceruntime

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func lpacFixture() (info *tokenInfoBuffer, header *nativeSecurityAttributes, attribute *nativeSecurityAttribute) {
	info = new(tokenInfoBuffer)
	info.size = 1024
	base := unsafe.Pointer(&info.words[0])
	header = (*nativeSecurityAttributes)(base)
	header.Version = 1
	header.Count = 1
	header.Attributes = uintptr(unsafe.Add(base, 32))
	attribute = (*nativeSecurityAttribute)(unsafe.Add(base, 32))
	encoded, _ := windows.UTF16FromString(lpacAttributeName)
	name := (*uint16)(unsafe.Add(base, 256))
	copy(unsafe.Slice(name, len(encoded)), encoded)
	attribute.Name = windows.NTUnicodeString{Length: uint16((len(encoded) - 1) * 2), MaximumLength: uint16(len(encoded) * 2), Buffer: name}
	attribute.ValueType = 2
	attribute.Count = 1
	attribute.Values = uintptr(unsafe.Add(base, 512))
	*(*uint64)(unsafe.Add(base, 512)) = 1
	return
}
func TestWindowsLPACAttributes(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) == 8 && (unsafe.Sizeof(nativeSecurityAttributes{}) != 16 || unsafe.Sizeof(nativeSecurityAttribute{}) != 40) {
		t.Fatal("native security attribute ABI layout mismatch")
	}
	info, _, _ := lpacFixture()
	if value, err := lpacAttributeFlag(info); err != nil || value != 1 {
		t.Fatal("valid LPAC marker rejected")
	}
	t.Run("missing-is-not-LPAC", func(t *testing.T) {
		info, h, _ := lpacFixture()
		h.Count = 0
		h.Attributes = 0
		if flag, err := lpacAttributeFlag(info); err != nil || flag != 0 {
			t.Fatal("missing marker did not fail closed")
		}
	})
	t.Run("zero-is-not-LPAC", func(t *testing.T) {
		info, _, a := lpacFixture()
		p, _ := info.region(a.Values, 8, 8)
		*(*uint64)(p) = 0
		if flag, err := lpacAttributeFlag(info); err != nil || flag != 0 {
			t.Fatal("zero marker accepted")
		}
	})
	cases := []struct {
		name   string
		change func(*tokenInfoBuffer, *nativeSecurityAttributes, *nativeSecurityAttribute)
	}{
		{"version", func(_ *tokenInfoBuffer, h *nativeSecurityAttributes, _ *nativeSecurityAttribute) { h.Version = 2 }},
		{"count", func(_ *tokenInfoBuffer, h *nativeSecurityAttributes, _ *nativeSecurityAttribute) { h.Count = 65 }},
		{"array-bounds", func(i *tokenInfoBuffer, h *nativeSecurityAttributes, _ *nativeSecurityAttribute) {
			h.Attributes = uintptr(unsafe.Pointer(&i.words[0])) + uintptr(i.size)
		}},
		{"name-bounds", func(i *tokenInfoBuffer, _ *nativeSecurityAttributes, a *nativeSecurityAttribute) {
			a.Name.Buffer = (*uint16)(unsafe.Add(unsafe.Pointer(&i.words[0]), i.size-2))
		}},
		{"odd-name", func(_ *tokenInfoBuffer, _ *nativeSecurityAttributes, a *nativeSecurityAttribute) { a.Name.Length-- }},
		{"wrong-type", func(_ *tokenInfoBuffer, _ *nativeSecurityAttributes, a *nativeSecurityAttribute) { a.ValueType = 6 }},
		{"empty-values", func(_ *tokenInfoBuffer, _ *nativeSecurityAttributes, a *nativeSecurityAttribute) { a.Count = 0 }},
		{"disabled", func(_ *tokenInfoBuffer, _ *nativeSecurityAttributes, a *nativeSecurityAttribute) { a.Flags = 0x10 }},
		{"deny-only", func(_ *tokenInfoBuffer, _ *nativeSecurityAttributes, a *nativeSecurityAttribute) { a.Flags = 4 }},
		{"value-bounds", func(i *tokenInfoBuffer, _ *nativeSecurityAttributes, a *nativeSecurityAttribute) {
			a.Values = uintptr(unsafe.Pointer(&i.words[0])) + uintptr(i.size)
		}},
		{"unaligned-value", func(_ *tokenInfoBuffer, _ *nativeSecurityAttributes, a *nativeSecurityAttribute) { a.Values++ }},
		{"unknown-value", func(i *tokenInfoBuffer, _ *nativeSecurityAttributes, a *nativeSecurityAttribute) {
			p, _ := i.region(a.Values, 8, 8)
			*(*uint64)(p) = 2
		}},
		{"duplicate", func(i *tokenInfoBuffer, h *nativeSecurityAttributes, a *nativeSecurityAttribute) {
			h.Count = 2
			other := (*nativeSecurityAttribute)(unsafe.Add(unsafe.Pointer(&i.words[0]), 32+unsafe.Sizeof(*a)))
			*other = *a
		}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			info, h, a := lpacFixture()
			item.change(info, h, a)
			if _, err := lpacAttributeFlag(info); err == nil {
				t.Fatal("invalid LPAC claim accepted")
			}
		})
	}
}
