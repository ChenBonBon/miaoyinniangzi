package main

import (
	"context"
	"errors"
	"time"

	"miaoyinniangzi/internal/music"
)

func (a *App) beginDiscovery(id string) (context.Context, func(), music.DiscoveryReader, error) {
	if !validSearchID(id) {
		return nil, nil, nil, errors.New("推荐请求无效，请重试")
	}
	a.mu.Lock()
	if a.ctx == nil || a.discovery == nil {
		a.mu.Unlock()
		return nil, nil, nil, errors.New("请在桌面应用中读取推荐")
	}
	if a.discoveryCancel != nil {
		a.discoveryCancel()
	}
	ctx, cancel := context.WithTimeout(a.ctx, 35*time.Second)
	a.discoverySequence++
	sequence := a.discoverySequence
	a.discoveryID = id
	a.discoveryCancel = cancel
	reader := a.discovery
	a.mu.Unlock()
	cleanup := func() {
		cancel()
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.discoverySequence == sequence {
			a.discoveryID = ""
			a.discoveryCancel = nil
		}
	}
	return ctx, cleanup, reader, nil
}
func discoveryError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, music.ErrRestricted):
		return errors.New("QQ 音乐暂时无法提供推荐内容，请稍后重试")
	case errors.Is(err, music.ErrRateLimit):
		return errors.New("请求过于频繁，请稍后重试")
	case errors.Is(err, music.ErrTimeout):
		return errors.New("读取 QQ 音乐推荐超时，请重试")
	case errors.Is(err, music.ErrCanceled):
		return errors.New("推荐加载已取消")
	case errors.Is(err, music.ErrNetwork):
		return music.ErrNetwork
	default:
		return errors.New("QQ 音乐推荐返回异常数据，请稍后重试")
	}
}
func (a *App) GetDiscoveryCatalog(kind, requestID string) (music.DiscoveryCatalog, error) {
	if !music.ValidDiscoveryKind(kind) {
		return music.DiscoveryCatalog{}, errors.New("推荐类型无效")
	}
	ctx, cleanup, reader, err := a.beginDiscovery(requestID)
	if err != nil {
		return music.DiscoveryCatalog{}, err
	}
	defer cleanup()
	result, err := reader.DiscoveryCatalog(ctx, kind)
	if err == nil && result.Kind == kind && music.ValidDiscoveryCatalogValue(result) {
		a.saveDiscoverySnapshot(ctx, discoveryCacheKey(kind, 0, ""), result)
	}
	return result, discoveryError(err)
}
func (a *App) GetDiscoveryCollection(kind string, id int, period, requestID string) (music.DiscoveryCollection, error) {
	if !music.ValidDiscoverySelection(kind, id, period) {
		return music.DiscoveryCollection{}, errors.New("推荐列表参数无效")
	}
	ctx, cleanup, reader, err := a.beginDiscovery(requestID)
	if err != nil {
		return music.DiscoveryCollection{}, err
	}
	defer cleanup()
	result, err := reader.DiscoveryCollection(ctx, kind, id, period)
	if err == nil && result.Entry.Kind == kind && result.Entry.ID == id && (period == "" || result.Entry.Period == period) && music.ValidDiscoveryCollectionValue(result) {
		a.saveDiscoverySnapshot(ctx, discoveryCacheKey(kind, id, period), result)
	}
	return result, discoveryError(err)
}
func (a *App) CancelDiscovery(requestID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if requestID == "" || a.discoveryID != requestID || a.discoveryCancel == nil {
		return false
	}
	a.discoveryCancel()
	return true
}
