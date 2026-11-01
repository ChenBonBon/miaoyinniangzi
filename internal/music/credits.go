package music

import "context"

type SongCredits struct {
	ID       string   `json:"id"`
	Artists  []Artist `json:"artists"`
	AlbumMid string   `json:"albumMid"`
}
type SongCreditReader interface {
	SongCredits(context.Context, string) (SongCredits, error)
}

func (c *QQClient) SongCredits(ctx context.Context, id string) (SongCredits, error) {
	result := SongCredits{ID: id, Artists: []Artist{}}
	if !safeID(id) {
		return result, ErrQuery
	}
	var data struct {
		Track rawSong `json:"track_info"`
	}
	if err := c.discoveryRequest(ctx, "song_credits", "music.pf_song_detail_svr", "get_song_detail_yqq", map[string]any{"song_mid": id, "song_type": 0}, &data); err != nil {
		return result, err
	}
	song, ok := normalizeSong(data.Track)
	if !ok || song.ID != id {
		return result, ErrResponse
	}
	result.Artists = song.Artists
	result.AlbumMid = song.AlbumMid
	return result, nil
}
