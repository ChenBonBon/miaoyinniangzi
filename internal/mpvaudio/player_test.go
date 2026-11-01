package mpvaudio

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeClient struct {
	options          map[string]string
	properties       map[string]string
	events           []event
	destroyed        bool
	rejectInitialize bool
	commands         [][]string
}

func fake() *fakeClient {
	return &fakeClient{options: map[string]string{}, properties: map[string]string{"duration": "6", "time-pos": "0", "pause": "yes", "seekable": "yes"}, events: []event{{id: 8}, {id: 21}}}
}
func (f *fakeClient) option(k, v string) bool { f.options[k] = v; return true }
func (f *fakeClient) initialize() bool        { return !f.rejectInitialize }
func (f *fakeClient) command(args ...string) bool {
	f.commands = append(f.commands, args)
	if args[0] == "seek" {
		f.properties["time-pos"] = args[1]
		f.events = append(f.events, event{id: 20}, event{id: 21})
	}
	if args[0] == "set" {
		f.properties[args[1]] = args[2]
	}
	return true
}
func (f *fakeClient) get(k string) string { return f.properties[k] }
func (f *fakeClient) poll() event {
	if len(f.events) == 0 {
		return event{}
	}
	e := f.events[0]
	f.events = f.events[1:]
	return e
}
func (f *fakeClient) destroy() { f.destroyed = true }
func newFake(t *testing.T) (*Player, *fakeClient) {
	t.Helper()
	f := fake()
	p, err := open(context.Background(), "http://127.0.0.1:12345/media/token", "flac", func(string) (client, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p, f
}

func TestOnlyOpaqueParentLoopbackAccepted(t *testing.T) {
	for _, address := range []string{"https://example.com/signed", "file:///tmp/song.flac", "http://localhost:1234/x", "http://127.0.0.1:0/x", "http://user@127.0.0.1:1234/x", "http://127.0.0.1:1234/x?key=value"} {
		_, err := open(context.Background(), address, "flac", func(string) (client, error) { t.Fatal("unsafe input reached library"); return nil, nil })
		if err == nil {
			t.Fatal("unsafe media accepted")
		}
	}
}
func TestControlsPauseEOFAndExactSeek(t *testing.T) {
	p, f := newFake(t)
	if f.options["config"] != "no" || f.options["load-scripts"] != "no" || f.options["demuxer-lavf-format"] != "flac" || f.options["volume"] != "0" {
		t.Fatal("unsafe library defaults")
	}
	s, err := p.Control("seek", 6)
	if err != nil || s.Playing || s.Ended || s.Position >= 6 {
		t.Fatal("paused endpoint changed intent")
	}
	f.properties["eof-reached"] = "yes"
	s, _ = p.Control("state", 0)
	if s.Ended {
		t.Fatal("paused EOF advanced queue")
	}
	f.properties["eof-reached"] = "no"
	s, err = p.Control("play", 0)
	if err != nil || !s.Playing {
		t.Fatal("play failed")
	}
	f.properties["eof-reached"] = "yes"
	s, _ = p.Control("state", 0)
	if !s.Ended {
		t.Fatal("playing EOF not reported")
	}
	if _, err = p.Control("volume", math.NaN()); err == nil {
		t.Fatal("nonfinite volume accepted")
	}
	p.Close()
	if !f.destroyed {
		t.Fatal("client not closed")
	}
	if _, err = p.Control("state", 0); err == nil {
		t.Fatal("closed session accepted command")
	}
}
func TestSeekRequiresRestartAfterSeekEvent(t *testing.T) {
	p, f := newFake(t)
	f.events = []event{{id: 21}, {id: 20}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err := p.wait(ctx, true); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("accepted an old restart preceding the seek")
	}
	f.events = []event{{id: 20}, {id: 21}}
	if err := p.wait(context.Background(), true); err != nil {
		t.Fatal("fresh seek acknowledgement rejected")
	}
}

func TestFailedInitializationClosesClient(t *testing.T) {
	f := fake()
	f.rejectInitialize = true
	_, err := open(context.Background(), "http://127.0.0.1:1234/x", "mp3", func(string) (client, error) { return f, nil })
	if err != ErrPlayer || !f.destroyed {
		t.Fatal("failed initialization leaked client")
	}
}

func TestCancelledOpenAndMissingLibrary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := open(ctx, "http://127.0.0.1:1234/x", "mp3", func(string) (client, error) { t.Fatal("cancelled open created client"); return nil, nil }); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
	if _, err := open(context.Background(), "http://127.0.0.1:1234/x", "mp3", func(string) (client, error) { return nil, errors.New("private native diagnostic") }); err != ErrLibrary {
		t.Fatal("native diagnostic escaped")
	}
}

func TestLibmpvLoopbackPlaybackAndSeeking(t *testing.T) {
	if os.Getenv("MIAOYIN_MPV_SMOKE") != "1" {
		t.Skip("explicit libmpv/device smoke only; output starts muted")
	}
	// FFmpeg must not send the private loopback capability to an ambient proxy.
	for _, name := range []string{"http_proxy", "HTTP_PROXY"} {
		t.Setenv(name, "http://127.0.0.1:9")
	}
	for _, name := range []string{"no_proxy", "NO_PROXY"} {
		t.Setenv(name, "")
	}
	t.Run("flac", func(t *testing.T) { smokePlayer(t, "../../frontend/e2e/fixtures/tone.flac", "flac") })
	if directory := os.Getenv("MIAOYIN_MPV_MP3_TEST_DIR"); directory != "" {
		for _, quality := range []string{"320k", "128k"} {
			t.Run(quality, func(t *testing.T) { smokePlayer(t, filepath.Join(directory, quality+".mp3"), "mp3") })
		}
	}
}

func smokePlayer(t *testing.T, path, codec string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, path)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var measured []StartupStage
	p, err := OpenObserved(ctx, server.URL+"/media/test", codec, func(stage StartupStage, duration time.Duration) {
		if duration < 0 {
			t.Error("negative observed duration")
		}
		measured = append(measured, stage)
	})
	if len(measured) != 3 || measured[0] != StartupLibrary || measured[1] != StartupInitialize || measured[2] != StartupLoadReady {
		t.Fatal("missing real startup phases")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	s, err := p.Control("state", 0)
	if err != nil || s.Duration < 5.9 || s.Playing {
		t.Fatalf("initial state: %+v %v", s, err)
	}
	s, err = p.Control("seek", s.Duration)
	if err != nil || s.Playing || s.Ended || s.Position < 5.9 {
		t.Fatalf("paused endpoint: %+v %v", s, err)
	}
	if _, err = p.Control("seek", 1); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Control("play", 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	s, err = p.Control("state", 0)
	if err != nil || !s.Playing || s.Position <= 1 {
		t.Fatalf("clock: %+v %v", s, err)
	}
	s, err = p.Control("seek", 4)
	if err != nil || math.Abs(s.Position-4) > .05 || !s.Playing {
		t.Fatalf("playing seek: %+v %v", s, err)
	}
	if _, err = p.Control("seek", 5.8); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s, err = p.Control("state", 0)
		if err != nil || s.Ended {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil || !s.Ended {
		t.Fatal("natural EOF not delivered to host queue")
	}
	if _, err = p.Control("seek", 0); err != nil {
		t.Fatal("repeat-one rewind failed")
	}
	if _, err = p.Control("play", 0); err != nil {
		t.Fatal("repeat-one resume failed")
	}
	_, _ = p.Control("pause", 0)
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done(); _, _ = p.Control("state", 0) }()
	p.Close()
	group.Wait()
}
