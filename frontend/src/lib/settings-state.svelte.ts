import { tick } from 'svelte'
import { toast } from 'svelte-sonner'
import { SettingsAutosave, type SaveStatus } from '$lib/settings-autosave'
import { loadSettings, saveSettings, chooseDownloadDirectory, errorMessage, type Settings } from '$lib/settings'

// Application-scoped, loaded with the settings page. Unmounting a page never
// cancels its pending save or discards edits. No settings fetch on app startup.
export const settingsState = $state({
  draft: null as Settings | null,
  loading: false,
  choosing: false,
  status: 'saved' as SaveStatus,
  error: ''
})

let autosave: SettingsAutosave | undefined
let loading: Promise<void> | undefined

export function ensureSettings(): Promise<void> {
  if (settingsState.draft) return Promise.resolve()
  if (loading) return loading
  settingsState.loading = true
  settingsState.error = ''
  loading = Promise.resolve().then(async () => {
    try {
      const value = await loadSettings()
      autosave = new SettingsAutosave(value, saveSettings, {
        status: (status) => {
          settingsState.status = status
          if (status !== 'error') settingsState.error = ''
        },
        success: (saved, previous) => {
          // Volume gets immediate percentage/mute feedback from the player.
          if (
            saved.downloadDirectory !== previous.downloadDirectory ||
            saved.downloadQuality !== previous.downloadQuality ||
            saved.playbackQuality !== previous.playbackQuality
          )
            toast.success('设置已保存', { id: 'settings-save' })
        },
        error: (cause) => {
          settingsState.error = `保存失败：${errorMessage(cause)}`
          toast.error('设置保存失败', { id: 'settings-save' })
        }
      })
      settingsState.draft = value
    } catch (cause) {
      settingsState.error = errorMessage(cause)
    } finally {
      settingsState.loading = false
      loading = undefined
    }
  })
  return loading
}

export async function refreshSettings() {
  await tick()
  if (settingsState.choosing || settingsState.status !== 'saved') throw Error('请等待设置保存完成后再刷新')
  if (loading) return loading
  autosave?.dispose(); autosave = undefined
  settingsState.draft = null
  await ensureSettings()
  if (settingsState.error) throw Error(settingsState.error)
}

export async function selectDownloadDirectory() {
  if (settingsState.choosing || !settingsState.draft) return
  settingsState.choosing = true
  try {
    const directory = await chooseDownloadDirectory(settingsState.draft.downloadDirectory)
    if (directory && settingsState.draft) settingsState.draft.downloadDirectory = directory
  } catch (cause) {
    settingsState.error = errorMessage(cause)
  } finally {
    settingsState.choosing = false
  }
}

export function retrySettingsSave() {
  autosave?.retry()
}

const stop = $effect.root(() => {
  $effect(() => {
    if (settingsState.draft) autosave?.update({ ...settingsState.draft })
  })
})

if (import.meta.hot)
  import.meta.hot.dispose(() => {
    stop()
    autosave?.dispose()
  })
