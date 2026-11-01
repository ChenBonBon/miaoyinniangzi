package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestQueuedNextPersistsAndLegacyQueueStillLoads(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	downloads := filepath.Join(t.TempDir(), "Downloads")
	store, err := Open(directory, downloads)
	if err != nil {
		t.Fatal(err)
	}
	next := 2
	snapshot := Playback{Queue: []Song{song("a"), song("b"), song("c")}, Index: 0, Position: 1.25, Mode: "shuffle", QueuedNext: &next}
	if err = store.SavePlayback(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(directory, downloads)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	restored, err := store.Playback(ctx)
	if err != nil || restored.QueuedNext == nil || *restored.QueuedNext != 2 {
		t.Fatal("queued next lost across reopen")
	}
	var legacy Playback
	if err = json.Unmarshal([]byte(`{"queue":[{"id":"old","source":"tx","title":"old"}],"index":0,"position":0,"mode":"shuffle"}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if err = store.SavePlayback(ctx, legacy); err != nil || legacy.QueuedNext != nil {
		t.Fatal("legacy playback rejected")
	}
	for _, invalid := range []int{-1, 0, 3} {
		snapshot.QueuedNext = &invalid
		if err = store.SavePlayback(ctx, snapshot); err == nil {
			t.Fatal("invalid queued-next occurrence accepted")
		}
	}
	empty := Playback{Queue: []Song{}, Index: -1, Mode: "shuffle"}
	if err = store.SavePlayback(ctx, empty); err != nil {
		t.Fatal(err)
	}
}
