import type { Song } from './music'

export interface FavoritesAPI {
  load(): Promise<unknown>
  add(song: Song): Promise<void>
  remove(id: string): Promise<void>
}
export interface FavoritesState {
  songs: Song[]
  status: 'idle' | 'loading' | 'ready' | 'error'
  pending: string[]
  error: string
}
export const initialFavoritesState = (): FavoritesState => ({ songs: [], status: 'idle', pending: [], error: '' })

function normalize(value: unknown): Song[] {
  if (!Array.isArray(value)) throw Error('invalid favorites')
  const seen = new Set<string>()
  return value.map(item => {
    if (!item || item.source !== 'tx' || typeof item.id !== 'string' || !/^[a-zA-Z0-9]{1,64}$/.test(item.id) || typeof item.title !== 'string' || !item.title || seen.has(item.id)) throw Error('invalid favorite')
    for (const key of ['artist', 'album', 'coverUrl', 'songId', 'mediaMid']) {
      if (item[key] !== undefined && typeof item[key] !== 'string') throw Error('invalid metadata')
    }
    if (item.duration !== undefined && (!Number.isFinite(item.duration) || item.duration < 0)) throw Error('invalid duration')
    seen.add(item.id)
    return { ...item, artist: item.artist ?? '', album: item.album ?? '', coverUrl: item.coverUrl ?? '', songId: item.songId ?? '', mediaMid: item.mediaMid ?? '', duration: item.duration ?? 0, subtitle: '', artists: item.artists ?? [], albumMid: item.albumMid ?? '', availableQualities: [] } as Song
  })
}

export class FavoritesController {
  private state = initialFavoritesState()
  private loading: Promise<boolean> | null = null
  private operations: Promise<unknown> = Promise.resolve()
  private api: FavoritesAPI
  private publish: (state: FavoritesState) => void
  private notice: (message: string) => void
  constructor(api: FavoritesAPI, publish: (state: FavoritesState) => void, notice: (message: string) => void = () => {}) {
    this.api = api
    this.publish = publish
    this.notice = notice
  }
  private update(patch: Partial<FavoritesState>) { this.state = { ...this.state, ...patch }; this.publish(this.state) }
  load(force = false): Promise<boolean> {
    if (this.loading) return this.loading
    if (this.state.pending.length || !force && this.state.status === 'ready') return Promise.resolve(this.state.status === 'ready')
    this.update({ status: 'loading', error: '' })
    this.loading = this.read().finally(() => { this.loading = null })
    return this.loading
  }
  private async read(): Promise<boolean> {
    try {
      const songs = normalize(await this.api.load())
      this.update({ songs, status: 'ready', error: '' })
      return true
    } catch {
      // Keep the last confirmed snapshot. A failed read must not pretend to be
      // an empty collection, or enable writes against an unknown membership.
      this.update({ status: 'error', error: '无法读取收藏，请重试' })
      return false
    }
  }
  toggle(song: Song): Promise<boolean> {
    if (this.state.status !== 'ready' || this.state.pending.includes(song.id)) return Promise.resolve(false)
    let normalized: Song
    try { normalized = normalize([song])[0] } catch { this.notice('歌曲信息无效，无法收藏'); return Promise.resolve(false) }
    const removing = this.state.songs.some(entry => entry.id === song.id)
    this.update({ pending: [...this.state.pending, song.id] })
    // One ordered writer gives successful adds the same newest-first order as
    // SQLite, even if several different buttons are pressed in quick succession.
    const task = this.operations.then(async () => {
      try {
        if (removing) await this.api.remove(song.id)
        else await this.api.add(normalized)
        this.update({ songs: removing ? this.state.songs.filter(entry => entry.id !== song.id) : [normalized, ...this.state.songs.filter(entry => entry.id !== song.id)] })
        return true
      } catch {
        this.notice(removing ? '取消收藏失败，请重试' : '收藏失败，请重试')
        return false
      } finally { this.update({ pending: this.state.pending.filter(id => id !== song.id) }) }
    })
    this.operations = task
    return task
  }
}
