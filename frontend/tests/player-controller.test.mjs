import { test } from 'node:test'
import assert from 'node:assert/strict'
import { PlayerController } from '../src/lib/player-controller.ts'
const song = (id) => ({
  id,
  source: 'tx',
  songId: '123',
  mediaMid: 'Mid1',
  title: id,
  artist: '歌手',
  album: '专辑',
  coverUrl: '',
  duration: 6
})
const prepared = (id) => ({
  songId: id,
  engine: 'libmpv',
  mediaUrl: '',
  quality: 'flac',
  codec: 'flac',
  duration: 6,
  seekable: true,
  downgraded: false
})
const flush = async () => {
  for (let i = 0; i < 8; i++) await Promise.resolve()
}
function fixture(
  load = async () => ({ playback: { queue: [], index: -1, position: 0, mode: 'sequential' }, volume: 1 })
) {
  let state
  const calls = [],
    canceled = [],
    cancelIntents = [],
    audios = [],
    saves = [],
    notices = [],
    neighborhoods = []
  const controller = new PlayerController(
    {
      prepare: (song, id, previous, next) =>
        new Promise((resolve, reject) =>
          calls.push({
            song,
            id,
            previous,
            next,
            resolve: (value) => resolve({ ...value, nativeSession: value.nativeSession ?? id }),
            reject
          })
        ),
      neighbors: async (...args) => { neighborhoods.push(args) },
      cancel: async (id, discardCache) => {
        canceled.push(id)
        cancelIntents.push(discardCache)
        return true
      },
      load,
      save: async (value) => {
        saves.push(value)
      }
    },
    (value) => (state = value),
    (url, events) => {
      const audio = {
        url,
        events,
        seeks: [],
        volumes: [],
        destroyed: false,
        plays: 0,
        async play() {
          this.plays++
          events.playing()
        },
        pause() {
          events.paused()
        },
        seek(value) {
          this.seeks.push(value)
        },
        volume(value) {
          this.volumes.push(value)
        },
        destroy() {
          this.destroyed = true
        }
      }
      audios.push(audio)
      return audio
    },
    (message, error) => notices.push({ message, error }),
    () => `play-${calls.length}`
  )
  return {
    controller,
    calls,
    canceled,
    cancelIntents,
    audios,
    saves,
    notices,
    neighborhoods,
    get state() {
      return state
    }
  }
}
test('real quit preserves paused queue and exact progress without preparing media',async()=>{
 const f=fixture(async()=>({playback:{queue:[song('One')],index:0,position:2.5,mode:'sequential'},volume:1}))
 await f.controller.restore();await f.controller.prepareQuit()
 assert.equal(f.state.status,'paused');assert.equal(f.state.position,2.5);assert.equal(f.calls.length,0)
 assert.equal(f.saves.at(-1).position,2.5);assert.equal(f.saves.at(-1).queue[0].id,'One')
})

test('restored paused progress can be selected locally and is applied before explicit playback', async () => {
  const f = fixture(async () => ({
    playback: { queue: [song('One')], index: 0, position: 1, mode: 'sequential' },
    volume: 1
  }))
  await f.controller.restore()
  assert.equal(f.state.seekable, true)
  f.controller.seek(4)
  assert.equal(f.state.position, 4)
  assert.equal(f.state.status, 'paused')
  assert.equal(f.calls.length, 0)
  assert.equal(f.audios.length, 0)
  await flush()
  assert.equal(f.saves.at(-1).position, 4)
  f.controller.toggle()
  f.calls[0].resolve(prepared('One'))
  await flush()
  assert.deepEqual(f.audios[0].seeks, [4])
})

test('decoded duration is retained in the queue for the next startup', async () => {
  const f = fixture()
  f.controller.playFrom([{ ...song('One'), duration: 0 }], 0)
  f.calls[0].resolve({ ...prepared('One'), duration: 6.125 })
  await flush()
  assert.equal(f.state.queue[0].duration, 6.125)
  assert.equal(f.saves.at(-1).queue[0].duration, 6.125)
})

