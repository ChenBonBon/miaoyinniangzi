package sources

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"miaoyinniangzi/internal/sourceruntime"
)

func warmFixture(t *testing.T) (*Manager, *fakeKeys, *resolverInspector, string) {
	t.Helper()
	m, _, keys, _ := fixture(t)
	t.Cleanup(m.Close)
	inspector := &resolverInspector{fakeInspector: fakeInspector{report: sourceruntime.Report{Ready: true, Qualities: []string{"flac"}}}}
	inspector.resolve = func(_ context.Context, data []byte, _ sourceruntime.ResolveInput, q string) (sourceruntime.Resolution, error) {
		if !bytes.Equal(data, []byte(example)) {
			t.Error("incorrect script snapshot")
		}
		return sourceruntime.Resolution{URL: "https://cdn.example/song", Quality: q}, nil
	}
	m.checker = inspector
	imported, err := m.Import(context.Background(), script(t, t.TempDir(), "a.js", example))
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Activate(context.Background(), imported.Source.ID); err != nil {
		t.Fatal(err)
	}
	return m, keys, inspector, imported.Source.ID
}
func TestWarmupCachesOnlySelectedPlaintextWithoutExecuting(t *testing.T) {
	m, keys, inspector, id := warmFixture(t)
	checked := inspector.calls
	reads := len(keys.creates)
	if ready, err := m.Warmup(context.Background()); err != nil || !ready {
		t.Fatal("warmup failed")
	}
	if len(keys.creates) != reads+1 || keys.creates[len(keys.creates)-1] || inspector.calls != checked {
		t.Fatal("warmup created key or executed script")
	}
	retained := m.scriptCache
	keys.err = errors.New("key unavailable after warmup")
	var hit bool
	_, err := m.ResolveObserved(context.Background(), sourceruntime.ResolveInput{SongMID: "Song1"}, "flac", func(phase ResolvePhase, _ time.Duration) {
		if phase == PhaseScriptHit {
			hit = true
		}
	})
	if err != nil || !hit || len(keys.creates) != reads+1 {
		t.Fatal("resolve did not reuse prepared script")
	}
	// Explicit activation still verifies the real system key, never trusting cache.
	if err = m.Activate(context.Background(), id); err == nil {
		t.Fatal("activation bypassed fresh verification")
	}
	for _, b := range retained {
		if b != 0 {
			t.Fatal("invalidated cache was not cleared")
		}
	}
	if len(m.scriptCache) != 0 {
		t.Fatal("cache survived recheck")
	}
}

type blockingWarmKeys struct {
	key              []byte
	started, release chan struct{}
	calls            atomic.Int32
}

func (k *blockingWarmKeys) Key(create bool) ([]byte, error) {
	if create {
		return nil, errors.New("must not create")
	}
	if k.calls.Add(1) == 1 {
		close(k.started)
	}
	<-k.release
	return bytes.Clone(k.key), nil
}
func TestWarmupDoesNotBlockDisableAndLateResultCannotRepublish(t *testing.T) {
	m, keys, _, _ := warmFixture(t)
	blocker := &blockingWarmKeys{key: bytes.Clone(keys.key), started: make(chan struct{}), release: make(chan struct{})}
	m.keys = blocker
	finished := make(chan error, 1)
	go func() { _, err := m.Warmup(context.Background()); finished <- err }()
	<-blocker.started
	m.mu.Lock()
	job := m.scriptJob
	m.mu.Unlock()
	disabled := make(chan error, 1)
	go func() { disabled <- m.Disable(context.Background()) }()
	select {
	case err := <-disabled:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("authorization held management mutex")
	}
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("revoked warmup succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("invalidation did not wake waiter")
	}
	close(blocker.release)
	<-job.done
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.scriptCache) != 0 {
		t.Fatal("late plaintext reappeared after disable")
	}
}
func TestConcurrentWarmupSharesOneKeyRead(t *testing.T) {
	m, keys, _, _ := warmFixture(t)
	blocker := &blockingWarmKeys{key: bytes.Clone(keys.key), started: make(chan struct{}), release: make(chan struct{})}
	m.keys = blocker
	done := make(chan error, 2)
	for range 2 {
		go func() { _, err := m.Warmup(context.Background()); done <- err }()
	}
	<-blocker.started
	close(blocker.release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if blocker.calls.Load() != 1 {
		t.Fatal("duplicate background key reads")
	}
	retained := m.scriptCache
	m.Close()
	for _, b := range retained {
		if b != 0 {
			t.Fatal("shutdown retained plaintext")
		}
	}
	if _, err := m.Warmup(context.Background()); err == nil {
		t.Fatal("closed manager warmed up")
	}
}
func TestNoActiveSourceDoesNotReadOrCreateKey(t *testing.T) {
	m, _, keys, _ := fixture(t)
	defer m.Close()
	if ready, err := m.Warmup(context.Background()); ready || err != nil || len(keys.creates) != 0 {
		t.Fatal("empty startup accessed credentials")
	}
}
