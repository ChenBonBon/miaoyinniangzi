package storage

import (
	"context"
	"math"
	"miaoyinniangzi/internal/privatefs"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), filepath.Join(t.TempDir(), "Downloads"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func song(id string) Song { return Song{ID: id, Source: "tx", Title: "歌曲 " + id, Artist: "歌手"} }
func TestDefaults(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	v, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v.DownloadQuality != "highest" || v.PlaybackQuality != "highest" || v.Volume != 1 || !filepath.IsAbs(v.DownloadDirectory) {
		t.Fatalf("unexpected defaults: %+v", v)
	}
	p, err := s.Playback(ctx)
	if err != nil || p.Index != -1 || p.Queue == nil || p.Mode != "sequential" {
		t.Fatalf("playback: %+v %v", p, err)
	}
	favorites, err := s.Favorites(ctx)
	if err != nil || favorites == nil || len(favorites) != 0 {
		t.Fatalf("favorites: %+v %v", favorites, err)
	}
}
func TestReopenPreservesAllState(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	downloads := filepath.Join(t.TempDir(), "Downloads")
	s, err := Open(dir, downloads)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	settings := Settings{downloads, "320k", "flac", .5}
	playback := Playback{Queue: []Song{song("a"), song("b")}, Index: 1, Position: 35.5, Mode: "shuffle"}
	if err := s.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePlayback(ctx, playback); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "a"} {
		if err := s.AddFavorite(ctx, song(id)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir, downloads)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	gotSettings, err := s.Settings(ctx)
	if err != nil || gotSettings != settings {
		t.Fatalf("settings: %+v %v", gotSettings, err)
	}
	gotPlayback, err := s.Playback(ctx)
	if err != nil || !reflect.DeepEqual(gotPlayback, playback) {
		t.Fatalf("playback: %+v %v", gotPlayback, err)
	}
	favorites, err := s.Favorites(ctx)
	if err != nil || !reflect.DeepEqual(favorites, []Song{song("b"), song("a")}) {
		t.Fatalf("favorites: %+v %v", favorites, err)
	}
	if err := s.RemoveFavorite(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	favorites, err = s.Favorites(ctx)
	if err != nil || len(favorites) != 1 || favorites[0].ID != "a" {
		t.Fatalf("remove: %+v %v", favorites, err)
	}
	if err := privatefs.Check(filepath.Join(dir, "miaoyin.sqlite3"), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestRejectInvalidStateWithoutOverwriting(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	defaults, _ := s.Settings(ctx)
	for _, change := range []func(*Settings){func(v *Settings) { v.Volume = math.NaN() }, func(v *Settings) { v.Volume = 2 }, func(v *Settings) { v.DownloadDirectory = "relative" }, func(v *Settings) { v.DownloadQuality = "atmos" }} {
		v := defaults
		change(&v)
		if err := s.SaveSettings(ctx, v); err == nil {
			t.Fatalf("accepted %+v", v)
		}
	}
	for _, p := range []Playback{{Queue: nil, Index: 0, Mode: "sequential"}, {Queue: []Song{song("a")}, Index: 0, Position: -1, Mode: "sequential"}, {Queue: []Song{{ID: "a", Source: "wy", Title: "wrong"}}, Index: 0, Mode: "sequential"}, {Index: -1, Mode: "unknown"}, {Index: -1, Position: 1, Mode: "sequential"}} {
		if err := s.SavePlayback(ctx, p); err == nil {
			t.Fatalf("accepted %+v", p)
		}
	}
	if err := s.AddFavorite(ctx, Song{ID: "a", Title: "wrong", Source: "kg"}); err == nil {
		t.Fatal("accepted non-QQ song")
	}
	got, _ := s.Settings(ctx)
	if got != defaults {
		t.Fatalf("invalid settings overwrote defaults: %+v", got)
	}
}
func TestConcurrentFavorites(t *testing.T) {
	s := testStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.AddFavorite(context.Background(), song("same")); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	favorites, err := s.Favorites(context.Background())
	if err != nil || len(favorites) != 1 {
		t.Fatalf("favorites: %+v %v", favorites, err)
	}
}
func TestNewerDatabaseIsNotDowngraded(t *testing.T) {
	s := testStore(t)
	if _, err := s.db.Exec("PRAGMA user_version=99"); err != nil {
		t.Fatal(err)
	}
	if err := s.initialize(); err == nil {
		t.Fatal("accepted future schema")
	}
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 99 {
		t.Fatalf("version %d: %v", version, err)
	}
}
func TestCanceledContext(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.AddFavorite(ctx, song("a")); err == nil {
		t.Fatal("ignored canceled context")
	}
}
