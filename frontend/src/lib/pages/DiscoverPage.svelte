<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { ArrowLeft, ChevronLeft, ChevronRight, Disc3, Play, SlidersHorizontal } from '@lucide/svelte'
  import { toast } from 'svelte-sonner'
  import { discoveryLayout } from '$lib/discovery-preferences'
  import { discoveryPreferencesState as preferences, discoveryPreferencesController as preferencesController } from '$lib/discovery-preferences-state.svelte'
  import { Button } from '$lib/components/ui/button'
  import { Skeleton } from '$lib/components/ui/skeleton'
  import * as Alert from '$lib/components/ui/alert'
  import * as Empty from '$lib/components/ui/empty'
  import DownloadButton from '$lib/components/DownloadButton.svelte'
  import SongTable from '$lib/components/music/SongTable.svelte'
  import CollectionCover from '$lib/components/music/CollectionCover.svelte'
  import { discoveryController, discoveryMemory, discoveryState as view } from '$lib/discovery-state.svelte'
  import { discoveryKey, type DiscoveryEntry } from '$lib/discovery-controller'
  import { mountNavigationPage, recordBrowse } from '$lib/navigation.svelte'
  import { playerController } from '$lib/player-state.svelte'

  let root: HTMLDivElement
  let heading = $state<HTMLHeadingElement>()
  let disposed = false, restoring = false, actionRevision = 0
  let Adjuster = $state<typeof import('$lib/components/music/DiscoveryAdjuster.svelte').default | null>(null)
  let adjustOpen = $state(false), adjustLoading = $state(false)
  const groups = $derived(discoveryLayout(view.catalog?.entries ?? [], preferences.value).filter(group => !group.hidden && group.charts.some(chart => !chart.hidden)).map(group => ({ ...group, entries: group.charts.filter(chart => !chart.hidden).map(chart => chart.entry) })))
  async function adjust() {
    adjustLoading = true
    try { Adjuster ??= (await import('$lib/components/music/DiscoveryAdjuster.svelte')).default; adjustOpen = true }
    catch { toast.error('无法打开推荐调整，请重试') }
    finally { adjustLoading = false }
  }
  const songs = $derived(view.collection?.songs ?? [])
  const pages = $derived(Math.max(1, Math.ceil(songs.length / 50)))
  const page = $derived(Math.min(discoveryMemory.page, pages))
  const offset = $derived((page - 1) * 50)
  const visible = $derived(songs.slice(offset, offset + 50))
  $effect(() => { if (view.status === 'ready' && view.collection && discoveryMemory.page > pages) discoveryMemory.page = pages })
  function periodLabel(entry: DiscoveryEntry) {
    const value = entry.updated || entry.period
    const week = /^(\d{4})_(\d{1,2})$/.exec(value)
    return week ? `${week[1]} 年第 ${Number(week[2])} 周` : value
  }
  function pane() { return root?.closest<HTMLElement>('[data-content-scroll]') }
  function remember() {
    const scroll = pane()?.scrollTop ?? 0
    if (view.selected) discoveryMemory.collectionScroll = scroll
    else discoveryMemory.catalogScroll[view.kind] = scroll
  }
  onMount(() => {
    void preferencesController.load()
    const scroll = pane()
    restoring = true
    const listener = () => { if (!restoring) remember() }
    scroll?.addEventListener('scroll', listener, { passive: true })
    const binding = mountNavigationPage('推荐', {
      capture: () => ({ entry: view.selected ? { ...view.selected } : null, page: discoveryMemory.page, scroll: pane()?.scrollTop ?? 0 }),
      restore: async (data, current) => {
        const revision = ++actionRevision
        restoring = true
        if (data) {
          discoveryMemory.page = data.page
          if (data.entry) { discoveryMemory.collectionScroll = data.scroll; await discoveryController.open(data.entry) }
          else { discoveryMemory.catalogScroll.chart = data.scroll; await discoveryController.back() }
        } else await discoveryController.activate()
        await tick()
        if (disposed || !current() || revision !== actionRevision) return
        if (scroll) scroll.scrollTop = view.selected ? discoveryMemory.collectionScroll : discoveryMemory.catalogScroll[view.kind]
        restoring = false
      },
      refresh: () => discoveryController.refresh(),
    })
    return () => { disposed = true; binding.dispose(); scroll?.removeEventListener('scroll', listener); discoveryController.suspend() }
  })
  async function open(entry: DiscoveryEntry) {
    const revision = ++actionRevision
    remember(); restoring = true
    let pending = Promise.resolve()
    recordBrowse(() => { discoveryMemory.focusEntry = discoveryKey(entry); discoveryMemory.collectionScroll = 0; discoveryMemory.page = 1; pending = discoveryController.open(entry) })
    if (pane()) pane()!.scrollTop = 0
    await pending; await tick()
    if (disposed || revision !== actionRevision) return
    if (pane()) pane()!.scrollTop = 0
    heading?.focus({ preventScroll: true })
    restoring = false
  }
  async function back() {
    const revision = ++actionRevision
    restoring = true
    let pending = Promise.resolve()
    recordBrowse(() => { pending = discoveryController.back() }); await pending; await tick()
    if (disposed || revision !== actionRevision) return
    if (pane()) pane()!.scrollTop = discoveryMemory.catalogScroll[view.kind]
    restoring = false
    root.querySelector<HTMLElement>(`[data-discovery-key="${discoveryMemory.focusEntry}"]`)?.focus({ preventScroll: true })
  }
  async function turnPage(value: number) {
    recordBrowse(() => { discoveryMemory.page = value; discoveryMemory.collectionScroll = 0 })
    await tick(); if (pane()) pane()!.scrollTop = 0
    heading?.focus({ preventScroll: true })
  }
