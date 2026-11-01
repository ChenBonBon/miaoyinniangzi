package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"miaoyinniangzi/internal/storage"
)

func TestFavoritesNewestFirstIdempotentAndIndependentOfPlaybackAndFiles(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	downloads := t.TempDir()
	store, err := storage.Open(directory, downloads)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: store}
	a := storage.Song{ID: "First1", Source: "tx", Title: "First", Duration: 123.25}
	b := storage.Song{ID: "Second1", Source: "tx", Title: "Second"}
	snapshot := storage.Playback{Queue: []storage.Song{a, b}, Index: 0, Position: 12.5, Mode: "sequential"}
	if err = store.SavePlayback(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(downloads, "existing.flac")
	if err = os.WriteFile(path, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, song := range []storage.Song{a, b, a} {
		if err = app.AddFavorite(song); err != nil {
			t.Fatal(err)
		}
	}
	favorites, err := app.GetFavorites()
	if err != nil || len(favorites) != 2 || favorites[0].ID != b.ID {
		t.Fatal("duplicate add changed newest-first order")
	}
	if err = app.RemoveFavorite(a.ID); err != nil {
		t.Fatal(err)
	}
	if err = app.AddFavorite(a); err != nil {
		t.Fatal(err)
	}
	favorites, _ = app.GetFavorites()
	if favorites[0].ID != a.ID || favorites[0].Duration != 123.25 {
		t.Fatal("re-add or fractional metadata lost")
	}
	restored, _ := store.Playback(ctx)
	if !reflect.DeepEqual(snapshot, restored) {
		t.Fatal("favorite operation changed playback")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "untouched" {
		t.Fatal("favorite operation touched download")
	}
	store.Close()
	store, err = storage.Open(directory, downloads)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app.store = store
	favorites, err = app.GetFavorites()
	if err != nil || len(favorites) != 2 || favorites[0].ID != a.ID {
		t.Fatal("favorites not preserved on reopen")
	}
}
func TestFavoriteBoundaryFailsClosedWithoutStore(t *testing.T) {
	app := &App{}
	if _, err := app.GetFavorites(); err == nil {
		t.Fatal("missing store reported success")
	}
	if app.AddFavorite(storage.Song{}) == nil || app.RemoveFavorite("A") == nil {
		t.Fatal("missing store accepted write")
	}
	store, err := storage.Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app.store = store
	for _, song := range []storage.Song{{ID: "bad/id", Source: "tx", Title: "Song"}, {ID: "Valid1", Source: "wy", Title: "Song"}} {
		if app.AddFavorite(song) == nil {
			t.Fatal("invalid favorite accepted")
		}
	}
	if app.RemoveFavorite("' OR 1=1") == nil {
		t.Fatal("invalid removal ID accepted")
	}
}
