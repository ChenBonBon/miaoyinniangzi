package music

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func detailSong(index int) map[string]any {
	return map[string]any{"mid": fmt.Sprintf("Song%d", index), "title": "track", "interval": 181.25, "singer": []any{map[string]any{"mid": "Artist1", "name": "artist"}}, "album": map[string]any{"mid": "Album1", "name": "album"}}
}
func TestAlbumLoadsCompleteOriginalSequenceIncludingRepeatedSongs(t *testing.T) {
	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" {
			t.Error("anonymous request included cookies")
		}
		var body struct {
			Request struct {
				Method string `json:"method"`
				Param  struct {
					Begin int    `json:"begin"`
					Mid   string `json:"albumMid"`
				} `json:"param"`
			} `json:"req_1"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var data any
		if body.Request.Method == "GetAlbumDetail" {
			data = map[string]any{"basicInfo": map[string]any{"albumMid": "Album1", "albumName": "album", "publishDate": "2026-01-01"}, "singer": map[string]any{"singerList": []any{map[string]any{"mid": "Artist1", "name": "artist"}}}}
		} else {
			pages++
			offset := body.Request.Param.Begin
			list := []any{}
			for i := offset; i < min(offset+100, 101); i++ {
				index := i
				if index == 100 {
					index = 0
				}
				list = append(list, map[string]any{"songInfo": detailSong(index)})
			}
			data = map[string]any{"albumMid": "Album1", "totalNum": 101, "curBegin": offset, "songList": list}
		}
		_, _ = w.Write(discoveryBody(data))
	}))
	defer server.Close()
	result, err := newQQClient(server.Client().Transport, server.URL).Detail(context.Background(), "album", "Album1", 1)
	if err != nil || len(result.Songs) != 101 || pages != 2 || result.Songs[99].ID != "Song99" || result.Songs[100].ID != "Song0" || result.Songs[0].Duration != 181.25 || result.Artists[0].ID != "Artist1" {
		t.Fatal("album lost complete sequence, duplicate occurrence or fractional metadata")
	}
}
func TestAlbumRejectsIncompletePaginationAndInvalidIdentity(t *testing.T) {
	for _, mode := range []string{"short", "total", "offset", "identity"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Request struct {
					Method string `json:"method"`
				} `json:"req_1"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			data := map[string]any{"basicInfo": map[string]any{"albumMid": "Album1", "albumName": "album"}}
			if body.Request.Method != "GetAlbumDetail" {
				data = map[string]any{"albumMid": "Album1", "totalNum": 1, "curBegin": 0, "songList": []any{map[string]any{"songInfo": detailSong(1)}}}
				switch mode {
				case "short":
					data["songList"] = []any{}
				case "total":
					data["totalNum"] = 1001
				case "offset":
					data["curBegin"] = 1
				case "identity":
					data["albumMid"] = "Other"
				}
			}
			_, _ = w.Write(discoveryBody(data))
		}))
		_, err := newQQClient(server.Client().Transport, server.URL).Detail(context.Background(), "album", "Album1", 1)
		server.Close()
		if err == nil {
			t.Fatal("unsafe album response accepted")
		}
	}
}
func TestArtistHotSongsAndAlbumPaging(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Request struct {
				Method string `json:"method"`
				Param  struct {
					Begin int `json:"begin"`
					Sort  int `json:"sort"`
				} `json:"param"`
			} `json:"req_1"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		var data any
		if body.Request.Method == "get_singer_detail_info" {
			if body.Request.Param.Sort != 5 {
				t.Error("incorrect hot song ordering")
			}
			data = map[string]any{"singer_info": map[string]any{"mid": "Artist1", "name": "artist"}, "songlist": []any{detailSong(1)}}
		} else {
			if body.Request.Param.Begin != 20 {
				t.Error("wrong album offset")
			}
			data = map[string]any{"singerMid": "Artist1", "total": 21, "albumList": []any{map[string]any{"albumMid": "Album1", "albumName": "album", "publishDate": "2026-01-01"}}}
		}
		_, _ = w.Write(discoveryBody(data))
	}))
	defer server.Close()
	result, err := newQQClient(server.Client().Transport, server.URL).Detail(context.Background(), "artist", "Artist1", 2)
	if err != nil || len(result.Songs) != 1 || len(result.Albums) != 1 || result.HasMoreAlbums || result.AlbumPage != 2 || result.CoverURL == "" {
		t.Fatal("artist detail or album pagination failed")
	}
	for _, value := range []struct {
		kind, id string
		page     int
	}{{"other", "Artist1", 1}, {"artist", "../", 1}, {"album", "Album1", 2}, {"artist", "Artist1", 101}} {
		if ValidDetailSelection(value.kind, value.id, value.page) {
			t.Fatal("invalid selection allowed")
		}
	}
}
func TestLiveQQMusicDetails(t *testing.T) {
	if os.Getenv("MIAOYIN_LIVE_QQ_DETAILS") != "1" {
		t.Skip("explicit anonymous QQ metadata opt-in")
	}
	client := NewQQClient()
	ctx := context.Background()
	collection, err := client.DiscoveryCollection(ctx, "new", 5, "")
	if err != nil || len(collection.Songs) == 0 {
		t.Fatal("metadata seed unavailable")
	}
	seed := collection.Songs[0]
	credits, creditErr := client.SongCredits(ctx, seed.ID)
	if creditErr != nil || credits.ID != seed.ID || len(credits.Artists) == 0 {
		t.Fatal("real song credits unavailable")
	}
	if len(seed.Artists) == 0 {
		t.Fatal("metadata seed has no artist")
	}
	for _, request := range []struct {
		kind, id string
		page     int
	}{{"artist", seed.Artists[0].ID, 1}, {"artist", seed.Artists[0].ID, 2}, {"album", seed.AlbumMid, 1}} {
		detail, err := client.Detail(ctx, request.kind, request.id, request.page)
		if err != nil || detail.Title == "" || detail.CoverURL == "" {
			t.Fatal("real detail endpoint failed")
		}
	}
	t.Log("anonymous artist, album list pagination and complete album metadata passed; no identifying metadata logged")
}
