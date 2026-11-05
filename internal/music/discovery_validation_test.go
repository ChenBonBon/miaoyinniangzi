package music

import (
	"math"
	"testing"
)

func TestDiscoveryCacheValuesRejectUnsafeMetadata(t *testing.T) {
	entry := DiscoveryEntry{Kind: "chart", ID: 1, Title: "fixture", Period: "2026_43", Total: 1}
	catalog := DiscoveryCatalog{Kind: "chart", Entries: []DiscoveryEntry{entry}}
	if !ValidDiscoveryCatalogValue(catalog) || ValidDiscoveryCatalogValue(DiscoveryCatalog{Kind: "chart"}) {
		t.Fatal("catalog validation failed")
	}
	if ValidDiscoveryCatalogValue(DiscoveryCatalog{Kind: "chart", Entries: []DiscoveryEntry{entry, entry}}) {
		t.Fatal("duplicate entries accepted")
	}
	song := Song{ID: "FixtureSong", Source: "tx", Title: "fixture"}
	collection := func(value Song) DiscoveryCollection { return DiscoveryCollection{Entry: entry, Songs: []Song{value}} }
	if !ValidDiscoveryCollectionValue(collection(song)) {
		t.Fatal("valid collection rejected")
	}
	for _, change := range []func(*Song){func(v *Song) { v.Source = "other" }, func(v *Song) { v.CoverURL = "https://example.invalid/private" }, func(v *Song) { v.Duration = math.NaN() }, func(v *Song) { v.ID = "unsafe/id" }, func(v *Song) { v.Title = "fixture\x00" }, func(v *Song) { v.AvailableQualities = []string{"unknown"} }} {
		bad := song
		change(&bad)
		if ValidDiscoveryCollectionValue(collection(bad)) {
			t.Fatal("unsafe cached song accepted")
		}
	}
}
