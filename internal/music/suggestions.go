package music

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"
)

const qqSuggestionURL = "https://c.y.qq.com/splcloud/fcgi-bin/smartbox_new.fcg"
const maxSuggestionBytes = 128 * 1024

// Suggestions are anonymous metadata, independent of source scripts and full
// searches. Never follow redirects, retry restrictions, or return other media.
func (c *QQClient) Suggest(ctx context.Context, input string) (items []Suggestion, err error) {
	query, err := validateQuery(input, 1)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	event := diagnostic{Operation: "suggestions"}
	defer func() { c.diagnostics.record(event, started, err) }()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	endpoint, err := url.Parse(c.suggestionEndpoint)
	if err != nil {
		return nil, ErrNetwork
	}
	params := endpoint.Query()
	params.Set("key", query)
	params.Set("format", "json")
	endpoint.RawQuery = params.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, ErrNetwork
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Referer", "https://y.qq.com/")
	request.Header.Set("User-Agent", "Miaoyin/0.1 (macOS; music metadata)")
	response, err := c.http.Do(request)
	if err != nil {
		return nil, networkError(ctx, err)
	}
	defer response.Body.Close()
	event.HTTPStatus = response.StatusCode
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusTooManyRequests:
		return nil, ErrRateLimit
	case http.StatusForbidden, http.StatusUnauthorized:
		return nil, ErrRestricted
	default:
		return nil, ErrResponse
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxSuggestionBytes+1))
	if err != nil {
		return nil, networkError(ctx, err)
	}
	if len(body) > maxSuggestionBytes {
		return nil, ErrResponse
	}
	return decodeSuggestionsDiagnostic(body, &event)
}
func decodeSuggestions(body []byte) ([]Suggestion, error) {
	return decodeSuggestionsDiagnostic(body, nil)
}
func decodeSuggestionsDiagnostic(body []byte, event *diagnostic) ([]Suggestion, error) {
	var raw struct {
		Code    *int `json:"code"`
		Subcode *int `json:"subcode"`
		Data    *struct {
			Song *struct {
				Items []struct {
					Mid    string `json:"mid"`
					Name   string `json:"name"`
					Singer string `json:"singer"`
				} `json:"itemlist"`
			} `json:"song"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &raw) != nil || raw.Code == nil {
		return nil, ErrResponse
	}
	if event != nil {
		event.Code = raw.Code
		event.Subcode = raw.Subcode
	}
	if *raw.Code != 0 || (raw.Subcode != nil && *raw.Subcode != 0) {
		return nil, ErrRestricted
	}
	if raw.Data == nil {
		return nil, ErrResponse
	}
	items := make([]Suggestion, 0)
	if raw.Data.Song == nil {
		return items, nil
	}
	seen := make(map[string]bool)
	for _, item := range raw.Data.Song.Items {
		title := cleanText(item.Name, 100)
		if !safeID(item.Mid) || title == "" || seen[item.Mid] {
			continue
		}
		if _, err := validateQuery(title, 1); err != nil {
			continue
		}
		seen[item.Mid] = true
		items = append(items, Suggestion{ID: item.Mid, Source: "tx", Title: title, Artist: cleanText(item.Singer, 256)})
		if len(items) == 8 {
			break
		}
	}
	return items, nil
}
