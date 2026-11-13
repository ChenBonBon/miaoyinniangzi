package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"miaoyinniangzi/internal/music"
	"miaoyinniangzi/internal/storage"
)

const forYouSeedLimit = 5
const forYouCacheLimit = 20
const forYouFresh = time.Hour
const forYouRetain = 72 * time.Hour

type ForYouResult struct {
	Songs     []music.Song `json:"songs"`
	Reason    string       `json:"reason"`
	SeedCount int          `json:"seedCount"`
	Skipped   int          `json:"skipped"`
	Warning   string       `json:"warning"`
}
type forYouCached struct {
	Songs []music.Song
	Saved time.Time
}

func favoriteFingerprint(songs []storage.Song) string {
	data, _ := json.Marshal(songs)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func favoriteArtist(song storage.Song) string {
	for _, artist := range song.Artists {
		if artist.ID != "" {
			return artist.ID
		}
	}
	return song.Artist
}
func forYouSeeds(favorites []storage.Song) ([]storage.Song, int) {
	eligible := []storage.Song{}
	seen := map[string]bool{}
	skipped := 0
	for _, song := range favorites {
		if song.Source != "tx" || !music.ValidSongSeed(song.SongID) {
			skipped++
			continue
		}
		if !seen[song.SongID] {
			eligible = append(eligible, song)
			seen[song.SongID] = true
		}
	}
	seeds := []storage.Song{}
	artists := map[string]bool{}
	selected := map[string]bool{}
	for _, song := range eligible {
		artist := favoriteArtist(song)
		if artist != "" && artists[artist] {
			continue
		}
		seeds = append(seeds, song)
		artists[artist] = true
		selected[song.SongID] = true
		if len(seeds) == forYouSeedLimit {
			return seeds, skipped
		}
	}
	for _, song := range eligible {
		if selected[song.SongID] {
			continue
		}
		seeds = append(seeds, song)
		if len(seeds) == forYouSeedLimit {
			break
		}
	}
	return seeds, skipped
}
func forYouContextError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return discoveryError(music.ErrTimeout)
	}
	return discoveryError(music.ErrCanceled)
}
func (a *App) CancelForYou(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id == "" || id != a.forYouID || a.forYouCancel == nil {
		return false
	}
	a.forYouCancel()
	return true
}
func (a *App) beginForYou(id string) (context.Context, func(), music.SimilarReader, error) {
	if !validSearchID(id) {
		return nil, nil, nil, errors.New("猜你喜欢请求无效")
	}
	a.mu.Lock()
	if a.ctx == nil || a.similar == nil || a.store == nil {
		a.mu.Unlock()
		return nil, nil, nil, errors.New("请在桌面应用中读取猜你喜欢")
	}
	if a.forYouCancel != nil {
		a.forYouCancel()
	}
	ctx, cancel := context.WithTimeout(a.ctx, 35*time.Second)
	a.forYouSequence++
	sequence := a.forYouSequence
	a.forYouID = id
	a.forYouCancel = cancel
	reader := a.similar
	a.mu.Unlock()
	return ctx, func() {
		cancel()
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.forYouSequence == sequence {
			a.forYouCancel = nil
			a.forYouID = ""
		}
	}, reader, nil
}

