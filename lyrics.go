package main

import (
	"context"
	"errors"
	"miaoyinniangzi/internal/music"
	"time"
)

func (a *App) GetLyrics(songMID, requestID string) (music.Lyrics, error) {
	if !validSearchID(requestID) {
		return music.Lyrics{}, errors.New("歌词请求无效")
	}
	a.mu.Lock()
	if a.ctx == nil || a.lyrics == nil {
		a.mu.Unlock()
		return music.Lyrics{}, errors.New("请在桌面应用中读取歌词")
	}
	if a.lyricsCancel != nil {
		a.lyricsCancel()
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	a.lyricsSequence++
	sequence := a.lyricsSequence
	a.lyricsID = requestID
	a.lyricsCancel = cancel
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.lyricsSequence == sequence {
			a.lyricsCancel = nil
			a.lyricsID = ""
		}
	}()
	result, err := a.lyrics.Lyrics(ctx, songMID)
	if err != nil {
		return music.Lyrics{}, errors.New("歌词暂时无法加载，请重试")
	}
	return result, nil
}
func (a *App) CancelLyrics(requestID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if requestID == "" || a.lyricsID != requestID || a.lyricsCancel == nil {
		return false
	}
	a.lyricsCancel()
	return true
}
