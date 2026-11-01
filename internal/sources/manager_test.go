package sources

import (
	"bytes"
	"context"
	"errors"
	"miaoyinniangzi/internal/testfs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"miaoyinniangzi/internal/storage"
)

type fakeKeys struct {
	key     []byte
	err     error
	creates []bool
}

func (k *fakeKeys) Key(create bool) ([]byte, error) {
	k.creates = append(k.creates, create)
	return bytes.Clone(k.key), k.err
}
func fixture(t *testing.T) (*Manager, *storage.Store, *fakeKeys, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := storage.Open(dir, filepath.Join(dir, "Downloads"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	k := &fakeKeys{key: bytes.Repeat([]byte{42}, 32)}
	return New(s, k), s, k, dir
}
func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

const example = "/*!\n * @name 测试音源\n * @author 测试作者\n * @version v1\n */\nconst SECRET='never-persist-plaintext'; throw new Error('must not execute');"

func TestEncryptedImportDuplicateReopenAndDelete(t *testing.T) {
	m, s, k, dir := fixture(t)
	ctx := context.Background()
	path := script(t, t.TempDir(), "source.js", example)
	result, err := m.Import(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Duplicate || result.Source.Name != "测试音源" || result.Source.Author != "测试作者" || result.Source.Version != "v1" {
		t.Fatalf("metadata: %+v", result)
	}
	encrypted, err := s.SourceCiphertext(ctx, result.Source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte("never-persist-plaintext")) {
		t.Fatal("plaintext in ciphertext")
	}
	duplicate, err := m.Import(ctx, path)
	if err != nil || !duplicate.Duplicate || duplicate.Source.ID != result.Source.ID {
		t.Fatalf("duplicate: %+v %v", duplicate, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "miaoyin.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("never-persist-plaintext")) || bytes.Contains(raw, k.key) {
		t.Fatal("secret/key leaked to database")
	}
	reopened, err := storage.Open(dir, filepath.Join(dir, "Downloads"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	m = New(reopened, k)
	list, err := m.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("reopen: %+v %v", list, err)
	}
	encrypted, err = reopened.SourceCiphertext(ctx, list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := unseal(k.key, encrypted, list[0].ID)
	if err != nil || string(plain) != example {
		t.Fatal("independent copy failed")
	}
	clear(plain)
	if err := m.Delete(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.SourceCiphertext(ctx, list[0].ID); err == nil {
		t.Fatal("encrypted copy remained")
	}
	list, err = m.List(ctx)
	if err != nil || len(list) != 0 {
		t.Fatal("deletion failed")
	}
}
func TestRejectInvalidFiles(t *testing.T) {
	m, _, k, _ := fixture(t)
	dir := t.TempDir()
	files := []string{script(t, dir, "empty.js", " "), script(t, dir, "wrong.txt", "const x=1"), script(t, dir, "binary.js", string([]byte{0xff, 0x00})), script(t, dir, "large.js", strings.Repeat("x", MaxScriptSize+1))}
	valid := script(t, dir, "valid.js", "const x=1")
	link := filepath.Join(dir, "link.js")
	t.Run("symlink", func(t *testing.T) {
		testfs.Symlink(t, valid, link)
		if _, err := m.Import(context.Background(), link); err == nil {
			t.Fatal("symlink accepted")
		}
	})
	files = append(files, filepath.Join(dir, "missing.js"), dir)
	for _, file := range files {
		if _, err := m.Import(context.Background(), file); err == nil {
			t.Fatalf("accepted invalid file %s", filepath.Base(file))
		}
	}
	if len(k.creates) != 0 {
		t.Fatal("invalid files reached keychain")
	}
}
func TestKeychainFailureNeverPersistsPlaintext(t *testing.T) {
	m, _, k, _ := fixture(t)
	k.err = errors.New("keychain denied")
	path := script(t, t.TempDir(), "source.js", example)
	if _, err := m.Import(context.Background(), path); err == nil {
		t.Fatal("ignored keychain failure")
	}
	list, err := m.List(context.Background())
	if err != nil || len(list) != 0 {
		t.Fatal("partial record remained")
	}
}
func TestTamperingWrongKeyAndRecordSwapRejected(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	data, err := seal(key, []byte(example), "id-a")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		key, data []byte
		id        string
	}{{bytes.Repeat([]byte{2}, 32), data, "id-a"}, {key, data, "id-b"}, {key, data[:10], "id-a"}} {
		if _, err := unseal(tc.key, tc.data, tc.id); err == nil {
			t.Fatal("accepted tampering")
		}
	}
	data[len(data)-1] ^= 1
	if _, err := unseal(key, data, "id-a"); err == nil {
		t.Fatal("accepted modified ciphertext")
	}
}
func TestExistingCopiesPreventSilentKeyReplacement(t *testing.T) {
	m, _, k, _ := fixture(t)
	dir := t.TempDir()
	ctx := context.Background()
	if _, err := m.Import(ctx, script(t, dir, "a.js", example)); err != nil {
		t.Fatal(err)
	}
	k.key = bytes.Repeat([]byte{5}, 32)
	if _, err := m.Import(ctx, script(t, dir, "b.js", example+"\n//different")); err == nil {
		t.Fatal("accepted wrong vault key")
	}
	if len(k.creates) != 2 || k.creates[1] {
		t.Fatal("requested replacement key")
	}
	entries, err := m.List(ctx)
	if err != nil || len(entries) != 1 {
		t.Fatal("changed existing records")
	}
}
func TestMetadataOnlyReadsBoundedLeadingComment(t *testing.T) {
	v := metadata([]byte("const x='@name bad';"), "fallback.js")
	if v.Name != "fallback" {
		t.Fatal("read executable code for metadata")
	}
	v = metadata([]byte("\xef\xbb\xbf"+example), "a.js")
	if v.Name != "测试音源" {
		t.Fatal("BOM header not supported")
	}
}
func TestSameNameDifferentContentIsNotOverwritten(t *testing.T) {
	m, _, _, _ := fixture(t)
	dir := t.TempDir()
	ctx := context.Background()
	first, err := m.Import(ctx, script(t, dir, "a.js", example))
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Import(ctx, script(t, dir, "b.js", example+"\n// version changed"))
	if err != nil {
		t.Fatal(err)
	}
	if second.Duplicate || first.Source.ID == second.Source.ID {
		t.Fatal("overwrote same-name source")
	}
}
