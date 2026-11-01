import { test } from 'node:test'
import assert from 'node:assert/strict'
import { LyricsController, currentLyric } from '../src/lib/lyrics-controller.ts'
const lyric = (songId) => ({
  songId,
  lines: [
    { time: 1, text: '<b>text</b>' },
    { time: 3, text: 'second' }
  ],
  text: ''
})
test('lyric lookup follows confirmed time in both directions', () => {
  const lines = lyric('x').lines
  assert.equal(currentLyric(lines, 0), -1)
  assert.equal(currentLyric(lines, 1), 0)
  assert.equal(currentLyric(lines, 5), 1)
  assert.equal(currentLyric(lines, 2), 0)
})
test('switching and disposal cancel independently and reject late lyrics', async () => {
  let state
  const calls = [],
    canceled = []
  const c = new LyricsController(
    {
      get: (song, id) => new Promise((resolve) => calls.push({ song, id, resolve })),
      cancel: async (id) => canceled.push(id)
    },
    (s) => (state = s),
    () => `id${calls.length}`
  )
  const first = c.select('FirstTestSong'),
    second = c.select('SecondTestSong')
  calls[1].resolve(lyric('SecondTestSong'))
  await second
  calls[0].resolve(lyric('FirstTestSong'))
  await first
  assert.equal(state.songId, 'SecondTestSong')
  assert.equal(state.status, 'ready')
  assert.deepEqual(canceled, ['id0'])
  const third = c.select('ThirdTestSong')
  c.destroy()
  calls[2].resolve(lyric('ThirdTestSong'))
  await third
  assert.equal(state.status, 'loading')
})
test('errors retry, empty response is distinct, and invalid data is rejected', async () => {
  let state,
    fail = true
  const c = new LyricsController(
    {
      get: async (id) => {
        if (fail) throw Error('secret')
        return { songId: id, lines: [], text: '' }
      },
      cancel: async () => {}
    },
    (s) => (state = s)
  )
  await c.select('RetryTestSong')
  assert.equal(state.status, 'error')
  assert.ok(!state.error.includes('secret'))
  fail = false
  await c.select('RetryTestSong', true)
  assert.equal(state.status, 'empty')
  c.destroy()
  const invalid = new LyricsController({ get: async () => lyric('wrong'), cancel: async () => {} }, (s) => (state = s))
  await invalid.select('ExpectedTestSong')
  assert.equal(state.status, 'error')
  invalid.destroy()
})
