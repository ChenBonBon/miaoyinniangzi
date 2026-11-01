import { test } from 'node:test'
import assert from 'node:assert/strict'
import { FavoritesController } from '../src/lib/favorites-controller.ts'
const song = id => ({ id, source: 'tx', title: id, artist: 'Artist', album: '', coverUrl: '', songId: '123', mediaMid: 'Media1', duration: 60.125 })
const flush = async () => { for (let i = 0; i < 10; i++) await Promise.resolve() }
function fixture(api = {}) {
  let state
  const notices = [], writes = []
  const controller = new FavoritesController({ load: async () => [], add: async s => writes.push(['add', s.id]), remove: async id => writes.push(['remove', id]), ...api }, value => state = value, message => notices.push(message))
  return { controller, notices, writes, get state() { return state } }
}
test('loads newest-first metadata and fills only absent playback fields', async () => {
  const f = fixture({ load: async () => [song('B'), song('A')] })
  assert.equal(await f.controller.load(), true)
  assert.deepEqual(f.state.songs.map(s => s.id), ['B', 'A'])
  assert.equal(f.state.songs[0].duration, 60.125)
  assert.deepEqual(f.state.songs[0].availableQualities, [])
})
test('initial loads are single flight; failed reads do not pretend to be empty', async () => {
  let finish, calls = 0
  const f = fixture({ load: () => { calls++; return new Promise(resolve => finish = resolve) } })
  const a = f.controller.load(), b = f.controller.load()
  assert.equal(calls, 1)
  finish([song('A')]); await Promise.all([a,b])
  await f.controller.load()
  assert.equal(calls, 1)
  const bad = fixture({ load: async () => { throw Error('private detail') } })
  assert.equal(await bad.controller.load(), false)
  assert.equal(bad.state.status, 'error')
  assert.equal(await bad.controller.toggle(song('A')), false)
  assert.deepEqual(bad.writes, [])
})
test('writes update membership only after success and block duplicate pending toggles', async () => {
  let finish
  const f = fixture({ add: () => new Promise(resolve => finish = resolve) })
  await f.controller.load()
  const pending = f.controller.toggle(song('A')); await flush()
  assert.deepEqual(f.state.songs, [])
  assert.deepEqual(f.state.pending, ['A'])
  assert.equal(await f.controller.toggle(song('A')), false)
  finish(); assert.equal(await pending, true)
  assert.equal(f.state.songs[0].id, 'A')
  assert.deepEqual(f.state.pending, [])
})
test('different writes are serialized and successful adds remain newest first', async () => {
  let finish
  const f = fixture({ add: s => { f.writes.push(['add', s.id]); return s.id === 'A' && f.writes.length === 1 ? new Promise(resolve => finish = resolve) : Promise.resolve() } })
  await f.controller.load()
  const a = f.controller.toggle(song('A')), b = f.controller.toggle(song('B'))
  await flush(); assert.deepEqual(f.writes, [['add','A']])
  finish(); await Promise.all([a,b])
  assert.deepEqual(f.state.songs.map(s => s.id), ['B','A'])
  await f.controller.toggle(song('A'))
  assert.deepEqual(f.state.songs.map(s => s.id), ['B'])
  await f.controller.toggle(song('A'))
  assert.deepEqual(f.state.songs.map(s => s.id), ['A','B'])
})
test('failed writes retain the confirmed snapshot and do not poison following operations', async () => {
  const f = fixture({ load: async () => [song('A')], remove: async () => { throw Error('private path') }, add: async () => {} })
  await f.controller.load()
  assert.equal(await f.controller.toggle(song('A')), false)
  assert.deepEqual(f.state.songs.map(s => s.id), ['A'])
  assert.deepEqual(f.state.pending, [])
  assert.equal(await f.controller.toggle(song('B')), true)
  assert.deepEqual(f.state.songs.map(s => s.id), ['B','A'])
  assert.deepEqual(f.notices, ['取消收藏失败，请重试'])
})
test('refresh is blocked during writes and rejects corrupt or duplicate snapshots', async () => {
  let fail = false
  const f = fixture({ load: async () => fail ? [song('A'), song('A')] : [song('A')] })
  await f.controller.load(); fail = true
  assert.equal(await f.controller.load(true), false)
  assert.equal(f.state.status, 'error')
  assert.deepEqual(f.state.songs.map(s => s.id), ['A'])
  for (const value of [null, [{ ...song('A'), source: 'wy' }], [{ ...song('A'), duration: -1 }]]) {
    const bad = fixture({ load: async () => value })
    assert.equal(await bad.controller.load(), false)
  }
})
