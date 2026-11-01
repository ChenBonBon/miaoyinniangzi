package storage

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchHistoryOrderDedupLimitAndRemoval(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	entries, err := s.SearchHistory(ctx)
	if err != nil || entries == nil || len(entries) != 0 {
		t.Fatal("invalid empty history")
	}
	for i := 0; i < 25; i++ {
		if _, err := s.RecordSearchHistory(ctx, fmt.Sprintf("歌曲%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	entries, err = s.SearchHistory(ctx)
	if err != nil || len(entries) != 20 || entries[0] != "歌曲24" || entries[19] != "歌曲5" {
		t.Fatalf("limit/order: %v %v", entries, err)
	}
	entries, err = s.RecordSearchHistory(ctx, " 歌曲8 ")
	if err != nil || len(entries) != 20 || entries[0] != "歌曲8" {
		t.Fatal("dedup failed")
	}
	query := `'); DROP TABLE favorites; --`
	if _, err := s.RecordSearchHistory(ctx, query); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Favorites(ctx); err != nil {
		t.Fatal("query was not parameterized")
	}
	entries, err = s.RemoveSearchHistory(ctx, query)
	if err != nil || len(entries) != 19 {
		t.Fatal("remove failed")
	}
	for _, query := range []string{"", "  ", strings.Repeat("歌", 101), "歌\x00名"} {
		if _, err := s.RecordSearchHistory(ctx, query); err == nil {
			t.Fatal("accepted invalid history")
		}
	}
	entries, err = s.ClearSearchHistory(ctx)
	if err != nil || entries == nil || len(entries) != 0 {
		t.Fatal("clear failed")
	}
}
func TestHistoryReopenAndV3MigrationPreserveOtherState(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	downloads := filepath.Join(dir, "Downloads")
	s, err := Open(dir, downloads)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	settings := Settings{downloads, "flac", "320k", .6}
	if err := s.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	cipherCopy := bytes.Repeat([]byte{7}, 64)
	if _, _, err := s.InsertSource(ctx, SourceInfo{ID: "kept-source", Name: "已有音源"}, strings.Repeat("a", 64), cipherCopy); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordSourceCheck(ctx, "kept-source", "ready", "", "engine-v1", []string{"128k"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ActivateSource(ctx, "kept-source", "engine-v1"); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`DROP TABLE search_history`, `PRAGMA user_version=3`} {
		if _, err := s.db.Exec(statement); err != nil {
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
	if got, err := s.Settings(ctx); err != nil || got != settings {
		t.Fatal("migration changed settings")
	}
	source, err := s.Source(ctx, "kept-source")
	if err != nil || !source.Active || source.CheckStatus != "ready" {
		t.Fatal("migration changed source state")
	}
	encrypted, err := s.SourceCiphertext(ctx, "kept-source")
	if err != nil || !bytes.Equal(encrypted, cipherCopy) {
		t.Fatal("migration changed encrypted copy")
	}
	if _, err := s.RecordSearchHistory(ctx, "周杰伦"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir, downloads)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	entries, err := reopened.SearchHistory(ctx)
	if err != nil || len(entries) != 1 || entries[0] != "周杰伦" {
		t.Fatalf("history was not persisted: %v %v", entries, err)
	}
}
func TestHistoryClearDoesNotLeaveTextInActiveDatabase(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, filepath.Join(dir, "Downloads"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const query = "private-history-marker-7654321"
	if _, err := s.RecordSearchHistory(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClearSearchHistory(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "miaoyin.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(query)) {
		t.Fatal("cleared query remained in active database pages")
	}
}
