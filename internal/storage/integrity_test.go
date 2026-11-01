package storage

import (
	"context"
	"testing"
)

func TestCorruptStateReturnsError(t *testing.T) {
	s := testStore(t)
	if _, err := s.db.Exec(`INSERT INTO state(key,payload) VALUES('settings','not json')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Settings(context.Background()); err == nil {
		t.Fatal("corrupt state silently replaced by defaults")
	}
}

func TestFailedMigrationRollsBack(t *testing.T) {
	s := testStore(t)
	// Simulate a legacy/conflicting schema: migration must not leave behind its
	// first table or advance user_version when creating the second table fails.
	for _, statement := range []string{`DROP TABLE state`, `PRAGMA user_version=0`} {
		if _, err := s.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.initialize(); err == nil {
		t.Fatal("expected schema conflict")
	}
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 0 {
		t.Fatalf("version %d: %v", version, err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='state'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial migration: count=%d error=%v", count, err)
	}
}
