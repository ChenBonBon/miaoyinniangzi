package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestDiscoveryCachePersistsAndBoundsEntries(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	download := t.TempDir()
	store, err := Open(dir, download)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveDiscoveryCache(ctx, DiscoveryCacheEntry{Key: "chart:catalog", SavedAt: 1, Data: json.RawMessage(`[]`)}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		if err = store.SaveDiscoveryCache(ctx, DiscoveryCacheEntry{Key: fmt.Sprintf("chart:collection:%d:", i+1), SavedAt: int64(i + 1), Data: json.RawMessage(`{"ok":true}`)}); err != nil {
			t.Fatal(err)
		}
	}
	store.Close()
	store, err = Open(filepath.Clean(dir), download)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old, err := store.DiscoveryCache(ctx, "chart:collection:1:")
	if err != nil || old.Key != "" {
		t.Fatal("oldest cache was not evicted")
	}
	catalog, err := store.DiscoveryCache(ctx, "chart:catalog")
	if err != nil || catalog.Key == "" {
		t.Fatal("list browsing evicted the landing-page catalog")
	}
	newest, err := store.DiscoveryCache(ctx, "chart:collection:12:")
	if err != nil || newest.SavedAt != 12 {
		t.Fatal("cache did not survive reopen")
	}
}
func TestDiscoveryCacheConcurrentWritesAndCorruptionRecovery(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var workers sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			errs <- store.SaveDiscoveryCache(ctx, DiscoveryCacheEntry{Key: fmt.Sprintf("chart:collection:%d:", i+1), SavedAt: int64(i + 1), Data: json.RawMessage(`[]`)})
		}(i)
	}
	workers.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	entries, err := store.discoveryCacheEntries(ctx)
	if err != nil || len(entries) != 8 {
		t.Fatal("concurrent cache write lost an entry")
	}
	for _, raw := range []string{"broken", strings.Repeat("x", maxDiscoveryCacheBytes+1)} {
		if _, err = store.db.Exec(`UPDATE state SET payload=? WHERE key=?`, raw, discoveryCacheState); err != nil {
			t.Fatal(err)
		}
		value, err := store.DiscoveryCache(ctx, "chart:catalog")
		if err != nil || value.Key != "" {
			t.Fatal("corrupt cache was served")
		}
		if err = store.SaveDiscoveryCache(ctx, DiscoveryCacheEntry{Key: "chart:catalog", SavedAt: 100, Data: json.RawMessage(`[]`)}); err != nil {
			t.Fatal("fresh data could not replace corrupt cache")
		}
	}
}
func TestDiscoveryCachePayloadAndAggregateBounds(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if store.SaveDiscoveryCache(ctx, DiscoveryCacheEntry{Key: "bad/key", SavedAt: 1, Data: json.RawMessage(`[]`)}) == nil || store.SaveDiscoveryCache(ctx, DiscoveryCacheEntry{Key: "chart:catalog", SavedAt: 1, Data: json.RawMessage(strings.Repeat("x", MaxDiscoveryCachePayload+1))}) == nil {
		t.Fatal("invalid cache accepted")
	}
	if err = store.SaveDiscoveryCache(ctx, DiscoveryCacheEntry{Key: "chart:catalog", SavedAt: 1, Data: json.RawMessage(`[]`)}); err != nil {
		t.Fatal(err)
	}
	escaped := json.RawMessage(`"` + strings.Repeat("<", 800000) + `"`)
	if store.SaveDiscoveryCache(ctx, DiscoveryCacheEntry{Key: "chart:collection:99:", SavedAt: 99, Data: escaped}) == nil {
		t.Fatal("encoded cache entry exceeded aggregate bound")
	}
	data := json.RawMessage(`"` + strings.Repeat("x", 900000) + `"`)
	for i := 0; i < 8; i++ {
		if err = store.SaveDiscoveryCache(ctx, DiscoveryCacheEntry{Key: fmt.Sprintf("chart:collection:%d:", i+1), SavedAt: int64(i + 1), Data: data}); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := store.DiscoveryCache(ctx, "chart:catalog")
	if err != nil || catalog.Key == "" {
		t.Fatal("large collection eviction removed the catalog")
	}
	var size int
	if err = store.db.QueryRow(`SELECT length(CAST(payload AS BLOB)) FROM state WHERE key=?`, discoveryCacheState).Scan(&size); err != nil || size > maxDiscoveryCacheBytes {
		t.Fatal("cache total bound exceeded")
	}
}
