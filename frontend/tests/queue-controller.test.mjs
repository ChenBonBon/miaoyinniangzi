import { test } from 'node:test'
import assert from 'node:assert/strict'
import { PlayerController } from '../src/lib/player-controller.ts'

const song = id => ({ id, source: 'tx', title: id, artist: 'Artist', album: '', coverUrl: '', duration: 60, songId: '123', mediaMid: 'Media1' })
const flush = async () => { for (let i = 0; i < 12; i++) await Promise.resolve() }
function fixture({ pending = false, load } = {}) {
  let state, sequence = 0
  const prepares = [], audios = [], cancellations = [], neighborhoods = [], saves = [], notices = []
  const controller = new PlayerController({
    load: load ?? (async () => ({ playback: { queue: [], index: -1, mode: 'sequential', position: 0 }, volume: 1 })),
    save: async s => saves.push(s),
    cancel: async (...args) => { cancellations.push(args); return true },
    neighbors: async (...args) => neighborhoods.push(args),
    prepare: (s, id, previous, next) => {
      const value = { songId: s.id, nativeSession: id, engine: 'libmpv', mediaUrl: '', quality: 'flac', codec: 'flac', duration: 60.125, seekable: true, downgraded: false }
      return new Promise(resolve => { prepares.push({ song: s, previous, next, finish: () => resolve(value) }); if (!pending) resolve(value) })
    }
  }, s => state = s, (_url, events) => {
    const audio = { events, destroyed: false, plays: 0, async play() { this.plays++; events.playing() }, pause() { events.paused() }, seek() {}, volume() {}, destroy() { this.destroyed = true } }
    audios.push(audio)
    return audio
  }, (...args) => notices.push(args), () => `session-${++sequence}`)
  return { controller, prepares, audios, cancellations, neighborhoods, saves, notices, get state() { return state } }
}

test('selecting a queue occurrence preserves the queue and does not restart current playback', async () => {
  const f = fixture()
  f.controller.playFrom(['A', 'B', 'C'].map(song), 0); await flush()
  f.controller.playQueue(0)
  assert.equal(f.prepares.length, 1)
  f.controller.playQueue(2); await flush()
  assert.deepEqual(f.state.queue.map(s => s.id), ['A', 'B', 'C'])
  assert.equal(f.state.index, 2)
  assert.equal(f.state.status, 'playing')
  assert.equal(f.audios[0].destroyed, true)
})

test('removing another occurrence preserves audio, progress and updates cache neighbors', async () => {
  const f = fixture()
  f.controller.playFrom(['A', 'B', 'C', 'D'].map(song), 1); await flush()
  f.audios[0].events.tick(17.25, 60.125, true)
  f.controller.removeQueue(0)
  assert.equal(f.state.index, 0)
  assert.equal(f.state.queue[0].id, 'B')
  assert.equal(f.state.position, 17.25)
  assert.equal(f.audios[0].destroyed, false)
  assert.equal(f.prepares.length, 1)
  assert.equal(f.neighborhoods.at(-1)[3].id, 'D')
  assert.equal(f.neighborhoods.at(-1)[4].id, 'C')
  f.controller.removeQueue(1)
  assert.equal(f.neighborhoods.at(-1)[4].id, 'D')
})

test('removing current playback advances circularly; removing paused current does not autoplay', async () => {
  const f = fixture()
  f.controller.playFrom(['A', 'B', 'C'].map(song), 2); await flush()
  f.controller.removeQueue(2); await flush()
  assert.equal(f.state.queue[f.state.index].id, 'A')
  assert.equal(f.state.status, 'playing')
  f.controller.toggle()
  f.controller.removeQueue(0); await flush()
  assert.equal(f.state.queue[f.state.index].id, 'B')
  assert.equal(f.state.status, 'paused')
  assert.equal(f.prepares.length, 2)
  assert.equal(f.cancellations.at(-1)[1], true)
  f.controller.removeQueue(0)
  assert.equal(f.state.index, -1)
  assert.equal(f.state.status, 'idle')
  assert.deepEqual(f.state.queue, [])
})