// GetForYou reads only favorite metadata and anonymous similar-song metadata.
// It cannot resolve sources, produce media URLs, start playback or download.
func (a *App) GetForYou(id string) (ForYouResult, error) {
	result := ForYouResult{Songs: []music.Song{}, Reason: "no_favorites"}
	ctx, cleanup, reader, err := a.beginForYou(id)
	if err != nil {
		return result, err
	}
	defer cleanup()
	favorites, err := a.store.Favorites(ctx)
	if err != nil {
		return result, errors.New("无法读取收藏，请重试")
	}
	if len(favorites) == 0 {
		return result, nil
	}
	fingerprint := favoriteFingerprint(favorites)
	seeds, skipped := forYouSeeds(favorites)
	result.Skipped = skipped
	result.SeedCount = len(seeds)
	result.Reason = "missing_song_ids"
	if len(seeds) == 0 {
		return result, nil
	}
	a.forYouMu.Lock()
	defer a.forYouMu.Unlock()
	if ctx.Err() != nil {
		return result, forYouContextError(ctx)
	}
	if a.forYouCache == nil {
		a.forYouCache = map[string]forYouCached{}
	}
	lists := [][]music.Song{}
	failed := 0
	blocked := false
	var firstErr error
	for _, seed := range seeds {
		if ctx.Err() != nil {
			return result, forYouContextError(ctx)
		}
		cached, exists := a.forYouCache[seed.SongID]
		age := time.Since(cached.Saved)
		if exists && age >= 0 && age < forYouFresh {
			lists = append(lists, cached.Songs)
			continue
		}
		if blocked {
			failed++
			if exists && age >= 0 && age <= forYouRetain {
				lists = append(lists, cached.Songs)
			}
			continue
		}
		songs, readErr := reader.SimilarSongs(ctx, seed.SongID)
		if ctx.Err() != nil {
			return result, forYouContextError(ctx)
		}
		if readErr == nil && (!music.ValidSongs(songs) || len(songs) > 200) {
			readErr = music.ErrResponse
		}
		if readErr != nil {
			if errors.Is(readErr, music.ErrRateLimit) || errors.Is(readErr, music.ErrRestricted) || errors.Is(readErr, music.ErrNetwork) || errors.Is(readErr, music.ErrTimeout) {
				blocked = true
			}
			failed++
			if firstErr == nil {
				firstErr = readErr
			}
			if exists && age >= 0 && age <= forYouRetain {
				lists = append(lists, cached.Songs)
			}
			continue
		}
		a.mu.Lock()
		if ctx.Err() != nil || a.forYouID != id {
			a.mu.Unlock()
			return result, forYouContextError(ctx)
		}
		a.forYouCache[seed.SongID] = forYouCached{Songs: songs, Saved: time.Now()}
		for len(a.forYouCache) > forYouCacheLimit {
			oldest := ""
			var oldestTime time.Time
			for key, value := range a.forYouCache {
				if oldest == "" || value.Saved.Before(oldestTime) {
					oldest = key
					oldestTime = value.Saved
				}
			}
			delete(a.forYouCache, oldest)
		}
		a.mu.Unlock()
		lists = append(lists, songs)
	}
	latest, err := a.store.Favorites(ctx)
	if err != nil {
		return result, errors.New("无法读取收藏，请重试")
	}
	if favoriteFingerprint(latest) != fingerprint {
		return result, errors.New("收藏已更新，请重新获取推荐")
	}
	excludedMID := map[string]bool{}
	excludedID := map[string]bool{}
	for _, song := range favorites {
		excludedMID[song.ID] = true
		if song.SongID != "" {
			excludedID[song.SongID] = true
		}
	}
	seenMID := map[string]bool{}
	seenID := map[string]bool{}
	// Round-robin preserves each seed's upstream ranking without letting the
	// first favorite monopolize the pool. Alternate versions with the same ID
	// are not separate candidates.
	for rank := 0; rank < 200 && len(result.Songs) < 128; rank++ {
		for _, songs := range lists {
			if rank >= len(songs) {
				continue
			}
			song := songs[rank]
			if excludedMID[song.ID] || song.SongID != "" && excludedID[song.SongID] || seenMID[song.ID] || song.SongID != "" && seenID[song.SongID] || song.AlbumMid == "" || !music.ValidSongSeed(song.SongID) {
				continue
			}
			seenMID[song.ID] = true
			if song.SongID != "" {
				seenID[song.SongID] = true
			}
			result.Songs = append(result.Songs, song)
			if len(result.Songs) == 128 {
				break
			}
		}
	}
	a.mu.RLock()
	current := a.forYouID == id && ctx.Err() == nil
	a.mu.RUnlock()
	if !current {
		return ForYouResult{Songs: []music.Song{}}, forYouContextError(ctx)
	}
	if len(result.Songs) == 0 && firstErr != nil {
		return result, discoveryError(firstErr)
	}
	result.Reason = "ready"
	if len(result.Songs) == 0 {
		result.Reason = "no_candidates"
	}
	if failed > 0 {
		result.Warning = "部分推荐更新未完成，已保留可用内容"
	}
	return result, nil
}
