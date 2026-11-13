package main

import (
	"context"
	"errors"
	"fmt"
	"miaoyinniangzi/internal/music"
	"miaoyinniangzi/internal/storage"
	"os"
	"sync"
	"testing"
	"time"
)

type similarFunc func(context.Context, string) ([]music.Song, error)

func (f similarFunc) SimilarSongs(ctx context.Context, id string) ([]music.Song, error) {
	return f(ctx, id)
}
func forYouApp(t *testing.T, reader similarFunc) *App {
	t.Helper()
	store, err := storage.Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return &App{store: store, ctx: context.Background(), similar: reader}
}
func seedFavorite(t *testing.T, a *App, id string, number string, artist string) {
	t.Helper()
	if a.store.AddFavorite(context.Background(), storage.Song{ID: id, Source: "tx", SongID: number, Title: "Seed", Artist: artist, Artists: []storage.Artist{{ID: artist, Name: "Artist"}}}) != nil {
		t.Fatal("favorite fixture failed")
	}
}
func recommendationSong(id string, number string) music.Song {
	return music.Song{ID: id, SongID: number, Source: "tx", Title: "Candidate", Artist: "Artist", Artists: []music.Artist{}, Album: "Album", AlbumMid: "AlbumMid", AvailableQualities: []string{"128k"}}
}
func TestLiveForYouWithIsolatedPublicSeed(t *testing.T) {
	if os.Getenv("MIAOYIN_LIVE_QQ_FOR_YOU") != "1" {
		t.Skip("explicit isolated public metadata opt-in")
	}
	app := forYouApp(t, similarFunc(music.NewQQClient().SimilarSongs))
	seedFavorite(t, app, "PublicSeedFixture", "97773", "PublicArtistFixture")
	result, err := app.GetForYou("live-public-seed")
	if err != nil || len(result.Songs) == 0 {
		t.Fatal("live candidate pipeline failed")
	}
	t.Logf("live favorite-seeded candidates=%d; isolated store and no media resolution", len(result.Songs))
}

func TestForYouUsesFavoritesFiltersDuplicatesAndReusesBoundedCache(t *testing.T) {
	calls := 0
	app := forYouApp(t, func(ctx context.Context, id string) ([]music.Song, error) {
		calls++
		return []music.Song{recommendationSong("SeedOne", "101"), recommendationSong("Candidate", "501"), recommendationSong("AlternateMid", "501")}, nil
	})
	seedFavorite(t, app, "SeedOne", "101", "ArtistA")
	seedFavorite(t, app, "SeedTwo", "102", "ArtistB")
	first, err := app.GetForYou("first")
	if err != nil || first.SeedCount != 2 || len(first.Songs) != 1 || first.Songs[0].ID != "Candidate" || calls != 2 {
		t.Fatal("candidate filtering or seeding failed")
	}
	if _, err = app.GetForYou("again"); err != nil || calls != 2 {
		t.Fatal("fresh cache generated traffic")
	}
	for id, entry := range app.forYouCache {
		entry.Saved = time.Now().Add(-2 * time.Hour)
		app.forYouCache[id] = entry
	}
	app.similar = similarFunc(func(context.Context, string) ([]music.Song, error) { return nil, music.ErrNetwork })
	stale, err := app.GetForYou("stale")
	if err != nil || len(stale.Songs) != 1 || stale.Warning == "" {
		t.Fatal("failed refresh discarded usable candidates")
	}
}
func TestForYouSkipsMissingIDsWithoutMetadataLookupAndCapsSeeds(t *testing.T) {
	var queried []string
	app := forYouApp(t, func(ctx context.Context, id string) ([]music.Song, error) {
		queried = append(queried, id)
		return []music.Song{}, nil
	})
	empty, err := app.GetForYou("empty")
	if err != nil || empty.Reason != "no_favorites" || len(queried) != 0 {
		t.Fatal("empty favorites made network requests")
	}
	seedFavorite(t, app, "OldFavorite", "", "ArtistA")
	missing, err := app.GetForYou("missing")
	if err != nil || missing.Reason != "missing_song_ids" || len(queried) != 0 {
		t.Fatal("missing IDs were resolved or misrepresented")
	}
	for i := 0; i < 9; i++ {
		seedFavorite(t, app, fmt.Sprintf("Seed%d", i), fmt.Sprint(100+i), fmt.Sprintf("Artist%d", i))
	}
	result, err := app.GetForYou("bounded")
	if err != nil || len(queried) != forYouSeedLimit || result.Skipped != 1 || result.Reason != "no_candidates" {
		t.Fatal("seed budget or empty state incorrect")
	}
}
func TestForYouCancellationAndFavoriteChangesFenceResults(t *testing.T) {
	for _, change := range []bool{false, true} {
		t.Run(fmt.Sprint(change), func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			app := forYouApp(t, func(ctx context.Context, id string) ([]music.Song, error) {
				once.Do(func() { close(started) })
				if change {
					<-release
				} else {
					<-ctx.Done()
				}
				return []music.Song{recommendationSong("Candidate", "777")}, nil
			})
			seedFavorite(t, app, "Seed", "123", "ArtistA")
			done := make(chan error, 1)
			go func() { _, err := app.GetForYou("active"); done <- err }()
			<-started
			if change {
				if app.store.RemoveFavorite(context.Background(), "Seed") != nil {
					t.Fatal("fixture remove failed")
				}
				close(release)
			} else {
				if !app.CancelForYou("active") {
					t.Fatal("cancel not accepted")
				}
			}
			if err := <-done; err == nil {
				t.Fatal("stale/canceled result committed")
			}
			if !change && len(app.forYouCache) != 0 {
				t.Fatal("canceled seed replaced cache")
			}
		})
	}
}
func TestForYouSeedCacheIsBounded(t *testing.T) {
	app := forYouApp(t, func(context.Context, string) ([]music.Song, error) { return []music.Song{}, nil })
	for i := 0; i < 25; i++ {
		seedFavorite(t, app, fmt.Sprintf("Seed%d", i), fmt.Sprint(100+i), fmt.Sprintf("Artist%d", i))
		if _, err := app.GetForYou(fmt.Sprintf("request%d", i)); err != nil {
			t.Fatal(err)
		}
		if len(app.forYouCache) > forYouCacheLimit {
			t.Fatal("unbounded candidate cache")
		}
	}
}

func TestForYouStopsAfterUpstreamRateLimit(t *testing.T) {
	calls := 0
	app := forYouApp(t, func(context.Context, string) ([]music.Song, error) { calls++; return nil, music.ErrRateLimit })
	for i := 0; i < 7; i++ {
		seedFavorite(t, app, fmt.Sprintf("Seed%d", i), fmt.Sprint(100+i), fmt.Sprintf("Artist%d", i))
	}
	if _, err := app.GetForYou("limit"); err == nil || calls != 1 {
		t.Fatal("upstream rejection caused additional requests")
	}
}

func TestForYouRejectsInvalidReaderAndDoesNotReturnUnknownData(t *testing.T) {
	app := forYouApp(t, func(context.Context, string) ([]music.Song, error) {
		return []music.Song{{ID: "bad", Source: "other"}}, nil
	})
	seedFavorite(t, app, "Seed", "123", "ArtistA")
	if _, err := app.GetForYou("invalid"); err == nil {
		t.Fatal("invalid reader accepted")
	}
	app.similar = similarFunc(func(context.Context, string) ([]music.Song, error) { return nil, errors.New("PRIVATE ERROR") })
	if _, err := app.GetForYou("redacted"); err == nil || err.Error() == "PRIVATE ERROR" {
		t.Fatal("raw reader error escaped")
	}
}
