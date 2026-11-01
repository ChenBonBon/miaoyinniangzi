package main

import (
	"miaoyinniangzi/internal/music"
	"strings"
	"testing"
)

func TestResolveInputIsQQOnlyAndNeverPassesRemoteURLs(t *testing.T) {
	for _, song := range []music.Song{{Source: "wy", ID: "Song1"}, {Source: "tx", ID: "file:///etc/passwd"}, {Source: "tx", ID: "Song1", SongID: "secret-key"}, {Source: "tx", ID: "Song1", MediaMid: "../private"}, {Source: "tx", ID: "Song1", Title: strings.Repeat("x", 8193)}} {
		if _, err := resolveSongInfo(song); err == nil {
			t.Fatal("invalid resolve input accepted")
		}
	}
	info, err := resolveSongInfo(music.Song{Source: "tx", ID: "Song1", SongID: "123", Title: strings.Repeat("曲", 200), CoverURL: "https://private.example/?secret=key"})
	if err != nil || info.SongMID != "Song1" || len([]rune(info.Name)) != 100 {
		t.Fatal("wrong metadata projection")
	}
	if _, err := (&App{}).ResolveSong(music.Song{Source: "tx", ID: "Song1"}, "request-1"); err == nil {
		t.Fatal("resolution fabricated outside desktop")
	}
	if (&App{}).CancelSongResolution("stale") {
		t.Fatal("canceled nonexistent request")
	}
}
