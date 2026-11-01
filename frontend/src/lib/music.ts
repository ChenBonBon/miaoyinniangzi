export interface Artist { id: string; name: string }
export interface Song {
  id: string
  source: 'tx'
  songId: string
  mediaMid: string
  title: string
  subtitle: string
  artist: string
  artists: Artist[]
  album: string
  albumMid: string
  coverUrl: string
  duration: number
  availableQualities: string[]
}
export interface SearchResult {
  query: string
  page: number
  pageSize: number
  total: number
  hasMore: boolean
  songs: Song[]
}
export interface SearchAPI {
  search(query: string, page: number, requestID: string): Promise<SearchResult>
  cancel(requestID: string): Promise<boolean>
}

type MusicBridge = {
  SearchSongs(query: string, page: number, requestID: string): Promise<SearchResult>
  CancelSongSearch(requestID: string): Promise<boolean>
}
function bridge(): MusicBridge {
  const host = window as Window & { go?: { main?: { App?: MusicBridge } } }
  if (!host.go?.main?.App?.SearchSongs) throw new Error('请在 macOS 桌面应用中搜索 QQ 音乐。')
  return host.go.main.App
}
export const musicAPI: SearchAPI = {
  search: (query, page, requestID) => bridge().SearchSongs(query, page, requestID),
  cancel: requestID => bridge().CancelSongSearch(requestID),
}
