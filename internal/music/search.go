// Package music defines platform-neutral music data. QQ search is independent
// of address-resolution scripts and never reads their keys or enabled state.
package music

import (
	"context"
	"errors"
)

const SearchPageSize = 20
const MaxSearchPage = 100

type Artist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Song struct {
	ID                 string   `json:"id"` // QQ songmid, the stable identifier used by source adapters.
	Source             string   `json:"source"`
	SongID             string   `json:"songId"`
	MediaMid           string   `json:"mediaMid"`
	Title              string   `json:"title"`
	Subtitle           string   `json:"subtitle"`
	Artist             string   `json:"artist"`
	Artists            []Artist `json:"artists"`
	Album              string   `json:"album"`
	AlbumMid           string   `json:"albumMid"`
	CoverURL           string   `json:"coverUrl"`
	Duration           float64  `json:"duration"`
	AvailableQualities []string `json:"availableQualities"`
}
type SearchResult struct {
	Query    string `json:"query"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	Total    int    `json:"total"` // Upstream result count may be capped/approximate.
	HasMore  bool   `json:"hasMore"`
	Songs    []Song `json:"songs"`
}
type Suggestion struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
}
type Suggester interface {
	Suggest(context.Context, string) ([]Suggestion, error)
}

type Searcher interface {
	Search(context.Context, string, int) (SearchResult, error)
}

var (
	ErrQuery      = errors.New("请输入 1–100 个字符的歌曲、歌手或专辑名称")
	ErrPage       = errors.New("页码无效，请重新搜索")
	ErrCanceled   = errors.New("搜索已取消")
	ErrTimeout    = errors.New("QQ 音乐搜索超时，请重试")
	ErrNetwork    = errors.New("无法连接 QQ 音乐，请检查网络后重试")
	ErrRestricted = errors.New("QQ 音乐暂时拒绝搜索请求，请稍后重试")
	ErrRateLimit  = errors.New("搜索过于频繁，请稍后重试")
	ErrResponse   = errors.New("QQ 音乐返回异常数据，请稍后重试")
)
