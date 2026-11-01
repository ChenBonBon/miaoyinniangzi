package main

import (
	"context"
	"errors"
	"time"

	"miaoyinniangzi/internal/music"
)

func validSearchID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

// SearchSongs keeps one outstanding search per app. The server sequence, not
// the client identifier, owns cleanup so reused IDs cannot clear a newer call.
func (a *App) SearchSongs(query string, page int, requestID string) (music.SearchResult, error) {
	if !validSearchID(requestID) {
		return music.SearchResult{}, errors.New("搜索请求无效，请重试")
	}
	a.mu.Lock()
	if a.ctx == nil || a.music == nil {
		a.mu.Unlock()
		return music.SearchResult{}, errors.New("请在 macOS 桌面应用中搜索 QQ 音乐")
	}
	if a.searchCancel != nil {
		a.searchCancel()
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	a.searchSequence++
	sequence := a.searchSequence
	a.searchID = requestID
	a.searchCancel = cancel
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.searchSequence == sequence {
			a.searchID = ""
			a.searchCancel = nil
		}
	}()
	return a.music.Search(ctx, query, page)
}
func (a *App) CancelSongSearch(requestID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.searchID != requestID || a.searchCancel == nil {
		return false
	}
	a.searchCancel()
	return true
}
