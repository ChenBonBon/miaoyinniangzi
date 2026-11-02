package sourceruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"io"
	"miaoyinniangzi/internal/mediastream"
	"miaoyinniangzi/internal/sourcenetwork"
	"net/http"
)

const resolveFixture = `
const {on,send,request}=lx;
request('https://startup.example/update?key=API-SECRET',{method:'GET'},()=>{});
on('request',async ({action,source,info})=>{
 if(action!=='musicUrl'||source!=='tx')throw Error('SECRET');
 const response=await new Promise((resolve,reject)=>request('https://api.example/resolve',{method:'POST',headers:{'X-Key':'API-SECRET'},body:{id:info.musicInfo.songmid,quality:info.type}},(err,res)=>err?reject(err):resolve(res)));
 return response.body.url;
});
send('inited',{status:true,sources:{tx:{actions:['musicUrl'],qualitys:['flac','320k','128k']}}});`

type fakeNetwork struct {
	calls  []sourcenetwork.Request
	fail   bool
	unsafe bool
}

func (n *fakeNetwork) Do(_ context.Context, r sourcenetwork.Request) (sourcenetwork.Response, error) {
	n.calls = append(n.calls, r)
	if n.fail {
		return sourcenetwork.Response{}, errors.New("SECRET raw error")
	}
	return sourcenetwork.Response{Status: 200, Body: []byte(`{"code":200,"url":"https://cdn.example/audio.flac"}`)}, nil
}
func (n *fakeNetwork) ValidateMedia(_ context.Context, url string) error {
	if n.unsafe {
		return errors.New("blocked")
	}
	if url != "https://cdn.example/audio.flac" {
		return errors.New("invalid")
	}
	return nil
}

type liveResolveProbe struct {
	client *sourcenetwork.Client
	t      *testing.T
}

func (p *liveResolveProbe) Do(ctx context.Context, r sourcenetwork.Request) (sourcenetwork.Response, error) {
	response, err := p.client.Do(ctx, r)
	p.t.Logf("source HTTP: status=%d success=%t", response.Status, err == nil)
	return response, err
}
func (p *liveResolveProbe) ValidateMedia(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		p.t.Log("media URL parse failed")
	} else {
		scheme := "other"
		if u.Scheme == "http" || u.Scheme == "https" {
			scheme = u.Scheme
		}
		p.t.Logf("media scheme=%s default_port=%t", scheme, u.Port() == "")
	}
	err = p.client.ValidateMedia(ctx, raw)
	p.t.Logf("media validation success=%t", err == nil)
	return err
}
func TestProvidedSourceResolveLive(t *testing.T) {
	if os.Getenv("MIAOYIN_LIVE_SOURCE_RESOLVE") != "1" {
		t.Skip("explicit source-service opt-in")
	}
	path := os.Getenv("MIAOYIN_SOURCE_TEST_PATH")
	if path == "" {
		t.Fatal("missing explicit source file")
	}
	script, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("cannot read source")
	}
	defer clear(script)
	executable, _ := os.Executable()
	result, err := (&ProcessInspector{Executable: executable, Network: &liveResolveProbe{client: sourcenetwork.New(), t: t}}).Resolve(context.Background(), script, ResolveInput{SongMID: "0039MnYb0qxYhV"}, "flac")
	if err != nil {
		t.Fatalf("resolution stage failed: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("source resolution failed: %s", result.Error)
	}
	t.Logf("source resolution succeeded: quality=%s", result.Quality)
}

