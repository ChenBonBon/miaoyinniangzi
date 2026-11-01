import { listSources, type SourceInfo } from '$lib/sources'

export const sourceState = $state<{ entries: SourceInfo[] }>({ entries: [] })
let pending: Promise<void> | undefined

export function refreshSources(): Promise<void> {
  if (pending) return pending
  pending = Promise.resolve().then(async () => {
    try { sourceState.entries = await listSources() }
    finally { pending = undefined }
  })
  return pending
}
