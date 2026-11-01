import { test } from 'node:test'
import assert from 'node:assert/strict'
import { SettingsAutosave } from '../src/lib/settings-autosave.ts'

const initial = { downloadDirectory: '/Downloads', downloadQuality: 'highest', playbackQuality: 'highest', volume: 0.7 }
const settle = async () => {
  for (let i = 0; i < 8; i++) await Promise.resolve()
}
function setup(t, write = async () => {}) {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const statuses = [],
    successes = [],
    errors = []
  const saver = new SettingsAutosave(initial, write, {
    status: (s) => statuses.push(s),
    success: () => successes.push(true),
    error: (e) => errors.push(e)
  })
  t.after(() => saver.dispose())
  return { saver, statuses, successes, errors }
}

test('initial load does not save; changes debounce for exactly 1000ms', async (t) => {
  const writes = []
  const { saver, successes } = setup(t, async (v) => writes.push(v))
  saver.update({ ...initial })
  t.mock.timers.tick(2000)
  assert.equal(writes.length, 0)
  saver.update({ ...initial, volume: 0.5 })
  t.mock.timers.tick(999)
  assert.equal(writes.length, 0)
  saver.update({ ...initial, volume: 0.4 })
  t.mock.timers.tick(999)
  assert.equal(writes.length, 0)
  t.mock.timers.tick(1)
  await settle()
  assert.equal(writes.length, 1)
  assert.equal(writes[0].volume, 0.4)
  assert.equal(successes.length, 1)
})

test('reverting before the timeout cancels the write', async (t) => {
  const writes = []
  const { saver } = setup(t, async (v) => writes.push(v))
  saver.update({ ...initial, volume: 0.2 })
  saver.update({ ...initial })
  t.mock.timers.tick(2000)
  await settle()
  assert.equal(writes.length, 0)
})

test('writes are serialized and only latest saved state triggers success', async (t) => {
  let release
  const writes = []
  const { saver, successes } = setup(t, (v) => {
    writes.push(v)
    return writes.length === 1 ? new Promise((resolve) => (release = resolve)) : Promise.resolve()
  })
  saver.update({ ...initial, volume: 0.1 })
  t.mock.timers.tick(1000)
  saver.update({ ...initial, volume: 0.2 })
  t.mock.timers.tick(1000)
  assert.equal(writes.length, 1)
  release()
  await settle()
  assert.equal(writes.length, 2)
  assert.equal(writes[1].volume, 0.2)
  assert.equal(successes.length, 1)
})

test('an edit during save still receives its full debounce interval', async (t) => {
  let release
  const writes = []
  const { saver } = setup(t, (v) => {
    writes.push(v)
    return writes.length === 1 ? new Promise((resolve) => (release = resolve)) : Promise.resolve()
  })
  saver.update({ ...initial, volume: 0.1 })
  t.mock.timers.tick(1000)
  saver.update({ ...initial, volume: 0.2 })
  t.mock.timers.tick(500)
  release()
  await settle()
  assert.equal(writes.length, 1)
  t.mock.timers.tick(499)
  assert.equal(writes.length, 1)
  t.mock.timers.tick(1)
  await settle()
  assert.equal(writes.length, 2)
})

test('failed saves retain the draft and can be retried without an infinite loop', async (t) => {
  let attempts = 0
  const { saver, errors, successes, statuses } = setup(t, async () => {
    if (++attempts === 1) throw Error('disk full')
  })
  saver.update({ ...initial, volume: 0.1 })
  t.mock.timers.tick(1000)
  await settle()
  assert.equal(errors.length, 1)
  assert.equal(statuses.at(-1), 'error')
  t.mock.timers.tick(10000)
  await settle()
  assert.equal(attempts, 1)
  saver.retry()
  t.mock.timers.tick(1000)
  await settle()
  assert.equal(attempts, 2)
  assert.equal(successes.length, 1)
})

test('success identifies all changes since the last acknowledged save, including overlapping volume edits', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  let release
  let count = 0
  const acknowledged = []
  const saver = new SettingsAutosave(
    initial,
    async () => {
      if (++count === 1) await new Promise((resolve) => (release = resolve))
    },
    { status: () => {}, success: (saved, previous) => acknowledged.push({ saved, previous }), error: () => {} }
  )
  t.after(() => saver.dispose())
  saver.update({ ...initial, playbackQuality: '320k' })
  t.mock.timers.tick(1000)
  saver.update({ ...initial, playbackQuality: '320k', volume: 0.4 })
  t.mock.timers.tick(1000)
  release()
  await settle()
  assert.equal(acknowledged.length, 1)
  assert.equal(acknowledged[0].previous.playbackQuality, 'highest')
  assert.equal(acknowledged[0].saved.playbackQuality, '320k')
  saver.update({ ...initial, playbackQuality: '320k', volume: 0.3 })
  t.mock.timers.tick(1000)
  await settle()
  assert.equal(acknowledged[1].previous.volume, 0.4)
  assert.equal(acknowledged[1].saved.volume, 0.3)
})

test('disposal cancels the pending debounce', async (t) => {
  let writes = 0
  const { saver } = setup(t, async () => {
    writes++
  })
  saver.update({ ...initial, volume: 0.1 })
  saver.dispose()
  t.mock.timers.tick(1000)
  await settle()
  assert.equal(writes, 0)
})
