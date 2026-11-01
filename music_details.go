package main

import (
	"context"
	"errors"
	"miaoyinniangzi/internal/music"
	"time"
)

func (a *App) GetMusicDetail(kind, id string, page int, requestID string) (music.MusicDetail, error) {
	if !music.ValidDetailSelection(kind, id, page) || !validSearchID(requestID) {
		return music.MusicDetail{}, errors.New("详情参数无效")
	}
	a.mu.Lock()
	if a.ctx == nil || a.details == nil {
		a.mu.Unlock()
		return music.MusicDetail{}, errors.New("详情服务不可用")
	}
	if a.detailCancel != nil {
		a.detailCancel()
	}
	ctx, cancel := context.WithTimeout(a.ctx, 35*time.Second)
	a.detailSequence++
	sequence := a.detailSequence
	a.detailCancel = cancel
	a.detailID = requestID
	reader := a.details
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.detailSequence == sequence {
			a.detailCancel = nil
			a.detailID = ""
		}
	}()
	result, err := reader.Detail(ctx, kind, id, page)
	if err == nil {
		return result, nil
	}
	switch {
	case errors.Is(err, music.ErrCanceled):
		return music.MusicDetail{}, errors.New("详情加载已取消")
	case errors.Is(err, music.ErrRestricted):
		return music.MusicDetail{}, errors.New("QQ 音乐暂时无法提供详情，请稍后重试")
	case errors.Is(err, music.ErrRateLimit):
		return music.MusicDetail{}, errors.New("请求过于频繁，请稍后重试")
	case errors.Is(err, music.ErrTimeout):
		return music.MusicDetail{}, errors.New("读取详情超时，请重试")
	default:
		return music.MusicDetail{}, errors.New("无法读取 QQ 音乐详情，请稍后重试")
	}
}
func (a *App) CancelMusicDetail(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id == "" || a.detailID != id || a.detailCancel == nil {
		return false
	}
	a.detailCancel()
	return true
}
