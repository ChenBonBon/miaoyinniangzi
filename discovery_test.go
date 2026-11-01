package main

import (
	"context"
	"testing"
	"time"

	"miaoyinniangzi/internal/music"
)

type fakeDiscovery struct {
	catalog    func(context.Context, string) (music.DiscoveryCatalog, error)
	collection func(context.Context, string, int, string) (music.DiscoveryCollection, error)
}

func (f fakeDiscovery) DiscoveryCatalog(ctx context.Context, kind string) (music.DiscoveryCatalog, error) {
	return f.catalog(ctx, kind)
}
func (f fakeDiscovery) DiscoveryCollection(ctx context.Context, kind string, id int, period string) (music.DiscoveryCollection, error) {
	return f.collection(ctx, kind, id, period)
}
func TestDiscoveryCancellationIsIndependentAndLateCleanupIsSafe(t *testing.T) {
	entered := make(chan context.Context, 2)
	release := make(chan struct{})
	app := &App{ctx: context.Background(), discovery: fakeDiscovery{catalog: func(ctx context.Context, kind string) (music.DiscoveryCatalog, error) {
		entered <- ctx
		<-release
		return music.DiscoveryCatalog{Kind: kind, Entries: []music.DiscoveryEntry{}}, nil
	}}}
	searchCtx, searchCancel := context.WithCancel(context.Background())
	defer searchCancel()
	app.searchCancel = searchCancel
	firstDone := make(chan struct{})
	go func() { _, _ = app.GetDiscoveryCatalog("chart", "old"); close(firstDone) }()
	first := <-entered
	secondDone := make(chan struct{})
	go func() { _, _ = app.GetDiscoveryCatalog("new", "new"); close(secondDone) }()
	second := <-entered
	if first.Err() == nil || searchCtx.Err() != nil {
		t.Fatal("wrong cancellation channel")
	}
	if app.CancelDiscovery("old") {
		t.Fatal("stale cancel affected current discovery")
	}
	if second.Err() != nil {
		t.Fatal("current request unexpectedly canceled")
	}
	if !app.CancelDiscovery("new") || second.Err() == nil {
		t.Fatal("current cancel failed")
	}
	close(release)
	for _, done := range []chan struct{}{firstDone, secondDone} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("discovery request leaked")
		}
	}
}
func TestInvalidDiscoveryParametersDoNotStartRequest(t *testing.T) {
	app := &App{}
	if _, err := app.GetDiscoveryCatalog("bad", "request"); err == nil {
		t.Fatal("invalid kind accepted")
	}
	if _, err := app.GetDiscoveryCollection("new", 10, "", "request"); err == nil {
		t.Fatal("invalid category accepted")
	}
	if _, err := app.GetDiscoveryCatalog("chart", "request"); err == nil {
		t.Fatal("desktop guard missing")
	}
}
