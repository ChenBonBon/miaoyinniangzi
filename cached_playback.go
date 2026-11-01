package main

import (
	"context"
	"errors"
	"time"

	"miaoyinniangzi/internal/music"
	"miaoyinniangzi/internal/songcache"
	"miaoyinniangzi/internal/sources"
)

func (a *App) GetPlaybackCacheStatus() songcache.Status {
	a.mu.RLock()
	cache := a.songCache
	a.mu.RUnlock()
	if cache == nil {
		return songcache.Status{Neighbors: []songcache.SlotStatus{}}
	}
	return cache.Status()
}
func (a *App) activeCacheSource(ctx context.Context) (string, error) {
	entries, err := a.store.Sources(ctx)
	if err != nil {
		return "", errors.New("无法读取当前音源")
	}
	for _, entry := range entries {
		if entry.Active && entry.CheckStatus == "ready" {
			return entry.ID, nil
		}
	}
	return "", errors.New("请先启用已校验的音源")
}
func (a *App) cacheRequest(song music.Song, source, quality string) songcache.Request {
	a.mu.RLock()
	probe := a.media
	a.mu.RUnlock()
	key := songcache.Key{Source: source, Quality: quality, Song: song.ID}
	return songcache.Request{Key: key, Load: func(ctx context.Context) (songcache.Spec, error) {
		info, err := resolveSongInfo(song)
		if err != nil {
			return songcache.Spec{}, err
		}
		var timings []songcache.LoadTiming
		started := time.Now()
		resolved, err := a.sources.ResolveObserved(ctx, info, quality, func(phase sources.ResolvePhase, duration time.Duration) {
			stage := songcache.LoadSnapshot
			switch phase {
			case sources.PhaseScriptHit:
				stage = songcache.LoadScriptHit
			case sources.PhaseScriptPrepare:
				stage = songcache.LoadScriptPrepare
			case sources.PhaseRequest:
				stage = songcache.LoadRequest
			}
			timings = append(timings, songcache.LoadTiming{Stage: stage, Duration: duration})
		})
		timings = append(timings, songcache.LoadTiming{Stage: songcache.LoadResolve, Duration: time.Since(started)})
		if err != nil {
			return songcache.Spec{}, err
		}
		if resolved.SourceID != source {
			return songcache.Spec{}, errors.New("音源选择已变化")
		}
		if probe == nil {
			return songcache.Spec{}, errors.New("播放已关闭")
		}
		started = time.Now()
		metadata, err := probe.Probe(ctx, resolved.URL)
		timings = append(timings, songcache.LoadTiming{Stage: songcache.LoadProbe, Duration: time.Since(started)})
		if err != nil {
			return songcache.Spec{}, err
		}
		current, err := a.store.Source(ctx, source)
		if err != nil || !current.Active || current.CheckStatus != "ready" {
			return songcache.Spec{}, errors.New("音源选择已变化")
		}
		return songcache.Spec{Remote: resolved.URL, Info: metadata, Quality: resolved.Quality, Downgraded: resolved.Downgraded, Timings: timings}, nil
	}}
}
func (a *App) cacheNeighbors(current music.Song, previous, next *music.Song, source, quality string) []songcache.Request {
	result := []songcache.Request{}
	seen := map[string]bool{current.ID: true}
	// Prioritize the natural next song before the previous song.
	for _, song := range []*music.Song{next, previous} {
		if song == nil || seen[song.ID] {
			continue
		}
		if _, err := resolveSongInfo(*song); err != nil {
			continue
		}
		seen[song.ID] = true
		result = append(result, a.cacheRequest(*song, source, quality))
	}
	return result
}
func (a *App) PrefetchNeighbors(requestID string, revision uint64, current music.Song, previous, next *music.Song) bool {
	a.mu.Lock()
	if a.playID != requestID || a.nativePlayer == nil || a.songCache == nil || a.cacheKey.Song != current.ID || revision <= a.neighborRevision {
		a.mu.Unlock()
		return false
	}
	a.neighborRevision = revision
	cache, key, sequence := a.songCache, a.cacheKey, a.playSequence
	a.mu.Unlock()
	cache.Prefetch(sequence, revision, key, a.cacheNeighbors(current, previous, next, key.Source, key.Quality))
	return true
}
