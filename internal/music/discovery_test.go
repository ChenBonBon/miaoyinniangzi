package music

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func discoveryBody(data any) []byte {
	body, _ := json.Marshal(map[string]any{"code": 0, "req_1": map[string]any{"code": 0, "data": data}})
	return body
}
func TestDiscoveryCatalogAnonymousProtocolAndSafeImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Referer") != "https://y.qq.com/" {
			t.Error("non-anonymous metadata request")
		}
		var request struct {
			Comm  map[string]any `json:"comm"`
			Query struct {
				Module, Method string
				Param          map[string]any
			} `json:"req_1"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || request.Comm["uin"] != float64(0) || request.Query.Method != "GetAll" || request.Query.Module != "musicToplist.ToplistInfoServer" {
			t.Error("wrong catalog protocol")
		}
		_, _ = w.Write(discoveryBody(map[string]any{"group": []any{map[string]any{"groupId": 0, "groupName": "榜单分组", "toplist": []any{map[string]any{"topId": 1, "title": "真实榜单", "totalNum": 1, "period": "2026_43", "frontPicUrl": "https://y.gtimg.cn/music/example.jpg"}, map[string]any{"topId": 2, "title": "无封面榜单", "totalNum": 1, "period": "", "frontPicUrl": "http://127.0.0.1/private"}}}}}))
	}))
	defer server.Close()
	client := newQQClient(server.Client().Transport, server.URL)
	catalog, err := client.DiscoveryCatalog(context.Background(), "chart")
	if err != nil || len(catalog.Entries) != 2 || catalog.Entries[0].Group != "榜单分组" || catalog.Entries[0].GroupID == nil || *catalog.Entries[0].GroupID != 0 || catalog.Entries[1].CoverURL != "" {
		t.Fatal("catalog normalization failed")
	}
}
func TestWholeChartPaginationAndRanking(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Query struct {
				Param struct {
					ID     int    `json:"topid"`
					Offset int    `json:"offset"`
					Num    int    `json:"num"`
					Period string `json:"period"`
				} `json:"param"`
			} `json:"req_1"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("request invalid")
			w.WriteHeader(400)
			return
		}
		p := request.Query.Param
		if p.ID != 7 || p.Num != 100 || p.Period != "2026_43" {
			t.Error("unbounded or unpinned chart request")
		}
		calls++
		count := 205 - p.Offset
		if count > 100 {
			count = 100
		}
		songs := []any{}
		ranked := []any{}
		for i := 0; i < count; i++ {
			id := p.Offset + i + 1
			songs = append([]any{map[string]any{"id": id, "mid": fmt.Sprintf("Song%d", id), "title": "歌曲", "interval": 60}}, songs...)
			ranked = append(ranked, map[string]any{"songId": id})
		}
		_, _ = w.Write(discoveryBody(map[string]any{"data": map[string]any{"topId": 7, "title": "榜单", "period": "2026_43", "totalNum": 205, "song": ranked}, "songInfoList": songs}))
	}))
	defer server.Close()
	result, err := newQQClient(server.Client().Transport, server.URL).DiscoveryCollection(context.Background(), "chart", 7, "2026_43")
	if err != nil || calls != 3 || len(result.Songs) != 205 || result.Songs[0].ID != "Song1" || result.Songs[204].ID != "Song205" {
		t.Fatal("whole chart or rank order incorrect")
	}
}
func TestDiscoveryRejectsIncompleteOrChangedPeriod(t *testing.T) {
	for _, body := range []any{
		map[string]any{"data": map[string]any{"topId": 7, "title": "榜单", "period": "changed", "totalNum": 0, "song": []any{}}, "songInfoList": []any{}},
		map[string]any{"data": map[string]any{"topId": 7, "title": "榜单", "period": "2026_43", "totalNum": 10, "song": []any{}}, "songInfoList": []any{}},
		map[string]any{"data": map[string]any{"topId": 7, "title": "榜单", "period": "2026_43", "totalNum": 1001, "song": []any{}}, "songInfoList": []any{}},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(discoveryBody(body)) }))
		_, err := newQQClient(server.Client().Transport, server.URL).DiscoveryCollection(context.Background(), "chart", 7, "2026_43")
		server.Close()
		if !errors.Is(err, ErrResponse) {
			t.Fatal("incomplete or changed chart accepted")
		}
	}
}
func TestNewSongsCategoriesAndEmptyCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Query struct {
				Module, Method string
				Param          struct {
					Type int `json:"type"`
				} `json:"param"`
			} `json:"req_1"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.Query.Module != "newsong.NewSongServer" || request.Query.Method != "get_new_song_info" {
			t.Error("wrong new song protocol")
		}
		_, _ = w.Write(discoveryBody(map[string]any{"type": request.Query.Param.Type, "lan": "最新", "lanlist": []any{map[string]any{"type": 5, "lan": "最新"}, map[string]any{"type": 1, "lan": "内地"}}, "songlist": []any{}}))
	}))
	defer server.Close()
	client := newQQClient(server.Client().Transport, server.URL)
	catalog, err := client.DiscoveryCatalog(context.Background(), "new")
	if err != nil || len(catalog.Entries) != 2 {
		t.Fatal("new song catalog failed")
	}
	result, err := client.DiscoveryCollection(context.Background(), "new", 5, "")
	if err != nil || result.Songs == nil || len(result.Songs) != 0 {
		t.Fatal("valid empty collection not preserved")
	}
}
func TestDiscoveryParametersDoNotReachNetwork(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	client := newQQClient(server.Client().Transport, server.URL)
	for _, value := range []struct {
		kind   string
		id     int
		period string
	}{{"other", 1, ""}, {"chart", 0, ""}, {"new", 9, ""}, {"chart", 1, "../unsafe"}, {"new", 1, "2026_43"}} {
		if _, err := client.DiscoveryCollection(context.Background(), value.kind, value.id, value.period); err == nil {
			t.Fatal("invalid selection accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid parameters reached network")
	}
}
func TestLiveQQDiscovery(t *testing.T) {
	if os.Getenv("MIAOYIN_LIVE_QQ_DISCOVERY") != "1" {
		t.Skip("explicit anonymous QQ discovery metadata opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	client := NewQQClient()
	charts, err := client.DiscoveryCatalog(ctx, "chart")
	if err != nil || len(charts.Entries) == 0 {
		t.Fatal("real chart catalog failed")
	}
	entry := charts.Entries[0]
	if entry.CoverURL == "" {
		t.Fatal("real chart cover was discarded")
	}
	collection, err := client.DiscoveryCollection(ctx, entry.Kind, entry.ID, entry.Period)
	if err != nil || len(collection.Songs) == 0 || collection.Songs[0].CoverURL == "" {
		t.Fatal("real chart or song cover failed")
	}
	categories, err := client.DiscoveryCatalog(ctx, "new")
	if err != nil || len(categories.Entries) == 0 {
		t.Fatal("real new song catalog failed")
	}
	entry = categories.Entries[0]
	collection, err = client.DiscoveryCollection(ctx, entry.Kind, entry.ID, entry.Period)
	if err != nil || len(collection.Songs) == 0 {
		t.Fatal("real new song collection failed")
	}
	t.Log("real anonymous chart and new song metadata passed; no song metadata logged")
}
