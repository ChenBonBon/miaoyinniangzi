import { tick } from 'svelte'
import { toast } from 'svelte-sonner'

export type Page = '推荐' | '喜欢' | '搜索' | '下载' | '设置' | '正在播放' | '音乐详情'
export type SettingsSection = '播放' | '下载' | '音源'
export const navigation = $state({ page: '推荐' as Page, settingsSection: '播放' as SettingsSection, queueOpen: false })
export const browsing = $state({ back: false, forward: false, restoring: false, refreshing: false })
interface Entry { page: Page; data?: unknown; scroll?: number }
interface PageHooks { capture(): unknown; restore(data: any, current: () => boolean): void | Promise<unknown>; refresh(current: () => boolean): void | Promise<unknown> }
const entries: Entry[] = [{ page: '推荐' }]
let index = 0, revision = 0, refreshRevision = 0
let active: { page: Page; hooks: PageHooks } | undefined
const content = () => document.querySelector<HTMLElement>('[data-content-scroll]')
function availability() { browsing.back = index > 0; browsing.forward = index < entries.length - 1 }
function checkpoint() {
  if (active?.page !== navigation.page) return
  if (browsing.restoring) { if (entries[index].data === undefined) entries[index].data = active.hooks.capture(); return }
  entries[index].data = active.hooks.capture()
  entries[index].scroll = content()?.scrollTop ?? 0
}
async function restoreCurrent() {
  const owner = active, entry = entries[index], version = ++revision
  if (!owner || owner.page !== entry.page) return
  browsing.restoring = true
  try {
    await owner.hooks.restore(entry.data, () => version === revision && active === owner && entries[index] === entry)
    await tick()
    await new Promise<void>(resolve => requestAnimationFrame(() => resolve()))
    if (version !== revision || active !== owner || entries[index] !== entry) return
    if (entry.scroll !== undefined && content()) content()!.scrollTop = entry.scroll
    entry.data = owner.hooks.capture()
  } catch { if (version === revision) toast.error('页面恢复失败，请刷新重试') }
  finally { if (version === revision) browsing.restoring = false }
}
// Page adapters own only browsing data. No adapter can restore/start playback.
export function mountNavigationPage(page: Page, hooks: PageHooks) {
  const owner = { page, hooks }; active = owner
  const ready = restoreCurrent()
  return { ready, dispose: () => { if (active === owner) { active = undefined; revision++ } } }
}
export function navigate(page: Page, change?: () => void) {
  if (page === navigation.page && !change) return
  checkpoint()
  const samePage = page === navigation.page
  change?.()
  if (!samePage && content()) content()!.scrollTop = 0
  entries.splice(index + 1)
  entries.push({ page, data: samePage && active?.page === page ? active.hooks.capture() : undefined })
  if (entries.length > 100) entries.shift()
  index = entries.length - 1
  revision++; refreshRevision++; browsing.refreshing = false; availability()
  browsing.restoring = !samePage
  navigation.page = page
}
export function recordBrowse(change: () => void) { navigate(navigation.page, change) }
function travel(delta: number) {
  if (index + delta < 0 || index + delta >= entries.length) return
  checkpoint(); index += delta; revision++; refreshRevision++; browsing.refreshing = false; availability()
  const samePage = navigation.page === entries[index].page
  browsing.restoring = true; navigation.page = entries[index].page
  if (samePage || active?.page === entries[index].page) void restoreCurrent()
}
export function navigationGuard() { const version=revision,entry=entries[index];return () => version===revision && entries[index]===entry }
export function goBack() { travel(-1) }
export function goForward() { travel(1) }
export async function refreshPage() {
  if (browsing.refreshing || browsing.restoring || active?.page !== navigation.page) return
  const owner = active, entry = entries[index], version = ++refreshRevision
  browsing.refreshing = true
  try { await owner.hooks.refresh(() => version === refreshRevision && active === owner && entries[index] === entry); if (active === owner && entries[index] === entry) checkpoint() }
  catch (error) { if (version === refreshRevision) toast.error(error instanceof Error ? error.message : '页面刷新失败，请重试') }
  finally { if (version === refreshRevision) browsing.refreshing = false }
}
export function openNowPlaying() { navigate('正在播放') }
export function closeNowPlaying() { if (browsing.back) goBack(); else navigate('推荐') }
export function closeQueue() {
  navigation.queueOpen = false
  void tick().then(() => document.getElementById('player-queue-toggle')?.focus({ preventScroll: true }))
}
