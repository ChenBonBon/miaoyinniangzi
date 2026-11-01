package songcache

import (
	"context"
	"testing"
	"time"
)

func TestPrefetchLeavesAnUpstreamSlotForCurrentDecoder(t *testing.T) {
	b := newBudget()
	for range 3 {
		release, err := b.acquire(context.Background(), false)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		release, err := b.acquire(ctx, false)
		if err == nil {
			release()
		}
	}()
	release, err := b.acquire(ctx, true)
	if err != nil {
		t.Fatal("speculative reads occupied all current-decoder capacity")
	}
	release()
	cancel()
	<-done
}
func TestStaleModeOrSessionCannotReplaceCacheTopology(t *testing.T) {
	m := New(context.Background(), &memorySource{})
	defer m.Close()
	key := Key{Source: "source", Quality: "flac", Song: "current"}
	if !m.apply(5, 3, key, nil) {
		t.Fatal("current topology rejected")
	}
	if m.apply(5, 2, key, nil) || m.apply(4, 99, key, nil) {
		t.Fatal("stale mode or session replaced topology")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.entries[key] = &item{key: key, ctx: ctx, cancel: cancel}
	m.mu.Unlock()
	m.CancelPending(4)
	m.Invalidate(4)
	if ctx.Err() != nil {
		t.Fatal("late cancellation revoked a newer cache entry")
	}
	m.mu.Lock()
	retained := m.entries[key] != nil && m.desired[key]
	m.mu.Unlock()
	if !retained {
		t.Fatal("late invalidation removed newer topology")
	}
	m.CancelPending(6)
	if !m.apply(6, 0, key, nil) {
		t.Fatal("new foreground retained previous mode revision")
	}
}
