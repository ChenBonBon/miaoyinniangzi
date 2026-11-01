import { toast } from 'svelte-sonner'
import {
  PlayerController,
  initialPlayerState,
  type PlayerAPI,
  type AudioEvents,
  type AudioPort,
  type Prepared
} from './player-controller'
import { nativeAudio, type NativeControl } from './native-audio'
import { ensureSettings, settingsState } from './settings-state.svelte'

type Bridge = {
  PrepareSong: PlayerAPI['prepare']
  NativePlayback: NativeControl
  PrefetchNeighbors: NonNullable<PlayerAPI['neighbors']>
  CancelPlayback: PlayerAPI['cancel']
  GetPlayerSession: PlayerAPI['load']
  SavePlayback: PlayerAPI['save']
}
function bridge(): Bridge {
  const host = window as Window & { go?: { main?: { App?: Bridge } } }
  if (!host.go?.main?.App?.PrepareSong) throw Error('请在桌面应用中播放音乐')
  return host.go.main.App
}
const api: PlayerAPI = {
  prepare: (song, id, previous, next) => bridge().PrepareSong(song, id, previous, next),
  neighbors: async (...args) => bridge().PrefetchNeighbors(...args),
  cancel: (id, discardCache) => bridge().CancelPlayback(id, discardCache),
  load: () => bridge().GetPlayerSession(),
  save: (snapshot) => bridge().SavePlayback(snapshot)
}
export const playerState = $state(initialPlayerState())
function audioFactory(_url: string, events: AudioEvents, prepared: Prepared): AudioPort {
  return nativeAudio(prepared.nativeSession!, events, (...args) => bridge().NativePlayback(...args))
}
export const playerController = new PlayerController(
  api,
  (state) => Object.assign(playerState, state),
  audioFactory,
  (message, error) => {
    if (error) toast.error(message, { id: 'playback' })
    else toast.success(message, { id: 'playback' })
  }
)
let initialized = false
let unsubscribe: (() => void) | undefined
export function initializePlayer() {
  const host = window as Window & { go?: { main?: { App?: Partial<Bridge> } } }
  if (initialized || !host.go?.main?.App?.GetPlayerSession) return
  initialized = true
  void playerController.restore()
  const runtime = (window as Window & { runtime?: { EventsOn?: (name: string, callback: () => void) => () => void } })
    .runtime
  unsubscribe = runtime?.EventsOn?.('playback:source-changed', () => playerController.stop())
}
let pendingVolume: number | null = null
let volumeRevision = 0
function volumeNotice(value: number) {
  toast(value === 0 ? '已静音' : `音量 ${Math.round(value * 100)}%`, { id: 'player-volume', duration: 1200 })
}
export function setPlayerVolume(value: number) {
  if (!Number.isFinite(value)) return
  value = Math.max(0, Math.min(1, value))
  const changed = value !== playerState.volume
  const revision = ++volumeRevision
  pendingVolume = value
  if (settingsState.draft) {
    settingsState.draft.volume = value
    pendingVolume = null
  }
  playerController.setVolume(value)
  if (changed) volumeNotice(value)
  if (!settingsState.draft)
    void ensureSettings().then(() => {
      if (revision !== volumeRevision || !settingsState.draft) return
      settingsState.draft.volume = value
      pendingVolume = null
      playerController.setVolume(value)
    })
}
export function togglePlayerMute() {
  const value = playerController.toggleMute()
  setPlayerVolume(value)
  volumeNotice(value)
}
// A settings edit immediately affects the audio engine, without waiting for SQL.
let settingsVolumeSeen: number | undefined
const stop = $effect.root(() => {
  $effect(() => {
    const value = settingsState.draft?.volume
    if (value === undefined) return
    if (pendingVolume !== null) {
      settingsState.draft!.volume = pendingVolume
      settingsVolumeSeen = pendingVolume
      return
    }
    const changed = settingsVolumeSeen !== undefined && settingsVolumeSeen !== value
    settingsVolumeSeen = value
    if (value !== playerState.volume) {
      playerController.setVolume(value)
      if (changed) volumeNotice(value)
    }
  })
})
if (import.meta.hot)
  import.meta.hot.dispose(() => {
    stop()
    unsubscribe?.()
    playerController.dispose()
  })
