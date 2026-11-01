package main

import (
	"context"
	"miaoyinniangzi/internal/music"
	"testing"
	"time"
)

type lyricCall struct {
	ctx     context.Context
	release chan struct{}
}
type testLyricReader struct{ calls chan lyricCall }

func (r *testLyricReader) Lyrics(ctx context.Context, id string) (music.Lyrics, error) {
	call := lyricCall{ctx: ctx, release: make(chan struct{})}
	r.calls <- call
	select {
	case <-ctx.Done():
		return music.Lyrics{}, ctx.Err()
	case <-call.release:
		return music.Lyrics{SongID: id, Lines: []music.LyricLine{}}, nil
	}
}
func TestLyricsCancellationDoesNotTouchSearchOrClearNewerRequest(t *testing.T) {
	searchCtx, searchCancel := context.WithCancel(context.Background())
	defer searchCancel()
	reader := &testLyricReader{calls: make(chan lyricCall, 2)}
	app := &App{ctx: context.Background(), lyrics: reader, searchCancel: searchCancel}
	done1 := make(chan struct{})
	go func() { defer close(done1); _, _ = app.GetLyrics("Song1", "lyrics-1") }()
	first := <-reader.calls
	done2 := make(chan struct{})
	go func() { defer close(done2); _, _ = app.GetLyrics("Song2", "lyrics-2") }()
	second := <-reader.calls
	select {
	case <-first.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("old lyric request not canceled")
	}
	<-done1
	if app.CancelLyrics("lyrics-1") {
		t.Fatal("stale cancel accepted")
	}
	if !app.CancelLyrics("lyrics-2") {
		t.Fatal("new lyric request cleared by old completion")
	}
	<-done2
	if searchCtx.Err() != nil {
		t.Fatal("lyric cancellation canceled search")
	}
	close(second.release)
}
