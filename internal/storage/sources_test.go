package storage

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
)

func TestMigrateV1PreservesSettings(t *testing.T) {
	dir := t.TempDir()
	downloads := filepath.Join(dir, "Downloads")
	s, err := Open(dir, downloads)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	value := Settings{downloads, "flac", "320k", .4}
	if err := s.SaveSettings(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{`DROP TABLE sources`, `DROP TABLE search_history`, `PRAGMA user_version=1`} {
		if _, err := s.db.Exec(sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir, downloads)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.Settings(context.Background())
	if err != nil || got != value {
		t.Fatalf("lost settings: %+v %v", got, err)
	}
	entries, err := reopened.Sources(context.Background())
	if err != nil || len(entries) != 0 {
		t.Fatal("source migration failed")
	}
	var version int
	if err := reopened.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 4 {
		t.Fatalf("version %d %v", version, err)
	}
}
func TestSourceWriteFailureLeavesNoPartialCopy(t *testing.T) {
	s := testStore(t)
	if _, err := s.db.Exec(`CREATE TRIGGER reject_source BEFORE INSERT ON sources BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, _, err := s.InsertSource(context.Background(), SourceInfo{ID: "id", Name: "name"}, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", bytes.Repeat([]byte{1}, 64))
	if err == nil {
		t.Fatal("expected write failure")
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sources`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial source: %d %v", count, err)
	}
}
