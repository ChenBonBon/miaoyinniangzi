//go:build source_live

package sourceruntime

import (
	"context"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"miaoyinniangzi/internal/mediastream"
	"miaoyinniangzi/internal/music"
	"miaoyinniangzi/internal/nativeaudio"
	"miaoyinniangzi/internal/sourcenetwork"
)

func TestProvidedFLACNativeSeekLive(t *testing.T) {
	if os.Getenv("MIAOYIN_LIVE_FLAC_SEEK") != "1" {
		t.Skip("explicit source/full temporary FLAC cache opt-in")
	}
	path := os.Getenv("MIAOYIN_SOURCE_TEST_PATH")
	if path == "" {
		t.Fatal("missing source")
	}
	script, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("cannot read source")
	}
	defer clear(script)
	executable, _ := os.Executable()
	for index, test := range []struct{ title, artist string }{{"偷心", "张学友"}, {"可惜没如果", "林俊杰"}} {
		t.Run("fixture-"+strconv.Itoa(index+1), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			result, err := music.NewQQClient().Search(ctx, test.title+" "+test.artist, 1)
			if err != nil {
				t.Fatal("metadata search failed")
			}
			var mid string
			for _, song := range result.Songs {
				if song.Title == test.title && strings.Contains(song.Artist, test.artist) {
					mid = song.ID
					break
				}
			}
			if mid == "" {
				t.Fatal("matching song not found")
			}
			resolved, err := (&ProcessInspector{Executable: executable}).Resolve(ctx, script, ResolveInput{SongMID: mid}, "flac")
			if err != nil || resolved.Error != "" {
				t.Fatal("FLAC resolution failed")
			}
			media, err := mediastream.New(sourcenetwork.New())
			if err != nil {
				t.Fatal(err)
			}
			defer media.Close()
			cache, cleanup, err := media.CacheFLAC(ctx, resolved.URL)
			if err != nil {
				t.Fatal("FLAC cache failed")
			}
			defer cleanup()
			player, err := nativeaudio.Open(cache)
			if err != nil {
				t.Fatal(err)
			}
			defer player.Close()
			state, err := player.Control("state", 0)
			if err != nil || state.Duration < 60 {
				t.Fatal("invalid native duration")
			}
			for _, fraction := range []float64{.5, .98, .999} {
				target := state.Duration * fraction
				s, err := player.Control("seek", target)
				if err != nil || math.Abs(s.Position-target) > .001 || s.Playing {
					t.Fatal("sample-frame seek mismatch")
				}
			}
			player.Control("volume", 0)
			player.Control("play", 0)
			time.Sleep(60 * time.Millisecond)
			player.Control("pause", 0)
			t.Log("real FLAC cached privately; midpoint/98%/99.9% native frame positioning passed; output muted")
		})
	}
}
