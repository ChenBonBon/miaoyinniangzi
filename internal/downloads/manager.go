package downloads

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"miaoyinniangzi/internal/mediastream"
	"miaoyinniangzi/internal/storage"
)

type Manager struct {
	mu          sync.Mutex
	store       Store
	raw         mediastream.Fetcher
	load        Loader
	records     map[string]*record
	ctx         context.Context
	cancel      context.CancelFunc
	wake        chan struct{}
	done        chan struct{}
	running     string
	stop        context.CancelFunc
	closing     bool
	sequence    int64
	onDuplicate func()
	// Native atomic publisher; tests can inject removable-volume failures.
	publish func(*os.Root, *os.File, fileIdentity, string, string) (fileIdentity, error)
	// A stopped worker may still be unwinding when its task is queued again.
	restartID string
}

func New(store Store, raw mediastream.Fetcher, load Loader) (*Manager, error) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{store: store, raw: raw, load: load, records: map[string]*record{}, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), done: make(chan struct{})}
	records, err := store.DownloadRecords(ctx)
	if err != nil {
		cancel()
		return nil, ErrDownload
	}
	changed := map[string]json.RawMessage{}
	for _, data := range records {
		var r record
		if json.Unmarshal(data, &r) != nil || !validRecord(r) {
			cancel()
			return nil, ErrDownload
		}
		needsSave := active(r.Task.State) || r.Task.State != "success" && r.Task.Bytes != r.Checkpoint
		if active(r.Task.State) {
			r.Task.State = "paused"
			r.Task.Message = "等待手动继续"
		}
		if r.Task.State != "success" {
			r.Task.Bytes = r.Checkpoint
		}
		m.records[r.Task.ID] = &r
		if r.Task.Created > m.sequence {
			m.sequence = r.Task.Created
		}
		if needsSave {
			data, err := json.Marshal(r)
			if err != nil {
				cancel()
				return nil, ErrDownload
			}
			changed[r.Task.ID] = data
		}
	}
	if len(changed) > 0 {
		if err = store.SaveDownloads(ctx, changed); err != nil {
			cancel()
			return nil, ErrDownload
		}
	}
	m.pruneCanceledLocked()
	go m.worker()
	return m, nil
}
func validRecord(r record) bool {
	if r.Publication != nil && (r.Publication.File == 0 || r.Task.File == "" || r.Checkpoint <= 0 || r.Checkpoint != r.Task.Total) {
		return false
	}
	id, err := hex.DecodeString(r.Task.ID)
	if err != nil || len(id) != 16 || !filepath.IsAbs(r.Task.Directory) || r.Task.Song.Source != "tx" || r.Task.Song.ID == "" || rank(r.Task.Requested) == 0 || r.Checkpoint < 0 || r.Checkpoint > mediastream.MaxMediaBytes || r.Task.Total < 0 || r.Task.Total > mediastream.MaxMediaBytes {
		return false
	}
	if r.Task.Bytes < 0 || r.Task.Bytes > mediastream.MaxMediaBytes || r.Checkpoint > r.Task.Total {
		return false
	}
	if r.Checkpoint > 0 {
		sum, err := hex.DecodeString(r.Digest)
		if err != nil || len(sum) != 32 {
			return false
		}
	}
	if r.Task.State == "success" && (r.Task.Total <= 0 || r.Task.Bytes != r.Task.Total || r.Checkpoint != r.Task.Total || r.Task.File == "") {
		return false
	}
	if r.Task.File != "" && (filepath.Base(r.Task.File) != r.Task.File || r.Task.File == "." || r.Task.File == "..") {
		return false
	}
	switch r.Task.State {
	case "queued", "downloading", "success", "failed", "skipped", "canceled", "paused":
		return true
	}
	return false
}
func (m *Manager) saveLocked(r *record) error {
	data, err := json.Marshal(r)
	if err != nil {
		return ErrDownload
	}
	if m.store.SaveDownload(context.Background(), r.Task.ID, data) != nil {
		return errors.New("下载记录保存失败")
	}
	return nil
}
func (m *Manager) update(id string, fn func(*record)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.records[id]
	if r == nil {
		return context.Canceled
	}
	before := *r
	fn(r)
	if err := m.saveLocked(r); err != nil {
		*r = before
		return err
	}
	return nil
}
func (m *Manager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func (m *Manager) List() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	tasks := make([]Task, 0, len(m.records))
	for _, r := range m.records {
		if r.Task.State == "canceled" {
			continue
		}
		tasks = append(tasks, r.Task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Created == tasks[j].Created {
			return tasks[i].ID > tasks[j].ID
		}
		return tasks[i].Created > tasks[j].Created
	})
	return tasks
}
func (m *Manager) SetDuplicateNotice(notice func()) {
	m.mu.Lock()
	m.onDuplicate = notice
	m.mu.Unlock()
}

