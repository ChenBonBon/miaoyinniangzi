package downloads

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
)

func canControl(state, action string) bool {
	switch action {
	case "start":
		return state == "paused" || state == "failed"
	case "pause":
		return active(state)
	case "cancel":
		return active(state) || state == "paused" || state == "failed"
	}
	return false
}

// Control applies one durable transition to the entire selection before waking
// or interrupting the worker. Completed files, checkpoints and settings remain
// untouched. An empty selection never means "all" implicitly.
func (m *Manager) Control(ids []string, action string, all bool) error {
	if action != "start" && action != "pause" && action != "cancel" || all && (len(ids) != 0 || action == "cancel") || !all && (len(ids) == 0 || len(ids) > MaxTasks) {
		return errors.New("下载操作无效")
	}
	selected := map[string]bool{}
	for _, id := range ids {
		if len(id) != 32 {
			return errors.New("下载任务标识无效")
		}
		data, err := hex.DecodeString(id)
		if err != nil || len(data) != 16 {
			return errors.New("下载任务标识无效")
		}
		selected[id] = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing {
		return errors.New("下载服务正在关闭")
	}
	staged := map[string]record{}
	payloads := map[string]json.RawMessage{}
	for id, original := range m.records {
		if !all && !selected[id] || !canControl(original.Task.State, action) {
			continue
		}
		next := *original
		switch action {
		case "start":
			next.Task.State = "queued"
			next.Task.Message = ""
		case "pause":
			next.Task.State = "paused"
			next.Task.Message = ""
		case "cancel":
			next.Task.State = "canceled"
			next.Task.Message = "已取消"
		}
		data, err := json.Marshal(next)
		if err != nil {
			return ErrDownload
		}
		staged[id] = next
		payloads[id] = data
	}
	if len(staged) == 0 {
		return nil
	}
	if m.store.SaveDownloads(context.Background(), payloads) != nil {
		return errors.New("下载操作保存失败，请重试")
	}
	for id, next := range staged {
		*m.records[id] = next
		if id == m.running {
			if action == "start" {
				m.restartID = id
			} else if m.stop != nil {
				m.stop()
			}
		}
	}
	if action == "start" {
		m.signal()
	}
	m.pruneCanceledLocked()
	return nil
}
