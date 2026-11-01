package main

import (
	"context"
	"miaoyinniangzi/internal/music"
	"testing"
	"time"
)

type detailReaderFunc func(context.Context, string, string, int) (music.MusicDetail, error)

func (f detailReaderFunc) Detail(ctx context.Context, kind, id string, page int) (music.MusicDetail, error) {
	return f(ctx, kind, id, page)
}
func TestDetailCancellationIsIndependentAndValidatesBeforeCallingReader(t *testing.T) {
	entered := make(chan struct{})
	done := make(chan struct{})
	unrelated := false
	app := &App{ctx: context.Background(), searchCancel: func() { unrelated = true }, discoveryCancel: func() { unrelated = true }, playCancel: func() { unrelated = true }, details: detailReaderFunc(func(ctx context.Context, _ string, _ string, _ int) (music.MusicDetail, error) {
		close(entered)
		<-ctx.Done()
		return music.MusicDetail{}, ctx.Err()
	})}
	if _, err := app.GetMusicDetail("artist", "../invalid", 1, "request"); err == nil {
		t.Fatal("invalid detail accepted")
	}
	go func() { defer close(done); _, _ = app.GetMusicDetail("artist", "Artist1", 1, "request") }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("detail not started")
	}
	if app.CancelMusicDetail("other") {
		t.Fatal("wrong request canceled")
	}
	if !app.CancelMusicDetail("request") {
		t.Fatal("request not canceled")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("detail not stopped")
	}
	if unrelated {
		t.Fatal("detail canceled another feature")
	}
}
