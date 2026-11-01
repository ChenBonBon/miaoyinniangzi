// Package sources stores encrypted scripts and delegates explicit compatibility
// checks to an isolated process. Importing a script never executes it.
package sources

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"miaoyinniangzi/internal/sourceruntime"
	"miaoyinniangzi/internal/storage"
)

const MaxScriptSize = 1024 * 1024

type KeyProvider interface {
	Key(create bool) ([]byte, error)
}
type Manager struct {
	store             *storage.Store
	keys              KeyProvider
	mu                sync.Mutex
	checker           sourceruntime.Inspector
	selectionRevision uint64
	resolveCancel     context.CancelFunc
	scriptRevision    uint64
	scriptCache       []byte
	scriptCacheID     string
	scriptChanged     chan struct{}
	scriptJob         *scriptJob
	closed            bool
}
type ImportResult struct {
	Source          storage.SourceInfo `json:"source"`
	Duplicate       bool               `json:"duplicate"`
	ValidationError string             `json:"validationError,omitempty"`
}

func New(store *storage.Store, keys KeyProvider, inspectors ...sourceruntime.Inspector) *Manager {
	m := &Manager{store: store, keys: keys, scriptChanged: make(chan struct{})}
	if len(inspectors) > 0 {
		m.checker = inspectors[0]
	}
	return m
}

func readScript(path string) ([]byte, error) {
	if !strings.EqualFold(filepath.Ext(path), ".js") {
		return nil, errors.New("请选择 .js 音源脚本")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return nil, errors.New("请选择可读取的普通脚本文件，不支持符号链接")
	}
	if before.Size() > MaxScriptSize {
		return nil, errors.New("音源脚本不能超过 1MB")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("无法读取音源脚本")
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, errors.New("脚本文件发生变化，请重新选择")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxScriptSize+1))
	if err != nil {
		clear(data)
		return nil, errors.New("无法读取音源脚本")
	}
	if len(data) > MaxScriptSize {
		clear(data)
		return nil, errors.New("音源脚本不能超过 1MB")
	}
	if len(bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))) == 0 || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		clear(data)
		return nil, errors.New("音源必须是非空 UTF-8 文本脚本")
	}
	return data, nil
}

var tagPattern = regexp.MustCompile(`(?m)^\s*\*?\s*@(name|author|version)\s+([^\r\n]+)`)

func cleanLabel(value string, limit int) string {
	value = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, value))
	runes := []rune(value)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}
func metadata(data []byte, path string) storage.SourceInfo {
	v := storage.SourceInfo{Name: cleanLabel(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), 120)}
	head := bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	if len(head) > 16384 {
		head = head[:16384]
	}
	if bytes.HasPrefix(head, []byte("/*")) {
		if end := bytes.Index(head, []byte("*/")); end >= 0 {
			for _, match := range tagPattern.FindAllSubmatch(head[:end], -1) {
				switch string(match[1]) {
				case "name":
					if name := cleanLabel(string(match[2]), 120); name != "" {
						v.Name = name
					}
				case "author":
					v.Author = cleanLabel(string(match[2]), 80)
				case "version":
					v.Version = cleanLabel(string(match[2]), 40)
				}
			}
		}
	}
	if v.Name == "" {
		v.Name = "未命名音源"
	}
	return v
}
func seal(key, plaintext []byte, id string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, []byte("miaoyin/source/v1/"+id)), nil
}
func unseal(key, data []byte, id string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize()+gcm.Overhead() {
		return nil, errors.New("invalid encrypted source")
	}
	return gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], []byte("miaoyin/source/v1/"+id))
}

func (m *Manager) Import(ctx context.Context, path string) (ImportResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := readScript(path)
	if err != nil {
		return ImportResult{}, err
	}
	defer clear(data)
	digestBytes := sha256.Sum256(data)
	digest := hex.EncodeToString(digestBytes[:])
	existing, found, err := m.store.SourceByDigest(ctx, digest)
	if err != nil {
		return ImportResult{}, errors.New("无法查询本地音源")
	}
	if found {
		return ImportResult{Source: existing, Duplicate: true}, nil
	}
	entries, err := m.store.Sources(ctx)
	if err != nil {
		return ImportResult{}, errors.New("无法查询本地音源")
	}
	key, err := m.keys.Key(len(entries) == 0)
	if err != nil {
		return ImportResult{}, err
	}
	defer clear(key)
	if len(key) != 32 {
		return ImportResult{}, errors.New("音源加密密钥异常")
	}
	// Never silently replace a lost/wrong key while existing copies are present.
	if len(entries) > 0 {
		encrypted, err := m.store.SourceCiphertext(ctx, entries[0].ID)
		if err != nil {
			return ImportResult{}, errors.New("无法验证已有音源副本")
		}
		plain, err := unseal(key, encrypted, entries[0].ID)
		clear(plain)
		if err != nil {
			return ImportResult{}, errors.New("已有音源副本无法解密，请检查钥匙串或重新导入")
		}
	}
	info := metadata(data, path)
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return ImportResult{}, errors.New("无法生成音源标识")
	}
	info.ID = hex.EncodeToString(id)
	info.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	encrypted, err := seal(key, data, info.ID)
	if err != nil {
		return ImportResult{}, errors.New("无法加密音源，未保存明文副本")
	}
	result, duplicate, err := m.store.InsertSource(ctx, info, digest, encrypted)
	if err != nil {
		return ImportResult{}, errors.New("保存音源失败，未创建副本")
	}
	return ImportResult{Source: result, Duplicate: duplicate}, nil
}
func (m *Manager) List(ctx context.Context) ([]storage.SourceInfo, error) {
	result, err := m.store.Sources(ctx)
	if err != nil {
		return nil, errors.New("无法读取本地音源列表")
	}
	return result, nil
}
func (m *Manager) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, _ := m.store.Source(ctx, id)
	if current.Active {
		m.invalidateResolveLocked()
	}
	if err := m.store.DeleteSource(ctx, id); err != nil {
		return errors.New("删除音源失败，请刷新列表后重试")
	}
	return nil
}
