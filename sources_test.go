package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"miaoyinniangzi/internal/sources"
	"miaoyinniangzi/internal/storage"
)

func TestDeleteSourceNeedsNoNativeConfirmationAndRejectsMissingRecord(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.Open(dir, filepath.Join(dir, "Downloads"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _, err = store.InsertSource(ctx, storage.SourceInfo{ID: "source-id", Name: "test"}, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", bytes.Repeat([]byte{1}, 64))
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: store, sources: sources.New(store, nil), ctx: ctx}
	// A plain context has no Wails frontend. Successful deletion proves that the
	// backend no longer attempts to open a native confirmation window.
	deleted, err := app.DeleteSource("source-id")
	if err != nil || !deleted {
		t.Fatalf("delete failed: %v", err)
	}
	if _, err := store.SourceCiphertext(ctx, "source-id"); err == nil {
		t.Fatal("copy still exists")
	}
	if deleted, err := app.DeleteSource("source-id"); err == nil || deleted {
		t.Fatal("missing record reported as deleted")
	}
}
