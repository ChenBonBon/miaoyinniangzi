import { test } from 'node:test'
import assert from 'node:assert/strict'
import { nativeAudio } from '../src/lib/native-audio.ts'

const flush = async () => {
  for (let i = 0; i < 16; i++) await Promise.resolve()
}
const state = (extra = {}) => ({ position: 1, duration: 6, playing: true, ended: false, ...extra })
function events() {
  const counts = { playing: 0, paused: 0, waiting: 0 }
  return {
    counts,
    handlers: {
      tick() {},
      seeked() {},
      ended() {},
      error() {},
      playing: () => counts.playing++,
      paused: () => counts.paused++,
      waiting: () => counts.waiting++
    }
  }
}

test('late play acknowledgement cannot override a newer pause', async (t) => {
  t.mock.timers.enable({ apis: ['setInterval'] })
  const observed = events()
  let finishPlay
  const audio = nativeAudio('session', observed.handlers, async (_id, action) => {
    if (action === 'play')
      return new Promise((resolve) => {
        finishPlay = resolve
      })
    return state({ playing: false })
  })
  const playing = audio.play()
  await flush()
  audio.pause()
  finishPlay(state())
  await playing
  await flush()
  assert.equal(observed.counts.playing, 0)
  assert.equal(observed.counts.paused, 1)
  audio.destroy()
})

test('network buffering is waiting, not an unsolicited pause', async (t) => {
  t.mock.timers.enable({ apis: ['setInterval'] })
  const observed = events()
  let current = state()
  const audio = nativeAudio('session', observed.handlers, async () => current)
  await audio.play()
  current = state({ playing: false, buffering: true })
  t.mock.timers.tick(150)
  await flush()
  assert.equal(observed.counts.waiting, 1)
  assert.equal(observed.counts.paused, 0)
  current = state()
  t.mock.timers.tick(150)
  await flush()
  assert.equal(observed.counts.playing, 2)
  audio.destroy()
})
