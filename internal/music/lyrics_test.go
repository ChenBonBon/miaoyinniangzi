package music

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestLRCMultipleStampsOffsetsAndStableGrouping(t *testing.T) {
	result, err := parseLyrics("[ti:ignored]\n[offset:100]\n[00:02.30][00:01.2]你好\n[00:01.20]第二行\n[00:04.000]\n")
	if err != nil || len(result.Lines) != 3 || math.Abs(result.Lines[0].Time-1.1) > .000001 || result.Lines[0].Text != "你好\n第二行" || result.Lines[2].Text != "" {
		t.Fatalf("unexpected parsed lyric: %+v %v", result, err)
	}
	plain, err := parseLyrics("[ar:ignored]\n普通歌词\n下一行")
	if err != nil || plain.Text != "普通歌词\n下一行" || len(plain.Lines) != 0 {
		t.Fatal("untimed fallback")
	}
	if _, err = parseLyrics("[00:99.0]invalid"); err == nil {
		t.Fatal("invalid seconds accepted")
	}
}
func TestQQLyricsPublicRequestAndNoTranslation(t *testing.T) {
	for _, encoded := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("songmid") != "Song1" || r.Header.Get("Referer") != "https://y.qq.com/" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
				t.Error("unexpected request")
			}
			lyric := "[00:01.00]&lt;b&gt;原文&lt;/b&gt;"
			if encoded {
				lyric = base64.StdEncoding.EncodeToString([]byte(lyric))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "lyric": lyric, "trans": "SECRET_TRANSLATION"})
		}))
		client := newQQClient(server.Client().Transport, server.URL)
		client.lyricsEndpoint = server.URL
		value, err := client.Lyrics(context.Background(), "Song1")
		server.Close()
		if err != nil || value.SongID != "Song1" || len(value.Lines) != 1 || value.Lines[0].Text != "<b>原文</b>" {
			t.Fatalf("invalid lyrics: %+v %v", value, err)
		}
	}
}
func TestQQLyricsErrorsAndEmptyAreDifferent(t *testing.T) {
	for _, tc := range []struct {
		body string
		bad  bool
	}{{`{"code":0,"lyric":""}`, false}, {`{"code":-1}`, true}, {`{"lyric":"x"}`, true}, {`invalid`, true}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) }))
		client := newQQClient(server.Client().Transport, server.URL)
		client.lyricsEndpoint = server.URL
		_, err := client.Lyrics(context.Background(), "Song1")
		server.Close()
		if (err != nil) != tc.bad {
			t.Fatal("empty/error conflated")
		}
	}
}
func TestQQLyricsLive(t *testing.T) {
	if os.Getenv("MIAOYIN_LIVE_QQ_LYRICS") != "1" {
		t.Skip("explicit real QQ metadata opt-in")
	}
	value, err := NewQQClient().Lyrics(context.Background(), "0039MnYb0qxYhV")
	if err != nil || len(value.Lines) < 10 {
		t.Fatal("live timed lyrics unavailable")
	}
	t.Log("real QQ timestamped lyric response parsed; text not logged")
}
