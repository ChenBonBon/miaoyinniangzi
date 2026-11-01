package privatefs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsPrivateACLRejectsAndRemovesEveryoneAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := open(path, windows.READ_CONTROL|windows.WRITE_DAC)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	err = windows.SetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
	runtime.KeepAlive(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if Check(path, 0600) == nil {
		t.Fatal("Everyone access accepted as private")
	}
	if err = Protect(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err = Check(path, 0600); err != nil {
		t.Fatal(err)
	}
}
