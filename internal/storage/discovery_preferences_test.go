package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoveryPreferencesPersistIndependently(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	downloads := filepath.Join(t.TempDir(), "Downloads")
	store, err := Open(dir, downloads)
	if err != nil {
		t.Fatal(err)
	}
	defaults, err := store.DiscoveryPreferences(ctx)
	if err != nil || len(defaults.Groups) != 0 {
		t.Fatal("new preferences not default")
	}
	before, _ := store.Settings(ctx)
	value := DiscoveryPreferences{Groups: []DiscoveryGroupPreference{{Key: "qq:0", Hidden: true, Charts: []DiscoveryChartPreference{{ID: 62, Hidden: true}, {ID: 26}}}, {Key: "qq:3", Charts: []DiscoveryChartPreference{{ID: 27}}}}}
	if err = store.SaveDiscoveryPreferences(ctx, value); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(dir, downloads)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	restored, err := store.DiscoveryPreferences(ctx)
	if err != nil || !reflect.DeepEqual(value, restored) {
		t.Fatal("layout did not survive reopen")
	}
	after, _ := store.Settings(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("layout changed player/download settings")
	}
	if err = store.SaveDiscoveryPreferences(ctx, DiscoveryPreferences{}); err != nil {
		t.Fatal(err)
	}
	restored, err = store.DiscoveryPreferences(ctx)
	if err != nil || restored.Groups == nil || len(restored.Groups) != 0 {
		t.Fatal("reset failed")
	}
}
func TestDiscoveryPreferencesRejectInvalidWithoutOverwriting(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir(), filepath.Join(t.TempDir(), "Downloads"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	valid := DiscoveryPreferences{Groups: []DiscoveryGroupPreference{{Key: "qq:0", Charts: []DiscoveryChartPreference{{ID: 1}}}}}
	if err = store.SaveDiscoveryPreferences(ctx, valid); err != nil {
		t.Fatal(err)
	}
	for _, value := range []DiscoveryPreferences{
		{Groups: []DiscoveryGroupPreference{{Key: "qq:-1"}}},
		{Groups: []DiscoveryGroupPreference{{Key: "qq:01"}}},
		{Groups: []DiscoveryGroupPreference{{Key: "name:"}}},
		{Groups: []DiscoveryGroupPreference{{Key: "name:a\n"}}},
		{Groups: []DiscoveryGroupPreference{{Key: "qq:1"}, {Key: "qq:1"}}},
		{Groups: []DiscoveryGroupPreference{{Key: "qq:0", Charts: []DiscoveryChartPreference{{ID: 0}}}}},
		{Groups: []DiscoveryGroupPreference{{Key: "qq:0", Charts: []DiscoveryChartPreference{{ID: 1}, {ID: 1}}}}},
	} {
		if store.SaveDiscoveryPreferences(ctx, value) == nil {
			t.Fatal("invalid layout accepted")
		}
	}
	restored, err := store.DiscoveryPreferences(ctx)
	if err != nil || !reflect.DeepEqual(valid, restored) {
		t.Fatal("invalid layout overwrote prior state")
	}
	if _, err = store.db.ExecContext(ctx, `UPDATE state SET payload='broken' WHERE key='discovery_preferences'`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DiscoveryPreferences(ctx); err == nil {
		t.Fatal("corrupted layout silently reset")
	}
}
