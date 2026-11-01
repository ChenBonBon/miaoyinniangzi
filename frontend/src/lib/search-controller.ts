import type { SearchAPI, SearchResult } from './music'

export interface SearchState {
  status: 'idle' | 'loading' | 'success' | 'error' | 'canceled'
  query: string
  page: number
  result: SearchResult | null
  error: string
  fieldError: string
}
export const initialSearchState = (): SearchState => ({ status:'idle', query:'', page:1, result:null, error:'', fieldError:'' })

export function normalizeSearchQuery(input: string): string | null {
 const query=input.trim()
 return input.length>400 || !query || [...query].length>100 || /\p{Cc}/u.test(query) ? null : query
}

export function requestID() {
  if (globalThis.crypto.randomUUID) return globalThis.crypto.randomUUID()
  const bytes = globalThis.crypto.getRandomValues(new Uint8Array(16))
  return Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('')
}

// Owns request ordering independently of component lifetime. Canceling is best
// effort; the revision check still rejects old results if transport cancellation
// is delayed or unavailable.
export class SearchController {
  private api: SearchAPI
  private publish: (state: SearchState) => void
  private makeID: () => string
  private onSubmitted: (query: string, page: number) => void
  private state = initialSearchState()
  private revision = 0
  private requestID: string | null = null

  constructor(api: SearchAPI, publish: (state: SearchState) => void, makeID = requestID, onSubmitted: (query: string, page: number) => void = () => {}) {
    this.api = api; this.publish = publish; this.makeID = makeID; this.onSubmitted = onSubmitted
  }
  private update(patch: Partial<SearchState>) {
    this.state = { ...this.state, ...patch }
    this.publish(this.state)
  }
  private cancelRequest(id: string) {
    try { void this.api.cancel(id).catch(() => {}) } catch { /* Revision still guards stale responses. */ }
  }
  async search(input: string, page = 1): Promise<void> {
    const query = normalizeSearchQuery(input)
    if (query === null) {
      this.update({ fieldError:'请输入 1–100 个字符的歌曲、歌手或专辑名称' })
      return
    }
    if (!Number.isInteger(page) || page < 1 || page > 100) {
      this.update({ status:'error', error:'页码无效，请重新搜索' })
      return
    }
    this.clearFieldError()
    if (this.state.status === 'loading' && query === this.state.query && page === this.state.page) return
    const revision = ++this.revision
    const oldID = this.requestID
    this.requestID = null
    if (oldID) this.cancelRequest(oldID)
    let id: string
    try { id = this.makeID() }
    catch { this.update({ status:'error', query, page, result:null, error:'无法创建搜索请求，请重启应用后重试。', fieldError:'' }); return }
    this.requestID = id
    this.update({ status:'loading', query, page, result:null, error:'', fieldError:'' })
    try { this.onSubmitted(query, page) } catch { /* History errors must not block search. */ }
    try {
      const result = await this.api.search(query, page, id)
      if (revision !== this.revision) return
      if (result.query !== query || result.page !== page || !Array.isArray(result.songs) || result.songs.some(song => song.source !== 'tx')) throw new Error('QQ 音乐搜索返回异常数据，请重试。')
      this.update({ status:'success', result })
    } catch (cause) {
      if (revision !== this.revision) return
      const message = cause instanceof Error ? cause.message : String(cause)
      this.update({ status:'error', error:message || 'QQ 音乐搜索失败，请重试。' })
    } finally {
      if (revision === this.revision) this.requestID = null
    }
  }
  clearFieldError() { if (this.state.fieldError) this.update({fieldError:''}) }
  cancel() {
    const id = this.requestID
    if (!id) return
    this.requestID = null; this.revision++
    this.update({ status:'canceled', result:null, error:'', fieldError:'' })
    this.cancelRequest(id)
  }
  restore(snapshot: SearchState) {
    this.cancel(); this.revision++
    this.update({ ...snapshot, status: snapshot.status === 'loading' ? 'idle' : snapshot.status })
    if (snapshot.status === 'loading') return this.search(snapshot.query, snapshot.page)
  }
  retry() { return this.search(this.state.query, this.state.page) }
}
