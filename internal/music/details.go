package music

import "context"

type AlbumSummary struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	CoverURL string `json:"coverUrl"`
	Date     string `json:"date"`
}
type MusicDetail struct {
	Kind          string         `json:"kind"`
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	Description   string         `json:"description"`
	CoverURL      string         `json:"coverUrl"`
	Artists       []Artist       `json:"artists"`
	Date          string         `json:"date"`
	Songs         []Song         `json:"songs"`
	Albums        []AlbumSummary `json:"albums"`
	AlbumPage     int            `json:"albumPage"`
	TotalAlbums   int            `json:"totalAlbums"`
	HasMoreAlbums bool           `json:"hasMoreAlbums"`
}
type DetailReader interface {
	Detail(context.Context, string, string, int) (MusicDetail, error)
}

func ValidDetailSelection(kind, id string, page int) bool {
	return (kind == "artist" || kind == "album") && safeID(id) && page >= 1 && page <= 100 && (kind != "album" || page == 1)
}
func qqCover(kind, id string) string {
	if !safeID(id) {
		return ""
	}
	prefix := "T002"
	if kind == "artist" {
		prefix = "T001"
	}
	return "https://y.gtimg.cn/music/photo_new/" + prefix + "R300x300M000" + id + ".jpg"
}
func (c *QQClient) Detail(ctx context.Context, kind, id string, page int) (MusicDetail, error) {
	if !ValidDetailSelection(kind, id, page) {
		return MusicDetail{}, ErrQuery
	}
	if kind == "artist" {
		return c.artistDetail(ctx, id, page)
	}
	return c.albumDetail(ctx, id)
}
func (c *QQClient) artistDetail(ctx context.Context, id string, page int) (MusicDetail, error) {
	result := MusicDetail{Kind: "artist", ID: id, CoverURL: qqCover("artist", id), Artists: []Artist{}, Songs: []Song{}, Albums: []AlbumSummary{}, AlbumPage: page}
	var data struct {
		Brief string `json:"singer_brief"`
		Info  struct {
			Mid  string `json:"mid"`
			Name string `json:"name"`
		} `json:"singer_info"`
		Songs *[]rawSong `json:"songlist"`
	}
	if err := c.discoveryRequest(ctx, "artist_detail", "music.web_singer_info_svr", "get_singer_detail_info", map[string]any{"singermid": id, "sort": 5, "sin": 0, "num": 50}, &data); err != nil {
		return result, err
	}
	if data.Info.Mid != id || data.Songs == nil || len(*data.Songs) > 50 {
		return result, ErrResponse
	}
	result.Title = cleanText(data.Info.Name, 256)
	result.Description = cleanText(data.Brief, 2048)
	if result.Title == "" {
		return result, ErrResponse
	}
	songs, err := normalizeDiscoverySongs(*data.Songs)
	if err != nil {
		return result, err
	}
	result.Songs = songs
	var albums struct {
		Mid   string `json:"singerMid"`
		Total int    `json:"total"`
		List  *[]struct {
			ID     string `json:"albumMid"`
			Title  string `json:"albumName"`
			Artist string `json:"singerName"`
			Date   string `json:"publishDate"`
		} `json:"albumList"`
	}
	if err = c.discoveryRequest(ctx, "artist_albums", "music.musichallAlbum.AlbumListServer", "GetAlbumList", map[string]any{"singerMid": id, "begin": (page - 1) * 20, "num": 20, "order": 0}, &albums); err != nil {
		return result, err
	}
	if albums.Mid != id || albums.List == nil || len(*albums.List) > 20 || albums.Total < 0 || albums.Total > 100000 {
		return result, ErrResponse
	}
	seen := map[string]bool{}
	for _, album := range *albums.List {
		title := cleanText(album.Title, 512)
		if !safeID(album.ID) || title == "" || seen[album.ID] {
			return result, ErrResponse
		}
		seen[album.ID] = true
		result.Albums = append(result.Albums, AlbumSummary{ID: album.ID, Title: title, Artist: cleanText(album.Artist, 256), Date: cleanText(album.Date, 32), CoverURL: qqCover("album", album.ID)})
	}
	result.TotalAlbums = albums.Total
	result.HasMoreAlbums = page < 100 && page*20 < albums.Total
	if result.HasMoreAlbums && len(result.Albums) == 0 {
		return result, ErrResponse
	}
	return result, nil
}
func (c *QQClient) albumDetail(ctx context.Context, id string) (MusicDetail, error) {
	result := MusicDetail{Kind: "album", ID: id, CoverURL: qqCover("album", id), Artists: []Artist{}, Songs: []Song{}, Albums: []AlbumSummary{}, AlbumPage: 1}
	var info struct {
		Basic struct {
			ID    string `json:"albumMid"`
			Title string `json:"albumName"`
			Date  string `json:"publishDate"`
		} `json:"basicInfo"`
		Singer struct {
			List []rawArtist `json:"singerList"`
		} `json:"singer"`
	}
	if err := c.discoveryRequest(ctx, "album_detail", "music.musichallAlbum.AlbumInfoServer", "GetAlbumDetail", map[string]any{"albumMid": id}, &info); err != nil {
		return result, err
	}
	result.Title = cleanText(info.Basic.Title, 512)
	result.Date = cleanText(info.Basic.Date, 32)
	if info.Basic.ID != id || result.Title == "" || len(info.Singer.List) > 32 {
		return result, ErrResponse
	}
	for _, artist := range info.Singer.List {
		if safeID(artist.Mid) && cleanText(artist.Name, 256) != "" {
			result.Artists = append(result.Artists, Artist{ID: artist.Mid, Name: cleanText(artist.Name, 256)})
		}
	}
	total := -1
	for offset := 0; offset == 0 || offset < total; offset += 100 {
		var data struct {
			ID    string `json:"albumMid"`
			Total *int   `json:"totalNum"`
			Begin *int   `json:"curBegin"`
			List  *[]struct {
				Song rawSong `json:"songInfo"`
			} `json:"songList"`
		}
		if err := c.discoveryRequest(ctx, "album_detail", "music.musichallAlbum.AlbumSongList", "GetAlbumSongList", map[string]any{"albumMid": id, "begin": offset, "num": 100, "order": 2}, &data); err != nil {
			return result, err
		}
		if data.ID != id || data.Total == nil || *data.Total < 0 || *data.Total > 1000 || data.Begin == nil || *data.Begin != offset || data.List == nil {
			return result, ErrResponse
		}
		if total < 0 {
			total = *data.Total
		} else if total != *data.Total {
			return result, ErrResponse
		}
		expected := min(100, total-offset)
		if len(*data.List) != expected {
			return result, ErrResponse
		}
		for _, item := range *data.List {
			song, ok := normalizeSong(item.Song)
			if !ok {
				return result, ErrResponse
			}
			result.Songs = append(result.Songs, song)
		}
	}
	return result, nil
}
