package main

import (
	"errors"
	"miaoyinniangzi/internal/storage"
)

func (a *App) CheckSource(id string) ([]storage.SourceInfo, error) {
	a.mu.RLock()
	ctx := a.ctx
	a.mu.RUnlock()
	if ctx == nil {
		return nil, errors.New("请在桌面端校验音源")
	}
	current, _ := a.store.Source(ctx, id)
	if current.Active {
		a.sourcePlaybackChanged()
	}
	if _, err := a.sources.Check(ctx, id); err != nil {
		return nil, err
	}
	return a.sources.List(ctx)
}
func (a *App) ActivateSource(id string) ([]storage.SourceInfo, error) {
	a.mu.RLock()
	ctx := a.ctx
	a.mu.RUnlock()
	if ctx == nil {
		return nil, errors.New("请在桌面端启用音源")
	}
	a.sourcePlaybackChanged()
	if err := a.sources.Activate(ctx, id); err != nil {
		return nil, err
	}
	return a.sources.List(ctx)
}
func (a *App) DisableSource() ([]storage.SourceInfo, error) {
	a.mu.RLock()
	ctx := a.ctx
	a.mu.RUnlock()
	if ctx == nil {
		return nil, errors.New("请在桌面端禁用音源")
	}
	a.sourcePlaybackChanged()
	if err := a.sources.Disable(ctx); err != nil {
		return nil, err
	}
	return a.sources.List(ctx)
}
