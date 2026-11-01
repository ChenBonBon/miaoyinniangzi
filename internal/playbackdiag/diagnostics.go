// Package playbackdiag records bounded local timing data, never song metadata,
// request IDs, source URLs, script contents, native diagnostics or raw errors.
package playbackdiag

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

const limit = 64 * 1024

type Stage uint8

const (
	Setup Stage = iota + 1
	Settings
	Resolve
	Probe
	SourceCheck
	Lease
	Library
	Initialize
	LoadReady
	Commit
	Unpause
	SourcePrepare
	SourceSnapshot
	SourceScriptHit
	SourceScriptPrepare
	SourceRequest
	CachePrepare
	CacheReuse
)

func (s Stage) name() string {
	switch s {
	case Setup:
		return "setup"
	case Settings:
		return "settings"
	case Resolve:
		return "resolve"
	case Probe:
		return "probe"
	case SourceCheck:
		return "source_check"
	case Lease:
		return "lease"
	case Library:
		return "mpv_library"
	case Initialize:
		return "mpv_initialize"
	case LoadReady:
		return "mpv_load_ready"
	case Commit:
		return "commit"
	case Unpause:
		return "unpause_command"
	case SourcePrepare:
		return "source_prepare"
	case SourceSnapshot:
		return "source_snapshot"
	case SourceScriptHit:
		return "source_script_hit"
	case SourceScriptPrepare:
		return "source_script_prepare"
	case SourceRequest:
		return "source_request"
	case CachePrepare:
		return "cache_prepare"
	case CacheReuse:
		return "cache_reuse"
	default:
		return ""
	}
}

type timing struct {
	Stage      string `json:"stage"`
	DurationMS int64  `json:"duration_ms"`
}
type entry struct {
	Time           string   `json:"time"`
	Sequence       uint64   `json:"sequence"`
	Operation      string   `json:"operation"`
	Result         string   `json:"result"`
	DurationMS     int64    `json:"duration_ms"`
	SincePrepareMS int64    `json:"since_prepare_ms"`
	Codec          string   `json:"codec,omitempty"`
	Quality        string   `json:"quality,omitempty"`
	Playing        bool     `json:"playing"`
	Buffering      bool     `json:"buffering"`
	Stages         []timing `json:"stages"`
}

type Diagnostics struct {
	directory string
	mu        sync.RWMutex
	closed    bool
	queue     chan entry
	done      chan struct{}
}

func New(directory string) *Diagnostics {
	d := &Diagnostics{directory: directory, queue: make(chan entry, 32), done: make(chan struct{})}
	go func() {
		defer close(d.done)
		for e := range d.queue {
			d.write(e)
		}
	}()
	return d
}
func (d *Diagnostics) enqueue(e entry) {
	if d == nil {
		return
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return
	}
	select {
	case d.queue <- e:
	default:
	} // Dropping diagnostics is safer than delaying audio.
}
func (d *Diagnostics) Close() {
	if d == nil {
		return
	}
	d.mu.Lock()
	if !d.closed {
		d.closed = true
		close(d.queue)
	}
	d.mu.Unlock()
	select {
	case <-d.done:
	case <-time.After(250 * time.Millisecond):
	}
}
func (d *Diagnostics) Warmup(stage Stage, started time.Time, err error) {
	if stage != Library && stage != SourcePrepare {
		return
	}
	elapsed := time.Since(started).Milliseconds()
	d.enqueue(entry{Time: time.Now().UTC().Format(time.RFC3339Nano), Operation: "warmup", Result: outcome(err), DurationMS: elapsed, Stages: []timing{{stage.name(), elapsed}}})
}

func outcome(err error) string {
	switch {
	case err == nil:
		return "success"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "failed"
	}
}

type Trace struct {
	diagnostics    *Diagnostics
	started        time.Time
	sequence       uint64
	codec, quality string
	stages         []timing
	firstPlay      sync.Once
}

// A Trace's phases and format are collected by one prepare goroutine before
// handoff. Finish snapshots them; FirstPlay reads immutable fields and is
// concurrency-safe. No phase/format changes are allowed after handoff.
func Begin(d *Diagnostics, sequence uint64, started time.Time) *Trace {
	return &Trace{diagnostics: d, sequence: sequence, started: started}
}
func (t *Trace) Add(stage Stage, duration time.Duration) {
	if t == nil || stage.name() == "" || duration < 0 {
		return
	}
	t.stages = append(t.stages, timing{stage.name(), duration.Milliseconds()})
}
func (t *Trace) Since(stage Stage, started time.Time) {
	if t != nil {
		t.Add(stage, time.Since(started))
	}
}
func (t *Trace) Format(codec, quality string) {
	if t == nil {
		return
	}
	switch codec {
	case "flac", "mp3":
		t.codec = codec
	}
	switch quality {
	case "flac", "320k", "128k":
		t.quality = quality
	}
}
func (t *Trace) Finish(err error) {
	if t == nil {
		return
	}
	duration := time.Since(t.started).Milliseconds()
	t.diagnostics.enqueue(entry{Time: time.Now().UTC().Format(time.RFC3339Nano), Sequence: t.sequence,
		Operation: "prepare", Result: outcome(err), DurationMS: duration, SincePrepareMS: duration,
		Codec: t.codec, Quality: t.quality, Stages: append([]timing(nil), t.stages...)})
}

// FirstPlay measures the first unpause command acknowledgement, NOT physical
// device output or audible startup. Normal pause/resume does not generate logs.
func (t *Trace) FirstPlay(started time.Time, err error, playing, buffering bool) {
	if t == nil {
		return
	}
	t.firstPlay.Do(func() {
		elapsed := time.Since(started).Milliseconds()
		t.diagnostics.enqueue(entry{Time: time.Now().UTC().Format(time.RFC3339Nano), Sequence: t.sequence,
			Operation: "first_unpause", Result: outcome(err), DurationMS: elapsed, SincePrepareMS: time.Since(t.started).Milliseconds(),
			Codec: t.codec, Quality: t.quality, Playing: playing, Buffering: buffering, Stages: []timing{{"unpause_command", elapsed}}})
	})
}

func (d *Diagnostics) write(e entry) {
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	line = append(line, '\n')
	if os.MkdirAll(d.directory, 0700) != nil {
		return
	}
	directory, err := os.Lstat(d.directory)
	if err != nil || !directory.IsDir() || directory.Mode()&os.ModeSymlink != 0 || os.Chmod(d.directory, 0700) != nil {
		return
	}
	root, err := os.OpenRoot(d.directory)
	if err != nil {
		return
	}
	defer root.Close()
	const name = "playback.jsonl"
	info, err := root.Lstat(name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
	if err == nil {
		if !info.Mode().IsRegular() {
			return
		}
		if info.Size()+int64(len(line)) > limit {
			if root.Rename(name, name+".1") != nil {
				return
			}
			info = nil
		}
	}
	flags := os.O_WRONLY | os.O_APPEND
	if info == nil {
		flags |= os.O_CREATE | os.O_EXCL
	}
	file, err := root.OpenFile(name, flags, 0600)
	if err != nil {
		return
	}
	defer file.Close()
	current, err := file.Stat()
	if err != nil || !current.Mode().IsRegular() || info != nil && !os.SameFile(info, current) || file.Chmod(0600) != nil {
		return
	}
	_, _ = file.Write(line)
}
