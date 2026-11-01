//go:build darwin

package sources

import (
	"crypto/rand"
	"errors"

	"github.com/keybase/go-keychain"
)

// SystemKeychain uses the native Security framework, not a shell command whose
// arguments could expose a key in process listings. It never enables sync.
type SystemKeychain struct{}

func NewSystemKeys(string) KeyProvider { return SystemKeychain{} }

const keyService = "com.miaoyinniangzi.sources"
const keyAccount = "aes256-gcm-v1"

func (SystemKeychain) Key(create bool) ([]byte, error) {
	key, err := keychain.GetGenericPassword(keyService, keyAccount, "", "")
	if err == nil && len(key) == 32 {
		return key, nil
	}
	if err != nil && !errors.Is(err, keychain.ErrorItemNotFound) {
		return nil, errors.New("无法访问 macOS 钥匙串，请解锁或授权后重试")
	}
	if len(key) != 0 {
		return nil, errors.New("音源加密密钥异常，未修改已保存副本")
	}
	if !create {
		return nil, errors.New("音源密钥缺失，无法读取现有副本，请重新导入")
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, errors.New("无法生成音源加密密钥")
	}
	defer clear(key)
	item := keychain.NewGenericPassword(keyService, keyAccount, "妙音娘子音源加密密钥", key, "")
	item.SetSynchronizable(keychain.SynchronizableNo)
	item.SetAccessible(keychain.AccessibleWhenUnlockedThisDeviceOnly)
	if err := keychain.AddItem(item); err != nil && !errors.Is(err, keychain.ErrorDuplicateItem) {
		return nil, errors.New("无法将音源密钥保存到 macOS 钥匙串")
	}
	// Re-read both for duplicate races and to verify the key is actually usable.
	result, err := keychain.GetGenericPassword(keyService, keyAccount, "", "")
	if err != nil || len(result) != 32 {
		return nil, errors.New("无法读取 macOS 钥匙串音源密钥")
	}
	return result, nil
}
