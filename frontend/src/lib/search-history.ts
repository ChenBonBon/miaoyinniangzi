export interface HistoryAPI {
  list(): Promise<string[]>
  record(query: string): Promise<string[]>
  remove(query: string): Promise<string[]>
  clear(): Promise<string[]>
}
type HistoryBridge = {
  GetSearchHistory(): Promise<string[]>
  RecordSearchHistory(query: string): Promise<string[]>
  RemoveSearchHistory(query: string): Promise<string[]>
  ClearSearchHistory(): Promise<string[]>
}
function bridge(): HistoryBridge {
  const host = window as Window & { go?: { main?: { App?: HistoryBridge } } }
  if (!host.go?.main?.App?.GetSearchHistory) throw new Error('请在 macOS 桌面应用中使用本地搜索历史。')
  return host.go.main.App
}
export const historyAPI: HistoryAPI = {
  list: () => bridge().GetSearchHistory(),
  record: query => bridge().RecordSearchHistory(query),
  remove: query => bridge().RemoveSearchHistory(query),
  clear: () => bridge().ClearSearchHistory(),
}
export interface HistoryState { items: string[]; loaded: boolean; busy: boolean; error: string }
export const initialHistoryState = (): HistoryState => ({ items:[], loaded:false, busy:false, error:'' })

// Serialize reads and edits so a slow initial read cannot overwrite a new tag,
// and clearing during an in-flight record remains cleared after it finishes.
export class HistoryController {
  private api: HistoryAPI
  private publish: (state: HistoryState) => void
  private state = initialHistoryState()
  private tail: Promise<void> = Promise.resolve()
  private pending = 0
  constructor(api: HistoryAPI, publish: (state: HistoryState) => void) { this.api = api; this.publish = publish }
  private update(patch: Partial<HistoryState>) { this.state = { ...this.state, ...patch }; this.publish(this.state) }
  private enqueue(task: () => Promise<string[]>) {
    this.pending++
    this.update({ busy:true, error:'' })
    this.tail = this.tail.then(async () => {
      try { this.update({ items:await task(), loaded:true, error:'' }) }
      catch (cause) { this.update({ error:cause instanceof Error ? cause.message : String(cause) }) }
      finally { this.pending--; this.update({ busy:this.pending > 0 }) }
    })
    return this.tail
  }
  load(force = false) {
    if (!force && this.state.loaded) return Promise.resolve()
    if (!force && this.pending > 0) return this.tail
    return this.enqueue(() => this.api.list())
  }
  record(input: string) {
    const query = input.trim()
    if (!query || query.length > 400 || [...query].length > 100 || /\p{Cc}/u.test(query)) return Promise.resolve()
    return this.enqueue(() => this.api.record(query))
  }
  remove(query: string) { return this.enqueue(() => this.api.remove(query)) }
  clear() { return this.enqueue(() => this.api.clear()) }
}
