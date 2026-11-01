import { test, afterEach } from 'node:test'
import assert from 'node:assert/strict'
import { loadSettings, saveSettings, chooseDownloadDirectory, errorMessage } from '../src/lib/settings.ts'

afterEach(() => { delete globalThis.window })

test('browser preview rejects persistence instead of faking success', () => {
  globalThis.window = {}
  assert.throws(() => loadSettings(), /桌面应用/)
  assert.throws(() => saveSettings({}), /桌面应用/)
  assert.throws(() => chooseDownloadDirectory('/draft/Documents'), /桌面应用/)
})

test('desktop bridge reads, saves and preserves canceled directory selection', async () => {
  const settings = { downloadDirectory:'/Users/test/Downloads', downloadQuality:'highest', playbackQuality:'flac', volume:0.5 }
  let saved
  globalThis.window = { go: { main: { App: {
    GetSettings: async () => settings,
    SaveSettings: async (value) => { saved = value },
    ChooseDownloadDirectory: async (initial) => { assert.equal(initial, '/draft/Documents'); return '' },
  } } } }
  assert.deepEqual(await loadSettings(), settings)
  await saveSettings(settings)
  assert.deepEqual(saved, settings)
  assert.equal(await chooseDownloadDirectory('/draft/Documents'), '')
})

test('backend errors propagate to the caller', async () => {
  globalThis.window = { go: { main: { App: { GetSettings: async () => { throw new Error('database unavailable') } } } } }
  await assert.rejects(loadSettings(), /database unavailable/)
  assert.equal(errorMessage('permission denied'), 'permission denied')
  assert.equal(errorMessage(new Error('failed')), 'failed')
})
