// Package mpvaudio controls libmpv through its stable client API. Media may only
// enter through the parent's opaque loopback capability, never a signed URL.
package mpvaudio

import (
	"context"
	"errors"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
)

var ErrPlayer = errors.New("libmpv 播放失败，请重试")
var ErrLibrary = errors.New("未找到可用的 libmpv 播放库，请使用完整应用包")

type State struct {
	Position  float64 `json:"position"`
	Duration  float64 `json:"duration"`
	Playing   bool    `json:"playing"`
	Ended     bool    `json:"ended"`
	Buffering bool    `json:"buffering"`
	Seekable  bool    `json:"seekable"`
}

type event struct{ id, failure int }
type client interface {
	option(string, string) bool
	initialize() bool
	command(...string) bool
	get(string) string
	poll() event
	destroy()
}

type Player struct {
	operation sync.Mutex
	mu        sync.Mutex
	client    client
	paused    bool
	duration  float64
	failure   bool
}

// LibraryPath does not search the current directory or arbitrary system DLL
// names. MIAOYIN_MPV_LIBRARY is an explicit development/test override only.
func LibraryPath() string {
	if value := os.Getenv("MIAOYIN_MPV_LIBRARY"); value != "" {
		if filepath.IsAbs(value) {
			return value
		}
		return ""
	}
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(filepath.Dir(executable), "..", "Frameworks", "libmpv.2.dylib")
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(filepath.Dir(executable), "libmpv-2.dll")
	}
	return ""
}

func validMedia(value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path == "" {
		return false
	}
	host, port, err := net.SplitHostPort(u.Host)
	number, parseErr := strconv.Atoi(port)
	return err == nil && parseErr == nil && host == "127.0.0.1" && number > 0 && number <= 65535
}

func Open(ctx context.Context, media, codec string) (*Player, error) {
	return open(ctx, media, codec, newClient)
}

type StartupStage uint8

const (
	StartupLibrary StartupStage = iota + 1
	StartupInitialize
	StartupLoadReady
)

type StartupObserver func(StartupStage, time.Duration)

