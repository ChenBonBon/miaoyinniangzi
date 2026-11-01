// Package mediastream serves a single revocable, opaque playback capability on
// loopback. Signed source URLs stay in Go memory, never in webview or SQLite.
package mediastream

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const MaxMediaBytes int64 = 512 * 1024 * 1024

var ErrMedia = errors.New("无法读取音频，请重新解析后重试")
var ErrFormat = errors.New("音源返回了不支持的音频格式，仅支持普通 FLAC 和 MP3")

type Fetcher interface {
	OpenMedia(context.Context, string, string, string) (*http.Response, error)
}
type Info struct {
	Codec     string  `json:"codec"`
	Duration  float64 `json:"duration"`
	Size      int64   `json:"size"`
	Seekable  bool    `json:"seekable"`
	EntityTag string  `json:"-"`
}
type lease struct {
	token, remote string
	info          Info
	ctx           context.Context
	cancel        context.CancelFunc
	expires       time.Time
}
type Server struct {
	mu       sync.Mutex
	fetch    Fetcher
	listener net.Listener
	http     *http.Server
	current  *lease
	slots    chan struct{}
	address  string
}

func New(fetch Fetcher) (*Server, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, errors.New("无法启动本地音频通道")
	}
	s := &Server{fetch: fetch, listener: listener, address: listener.Addr().String(), slots: make(chan struct{}, 4)}
	s.http = &http.Server{Handler: s, ReadHeaderTimeout: 3 * time.Second, IdleTimeout: 15 * time.Second, WriteTimeout: 20 * time.Minute, MaxHeaderBytes: 16 * 1024, ErrorLog: log.New(io.Discard, "", 0)}
	go func() { _ = s.http.Serve(listener) }()
	return s, nil
}
func (s *Server) Close() error { s.Clear(); return s.http.Close() }
func (s *Server) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil {
		s.current.cancel()
		s.current = nil
	}
}
func (s *Server) Install(ctx context.Context, remote string) (string, Info, error) {
	info, err := s.Probe(ctx, remote)
	if err != nil {
		return "", Info{}, err
	}
	return s.InstallPrepared(remote, info)
}
func (s *Server) InstallPrepared(remote string, info Info) (string, Info, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", Info{}, ErrMedia
	}
	token := hex.EncodeToString(bytes)
	leaseCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.current != nil {
		s.current.cancel()
	}
	s.current = &lease{token: token, remote: remote, info: info, ctx: leaseCtx, cancel: cancel, expires: time.Now().Add(6 * time.Hour)}
	s.mu.Unlock()
	return "http://" + s.address + "/media/" + token, info, nil
}
func mediaSize(response *http.Response) (int64, error) {
	if response.ContentLength > MaxMediaBytes {
		return 0, ErrMedia
	}
	if response.StatusCode == 206 {
		value := response.Header.Get("Content-Range")
		parts := strings.Split(value, "/")
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "bytes ") {
			return 0, ErrMedia
		}
		total, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || total <= 0 || total > MaxMediaBytes {
			return 0, ErrMedia
		}
		bounds := strings.Split(strings.TrimPrefix(parts[0], "bytes "), "-")
		if len(bounds) != 2 {
			return 0, ErrMedia
		}
		start, e1 := strconv.ParseInt(bounds[0], 10, 64)
		end, e2 := strconv.ParseInt(bounds[1], 10, 64)
		if e1 != nil || e2 != nil || start < 0 || end < start || end >= total || response.ContentLength >= 0 && response.ContentLength != end-start+1 {
			return 0, ErrMedia
		}
		return total, nil
	}
	if response.ContentLength > MaxMediaBytes {
		return 0, ErrMedia
	}
	if response.ContentLength < 0 {
		return 0, nil
	}
	return response.ContentLength, nil
}
func (s *Server) Probe(ctx context.Context, remote string) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	response, err := s.fetch.OpenMedia(ctx, remote, "GET", "bytes=0-4095")
	if err != nil {
		return Info{}, ErrMedia
	}
	defer response.Body.Close()
	if response.StatusCode != 200 && response.StatusCode != 206 {
		return Info{}, ErrMedia
	}
	size, err := mediaSize(response)
	if err != nil {
		return Info{}, err
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return Info{}, ErrMedia
	}
	info := Info{Size: size, Seekable: response.StatusCode == 206 && size > 0, EntityTag: response.Header.Get("ETag")}
	if len(data) >= 42 && string(data[:4]) == "fLaC" && data[4]&0x7f == 0 && data[5] == 0 && data[6] == 0 && data[7] == 34 {
		packed := binary.BigEndian.Uint64(data[18:26])
		rate := packed >> 44
		samples := packed & ((1 << 36) - 1)
		if rate == 0 {
			return Info{}, ErrFormat
		}
		info.Codec = "flac"
		info.Duration = float64(samples) / float64(rate)
		return info, nil
	}
	if len(data) >= 10 && string(data[:3]) == "ID3" {
		info.Codec = "mp3"
		return info, nil
	}
	for i := 0; i+4 <= len(data) && i < 1024; i++ {
		if data[i] == 0xff && data[i+1]&0xe0 == 0xe0 && data[i+1]&0x06 == 0x02 && data[i+2]&0xf0 != 0 && data[i+2]&0xf0 != 0xf0 {
			info.Codec = "mp3"
			return info, nil
		}
	}
	return Info{}, ErrFormat
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Host != s.address || r.URL.RawQuery != "" {
		http.Error(w, "forbidden", 403)
		return
	}
	s.mu.Lock()
	current := s.current
	valid := current != nil && time.Now().Before(current.expires) && strings.HasPrefix(r.URL.Path, "/media/") && subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(r.URL.Path, "/media/")), []byte(current.token)) == 1
	s.mu.Unlock()
	if !valid {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")
	w.Header().Set("Access-Control-Allow-Private-Network", "true")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		w.WriteHeader(429)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Minute)
	defer cancel()
	stop := context.AfterFunc(current.ctx, cancel)
	defer stop()
	stopWrite := context.AfterFunc(ctx, func() { _ = http.NewResponseController(w).SetWriteDeadline(time.Now()) })
	defer stopWrite()
	if current.ctx.Err() != nil {
		w.WriteHeader(410)
		return
	}
	response, err := s.fetch.OpenMedia(ctx, current.remote, r.Method, r.Header.Get("Range"))
	if err != nil {
		http.Error(w, "audio unavailable", 502)
		return
	}
	defer response.Body.Close()
	if response.StatusCode == 416 {
		if current.info.Size > 0 {
			w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(current.info.Size, 10))
		}
		w.WriteHeader(416)
		return
	}
	size, err := mediaSize(response)
	if err != nil || current.info.Size > 0 && size > 0 && current.info.Size != size {
		w.WriteHeader(502)
		return
	}
	if response.StatusCode == 206 {
		w.Header().Set("Content-Range", response.Header.Get("Content-Range"))
	}
	if response.ContentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	}
	if current.info.Seekable {
		w.Header().Set("Accept-Ranges", "bytes")
	}
	mime := "audio/mpeg"
	if current.info.Codec == "flac" {
		mime = "audio/flac"
	}
	w.Header().Set("Content-Type", mime)
	w.WriteHeader(response.StatusCode)
	if r.Method == "HEAD" {
		return
	}
	_, _ = io.CopyBuffer(w, io.LimitReader(response.Body, MaxMediaBytes), make([]byte, 32*1024))
}
