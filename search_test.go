package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"miaoyinniangzi/internal/music"
)

type searchFunc func(context.Context, string, int) (music.SearchResult, error)

func (f searchFunc) Search(ctx context.Context, query string, page int) (music.SearchResult, error) {
	return f(ctx, query, page)
}

func TestNewSearchCancelsOldAndStaleCancellationCannotCancelNew(t *testing.T) {
	started := make(chan string, 2)
	app := &App{ctx: context.Background(), music: searchFunc(func(ctx context.Context, query string, page int) (music.SearchResult, error) {
		started <- query
		<-ctx.Done()
		return music.SearchResult{}, ctx.Err()
	})}
	first := make(chan error, 1)
	go func() { _, err := app.SearchSongs("first", 1, "request-first"); first <- err }()
	<-started
	second := make(chan error, 1)
	go func() { _, err := app.SearchSongs("second", 1, "request-second"); second <- err }()
	<-started
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("old search not canceled: %v", err)
	}
	if app.CancelSongSearch("request-first") {
		t.Fatal("old request canceled current search")
	}
	if !app.CancelSongSearch("request-second") {
		t.Fatal("current search was lost")
	}
	if err := <-second; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel failed: %v", err)
	}
}
func TestSearchSequenceProtectsReusedRequestIDs(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	app := &App{ctx: context.Background(), music: searchFunc(func(ctx context.Context, query string, page int) (music.SearchResult, error) {
		started <- query
		<-ctx.Done()
		if query == "first" {
			<-release
		}
		return music.SearchResult{}, ctx.Err()
	})}
	first := make(chan error, 1)
	go func() { _, err := app.SearchSongs("first", 1, "same"); first <- err }()
	<-started
	second := make(chan error, 1)
	go func() { _, err := app.SearchSongs("second", 1, "same"); second <- err }()
	<-started
	close(release)
	<-first
	if !app.CancelSongSearch("same") {
		t.Fatal("older completion cleared the new cancellation handle")
	}
	<-second
}
func TestSearchCompletesWithoutAnEnabledSource(t *testing.T) {
	app := &App{ctx: context.Background(), music: searchFunc(func(ctx context.Context, query string, page int) (music.SearchResult, error) {
		return music.SearchResult{Query: query, Page: page, PageSize: 20, Songs: []music.Song{{ID: "Song1", Source: "tx", Title: "歌曲"}}}, nil
	})}
	result, err := app.SearchSongs("歌曲", 1, "request-1")
	if err != nil || len(result.Songs) != 1 {
		t.Fatalf("search failed: %+v %v", result, err)
	}
	if app.CancelSongSearch("request-1") {
		t.Fatal("completed search left cancellation state")
	}
}
func TestInvalidSearchRequestNeverReachesClient(t *testing.T) {
	var calls atomic.Int32
	app := &App{ctx: context.Background(), music: searchFunc(func(context.Context, string, int) (music.SearchResult, error) {
		calls.Add(1)
		return music.SearchResult{}, nil
	})}
	for _, id := range []string{"", "bad id", "../path"} {
		if _, err := app.SearchSongs("歌曲", 1, id); err == nil {
			t.Fatal("accepted invalid request ID")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request reached client")
	}
	if _, err := (&App{}).SearchSongs("歌曲", 1, "valid-id"); err == nil {
		t.Fatal("accepted search without desktop startup")
	}
}
