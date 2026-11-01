package main

import (
	"errors"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"miaoyinniangzi/internal/sources"
	"miaoyinniangzi/internal/storage"
)

func (a *App) GetSources() ([]storage.SourceInfo, error) {
	a.mu.RLock()
	ctx := a.ctx
	a.mu.RUnlock()
	if ctx == nil {
		return nil, errors.New("请在桌面端管理音源")
	}
	return a.sources.List(ctx)
}

// A canceled picker returns null. The frontend never supplies an import path.
func (a *App) ImportSource() (*sources.ImportResult, error) {
	a.mu.Lock()
	if a.ctx == nil {
		a.mu.Unlock()
		return nil, errors.New("请在桌面端导入音源")
	}
	if a.sourcePicking {
		a.mu.Unlock()
		return nil, errors.New("音源导入正在进行")
	}
	a.sourcePicking = true
	ctx := a.ctx
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.sourcePicking = false; a.mu.Unlock() }()
	path, err := runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{Title: "导入洛雪音源脚本", Filters: []runtime.FileFilter{{DisplayName: "JavaScript 音源脚本", Pattern: "*.js"}}})
	if err != nil {
		return nil, errors.New("无法打开文件选择器")
	}
	if path == "" {
		return nil, nil
	}
	result, err := a.sources.ImportAndValidate(ctx, path)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *App) DeleteSource(id string) (bool, error) {
	a.mu.RLock()
	ctx := a.ctx
	a.mu.RUnlock()
	if ctx == nil {
		return false, errors.New("请在桌面端管理音源")
	}
	// Confirmation belongs to the application UI. The backend still validates
	// existence and performs the deletion atomically in the source manager.
	current, _ := a.store.Source(ctx, id)
	if current.Active {
		a.sourcePlaybackChanged()
	}
	if err := a.sources.Delete(ctx, id); err != nil {
		return false, err
	}
	return true, nil
}