func TestProvidedSourcePlaybackProbeLive(t *testing.T) {
	if os.Getenv("MIAOYIN_LIVE_SOURCE_PLAYBACK") != "1" {
		t.Skip("explicit source/media opt-in")
	}
	path := os.Getenv("MIAOYIN_SOURCE_TEST_PATH")
	if path == "" {
		t.Fatal("missing explicit source file")
	}
	script, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("cannot read source")
	}
	defer clear(script)
	executable, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	resolved, err := (&ProcessInspector{Executable: executable}).Resolve(ctx, script, ResolveInput{SongMID: "0039MnYb0qxYhV"}, "flac")
	if err != nil || resolved.Error != "" {
		t.Fatal("live source resolution failed")
	}
	server, err := mediastream.New(sourcenetwork.New())
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	local, info, err := server.Install(ctx, resolved.URL)
	if err != nil {
		t.Fatalf("media probe failed: %v", err)
	}
	request, _ := http.NewRequestWithContext(ctx, "GET", local, nil)
	request.Header.Set("Range", "bytes=1000-1999")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal("loopback media failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1001))
	if err != nil || response.StatusCode != 206 || len(body) != 1000 {
		t.Fatal("live byte range failed")
	}
	t.Logf("live source/media passed: codec=%s seekable=%t; only probe/range bytes read", info.Codec, info.Seekable)
}

func TestProvidedSourceResolveOffline(t *testing.T) {
	path := os.Getenv("MIAOYIN_SOURCE_TEST_PATH")
	if path == "" {
		t.Skip("explicit local source opt-in")
	}
	script, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("cannot read provided source")
	}
	defer clear(script)
	executable, _ := os.Executable()
	network := &fakeNetwork{}
	result, err := (&ProcessInspector{Executable: executable, Network: network}).Resolve(context.Background(), script, ResolveInput{SongMID: "0039MnYb0qxYhV"}, "flac")
	if err != nil || result.Error != "" || result.Quality != "flac" || len(network.calls) != 1 {
		t.Fatal("provided source did not resolve with the offline fixture")
	}
	// No actual source service is contacted and no URL/header/script is printed.
	t.Log("provided source: offline FLAC handler passed")
}

func TestUnsupportedPlatformSourceWorkersRemainFailClosed(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("unsupported-platform contract")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	network := &fakeNetwork{}
	worker := &ProcessInspector{Executable: executable, Network: network}
	result, resolveErr := worker.Resolve(context.Background(), []byte(resolveFixture), ResolveInput{SongMID: "Song1"}, "flac")
	if result.URL != "" || len(network.calls) != 0 || (resolveErr == nil && result.Error == "") {
		t.Fatal("unsupported platform executed source or exposed media")
	}
	if sandboxWorker() == nil {
		t.Fatal("missing sandbox accepted")
	}
}

