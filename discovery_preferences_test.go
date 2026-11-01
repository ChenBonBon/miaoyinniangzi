package main

import (
	"miaoyinniangzi/internal/storage"
	"testing"
)

func TestDiscoveryPreferencesBridgeRejectsUnavailableStorageAndInvalidWrites(t *testing.T) {
	unavailable := &App{}
	if _, err := unavailable.GetDiscoveryPreferences(); err == nil {
		t.Fatal("unavailable storage accepted")
	}
	if err := unavailable.SaveDiscoveryPreferences(storage.DiscoveryPreferences{}); err == nil {
		t.Fatal("unavailable write accepted")
	}
	store, err := storage.Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &App{store: store}
	if err = app.SaveDiscoveryPreferences(storage.DiscoveryPreferences{Groups: []storage.DiscoveryGroupPreference{{Key: "invalid"}}}); err == nil {
		t.Fatal("invalid layout accepted")
	}
	value, err := app.GetDiscoveryPreferences()
	if err != nil || len(value.Groups) != 0 {
		t.Fatal("invalid request changed layout")
	}
}
