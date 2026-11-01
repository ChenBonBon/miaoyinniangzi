package mediastream

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

type fixtureFetcher struct{ data []byte }

func (f fixtureFetcher) OpenMedia(ctx context.Context, raw, method, byteRange string) (*http.Response, error) {
	request := httptest.NewRequest(method, "https://fixture.example/audio", nil).WithContext(ctx)
	if byteRange != "" {
		request.Header.Set("Range", byteRange)
	}
	recorder := httptest.NewRecorder()
	http.ServeContent(recorder, request, "tone.flac", time.Time{}, bytes.NewReader(f.data))
	return recorder.Result(), nil
}
func fixture(t *testing.T) (*Server, string) {
	t.Helper()
	data, err := os.ReadFile("../../frontend/e2e/fixtures/tone.flac")
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(fixtureFetcher{data: data})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	url, info, err := server.Install(context.Background(), "https://source.example/?SECRET=token")
	if err != nil || info.Codec != "flac" || info.Duration < 5.9 || !info.Seekable {
		t.Fatalf("bad probe %+v %v", info, err)
	}
	return server, url
}
func TestOpaqueMediaRangeAndRevocation(t *testing.T) {
	server, url := fixture(t)
	if strings.Contains(url, "SECRET") || strings.Contains(url, "source.example") {
		t.Fatal("source address leaked")
	}
	request, _ := http.NewRequest("GET", url, nil)
	request.Header.Set("Range", "bytes=100-199")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 206 || len(body) != 100 || response.Header.Get("Content-Type") != "audio/flac" || response.Header.Get("Content-Range") == "" {
		t.Fatal("range forwarding failed")
	}
	head, _ := http.NewRequest("HEAD", url, nil)
	response, err = http.DefaultClient.Do(head)
	if err != nil || response.StatusCode != 200 {
		t.Fatal("HEAD failed")
	}
	response.Body.Close()
	request, _ = http.NewRequest("GET", url, nil)
	request.Host = "evil.example"
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != 403 {
		t.Fatal("DNS rebinding Host accepted")
	}
	response.Body.Close()
	if _, _, err := server.Install(context.Background(), "https://other.example/file"); err != nil {
		t.Fatal(err)
	}
	response, _ = http.Get(url)
	if response.StatusCode != 404 {
		t.Fatal("old token not revoked")
	}
	response.Body.Close()
	server.Clear()
}
func TestNonAudioRejectedBeforePublishingCapability(t *testing.T) {
	server, err := New(fixtureFetcher{data: []byte(`<html>not audio SECRET</html>`)})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if _, _, err := server.Install(context.Background(), "https://source.example/"); err != ErrFormat {
		t.Fatal("non-audio published")
	}
}
func TestFLACTemporaryCacheIsPrivateAndRemoved(t *testing.T) {
	server, _ := fixture(t)
	path, cleanup, err := server.CacheFLAC(context.Background(), "https://fixture.example/SECRET")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 || info.Size() == 0 {
		t.Fatal("cache is not private")
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("cache not removed")
	}
}

func TestNativeWebKitFLACAndSeeking(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("MIAOYIN_WEBKIT_SMOKE") != "1" {
		t.Skip("explicit native WebKit opt-in")
	}
	_, url := fixture(t)
	cmd := exec.Command("swift", "testdata/wk-playback.swift", url)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("WebKit FLAC/seek smoke failed: %s", output)
	}
	if !bytes.Contains(output, []byte("WEBKIT_FLAC_SEEK_OK")) {
		t.Fatal(fmt.Sprintf("missing native result: %s", output))
	}
	t.Log("native WebKit: FLAC decoded, played muted, and sought through loopback proxy")
}
