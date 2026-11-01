package main

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"miaoyinniangzi/internal/music"
	"miaoyinniangzi/internal/sourceruntime"
	"miaoyinniangzi/internal/sources"
)

func resolveSongInfo(song music.Song) (sourceruntime.ResolveInput, error) {
	validID := func(id string, required bool) bool {
		if id == "" {
			return !required
		}
		if len(id) > 64 {
			return false
		}
		for _, c := range id {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
				return false
			}
		}
		return true
	}
	if song.Source != "tx" || !validID(song.ID, true) || !validID(song.MediaMid, false) || len(song.SongID) > 20 || len(song.Title) > 8192 || len(song.Artist) > 8192 || len(song.Album) > 8192 {
		return sourceruntime.ResolveInput{}, errors.New("仅支持有效的 QQ 音乐歌曲")
	}
	for _, c := range song.SongID {
		if c < '0' || c > '9' {
			return sourceruntime.ResolveInput{}, errors.New("歌曲标识无效")
		}
	}
	clean := func(value string, limit int) string {
		value = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, value)
		runes := []rune(value)
		if len(runes) > limit {
			runes = runes[:limit]
		}
		return strings.TrimSpace(string(runes))
	}
	return sourceruntime.ResolveInput{SongMID: song.ID, SongID: song.SongID, MediaMID: song.MediaMid, Name: clean(song.Title, 100), Singer: clean(song.Artist, 256), Album: clean(song.Album, 256)}, nil
}

// Explicit resolution only: no playback, URL fetch or download is started.
func (a *App) ResolveSong(song music.Song, requestID string) (sources.ResolveResult, error) {
	info, err := resolveSongInfo(song)
	if err != nil {
		return sources.ResolveResult{}, err
	}
	if !validSearchID(requestID) {
		return sources.ResolveResult{}, errors.New("解析请求无效")
	}
	a.mu.Lock()
	if a.ctx == nil || a.sources == nil || a.store == nil {
		a.mu.Unlock()
		return sources.ResolveResult{}, errors.New("请在 macOS 桌面应用中解析音源")
	}
	if a.resolveCancel != nil {
		a.resolveCancel()
	}
	ctx, cancel := context.WithTimeout(a.ctx, 25*time.Second)
	a.resolveSequence++
	sequence := a.resolveSequence
	a.resolveCancel = cancel
	a.resolveID = requestID
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.resolveSequence == sequence {
			a.resolveCancel = nil
			a.resolveID = ""
		}
	}()
	settings, err := a.store.Settings(ctx)
	if err != nil {
		return sources.ResolveResult{}, errors.New("无法读取播放音质设置")
	}
	return a.sources.Resolve(ctx, info, settings.PlaybackQuality)
}
func (a *App) CancelSongResolution(requestID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.resolveID != requestID || a.resolveCancel == nil {
		return false
	}
	a.resolveCancel()
	return true
}
