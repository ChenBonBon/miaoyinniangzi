package music

import (
	"encoding/json"
	"errors"
	"miaoyinniangzi/internal/privatefs"
	"os"
	"sync"
	"time"
)

const diagnosticLimit = 64 * 1024

// Only fixed operation/result names and numeric protocol fields reach disk.
// Never pass request URLs, headers, queries, bodies or raw error messages here.
type sourceNetworkDiagnostic struct {
	Stage   string `json:"stage"`
	Answers string `json:"answers"`
	Reason  string `json:"reason"`
}

type diagnostic struct {
	Network     *sourceNetworkDiagnostic `json:"network,omitempty"`
	Timing      *requestTiming           `json:"timing,omitempty"`
	Time        string                   `json:"time"`
	Operation   string                   `json:"operation"`
	DurationMS  int64                    `json:"duration_ms"`
	HTTPStatus  int                      `json:"http_status,omitempty"`
	Result      string                   `json:"result"`
	Code        *int                     `json:"code,omitempty"`
	RequestCode *int                     `json:"request_code,omitempty"`
	DataCode    *int                     `json:"data_code,omitempty"`
	MetaRet     *int                     `json:"meta_ret,omitempty"`
	Filter      *int                     `json:"filter,omitempty"`
	Subcode     *int                     `json:"subcode,omitempty"`
}

type Diagnostics struct {
	mu        sync.Mutex
	directory string
}

func NewDiagnostics(directory string) *Diagnostics { return &Diagnostics{directory: directory} }

// WithDiagnostics configures the client before its first request.
func (c *QQClient) WithDiagnostics(d *Diagnostics) *QQClient { c.diagnostics = d; return c }

// RecordSourceNetwork accepts fixed classifications only, never request data.
func (d *Diagnostics) RecordSourceNetwork(stage, answers, reason string) {
	switch stage {
	case "request", "lookup", "fallback", "address_check", "media_url":
	default:
		return
	}
	switch answers {
	case "none", "invalid", "zoned", "fake_v4", "fake_v6", "non_public", "public_v4", "public_v6", "mixed":
	default:
		return
	}
	switch reason {
	case "success", "begin", "lookup_failed", "local_inspection_failed", "local_interface", "non_public_or_zone", "invalid_url", "request_shape", "request_build", "headers", "redirect", "scheme", "credentials", "fragment", "port", "host":
	default:
		return
	}
	var err error
	switch reason {
	case "lookup_failed":
		err = ErrNetwork
	case "success", "begin":
	default:
		err = ErrRestricted
	}
	d.record(diagnostic{Operation: "source_network", Network: &sourceNetworkDiagnostic{Stage: stage, Answers: answers, Reason: reason}}, time.Now(), err)
}
func diagnosticResult(err error) string {
	switch {
	case err == nil:
		return "success"
	case errors.Is(err, ErrRestricted):
		return "restricted"
	case errors.Is(err, ErrRateLimit):
		return "rate_limit"
	case errors.Is(err, ErrTimeout):
		return "timeout"
	case errors.Is(err, ErrCanceled):
		return "canceled"
	case errors.Is(err, ErrNetwork):
		return "network"
	case errors.Is(err, ErrResponse):
		return "invalid_response"
	default:
		return "other"
	}
}
func (d *Diagnostics) record(event diagnostic, started time.Time, err error) {
	if d == nil {
		return
	}
	if event.Operation != "search" && event.Operation != "suggestions" && event.Operation != "discovery_catalog" && event.Operation != "discovery_collection" && event.Operation != "artist_detail" && event.Operation != "artist_albums" && event.Operation != "album_detail" && event.Operation != "song_credits" && event.Operation != "similar_songs" && event.Operation != "source_network" {
		return
	}
	event.Time = time.Now().UTC().Format(time.RFC3339Nano)
	event.DurationMS = time.Since(started).Milliseconds()
	event.Result = diagnosticResult(err)
	event.Timing = sanitizeTiming(event.Timing)
	line, err := json.Marshal(event)
	if err != nil {
		return
	}
	line = append(line, '\n')
	d.mu.Lock()
	defer d.mu.Unlock()
	// Best effort: a broken diagnostics destination must not fail music requests.
	if os.MkdirAll(d.directory, 0700) != nil {
		return
	}
	info, err := os.Lstat(d.directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	if privatefs.Protect(d.directory, 0700) != nil {
		return
	}
	root, err := os.OpenRoot(d.directory)
	if err != nil {
		return
	}
	defer root.Close()
	const name = "qq-metadata.jsonl"
	info, err = root.Lstat(name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
	if err == nil {
		if !info.Mode().IsRegular() {
			return
		}
		if info.Size()+int64(len(line)) > diagnosticLimit {
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
	if err != nil || !current.Mode().IsRegular() || info != nil && !os.SameFile(info, current) {
		return
	}
	if privatefs.ProtectFile(file, 0600) != nil {
		return
	}
	_, _ = file.Write(line)
}