func OpenObserved(ctx context.Context, media, codec string, observer StartupObserver) (*Player, error) {
	return openObserved(ctx, media, codec, newClient, observer)
}
func observe(observer StartupObserver, stage StartupStage, started time.Time) {
	if observer != nil {
		observer(stage, time.Since(started))
	}
}
func open(ctx context.Context, media, codec string, factory func(string) (client, error)) (*Player, error) {
	return openObserved(ctx, media, codec, factory, nil)
}
func openObserved(ctx context.Context, media, codec string, factory func(string) (client, error), observer StartupObserver) (*Player, error) {
	if !validMedia(media) || (codec != "flac" && codec != "mp3") {
		return nil, ErrPlayer
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	started := time.Now()
	c, err := factory(LibraryPath())
	observe(observer, StartupLibrary, started)
	if err != nil {
		return nil, ErrLibrary
	}
	p := &Player{client: c, paused: true}
	success := false
	defer func() {
		if !success {
			p.Close()
		}
	}()
	started = time.Now()
	// No configuration, user scripts, external sidecars, playlists, or ytdl.
	// Only mp3/flac demuxers may interpret the already-probed media bytes.
	options := [][2]string{
		{"config", "no"}, {"load-scripts", "no"}, {"ytdl", "no"},
		{"osc", "no"}, {"load-auto-profiles", "no"}, {"input-default-bindings", "no"},
		{"terminal", "no"}, {"video", "no"}, {"audio-display", "no"},
		{"audio-file-auto", "no"}, {"sub-auto", "no"}, {"autoload-files", "no"},
		{"access-references", "no"}, {"demuxer", "lavf"}, {"demuxer-lavf-format", codec},
		{"demuxer-lavf-o", "protocol_whitelist=[http,tcp],http_proxy="},
		{"stream-lavf-o", "protocol_whitelist=[http,tcp],http_proxy="},
		{"idle", "yes"}, {"pause", "yes"}, {"volume", "0"}, {"keep-open", "always"},
		{"hr-seek", "yes"}, {"hr-seek-demuxer-offset", "0.25"},
		{"network-timeout", "8"}, {"cache", "yes"}, {"cache-secs", "5"}, {"cache-on-disk", "no"},
		{"demuxer-max-bytes", "8MiB"}, {"demuxer-max-back-bytes", "2MiB"},
	}
	initialized := func() bool {
		for _, option := range options {
			if !c.option(option[0], option[1]) {
				return false
			}
		}
		return c.initialize()
	}()
	observe(observer, StartupInitialize, started)
	if !initialized {
		return nil, ErrPlayer
	}
	started = time.Now()
	loadErr := func() error {
		if !c.command("loadfile", media) {
			return ErrPlayer
		}
		// FILE_LOADED alone is too early: wait for initial audio-output setup.
		return p.wait(ctx, false)
	}()
	observe(observer, StartupLoadReady, started)
	if loadErr != nil {
		return nil, loadErr
	}
	p.mu.Lock()
	p.stateLocked()
	p.mu.Unlock()
	success = true
	return p, nil
}

func (p *Player) drainLocked() (events []event) {
	for count := 0; count < 256; count++ {
		e := p.client.poll()
		if e.id == 0 {
			break
		}
		if e.id == 7 && e.failure < 0 {
			p.failure = true
		}
		if e.id == 1 || e.id == 24 {
			p.failure = true
		}
		events = append(events, e)
	}
	return
}

func (p *Player) wait(ctx context.Context, seeking bool) error {
	seenSeek := !seeking
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		p.mu.Lock()
		if p.client == nil {
			p.mu.Unlock()
			return ErrPlayer
		}
		restart := false
		for _, e := range p.drainLocked() {
			if e.id == 20 {
				seenSeek = true
				restart = false
			}
			if e.id == 21 && seenSeek {
				restart = true
			}
		}
		failure := p.failure
		p.mu.Unlock()
		if failure {
			return ErrPlayer
		}
		if seenSeek && restart {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func number(value string) float64 {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return v
}
func (p *Player) stateLocked() State {
	position := number(p.client.get("time-pos"))
	if duration := number(p.client.get("duration")); duration > 0 {
		p.duration = duration
	}
	eof := p.client.get("eof-reached") == "yes"
	buffering := !p.paused && p.client.get("paused-for-cache") == "yes"
	// A paused seek near EOF must never advance the host queue.
	ended := eof && !p.paused
	return State{Position: position, Duration: p.duration, Playing: !p.paused && p.client.get("pause") != "yes" && !buffering && !ended, Ended: ended, Buffering: buffering, Seekable: p.client.get("seekable") == "yes"}
}

func (p *Player) Control(action string, value float64) (State, error) {
	p.operation.Lock()
	defer p.operation.Unlock()
	p.mu.Lock()
	if p.client == nil {
		p.mu.Unlock()
		return State{}, ErrPlayer
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		p.mu.Unlock()
		return State{}, ErrPlayer
	}
	// Remove old restart events before acknowledging a new seek.
	p.drainLocked()
	if p.failure {
		p.mu.Unlock()
		return State{}, ErrPlayer
	}
	ok := true
	switch action {
	case "play":
		ok = p.client.command("set", "pause", "no")
		if ok {
			p.paused = false
		}
	case "pause":
		ok = p.client.command("set", "pause", "yes")
		if ok {
			p.paused = true
		}
	case "volume":
		value = math.Max(0, math.Min(1, value))
		ok = p.client.command("set", "volume", strconv.FormatFloat(value*100, 'f', -1, 64))
	case "seek":
		value = math.Max(0, value)
		if p.duration > 0 {
			value = math.Min(value, math.Max(0, p.duration-1.0/44100))
		}
		ok = p.client.command("seek", strconv.FormatFloat(value, 'f', -1, 64), "absolute+exact")
	case "state":
	default:
		ok = false
	}
	p.mu.Unlock()
	if !ok {
		return State{}, ErrPlayer
	}
	if action == "seek" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := p.wait(ctx, true)
		cancel()
		if err != nil {
			return State{}, ErrPlayer
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client == nil {
		return State{}, ErrPlayer
	}
	return p.stateLocked(), nil
}

func (p *Player) Close() {
	p.mu.Lock()
	c := p.client
	p.client = nil
	p.mu.Unlock()
	if c != nil {
		c.destroy()
	}
}
