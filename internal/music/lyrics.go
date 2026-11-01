package music

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const qqLyricsURL = "https://c.y.qq.com/lyric/fcgi-bin/fcg_query_lyric_new.fcg"
const maxLyricsBytes = 512 * 1024

type LyricLine struct {
	Time float64 `json:"time"`
	Text string  `json:"text"`
}
type Lyrics struct {
	SongID string      `json:"songId"`
	Lines  []LyricLine `json:"lines"`
	Text   string      `json:"text"`
}
type LyricReader interface {
	Lyrics(context.Context, string) (Lyrics, error)
}

func validSongMID(mid string) bool {
	if len(mid) == 0 || len(mid) > 64 {
		return false
	}
	for _, r := range mid {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
func (c *QQClient) Lyrics(ctx context.Context, mid string) (Lyrics, error) {
	if !validSongMID(mid) {
		return Lyrics{}, ErrQuery
	}
	endpoint := c.lyricsEndpoint
	if endpoint == "" {
		endpoint = qqLyricsURL
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return Lyrics{}, ErrResponse
	}
	query := u.Query()
	query.Set("songmid", mid)
	query.Set("format", "json")
	query.Set("nobase64", "1")
	query.Set("g_tk", "5381")
	u.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Lyrics{}, ErrResponse
	}
	request.Header.Set("Referer", "https://y.qq.com/")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Miaoyin/0.1 (music metadata)")
	response, err := c.http.Do(request)
	if err != nil {
		return Lyrics{}, networkError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode == 429 {
		return Lyrics{}, ErrRateLimit
	}
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return Lyrics{}, ErrRestricted
	}
	if response.StatusCode != 200 {
		return Lyrics{}, ErrResponse
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxLyricsBytes+1))
	if err != nil || len(body) > maxLyricsBytes {
		return Lyrics{}, ErrResponse
	}
	var payload struct {
		Code    *int   `json:"code"`
		RetCode *int   `json:"retcode"`
		Lyric   string `json:"lyric"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.Code == nil && payload.RetCode == nil {
		return Lyrics{}, ErrResponse
	}
	if payload.Code != nil && *payload.Code != 0 || payload.RetCode != nil && *payload.RetCode != 0 {
		return Lyrics{}, ErrResponse
	}
	text := payload.Lyric
	if text != "" && !strings.Contains(text, "[") && !strings.Contains(text, "\n") {
		if decoded, err := base64.StdEncoding.DecodeString(text); err == nil && utf8.Valid(decoded) && (strings.Contains(string(decoded), "[") || strings.Contains(string(decoded), "\n")) {
			text = string(decoded)
		}
	}
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return Lyrics{}, ErrResponse
	}
	result, err := parseLyrics(html.UnescapeString(text))
	result.SongID = mid
	return result, err
}

var lyricStamp = regexp.MustCompile(`\[(\d{1,3}):(\d{2})(?:[.:](\d{1,3}))?\]`)
var lyricOffset = regexp.MustCompile(`(?i)^\[offset:([+-]?\d+)\]$`)
var lyricMeta = regexp.MustCompile(`(?i)^\[(?:ti|ar|al|by|re|ve|length|offset):.*\]$`)

func parseLyrics(text string) (Lyrics, error) {
	result := Lyrics{Lines: []LyricLine{}}
	rows := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(rows) > 4000 {
		return result, ErrResponse
	}
	offset := 0
	for _, row := range rows {
		if match := lyricOffset.FindStringSubmatch(strings.TrimSpace(row)); match != nil {
			value, err := strconv.Atoi(match[1])
			if err != nil || value > 3600000 || value < -3600000 {
				return result, ErrResponse
			}
			offset = value
		}
	}
	plain := []string{}
	for _, row := range rows {
		row = strings.TrimSpace(strings.TrimPrefix(row, "\ufeff"))
		if row == "" || lyricMeta.MatchString(row) {
			continue
		}
		if utf8.RuneCountInString(row) > 2000 {
			return result, ErrResponse
		}
		matches := lyricStamp.FindAllStringSubmatchIndex(row, -1)
		end := 0
		var times []float64
		for _, match := range matches {
			if match[0] != end {
				break
			}
			minutes, _ := strconv.Atoi(row[match[2]:match[3]])
			seconds, _ := strconv.Atoi(row[match[4]:match[5]])
			if seconds >= 60 {
				return result, ErrResponse
			}
			fraction := 0.0
			if match[6] >= 0 {
				digits := row[match[6]:match[7]]
				n, _ := strconv.Atoi(digits)
				divisor := 1
				for range len(digits) {
					divisor *= 10
				}
				fraction = float64(n) / float64(divisor)
			}
			timestamp := float64(minutes*60+seconds) + fraction - float64(offset)/1000
			if timestamp < 0 {
				timestamp = 0
			}
			times = append(times, timestamp)
			end = match[1]
		}
		content := strings.TrimSpace(row[end:])
		if len(times) == 0 {
			plain = append(plain, content)
			continue
		}
		for _, timestamp := range times {
			result.Lines = append(result.Lines, LyricLine{Time: timestamp, Text: content})
		}
		if len(result.Lines) > 4000 {
			return Lyrics{Lines: []LyricLine{}}, ErrResponse
		}
	}
	sort.SliceStable(result.Lines, func(i, j int) bool { return result.Lines[i].Time < result.Lines[j].Time })
	combined := []LyricLine{}
	for _, line := range result.Lines {
		if len(combined) > 0 && combined[len(combined)-1].Time == line.Time {
			if line.Text != "" {
				if combined[len(combined)-1].Text != "" {
					combined[len(combined)-1].Text += "\n"
				}
				combined[len(combined)-1].Text += line.Text
			}
		} else {
			combined = append(combined, line)
		}
	}
	result.Lines = combined
	if len(combined) == 0 {
		result.Text = strings.Join(plain, "\n")
	}
	return result, nil
}
