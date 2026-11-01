package main

import (
	"encoding/json"
	"math"
	"testing"

	"miaoyinniangzi/internal/music"
)

func TestCacheNeighborWireFormatAcceptsDecodedFractionalDurations(t *testing.T) {
	var arguments struct {
		Current  music.Song  `json:"current"`
		Previous *music.Song `json:"previous"`
		Next     *music.Song `json:"next"`
	}
	err := json.Unmarshal([]byte(`{"current":{"id":"Current1","source":"tx","duration":298.4130612244898},"previous":{"id":"Previous1","source":"tx","duration":321.8275},"next":null}`), &arguments)
	if err != nil {
		t.Fatal("Wails song argument decode rejected decoded native duration", err)
	}
	if arguments.Previous == nil || arguments.Next != nil || math.Abs(arguments.Current.Duration-298.4130612244898) > 1e-8 {
		t.Fatal("wire format discarded local duration or nullable neighbor")
	}
	if _, err = resolveSongInfo(arguments.Current); err != nil {
		t.Fatal(err)
	}
	if _, err = resolveSongInfo(*arguments.Previous); err != nil {
		t.Fatal(err)
	}
}
