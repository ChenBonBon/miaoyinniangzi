package music

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSongCreditsUsesExactSongIdentityAndAnonymousMetadataOnly(t *testing.T) {
	for _, valid := range []bool{true, false} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Cookie") != "" {
				t.Error("metadata included cookies")
			}
			track := detailSong(1)
			if !valid {
				track["mid"] = "Other"
			}
			_, _ = w.Write(discoveryBody(map[string]any{"track_info": track}))
		}))
		result, err := newQQClient(server.Client().Transport, server.URL).SongCredits(context.Background(), "Song1")
		server.Close()
		if valid && (err != nil || len(result.Artists) != 1 || result.Artists[0].ID != "Artist1" || result.AlbumMid != "Album1") {
			t.Fatal("song credits not recovered")
		}
		if !valid && err == nil {
			t.Fatal("wrong song identity accepted")
		}
	}
}
