package music

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const qqSearchURL = "https://u.y.qq.com/cgi-bin/musicu.fcg"
const maxResponseBytes = 2 * 1024 * 1024

type QQClient struct {
	http               *http.Client
	endpoint           string
	lyricsEndpoint     string
	suggestionEndpoint string
	diagnostics        *Diagnostics
}

func NewQQClient() *QQClient {
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 4 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 4 * time.Second, ResponseHeaderTimeout: 6 * time.Second,
		IdleConnTimeout: 60 * time.Second, MaxIdleConns: 4, MaxIdleConnsPerHost: 2,
		MaxResponseHeaderBytes: 64 * 1024, ForceAttemptHTTP2: true,
	}
	return newQQClient(transport, qqSearchURL)
}
func newQQClient(transport http.RoundTripper, endpoint string) *QQClient {
	return &QQClient{endpoint: endpoint, suggestionEndpoint: qqSuggestionURL, http: &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func validateQuery(query string, page int) (string, error) {
	if len(query) > 400 || !utf8.ValidString(query) {
		return "", ErrQuery
	}
	query = strings.TrimSpace(query)
	if query == "" || utf8.RuneCountInString(query) > 100 {
		return "", ErrQuery
	}
	for _, r := range query {
		if unicode.IsControl(r) {
			return "", ErrQuery
		}
	}
	if page < 1 || page > MaxSearchPage {
		return "", ErrPage
	}
	return query, nil
}

// These anonymous desktop search parameters are a public metadata protocol,
// not credentials. No cookies, script API keys or user account identifiers are
// sent. The service's restrictions are reported, never bypassed with retries.
func searchPayload(query string, page int) ([]byte, error) {
	random := make([]byte, 20)
	if _, err := rand.Read(random); err != nil {
		return nil, ErrNetwork
	}
	searchID := strings.ToUpper(hex.EncodeToString(random[:16])) + fmt.Sprintf("%05d", binary.BigEndian.Uint32(random[16:])%100000)
	payload := struct {
		Comm    map[string]any `json:"comm"`
		Request struct {
			Module string         `json:"module"`
			Method string         `json:"method"`
			Param  map[string]any `json:"param"`
		} `json:"req_1"`
	}{Comm: map[string]any{"ct": "19", "cv": "2151", "uin": "0", "tmeLoginType": 0, "tmeAppID": "qqmusic"}}
	payload.Request.Module = "music.search.SearchCgiService"
	payload.Request.Method = "DoSearchForQQMusicDesktop"
	payload.Request.Param = map[string]any{"grp": 1, "num_per_page": SearchPageSize, "page_num": page, "query": query, "remoteplace": "txt.newclient.top", "search_type": 0, "searchid": searchID}
	return json.Marshal(payload)
}
func (c *QQClient) Search(ctx context.Context, query string, page int) (result SearchResult, err error) {
	query, err = validateQuery(query, page)
	if err != nil {
		return SearchResult{}, err
	}
	started := time.Now()
	event := diagnostic{Operation: "search"}
	defer func() { c.diagnostics.record(event, started, err) }()
	payload, err := searchPayload(query, page)
	if err != nil {
		return SearchResult{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return SearchResult{}, ErrNetwork
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Referer", "https://y.qq.com/")
	request.Header.Set("User-Agent", "Miaoyin/0.1 (macOS; music metadata)")
	response, err := c.http.Do(request)
	if err != nil {
		return SearchResult{}, networkError(ctx, err)
	}
	defer response.Body.Close()
	event.HTTPStatus = response.StatusCode
	if response.StatusCode == http.StatusTooManyRequests {
		return SearchResult{}, ErrRateLimit
	}
	if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusUnauthorized {
		return SearchResult{}, ErrRestricted
	}
	if response.StatusCode != http.StatusOK {
		return SearchResult{}, ErrResponse
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return SearchResult{}, networkError(ctx, err)
	}
	if len(data) > maxResponseBytes {
		return SearchResult{}, ErrResponse
	}
	return decodeSearchDiagnostic(data, query, page, &event)
}
func networkError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return ErrCanceled
	}
	var timeout net.Error
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
		return ErrTimeout
	}
	return ErrNetwork
}

type rawArtist struct {
	Mid  string `json:"mid"`
	Name string `json:"name"`
}
type rawSong struct {
	ID       int64       `json:"id"`
	Mid      string      `json:"mid"`
	Title    string      `json:"title"`
	Name     string      `json:"name"`
	Subtitle string      `json:"subtitle"`
	Interval float64     `json:"interval"`
	Singer   []rawArtist `json:"singer"`
	Album    struct {
		Mid   string `json:"mid"`
		Name  string `json:"name"`
		Title string `json:"title"`
	} `json:"album"`
	File struct {
		MediaMid string `json:"media_mid"`
		SizeFLAC int64  `json:"size_flac"`
		Size320  int64  `json:"size_320mp3"`
		Size128  int64  `json:"size_128mp3"`
	} `json:"file"`
}
type rawSearchData struct {
	Code *int `json:"code"`
	Body struct {
		Song *struct {
			List []rawSong `json:"list"`
		} `json:"song"`
	} `json:"body"`
	Meta *struct {
		Page     int  `json:"curpage"`
		Sum      int  `json:"sum"`
		NextPage *int `json:"nextpage"`
		Ret      *int `json:"ret"`
		Filter   *int `json:"is_filter"`
	} `json:"meta"`
}

func decodeSearch(data []byte, query string, page int) (SearchResult, error) {
	return decodeSearchDiagnostic(data, query, page, nil)
}
func decodeSearchDiagnostic(data []byte, query string, page int, event *diagnostic) (SearchResult, error) {
	var response struct {
		Code    *int `json:"code"`
		Request *struct {
			Code *int           `json:"code"`
			Data *rawSearchData `json:"data"`
		} `json:"req_1"`
	}
	if json.Unmarshal(data, &response) != nil || response.Code == nil {
		return SearchResult{}, ErrResponse
	}
	if event != nil {
		event.Code = response.Code
		if response.Request != nil {
			event.RequestCode = response.Request.Code
			if raw := response.Request.Data; raw != nil {
				event.DataCode = raw.Code
				if raw.Meta != nil {
					event.MetaRet = raw.Meta.Ret
					event.Filter = raw.Meta.Filter
				}
			}
		}
	}
	if *response.Code != 0 {
		return SearchResult{}, ErrRestricted
	}
	if response.Request == nil || response.Request.Code == nil {
		return SearchResult{}, ErrResponse
	}
	if *response.Request.Code != 0 {
		return SearchResult{}, ErrRestricted
	}
	raw := response.Request.Data
	if raw == nil || raw.Code == nil {
		return SearchResult{}, ErrResponse
	}
	if *raw.Code != 0 {
		return SearchResult{}, ErrRestricted
	}
	if raw.Meta == nil || raw.Body.Song == nil || raw.Body.Song.List == nil {
		return SearchResult{}, ErrResponse
	}
	if (raw.Meta.Ret != nil && *raw.Meta.Ret != 0) || (raw.Meta.Filter != nil && *raw.Meta.Filter < 0) {
		return SearchResult{}, ErrRestricted
	}
	if raw.Meta.Sum < 0 || raw.Meta.Sum > 10000000 || raw.Meta.Page != page || len(raw.Body.Song.List) > SearchPageSize {
		return SearchResult{}, ErrResponse
	}
	result := SearchResult{Query: query, Page: page, PageSize: SearchPageSize, Total: raw.Meta.Sum, Songs: []Song{}}
	seen := map[string]bool{}
	for _, item := range raw.Body.Song.List {
		song, ok := normalizeSong(item)
		if !ok || seen[song.ID] {
			continue
		}
		seen[song.ID] = true
		result.Songs = append(result.Songs, song)
	}
	if len(raw.Body.Song.List) > 0 && len(result.Songs) == 0 {
		return SearchResult{}, ErrResponse
	}
	if len(raw.Body.Song.List) > 0 && page < MaxSearchPage {
		if raw.Meta.NextPage != nil {
			result.HasMore = *raw.Meta.NextPage > page
		} else {
			result.HasMore = page*SearchPageSize < result.Total
		}
		if result.Total > 0 && page*SearchPageSize >= result.Total {
			result.HasMore = false
		}
	}
	return result, nil
}
func cleanText(value string, limit int) string {
	value = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value))
	runes := []rune(value)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}
