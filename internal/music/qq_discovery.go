package music

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"
)

type chartInfo struct {
	ID        int    `json:"topId"`
	Title     string `json:"title"`
	Period    string `json:"period"`
	Updated   string `json:"updateTime"`
	Total     int    `json:"totalNum"`
	Cover     string `json:"frontPicUrl"`
	HeadCover string `json:"headPicUrl"`
	Songs     []struct {
		ID int64 `json:"songId"`
	} `json:"song"`
}

func discoveryCover(raw string) string {
	if len(raw) > 2048 {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() != "y.gtimg.cn" || u.User != nil || u.Port() != "" && u.Port() != "443" || u.Fragment != "" {
		return ""
	}
	u.Scheme = "https"
	return u.String()
}
func chartEntry(info chartInfo, group string) (DiscoveryEntry, bool) {
	title := cleanText(info.Title, 128)
	if !ValidDiscoverySelection("chart", info.ID, info.Period) || title == "" || info.Total < 0 || info.Total > MaxDiscoverySongs {
		return DiscoveryEntry{}, false
	}
	cover := discoveryCover(info.Cover)
	if cover == "" {
		cover = discoveryCover(info.HeadCover)
	}
	return DiscoveryEntry{Kind: "chart", ID: info.ID, Title: title, Group: cleanText(group, 128), CoverURL: cover, Period: info.Period, Updated: cleanText(info.Updated, 64), Total: info.Total}, true
}
func (c *QQClient) discoveryRequest(ctx context.Context, operation, module, method string, param any, target any) (err error) {
	started := time.Now()
	event := diagnostic{Operation: operation}
	defer func() { c.diagnostics.record(event, started, err) }()
	payload, err := json.Marshal(map[string]any{"comm": map[string]any{"ct": 24, "cv": 0, "uin": 0, "format": "json"}, "req_1": map[string]any{"module": module, "method": method, "param": param}})
	if err != nil {
		return ErrResponse
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return ErrNetwork
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Referer", "https://y.qq.com/")
	request.Header.Set("User-Agent", "Miaoyin/0.1 (music metadata)")
	response, err := c.http.Do(request)
	if err != nil {
		return networkError(ctx, err)
	}
	defer response.Body.Close()
	event.HTTPStatus = response.StatusCode
	switch response.StatusCode {
	case 429:
		return ErrRateLimit
	case 401, 403:
		return ErrRestricted
	case 200:
	default:
		return ErrResponse
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return networkError(ctx, err)
	}
	if len(data) > maxResponseBytes {
		return ErrResponse
	}
	var envelope struct {
		Code    *int `json:"code"`
		Request *struct {
			Code *int            `json:"code"`
			Data json.RawMessage `json:"data"`
		} `json:"req_1"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Code == nil {
		return ErrResponse
	}
	event.Code = envelope.Code
	if envelope.Request != nil {
		event.RequestCode = envelope.Request.Code
	}
	if *envelope.Code != 0 {
		return ErrRestricted
	}
	if envelope.Request == nil || envelope.Request.Code == nil {
		return ErrResponse
	}
	if *envelope.Request.Code != 0 {
		return ErrRestricted
	}
	if len(envelope.Request.Data) == 0 || string(envelope.Request.Data) == "null" || json.Unmarshal(envelope.Request.Data, target) != nil {
		return ErrResponse
	}
	return nil
}
func (c *QQClient) DiscoveryCatalog(ctx context.Context, kind string) (DiscoveryCatalog, error) {
	result := DiscoveryCatalog{Kind: kind, Entries: []DiscoveryEntry{}}
	if !ValidDiscoveryKind(kind) {
		return result, ErrQuery
	}
	if kind == "new" {
		data, err := c.newSongs(ctx, 5, "discovery_catalog")
		if err != nil {
			return result, err
		}
		if data.Categories == nil || len(data.Categories) > 16 {
			return result, ErrResponse
		}
		seen := map[int]bool{}
		for _, category := range data.Categories {
			title := cleanText(category.Title, 64)
			if !ValidDiscoverySelection("new", category.ID, "") || seen[category.ID] || title == "" {
				continue
			}
			seen[category.ID] = true
			result.Entries = append(result.Entries, DiscoveryEntry{Kind: kind, ID: category.ID, Title: title, Group: "新歌"})
		}
		if len(data.Categories) > 0 && len(result.Entries) == 0 {
			return result, ErrResponse
		}
		return result, nil
	}
	var data struct {
		Groups *[]struct {
			ID     *int        `json:"groupId"`
			Name   string      `json:"groupName"`
			Charts []chartInfo `json:"toplist"`
		} `json:"group"`
	}
	if err := c.discoveryRequest(ctx, "discovery_catalog", "musicToplist.ToplistInfoServer", "GetAll", map[string]any{}, &data); err != nil {
		return result, err
	}
	if data.Groups == nil || len(*data.Groups) > 32 {
		return result, ErrResponse
	}
	seen := map[int]bool{}
	rawCount := 0
	for _, group := range *data.Groups {
		rawCount += len(group.Charts)
		if len(group.Charts) > 128 {
			return result, ErrResponse
		}
		for _, info := range group.Charts {
			entry, ok := chartEntry(info, group.Name)
			if !ok || seen[entry.ID] {
				continue
			}
			seen[entry.ID] = true
			if group.ID != nil && *group.ID >= 0 && *group.ID <= 1000000 {
				entry.GroupID = group.ID
			}
			result.Entries = append(result.Entries, entry)
		}
	}
	if rawCount > 0 && len(result.Entries) == 0 {
		return result, ErrResponse
	}
	if len(result.Entries) > 128 {
		return DiscoveryCatalog{}, ErrResponse
	}
	return result, nil
}

type newSongData struct {
	Categories []struct {
		ID    int    `json:"type"`
		Title string `json:"lan"`
	} `json:"lanlist"`
	Type    *int       `json:"type"`
	Title   string     `json:"lan"`
	Message string     `json:"ret_msg"`
	Songs   *[]rawSong `json:"songlist"`
}

func (c *QQClient) newSongs(ctx context.Context, id int, operation string) (newSongData, error) {
	var data newSongData
	err := c.discoveryRequest(ctx, operation, "newsong.NewSongServer", "get_new_song_info", map[string]any{"type": id}, &data)
	if err != nil {
		return data, err
	}
	if data.Message != "" {
		return data, ErrRestricted
	}
	if data.Type == nil || *data.Type != id || data.Songs == nil || len(*data.Songs) > MaxDiscoverySongs {
		return data, ErrResponse
	}
	return data, nil
}
func normalizeDiscoverySongs(raw []rawSong) ([]Song, error) {
	songs := []Song{}
	seen := map[string]bool{}
	for _, item := range raw {
		song, ok := normalizeSong(item)
		if !ok || seen[song.ID] {
			continue
		}
		seen[song.ID] = true
		songs = append(songs, song)
	}
	if len(raw) > 0 && len(songs) == 0 {
		return nil, ErrResponse
	}
	return songs, nil
}
func (c *QQClient) DiscoveryCollection(ctx context.Context, kind string, id int, period string) (DiscoveryCollection, error) {
	result := DiscoveryCollection{Songs: []Song{}}
	if !ValidDiscoverySelection(kind, id, period) {
		return result, ErrQuery
	}
	if kind == "new" {
		data, err := c.newSongs(ctx, id, "discovery_collection")
		if err != nil {
			return result, err
		}
		result.Songs, err = normalizeDiscoverySongs(*data.Songs)
		if err != nil {
			return result, err
		}
		title := cleanText(data.Title, 64)
		if title == "" {
			return result, ErrResponse
		}
		result.Entry = DiscoveryEntry{Kind: kind, ID: id, Title: title + "新歌", Group: "新歌", Total: len(result.Songs)}
		return result, nil
	}
	// Pin a chart period and total across all pages. Never silently give Play All
	// or batch download a partial chart after a late failure or server reordering.
	all := []rawSong{}
	seen := map[int64]bool{}
	total := -1
	for offset := 0; total < 0 || offset < total; offset += 100 {
		var data struct {
			Chart *chartInfo `json:"data"`
			Songs *[]rawSong `json:"songInfoList"`
		}
		err := c.discoveryRequest(ctx, "discovery_collection", "musicToplist.ToplistInfoServer", "GetDetail", map[string]any{"topid": id, "num": 100, "offset": offset, "period": period}, &data)
		if err != nil {
			return result, err
		}
		if data.Chart == nil || data.Songs == nil || data.Chart.ID != id || len(*data.Songs) > 100 {
			return result, ErrResponse
		}
		entry, ok := chartEntry(*data.Chart, "")
		if !ok || period != "" && entry.Period != period {
			return result, ErrResponse
		}
		if total < 0 {
			total = data.Chart.Total
			period = entry.Period
			result.Entry = entry
		} else if total != data.Chart.Total || period != entry.Period {
			return DiscoveryCollection{}, ErrResponse
		}
		expected := total - offset
		if expected > 100 {
			expected = 100
		}
		if expected < 0 || len(*data.Songs) != expected || len(data.Chart.Songs) != expected {
			return DiscoveryCollection{}, ErrResponse
		}
		info := map[int64]rawSong{}
		for _, song := range *data.Songs {
			if song.ID <= 0 || seen[song.ID] {
				return DiscoveryCollection{}, ErrResponse
			}
			info[song.ID] = song
			seen[song.ID] = true
		}
		for _, ranked := range data.Chart.Songs {
			song, ok := info[ranked.ID]
			if !ok {
				return DiscoveryCollection{}, ErrResponse
			}
			all = append(all, song)
			delete(info, ranked.ID)
		}
		if total == 0 {
			break
		}
	}
	var err error
	result.Songs, err = normalizeDiscoverySongs(all)
	if err != nil {
		return DiscoveryCollection{}, err
	}
	result.Entry.Total = len(result.Songs)
	return result, nil
}
