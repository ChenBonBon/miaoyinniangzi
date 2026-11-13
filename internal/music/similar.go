package music

import (
	"context"
	"encoding/json"
	"strconv"
)

type SimilarReader interface {
	SimilarSongs(context.Context, string) ([]Song, error)
}

func ValidSongSeed(id string) bool {
	value, err := strconv.ParseInt(id, 10, 64)
	return err == nil && value > 0 && strconv.FormatInt(value, 10) == id
}
func (c *QQClient) SimilarSongs(ctx context.Context, id string) ([]Song, error) {
	if !ValidSongSeed(id) {
		return nil, ErrQuery
	}
	type track struct {
		Track *rawSong `json:"track"`
	}
	var data struct {
		Retcode *int     `json:"retcode"`
		Old     *[]track `json:"vecSong"`
		New     *[]struct {
			Songs *[]track `json:"songs"`
		} `json:"vecSongNew"`
	}
	if err := c.discoveryRequest(ctx, "similar_songs", "music.recommend.TrackRelationServer", "GetSimilarSongs", map[string]any{"songid": json.Number(id)}, &data); err != nil {
		return nil, err
	}
	if data.Retcode == nil {
		return nil, ErrResponse
	}
	if *data.Retcode != 0 {
		return nil, ErrRestricted
	}
	raw := []rawSong{}
	// Current desktop responses use vecSong[*].track; never recursively harvest
	// mid/name objects, which would incorrectly admit albums and singers.
	if data.Old != nil && (len(*data.Old) > 0 || data.New == nil) {
		if len(*data.Old) > 100 {
			return nil, ErrResponse
		}
		for _, entry := range *data.Old {
			if entry.Track == nil {
				return nil, ErrResponse
			}
			raw = append(raw, *entry.Track)
		}
	} else if data.New != nil {
		if len(*data.New) > 32 {
			return nil, ErrResponse
		}
		for _, group := range *data.New {
			if group.Songs == nil || len(*group.Songs) > 100 {
				return nil, ErrResponse
			}
			for _, entry := range *group.Songs {
				if entry.Track == nil || len(raw) >= 200 {
					return nil, ErrResponse
				}
				raw = append(raw, *entry.Track)
			}
		}
	} else {
		return nil, ErrResponse
	}
	songs, err := normalizeDiscoverySongs(raw)
	if err != nil || !ValidSongs(songs) {
		return nil, ErrResponse
	}
	return songs, nil
}
