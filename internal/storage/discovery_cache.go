package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
)

const MaxDiscoveryCachePayload = 1 << 20
const maxDiscoveryCacheBytes = 4 << 20
const maxDiscoveryCacheEntries = 10
const discoveryCacheState = "discovery_cache_v1"

type DiscoveryCacheEntry struct {
	Key     string          `json:"key"`
	SavedAt int64           `json:"savedAt"`
	Data    json.RawMessage `json:"data"`
}

func validDiscoveryCacheEntry(entry DiscoveryCacheEntry) bool {
	if len(entry.Key) == 0 || len(entry.Key) > 128 || entry.SavedAt <= 0 || len(entry.Data) == 0 || len(entry.Data) > MaxDiscoveryCachePayload || !json.Valid(entry.Data) {
		return false
	}
	for _, ch := range entry.Key {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == ':' || ch == '_' || ch == '-') {
			return false
		}
	}
	return true
}
func (s *Store) discoveryCacheEntries(ctx context.Context) ([]DiscoveryCacheEntry, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM state WHERE key=? AND length(CAST(payload AS BLOB))<=?`, discoveryCacheState, maxDiscoveryCacheBytes).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("无法读取推荐缓存")
	}
	var entries []DiscoveryCacheEntry
	if json.Unmarshal([]byte(raw), &entries) != nil || len(entries) > maxDiscoveryCacheEntries {
		return nil, nil
	}
	valid := make([]DiscoveryCacheEntry, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		if validDiscoveryCacheEntry(entry) && !seen[entry.Key] {
			valid = append(valid, entry)
			seen[entry.Key] = true
		}
	}
	return valid, nil
}
func (s *Store) DiscoveryCache(ctx context.Context, key string) (DiscoveryCacheEntry, error) {
	s.discoveryCacheMu.Lock()
	defer s.discoveryCacheMu.Unlock()
	entries, err := s.discoveryCacheEntries(ctx)
	if err != nil {
		return DiscoveryCacheEntry{}, err
	}
	for _, entry := range entries {
		if entry.Key == key {
			return entry, nil
		}
	}
	return DiscoveryCacheEntry{}, nil
}
func (s *Store) SaveDiscoveryCache(ctx context.Context, entry DiscoveryCacheEntry) error {
	if !validDiscoveryCacheEntry(entry) {
		return errors.New("推荐缓存数据无效")
	}
	encoded, encodeErr := json.Marshal([]DiscoveryCacheEntry{entry})
	if encodeErr != nil || len(encoded) > maxDiscoveryCacheBytes {
		return errors.New("推荐缓存数据无效")
	}
	s.discoveryCacheMu.Lock()
	defer s.discoveryCacheMu.Unlock()
	entries, err := s.discoveryCacheEntries(ctx)
	if err != nil {
		return err
	}
	next := make([]DiscoveryCacheEntry, 0, len(entries)+1)
	for _, old := range entries {
		if old.Key != entry.Key {
			next = append(next, old)
		}
	}
	next = append(next, entry)
	sort.SliceStable(next, func(i, j int) bool { return next[i].SavedAt < next[j].SavedAt })
	isCatalog := func(key string) bool { return key == "chart:catalog" || key == "new:catalog" }
	collectionCount := 0
	for _, value := range next {
		if !isCatalog(value.Key) {
			collectionCount++
		}
	}
	dropOldest := func() {
		for i, value := range next {
			if !isCatalog(value.Key) {
				next = append(next[:i], next[i+1:]...)
				return
			}
		}
		next = next[1:]
	}
	// Browsing many lists must not evict the landing-page catalogs.
	for collectionCount > 8 || len(next) > maxDiscoveryCacheEntries {
		dropOldest()
		collectionCount--
	}
	for {
		raw, err := json.Marshal(next)
		if err != nil {
			return errors.New("推荐缓存数据无效")
		}
		if len(raw) <= maxDiscoveryCacheBytes {
			break
		}
		dropOldest()
	}
	if err = s.save(ctx, discoveryCacheState, next); err != nil {
		return errors.New("无法保存推荐缓存")
	}
	return nil
}
