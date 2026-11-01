package sources

import (
	"context"
	"errors"

	"miaoyinniangzi/internal/sourceruntime"
	"miaoyinniangzi/internal/storage"
)

// Check and activation share the import/delete mutex, so a script cannot be
// removed or replaced midway through verification in this app instance.
func (m *Manager) Check(ctx context.Context, id string) (storage.SourceInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.checkLocked(ctx, id)
}
func (m *Manager) checkLocked(ctx context.Context, id string) (storage.SourceInfo, error) {
	current, readErr := m.store.Source(ctx, id)
	if current.Active {
		m.invalidateResolveLocked()
	}
	if readErr != nil {
		return storage.SourceInfo{}, errors.New("音源不存在，请刷新列表")
	}
	report := sourceruntime.Report{Qualities: []string{}}
	data, err := m.store.SourceCiphertext(ctx, id)
	if err != nil {
		return storage.SourceInfo{}, errors.New("无法读取音源副本")
	}
	key, err := m.keys.Key(false)
	defer clear(key)
	if err != nil {
		report.Error = "无法读取音源密钥，请解锁钥匙串并授权后重试"
	} else if len(key) != 32 {
		report.Error = "音源加密密钥异常"
	} else {
		plain, err := unseal(key, data, id)
		if err != nil {
			report.Error = "音源副本无法解密，请重新导入"
		} else {
			defer clear(plain)
			if m.checker == nil {
				report.Error = "音源隔离运行器尚未配置"
			} else {
				report, err = m.checker.Inspect(ctx, plain)
				if err != nil {
					report = sourceruntime.Report{Error: "音源隔离校验失败或超出资源限制，请重试", Qualities: []string{}}
				}
			}
		}
	}
	status := "failed"
	if report.Ready {
		status = "ready"
	}
	if err := m.store.RecordSourceCheck(ctx, id, status, report.Error, sourceruntime.EngineVersion, report.Qualities); err != nil {
		return storage.SourceInfo{}, errors.New("无法保存音源校验结果")
	}
	result, err := m.store.Source(ctx, id)
	if err != nil {
		return storage.SourceInfo{}, errors.New("无法读取音源校验结果")
	}
	return result, nil
}
func (m *Manager) Activate(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalidateResolveLocked()
	// Re-decrypt and re-check at every activation, even if the cached badge says
	// ready. A lost Keychain key or tampered copy must not remain selectable.
	info, err := m.checkLocked(ctx, id)
	if err != nil {
		return err
	}
	if info.CheckStatus != "ready" {
		return errors.New(info.CheckError)
	}
	if err := m.store.ActivateSource(ctx, id, sourceruntime.EngineVersion); err != nil {
		return errors.New("无法启用音源，请重试")
	}
	return nil
}
func (m *Manager) Disable(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalidateResolveLocked()
	if err := m.store.DisableSources(ctx); err != nil {
		return errors.New("无法禁用音源，请重试")
	}
	return nil
}
