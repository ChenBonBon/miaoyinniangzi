//go:build windows

package sources

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows keys are protected for the current user, never LOCAL_MACHINE.
// DPAPI normally needs no UI. It does not provide Keychain's application ACL.
type DPAPIKeys struct{ directory string }

func NewSystemKeys(directory string) KeyProvider { return &DPAPIKeys{directory: directory} }

const protectedKeyName = "source-key.dpapi"
const maxProtectedKeySize = 16 * 1024

var errProtectedKey = errors.New("无法读取 Windows 音源密钥，请勿删除或重置现有密钥文件")

func (p *DPAPIKeys) Key(create bool) ([]byte, error) {
	path := filepath.Join(p.directory, protectedKeyName)
	protected, err := readProtectedKey(path)
	if err == nil {
		return unprotectKey(protected)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, errProtectedKey
	}
	if !create {
		return nil, errors.New("Windows 音源密钥缺失，请恢复密钥文件或重新导入音源")
	}
	key := make([]byte, 32)
	defer clear(key)
	if _, err := rand.Read(key); err != nil {
		return nil, errProtectedKey
	}
	protected, err = protectKey(key)
	if err != nil {
		return nil, errProtectedKey
	}
	if err := writeProtectedKey(p.directory, path, protected); err != nil {
		return nil, errProtectedKey
	}
	// Re-read the winner of a concurrent creation, and verify DPAPI round-trip.
	protected, err = readProtectedKey(path)
	if err != nil {
		return nil, errProtectedKey
	}
	return unprotectKey(protected)
}

func protectKey(key []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errProtectedKey
	}
	input := windows.DataBlob{Size: uint32(len(key)), Data: &key[0]}
	var output windows.DataBlob
	err := windows.CryptProtectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	runtime.KeepAlive(key)
	if err != nil {
		return nil, errProtectedKey
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))
	if output.Size == 0 || output.Size > maxProtectedKeySize {
		return nil, errProtectedKey
	}
	data := unsafe.Slice(output.Data, int(output.Size))
	defer clear(data)
	return append([]byte(nil), data...), nil
}

func unprotectKey(protected []byte) ([]byte, error) {
	if len(protected) == 0 || len(protected) > maxProtectedKeySize {
		return nil, errProtectedKey
	}
	input := windows.DataBlob{Size: uint32(len(protected)), Data: &protected[0]}
	var output windows.DataBlob
	err := windows.CryptUnprotectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	runtime.KeepAlive(protected)
	if err != nil {
		return nil, errProtectedKey
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))
	if output.Size > maxProtectedKeySize {
		return nil, errProtectedKey
	}
	data := unsafe.Slice(output.Data, int(output.Size))
	defer clear(data)
	if len(data) != 32 {
		return nil, errProtectedKey
	}
	return append([]byte(nil), data...), nil
}

func readProtectedKey(path string) ([]byte, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, errProtectedKey
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return nil, os.ErrNotExist
		}
		return nil, errProtectedKey
	}
	file := os.NewFile(uintptr(handle), path)
	defer file.Close()
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return nil, errProtectedKey
	}
	data, err := io.ReadAll(io.LimitReader(file, maxProtectedKeySize+1))
	if err != nil || len(data) == 0 || len(data) > maxProtectedKeySize {
		return nil, errProtectedKey
	}
	return data, nil
}

func writeProtectedKey(directory, path string, protected []byte) error {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	dirName, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(dirName)
	if err != nil || attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errProtectedKey
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	// Protected DACL: only this user and SYSTEM; never rely on chmod on Windows.
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return err
	}
	security := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: descriptor}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	temporary := filepath.Join(directory, ".source-key-"+hex.EncodeToString(nonce)+".tmp")
	temporaryName, err := windows.UTF16PtrFromString(temporary)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(temporaryName, windows.GENERIC_WRITE, 0, &security,
		windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	runtime.KeepAlive(descriptor)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), temporary)
	defer os.Remove(temporary)
	_, writeErr := file.Write(protected)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	target, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	// Same-directory atomic publication; deliberately no REPLACE_EXISTING.
	err = windows.MoveFileEx(temporaryName, target, windows.MOVEFILE_WRITE_THROUGH)
	if err != nil {
		// A winner opened without FILE_SHARE_DELETE can also make the losing
		// move fail with ACCESS_DENIED. Accept only a fully readable winner;
		// Key subsequently authenticates it through DPAPI.
		if _, readErr := readProtectedKey(path); readErr == nil {
			return nil
		}
	}
	return err
}
