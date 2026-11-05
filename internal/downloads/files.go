package downloads

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

func canonicalDirectory(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", ErrDownload
	}
	path, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", ErrDownload
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", ErrDownload
	}
	return path, nil
}
func partName(id string) string { return ".miaoyin-" + id + ".part" }
func regular(root *os.Root, name string, write, create bool) (*os.File, error) {
	if name == "" || filepath.Base(name) != name {
		return nil, ErrDownload
	}
	before, err := root.Lstat(name)
	flags := os.O_RDONLY
	if write {
		flags = os.O_RDWR
	}
	if os.IsNotExist(err) && create {
		flags |= os.O_CREATE | os.O_EXCL
	} else if err != nil {
		return nil, storageFailure(err)
	} else if !before.Mode().IsRegular() {
		return nil, ErrDownload
	}
	file, err := root.OpenFile(name, flags, 0600)
	if err != nil {
		return nil, storageFailure(err)
	}
	after, err := file.Stat()
	current, readErr := root.Lstat(name)
	if err != nil || readErr != nil || !after.Mode().IsRegular() || !os.SameFile(after, current) || before != nil && !os.SameFile(before, after) {
		file.Close()
		return nil, ErrDownload
	}
	return file, nil
}
func digest(file *os.File, size int64) (string, error) {
	before, err := file.Stat()
	if err != nil || size < 0 || before.Size() < size {
		return "", ErrDownload
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.NewSectionReader(file, 0, size))
	after, statErr := file.Stat()
	if err != nil || statErr != nil || n != size || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", ErrDownload
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func verified(root *os.Root, name string, size int64, expected string) bool {
	if expected == "" || size <= 0 {
		return false
	}
	file, err := regular(root, name, false, false)
	if err != nil {
		return false
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || stat.Size() != size {
		return false
	}
	got, err := digest(file, size)
	return err == nil && got == expected
}

// Verify ownership and bytes through one opened target, including a final
// identity check after hashing. Matching bytes alone never authorize adoption.
func verifiedIdentity(root *os.Root, name string, size int64, expected string, identity fileIdentity) bool {
	if expected == "" || size <= 0 {
		return false
	}
	file, err := regular(root, name, false, false)
	if err != nil {
		return false
	}
	defer file.Close()
	actual, err := identityOf(file)
	if err != nil || actual != identity {
		return false
	}
	stat, err := file.Stat()
	if err != nil || stat.Size() != size {
		return false
	}
	sum, err := digest(file, size)
	if err != nil || sum != expected {
		return false
	}
	actual, err = identityOf(file)
	return err == nil && actual == identity
}
func safeText(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, text)
	text = strings.Trim(text, " .")
	if text == "" {
		return "未知"
	}
	return text
}
func fileName(task Task) string {
	name := safeText(task.Song.Artist) + " - " + safeText(task.Song.Title)
	for len(name) > 180 {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return name + " [" + strings.ToUpper(task.Quality) + "]." + task.Codec
}