test('song click replaces queue, prepares and only real audio events mark playing', async () => {
  const f = fixture()
  f.controller.playFrom([song('One'), song('Two')], 0)
  assert.equal(f.state.status, 'resolving')
  assert.equal(f.audios.length, 0)
  f.calls[0].resolve(prepared('One'))
  await flush()
  assert.equal(f.state.status, 'playing')
  assert.equal(f.audios.length, 1)
  assert.equal(f.state.queue.length, 2)
  f.controller.toggle()
  assert.equal(f.state.status, 'paused')
  f.controller.toggle()
  await flush()
  assert.equal(f.state.status, 'playing')
  assert.equal(f.calls.length, 1)
  f.controller.playFrom([song('Other')], 0)
  assert.equal(f.state.queue.length, 1)
  assert.ok(f.audios[0].destroyed)
})
test('rapid selection cancels old preparation and stale callbacks never replace new track', async () => {
  const f = fixture()
  f.controller.playFrom([song('One'), song('Two')], 0)
  f.controller.playFrom([song('One'), song('Two')], 1)
  f.calls[1].resolve(prepared('Two'))
  await flush()
  f.calls[0].resolve(prepared('One'))
  await flush()
  assert.deepEqual(f.canceled, ['play-0'])
  assert.equal(f.state.index, 1)
  assert.equal(f.audios.length, 1)
  f.controller.stop()
  f.audios[0].events.playing()
  assert.equal(f.state.status, 'paused')
})
test('restore retains queue and progress without parsing or autoplay, then resumes at progress', async () => {
  const f = fixture(async () => ({
    playback: { queue: [song('One')], index: 0, position: 3, mode: 'sequential' },
    volume: 0.4
  }))
  await f.controller.restore()
  assert.equal(f.calls.length, 0)
  assert.equal(f.audios.length, 0)
  assert.equal(f.state.status, 'paused')
  assert.equal(f.state.volume, 0.4)
  f.controller.toggle()
  f.calls[0].resolve(prepared('One'))
  await flush()
  f.audios[0].events.tick(0, 6, true)
  assert.deepEqual(f.audios[0].seeks, [3])
  f.audios[0].events.seeked(3, 6, true)
  f.controller.seek(5)
  assert.equal(f.state.position, 3)
  assert.equal(f.state.seekTarget, 5)
  f.audios[0].events.tick(1, 6, true)
  assert.equal(f.state.position, 3)
  f.audios[0].events.seeked(4.9, 6, true)
  assert.equal(f.state.position, 4.9)
  assert.equal(f.state.seekTarget, null)
  f.controller.setVolume(0.7)
  assert.equal(f.audios[0].volumes.at(-1), 0.7)
  assert.ok(!JSON.stringify(f.saves).includes('/media/'))
})
test('mute remembers the last audible volume across slider changes and track selection', async () => {
  const f = fixture(async () => ({
    playback: { queue: [song('One')], index: 0, position: 0, mode: 'sequential' },
    volume: 0.4
  }))
  await f.controller.restore()
  assert.equal(f.controller.toggleMute(), 0)
  assert.equal(f.controller.toggleMute(), 0.4)
  f.controller.setVolume(0.27)
  f.controller.setVolume(0)
  assert.equal(f.controller.toggleMute(), 0.27)
  f.controller.playFrom([song('Two')], 0)
  assert.equal(f.controller.toggleMute(), 0)
  assert.equal(f.controller.toggleMute(), 0.27)
})

test('old startup read cannot overwrite an explicit selection or edited volume', async () => {
  let resolve
  const f = fixture(() => new Promise((yes) => (resolve = yes)))
  const restore = f.controller.restore()
  f.controller.setVolume(0.2)
  f.controller.playFrom([song('New')], 0)
  resolve({ playback: { queue: [song('Old')], index: 0, position: 5, mode: 'sequential' }, volume: 0.8 })
  await restore
  assert.equal(f.state.queue[0].id, 'New')
  assert.equal(f.state.volume, 0.2)
})
test('repeat-one reuses the session and sequential EOF wraps without pretending to play', async () => {
  const f = fixture()
  f.controller.playFrom([song('One'), song('Two')], 0)
  f.calls[0].resolve(prepared('One'))
  await flush()
  f.audios[0].events.ended()
  assert.equal(f.state.index, 1)
  f.calls[1].resolve(prepared('Two'))
  await flush()
  f.controller.setMode('repeat-one')
  f.audios[1].events.ended()
  await flush()
  assert.equal(f.audios[1].seeks.at(-1), 0)
  assert.equal(f.calls.length, 2)
  f.controller.setMode('sequential')
  f.audios[1].events.ended()
  assert.equal(f.state.index, 0)
  assert.equal(f.state.status, 'resolving')
  assert.equal(f.calls.length, 3)
})
test('previous always switches even after three seconds and manual controls wrap in repeat-one', async () => {
  const f = fixture()
  f.controller.playFrom([song('One'), song('Two'), song('Three')], 0)
  f.calls[0].resolve(prepared('One'))
  await flush()
  f.audios[0].events.tick(5, 6, true)
  f.controller.previous()
  assert.equal(f.state.index, 2)
  assert.deepEqual(f.audios[0].seeks, [])
  f.controller.next()
  assert.equal(f.state.index, 0)
  f.controller.setMode('repeat-one')
  f.controller.previous()
  assert.equal(f.state.index, 2)
  f.controller.next()
  assert.equal(f.state.index, 0)
})

