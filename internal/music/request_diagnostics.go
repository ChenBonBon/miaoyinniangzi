package music

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http/httptrace"
	"sync"
	"time"
)

// Fixed enums and numeric observations only. Callback addresses, DNS names,
// certificates and raw errors are never copied into trace state or logs.
type requestTiming struct {
	Stage             string `json:"stage"`
	Route             string `json:"route"`
	Page              int    `json:"page,omitempty"`
	Failure           string `json:"failure,omitempty"`
	HTTPBudgetMS      int64  `json:"http_budget_ms"`
	ContextBudgetMS   *int64 `json:"context_budget_ms,omitempty"`
	AcquireMS         *int64 `json:"acquire_ms,omitempty"`
	DNSMS             *int64 `json:"dns_ms,omitempty"`
	ConnectMS         *int64 `json:"connect_ms,omitempty"`
	TLSMS             *int64 `json:"tls_ms,omitempty"`
	WriteMS           *int64 `json:"write_ms,omitempty"`
	FirstByteMS       *int64 `json:"first_byte_ms,omitempty"`
	FirstByteObserved bool   `json:"first_byte_observed"`
	BodyMS            *int64 `json:"body_ms,omitempty"`
	DecodeMS          *int64 `json:"decode_ms,omitempty"`
	Reused            *bool  `json:"reused,omitempty"`
	DNSAttempts       int    `json:"dns_attempts,omitempty"`
	ConnectAttempts   int    `json:"connect_attempts,omitempty"`
	ConnectFailures   int    `json:"connect_failures,omitempty"`
	ResponseBytes     int    `json:"response_bytes"`
}
type discoveryPageKey struct{}

func discoveryPage(ctx context.Context, page int) context.Context {
	return context.WithValue(ctx, discoveryPageKey{}, page)
}
func requestRoute(module, method string) string {
	switch {
	case module == "musicToplist.ToplistInfoServer" && method == "GetAll":
		return "chart_catalog"
	case module == "musicToplist.ToplistInfoServer" && method == "GetDetail":
		return "chart_page"
	case module == "newsong.NewSongServer" && method == "get_new_song_info":
		return "new_songs"
	default:
		return "other"
	}
}
func timingStage(value string) string {
	switch value {
	case "request", "connection_wait", "dns", "connect", "tls", "write", "first_byte", "headers", "body", "decode", "validate", "complete":
		return value
	default:
		return "other"
	}
}
func sanitizeTiming(value *requestTiming) *requestTiming {
	if value == nil {
		return nil
	}
	copy := *value
	copy.Stage = timingStage(copy.Stage)
	switch copy.Route {
	case "chart_catalog", "chart_page", "new_songs", "other":
	default:
		copy.Route = "other"
	}
	switch copy.Failure {
	case "", "canceled", "context_deadline", "transport_timeout", "dns", "connect", "tls", "write", "read", "other":
	default:
		copy.Failure = "other"
	}
	if copy.Page < 1 || copy.Page > 10 {
		copy.Page = 0
	}
	return &copy
}

type requestTrace struct {
	mu                                                                                         sync.Mutex
	metrics                                                                                    requestTiming
	started, phaseSince, acquireSince, dnsSince, connectSince, tlsSince, writeSince, waitSince time.Time
	finished                                                                                   bool
}

