package songcache

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"miaoyinniangzi/internal/mediastream"
)

func TestNativePreparedNeighborsAndCompleteCacheSeeking(t *testing.T) {
	if os.Getenv("MIAOYIN_MPV_SMOKE") != "1" {
		t.Skip("explicit muted native cache integration")
	}
	fixtures := map[string]string{"flac": "../../frontend/e2e/fixtures/tone.flac"}
	if dir := os.Getenv("MIAOYIN_MPV_MP3_TEST_DIR"); dir != "" {
		fixtures["320k"] = filepath.Join(dir, "320k.mp3")
		fixtures["128k"] = filepath.Join(dir, "128k.mp3")
	}
	for name, path := range fixtures {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			codec := "mp3"
			if name == "flac" {
				codec = "flac"
			}
			source := &memorySource{data: data, etag: `"stable"`}
			m := New(context.Background(), source)
			defer m.Close()
			request := func(id string) Request {
				return Request{Key: Key{Source: "test", Quality: name, Song: id}, Load: func(context.Context) (Spec, error) {
					return Spec{Remote: "http://public.example/" + id, Info: mediastream.Info{Codec: codec, Size: int64(len(data)), Seekable: true}, Quality: name}, nil
				}}
			}
			one, two, three := request("one"), request("two"), request("three")
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			player, _, _, err := m.Use(ctx, 1, one, []Request{two, three})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = player.Control("play", 0); err != nil {
				t.Fatal(err)
			}
			waitUntil(t, func() bool {
				m.mu.Lock()
				defer m.mu.Unlock()
				return m.entries[two.Key] != nil && m.entries[two.Key].finished && m.entries[three.Key] != nil && m.entries[three.Key].finished
			})
			player.Close()
			started := time.Now()
			next, _, hit, err := m.Use(ctx, 2, two, []Request{one, three})
			if err != nil || !hit {
				t.Fatal("native neighbor reuse failed")
			}
			defer next.Close()
			if time.Since(started) > 500*time.Millisecond {
				t.Fatal("ready native neighbor was recreated instead of transferred")
			}
			m.mu.Lock()
			entry := m.entries[two.Key]
			m.mu.Unlock()
			waitUntil(t, func() bool {
				entry.disk.mu.Lock()
				defer entry.disk.mu.Unlock()
				return entry.disk.available(0) == int64(len(data))
			})
			counter, _ := source.byURL.Load("http://public.example/two")
			calls := counter.(*atomic.Int32).Load()
			for _, target := range []float64{4, 1, 5.8, 0} {
				state, err := next.Control("seek", target)
				if err != nil || math.Abs(state.Position-target) > .05 || state.Playing {
					t.Fatal("cached paused seek failed")
				}
			}
			if counter.(*atomic.Int32).Load() != calls {
				t.Fatal("complete cached seeks touched upstream")
			}
			if _, err = next.Control("play", 0); err != nil {
				t.Fatal(err)
			}
			t.Log("prepared native player reused; complete-cache seeks need no upstream; output muted")
		})
	}
}
