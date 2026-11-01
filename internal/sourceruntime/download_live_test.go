package sourceruntime

import (
	"context"
	"os"
	"testing"
	"time"

	"miaoyinniangzi/internal/downloads"
	"miaoyinniangzi/internal/mediastream"
	"miaoyinniangzi/internal/sourcenetwork"
	"miaoyinniangzi/internal/storage"
)

func TestProvidedDownloadLive(t *testing.T) {
	if os.Getenv("MIAOYIN_LIVE_DOWNLOAD") != "1" {
		t.Skip("explicit real-source quota and full-download opt-in")
	}
	script, err := os.ReadFile(os.Getenv("MIAOYIN_SOURCE_TEST_PATH"))
	if err != nil {
		t.Fatal("provided source unavailable")
	}
	defer clear(script)
	quality := os.Getenv("MIAOYIN_DOWNLOAD_TEST_QUALITY")
	if quality == "" {
		quality = "320k"
	}
	if quality != "flac" && quality != "320k" && quality != "128k" {
		t.Fatal("invalid test quality")
	}
	executable, _ := os.Executable()
	raw := sourcenetwork.New()
	probe, err := mediastream.New(raw)
	if err != nil {
		t.Fatal("probe unavailable")
	}
	defer probe.Close()
	directory := t.TempDir()
	store, err := storage.Open(t.TempDir(), directory)
	if err != nil {
		t.Fatal("storage unavailable")
	}
	defer store.Close()
	manager, err := downloads.New(store, raw, func(ctx context.Context, song storage.Song, quality string) (downloads.Media, error) {
		resolved, err := (&ProcessInspector{Executable: executable}).Resolve(ctx, script, ResolveInput{SongMID: song.ID, SongID: song.SongID, MediaMID: song.MediaMid}, quality)
		if err != nil || resolved.Error != "" {
			return downloads.Media{}, downloads.ErrDownload
		}
		info, err := probe.Probe(ctx, resolved.URL)
		if err != nil {
			return downloads.Media{}, err
		}
		return downloads.Media{URL: resolved.URL, Quality: quality, Source: "explicit-test", Info: info}, nil
	})
	if err != nil {
		t.Fatal("manager unavailable")
	}
	defer manager.Close()
	added, err := manager.Add([]storage.Song{{ID: "0039MnYb0qxYhV", Source: "tx", SongID: "97773", MediaMid: "003Qui1q2u1Zho", Title: "fixture", Artist: "fixture"}}, directory, quality)
	if err != nil {
		t.Fatal("queue failed")
	}
	deadline := time.Now().Add(100 * time.Second)
	for time.Now().Before(deadline) {
		for _, task := range manager.List() {
			if task.ID == added[0].ID {
				if task.State == "failed" {
					t.Fatal("real media download failed")
				}
				if task.State == "success" {
					if task.Bytes != task.Total || task.Total <= 0 {
						t.Fatal("incomplete result")
					}
					t.Log("real media downloaded and safely published; no URL or identifying metadata logged")
					return
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("real download timed out")
}
