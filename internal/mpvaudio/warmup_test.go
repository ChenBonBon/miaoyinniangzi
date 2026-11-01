package mpvaudio

import (
	"path/filepath"
	"testing"
)

func TestWarmupDoesNotCreateAPlayer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preloaded-library")
	t.Setenv("MIAOYIN_MPV_LIBRARY", path)
	created := false
	libraries.Lock()
	libraries.values[path] = &api{create: func() uintptr { created = true; return 0 }}
	libraries.Unlock()
	defer func() { libraries.Lock(); delete(libraries.values, path); libraries.Unlock() }()
	if err := Warmup(); err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("warmup created an audio player")
	}
}