</script>

<div bind:this={root} data-discovery-view={view.selected ? 'collection' : 'catalog'}>
  {#if view.selected}
    <div class="mb-6"><Button variant="ghost" size="sm" onclick={back}><ArrowLeft data-icon="inline-start" />返回推荐</Button></div>
    <div class="mb-6 flex min-w-0 items-center gap-5"><CollectionCover src={view.selected.coverUrl} large /><div class="flex min-w-0 flex-col gap-2"><h1 bind:this={heading} tabindex="-1" class="break-words text-2xl leading-tight font-semibold outline-none md:text-3xl">{view.selected.title}</h1><p class="text-sm text-muted-foreground">{periodLabel(view.selected)}{view.status === 'ready' ? `${periodLabel(view.selected) ? ' · ' : ''}${songs.length} 首` : ''}</p></div></div>
    <div class="mb-6 flex flex-wrap items-center gap-3"><Button disabled={view.status !== 'ready' || !songs.length} onclick={() => playerController.playFrom(songs, 0)}><Play data-icon="inline-start" />播放全部</Button><DownloadButton songs={view.status === 'ready' ? songs : []} label="下载全部" /></div>
  {:else}
    <div class="mb-8 flex items-start justify-between gap-4"><div><span class="text-sm text-muted-foreground">今天，听点什么？</span><h1 class="mt-3 mb-4 text-3xl font-semibold tracking-tight md:text-4xl">把时间交给音乐。</h1><p class="text-sm leading-7 text-muted-foreground">从新歌到榜单，发现下一首喜欢的歌。</p></div><Button id="discovery-adjust" variant="outline" size="sm" disabled={adjustLoading || preferences.status !== 'ready' || !view.catalog?.entries.length} onclick={adjust}><SlidersHorizontal data-icon="inline-start" />调整</Button></div>
    {#if preferences.error}<Alert.Root variant="destructive" class="mb-6"><Alert.Title>{preferences.error}</Alert.Title><Button variant="outline" size="sm" class="mt-2" onclick={() => preferencesController.retry()}>{preferences.status === 'error' ? '重试读取布局' : '重试保存布局'}</Button></Alert.Root>{/if}
  {/if}

  <section aria-label={view.selected?.title || '推荐榜单'}>
  {#if view.status === 'loading' || view.status === 'idle' || !view.selected && (preferences.status === 'idle' || preferences.status === 'loading')}
    <div role="status" aria-label="正在读取 QQ 音乐推荐" class="flex flex-col gap-6">{#each [0, 1, 2, 3, 4, 5] as row (row)}<div class="flex items-center gap-4"><Skeleton class={view.selected ? 'size-10' : 'size-16'} /><div class="flex flex-1 flex-col gap-2"><Skeleton class="h-4 w-2/3" /><Skeleton class="h-3 w-1/3" /></div></div>{/each}</div>
  {:else if view.status === 'error'}
    <Alert.Root variant="destructive"><Alert.Title>暂时无法读取推荐</Alert.Title><Alert.Description>{view.error}</Alert.Description><Button variant="outline" class="mt-3 w-fit" onclick={() => discoveryController.refresh()}>重试推荐</Button></Alert.Root>
  {:else if view.selected}
    {#if songs.length}
      <SongTable songs={visible} {offset} onplay={index => playerController.playFrom(songs, offset + index)} />
      {#if pages > 1}<nav aria-label="推荐曲目分页" class="mt-6 flex items-center justify-end gap-3"><Button variant="outline" disabled={page <= 1} onclick={() => turnPage(page - 1)}><ChevronLeft data-icon="inline-start" />上一页</Button><span class="text-xs text-muted-foreground tabular-nums" aria-live="polite">第 {page} / {pages} 页</span><Button variant="outline" disabled={page >= pages} onclick={() => turnPage(page + 1)}>下一页<ChevronRight data-icon="inline-end" /></Button></nav>{/if}
    {:else}<Empty.Root class="py-12"><Empty.Header><Empty.Media variant="icon"><Disc3 /></Empty.Media><Empty.Title>当前列表暂无歌曲</Empty.Title><Empty.Description>可以返回推荐查看其他列表。</Empty.Description></Empty.Header></Empty.Root>{/if}
  {:else if groups.length}
    <div class="flex flex-col gap-10">
      {#each groups as group (group.key)}
        <section aria-label={group.title} data-discovery-group={group.key}>
          <h2 class="mb-4 text-lg font-semibold">{group.title}</h2>
          <div class="grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-2 xl:grid-cols-3">
            {#each group.entries as entry (entry.id)}
              <Button data-discovery-key={discoveryKey(entry)} variant="ghost" class="h-auto min-w-0 justify-start gap-4 px-2 py-3 text-left" aria-label={`查看 ${entry.title}`} onclick={() => open(entry)}><CollectionCover src={entry.coverUrl} /><span class="flex min-w-0 flex-col gap-2"><span class="truncate text-base font-semibold" title={entry.title}>{entry.title}</span>{#if entry.updated || entry.period}<span class="truncate text-xs font-normal text-muted-foreground">{periodLabel(entry)}</span>{/if}</span></Button>
            {/each}
          </div>
        </section>
      {/each}
    </div>
  {:else if view.catalog?.entries.length}<Empty.Root class="py-12"><Empty.Header><Empty.Media variant="icon"><SlidersHorizontal /></Empty.Media><Empty.Title>还没有显示的榜单</Empty.Title><Empty.Description>可在「调整」中重新显示。</Empty.Description></Empty.Header></Empty.Root>
  {:else}<Empty.Root class="py-12"><Empty.Header><Empty.Media variant="icon"><Disc3 /></Empty.Media><Empty.Title>暂时没有推荐列表</Empty.Title><Empty.Description>稍后可以手动刷新。</Empty.Description></Empty.Header></Empty.Root>{/if}
  </section>
</div>
{#if Adjuster}<Adjuster bind:open={adjustOpen} entries={view.catalog?.entries ?? []} />{/if}
