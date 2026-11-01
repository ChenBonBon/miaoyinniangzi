//go:build !darwin && !windows

package sources

import "errors"

type SystemKeychain struct{}

func NewSystemKeys(string) KeyProvider { return SystemKeychain{} }

func (SystemKeychain) Key(bool) ([]byte, error) {
	return nil, errors.New("当前系统不支持音源密钥保护")
}
