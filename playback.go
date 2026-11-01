package main

import (
	"context"
	"errors"
	"time"

	"miaoyinniangzi/internal/mediastream"
	"miaoyinniangzi/internal/music"
	"miaoyinniangzi/internal/playbackdiag"
	"miaoyinniangzi/internal/songcache"
	"miaoyinniangzi/internal/sourcenetwork"
	"miaoyinniangzi/internal/storage"
)

type PlayerSession struct {
	Playback storage.Playback `json:"playback"`
	Volume   float64          `json:"volume"`
}
type PreparedSong struct {
	Engine        string  `json:"engine"`
	NativeSession string  `json:"nativeSession,omitempty"`
	SongID        string  `json:"songId"`
	MediaURL      string  `json:"mediaUrl"`
	Quality       string  `json:"quality"`
	Codec         string  `json:"codec"`
	Duration      float64 `json:"duration"`
	Seekable      bool    `json:"seekable"`
	Downgraded    bool    `json:"downgraded"`
	Cacheable     bool    `json:"cacheable"`
}

func (a *App) GetPlayerSession() (PlayerSession, error) {
	if a.store == nil {
		return PlayerSession{}, errors.New("请在桌面应用中恢复播放状态")
	}
	playback, err := a.store.Playback(context.Background())
	if err != nil {
		return PlayerSession{}, errors.New("无法读取播放记录")
	}
	settings, err := a.store.Settings(context.Background())
	if err != nil {
		return PlayerSession{}, errors.New("无法读取音量")
	}
	return PlayerSession{Playback: playback, Volume: settings.Volume}, nil
}
func (a *App) PrepareSong(song music.Song, requestID string, previous, next *music.Song) (prepared PreparedSong, prepareErr error) {
	started := time.Now()
	if _, err := resolveSongInfo(song); err != nil {
		return PreparedSong{}, err
	}
	if !validSearchID(requestID) {
		return PreparedSong{}, errors.New("播放请求无效")
	}
	a.mu.Lock()
	if a.ctx == nil || a.sources == nil || a.store == nil {
		a.mu.Unlock()
		return PreparedSong{}, errors.New("请在桌面应用中播放音乐")
	}
	if a.playCancel != nil {
		a.playCancel()
	}
	if a.media == nil {
		raw := sourcenetwork.New()
		var err error
		a.media, err = mediastream.New(raw)
		if err != nil {
			a.mu.Unlock()
			return PreparedSong{}, err
		}
		a.songCache = songcache.New(a.ctx, raw)
	}
	a.clearNativeLocked()
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	a.playSequence++
	sequence := a.playSequence
	a.playID = requestID
	a.playCancel = cancel
	a.neighborRevision = 0
	cache := a.songCache
	a.mu.Unlock()
	trace := playbackdiag.Begin(a.playDiagnostics, sequence, started)
	trace.Since(playbackdiag.Setup, started)
	ready := false
	defer func() {
		result := prepareErr
		if !ready && ctx.Err() != nil {
			result = ctx.Err()
		}
		cancel()
		a.mu.Lock()
		current := a.playSequence == sequence
		if current {
			a.playCancel = nil
		}
		a.mu.Unlock()
		if !ready && current {
			cache.CancelPending(sequence)
		}
		trace.Finish(result)
	}()
	phase := time.Now()
	settings, err := a.store.Settings(ctx)
	trace.Since(playbackdiag.Settings, phase)
	if err != nil {
		return PreparedSong{}, errors.New("无法读取播放设置")
	}
	source, err := a.activeCacheSource(ctx)
	if err != nil {
		return PreparedSong{}, err
	}
	request := a.cacheRequest(song, source, settings.PlaybackQuality)
	neighbors := a.cacheNeighbors(song, previous, next, source, settings.PlaybackQuality)
	phase = time.Now()
	player, spec, hit, err := cache.Use(ctx, sequence, request, neighbors)
	stage := playbackdiag.CachePrepare
	if hit {
		stage = playbackdiag.CacheReuse
	}
	trace.Since(stage, phase)
	if err != nil {
		return PreparedSong{}, err
	}
	trace.Format(spec.Info.Codec, spec.Quality)
	if !hit {
		for _, timing := range spec.Timings {
			stage := playbackdiag.SourceSnapshot
			switch timing.Stage {
			case songcache.LoadScriptHit:
				stage = playbackdiag.SourceScriptHit
			case songcache.LoadScriptPrepare:
				stage = playbackdiag.SourceScriptPrepare
			case songcache.LoadRequest:
				stage = playbackdiag.SourceRequest
			case songcache.LoadResolve:
				stage = playbackdiag.Resolve
			case songcache.LoadProbe:
				stage = playbackdiag.Probe
			}
			trace.Add(stage, timing.Duration)
		}
	}
	state, err := player.Control("state", 0)
	if err != nil {
		player.Close()
		return PreparedSong{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if ctx.Err() != nil || sequence != a.playSequence {
		player.Close()
		return PreparedSong{}, errors.New("播放请求已取消")
	}
	a.nativePlayer = player
	a.playTrace = trace
	a.cacheKey = request.Key
	ready = true
	return PreparedSong{Engine: "libmpv", NativeSession: requestID, SongID: song.ID, Quality: spec.Quality, Codec: spec.Info.Codec, Duration: state.Duration, Seekable: spec.Info.Seekable && state.Seekable, Downgraded: spec.Downgraded, Cacheable: spec.Cacheable}, nil
}
func (a *App) CancelPlayback(requestID string, discardCache bool) bool {
	a.mu.Lock()
	if requestID == "" || a.playID != requestID {
		a.mu.Unlock()
		return false
	}
	a.clearNativeLocked()
	a.playSequence++
	sequence := a.playSequence
	if a.playCancel != nil {
		a.playCancel()
		a.playCancel = nil
	}
	a.playID = ""
	cache := a.songCache
	a.mu.Unlock()
	if cache != nil {
		if discardCache {
			cache.Invalidate(sequence)
		} else {
			cache.CancelPending(sequence)
		}
	}
	return true
}
func (a *App) invalidatePlayback() {
	a.mu.Lock()
	a.clearNativeLocked()
	a.playSequence++
	sequence := a.playSequence
	if a.playCancel != nil {
		a.playCancel()
		a.playCancel = nil
	}
	a.playID = ""
	cache := a.songCache
	a.mu.Unlock()
	if cache != nil {
		cache.Invalidate(sequence)
	}
}
func (a *App) sourcePlaybackChanged() {
	if a.downloads != nil {
		a.downloads.SourceChanged()
	}
	a.invalidatePlayback()
	a.mu.RLock()
	emit, ctx := a.playbackChanged, a.ctx
	a.mu.RUnlock()
	if ctx != nil && a.playDiagnostics != nil {
		go a.warmSource(ctx)
	}
	if emit != nil {
		emit()
	}
}
func (a *App) closePlayback() {
	a.invalidatePlayback()
	a.mu.Lock()
	cache, media := a.songCache, a.media
	a.songCache = nil
	a.media = nil
	a.mu.Unlock()
	if cache != nil {
		cache.Close()
	}
	if media != nil {
		_ = media.Close()
	}
}
