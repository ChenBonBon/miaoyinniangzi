package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"miaoyinniangzi/internal/music"
	"miaoyinniangzi/internal/storage"
)

const discoveryCacheRetention = 72 * time.Hour

type DiscoverySnapshot struct {
	SavedAt    int64                      `json:"savedAt"`
	Catalog    *music.DiscoveryCatalog    `json:"catalog,omitempty"`
	Collection *music.DiscoveryCollection `json:"collection,omitempty"`
}

func discoveryCacheKey(kind string, id int, period string) string {
	if !music.ValidDiscoveryKind(kind) {
		return ""
	}
	if id == 0 && period == "" {
		return kind + ":catalog"
	}
	if !music.ValidDiscoverySelection(kind, id, period) {
		return ""
	}
	return fmt.Sprintf("%s:collection:%d:%s", kind, id, period)
}

// Cache access never starts a QQ request, resolves media, or touches source
// scripts. Invalid/corrupt/expired cache is a miss, not a fake empty catalog.
func (a *App) GetDiscoverySnapshot(kind string, id int, period string) DiscoverySnapshot {
	key := discoveryCacheKey(kind, id, period)
	if key == "" {
		return DiscoverySnapshot{}
	}
	a.mu.RLock()
	store, base := a.store, a.ctx
	a.mu.RUnlock()
	if store == nil || base == nil {
		return DiscoverySnapshot{}
	}
	ctx, cancel := context.WithTimeout(base, time.Second)
	defer cancel()
	cached, err := store.DiscoveryCache(ctx, key)
	if err != nil {
		return DiscoverySnapshot{}
	}
	now := time.Now().UnixMilli()
	if cached.SavedAt <= 0 || cached.SavedAt > now+60000 || now-cached.SavedAt > discoveryCacheRetention.Milliseconds() {
		return DiscoverySnapshot{}
	}
	result := DiscoverySnapshot{SavedAt: cached.SavedAt}
	if id == 0 {
		var value music.DiscoveryCatalog
		if json.Unmarshal(cached.Data, &value) != nil || value.Kind != kind || !music.ValidDiscoveryCatalogValue(value) {
			return DiscoverySnapshot{}
		}
		result.Catalog = &value
	} else {
		var value music.DiscoveryCollection
		if json.Unmarshal(cached.Data, &value) != nil || value.Entry.Kind != kind || value.Entry.ID != id || period != "" && value.Entry.Period != period || !music.ValidDiscoveryCollectionValue(value) {
			return DiscoverySnapshot{}
		}
		result.Collection = &value
	}
	return result
}
func (a *App) saveDiscoverySnapshot(ctx context.Context, key string, value any) {
	if key == "" || ctx.Err() != nil {
		return
	}
	a.mu.RLock()
	store := a.store
	a.mu.RUnlock()
	if store == nil {
		return
	}
	data, err := json.Marshal(value)
	if err != nil || len(data) > storage.MaxDiscoveryCachePayload {
		return
	}
	// Best effort: a disk/cache error must not turn valid live metadata into a
	// failed page or overwrite the previous success with an empty/error payload.
	cacheCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	_ = store.SaveDiscoveryCache(cacheCtx, storage.DiscoveryCacheEntry{Key: key, SavedAt: time.Now().UnixMilli(), Data: data})
}
