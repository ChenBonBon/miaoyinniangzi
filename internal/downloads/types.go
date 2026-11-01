package downloads

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"miaoyinniangzi/internal/mediastream"
	"miaoyinniangzi/internal/storage"
)

const MaxTasks = 10000

var ErrDownload = errors.New("下载失败，请检查网络、音源或目录后重试")
var ErrDiskFull = errors.New("磁盘空间不足，已保留断点，请释放空间后重试")
var ErrDirectoryUnavailable = errors.New("下载磁盘或目录不可用，请重新连接后手动继续")
var ErrDirectoryPermission = errors.New("无法写入下载目录，请检查磁盘权限后重试")
var ErrPublicationUnsupported = errors.New("此磁盘不支持安全保存，已保留下载数据，请更换目录后重新添加任务")
var ErrPublishedChanged = errors.New("落盘文件已发生变化，未覆盖任何文件，请检查原目录")

type Task struct {
	ID        string       `json:"id"`
	Batch     string       `json:"batch"`
	Song      storage.Song `json:"song"`
	Directory string       `json:"directory"`
	Requested string       `json:"requested"`
	Quality   string       `json:"quality"`
	Codec     string       `json:"codec"`
	State     string       `json:"state"`
	Bytes     int64        `json:"bytes"`
	Total     int64        `json:"total"`
	File      string       `json:"file"`
	Message   string       `json:"message"`
	Created   int64        `json:"created"`
}

// Private checkpoint data never crosses the Wails bridge. URLs are not persisted.
type record struct {
	Task        Task          `json:"task"`
	ETag        string        `json:"etag,omitempty"`
	Resource    string        `json:"resource,omitempty"`
	Source      string        `json:"source,omitempty"`
	Digest      string        `json:"digest,omitempty"`
	Checkpoint  int64         `json:"checkpoint,omitempty"`
	Publication *fileIdentity `json:"publication,omitempty"`
}
type EnqueueResult struct {
	Tasks    []Task `json:"tasks"`
	Existing int    `json:"existing"`
}
type Media struct {
	URL, Quality, Source string
	Info                 mediastream.Info
	Downgraded           bool
}
type Loader func(context.Context, storage.Song, string) (Media, error)
type Store interface {
	SaveDownload(context.Context, string, json.RawMessage) error
	SaveDownloads(context.Context, map[string]json.RawMessage) error
	DownloadRecords(context.Context) ([]json.RawMessage, error)
	RemoveDownload(context.Context, string) error
}

func newID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", ErrDownload
	}
	return hex.EncodeToString(data[:]), nil
}
func rank(q string) int {
	switch q {
	case "flac", "highest":
		return 3
	case "320k":
		return 2
	case "128k":
		return 1
	}
	return 0
}
func active(state string) bool { return state == "queued" || state == "downloading" }
func now() int64               { return time.Now().UnixMilli() }
