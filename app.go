package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"miaoyinniangzi/internal/downloads"
	"miaoyinniangzi/internal/mediastream"
	"miaoyinniangzi/internal/mpvaudio"
	"miaoyinniangzi/internal/music"
	"miaoyinniangzi/internal/playbackdiag"
	"miaoyinniangzi/internal/songcache"
	"miaoyinniangzi/internal/sourceruntime"
	"miaoyinniangzi/internal/sources"
	"miaoyinniangzi/internal/storage"
)

// App exposes desktop services through Wails. No playing flag is persisted, so
// restoring a snapshot never starts audio automatically.
type App struct {
	quitting           bool
	quitSequence       uint64
	quitID             string
	quitDone           chan struct{}
	store              *storage.Store
	downloads          *downloads.Manager
	downloadProbe      *mediastream.Server
	media              *mediastream.Server
	nativePlayer       *mpvaudio.Player
	songCache          *songcache.Manager
	cacheKey           songcache.Key
	neighborRevision   uint64
	playDiagnostics    *playbackdiag.Diagnostics
	playTrace          *playbackdiag.Trace
	playCancel         context.CancelFunc
	playID             string
	playSequence       uint64
	playbackChanged    func()
	sources            *sources.Manager
	sourcePicking      bool
	mu                 sync.RWMutex
	ctx                context.Context
	details            music.DetailReader
	detailCancel       context.CancelFunc
	detailID           string
	detailSequence     uint64
	discovery          music.DiscoveryReader
	discoveryCancel    context.CancelFunc
	discoveryID        string
	discoverySequence  uint64
	music              music.Searcher
	suggestions        music.Suggester
	suggestionCancel   context.CancelFunc
	suggestionID       string
	suggestionSequence uint64
	resolveCancel      context.CancelFunc
	resolveID          string
	resolveSequence    uint64
	lyrics             music.LyricReader
	lyricsCancel       context.CancelFunc
	lyricsID           string
	lyricsSequence     uint64
	searchCancel       context.CancelFunc
	searchID           string
	searchSequence     uint64
}

func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ctx = ctx
	a.playbackChanged = func() { runtime.EventsEmit(ctx, "playback:source-changed") }
	if a.downloads != nil {
		a.downloads.SetDuplicateNotice(func() { runtime.EventsEmit(ctx, "download:duplicate") })
	}
	if a.playDiagnostics != nil {
		go func() {
			started := time.Now()
			err := mpvaudio.Warmup()
			a.playDiagnostics.Warmup(playbackdiag.Library, started, err)
		}()
		go a.warmSource(ctx)
	}
}
func (a *App) warmSource(ctx context.Context) {
	if a.sources == nil {
		return
	}
	started := time.Now()
	ready, err := a.sources.Warmup(ctx)
	if ready || err != nil {
		a.playDiagnostics.Warmup(playbackdiag.SourcePrepare, started, err)
	}
}

// ChooseDownloadDirectory returns an empty string when the user cancels.
// It starts at the current form value, even before autosave has completed.
func (a *App) ChooseDownloadDirectory(initialDirectory string) (string, error) {
	a.mu.RLock()
	ctx := a.ctx
	a.mu.RUnlock()
	if ctx == nil {
		return "", errors.New("directory picker requires the desktop application")
	}
	settings, err := a.store.Settings(ctx)
	if err != nil {
		return "", err
	}
	initial := pickerDirectory(initialDirectory, settings.DownloadDirectory)
	return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
		Title: "选择下载目录", DefaultDirectory: initial,
	})
}

// Prefer the draft selection, falling back to the saved path only if needed.
func pickerDirectory(candidates ...string) string {
	for _, path := range candidates {
		if !filepath.IsAbs(path) {
			continue
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}

func openApp() (*App, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	store, err := storage.Open(filepath.Join(config, "妙音娘子"), filepath.Join(home, "Downloads"))
	if err != nil {
		return nil, err
	}
	inspector, err := sourceruntime.NewInspector()
	if err != nil {
		store.Close()
		return nil, err
	}
	if err := store.InvalidateSourceChecks(context.Background(), sourceruntime.EngineVersion); err != nil {
		store.Close()
		return nil, err
	}
	qq := music.NewQQClient().WithDiagnostics(music.NewDiagnostics(filepath.Join(config, "妙音娘子", "logs")))
	app := &App{store: store, sources: sources.New(store, sources.NewSystemKeys(filepath.Join(config, "妙音娘子")), inspector), music: qq, discovery: qq, details: qq, suggestions: qq, lyrics: qq, playDiagnostics: playbackdiag.New(filepath.Join(config, "妙音娘子", "logs"))}
	// A damaged download record must not prevent playback or access to settings.
	// The download page reports an unavailable service without resetting data.
	_ = app.initDownloads()
	return app, nil
}
func (a *App) GetSettings() (storage.Settings, error) { return a.store.Settings(context.Background()) }
func (a *App) SaveSettings(value storage.Settings) error {
	return a.store.SaveSettings(context.Background(), value)
}
func (a *App) GetPlayback() (storage.Playback, error) { return a.store.Playback(context.Background()) }
func (a *App) SavePlayback(value storage.Playback) error {
	return a.store.SavePlayback(context.Background(), value)
}
