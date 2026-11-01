import type { Settings } from './settings'

export type SaveStatus = 'saved' | 'waiting' | 'saving' | 'error'
type Callbacks = {
  status: (status: SaveStatus) => void
  success: (saved: Settings, previous: Settings) => void
  error: (error: unknown) => void
}

// Debounce edits, but serialize writes: an older request must never finish
// after a newer one and overwrite it. Edits remain enabled during disk writes.
export class SettingsAutosave {
  private desired: string
  private persisted: string
  private timer: ReturnType<typeof setTimeout> | undefined
  private running = false
  private ready = false
  private disposed = false
  private acknowledged: string

  private write: (value: Settings) => Promise<void>
  private callbacks: Callbacks

  constructor(initial: Settings, write: (value: Settings) => Promise<void>, callbacks: Callbacks) {
    this.write = write
    this.callbacks = callbacks
    this.desired = this.persisted = this.acknowledged = JSON.stringify(initial)
  }

  update(value: Settings) {
    const next = JSON.stringify(value)
    if (this.disposed || next === this.desired) return
    this.desired = next
    this.schedule()
  }

  retry() {
    if (!this.disposed && !this.running && this.desired !== this.persisted) this.schedule()
  }

  private schedule() {
    clearTimeout(this.timer)
    this.ready = false
    if (!this.running && this.desired === this.persisted) {
      this.callbacks.status('saved')
      return
    }
    this.callbacks.status('waiting')
    this.timer = setTimeout(() => {
      this.ready = true
      void this.drain()
    }, 1000)
  }

  private async drain() {
    if (this.disposed || this.running || !this.ready) return
    this.ready = false
    if (this.desired === this.persisted) {
      this.callbacks.status('saved')
      return
    }
    const snapshot = this.desired
    this.running = true
    this.callbacks.status('saving')
    try {
      await this.write(JSON.parse(snapshot) as Settings)
      this.persisted = snapshot
      if (!this.disposed && snapshot === this.desired) {
        this.callbacks.status('saved')
        const previous = JSON.parse(this.acknowledged) as Settings
        this.acknowledged = snapshot
        this.callbacks.success(JSON.parse(snapshot) as Settings, previous)
      }
    } catch (error) {
      if (!this.disposed && snapshot === this.desired) {
        this.callbacks.status('error')
        this.callbacks.error(error)
      }
    } finally {
      this.running = false
      if (!this.disposed && this.ready) void this.drain()
    }
  }

  dispose() {
    this.disposed = true
    clearTimeout(this.timer)
  }
}