func TestResolveWorkerControlledNetworkAndMapping(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("requires implemented source-worker sandbox")
	}
	executable, _ := os.Executable()
	network := &fakeNetwork{}
	worker := &ProcessInspector{Executable: executable, Network: network}
	result, err := worker.Resolve(context.Background(), []byte(resolveFixture), ResolveInput{SongMID: "Song1", SongID: "123", Name: "歌曲"}, "flac")
	if err != nil || result.Error != "" || result.Quality != "flac" || result.URL == "" {
		t.Fatalf("resolution failed: %+v %v", result, err)
	}
	if len(network.calls) != 1 || network.calls[0].URL != "https://api.example/resolve" || network.calls[0].Headers["X-Key"] != "API-SECRET" {
		t.Fatal("startup/update was networked or request changed")
	}
	var body map[string]string
	if json.Unmarshal(network.calls[0].Body, &body) != nil || body["id"] != "Song1" || body["quality"] != "flac" {
		t.Fatal("wrong LX song/quality mapping")
	}
}
func TestResolveErrorsAreSanitizedAndNoNetworkRetry(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("requires implemented source-worker sandbox")
	}
	executable, _ := os.Executable()
	network := &fakeNetwork{fail: true}
	worker := &ProcessInspector{Executable: executable, Network: network}
	result, err := worker.Resolve(context.Background(), []byte(resolveFixture), ResolveInput{SongMID: "Song1"}, "flac")
	if err != nil || result.Error == "" || result.RetryLower || strings.Contains(result.Error, "SECRET") {
		t.Fatalf("unsafe failure %+v %v", result, err)
	}
	network.fail = false
	network.unsafe = true
	if _, err := worker.Resolve(context.Background(), []byte(resolveFixture), ResolveInput{SongMID: "Song1"}, "flac"); err == nil {
		t.Fatal("unsafe media reached caller")
	}
	for _, handler := range []string{`()=>Promise.reject(Error('SCRIPT-SECRET'))`, `()=>{require('fs');return 'secret'}`, `()=>{throw Error('SCRIPT-SECRET')}`} {
		script := `lx.on('request',` + handler + `);lx.send('inited',{status:true,sources:{tx:{actions:['musicUrl'],qualitys:['flac']}}});`
		result, err := worker.Resolve(context.Background(), []byte(script), ResolveInput{SongMID: "Song1"}, "flac")
		if err != nil || result.Error == "" || strings.Contains(result.Error, "SECRET") {
			t.Fatal("script exception leaked")
		}
	}
}
func TestResolveCancellationAndBusyLoop(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("requires implemented source-worker sandbox")
	}
	executable, _ := os.Executable()
	worker := &ProcessInspector{Executable: executable, Network: &fakeNetwork{}}
	script := `lx.on('request',()=>{while(true){}});lx.send('inited',{status:true,sources:{tx:{actions:['musicUrl'],qualitys:['flac']}}});`
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := worker.Resolve(ctx, []byte(script), ResolveInput{SongMID: "Song1"}, "flac")
	if err == nil || time.Since(started) > 2*time.Second {
		t.Fatal("worker cancellation failed")
	}
}
func TestResolverProtocolRejectsUntrustedFramesAndBoundsRequests(t *testing.T) {
	for _, frame := range []string{
		`{"kind":"result","result":{"url":"https://cdn.example/audio.flac","quality":"flac","error":"","retryLower":false},"unknown":"SECRET"}` + "\n",
		`{"kind":"result","result":{"url":"","quality":"","error":"SECRET exception","retryLower":false}}` + "\n",
		`{"kind":"result","result":{"url":"https://cdn.example/audio.flac","quality":"flac","error":"","retryLower":false}}` + "\ntrailing",
		strings.Repeat("x", 192*1024) + "\n",
	} {
		if _, err := exchangeResolve(context.Background(), &bytes.Buffer{}, strings.NewReader(frame), []byte(`{}`), &fakeNetwork{}, "flac"); err == nil {
			t.Fatal("unsafe worker output accepted")
		}
	}
	var frames strings.Builder
	for i := 0; i < 7; i++ {
		fmt.Fprintln(&frames, `{"kind":"http","request":{"url":"https://api.example","method":"GET","headers":{},"body":null}}`)
	}
	network := &fakeNetwork{}
	if _, err := exchangeResolve(context.Background(), &bytes.Buffer{}, strings.NewReader(frames.String()), []byte(`{}`), network, "flac"); err == nil || len(network.calls) != 6 {
		t.Fatal("request budget not enforced")
	}
}

func TestResolveTimerAndQualityRejection(t *testing.T) {
	job := resolveJob{Script: []byte(`setTimeout(()=>{lx.on('request',()=>new Promise(resolve=>setTimeout(()=>resolve('https://cdn.example/audio.flac'),1)));lx.send('inited',{status:true,sources:{tx:{actions:['musicUrl'],qualitys:['flac']}}})},1)`), Info: ResolveInput{SongMID: "Song1"}, Quality: "flac"}
	result := resolveScript(job, func(sourcenetwork.Request) (sourcenetwork.Response, error) {
		t.Fatal("unexpected network")
		return sourcenetwork.Response{}, nil
	})
	if result.Error != "" || result.Quality != "flac" {
		t.Fatalf("timer promise failed %+v", result)
	}
}
