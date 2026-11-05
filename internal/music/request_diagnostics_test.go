package music

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type traceRoundTripper func(*http.Request) (*http.Response, error)

func (fn traceRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
func readRequestEvents(t *testing.T, dir string) []diagnostic {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "qq-metadata.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var events []diagnostic
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var event diagnostic
		if json.Unmarshal([]byte(line), &event) != nil {
			t.Fatal("invalid diagnostic")
		}
		events = append(events, event)
	}
	return events
}
func runDiscoveryDiagnostic(client *QQClient, ctx context.Context) error {
	var target map[string]any
	return client.discoveryRequest(ctx, "discovery_catalog", "musicToplist.ToplistInfoServer", "GetAll", map[string]any{}, &target)
}
func TestRequestTimingsConnectionReuseAndTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":0,"req_1":{"code":0,"data":{"fixture":true}}}`)
	}))
	defer server.Close()
	dir := t.TempDir()
	client := newQQClient(server.Client().Transport, server.URL).WithDiagnostics(NewDiagnostics(dir))
	for i := 0; i < 2; i++ {
		if err := runDiscoveryDiagnostic(client, context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	events := readRequestEvents(t, dir)
	if len(events) != 2 {
		t.Fatal("missing timing records")
	}
	first, second := events[0].Timing, events[1].Timing
	if first == nil || first.Stage != "complete" || first.Route != "chart_catalog" || first.Reused == nil || *first.Reused || first.AcquireMS == nil || first.TLSMS == nil || first.FirstByteMS == nil || !first.FirstByteObserved || first.BodyMS == nil || first.DecodeMS == nil || first.ResponseBytes <= 0 {
		t.Fatalf("missing first connection observations: %+v", first)
	}
	if second == nil || second.Reused == nil || !*second.Reused || second.TLSMS != nil || second.DNSMS != nil {
		t.Fatal("reused connection fabricated a DNS/TLS handshake")
	}
}
func TestRequestTimingsLocateHeaderAndBodyTimeouts(t *testing.T) {
	for _, body := range []bool{false, true} {
		t.Run(fmt.Sprintf("body-%t", body), func(t *testing.T) {
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if body {
					w.WriteHeader(200)
					fmt.Fprint(w, `{"partial":`)
					w.(http.Flusher).Flush()
				}
				<-release
			}))
			defer server.Close()
			defer close(release)
			dir := t.TempDir()
			client := newQQClient(server.Client().Transport, server.URL).WithDiagnostics(NewDiagnostics(dir))
			client.http.Timeout = 200 * time.Millisecond
			if !errors.Is(runDiscoveryDiagnostic(client, context.Background()), ErrTimeout) {
				t.Fatal("timeout was not preserved")
			}
			event := readDiagnostic(t, dir)
			timing := event.Timing
			expected := "first_byte"
			if body {
				expected = "body"
			}
			if event.Result != "timeout" || timing == nil || timing.Stage != expected || timing.Failure != "transport_timeout" || timing.HTTPBudgetMS != 200 {
				t.Fatalf("wrong timeout classification: %+v", timing)
			}
			if timing.FirstByteObserved != body {
				t.Fatal("first response-byte observation fabricated")
			}
			if body && (timing.ResponseBytes == 0 || timing.BodyMS == nil || event.HTTPStatus != 200) {
				t.Fatal("body progress missing")
			}
		})
	}
}
func TestRequestTimingsDistinguishContextBudgetAndPage(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer server.Close()
	defer close(release)
	dir := t.TempDir()
	client := newQQClient(server.Client().Transport, server.URL).WithDiagnostics(NewDiagnostics(dir))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var target map[string]any
	err := client.discoveryRequest(discoveryPage(ctx, 4), "discovery_collection", "musicToplist.ToplistInfoServer", "GetDetail", map[string]any{"topid": 987654, "period": "PERIOD-SECRET"}, &target)
	if !errors.Is(err, ErrTimeout) {
		t.Fatal("context timeout was not preserved")
	}
	timing := readDiagnostic(t, dir).Timing
	if timing == nil || timing.Page != 4 || timing.Route != "chart_page" || timing.ContextBudgetMS == nil || *timing.ContextBudgetMS > 200 || timing.Failure != "context_deadline" {
		t.Fatalf("root budget not distinguishable: %+v", timing)
	}
}
func TestRequestTimingsDNSFailureAndPrivacy(t *testing.T) {
	dir := t.TempDir()
	transport := traceRoundTripper(func(r *http.Request) (*http.Response, error) {
		hooks := httptrace.ContextClientTrace(r.Context())
		if hooks == nil {
			t.Fatal("missing trace")
		}
		hooks.GetConn("HOST-SECRET")
		hooks.DNSStart(httptrace.DNSStartInfo{Host: "DNS-SECRET"})
		failure := &net.DNSError{Err: "RAW-SECRET", Name: "DNS-SECRET"}
		hooks.DNSDone(httptrace.DNSDoneInfo{Err: failure})
		return nil, failure
	})
	client := newQQClient(transport, "https://example.invalid/URL-SECRET?key=KEY-SECRET").WithDiagnostics(NewDiagnostics(dir))
	_ = runDiscoveryDiagnostic(client, context.Background())
	event := readDiagnostic(t, dir)
	if event.Timing == nil || event.Timing.Stage != "dns" || event.Timing.Failure != "dns" || event.Timing.DNSMS == nil {
		t.Fatal("DNS classification missing")
	}
	NewDiagnostics(dir).record(diagnostic{Operation: "discovery_catalog", Timing: &requestTiming{Stage: "STAGE-SECRET", Route: "ROUTE-SECRET", Failure: "FAILURE-SECRET"}}, time.Now(), errors.New("ERROR-SECRET"))
	raw, err := os.ReadFile(filepath.Join(dir, "qq-metadata.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"HOST-SECRET", "DNS-SECRET", "RAW-SECRET", "URL-SECRET", "KEY-SECRET", "STAGE-SECRET", "ROUTE-SECRET", "FAILURE-SECRET", "ERROR-SECRET"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("private or non-allowlisted data reached diagnostics")
		}
	}
}
func TestRequestTimingsTLSFailureDoesNotExposeCertificatesOrErrors(t *testing.T) {
	dir := t.TempDir()
	transport := traceRoundTripper(func(r *http.Request) (*http.Response, error) {
		hooks := httptrace.ContextClientTrace(r.Context())
		hooks.GetConn("TLS-HOST-SECRET")
		hooks.TLSHandshakeStart()
		failure := errors.New("CERTIFICATE-SECRET")
		hooks.TLSHandshakeDone(tls.ConnectionState{}, failure)
		return nil, failure
	})
	client := newQQClient(transport, "https://example.invalid/TLS-URL-SECRET").WithDiagnostics(NewDiagnostics(dir))
	_ = runDiscoveryDiagnostic(client, context.Background())
	timing := readDiagnostic(t, dir).Timing
	if timing == nil || timing.Stage != "tls" || timing.Failure != "tls" || timing.TLSMS == nil {
		t.Fatal("TLS failure missing")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "qq-metadata.jsonl"))
	if strings.Contains(string(raw), "SECRET") {
		t.Fatal("TLS identity or raw error leaked")
	}
}
func TestRequestTraceCancellationAndMissingContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	trace := traceRequest(ctx, true, "", "", time.Second)
	trace.networkFailure(ctx, errors.New("ERROR-SECRET"))
	if value := trace.finish(ErrCanceled); value.Failure != "canceled" {
		t.Fatal("navigation cancellation confused with timeout")
	}
	trace = traceRequest(nil, true, "", "", time.Second)
	if trace.context(nil) != nil || trace.finish(ErrNetwork).ContextBudgetMS != nil {
		t.Fatal("missing context produced a fabricated budget")
	}
}
func TestRequestTraceCallbacksAreConcurrentAndFreezeAtCompletion(t *testing.T) {
	trace := traceRequest(context.Background(), true, "unknown-secret", "unknown-secret", 8*time.Second)
	hooks := httptrace.ContextClientTrace(trace.context(context.Background()))
	var workers sync.WaitGroup
	for i := 0; i < 20; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			hooks.ConnectStart("tcp", "ADDRESS-SECRET")
			hooks.ConnectDone("tcp", "ADDRESS-SECRET", errors.New("ERROR-SECRET"))
		}()
	}
	first := trace.finish(ErrNetwork)
	before, _ := json.Marshal(first)
	workers.Wait()
	hooks.DNSStart(httptrace.DNSStartInfo{Host: "LATE-SECRET"})
	after, _ := json.Marshal(trace.finish(nil))
	if string(before) != string(after) {
		t.Fatal("late callbacks mutated completed event")
	}
	if traceRequest(context.Background(), false, "", "", 0) != nil {
		t.Fatal("disabled diagnostics installed tracing")
	}
}
