package storage

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestSingleActiveSourceAndRollback(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, id := range []string{"a", "b"} {
		if _, _, err := s.InsertSource(ctx, SourceInfo{ID: id, Name: id}, strings.Repeat(id, 64), bytes.Repeat([]byte{1}, 64)); err != nil {
			t.Fatal(err)
		}
		if err := s.RecordSourceCheck(ctx, id, "ready", "", "test-engine", []string{"flac"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ActivateSource(ctx, "a", "test-engine"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"missing", "b"} {
		engine := "test-engine"
		if id == "b" {
			engine = "wrong-engine"
		}
		if err := s.ActivateSource(ctx, id, engine); err == nil {
			t.Fatal("accepted invalid selection")
		}
		entry, err := s.Source(ctx, "a")
		if err != nil || !entry.Active {
			t.Fatal("lost old selection on failed switch")
		}
	}
	if err := s.ActivateSource(ctx, "b", "test-engine"); err != nil {
		t.Fatal(err)
	}
	entries, err := s.Sources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	for _, entry := range entries {
		if entry.Active {
			active++
			if entry.ID != "b" {
				t.Fatal("wrong active source")
			}
		}
	}
	if active != 1 {
		t.Fatal("not exactly one active source")
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE sources SET active=1 WHERE id='a'`); err == nil {
		t.Fatal("database allowed two active sources")
	}
	if err := s.RecordSourceCheck(ctx, "b", "failed", "failure", "test-engine", nil); err != nil {
		t.Fatal(err)
	}
	entry, err := s.Source(ctx, "b")
	if err != nil || entry.Active {
		t.Fatal("failed recheck left active source enabled")
	}
	if err := s.ActivateSource(ctx, "b", "test-engine"); err == nil {
		t.Fatal("activated failed source")
	}
	if err := s.InvalidateSourceChecks(ctx, "next-engine"); err != nil {
		t.Fatal(err)
	}
	entry, err = s.Source(ctx, "a")
	if err != nil || entry.CheckStatus != "unchecked" || len(entry.Qualities) != 0 {
		t.Fatal("did not invalidate cached checks")
	}
}
