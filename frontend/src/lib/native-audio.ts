import type { AudioEvents, AudioPort } from './player-controller'
export interface NativeState {
  position: number
  duration: number
  playing: boolean
  ended: boolean
  buffering?: boolean
  seekable?: boolean
}
export type NativeControl = (id: string, action: string, value: number) => Promise<NativeState>

// Serialize mutations so a repeat-one seek cannot be overtaken by play.
export function nativeAudio(session: string, events: AudioEvents, control: NativeControl): AudioPort {
  let disposed = false,
    ended = false,
    lastPlaying = false,
    polling = false,
    generation = 0,
    seekRevision = 0,
    failed = false
  let commands = Promise.resolve()
  function update(state: NativeState) {
    if (disposed) return
    events.tick(state.position, state.duration, state.seekable ?? true)
    if (state.buffering) events.waiting()
    else if (state.playing && !lastPlaying) events.playing()
    if (!state.playing && lastPlaying && !state.ended && !state.buffering) events.paused()
    lastPlaying = state.playing
    if (state.ended && !ended) {
      ended = true
      events.ended()
    }
  }
  function command(action: string, value = 0): Promise<NativeState> {
    const task = commands.then(() => {
      if (disposed) throw Error('播放会话已取消')
      return control(session, action, value)
    })
    commands = task.then(
      () => {},
      () => {}
    )
    return task
  }
  function fail() {
    if (!disposed && !failed) {
      failed = true
      clearInterval(timer)
      events.error()
    }
  }
  const timer = setInterval(() => {
    if (disposed || failed || polling) return
    polling = true
    const revision = generation
    void command('state')
      .then((state) => {
        if (revision === generation) update(state)
      })
      .catch(fail)
      .finally(() => (polling = false))
  }, 150)
  return {
    play: async () => {
      const revision = ++generation
      ended = false
      const state = await command('play')
      if (!disposed && revision === generation) update(state)
    },
    pause: () => {
      const revision = ++generation
      void command('pause')
        .then((state) => {
          if (disposed || revision !== generation) return
          lastPlaying = false
          events.seeked(state.position, state.duration, state.seekable ?? true)
          events.paused()
        })
        .catch(fail)
    },
    seek: (value) => {
      const operation = ++generation
      const revision = ++seekRevision
      ended = false
      void command('seek', value)
        .then((state) => {
          if (disposed || revision !== seekRevision) return
          events.seeked(state.position, state.duration, state.seekable ?? true)
          if (operation === generation) update(state)
        })
        .catch(fail)
      return true
    },
    volume: (value) => {
      void command('volume', value).catch(fail)
    },
    destroy: () => {
      disposed = true
      generation++
      seekRevision++
      clearInterval(timer)
    }
  }
}
