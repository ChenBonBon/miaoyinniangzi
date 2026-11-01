package main

import (
	"context"
	"errors"
	"testing"

	"miaoyinniangzi/internal/music"
)

type suggestFunc func(context.Context, string) ([]music.Suggestion, error)

func (f suggestFunc) Suggest(ctx context.Context, query string) ([]music.Suggestion, error) {
	return f(ctx, query)
}
func TestSuggestionsCancellationDoesNotCancelSubmittedSearch(t *testing.T) {
	searchStarted := make(chan struct{})
	suggestStarted := make(chan struct{}, 2)
	app := &App{ctx: context.Background(), music: searchFunc(func(ctx context.Context, _ string, _ int) (music.SearchResult, error) {
		close(searchStarted)
		<-ctx.Done()
		return music.SearchResult{}, ctx.Err()
	}), suggestions: suggestFunc(func(ctx context.Context, _ string) ([]music.Suggestion, error) {
		suggestStarted <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	})}
	searchDone := make(chan error, 1)
	go func() { _, err := app.SearchSongs("歌曲", 1, "search-1"); searchDone <- err }()
	<-searchStarted
	first := make(chan error, 1)
	go func() { _, err := app.SuggestSongs("第一首", "suggest-1"); first <- err }()
	<-suggestStarted
	second := make(chan error, 1)
	go func() { _, err := app.SuggestSongs("第二首", "suggest-2"); second <- err }()
	<-suggestStarted
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatal("old suggestions were not canceled")
	}
	if app.CancelSongSuggestions("suggest-1") {
		t.Fatal("stale suggestion cancellation reached current request")
	}
	if !app.CancelSongSuggestions("suggest-2") {
		t.Fatal("current suggestions cannot be canceled")
	}
	if err := <-second; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case <-searchDone:
		t.Fatal("suggestions canceled the submitted search")
	default:
	}
	if !app.CancelSongSearch("search-1") {
		t.Fatal("search cancellation handle was lost")
	}
	<-searchDone
}
func TestSuggestionSequenceProtectsReusedIDs(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	app := &App{ctx: context.Background(), suggestions: suggestFunc(func(ctx context.Context, q string) ([]music.Suggestion, error) {
		started <- q
		<-ctx.Done()
		if q == "first" {
			<-release
		}
		return nil, ctx.Err()
	})}
	first := make(chan struct{})
	go func() { app.SuggestSongs("first", "same"); close(first) }()
	<-started
	second := make(chan struct{})
	go func() { app.SuggestSongs("second", "same"); close(second) }()
	<-started
	close(release)
	<-first
	if !app.CancelSongSuggestions("same") {
		t.Fatal("old response cleared the newer handle")
	}
	<-second
}
func TestSuggestionBridgeRequiresDesktopAndValidID(t *testing.T) {
	if _, err := (&App{}).SuggestSongs("歌曲", "request-1"); err == nil {
		t.Fatal("suggestions fabricated outside desktop")
	}
	app := &App{ctx: context.Background(), suggestions: suggestFunc(func(context.Context, string) ([]music.Suggestion, error) {
		t.Fatal("invalid request reached service")
		return nil, nil
	})}
	if _, err := app.SuggestSongs("歌曲", "invalid/id"); err == nil {
		t.Fatal("invalid identifier accepted")
	}
}