// Preflight only verified records in the selected directory. Unknown files and
// missing/modified files never suppress a task. Late resolver downgrades are
// checked again by the worker without leaving a skipped history record.
func (m *Manager) Enqueue(songs []storage.Song, directory, quality string) (EnqueueResult, error) {
	result := EnqueueResult{Tasks: []Task{}}
	if len(songs) == 0 || len(songs) > MaxTasks || rank(quality) == 0 {
		return result, ErrDownload
	}
	directory, err := canonicalDirectory(directory)
	if err != nil {
		return result, errors.New("下载目录不可用，请在设置中重新选择")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return result, ErrDownload
	}
	defer root.Close()
	remaining := []storage.Song{}
	seen := map[string]bool{}
	for _, song := range songs {
		if song.Source != "tx" || song.ID == "" || song.Title == "" {
			return result, ErrDownload
		}
		if seen[song.ID] {
			continue
		}
		seen[song.ID] = true
		if m.exists(root, Task{Song: song, Directory: directory}, quality) {
			result.Existing++
		} else {
			remaining = append(remaining, song)
		}
	}
	if len(remaining) == 0 {
		return result, nil
	}
	result.Tasks, err = m.Add(remaining, directory, quality)
	return result, err
}
func (m *Manager) Add(songs []storage.Song, directory, quality string) ([]Task, error) {
	if len(songs) == 0 || len(songs) > MaxTasks || rank(quality) == 0 {
		return nil, ErrDownload
	}
	directory, err := canonicalDirectory(directory)
	if err != nil {
		return nil, errors.New("下载目录不可用，请在设置中重新选择")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing || len(m.records)+len(songs) > MaxTasks {
		return nil, errors.New("下载记录已达上限，请先移除不需要的记录")
	}
	batch, err := newID()
	if err != nil {
		return nil, err
	}
	result := []Task{}
	staged := map[string]*record{}
	payloads := map[string]json.RawMessage{}
	existing := map[string]bool{}
	for _, r := range m.records {
		if active(r.Task.State) && r.Task.Directory == directory && r.Task.Requested == quality {
			existing[r.Task.Song.ID] = true
		}
	}
	sequence := m.sequence
	seen := map[string]bool{}
	for _, song := range songs {
		if song.Source != "tx" || song.ID == "" || song.Title == "" {
			return nil, ErrDownload
		}
		if seen[song.ID] {
			continue
		}
		seen[song.ID] = true
		if existing[song.ID] {
			continue
		}
		id, err := newID()
		if err != nil {
			return result, err
		}
		sequence++
		if sequence < now() {
			sequence = now()
		}
		r := &record{Task: Task{ID: id, Batch: batch, Song: song, Directory: directory, Requested: quality, State: "queued", Created: sequence}}
		data, err := json.Marshal(r)
		if err != nil {
			return nil, ErrDownload
		}
		staged[id] = r
		payloads[id] = data
		result = append(result, r.Task)
	}
	if err = m.store.SaveDownloads(context.Background(), payloads); err != nil {
		return nil, errors.New("无法保存下载任务")
	}
	for id, r := range staged {
		m.records[id] = r
	}
	m.sequence = sequence
	m.signal()
	return result, nil
}
func (m *Manager) Continue(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.records[id]
	if m.closing || r == nil || active(r.Task.State) || r.Task.State == "success" || r.Task.State == "skipped" || r.Task.State == "canceled" {
		return errors.New("当前任务不能继续")
	}
	before := *r
	r.Task.State = "queued"
	r.Task.Message = ""
	if err := m.saveLocked(r); err != nil {
		*r = before
		return err
	}
	if m.running == id {
		m.restartID = id
	}
	m.signal()
	return nil
}
func (m *Manager) Pause(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.records[id]
	if r == nil || !active(r.Task.State) {
		return errors.New("当前任务不能暂停")
	}
	r.Task.State = "paused"
	r.Task.Message = "等待手动继续"
	if m.running == id && m.stop != nil {
		m.stop()
	}
	return m.saveLocked(r)
}
func (m *Manager) CancelBatch(batch string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if batch == "" {
		return errors.New("批次无效")
	}
	return m.stopLocked(batch, "canceled", "已取消")
}
func (m *Manager) stopLocked(batch, state, message string) error {
	changed := map[string]json.RawMessage{}
	for _, r := range m.records {
		if active(r.Task.State) && (batch == "" || r.Task.Batch == batch) {
			if m.running == r.Task.ID && m.stop != nil {
				m.stop()
			}
			r.Task.State = state
			r.Task.Message = message
			data, err := json.Marshal(r)
			if err != nil {
				return ErrDownload
			}
			changed[r.Task.ID] = data
		}
	}
	if len(changed) > 0 && m.store.SaveDownloads(context.Background(), changed) != nil {
		return errors.New("下载记录保存失败")
	}
	if state == "canceled" {
		m.pruneCanceledLocked()
	}
	return nil
}
func (m *Manager) SourceChanged() {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = m.stopLocked("", "paused", "音源已变化，继续时将重新解析")
}

// Cancellation stays durable and hidden until the worker releases its file.
// Failed cleanup is retried at the next startup/control.
func (m *Manager) pruneCanceledLocked() {
	for id, r := range m.records {
		if r.Task.State == "canceled" && m.running != id {
			_ = m.removeLocked(id, false)
		}
	}
}
func (m *Manager) Remove(id string, deleteFile bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeLocked(id, deleteFile)
}
func (m *Manager) removeLocked(id string, deleteFile bool) error {
	r := m.records[id]
	if r == nil || active(r.Task.State) || m.running == id {
		return errors.New("请先暂停任务")
	}
	root, err := os.OpenRoot(r.Task.Directory)
	if err == nil {
		defer root.Close()
		if deleteFile {
			if r.Task.State != "success" || !verified(root, r.Task.File, r.Task.Total, r.Digest) {
				return errors.New("文件已缺失或发生变化，未删除任何文件")
			}
			if err = root.Remove(r.Task.File); err != nil {
				return errors.New("无法删除本地文件")
			}
		}
		// Only remove our generated checkpoint after validating its recorded hash.
		if r.Task.State != "success" && r.Checkpoint > 0 && verified(root, partName(id), r.Checkpoint, r.Digest) {
			_ = root.Remove(partName(id))
		} else if r.Task.State != "success" && r.Checkpoint == 0 {
			if file, openErr := regular(root, partName(id), false, false); openErr == nil {
				info, statErr := file.Stat()
				file.Close()
				if statErr == nil && info.Size() == 0 {
					_ = root.Remove(partName(id))
				}
			}
		}
	} else if deleteFile {
		return errors.New("无法打开原下载目录")
	}
	if m.store.RemoveDownload(context.Background(), id) != nil {
		return errors.New("无法移除下载记录")
	}
	delete(m.records, id)
	return nil
}
func (m *Manager) worker() {
	defer close(m.done)
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.wake:
		}
		for {
			m.mu.Lock()
			var selected *record
			for _, r := range m.records {
				if r.Task.State == "queued" && (selected == nil || r.Task.Created < selected.Task.Created) {
					selected = r
				}
			}
			if selected == nil || m.closing {
				m.mu.Unlock()
				break
			}
			id := selected.Task.ID
			ctx, cancel := context.WithTimeout(m.ctx, 20*time.Minute)
			m.running = id
			m.stop = cancel
			selected.Task.Message = "正在准备下载"
			err := m.saveLocked(selected)
			snapshot := *selected
			m.mu.Unlock()
			if err == nil {
				err = m.run(ctx, snapshot)
			}
			cancel()
			m.mu.Lock()
			if r := m.records[id]; r != nil {
				if err != nil && active(r.Task.State) && m.restartID != id {
					if m.closing || m.ctx.Err() != nil {
						r.Task.State = "paused"
						r.Task.Message = "等待手动继续"
					} else {
						r.Task.State = "failed"
						r.Task.Message = failureMessage(err)
					}
				}
				_ = m.saveLocked(r)
			}
			if m.restartID == id {
				m.restartID = ""
			}
			m.running = ""
			m.stop = nil
			m.pruneCanceledLocked()
			m.mu.Unlock()
		}
	}
}
func (m *Manager) Close() {
	m.mu.Lock()
	m.closing = true
	m.cancel()
	_ = m.stopLocked("", "paused", "等待手动继续")
	if m.stop != nil {
		m.stop()
	}
	m.mu.Unlock()
	<-m.done
}
