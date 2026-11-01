package main

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"testing"

	"miaoyinniangzi/internal/storage"
)

func TestPausedProgressWireFormatSavesFractionalDurationAndReopens(t *testing.T) {
	// Use the same JSON decoding step as Wails, not an already-typed Go value.
	wire := []byte(`{"queue":[{"id":"Song1","source":"tx","title":"Test","artist":"Test","album":"","coverUrl":"","duration":298.4130612244898}],"index":0,"position":152.32,"mode":"repeat-one"}`)
	var value storage.Playback
	if err := json.Unmarshal(wire, &value); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	store, err := storage.Open(directory, filepath.Join(directory, "Downloads"))
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: store}
	if err = app.SavePlayback(value); err != nil {
		t.Fatal(err)
	}
	if app.media != nil || app.nativePlayer != nil {
		t.Fatal("saving paused progress initialized playback")
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(directory, filepath.Join(directory, "Downloads"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app.store = store
	for _, target := range []float64{64.125, 287.75} {
		session, err := app.GetPlayerSession()
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(session.Playback.Queue[0].Duration-298.4130612244898) > .000001 {
			t.Fatal("fractional duration changed")
		}
		session.Playback.Position = target
		if err = app.SavePlayback(session.Playback); err != nil {
			t.Fatal(err)
		}
		got, err := app.GetPlayerSession()
		if err != nil || got.Playback.Position != target {
			t.Fatal("paused progress not persisted")
		}
	}
}

func TestPlayerSessionRestoresMetadataWithoutPreparingOrServingMedia(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.Open(dir, filepath.Join(dir, "Downloads"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	settings, err := store.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.Volume = .3
	if err = store.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	snapshot := storage.Playback{Queue: []storage.Song{{ID: "Song1", Source: "tx", Title: "歌曲", Artist: "歌手", SongID: "123", MediaMid: "Media1", Duration: 180}}, Index: 0, Position: 12.5, Mode: "sequential"}
	if err = store.SavePlayback(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	app := &App{store: store}
	session, err := app.GetPlayerSession()
	if err != nil || session.Volume != .3 || session.Playback.Position != 12.5 || session.Playback.Queue[0].SongID != "123" || session.Playback.Queue[0].MediaMid != "Media1" || session.Playback.Queue[0].Duration != 180 {
		t.Fatal("restore lost playback data")
	}
	if app.media != nil || app.playCancel != nil {
		t.Fatal("restoration started media services")
	}
	if app.CancelPlayback("", false) || app.CancelPlayback("old", false) {
		t.Fatal("accepted stale cancellation")
	}
}
