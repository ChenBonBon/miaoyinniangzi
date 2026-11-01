package privatefs

import (
	"errors"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var errPrivate = errors.New("cannot establish private Windows file permissions")

// FILE_ALL_ACCESS (STANDARD_RIGHTS_REQUIRED | SYNCHRONIZE | 0x1ff).
const fileAllAccess uint32 = 0x001f01ff

func open(path string, access uint32) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, errPrivate
	}
	handle, err := windows.CreateFile(name, access|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		windows.CloseHandle(handle)
		return nil, errPrivate
	}
	return os.NewFile(uintptr(handle), path), nil
}
func protect(file *os.File) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	inherit := ""
	if info.IsDir() {
		inherit = "OICI"
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;" + inherit + ";FA;;;SY)(A;" + inherit + ";FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return err
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	err = windows.SetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
	runtime.KeepAlive(descriptor)
	return err
}
func Protect(path string, _ os.FileMode) error {
	file, err := open(path, windows.READ_CONTROL|windows.WRITE_DAC)
	if err != nil {
		return err
	}
	defer file.Close()
	return protect(file)
}
func ProtectFile(file *os.File, _ os.FileMode) error {
	// Generic read/write handles lack WRITE_DAC. Reopen without following reparse
	// points, and prove it is still the caller's file before modifying its ACL.
	other, err := open(file.Name(), windows.READ_CONTROL|windows.WRITE_DAC)
	if err != nil {
		return err
	}
	defer other.Close()
	before, err := file.Stat()
	if err != nil {
		return err
	}
	after, err := other.Stat()
	if err != nil || !os.SameFile(before, after) {
		return errPrivate
	}
	return protect(other)
}
func Check(path string, _ os.FileMode) error {
	file, err := open(path, windows.READ_CONTROL)
	if err != nil {
		return err
	}
	defer file.Close()
	descriptor, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	acl, _, err := descriptor.DACL()
	if err != nil || acl == nil || acl.AceCount != 2 {
		return errPrivate
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	found := map[string]bool{}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			return errPrivate
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
		if sid != "S-1-5-18" && sid != user.User.Sid.String() {
			return errPrivate
		}
		if uint32(ace.Mask)&fileAllAccess != fileAllAccess {
			return errPrivate
		}
		found[sid] = true
	}
	runtime.KeepAlive(descriptor)
	if !found["S-1-5-18"] || !found[user.User.Sid.String()] {
		return errPrivate
	}
	return nil
}
