package main

import (
	"context"
	"errors"
	"miaoyinniangzi/internal/music"
	"time"
)

func (a *App) GetSongCredits(id string) (music.SongCredits, error) {
	if !music.ValidDetailSelection("artist", id, 1) {
		return music.SongCredits{}, errors.New("歌曲标识无效")
	}
	a.mu.RLock()
	parent := a.ctx
	reader, ok := a.details.(music.SongCreditReader)
	a.mu.RUnlock()
	if parent == nil || !ok {
		return music.SongCredits{}, errors.New("歌曲信息服务不可用")
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	result, err := reader.SongCredits(ctx, id)
	if err != nil {
		return music.SongCredits{}, errors.New("无法读取歌曲的歌手或专辑信息，请稍后重试")
	}
	return result, nil
}
