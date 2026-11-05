package music

import (
	"math"
	"unicode/utf8"
)

func validDiscoveryText(value string, limit int) bool {
	return utf8.ValidString(value) && cleanText(value, limit) == value
}
func validCachedDiscoveryEntry(entry DiscoveryEntry) bool {
	return ValidDiscoverySelection(entry.Kind, entry.ID, entry.Period) && entry.Title != "" && validDiscoveryText(entry.Title, 128) && validDiscoveryText(entry.Group, 128) && validDiscoveryText(entry.Updated, 64) && entry.Total >= 0 && entry.Total <= MaxDiscoverySongs && (entry.GroupID == nil || *entry.GroupID >= 0 && *entry.GroupID <= 1000000) && (entry.CoverURL == "" || discoveryCover(entry.CoverURL) == entry.CoverURL)
}
func ValidDiscoveryCatalogValue(value DiscoveryCatalog) bool {
	if !ValidDiscoveryKind(value.Kind) || value.Entries == nil || len(value.Entries) > 128 {
		return false
	}
	seen := map[int]bool{}
	for _, entry := range value.Entries {
		if entry.Kind != value.Kind || !validCachedDiscoveryEntry(entry) || seen[entry.ID] {
			return false
		}
		seen[entry.ID] = true
	}
	return true
}
func ValidDiscoveryCollectionValue(value DiscoveryCollection) bool {
	if !validCachedDiscoveryEntry(value.Entry) || value.Songs == nil || len(value.Songs) > MaxDiscoverySongs {
		return false
	}
	seen := map[string]bool{}
	for _, song := range value.Songs {
		if song.Source != "tx" || !safeID(song.ID) || seen[song.ID] || song.Title == "" || !validDiscoveryText(song.Title, 512) || !validDiscoveryText(song.Subtitle, 512) || !validDiscoveryText(song.Artist, 8448) || !validDiscoveryText(song.Album, 512) || len(song.Artists) > 32 || len(song.AvailableQualities) > 3 || math.IsNaN(song.Duration) || math.IsInf(song.Duration, 0) || song.Duration < 0 || song.Duration > 86400 || (song.CoverURL != "" && discoveryCover(song.CoverURL) != song.CoverURL) {
			return false
		}
		for _, id := range []string{song.SongID, song.MediaMid, song.AlbumMid} {
			if id != "" && !safeID(id) {
				return false
			}
		}
		for _, artist := range song.Artists {
			if artist.ID != "" && !safeID(artist.ID) || !validDiscoveryText(artist.Name, 256) {
				return false
			}
		}
		for _, quality := range song.AvailableQualities {
			if quality != "flac" && quality != "320k" && quality != "128k" {
				return false
			}
		}
		seen[song.ID] = true
	}
	return true
}
