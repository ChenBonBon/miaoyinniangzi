//go:build windows

package sources

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"golang.org/x/sys/windows"
)

func TestDPAPIKeysRoundTripAndReopen(t *testing.T) {
	directory := t.TempDir()
	provider := NewSystemKeys(directory)
	if _, err := provider.Key(false); err == nil {
		t.Fatal("missing key accepted")
	}
	path := filepath.Join(directory, protectedKeyName)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("read created a key")
	}
	key, err := provider.Key(true)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	if len(key) != 32 {
		t.Fatal("wrong key length")
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(disk, key) {
		t.Fatal("plaintext key written to disk")
	}
	reopened, err := NewSystemKeys(directory).Key(false)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(reopened)
	if !bytes.Equal(key, reopened) {
		t.Fatal("key changed after reopening")
	}
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil || dacl.AceCount != 2 {
		t.Fatal("key DACL does not contain exactly the user and SYSTEM grants")
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("key DACL inherits broad permissions")
	}
}

func TestDPAPIKeysCorruptionNeverOverwrites(t *testing.T) {
	for _, contents := range [][]byte{nil, []byte("invalid"), bytes.Repeat([]byte{1}, maxProtectedKeySize+1)} {
		directory := t.TempDir()
		path := filepath.Join(directory, protectedKeyName)
		if err := os.WriteFile(path, contents, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewSystemKeys(directory).Key(true); err == nil {
			t.Fatal("corrupt key reset")
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, contents) {
			t.Fatal("corrupt key was modified")
		}
	}
}

func TestDPAPIKeysConcurrentCreationUsesOneKey(t *testing.T) {
	directory := t.TempDir()
	var group sync.WaitGroup
	keys := make(chan []byte, 8)
	failures := make(chan error, 8)
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			key, err := NewSystemKeys(directory).Key(true)
			if err != nil {
				failures <- err
			} else {
				keys <- key
			}
		}()
	}
	group.Wait()
	close(keys)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var first []byte
	for key := range keys {
		if first == nil {
			first = append([]byte(nil), key...)
		}
		if !bytes.Equal(first, key) {
			t.Fatal("concurrent creation forked keys")
		}
		clear(key)
	}
	defer clear(first)
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || entries[0].Name() != protectedKeyName {
		t.Fatal("temporary key files remain")
	}
}

func TestDPAPIDamagedCiphertextRejected(t *testing.T) {
	key := bytes.Repeat([]byte{42}, 32)
	defer clear(key)
	protected, err := protectKey(key)
	if err != nil {
		t.Fatal(err)
	}
	protected[len(protected)-1] ^= 0xff
	if _, err := unprotectKey(protected); err == nil {
		t.Fatal("damaged DPAPI blob accepted")
	}
}
