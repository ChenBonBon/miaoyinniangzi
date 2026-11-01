package storage

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestActiveSourceSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	downloads := filepath.Join(dir, "Downloads")
	ctx := context.Background()
	s, err := Open(dir, downloads)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, _, err := s.InsertSource(ctx, SourceInfo{ID: "a", Name: "test"}, strings.Repeat("a", 64), bytes.Repeat([]byte{1}, 64)); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordSourceCheck(ctx, "a", "ready", "", "engine-v1", []string{"128k"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ActivateSource(ctx, "a", "engine-v1"); err != nil {
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
	v, err := reopened.Source(ctx, "a")
	if err != nil || !v.Active || v.CheckStatus != "ready" || len(v.Qualities) != 1 {
		t.Fatalf("lost state: %+v %v", v, err)
	}
	if err := reopened.DisableSources(ctx); err != nil {
		t.Fatal(err)
	}
	v, err = reopened.Source(ctx, "a")
	if err != nil || v.Active {
		t.Fatal("failed to disable persisted source")
	}
}
