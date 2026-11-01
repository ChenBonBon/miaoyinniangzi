package sources

import (
	"context"
	"errors"
	"time"

	"miaoyinniangzi/internal/storage"
)

type scriptJob struct {
	id       string
	revision uint64
	done     chan struct{}
	err      error
}

func (m *Manager) clearScriptLocked() {
	m.scriptRevision++
	clear(m.scriptCache)
	m.scriptCache = nil
	m.scriptCacheID = ""
	if m.scriptChanged != nil {
		close(m.scriptChanged)
	}
	m.scriptChanged = make(chan struct{})
}
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.invalidateResolveLocked()
}

// Warmup reads only the selected encrypted copy and its system-protected key.
// It never initializes a script, requests a music URL, or creates a key.
func (m *Manager) Warmup(ctx context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false, context.Canceled
	}
	entries, err := m.store.Sources(ctx)
	if err != nil {
		return false, errors.New("无法读取当前音源")
	}
	for _, active := range entries {
		if active.Active && active.CheckStatus == "ready" {
			plain, _, err := m.scriptLocked(ctx, active)
			clear(plain)
			return err == nil, err
		}
	}
	return false, nil
}

// Called with the management mutex held, and returns with it held. It releases
// the lock while waiting on the single background key/decryption job, keeping
// disable/delete responsive even if system authorization is waiting for a user.
func (m *Manager) scriptLocked(ctx context.Context, active storage.SourceInfo) ([]byte, bool, error) {
	revision := m.scriptRevision
	waited := false
	for {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		if m.closed || m.scriptRevision != revision {
			return nil, false, errors.New("音源选择已变化，请重新解析")
		}
		if m.scriptCacheID == active.ID && len(m.scriptCache) > 0 {
			return append([]byte(nil), m.scriptCache...), !waited, nil
		}
		job := m.scriptJob
		if job == nil {
			job = &scriptJob{id: active.ID, revision: revision, done: make(chan struct{})}
			m.scriptJob = job
			go m.prepareScript(job)
		}
		waited = true
		changed := m.scriptChanged
		m.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-job.done:
		case <-changed:
		}
		m.mu.Lock()
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		if m.closed || m.scriptRevision != revision {
			return nil, false, errors.New("音源选择已变化，请重新解析")
		}
		if job.revision == revision && job.id == active.ID && job.err != nil {
			return nil, false, job.err
		}
	}
}
func (m *Manager) prepareScript(job *scriptJob) {
	// Native authorization cannot be interrupted by context cancellation. Never
	// retain a late result after invalidation or shutdown; wipe it instead.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	plain, err := m.decryptScript(ctx, job.id)
	m.mu.Lock()
	defer m.mu.Unlock()
	job.err = err
	if m.scriptJob == job {
		m.scriptJob = nil
	}
	if err == nil && !m.closed && m.scriptRevision == job.revision {
		clear(m.scriptCache)
		m.scriptCache = plain
		m.scriptCacheID = job.id
	} else {
		clear(plain)
	}
	close(job.done)
}
func (m *Manager) decryptScript(ctx context.Context, id string) ([]byte, error) {
	encrypted, err := m.store.SourceCiphertext(ctx, id)
	if err != nil {
		return nil, errors.New("无法读取音源副本")
	}
	key, err := m.keys.Key(false)
	if err != nil || len(key) != 32 {
		clear(key)
		return nil, errors.New("无法读取音源密钥，请解锁或授权后重试")
	}
	plain, err := unseal(key, encrypted, id)
	clear(key)
	if err != nil {
		return nil, errors.New("音源副本无法解密，请重新导入")
	}
	if len(plain) == 0 || len(plain) > MaxScriptSize {
		clear(plain)
		return nil, errors.New("音源副本内容异常，请重新导入")
	}
	if err = ctx.Err(); err != nil {
		clear(plain)
		return nil, err
	}
	return plain, nil
}
