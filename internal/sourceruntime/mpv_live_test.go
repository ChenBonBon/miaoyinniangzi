package sourceruntime

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"miaoyinniangzi/internal/mediastream"
	"miaoyinniangzi/internal/mpvaudio"
	"miaoyinniangzi/internal/music"
	"miaoyinniangzi/internal/sourcenetwork"
)

// Explicit opt-in: source calls may consume allowance. No URL, key, script or
// native diagnostics are logged. Playback stays muted for the entire test.
func TestProvidedLibmpvSeekLive(t *testing.T) {
	if os.Getenv("MIAOYIN_LIVE_MPV_SEEK") != "1" {
		t.Skip("explicit real source/libmpv streaming opt-in")
	}
	script, err := os.ReadFile(os.Getenv("MIAOYIN_SOURCE_TEST_PATH"))
	if err != nil {
		t.Fatal("cannot read provided source")
	}
	defer clear(script)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("missing worker executable")
	}
	for _, song := range []struct{ title, artist string }{{"偷心", "张学友"}, {"可惜没如果", "林俊杰"}} {
		t.Run(song.title, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			result, err := music.NewQQClient().Search(ctx, song.title+" "+song.artist, 1)
			cancel()
			if err != nil {
				t.Fatal("metadata search failed")
			}
			var mid string
			for _, candidate := range result.Songs {
				if candidate.Title == song.title && strings.Contains(candidate.Artist, song.artist) {
					mid = candidate.ID
					break
				}
			}
			if mid == "" {
				t.Fatal("matching song not found")
			}
			for _, quality := range []string{"flac", "320k", "128k"} {
				t.Run(quality, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
					defer cancel()
					resolved, err := (&ProcessInspector{Executable: executable}).Resolve(ctx, script, ResolveInput{SongMID: mid}, quality)
					if err != nil || resolved.Error != "" {
						t.Fatal("source resolution failed")
					}
					media, err := mediastream.New(sourcenetwork.New())
					if err != nil {
						t.Fatal("proxy failed")
					}
					defer media.Close()
					info, err := media.Probe(ctx, resolved.URL)
					if err != nil {
						t.Fatal("media probe failed")
					}
					local, _, err := media.InstallPrepared(resolved.URL, info)
					if err != nil {
						t.Fatal("media lease failed")
					}
					player, err := mpvaudio.Open(ctx, local, info.Codec)
					if err != nil {
						t.Fatal("libmpv open failed")
					}
					defer player.Close()
					state, err := player.Control("state", 0)
					if err != nil || state.Duration < 60 || !info.Seekable || !state.Seekable {
						t.Fatal("invalid duration or seekability")
					}
					for _, fraction := range []float64{.5, .98, .999, .1} {
						target := state.Duration * fraction
						actual, err := player.Control("seek", target)
						if err != nil || math.Abs(actual.Position-target) > .06 || actual.Playing || actual.Ended {
							t.Fatal("paused seek acknowledgement mismatch")
						}
					}
					if _, err = player.Control("play", 0); err != nil {
						t.Fatal("muted playback failed")
					}
					time.Sleep(150 * time.Millisecond)
					actual, err := player.Control("seek", state.Duration*.5)
					if err != nil || math.Abs(actual.Position-state.Duration*.5) > .06 || !actual.Playing {
						t.Fatal("playing seek acknowledgement mismatch")
					}
					if _, err = player.Control("pause", 0); err != nil {
						t.Fatal("pause failed")
					}
					t.Log("controlled proxy streaming; midpoint/98%/99.9%/backward and playing seek acknowledgements passed; muted")
				})
			}
		})
	}
}
