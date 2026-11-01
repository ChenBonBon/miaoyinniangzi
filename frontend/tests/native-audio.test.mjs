import { test } from 'node:test'
import assert from 'node:assert/strict'
import { nativeAudio } from '../src/lib/native-audio.ts'
const flush = async () => {
  for (let i = 0; i < 12; i++) await Promise.resolve()
}
function fixture(t) {
  t.mock.timers.enable({ apis: ['setInterval'] })
  const calls = [],
    ticks = [],
    seeks = []
  let playing = 0,
    paused = 0,
    errors = 0,
    ended = 0
  let state = { position: 0, duration: 240, playing: false, ended: false }
  const audio = nativeAudio(
    'session',
    {
      tick: (...args) => ticks.push(args),
      seeked: (...args) => seeks.push(args),
      playing: () => playing++,
      paused: () => paused++,
      error: () => errors++,
      ended: () => ended++,
      waiting: () => {}
    },
    async (id, action, value) => {
      calls.push({ id, action, value })
      if (action === 'play') state.playing = true
      if (action === 'pause') state.playing = false
      if (action === 'seek') {
        state.position = Math.min(value, 239.999)
        state.ended = false
      }
      return { ...state }
    }
  )
  return {
    audio,
    calls,
    ticks,
    seeks,
    get playing() {
      return playing
    },
    get paused() {
      return paused
    },
    get errors() {
      return errors
    },
    get ended() {
      return ended
    }
  }
}
test('libmpv uses confirmed position, preserves pause at endpoint and serializes seek before resume', async (t) => {
  const f = fixture(t)
  await f.audio.play()
  assert.equal(f.playing, 1)
  f.audio.pause()
  await flush()
  assert.equal(f.paused, 1)
  f.audio.seek(240)
  await flush()
  assert.equal(f.seeks.at(-1)[0], 239.999)
  assert.equal(f.ended, 0)
  f.audio.seek(120)
  await f.audio.play()
  await flush()
  assert.deepEqual(
    f.calls.slice(-2).map((c) => c.action),
    ['seek', 'play']
  )
  assert.equal(f.ticks.at(-1)[0], 120)
  assert.equal(f.seeks.at(-1)[0], 120)
  f.audio.destroy()
  const count = f.calls.length
  t.mock.timers.tick(1000)
  await flush()
  assert.equal(f.calls.length, count)
})
test('disposed native session cannot publish late callbacks', async (t) => {
  t.mock.timers.enable({ apis: ['setInterval'] })
  let finish
  let emitted = 0
  const audio = nativeAudio(
    'session',
    {
      tick: () => emitted++,
      seeked: () => emitted++,
      playing: () => emitted++,
      paused: () => emitted++,
      error: () => emitted++,
      ended: () => emitted++,
      waiting: () => {}
    },
    () => new Promise((resolve) => (finish = resolve))
  )
  audio.seek(230)
  await flush()
  audio.destroy()
  finish({ position: 230, duration: 240, playing: true, ended: false })
  await flush()
  assert.equal(emitted, 0)
})
