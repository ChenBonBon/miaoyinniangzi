package music

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestLiveQQSimilar(t *testing.T) {
	if os.Getenv("MIAOYIN_LIVE_QQ_SIMILAR") != "1" {
		t.Skip("explicit anonymous metadata opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	songs, err := NewQQClient().SimilarSongs(ctx, "97773")
	if err != nil || len(songs) == 0 {
		t.Fatal("anonymous similar metadata failed")
	}
	for _, song := range songs {
		if !ValidSongSeed(song.SongID) || song.AlbumMid == "" {
			t.Fatal("candidate identifiers missing")
		}
	}
	t.Logf("anonymous metadata candidates=%d; no music identifiers logged", len(songs))
}

func TestSimilarSongsUsesNumericSeedAndPrimaryTracksOnly(t *testing.T) {
	track := map[string]any{"id": 42, "mid": "TrackMid", "title": "Candidate", "singer": []any{map[string]any{"mid": "ArtistMid", "name": "Artist"}}, "album": map[string]any{"mid": "AlbumMid", "name": "Album"}, "file": map[string]any{"size_128mp3": 123}, "interval": 200}
	for _, format := range []string{"old", "new", "empty", "null", "restricted", "missing-track"} {
		t.Run(format, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Cookie") != "" {
					t.Error("credentials sent")
				}
				var request struct {
					Call struct {
						Module string `json:"module"`
						Method string `json:"method"`
						Param  struct {
							SongID json.Number `json:"songid"`
						} `json:"param"`
					} `json:"req_1"`
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil || request.Call.Param.SongID.String() != "100" || request.Call.Module != "music.recommend.TrackRelationServer" || request.Call.Method != "GetSimilarSongs" {
					t.Error("wrong anonymous protocol")
				}
				data := map[string]any{"retcode": 0, "vecSongNew": nil}
				switch format {
				case "old":
					data["vecSong"] = []any{map[string]any{"track": track}, map[string]any{"track": track}}
				case "new":
					data["vecSongNew"] = []any{map[string]any{"songs": []any{map[string]any{"track": track}}}}
				case "empty":
					data["vecSong"] = []any{}
				case "null":
					data["vecSong"] = nil
				case "restricted":
					data["retcode"] = 1
				case "missing-track":
					data["vecSong"] = []any{map[string]any{"album": track}}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_1": map[string]any{"code": 0, "data": data}})
			}))
			defer server.Close()
			client := NewQQClient()
			client.endpoint = server.URL
			client.http = server.Client()
			songs, err := client.SimilarSongs(context.Background(), "100")
			switch format {
			case "old", "new":
				if err != nil || len(songs) != 1 || songs[0].ID != "TrackMid" || songs[0].AlbumMid != "AlbumMid" || songs[0].SongID != "42" {
					t.Fatal("primary metadata normalization failed")
				}
			case "empty":
				if err != nil || songs == nil || len(songs) != 0 {
					t.Fatal("legitimate empty rejected")
				}
			case "restricted":
				if !errors.Is(err, ErrRestricted) {
					t.Fatal("restriction hidden")
				}
			default:
				if !errors.Is(err, ErrResponse) {
					t.Fatal("malformed result admitted")
				}
			}
		})
	}
	for _, id := range []string{"", "0", "-1", "01", "MID", "1.0", "9223372036854775808"} {
		if ValidSongSeed(id) {
			t.Fatal("invalid seed admitted")
		}
	}
}
