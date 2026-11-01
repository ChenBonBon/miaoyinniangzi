package main

import (
	"context"
	"errors"
	"time"

	"miaoyinniangzi/internal/music"
)

// Typeahead must not cancel or replace an explicitly submitted full search.
func (a *App) SuggestSongs(query, requestID string) ([]music.Suggestion, error) {
	if !validSearchID(requestID) {
		return nil, errors.New("搜索提示请求无效")
	}
	a.mu.Lock()
	if a.ctx == nil || a.suggestions == nil {
		a.mu.Unlock()
		return nil, errors.New("请在 macOS 桌面应用中获取歌曲建议")
	}
	if a.suggestionCancel != nil {
		a.suggestionCancel()
	}
	ctx, cancel := context.WithTimeout(a.ctx, 4*time.Second)
	a.suggestionSequence++
	sequence := a.suggestionSequence
	a.suggestionID = requestID
	a.suggestionCancel = cancel
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.suggestionSequence == sequence {
			a.suggestionID = ""
			a.suggestionCancel = nil
		}
	}()
	return a.suggestions.Suggest(ctx, query)
}
func (a *App) CancelSongSuggestions(requestID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.suggestionID != requestID || a.suggestionCancel == nil {
		return false
	}
	a.suggestionCancel()
	return true
}