test('clear invalidates late prepares and persists an empty stopped queue', async () => {
  const f = fixture({ pending: true })
  f.controller.playFrom(['A', 'B'].map(song), 0)
  f.controller.clearQueue()
  f.prepares[0].finish(); await flush()
  assert.equal(f.audios.length, 0)
  assert.equal(f.state.status, 'idle')
  assert.equal(f.cancellations[0][1], true)
  assert.deepEqual(f.saves.at(-1).queue, [])
  assert.equal(f.saves.at(-1).index, -1)
})

test('moving an earlier occurrence next does not interrupt audio or duplicate it', async () => {
  const f = fixture()
  f.controller.playFrom(['A', 'B', 'C'].map(song), 1); await flush()
  f.audios[0].events.tick(12, 60.125, true)
  assert.equal(f.controller.playNext(f.state.queue[0], 0), true)
  assert.deepEqual(f.state.queue.map(s => s.id), ['B', 'A', 'C'])
  assert.equal(f.state.index, 0)
  assert.equal(f.state.position, 12)
  assert.equal(f.audios[0].destroyed, false)
  assert.equal(f.neighborhoods.at(-1)[4].id, 'A')
  f.controller.next(); await flush()
  assert.equal(f.state.queue[f.state.index].id, 'A')
  assert.equal(f.saves.at(-1).queuedNext, null)
})

test('explicit next overrides shuffle once, survives mode changes, and remaps actual history', async t => {
  t.mock.method(Math, 'random', () => 0)
  const f = fixture()
  f.controller.playFrom(['A', 'B', 'C', 'D'].map(song), 0); await flush()
  f.controller.setMode('shuffle')
  f.controller.next(); await flush() // A -> B
  f.controller.playNext(f.state.queue[3], 3)
  f.controller.setMode('repeat-one')
  f.controller.setMode('shuffle')
  assert.equal(f.state.queue[f.controller.nextIndex()].id, 'D')
  f.controller.next(); await flush()
  assert.equal(f.state.queue[f.state.index].id, 'D')
  f.controller.removeQueue(f.state.queue.findIndex(s => s.id === 'A'))
  f.controller.previous(); await flush()
  assert.equal(f.state.queue[f.state.index].id, 'B')
  assert.ok(f.controller.nextIndex() < f.state.queue.length)
})

test('duplicate IDs are separate queue occurrences when removing or moving', async () => {
  const a = song('A'), b = song('B')
  const f = fixture()
  f.controller.playFrom([a, b, a], 0); await flush()
  f.controller.playNext(a, 2)
  assert.deepEqual(f.state.queue.map(s => s.id), ['A', 'A', 'B'])
  f.controller.removeQueue(1)
  assert.deepEqual(f.state.queue.map(s => s.id), ['A', 'B'])
  assert.equal(f.audios[0].destroyed, false)
})

test('empty-queue insertion stays paused; queue limits and invalid operations are safe', async () => {
  const f = fixture()
  f.controller.playNext(song('A')); await flush()
  assert.equal(f.state.status, 'paused')
  assert.equal(f.prepares.length, 0)
  assert.equal(f.controller.playNext(song('A')), false)
  f.controller.removeQueue(-1); f.controller.playQueue(1.5)
  assert.equal(f.state.queue.length, 1)
  f.controller.playFrom(Array.from({ length: 10000 }, (_, i) => song(`S${i}`)), 0); await flush()
  assert.equal(f.controller.playNext(song('Overflow')), false)
  assert.equal(f.state.queue.length, 10000)
})

test('one-shot next survives paused restore without resolving media', async () => {
  const f = fixture({ load: async () => ({ playback: { queue: ['A','B','C'].map(song), index: 0, position: 3, mode: 'shuffle', queuedNext: 2 }, volume: 1 }) })
  await f.controller.restore()
  assert.equal(f.state.status, 'paused')
  assert.equal(f.prepares.length, 0)
  assert.equal(f.controller.nextIndex(), 2)
  f.controller.next(); await flush()
  assert.equal(f.state.queue[f.state.index].id, 'C')
})
