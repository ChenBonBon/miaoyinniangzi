package songcache

import (
	"context"
	"sync"
	"time"

	"miaoyinniangzi/internal/mediastream"
	"miaoyinniangzi/internal/mpvaudio"
)

type Key struct{ Source, Quality, Song string }
type LoadStage uint8

const (
	LoadSnapshot LoadStage = iota + 1
	LoadScriptHit
	LoadScriptPrepare
	LoadRequest
	LoadResolve
	LoadProbe
)

type LoadTiming struct {
	Stage    LoadStage
	Duration time.Duration
}
type Spec struct {
	Remote     string
	Info       mediastream.Info
	Quality    string
	Downgraded bool
	Cacheable  bool
	Timings    []LoadTiming
}
type Loader func(context.Context) (Spec, error)
type Request struct {
	Key  Key
	Load Loader
}
type item struct {
	key      Key
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	finished bool
	err      error
	spec     Spec
	disk     *diskFile
	server   *mediastream.Server
	local    string
	player   *mpvaudio.Player
	playerMu sync.Mutex
	created  time.Time
}
type Manager struct {
	mu       sync.Mutex
	buildMu  sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	raw      mediastream.Fetcher
	slots    *budget
	entries  map[Key]*item
	desired  map[Key]bool
	current  Key
	sequence uint64
	topology uint64
	closed   bool
	open     func(context.Context, string, string) (*mpvaudio.Player, error)
}

func New(ctx context.Context, raw mediastream.Fetcher) *Manager {
	cleanupOnce.Do(func() { go cleanupAbandoned() })
	own, cancel := context.WithCancel(ctx)
	return &Manager{ctx: own, cancel: cancel, raw: raw, slots: newBudget(), entries: map[Key]*item{}, desired: map[Key]bool{}, open: mpvaudio.Open}
}
func (m *Manager) dispose(e *item) {
	e.cancel()
	e.playerMu.Lock()
	if e.player != nil {
		e.player.Close()
		e.player = nil
	}
	e.playerMu.Unlock()
	if e.server != nil {
		_ = e.server.Close()
	}
	if e.disk != nil {
		e.disk.close()
	}
}
func (m *Manager) apply(sequence, topology uint64, current Key, neighbors []Request) bool {
	m.mu.Lock()
	if m.closed || sequence < m.sequence || sequence == m.sequence && topology < m.topology {
		m.mu.Unlock()
		return false
	}
	m.sequence = sequence
	m.topology = topology
	changed := m.current != current
	m.current = current
	desired := map[Key]bool{current: true}
	for _, r := range neighbors {
		if len(desired) < 3 || desired[r.Key] {
			desired[r.Key] = true
		}
	}
	m.desired = desired
	var removed []*item
	for key, e := range m.entries {
		if !desired[key] || changed && !e.finished && key != current {
			delete(m.entries, key)
			e.cancel()
			removed = append(removed, e)
			continue
		}
		if e.disk != nil {
			e.disk.setFull(key == current)
		}
	}
	m.mu.Unlock()
	for _, e := range removed {
		m.dispose(e)
	}
	return true
}
func (m *Manager) ensure(sequence, topology uint64, request Request) (*item, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || sequence != m.sequence || topology != m.topology || !m.desired[request.Key] {
		return nil, false, context.Canceled
	}
	if e := m.entries[request.Key]; e != nil {
		expired := false
		if e.disk != nil {
			e.disk.mu.Lock()
			expired = e.disk.failed || time.Since(e.created) > 5*time.Minute && e.disk.available(0) < e.spec.Info.Size
			e.disk.mu.Unlock()
		}
		if e.finished && (e.err != nil || expired) {
			delete(m.entries, request.Key)
			e.cancel()
			go m.dispose(e)
		} else {
			return e, true, nil
		}
	}
	ctx, cancel := context.WithCancel(m.ctx)
	e := &item{key: request.Key, ctx: ctx, cancel: cancel, done: make(chan struct{}), created: time.Now()}
	m.entries[request.Key] = e
	go m.build(e, request.Load)
	return e, false, nil
}
func (m *Manager) build(e *item, load Loader) {
	m.buildMu.Lock()
	defer m.buildMu.Unlock()
	if e.ctx.Err() != nil {
		m.finish(e, context.Canceled)
		return
	}
	spec, err := load(e.ctx)
	if err != nil {
		m.finish(e, err)
		return
	}
	disk, _ := newDisk(e.ctx, m.raw, spec.Remote, spec.Info, m.slots)
	if disk != nil {
		// Reassembly requires a strong entity validator, not only a matching URL
		// or file size. Unsupported sources retain the existing streaming path.
		if _, checkErr := disk.readAt(e.ctx, make([]byte, 1), 0); checkErr != nil {
			disk.close()
			disk = nil
		}
	}
	spec.Cacheable = disk != nil
	var fetch mediastream.Fetcher = m.raw
	if disk != nil {
		fetch = disk
	}
	server, err := mediastream.New(fetch)
	if err != nil {
		if disk != nil {
			disk.close()
		}
		m.finish(e, err)
		return
	}
	local, _, err := server.InstallPrepared(spec.Remote, spec.Info)
	if err != nil {
		server.Close()
		if disk != nil {
			disk.close()
		}
		m.finish(e, err)
		return
	}
	m.mu.Lock()
	if e.ctx.Err() != nil || m.entries[e.key] != e {
		m.mu.Unlock()
		server.Close()
		if disk != nil {
			disk.close()
		}
		m.finish(e, context.Canceled)
		return
	}
	e.spec = spec
	e.disk = disk
	e.server = server
	e.local = local
	full := m.current == e.key
	if disk != nil {
		disk.setFull(full)
	}
	m.mu.Unlock()
	if !full && disk != nil {
		if err = disk.preparePrefix(e.ctx); err != nil {
			m.finish(e, err)
			return
		}
	}
	if err = m.warmPlayer(e); err != nil {
		m.finish(e, err)
		return
	}
	m.finish(e, nil)
}
func (m *Manager) finish(e *item, err error) {
	m.mu.Lock()
	e.err = err
	e.finished = true
	close(e.done)
	m.mu.Unlock()
	if err != nil {
		e.cancel()
		go m.dispose(e)
	}
}
func (m *Manager) warmPlayer(e *item) error {
	e.playerMu.Lock()
	defer e.playerMu.Unlock()
	return m.warmPlayerLocked(e)
}
func (m *Manager) warmPlayerLocked(e *item) error {
	if e.ctx.Err() != nil {
		return context.Canceled
	}
	if e.player != nil {
		return nil
	}
	if e.disk != nil {
		e.disk.mu.Lock()
		needsPrefix := !e.disk.full && e.disk.prefix == 0
		e.disk.mu.Unlock()
		if needsPrefix {
			if err := e.disk.preparePrefix(e.ctx); err != nil {
				return err
			}
		}
	}
	ctx, cancel := context.WithTimeout(e.ctx, 20*time.Second)
	defer cancel()
	player, err := m.open(ctx, e.local, e.spec.Info.Codec)
	if err != nil {
		return err
	}
	if e.ctx.Err() != nil {
		player.Close()
		return context.Canceled
	}
	e.player = player
	return nil
}

