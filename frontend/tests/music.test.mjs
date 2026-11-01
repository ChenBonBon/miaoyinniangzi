import { test, afterEach } from 'node:test'
import assert from 'node:assert/strict'
import { musicAPI } from '../src/lib/music.ts'

afterEach(() => { delete globalThis.window })
test('browser preview does not fabricate search results', () => {
  globalThis.window = {}
  assert.throws(() => musicAPI.search('歌曲', 1, 'request-1'), /macOS 桌面/)
})
test('search bridge forwards query, page and cancellation identifier', async () => {
  const calls = []
  const response = { query:'歌曲', page:2, pageSize:20, total:0, hasMore:false, songs:[] }
  globalThis.window = { go:{ main:{ App:{
    SearchSongs:async (...args) => { calls.push(args); return response },
    CancelSongSearch:async id => { calls.push(id); return true },
  } } } }
  assert.deepEqual(await musicAPI.search('歌曲', 2, 'request-1'), response)
  assert.equal(await musicAPI.cancel('request-1'), true)
  assert.deepEqual(calls, [['歌曲',2,'request-1'], 'request-1'])
})
