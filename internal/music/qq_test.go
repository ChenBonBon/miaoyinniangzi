package music

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const fixtureSong = `{"id":97773,"mid":"0039MnYb0qxYhV","title":"晴天","subtitle":"录音室版","interval":269,"album":{"mid":"000MkMni19ClKG","name":"叶惠美"},"singer":[{"mid":"0025NhlN2yWrP4","name":"周杰伦"},{"mid":"OtherSinger","name":"合作歌手"}],"file":{"media_mid":"003Qui1q2u1Zho","size_flac":55,"size_320mp3":10,"size_128mp3":4,"size_hires":500},"title_hilight":"<em>ignored</em>"}`

func fixtureResponse(songs string, meta string) string {
	return `{"code":0,"req_1":{"code":0,"data":{"code":0,"body":{"song":{"list":` + songs + `}},"meta":` + meta + `}}}`
}
func goodResponse() string {
	return fixtureResponse(`[`+fixtureSong+`]`, `{"curpage":1,"sum":40,"nextpage":2,"ret":0,"is_filter":0}`)
}
func TestQQRequestAndNormalization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Referer") != "https://y.qq.com/" || r.Header.Get("Cookie") != "" {
			t.Error("unexpected request")
		}
		var payload struct {
			Comm    map[string]any `json:"comm"`
			Request struct {
				Module, Method string
				Param          map[string]any `json:"param"`
			} `json:"req_1"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.Comm["ct"] != "19" || payload.Comm["uin"] != "0" || payload.Request.Module != "music.search.SearchCgiService" || payload.Request.Method != "DoSearchForQQMusicDesktop" || payload.Request.Param["search_type"] != float64(0) || payload.Request.Param["query"] != "周杰伦" || payload.Request.Param["num_per_page"] != float64(20) {
			t.Errorf("incorrect search parameters: %+v", payload)
		}
		id, _ := payload.Request.Param["searchid"].(string)
		if len(id) != 37 {
			t.Error("missing anonymous search session")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(goodResponse()))
	}))
	defer server.Close()
	client := newQQClient(server.Client().Transport, server.URL)
	result, err := client.Search(context.Background(), " 周杰伦 ", 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Query != "周杰伦" || !result.HasMore || result.Total != 40 || len(result.Songs) != 1 {
		t.Fatalf("result: %+v", result)
	}
	song := result.Songs[0]
	if song.ID != "0039MnYb0qxYhV" || song.Source != "tx" || song.SongID != "97773" || song.MediaMid != "003Qui1q2u1Zho" || song.Title != "晴天" || song.Artist != "周杰伦 / 合作歌手" || song.AlbumMid != "000MkMni19ClKG" || song.Duration != 269 || len(song.Artists) != 2 || strings.Join(song.AvailableQualities, ",") != "flac,320k,128k" {
		t.Fatalf("song: %+v", song)
	}
	if !strings.HasPrefix(song.CoverURL, "https://y.gtimg.cn/music/photo_new/") {
		t.Fatal("unsafe cover URL")
	}
}
func TestValidationMakesNoRequests(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	client := newQQClient(server.Client().Transport, server.URL)
	for _, tc := range []struct {
		query string
		page  int
		want  error
	}{{"", 1, ErrQuery}, {"  ", 1, ErrQuery}, {strings.Repeat("歌", 101), 1, ErrQuery}, {"歌\x00名", 1, ErrQuery}, {"歌", 0, ErrPage}, {"歌", 101, ErrPage}, {string([]byte{0xff}), 1, ErrQuery}} {
		if _, err := client.Search(context.Background(), tc.query, tc.page); !errors.Is(err, tc.want) {
			t.Errorf("expected %v, got %v", tc.want, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid input reached QQ")
	}
}
func TestEmptyFilteredMalformedAndPagination(t *testing.T) {
	cases := []struct {
		name, body string
		want       error
		count      int
		more       bool
	}{
		{"empty", fixtureResponse(`[]`, `{"curpage":1,"sum":0,"nextpage":-1}`), nil, 0, false},
		{"filtered is not empty", fixtureResponse(`[]`, `{"curpage":1,"sum":0,"is_filter":-2}`), ErrRestricted, 0, false},
		{"malformed", "<html>gateway error</html>", ErrResponse, 0, false},
		{"missing data", `{"code":0,"req_1":{"code":0,"data":{}}}`, ErrResponse, 0, false},
		{"missing code", `{"req_1":{"data":{}}}`, ErrResponse, 0, false},
		{"duplicate", fixtureResponse(`[`+fixtureSong+`,`+fixtureSong+`]`, `{"curpage":1,"sum":40,"nextpage":2}`), nil, 1, true},
		{"last page", fixtureResponse(`[`+fixtureSong+`]`, `{"curpage":1,"sum":1,"nextpage":-1}`), nil, 1, false},
		{"wrong page", fixtureResponse(`[`+fixtureSong+`]`, `{"curpage":2,"sum":40,"nextpage":3}`), ErrResponse, 0, false},
		{"invalid songs", fixtureResponse(`[{"mid":"../unsafe","title":"bad"}]`, `{"curpage":1,"sum":1}`), ErrResponse, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := decodeSearch([]byte(tc.body), "关键词", 1)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if err == nil && (len(result.Songs) != tc.count || result.HasMore != tc.more || result.Songs == nil) {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}
func TestHTTPFailuresAndResponseLimit(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   error
	}{{429, "", ErrRateLimit}, {403, "", ErrRestricted}, {401, "", ErrRestricted}, {502, "", ErrResponse}, {200, strings.Repeat("x", maxResponseBytes+1), ErrResponse}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		client := newQQClient(server.Client().Transport, server.URL)
		_, err := client.Search(context.Background(), "关键词", 1)
		server.Close()
		if !errors.Is(err, tc.want) {
			t.Fatalf("status %d: %v", tc.status, err)
		}
	}
}
func TestRedirectsAreNotFollowed(t *testing.T) {
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Store(true) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer server.Close()
	_, err := newQQClient(server.Client().Transport, server.URL).Search(context.Background(), "关键词", 1)
	if !errors.Is(err, ErrResponse) || redirected.Load() {
		t.Fatal("followed unexpected redirect")
	}
}
func TestRequestTimeoutAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()
	client := newQQClient(server.Client().Transport, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := client.Search(ctx, "关键词", 1); !errors.Is(err, ErrTimeout) {
		t.Fatalf("timeout: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, err := client.Search(ctx, "关键词", 1); !errors.Is(err, ErrCanceled) {
		t.Fatalf("cancel: %v", err)
	}
}
func TestMissingMetadataIsSafe(t *testing.T) {
	var raw rawSong
	if err := json.Unmarshal([]byte(`{"mid":"Song1","name":"fallback","interval":-1,"album":{"mid":"../bad"},"singer":[{"mid":"Singer1","name":"歌手"}]}`), &raw); err != nil {
		t.Fatal(err)
	}
	song, ok := normalizeSong(raw)
	if !ok || song.Title != "fallback" || song.Duration != 0 || song.AlbumMid != "" || !strings.Contains(song.CoverURL, "T001") {
		t.Fatalf("normalization: %+v", song)
	}
}
func TestNumericIDsRemainExactAcrossJSON(t *testing.T) {
	song, ok := normalizeSong(rawSong{ID: 9007199254740993, Mid: "Song1", Title: "歌曲"})
	if !ok || song.SongID != "9007199254740993" {
		t.Fatal("lost numeric identifier precision")
	}
	data, err := json.Marshal(song)
	if err != nil || !strings.Contains(string(data), `"songId":"9007199254740993"`) {
		t.Fatal("identifier was not encoded as a string")
	}
}

func TestLiveQQSearch(t *testing.T) {
	if os.Getenv("MIAOYIN_LIVE_QQ_SEARCH") != "1" {
		t.Skip("live QQ search is opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := NewQQClient()
	result, err := client.Search(ctx, "周杰伦", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Songs) == 0 {
		t.Fatal("QQ returned no songs for smoke query")
	}
	for _, song := range result.Songs {
		if song.Source != "tx" {
			t.Fatal("non-QQ song")
		}
	}
	t.Logf("real QQ songs: %d; first title: %s; next page: %v", len(result.Songs), result.Songs[0].Title, result.HasMore)
	if result.HasMore {
		next, err := client.Search(ctx, "周杰伦", 2)
		if err != nil || next.Page != 2 || len(next.Songs) == 0 {
			t.Fatalf("real second page failed: %v", err)
		}
		t.Logf("real QQ second page: %d songs", len(next.Songs))
	}
}