// Use promotes the selected entry and transfers its paused native player to the
// application. The manager continues to own the cache, not that transferred player.
func (m *Manager) Use(ctx context.Context, sequence uint64, current Request, neighbors []Request) (*mpvaudio.Player, Spec, bool, error) {
	if ctx.Err() != nil || !m.apply(sequence, 0, current.Key, neighbors) {
		return nil, Spec{}, false, context.Canceled
	}
	e, hit, err := m.ensure(sequence, 0, current)
	if err != nil {
		return nil, Spec{}, false, err
	}
	select {
	case <-ctx.Done():
		return nil, Spec{}, hit, ctx.Err()
	case <-e.done:
	}
	if e.err != nil {
		return nil, Spec{}, hit, e.err
	}
	e.playerMu.Lock()
	err = m.warmPlayerLocked(e)
	player := e.player
	if err == nil {
		e.player = nil
	}
	e.playerMu.Unlock()
	if err != nil {
		return nil, Spec{}, hit, err
	}
	m.mu.Lock()
	valid := !m.closed && m.sequence == sequence && m.current == current.Key && m.entries[current.Key] == e
	m.mu.Unlock()
	if !valid || ctx.Err() != nil {
		if player != nil {
			player.Close()
		}
		return nil, Spec{}, hit, context.Canceled
	}
	m.Prefetch(sequence, 0, current.Key, neighbors)
	return player, e.spec, hit, nil
}
func (m *Manager) Prefetch(sequence, topology uint64, current Key, neighbors []Request) {
	if !m.apply(sequence, topology, current, neighbors) {
		return
	}
	go func() {
		for _, request := range neighbors {
			if request.Key == current {
				continue
			}
			e, _, err := m.ensure(sequence, topology, request)
			if err != nil {
				continue
			}
			select {
			case <-e.ctx.Done():
				continue
			case <-e.done:
			}
			if e.err != nil {
				continue
			}
			m.mu.Lock()
			valid := !m.closed && m.sequence == sequence && m.topology == topology && m.current == current && m.desired[request.Key]
			m.mu.Unlock()
			if !valid {
				return
			}
			_ = m.warmPlayer(e)
		}
	}()
}
func (m *Manager) CancelPending(sequence uint64) {
	m.mu.Lock()
	if sequence < m.sequence {
		m.mu.Unlock()
		return
	}
	if sequence > m.sequence {
		m.sequence = sequence
		m.topology = 0
	}
	e := m.entries[m.current]
	if e != nil && !e.finished {
		delete(m.entries, m.current)
		e.cancel()
	} else {
		e = nil
	}
	m.mu.Unlock()
	if e != nil {
		m.dispose(e)
	}
}
func (m *Manager) Invalidate(sequence uint64) {
	m.mu.Lock()
	if sequence < m.sequence {
		m.mu.Unlock()
		return
	}
	m.sequence = sequence
	entries := m.drainLocked()
	m.mu.Unlock()
	for _, e := range entries {
		m.dispose(e)
	}
}
func (m *Manager) drainLocked() map[Key]*item {
	entries := m.entries
	m.entries = map[Key]*item{}
	m.desired = map[Key]bool{}
	m.current = Key{}
	m.topology = 0
	for _, e := range entries {
		e.cancel()
	}
	return entries
}
func (m *Manager) Clear() {
	m.mu.Lock()
	entries := m.drainLocked()
	m.mu.Unlock()
	for _, e := range entries {
		m.dispose(e)
	}
}
func (m *Manager) Close() { m.mu.Lock(); m.closed = true; m.cancel(); m.mu.Unlock(); m.Clear() }
