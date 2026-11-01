package music

import "context"

type DiscoveryEntry struct {
	Kind     string `json:"kind"`
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Group    string `json:"group"`
	GroupID  *int   `json:"groupId,omitempty"`
	CoverURL string `json:"coverUrl"`
	Period   string `json:"period"`
	Updated  string `json:"updated"`
	Total    int    `json:"total"`
}
type DiscoveryCatalog struct {
	Kind    string           `json:"kind"`
	Entries []DiscoveryEntry `json:"entries"`
}
type DiscoveryCollection struct {
	Entry DiscoveryEntry `json:"entry"`
	Songs []Song         `json:"songs"`
}
type DiscoveryReader interface {
	DiscoveryCatalog(context.Context, string) (DiscoveryCatalog, error)
	DiscoveryCollection(context.Context, string, int, string) (DiscoveryCollection, error)
}

const MaxDiscoverySongs = 1000

func ValidDiscoveryKind(kind string) bool { return kind == "chart" || kind == "new" }
func ValidDiscoverySelection(kind string, id int, period string) bool {
	if !ValidDiscoveryKind(kind) || id < 1 || id > 1000000 || len(period) > 32 {
		return false
	}
	for _, r := range period {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r == '_') {
			return false
		}
	}
	return kind != "new" || (id >= 1 && id <= 6 && period == "")
}
