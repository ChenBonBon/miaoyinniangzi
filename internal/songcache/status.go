package songcache

// Status deliberately excludes IDs, URLs, validators, filenames and song names.
type SlotStatus struct {
	State       string `json:"state"`
	Cacheable   bool   `json:"cacheable"`
	Complete    bool   `json:"complete"`
	CachedBytes int64  `json:"cachedBytes"`
	TotalBytes  int64  `json:"totalBytes"`
	PrefixBytes int64  `json:"prefixBytes"`
	PlayerReady bool   `json:"playerReady"`
}
type Status struct {
	Current   SlotStatus   `json:"current"`
	Neighbors []SlotStatus `json:"neighbors"`
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := Status{Neighbors: []SlotStatus{}}
	for key, e := range m.entries {
		status := SlotStatus{State: "preparing"}
		if e.finished {
			status.State = "ready"
			if e.err != nil {
				status.State = "failed"
			}
		}
		if e.disk != nil {
			e.disk.mu.Lock()
			status.Cacheable = true
			status.TotalBytes = e.disk.info.Size
			status.PrefixBytes = e.disk.prefix
			status.Complete = e.disk.available(0) == e.disk.info.Size
			for _, s := range e.disk.spans {
				status.CachedBytes += s.end - s.start
			}
			if e.disk.failed {
				status.State = "failed"
			}
			e.disk.mu.Unlock()
		}
		if e.playerMu.TryLock() {
			status.PlayerReady = e.player != nil
			e.playerMu.Unlock()
		}
		if key == m.current {
			result.Current = status
		} else {
			result.Neighbors = append(result.Neighbors, status)
		}
	}
	return result
}
