package sourceruntime

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"miaoyinniangzi/internal/mediastream"
	"miaoyinniangzi/internal/music"
	"miaoyinniangzi/internal/songcache"
	"miaoyinniangzi/internal/sourcenetwork"
)

func TestProvidedThreeSlotCacheLive(t *testing.T) {
	if os.Getenv("MIAOYIN_LIVE_THREE_SLOT_CACHE") != "1" {
		t.Skip("explicit source quota/full media cache opt-in")
	}
	script, err := os.ReadFile(os.Getenv("MIAOYIN_SOURCE_TEST_PATH"))
	if err != nil {
		t.Fatal("provided source unavailable")
	}
	defer clear(script)
	quality := os.Getenv("MIAOYIN_CACHE_TEST_QUALITY")
	if quality == "" {
		quality = "320k"
	}
	executable, _ := os.Executable()
	raw := sourcenetwork.New()
	probe, err := mediastream.New(raw)
	if err != nil {
		t.Fatal("probe setup failed")
	}
	defer probe.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	var requests []songcache.Request
	for _, name := range []string{"偷心", "秋意浓", "可惜没如果"} {
		found, err := music.NewQQClient().Search(ctx, name, 1)
		if err != nil {
			t.Fatal("metadata lookup failed")
		}
		artist := "张学友"
		if name == "可惜没如果" {
			artist = "林俊杰"
		}
		var mid string
		for _, song := range found.Songs {
			if song.Title == name && strings.Contains(song.Artist, artist) {
				mid = song.ID
				break
			}
		}
		if mid == "" {
			t.Fatal("matching metadata unavailable")
		}
		requests = append(requests, songcache.Request{Key: songcache.Key{Source: "test", Quality: quality, Song: mid}, Load: func(ctx context.Context) (songcache.Spec, error) {
			resolved, err := (&ProcessInspector{Executable: executable}).Resolve(ctx, script, ResolveInput{SongMID: mid}, quality)
			if err != nil || resolved.Error != "" {
				return songcache.Spec{}, songcache.ErrCache
			}
			info, err := probe.Probe(ctx, resolved.URL)
			if err != nil {
				return songcache.Spec{}, err
			}
			return songcache.Spec{Remote: resolved.URL, Info: info, Quality: quality}, nil
		}})
	}
	pool := songcache.New(ctx, raw)
	defer pool.Close()
	first, spec, _, err := pool.Use(ctx, 1, requests[0], requests[1:])
	if err != nil {
		t.Fatal("current cache setup failed")
	}
	if !spec.Cacheable {
		first.Close()
		t.Fatal("source lacks a strong validator; stream fallback is available but not sufficient for this test")
	}
	if _, err = first.Control("play", 0); err != nil {
		t.Fatal("muted current playback failed")
	}
	for {
		status := pool.Status()
		ready := status.Current.Complete && len(status.Neighbors) == 2
		for _, neighbor := range status.Neighbors {
			ready = ready && neighbor.PlayerReady && neighbor.PrefixBytes > 0 && neighbor.CachedBytes <= neighbor.PrefixBytes
		}
		if ready {
			break
		}
		if ctx.Err() != nil {
			first.Close()
			t.Fatal("complete current / prepared neighbors timed out")
		}
		time.Sleep(100 * time.Millisecond)
	}
	first.Close()
	started := time.Now()
	second, _, hit, err := pool.Use(ctx, 2, requests[1], []songcache.Request{requests[2], requests[0]})
	if err != nil || !hit {
		t.Fatal("next prepared player not reused")
	}
	defer second.Close()
	if time.Since(started) > 500*time.Millisecond {
		t.Fatal("prepared transition unexpectedly waited on source")
	}
	for _, fraction := range []float64{.5, .9, .1} {
		state, err := second.Control("state", 0)
		if err != nil {
			t.Fatal("state failed")
		}
		target := state.Duration * fraction
		state, err = second.Control("seek", target)
		if err != nil || math.Abs(state.Position-target) > .06 {
			t.Fatal("cached/native seek acknowledgement failed")
		}
	}
	t.Log("three real tracks: complete current, frame-aligned neighbor prefixes, prepared native transition passed; muted; no private data logged")
}
