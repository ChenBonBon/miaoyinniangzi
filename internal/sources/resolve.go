package sources

import (
	"context"
	"errors"
	"time"

	"miaoyinniangzi/internal/sourceruntime"
	"miaoyinniangzi/internal/storage"
)

type ResolveResult struct {
	SongID           string `json:"songId"`
	SourceID         string `json:"sourceId"`
	RequestedQuality string `json:"requestedQuality"`
	Quality          string `json:"quality"`
	Downgraded       bool   `json:"downgraded"`
	URL              string `json:"url"`
}

func (m *Manager) invalidateResolveLocked() {
	m.clearScriptLocked()
	m.cancelResolveLocked()
}
func (m *Manager) cancelResolveLocked() {
	m.selectionRevision++
	if m.resolveCancel != nil {
		m.resolveCancel()
		m.resolveCancel = nil
	}
}
func qualityCandidates(preferred string, available []string) []string {
	result := []string{}
	enabled := preferred == "highest"
	for _, q := range []string{"flac", "320k", "128k"} {
		if q == preferred {
			enabled = true
		}
		if enabled {
			for _, v := range available {
				if q == v {
					result = append(result, q)
					break
				}
			}
		}
	}
	return result
}

type ResolvePhase uint8

const (
	PhaseSnapshot ResolvePhase = iota + 1
	PhaseScriptHit
	PhaseScriptPrepare
	PhaseRequest
)

type ResolveObserver func(ResolvePhase, time.Duration)

func (m *Manager) Resolve(ctx context.Context, info sourceruntime.ResolveInput, preferred string) (ResolveResult, error) {
	return m.ResolveObserved(ctx, info, preferred, nil)
}

// Decryption/authorization waits release the management mutex; mutations revoke
// cached plaintext and invalidate all in-flight results before publication.
func (m *Manager) ResolveObserved(ctx context.Context, info sourceruntime.ResolveInput, preferred string, observer ResolveObserver) (ResolveResult, error) {
	return m.resolve(ctx, info, preferred, observer, true)
}

// Downloads have an independent request lifetime: they must not cancel playback
// or neighbor preparation. Source mutations still revoke both through scriptChanged.
func (m *Manager) ResolveDownload(ctx context.Context, info sourceruntime.ResolveInput, preferred string) (ResolveResult, error) {
	return m.resolve(ctx, info, preferred, nil, false)
}
func (m *Manager) resolve(ctx context.Context, info sourceruntime.ResolveInput, preferred string, observer ResolveObserver, foreground bool) (ResolveResult, error) {
	started := time.Now()
	m.mu.Lock()
	sources, err := m.store.Sources(ctx)
	if err != nil {
		m.mu.Unlock()
		return ResolveResult{}, errors.New("无法读取当前音源")
	}
	var active storage.SourceInfo
	for _, s := range sources {
		if s.Active && s.CheckStatus == "ready" {
			active = s
			break
		}
	}
	if active.ID == "" {
		m.mu.Unlock()
		return ResolveResult{}, errors.New("请先在设置中启用一个已校验的音源")
	}
	resolver, ok := m.checker.(sourceruntime.Resolver)
	if !ok {
		m.mu.Unlock()
		return ResolveResult{}, errors.New("音源解析运行器尚未配置")
	}
	qualities := qualityCandidates(preferred, active.Qualities)
	if len(qualities) == 0 {
		m.mu.Unlock()
		return ResolveResult{}, errors.New("当前音源没有可用的普通音质")
	}
	// Resolve the automatic preference to the best declared ordinary quality.
	// The response carries this concrete target, so choosing the highest declared
	// quality is not falsely reported as a downgrade from the word "highest".
	if preferred == "highest" {
		preferred = qualities[0]
	}
	if observer != nil {
		observer(PhaseSnapshot, time.Since(started))
	}
	if foreground {
		m.cancelResolveLocked()
	}
	revision, scriptRevision := m.selectionRevision, m.scriptRevision
	changed := m.scriptChanged
	ctx, cancel := context.WithCancel(ctx)
	if foreground {
		m.resolveCancel = cancel
	} else {
		go func() {
			select {
			case <-ctx.Done():
			case <-changed:
				cancel()
			}
		}()
	}
	defer cancel()
	defer func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if foreground && m.selectionRevision == revision {
			m.resolveCancel = nil
		}
	}()
	started = time.Now()
	plain, hit, err := m.scriptLocked(ctx, active)
	if observer != nil {
		phase := PhaseScriptPrepare
		if hit {
			phase = PhaseScriptHit
		}
		observer(phase, time.Since(started))
	}
	if err != nil {
		m.mu.Unlock()
		return ResolveResult{}, err
	}
	defer clear(plain)
	m.mu.Unlock()
	for _, quality := range qualities {
		started = time.Now()
		result, err := resolver.Resolve(ctx, plain, info, quality)
		if observer != nil {
			observer(PhaseRequest, time.Since(started))
		}
		if ctx.Err() != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ResolveResult{}, sourceruntime.ErrResolveTimeout
			}
			return ResolveResult{}, errors.New("音源解析已取消或音源选择发生变化")
		}
		if err != nil {
			if message, safe := sourceruntime.PublicResolveError(err); safe {
				return ResolveResult{}, errors.New(message)
			}
			return ResolveResult{}, errors.New("音源解析失败，请重试")
		}
		if result.Error != "" {
			if result.Error == sourceruntime.MessageNetworkPolicy || result.Error == sourceruntime.MessageServiceRestricted {
				return ResolveResult{}, errors.New(result.Error)
			}
			if result.RetryLower {
				continue
			}
			return ResolveResult{}, errors.New("音源解析失败，请检查服务状态或兼容性")
		}
		if result.URL == "" || result.Quality != quality {
			return ResolveResult{}, errors.New("音源返回无效解析结果")
		}
		m.mu.Lock()
		current, readErr := m.store.Source(ctx, active.ID)
		valid := readErr == nil && current.Active && current.CheckStatus == "ready" && m.scriptRevision == scriptRevision && (!foreground || m.selectionRevision == revision)
		m.mu.Unlock()
		if !valid {
			return ResolveResult{}, errors.New("音源选择已变化，请重新解析")
		}
		return ResolveResult{SongID: info.SongMID, SourceID: active.ID, RequestedQuality: preferred, Quality: quality, Downgraded: preferred != quality, URL: result.URL}, nil
	}
	return ResolveResult{}, errors.New("当前音源未能解析该歌曲的普通音质")
}