test('shuffle plans one next track, traverses real history, and resets for new queues', (t) => {
  t.mock.method(Math, 'random', () => 0.6)
  const f = fixture()
  f.controller.playFrom([song('One'), song('Two'), song('Three')], 0)
  f.controller.setMode('shuffle')
  assert.equal(f.state.canPrevious, false)
  assert.equal(f.controller.previousIndex(), null)
  const next = f.controller.nextIndex()
  assert.notEqual(next, 0)
  assert.equal(f.controller.nextIndex(), next)
  f.controller.next()
  assert.equal(f.state.index, next)
  assert.equal(f.state.canPrevious, true)
  const future = f.controller.nextIndex()
  f.controller.previous()
  assert.equal(f.state.index, 0)
  assert.equal(f.controller.nextIndex(), next)
  f.controller.next()
  assert.equal(f.state.index, next)
  assert.equal(f.controller.nextIndex(), future)
  f.controller.playFrom([song('New')], 0)
  assert.equal(f.state.canPrevious, false)
  assert.equal(f.controller.nextIndex(), 0)
})

test('stream-only fallback remains visible together with a quality downgrade', async () => {
  const f = fixture()
  f.controller.playFrom([song('One')], 0)
  f.calls[0].resolve({ ...prepared('One'), cacheable: false, downgraded: true, quality: '320k', codec: 'mp3' })
  await flush()
  assert.equal(f.state.status, 'playing')
  assert.equal(f.notices.length, 1)
  assert.match(f.notices[0].message, /320K/)
  assert.match(f.notices[0].message, /流式播放/)
  assert.equal(f.notices[0].error, true)
})

test('transitions preserve prepared cache while explicit cancellation discards it', async () => {
  const f = fixture()
  f.controller.playFrom([song('One'), song('Two')], 0)
  f.calls[0].resolve(prepared('One'))
  await flush()
  f.controller.next()
  assert.equal(f.cancelIntents[0], false)
  f.controller.stop()
  assert.equal(f.cancelIntents[1], true)
})

test('cache neighbors wrap and follow the final mode even when changed during preparation', async (t) => {
  t.mock.method(Math, 'random', () => 0.6)
  const f = fixture()
  f.controller.playFrom([song('One'), song('Two'), song('Three')], 0)
  assert.equal(f.calls[0].previous.id, 'Three')
  assert.equal(f.calls[0].next.id, 'Two')
  f.controller.setMode('shuffle')
  const planned = f.controller.nextIndex()
  f.calls[0].resolve(prepared('One'))
  await flush()
  const [id, revision, current, previous, next] = f.neighborhoods.at(-1)
  assert.equal(id, f.calls[0].id)
  assert.ok(revision > 0)
  assert.equal(current.id, 'One')
  assert.equal(previous, null)
  assert.equal(next.id, f.state.queue[planned].id)
  f.controller.setMode('repeat-one')
  assert.equal(f.neighborhoods.at(-1)[3].id, 'Three')
  assert.equal(f.neighborhoods.at(-1)[4].id, 'Two')
  assert.ok(f.neighborhoods.at(-1)[1] > revision)
})

test('empty and one-song queues retain valid indices', () => {
  const f = fixture()
  f.controller.next()
  f.controller.previous()
  assert.equal(f.controller.nextIndex(), null)
  f.controller.playFrom([song('One')], 0)
  f.controller.previous()
  assert.equal(f.state.index, 0)
  f.controller.next()
  assert.equal(f.state.index, 0)
})

test('upstream errors and unsafe playback URLs never create an audio element', async () => {
  for (const result of [
    Error('请启用音源'),
    { ...prepared('One'), mediaUrl: 'https://source.example/private?key=secret' }
  ]) {
    const f = fixture()
    f.controller.playFrom([song('One')], 0)
    if (result instanceof Error) f.calls[0].reject(result)
    else f.calls[0].resolve(result)
    await flush()
    assert.equal(f.state.status, 'error')
    assert.equal(f.audios.length, 0)
  }
})