func safeID(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
func normalizeSong(raw rawSong) (Song, bool) {
	if !safeID(raw.Mid) {
		return Song{}, false
	}
	title := cleanText(raw.Title, 512)
	if title == "" {
		title = cleanText(raw.Name, 512)
	}
	if title == "" {
		return Song{}, false
	}
	song := Song{ID: raw.Mid, Source: "tx", Title: title, Subtitle: cleanText(raw.Subtitle, 512), Artists: []Artist{}, Album: cleanText(raw.Album.Name, 512), AvailableQualities: []string{}}
	if raw.ID > 0 {
		song.SongID = strconv.FormatInt(raw.ID, 10)
	}
	if song.Album == "" {
		song.Album = cleanText(raw.Album.Title, 512)
	}
	if safeID(raw.File.MediaMid) {
		song.MediaMid = raw.File.MediaMid
	}
	names := []string{}
	for index, artist := range raw.Singer {
		if index >= 32 {
			break
		}
		name := cleanText(artist.Name, 256)
		if name == "" {
			continue
		}
		id := ""
		if safeID(artist.Mid) {
			id = artist.Mid
		}
		song.Artists = append(song.Artists, Artist{ID: id, Name: name})
		names = append(names, name)
	}
	song.Artist = strings.Join(names, " / ")
	if safeID(raw.Album.Mid) {
		song.AlbumMid = raw.Album.Mid
		song.CoverURL = "https://y.gtimg.cn/music/photo_new/T002R300x300M000" + raw.Album.Mid + ".jpg"
	} else if len(song.Artists) > 0 && song.Artists[0].ID != "" {
		song.CoverURL = "https://y.gtimg.cn/music/photo_new/T001R300x300M000" + song.Artists[0].ID + ".jpg"
	}
	if raw.Interval > 0 && raw.Interval <= 86400 {
		song.Duration = float64(raw.Interval)
	}
	if raw.File.SizeFLAC > 0 {
		song.AvailableQualities = append(song.AvailableQualities, "flac")
	}
	if raw.File.Size320 > 0 {
		song.AvailableQualities = append(song.AvailableQualities, "320k")
	}
	if raw.File.Size128 > 0 {
		song.AvailableQualities = append(song.AvailableQualities, "128k")
	}
	return song, true
}
