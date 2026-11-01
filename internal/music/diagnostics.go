package music

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

const diagnosticLimit = 64 * 1024

// Only fixed operation/result names and numeric protocol fields reach disk.
// Never pass request URLs, headers, queries, bodies or raw error messages here.
type diagnostic struct {
	Time        string `json:"time"`
	Operation   string `json:"operation"`
	DurationMS  int64  `json:"duration_ms"`
	HTTPStatus  int    `json:"http_status,omitempty"`
	Result      string `json:"result"`
	Code        *int   `json:"code,omitempty"`
	RequestCode *int   `json:"request_code,omitempty"`
	DataCode    *int   `json:"data_code,omitempty"`
	MetaRet     *int   `json:"meta_ret,omitempty"`
	Filter      *int   `json:"filter,omitempty"`
	Subcode     *int   `json:"subcode,omitempty"`
}

type Diagnostics struct {
	mu        sync.Mutex
	directory string
}

func NewDiagnostics(directory string) *Diagnostics { return &Diagnostics{directory: directory} }

// WithDiagnostics configures the client before its first request.
func (c *QQClient) WithDiagnostics(d *Diagnostics) *QQClient { c.diagnostics = d; return c }
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
	if event.Operation != "search" && event.Operation != "suggestions" && event.Operation != "discovery_catalog" && event.Operation != "discovery_collection" && event.Operation != "artist_detail" && event.Operation != "artist_albums" && event.Operation != "album_detail" && event.Operation != "song_credits" {
		return
	}
	event.Time = time.Now().UTC().Format(time.RFC3339Nano)
	event.DurationMS = time.Since(started).Milliseconds()
	event.Result = diagnosticResult(err)
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
	if os.Chmod(d.directory, 0700) != nil {
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
	if file.Chmod(0600) != nil {
		return
	}
	_, _ = file.Write(line)
}
