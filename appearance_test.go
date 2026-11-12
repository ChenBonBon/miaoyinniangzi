package main

import (
	"miaoyinniangzi/internal/storage"
	"testing"
)

func TestAppearanceBridge(t *testing.T) {
	unavailable := &App{}
	if _, err := unavailable.GetAppearance(); err == nil {
		t.Fatal("unavailable read accepted")
	}
	if unavailable.SaveAppearance("dark") == nil {
		t.Fatal("unavailable write accepted")
	}
	store, err := storage.Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &App{store: store}
	if app.SaveAppearance("light") != nil || app.SaveAppearance("invalid") == nil {
		t.Fatal("appearance validation failed")
	}
	if value, err := app.GetAppearance(); err != nil || value != "light" {
		t.Fatal("bridge did not preserve confirmed mode")
	}
}