func msPointer(value int64) *int64 {
	if value < 0 {
		value = 0
	}
	return &value
}
func traceRequest(ctx context.Context, enabled bool, module, method string, budget time.Duration) *requestTrace {
	if !enabled {
		return nil
	}
	now := time.Now()
	trace := &requestTrace{started: now, phaseSince: now, metrics: requestTiming{Stage: "request", Route: requestRoute(module, method), HTTPBudgetMS: budget.Milliseconds()}}
	if ctx == nil {
		return trace
	}
	if end, ok := ctx.Deadline(); ok {
		trace.metrics.ContextBudgetMS = msPointer(time.Until(end).Milliseconds())
	}
	if page, ok := ctx.Value(discoveryPageKey{}).(int); ok && page >= 1 && page <= 10 {
		trace.metrics.Page = page
	}
	return trace
}
func (t *requestTrace) locked(update func()) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.finished {
		update()
	}
}
func (t *requestTrace) setStage(value string) { t.locked(func() { t.stageLocked(value) }) }
func (t *requestTrace) stageLocked(value string) {
	elapsed := time.Since(t.phaseSince).Milliseconds()
	switch t.metrics.Stage {
	case "body":
		old := int64(0)
		if t.metrics.BodyMS != nil {
			old = *t.metrics.BodyMS
		}
		t.metrics.BodyMS = msPointer(old + elapsed)
	case "decode":
		old := int64(0)
		if t.metrics.DecodeMS != nil {
			old = *t.metrics.DecodeMS
		}
		t.metrics.DecodeMS = msPointer(old + elapsed)
	}
	t.metrics.Stage = timingStage(value)
	t.phaseSince = time.Now()
}
func (t *requestTrace) context(ctx context.Context) context.Context {
	if t == nil || ctx == nil {
		return ctx
	}
	hooks := &httptrace.ClientTrace{
		GetConn: func(string) { t.locked(func() { t.acquireSince = time.Now(); t.stageLocked("connection_wait") }) },
		DNSStart: func(httptrace.DNSStartInfo) {
			t.locked(func() { t.metrics.DNSAttempts++; t.dnsSince = time.Now(); t.stageLocked("dns") })
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			t.locked(func() {
				if !t.dnsSince.IsZero() {
					t.metrics.DNSMS = msPointer(time.Since(t.dnsSince).Milliseconds())
				}
				if info.Err == nil && t.metrics.Stage == "dns" {
					t.stageLocked("connection_wait")
				}
			})
		},
		ConnectStart: func(string, string) {
			t.locked(func() {
				t.metrics.ConnectAttempts++
				if t.connectSince.IsZero() {
					t.connectSince = time.Now()
				}
				t.stageLocked("connect")
			})
		},
		ConnectDone: func(_ string, _ string, err error) {
			t.locked(func() {
				if !t.connectSince.IsZero() {
					t.metrics.ConnectMS = msPointer(time.Since(t.connectSince).Milliseconds())
				}
				if err != nil {
					t.metrics.ConnectFailures++
				} else if t.metrics.Stage == "connect" {
					t.stageLocked("connection_wait")
				}
			})
		},
		TLSHandshakeStart: func() { t.locked(func() { t.tlsSince = time.Now(); t.stageLocked("tls") }) },
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			t.locked(func() {
				if !t.tlsSince.IsZero() {
					t.metrics.TLSMS = msPointer(time.Since(t.tlsSince).Milliseconds())
				}
				if err == nil && t.metrics.Stage == "tls" {
					t.stageLocked("connection_wait")
				}
			})
		},
		GotConn: func(info httptrace.GotConnInfo) {
			t.locked(func() {
				reused := info.Reused
				t.metrics.Reused = &reused
				if !t.acquireSince.IsZero() {
					t.metrics.AcquireMS = msPointer(time.Since(t.acquireSince).Milliseconds())
				}
				t.writeSince = time.Now()
				t.stageLocked("write")
			})
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			t.locked(func() {
				if !t.writeSince.IsZero() {
					t.metrics.WriteMS = msPointer(time.Since(t.writeSince).Milliseconds())
				}
				if info.Err == nil {
					t.waitSince = time.Now()
					if !t.metrics.FirstByteObserved {
						t.stageLocked("first_byte")
					}
				}
			})
		},
		GotFirstResponseByte: func() {
			t.locked(func() {
				t.metrics.FirstByteObserved = true
				start := t.waitSince
				if start.IsZero() {
					start = t.started
				}
				t.metrics.FirstByteMS = msPointer(time.Since(start).Milliseconds())
				t.stageLocked("headers")
			})
		},
	}
	return httptrace.WithClientTrace(ctx, hooks)
}
func (t *requestTrace) bodyBytes(count int) { t.locked(func() { t.metrics.ResponseBytes = count }) }
func (t *requestTrace) networkFailure(ctx context.Context, err error) {
	t.locked(func() {
		var dns *net.DNSError
		var network net.Error
		switch {
		case errors.Is(ctx.Err(), context.Canceled):
			t.metrics.Failure = "canceled"
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			t.metrics.Failure = "context_deadline"
		case errors.As(err, &network) && network.Timeout():
			t.metrics.Failure = "transport_timeout"
		case errors.As(err, &dns):
			t.metrics.Failure = "dns"
		case t.metrics.Stage == "tls":
			t.metrics.Failure = "tls"
		case t.metrics.Stage == "connect":
			t.metrics.Failure = "connect"
		case t.metrics.Stage == "write":
			t.metrics.Failure = "write"
		case t.metrics.Stage == "body":
			t.metrics.Failure = "read"
		default:
			t.metrics.Failure = "other"
		}
	})
}
func (t *requestTrace) finish(err error) *requestTiming {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.finished {
		stage := t.metrics.Stage
		if !t.acquireSince.IsZero() && t.metrics.Reused == nil {
			t.metrics.AcquireMS = msPointer(time.Since(t.acquireSince).Milliseconds())
		}
		// Include in-progress spans when a timeout prevents the matching Done hook.
		switch stage {
		case "dns":
			if !t.dnsSince.IsZero() {
				t.metrics.DNSMS = msPointer(time.Since(t.dnsSince).Milliseconds())
			}
		case "connect":
			if !t.connectSince.IsZero() {
				t.metrics.ConnectMS = msPointer(time.Since(t.connectSince).Milliseconds())
			}
		case "tls":
			if !t.tlsSince.IsZero() {
				t.metrics.TLSMS = msPointer(time.Since(t.tlsSince).Milliseconds())
			}
		case "connection_wait":
			if !t.acquireSince.IsZero() {
				t.metrics.AcquireMS = msPointer(time.Since(t.acquireSince).Milliseconds())
			}
		case "write":
			if !t.writeSince.IsZero() {
				t.metrics.WriteMS = msPointer(time.Since(t.writeSince).Milliseconds())
			}
		case "first_byte":
			if !t.waitSince.IsZero() {
				t.metrics.FirstByteMS = msPointer(time.Since(t.waitSince).Milliseconds())
			}
		}
		t.stageLocked(stage)
		if err == nil {
			t.metrics.Stage = "complete"
		}
		t.finished = true
	}
	return sanitizeTiming(&t.metrics)
}
