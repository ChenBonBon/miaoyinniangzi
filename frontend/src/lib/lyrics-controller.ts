export interface LyricLine {
  time: number
  text: string
}
export interface Lyrics {
  songId: string
  lines: LyricLine[]
  text: string
}
export interface LyricsState extends Lyrics {
  status: 'idle' | 'loading' | 'ready' | 'empty' | 'error'
  error: string
}
export interface LyricsAPI {
  get(mid: string, id: string): Promise<Lyrics>
  cancel(id: string): Promise<unknown>
}
const cache = new Map<string, { value: Lyrics; expires: number }>()
export function currentLyric(lines: LyricLine[], position: number): number {
  let low = 0,
    high = lines.length
  while (low < high) {
    const mid = (low + high) >>> 1
    if (lines[mid].time <= position) low = mid + 1
    else high = mid
  }
  return low - 1
}
export class LyricsController {
  private revision = 0
  private request = ''
  private song = ''
  private disposed = false
  private api: LyricsAPI
  private changed: (state: LyricsState) => void
  private id: () => string
  constructor(api: LyricsAPI, changed: (state: LyricsState) => void, id = () => crypto.randomUUID()) {
    this.api = api
    this.changed = changed
    this.id = id
  }
  async select(songId: string, retry = false) {
    if (this.disposed || (!retry && songId === this.song)) return
    this.song = songId
    const revision = ++this.revision
    if (this.request) void this.api.cancel(this.request).catch(() => {})
    this.request = ''
    const publish = (partial: Partial<LyricsState>) =>
      this.changed({ songId, lines: [], text: '', status: 'idle', error: '', ...partial })
    if (!songId) {
      publish({})
      return
    }
    const cached = cache.get(songId)
    if (!retry && cached && cached.expires > Date.now()) {
      publish({ ...cached.value, status: cached.value.lines.length || cached.value.text ? 'ready' : 'empty' })
      return
    }
    publish({ status: 'loading' })
    try {
      const id = this.id()
      this.request = id
      const value = await this.api.get(songId, id)
      if (this.disposed || revision !== this.revision) return
      if (
        value.songId !== songId ||
        !Array.isArray(value.lines) ||
        value.lines.length > 4000 ||
        typeof value.text !== 'string' ||
        value.text.length > 524288
      )
        throw Error()
      let previous = -1
      for (const line of value.lines) {
        if (
          !Number.isFinite(line.time) ||
          line.time < 0 ||
          line.time <= previous ||
          typeof line.text !== 'string' ||
          line.text.length > 10000
        )
          throw Error()
        previous = line.time
      }
      cache.delete(songId)
      cache.set(songId, { value, expires: Date.now() + 600000 })
      while (cache.size > 20) cache.delete(cache.keys().next().value!)
      publish({ ...value, status: value.lines.length || value.text ? 'ready' : 'empty' })
    } catch {
      if (!this.disposed && revision === this.revision) publish({ status: 'error', error: '歌词暂时无法加载' })
    } finally {
      if (revision === this.revision) this.request = ''
    }
  }
  destroy() {
    this.disposed = true
    this.revision++
    if (this.request) void this.api.cancel(this.request).catch(() => {})
    this.request = ''
  }
}
