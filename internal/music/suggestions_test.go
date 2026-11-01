package music

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestSuggestionsAnonymousProtocolAndSongsOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Query().Get("key") != "只想" || r.URL.Query().Get("format") != "json" || r.Header.Get("Referer") != "https://y.qq.com/" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected suggestion request")
		}
		fmt.Fprint(w, `{"code":0,"data":{"song":{"itemlist":[{"mid":"Song1","name":"只想一生跟你走","singer":"张学友"},{"mid":"Song1","name":"重复"},{"mid":"bad/id","name":"非法"}]},"album":{"itemlist":[{"mid":"Album1","name":"专辑"}]}}}`)
	}))
	defer server.Close()
	client := newQQClient(server.Client().Transport, server.URL)
	client.suggestionEndpoint = server.URL
	items, err := client.Suggest(context.Background(), " 只想 ")
	if err != nil || len(items) != 1 || items[0].Source != "tx" || items[0].Title != "只想一生跟你走" || items[0].Artist != "张学友" {
		t.Fatalf("%+v %v", items, err)
	}
}
func TestSuggestionsFailuresAndBounds(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
		want       error
	}{
		{"missing code", `{"data":{}}`, 200, ErrResponse},
		{"restriction", `{"code":1}`, 200, ErrRestricted},
		{"missing data", `{"code":0}`, 200, ErrResponse},
		{"bad JSON", `not JSON`, 200, ErrResponse},
		{"too large", strings.Repeat("x", maxSuggestionBytes+1), 200, ErrResponse},
		{"rate limit", "", 429, ErrRateLimit},
		{"denied", "", 403, ErrRestricted},
		{"redirect", "", 302, ErrResponse},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "https://invalid.example/")
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			client := newQQClient(server.Client().Transport, server.URL)
			client.suggestionEndpoint = server.URL
			if _, err := client.Suggest(context.Background(), "歌曲"); !errors.Is(err, test.want) {
				t.Fatalf("got %v want %v", err, test.want)
			}
		})
	}
	items, err := decodeSuggestions([]byte(`{"code":0,"data":{"singer":{},"album":{}}}`))
	if err != nil || items == nil || len(items) != 0 {
		t.Fatal("empty suggestions are not a successful empty list")
	}
	var rows []string
	for i := 0; i < 20; i++ {
		rows = append(rows, fmt.Sprintf(`{"mid":"Song%d","name":"歌曲%d"}`, i, i))
	}
	items, err = decodeSuggestions([]byte(`{"code":0,"data":{"song":{"itemlist":[` + strings.Join(rows, ",") + `]}}}`))
	if err != nil || len(items) != 8 {
		t.Fatal("suggestions were not bounded")
	}
}
func TestSuggestionsInvalidAndCanceledNeverUseNetwork(t *testing.T) {
	client := NewQQClient()
	if _, err := client.Suggest(context.Background(), "  "); !errors.Is(err, ErrQuery) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Suggest(ctx, "歌曲"); !errors.Is(err, ErrCanceled) {
		t.Fatal(err)
	}
}
func TestLiveQQSuggestions(t *testing.T) {
	if os.Getenv("MIAOYIN_LIVE_QQ_SEARCH") != "1" {
		t.Skip("live QQ opt-in")
	}
	items, err := NewQQClient().Suggest(context.Background(), "只想")
	if err != nil || len(items) == 0 {
		t.Fatalf("%+v %v", items, err)
	}
	t.Logf("received %d song suggestions", len(items))
}
