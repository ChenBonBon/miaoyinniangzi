package music

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDiscoveryHTTPAndProtocolRestrictions(t *testing.T) {
	for _, sample := range []struct {
		status int
		body   string
		want   error
	}{
		{403, "", ErrRestricted}, {429, "", ErrRateLimit}, {502, "", ErrResponse},
		{200, `{"code":0,"req_1":{"code":2001,"data":{}}}`, ErrRestricted},
		{200, `{"code":0,"req_1":{"code":0,"data":null}}`, ErrResponse},
		{200, strings.Repeat("x", maxResponseBytes+1), ErrResponse},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(sample.status)
			_, _ = w.Write([]byte(sample.body))
		}))
		_, err := newQQClient(server.Client().Transport, server.URL).DiscoveryCatalog(context.Background(), "chart")
		server.Close()
		if !errors.Is(err, sample.want) {
			t.Fatal("incorrect discovery failure classification")
		}
	}
}
func TestDiscoveryDoesNotFollowRedirects(t *testing.T) {
	var called atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called.Store(true) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer server.Close()
	_, err := newQQClient(server.Client().Transport, server.URL).DiscoveryCatalog(context.Background(), "chart")
	if !errors.Is(err, ErrResponse) || called.Load() {
		t.Fatal("discovery followed an unexpected redirect")
	}
}
