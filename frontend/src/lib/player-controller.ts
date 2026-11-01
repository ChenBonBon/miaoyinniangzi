import type { Song } from './music'
import { requestID } from './search-controller.ts'

export type Mode = 'sequential' | 'shuffle' | 'repeat-one'
export interface Snapshot {
  queue: Song[]
  index: number
  position: number
  mode: Mode
  queuedNext?: number | null
}
export interface Prepared {
  engine: 'libmpv'
  nativeSession?: string
  songId: string
  mediaUrl: string
  quality: string
  codec: 'flac' | 'mp3'
  duration: number
  seekable: boolean
  downgraded: boolean
  cacheable?: boolean
}
export interface PlayerAPI {
  prepare(song: Song, id: string, previous: Song | null, next: Song | null): Promise<Prepared>
  neighbors?(id: string, revision: number, song: Song, previous: Song | null, next: Song | null): Promise<unknown>
  cancel(id: string, discardCache: boolean): Promise<boolean>
  load(): Promise<{ playback: Snapshot; volume: number }>
  save(snapshot: Snapshot): Promise<void>
}
export interface AudioPort {
  play(): Promise<void>
  pause(): void
  seek(value: number): boolean | void
  volume(value: number): void
  destroy(): void
}
export interface AudioEvents {
  playing(): void
  paused(): void
  tick(position: number, duration: number, seekable: boolean): void
  seeked(position: number, duration: number, seekable: boolean): void
  ended(): void
  error(): void
  waiting(): void
}
export interface PlayerState extends Snapshot {
  status: 'idle' | 'resolving' | 'buffering' | 'playing' | 'paused' | 'error'
  duration: number
  volume: number
  seekable: boolean
  canPrevious: boolean
  seekTarget: number | null
  quality: string
  codec: string
  error: string
  selectionRevision: number
}
export const initialPlayerState = (): PlayerState => ({
  queue: [],
  index: -1,
  position: 0,
  mode: 'sequential',
  status: 'idle',
  duration: 0,
  volume: 1,
  seekable: false,
  canPrevious: false,
  seekTarget: null,
  quality: '',
  codec: '',
  error: '',
  selectionRevision: 0
})
export class PlayerController {
  private state = initialPlayerState()
  private revision = 0
  private activeID: string | null = null
  private audio: AudioPort | null = null
  private pendingSave: Snapshot | null = null
  private saving = false
  private lastSave = 0
  private saveFailed = false
  private volumeEdited = false
  private lastAudibleVolume = 1
  private hasSession = false
  private shuffleHistory: number[] = []
  private shuffleCursor = 0
  private plannedNext: number | null = null
  private neighborRevision = 0
  private explicitNext: number | null = null
  constructor(
    privateAPI: PlayerAPI,
    privatePublish: (s: PlayerState) => void,
    privateFactory: (url: string, events: AudioEvents, prepared: Prepared) => AudioPort,
    privateNotice: (message: string, error?: boolean) => void = () => {},
    privateID = requestID
  ) {
    this.api = privateAPI
    this.publish = privatePublish
    this.factory = privateFactory
    this.notice = privateNotice
    this.makeID = privateID
  }
  private api: PlayerAPI
  private publish: (s: PlayerState) => void
  private factory: (url: string, e: AudioEvents, prepared: Prepared) => AudioPort
  private notice: (m: string, e?: boolean) => void
  private makeID: () => string
  private update(patch: Partial<PlayerState>) {
    const next = { ...this.state, ...patch }
    next.canPrevious = next.index >= 0 && next.queue.length > 0 && (next.mode !== 'shuffle' || this.shuffleCursor > 0)
    this.state = next
    if (next.mode === 'shuffle') this.nextIndex()
    this.publish(this.state)
  }
  async restore() {
    const revision = this.revision
    try {
      const session = await this.api.load()
      if (revision !== this.revision) return
      const p = session.playback
      if (
        !Array.isArray(p.queue) ||
        p.queue.length > 10000 ||
        !Number.isInteger(p.index) ||
        p.index < -1 ||
        p.index >= p.queue.length ||
        !['sequential', 'shuffle', 'repeat-one'].includes(p.mode) ||
        !Number.isFinite(p.position) ||
        p.position < 0 ||
        (p.queuedNext != null && (!Number.isInteger(p.queuedNext) || p.index < 0 || p.queuedNext < 0 || p.queuedNext >= p.queue.length || p.queuedNext === p.index)) ||
        p.queue.some((s) => !s || s.source !== 'tx' || typeof s.id !== 'string' || !s.id || !s.title)
      )
        throw Error()
      const queue = p.queue.map((s) => ({
        ...s,
        subtitle: s.subtitle ?? '',
        artists: s.artists ?? [],
        availableQualities: s.availableQualities ?? [],
        coverUrl: s.coverUrl ?? '',
        songId: s.songId ?? '',
        mediaMid: s.mediaMid ?? '',
        albumMid: s.albumMid ?? '',
        duration: s.duration ?? 0
      }))
      this.hasSession = true
      this.resetHistory(p.index)
      this.explicitNext = p.queuedNext ?? null
      if (!this.volumeEdited && Number.isFinite(session.volume) && session.volume > 0)
        this.lastAudibleVolume = Math.min(1, session.volume)
      this.update({
        queue,
        index: p.index,
        mode: p.mode,
        position: p.index < 0 ? 0 : p.position,
        status: p.index < 0 ? 'idle' : 'paused',
        duration: queue[p.index]?.duration ?? 0,
        seekable: p.index >= 0 && (queue[p.index]?.duration ?? 0) > 0,
        ...(!this.volumeEdited && Number.isFinite(session.volume)
          ? { volume: Math.max(0, Math.min(1, session.volume)) }
          : {})
      })
    } catch {
      this.notice('无法恢复播放记录，仍可选择歌曲播放', true)
    }
  }
  playFrom(queue: Song[], index: number) {
    if (index < 0 || index >= queue.length || queue.length > 10000) return
    this.hasSession = true
    this.resetHistory(index)
    this.update({ queue: [...queue], index, position: 0 })
    this.persist()
    void this.start(0)
  }
  /** Select an existing occurrence without replacing the rest of the queue. */
  playQueue(index: number) {
    if (!Number.isInteger(index) || !this.state.queue[index]) return
    if (index === this.state.index) {
      if (this.state.status === 'paused' || this.state.status === 'error') this.toggle()
      return
    }
    this.recordVisit(index)
    this.update({ index, position: 0 })
    this.persist()
    void this.start(0)
  }
  private recordVisit(index: number) {
    this.explicitNext = null
    this.plannedNext = null
    if (this.state.mode !== 'shuffle') return
    this.shuffleHistory = this.shuffleHistory.slice(0, this.shuffleCursor + 1)
    if (this.shuffleHistory.at(-1) !== index) this.shuffleHistory.push(index)
    if (this.shuffleHistory.length > 1000) this.shuffleHistory.shift()
    this.shuffleCursor = this.shuffleHistory.length - 1
  }
  // Remap occurrence indices, not song IDs: restored queues can contain repeats.
  private reorderQueue(queue: Song[], order: number[], index: number) {
    const mapping = new Map(order.map((oldIndex, newIndex) => [oldIndex, newIndex]))
    const history: number[] = []
    let cursor = -1
    this.shuffleHistory.forEach((oldIndex, position) => {
      const mapped = mapping.get(oldIndex)
      if (mapped === undefined) return
      history.push(mapped)
      if (position <= this.shuffleCursor) cursor = history.length - 1
    })
    this.shuffleHistory = history
    this.shuffleCursor = cursor
    this.plannedNext = this.plannedNext === null ? null : mapping.get(this.plannedNext) ?? null
    this.explicitNext = this.explicitNext === null ? null : mapping.get(this.explicitNext) ?? null
    this.update({ queue, index })
  }
  removeQueue(index: number) {
    if (!Number.isInteger(index) || !this.state.queue[index]) return
    if (this.state.queue.length === 1) { this.clearQueue(); return }
    const removingCurrent = index === this.state.index
    const continuePlaying = ['playing', 'resolving', 'buffering'].includes(this.state.status)
    const target = removingCurrent ? this.nextIndex()! : this.state.index
    const order = this.state.queue.map((_, i) => i).filter(i => i !== index)
    const newIndex = order.indexOf(target)
    if (removingCurrent) { this.revision++; this.release(!continuePlaying) }
    this.reorderQueue(order.map(i => this.state.queue[i]), order, newIndex)
    if (removingCurrent) {
      this.recordVisit(newIndex)
      this.update({ selectionRevision: this.revision, position: 0, duration: this.state.queue[newIndex]?.duration || 0, status: 'paused', quality: '', codec: '', error: '', seekTarget: null, seekable: (this.state.queue[newIndex]?.duration || 0) > 0 })
    }
    this.persist()
    if (removingCurrent && continuePlaying) void this.start(0)
    else this.notifyNeighbors()
  }
  clearQueue() {
    this.hasSession = true
    this.revision++
    this.release(true)
    this.resetHistory(-1)
    this.update({ ...initialPlayerState(), mode: this.state.mode, volume: this.state.volume })
    this.persist()
  }
  /** Move one occurrence, or insert a search result. Never interrupt current audio. */
  playNext(song: Song, occurrence?: number): boolean {
    if (!song || song.source !== 'tx' || !song.id || !song.title) return false
    const { queue, index } = this.state
    if (occurrence !== undefined && (!Number.isInteger(occurrence) || queue[occurrence]?.id !== song.id)) return false
    if (occurrence === undefined && queue[index]?.id === song.id) return false
    const existing = occurrence ?? queue.findIndex(entry => entry.id === song.id)
    if (existing === index && index >= 0) return false
    if (existing < 0 && queue.length >= 10000) { this.notice('播放队列最多容纳 10000 首歌曲', true); return false }
    if (index < 0) {
      this.hasSession = true
      this.revision++
      const remaining = queue.filter((_, i) => i !== existing)
      this.resetHistory(0)
      this.update({ queue: [song, ...remaining], index: 0, position: 0, duration: song.duration || 0, status: 'paused', seekable: (song.duration || 0) > 0 })
    } else {
      const order = queue.map((_, i) => i).filter(i => i !== existing)
      const current = order.indexOf(index)
      order.splice(current + 1, 0, existing < 0 ? -1 : existing)
      this.reorderQueue(order.map(i => i < 0 ? song : queue[i]), order, current)
      this.explicitNext = current + 1
      this.update({})
      this.notifyNeighbors()
    }
    this.persist()
    this.notice('已设为下一首播放')
    return true
  }
  private release(discardCache = false) {
    const id = this.activeID
    this.activeID = null
    if (id) {
      try {
        void this.api.cancel(id, discardCache).catch(() => {})
      } catch {}
    }
    this.audio?.destroy()
    this.audio = null
  }
  private async start(position: number) {
    const song = this.state.queue[this.state.index]
    if (!song) return
    const revision = ++this.revision
    this.release()
    this.update({
      status: 'resolving',
      selectionRevision: revision,
      error: '',
      quality: '',
      codec: '',
      seekable: false,
      seekTarget: null,
      duration: song.duration || 0,
      position
    })
    try {
      const id = this.makeID()
      this.activeID = id
      const previous = this.previousIndex(), next = this.nextIndex()
      const prepared = await this.api.prepare(song, id, previous === null ? null : this.state.queue[previous], next === null ? null : this.state.queue[next])
      if (revision !== this.revision) return
      if (
        prepared.songId !== song.id ||
        prepared.engine !== 'libmpv' ||
        prepared.nativeSession !== id ||
        prepared.mediaUrl !== '' ||
        !['flac', 'mp3'].includes(prepared.codec)
      )
        throw Error('播放通道返回异常数据')
      const duration = prepared.duration || song.duration || 0
      this.update({
        status: 'buffering',
        quality: prepared.quality,
        codec: prepared.codec,
        duration,
        queue:
          duration > 0 && duration !== song.duration
            ? this.state.queue.map((entry, index) => (index === this.state.index ? { ...entry, duration } : entry))
            : this.state.queue,
        seekable: false
      })
      this.persist()
      let restorePosition = position
      const current = () => revision === this.revision
      this.audio = this.factory(
        prepared.mediaUrl,
        {
          playing: () => {
            if (current()) this.update({ status: 'playing', error: '' })
          },
          paused: () => {
            if (current() && this.state.status !== 'error') {
              this.update({ status: 'paused' })
              this.persist()
            }
          },
          waiting: () => {
            if (current() && this.state.status === 'playing') this.update({ status: 'buffering' })
          },
          tick: (p, d, seekable) => {
            if (!current()) return
            const duration = Number.isFinite(d) && d > 0 ? d : this.state.duration
            if (restorePosition > 0 && seekable) {
              const target = Math.min(restorePosition, Math.max(0, duration - 0.05))
              restorePosition = 0
              this.update({ seekable: prepared.seekable && seekable })
              this.seek(target)
              return
            }
            if (this.state.seekTarget !== null) return
            this.update({
              position: Number.isFinite(p) ? Math.max(0, p) : 0,
              duration,
              seekable: prepared.seekable && seekable
            })
            if (Date.now() - this.lastSave >= 5000) this.persist()
          },
          seeked: (p, d, seekable) => {
            if (!current()) return
            this.update({
              position: Number.isFinite(p) ? Math.max(0, p) : this.state.position,
              duration: Number.isFinite(d) && d > 0 ? d : this.state.duration,
              seekable: prepared.seekable && seekable,
              seekTarget: null
            })
            this.persist()
          },
          ended: () => {
            if (current()) this.onEnded()
          },
          error: () => {
            if (current()) {
              this.update({
                status: 'error',
                error: '音频加载失败，请点击播放重新解析',
                seekTarget: null,
                seekable: false
              })
              this.persist()
              this.notice(this.state.error, true)
            }
          }
        },
        prepared
      )
      this.audio.volume(this.state.volume)
      this.notifyNeighbors()
      // libmpv is already initialized and paused: restore before unpausing so
      // resuming a session cannot briefly play the beginning of the track.
      if (restorePosition > 0 && prepared.seekable && this.state.duration > 0) {
        const target = Math.min(restorePosition, Math.max(0, this.state.duration - 0.05))
        restorePosition = 0
        this.update({ seekable: true })
        this.seek(target)
      }
      const notices: string[] = []
      if (prepared.downgraded) notices.push(`已降至 ${prepared.quality.toUpperCase()} 播放`)
      if (prepared.codec === 'mp3' && prepared.quality === 'flac') notices.push('音源实际返回 MP3，将按 MP3 播放')
      if (prepared.cacheable === false) notices.push('当前歌曲无法建立磁盘缓存，仍按流式播放')
      if (notices.length) this.notice(notices.join('；'), prepared.cacheable === false)
      await this.audio.play()
    } catch (cause) {
      if (revision !== this.revision) return
      const message = cause instanceof Error ? cause.message : String(cause)
      this.update({ status: 'error', error: message || '无法开始播放，请重试', seekTarget: null, seekable: false })
      this.notice(this.state.error, true)
    }
  }
  toggle() {
    if (this.state.status === 'resolving' || this.state.status === 'buffering') {
      this.stop()
      return
    }
    if (this.state.status === 'playing') {
      this.audio?.pause()
      return
    }
    if (this.audio && this.state.status === 'paused') {
      void this.audio.play().catch(() => {
        this.update({ status: 'error', error: '无法继续播放，请重试' })
      })
      return
    }
    void this.start(this.state.position)
  }
  stop() {
    this.revision++
    this.release(true)
    this.update({
      status: this.state.index < 0 ? 'idle' : 'paused',
      seekable: this.state.index >= 0 && this.state.duration > 0,
      seekTarget: null
    })
    this.persist()
  }
  seek(position: number) {
    if (!this.state.seekable || !Number.isFinite(position)) return
    const target = Math.max(0, Math.min(position, this.state.duration))
    if (!this.audio) {
      // Startup restores metadata only. Let users choose the resume position
      // without resolving media or autoplaying; apply it before the next play.
      if (this.state.status === 'paused') {
        this.update({ position: target, seekTarget: null })
        this.persist()
      }
      return
    }
    this.update({ seekTarget: target })
    if (this.audio.seek(target) === false) {
      this.update({ seekTarget: null })
      this.notice('暂时无法跳转进度，请稍后重试', true)
    }
  }
  setVolume(value: number) {
    if (!Number.isFinite(value)) return
    this.volumeEdited = true
    const volume = Math.max(0, Math.min(1, value))
    if (volume > 0) this.lastAudibleVolume = volume
    this.update({ volume })
    this.audio?.volume(volume)
  }
  toggleMute() {
    this.setVolume(this.state.volume > 0 ? 0 : this.lastAudibleVolume)
    return this.state.volume
  }
  private notifyNeighbors() {
    const id = this.activeID, song = this.state.queue[this.state.index]
    if (!id || !song || !this.audio || !this.api.neighbors) return
    const previous = this.previousIndex(), next = this.nextIndex()
    void this.api.neighbors(id, ++this.neighborRevision, song, previous === null ? null : this.state.queue[previous], next === null ? null : this.state.queue[next]).catch(() => {})
  }
  private resetHistory(index: number) {
    this.shuffleHistory = index < 0 ? [] : [index]
    this.shuffleCursor = 0
    this.plannedNext = null
    this.explicitNext = null
  }
  setMode(mode: Mode) {
    const explicit = this.explicitNext
    if (mode !== this.state.mode) this.resetHistory(this.state.index)
    this.explicitNext = explicit
    this.update({ mode })
    this.persist()
    this.notifyNeighbors()
  }
  // Choose once, then reuse until consumed. Future prebuffering and the actual
  // next action must agree; visiting a history entry must not reroll it.
  nextIndex(): number | null {
    const { queue, index, mode } = this.state
    if (!queue.length || index < 0) return null
    if (this.explicitNext !== null && this.explicitNext !== index) return this.explicitNext
    if (mode !== 'shuffle' || queue.length === 1) return (index + 1) % queue.length
    if (this.shuffleCursor < this.shuffleHistory.length - 1) return this.shuffleHistory[this.shuffleCursor + 1]
    if (this.plannedNext === null) {
      let candidate = Math.floor(Math.random() * (queue.length - 1))
      if (candidate >= index) candidate++
      this.plannedNext = candidate
    }
    return this.plannedNext
  }
  previousIndex(): number | null {
    const { queue, index, mode } = this.state
    if (!queue.length || index < 0) return null
    if (mode === 'shuffle') return this.shuffleCursor > 0 ? this.shuffleHistory[this.shuffleCursor - 1] : null
    return (index - 1 + queue.length) % queue.length
  }
  next() {
    const next = this.nextIndex()
    if (next === null) return
    const explicit = this.explicitNext !== null
    this.explicitNext = null
    if (this.state.mode === 'shuffle') {
      if (explicit) this.recordVisit(next)
      else if (this.shuffleCursor < this.shuffleHistory.length - 1) this.shuffleCursor++
      else {
        this.shuffleHistory.push(next)
        if (this.shuffleHistory.length > 1000) this.shuffleHistory.shift()
        this.shuffleCursor = this.shuffleHistory.length - 1
        this.plannedNext = null
      }
    }
    this.update({ index: next, position: 0 })
    this.persist()
    void this.start(0)
  }
  previous() {
    const previous = this.previousIndex()
    if (previous === null) return
    this.explicitNext = null
    if (this.state.mode === 'shuffle') this.shuffleCursor--
    this.update({ index: previous, position: 0 })
    this.persist()
    void this.start(0)
  }
  private onEnded() {
    if (this.state.mode === 'repeat-one') {
      this.audio?.seek(0)
      void this.audio?.play()
      return
    }
    this.next()
  }
  updateCredits(id: string, artists: Song['artists'], albumMid: string) {
    if (!this.state.queue.some(song => song.id === id)) return
    this.update({ queue: this.state.queue.map(song => song.id === id ? { ...song, artists, albumMid } : song) })
    this.persist()
  }
  private persist() {
    if (!this.hasSession) return
    this.lastSave = Date.now()
    this.pendingSave = {
      queue: this.state.queue.map(
        (s) =>
          ({
            id: s.id,
            source: s.source,
            title: s.title,
            artist: s.artist,
            artists: s.artists,
            album: s.album,
            albumMid: s.albumMid,
            coverUrl: s.coverUrl,
            songId: s.songId,
            mediaMid: s.mediaMid,
            duration: s.duration
          }) as Song
      ),
      index: this.state.index,
      position: this.state.position,
      mode: this.state.mode,
      queuedNext: this.explicitNext
    }
    void this.drain()
  }
  private async drain() {
    if (this.saving) return
    this.saving = true
    try {
      while (this.pendingSave) {
        const snapshot = this.pendingSave
        this.pendingSave = null
        try {
          await this.api.save(snapshot)
          this.saveFailed = false
        } catch {
          if (!this.saveFailed) this.notice('播放记录保存失败', true)
          this.saveFailed = true
        }
      }
    } finally {
      this.saving = false
    }
  }
  async prepareQuit() {
    this.stop()
    const deadline=Date.now()+1500
    while((this.saving||this.pendingSave)&&Date.now()<deadline) await new Promise(resolve=>setTimeout(resolve,10))
  }
  dispose() {
    this.stop()
  }
}
