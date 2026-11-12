package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAppearancePersistsIndependentlyAndRejectsCorruption(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	downloads := filepath.Join(t.TempDir(), "Downloads")
	store, err := Open(dir, downloads)
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.Appearance(ctx)
	if err != nil || value != "dark" {
		t.Fatal("legacy default changed")
	}
	settings, _ := store.Settings(ctx)
	for _, mode := range []string{"light", "system", "dark", "system"} {
		if store.SaveAppearance(ctx, mode) != nil {
			t.Fatal("valid mode rejected")
		}
	}
	if store.SaveSettings(ctx, settings) != nil {
		t.Fatal("settings write failed")
	}
	if store.SaveAppearance(ctx, "unknown") == nil {
		t.Fatal("invalid mode accepted")
	}
	store.Close()
	store, err = Open(dir, downloads)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if value, err = store.Appearance(ctx); err != nil || value != "system" {
		t.Fatal("appearance lost on reopen or overwritten by unrelated settings")
	}
	for _, payload := range []string{`"invalid"`, `null`, `[]`, `1`, `broken`} {
		if _, err = store.db.ExecContext(ctx, `UPDATE state SET payload=? WHERE key='appearance_v1'`, payload); err != nil {
			t.Fatal(err)
		}
		if _, err = store.Appearance(ctx); err == nil {
			t.Fatal("corruption silently adopted")
		}
	}
}
