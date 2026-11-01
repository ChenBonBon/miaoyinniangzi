package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMusicCreditIdentifiersSurviveFavoritesAndPausedQueueReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	downloads := filepath.Join(t.TempDir(), "Downloads")
	store, err := Open(dir, downloads)
	if err != nil {
		t.Fatal(err)
	}
	value := song("Credit1")
	value.Artists = []Artist{{ID: "Artist1", Name: "artist"}}
	value.AlbumMid = "Album1"
	value.Duration = 181.25
	if err = store.AddFavorite(ctx, value); err != nil {
		t.Fatal(err)
	}
	if err = store.SavePlayback(ctx, Playback{Queue: []Song{value}, Index: 0, Mode: "sequential"}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(dir, downloads)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	favorites, err := store.Favorites(ctx)
	if err != nil || len(favorites) != 1 || favorites[0].AlbumMid != "Album1" || len(favorites[0].Artists) != 1 || favorites[0].Artists[0].ID != "Artist1" {
		t.Fatal("favorite credits lost")
	}
	playback, err := store.Playback(ctx)
	if err != nil || playback.Queue[0].AlbumMid != "Album1" || playback.Queue[0].Artists[0].ID != "Artist1" || playback.Queue[0].Duration != 181.25 {
		t.Fatal("paused queue credits lost")
	}
}
