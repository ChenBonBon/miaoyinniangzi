package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"miaoyinniangzi/internal/music"
	"miaoyinniangzi/internal/storage"
)

func cacheCatalog() music.DiscoveryCatalog {
	return music.DiscoveryCatalog{Kind: "chart", Entries: []music.DiscoveryEntry{{Kind: "chart", ID: 1, Title: "fixture", Period: "2026_43", Total: 1}}}
}
func TestDiscoverySnapshotsPersistAndFailedRefreshKeepsLastSuccess(t *testing.T) {
	ctx := context.Background()
	dir, download := t.TempDir(), t.TempDir()
	store, err := storage.Open(dir, download)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{ctx: ctx, store: store, discovery: fakeDiscovery{catalog: func(context.Context, string) (music.DiscoveryCatalog, error) { return cacheCatalog(), nil }}}
	if _, err = app.GetDiscoveryCatalog("chart", "first"); err != nil {
		t.Fatal(err)
	}
	before := app.GetDiscoverySnapshot("chart", 0, "")
	if before.Catalog == nil || before.SavedAt == 0 {
		t.Fatal("successful catalog not cached")
	}
	store.Close()
	store, err = storage.Open(dir, download)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app = &App{ctx: ctx, store: store, discovery: fakeDiscovery{catalog: func(context.Context, string) (music.DiscoveryCatalog, error) {
		return music.DiscoveryCatalog{}, music.ErrTimeout
	}}}
	if _, err = app.GetDiscoveryCatalog("chart", "timeout"); err == nil {
		t.Fatal("failed refresh hidden")
	}
	after := app.GetDiscoverySnapshot("chart", 0, "")
	if after.Catalog == nil || len(after.Catalog.Entries) != 1 || after.SavedAt != before.SavedAt {
		t.Fatal("failed refresh destroyed persisted success")
	}
}
func TestDiscoverySnapshotsRejectBadSelectionExpiredAndUnsafePayloads(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &App{ctx: ctx, store: store}
	for _, change := range []func(*music.DiscoveryCatalog){func(v *music.DiscoveryCatalog) { v.Kind = "new" }, func(v *music.DiscoveryCatalog) { v.Entries[0].CoverURL = "https://example.invalid/unsafe" }, func(v *music.DiscoveryCatalog) { v.Entries = append(v.Entries, v.Entries[0]) }} {
		value := cacheCatalog()
		change(&value)
		data, _ := json.Marshal(value)
		if err = store.SaveDiscoveryCache(ctx, storage.DiscoveryCacheEntry{Key: "chart:catalog", SavedAt: time.Now().UnixMilli(), Data: data}); err != nil {
			t.Fatal(err)
		}
		if app.GetDiscoverySnapshot("chart", 0, "").Catalog != nil {
			t.Fatal("invalid persisted metadata served")
		}
	}
	data, _ := json.Marshal(cacheCatalog())
	for _, stamp := range []int64{time.Now().Add(-73 * time.Hour).UnixMilli(), time.Now().Add(time.Hour).UnixMilli()} {
		if err = store.SaveDiscoveryCache(ctx, storage.DiscoveryCacheEntry{Key: "chart:catalog", SavedAt: stamp, Data: data}); err != nil {
			t.Fatal(err)
		}
		if app.GetDiscoverySnapshot("chart", 0, "").Catalog != nil {
			t.Fatal("expired/future cache served")
		}
	}
	if app.GetDiscoverySnapshot("other", 0, "").SavedAt != 0 || app.GetDiscoverySnapshot("chart", 0, "unexpected").SavedAt != 0 {
		t.Fatal("invalid cache selection accepted")
	}
}
func TestCanceledDiscoveryCannotPopulateSnapshots(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	app := &App{ctx: ctx, store: store, discovery: fakeDiscovery{catalog: func(context.Context, string) (music.DiscoveryCatalog, error) {
		close(entered)
		<-release
		return cacheCatalog(), nil
	}}}
	done := make(chan struct{})
	go func() { _, _ = app.GetDiscoveryCatalog("chart", "canceled"); close(done) }()
	<-entered
	app.CancelDiscovery("canceled")
	close(release)
	<-done
	if app.GetDiscoverySnapshot("chart", 0, "").Catalog != nil {
		t.Fatal("canceled request persisted stale metadata")
	}
}
func TestDiscoveryCacheFailureDoesNotRejectValidLiveMetadata(t *testing.T) {
	store, err := storage.Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	app := &App{ctx: context.Background(), store: store, discovery: fakeDiscovery{catalog: func(context.Context, string) (music.DiscoveryCatalog, error) { return cacheCatalog(), nil }}}
	value, err := app.GetDiscoveryCatalog("chart", "disk-unavailable")
	if err != nil || len(value.Entries) != 1 {
		t.Fatal("cache I/O failure blocked valid metadata")
	}
}
func TestDiscoveryCollectionSnapshotRequiresExactPeriod(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &App{ctx: ctx, store: store, discovery: fakeDiscovery{collection: func(context.Context, string, int, string) (music.DiscoveryCollection, error) {
		return music.DiscoveryCollection{Entry: cacheCatalog().Entries[0], Songs: []music.Song{{ID: "FixtureSong", Source: "tx", Title: "fixture"}}}, nil
	}}}
	if _, err = app.GetDiscoveryCollection("chart", 1, "2026_43", "list"); err != nil {
		t.Fatal(err)
	}
	if app.GetDiscoverySnapshot("chart", 1, "2026_43").Collection == nil || app.GetDiscoverySnapshot("chart", 1, "2026_44").Collection != nil {
		t.Fatal("period identity ignored")
	}
}
