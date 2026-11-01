<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte'
  import { Heart, Play, RotateCcw, ListEnd, ChevronLeft, ChevronRight } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import { Skeleton } from '$lib/components/ui/skeleton'
  import * as Alert from '$lib/components/ui/alert'
  import * as Empty from '$lib/components/ui/empty'
  import * as Table from '$lib/components/ui/table'
  import DownloadButton from '$lib/components/DownloadButton.svelte'
  import FavoriteButton from '$lib/components/favorites/FavoriteButton.svelte'
  import MusicCredit from '$lib/components/music/MusicCredit.svelte'
  import SongCover from '$lib/components/search/SongCover.svelte'
  import { favoritesState, favoritesController } from '$lib/favorites-state.svelte'
  import { mountNavigationPage, recordBrowse } from '$lib/navigation.svelte'
  import { playerController, playerState } from '$lib/player-state.svelte'
  let page = $state(1)
  const pageSize = 50
  const total = $derived(favoritesState.songs.length)
  const pages = $derived(Math.max(1, Math.ceil(total / pageSize)))
  const currentPage = $derived(Math.min(page, pages))
  const offset = $derived((currentPage - 1) * pageSize)
  const visible = $derived(favoritesState.songs.slice(offset, offset + pageSize))
  const ready = $derived(favoritesState.status === 'ready')
  $effect(() => { if (page > pages) page = pages })
  let root: HTMLDivElement
  let heading: HTMLHeadingElement
  let disposed = false
  onMount(() => {
    const binding = mountNavigationPage('喜欢', {
      capture: () => ({ page }),
      restore: async (data, current) => { await favoritesController.load(); if (data && current()) page = data.page },
      refresh: () => favoritesController.load(true),
    })
    return binding.dispose
  })
  onDestroy(() => { disposed = true })
  function duration(value: number) {
    const seconds = Math.floor(value)
    return seconds > 0 ? `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}` : '未知'
  }
  function play(index: number) {
    // The existing queue limit is explicit, never silently truncate a collection.
    playerController.playFrom(total > 10000 ? visible : favoritesState.songs, total > 10000 ? index : offset + index)
  }
  async function removed(index: number, added: boolean) {
    if (added) return
    await tick()
    if (disposed || document.activeElement !== document.body) return
    const next = root.querySelectorAll<HTMLElement>('[data-favorite-row] [data-slot="button"][aria-pressed]')
    const target = next[Math.min(index, next.length - 1)] ?? heading
    target?.focus({ preventScroll: true })
  }
  async function turnPage(value: number) {
    recordBrowse(() => { page = value })
    await tick()
    root.closest<HTMLElement>('[data-content-scroll]')?.scrollTo({ top: 0, behavior: 'auto' })
    heading?.focus({ preventScroll: true })
  }
</script>

<div bind:this={root}>
  <div class="mb-6 flex flex-wrap items-center justify-between gap-3">
    <h1 bind:this={heading} tabindex="-1" class="text-2xl font-semibold outline-none">喜欢的音乐 <span class="ml-1 text-sm font-normal text-muted-foreground tabular-nums">{ready ? total : ''}</span></h1>
    <div class="flex flex-wrap items-center gap-2">
      <DownloadButton songs={ready ? favoritesState.songs : []} label="下载全部" />
      <Button variant="ghost" size="icon" aria-label="刷新收藏" title="刷新收藏" disabled={favoritesState.status === 'loading' || favoritesState.pending.length > 0} onclick={() => favoritesController.load(true)}><RotateCcw /></Button>
      <Button disabled={!ready || !total} onclick={() => playerController.playFrom(total > 10000 ? visible : favoritesState.songs, 0)}><Play data-icon="inline-start" />{total > 10000 ? '播放本页' : '播放全部'}</Button>
    </div>
  </div>
  {#if favoritesState.status === 'loading' || favoritesState.status === 'idle'}
    <div role="status" aria-label="正在读取收藏" class="flex flex-col gap-5">{#each [0, 1, 2, 3, 4] as row (row)}<div class="flex items-center gap-3"><Skeleton class="size-10" /><div class="flex flex-1 flex-col gap-2"><Skeleton class="h-4 w-2/3" /><Skeleton class="h-3 w-1/3" /></div></div>{/each}</div>
  {:else if favoritesState.status === 'error'}
    <Alert.Root variant="destructive"><Alert.Title>无法读取收藏</Alert.Title><Alert.Description>请重试，原有收藏不会被清空。</Alert.Description><Button variant="outline" class="mt-3" onclick={() => favoritesController.load(true)}>重试读取收藏</Button></Alert.Root>
  {:else if !total}
    <Empty.Root class="py-12"><Empty.Header><Empty.Media variant="icon"><Heart /></Empty.Media><Empty.Title>还没有收藏音乐</Empty.Title><Empty.Description>点击歌曲旁的爱心，把喜欢的歌留在这里。</Empty.Description></Empty.Header></Empty.Root>
  {:else}
    <Table.Root class="table-fixed" aria-label="喜欢的音乐列表">
      <Table.Header><Table.Row><Table.Head class="w-10 text-center">#</Table.Head><Table.Head>歌曲</Table.Head><Table.Head class="hidden w-1/5 lg:table-cell">专辑</Table.Head><Table.Head class="hidden w-16 text-right sm:table-cell">时长</Table.Head><Table.Head class="w-28 text-right"><span class="sr-only">收藏操作</span></Table.Head></Table.Row></Table.Header>
      <Table.Body>
        {#each visible as song, index (song.id)}
          <Table.Row data-favorite-row={song.id} class="cursor-pointer" onclick={event => { if (!(event.target instanceof Element) || !event.target.closest('button,a')) play(index) }}>
            <Table.Cell class="text-center"><span class="text-xs text-muted-foreground tabular-nums">{offset + index + 1}</span></Table.Cell>
            <Table.Cell class="py-3"><div class="flex min-w-0 items-center gap-3"><SongCover src={song.coverUrl} /><div class="min-w-0"><Button variant="ghost" class="h-auto max-w-full justify-start p-0" title={song.title} aria-label={`播放收藏 ${song.title}`} onclick={() => play(index)}><span class="truncate">{song.title}</span></Button><div class="mt-1 min-w-0 text-xs text-muted-foreground"><MusicCredit {song} /></div></div></div></Table.Cell>
            <Table.Cell class="hidden lg:table-cell"><div class="min-w-0 text-muted-foreground"><MusicCredit {song} album /></div></Table.Cell>
            <Table.Cell class="hidden text-right sm:table-cell"><span class="text-xs text-muted-foreground tabular-nums">{duration(song.duration)}</span></Table.Cell>
            <Table.Cell><div class="flex items-center justify-end gap-1"><Button variant="ghost" size="icon-sm" disabled={playerState.queue[playerState.index]?.id === song.id} aria-label={`下一首播放 ${song.title}`} title="下一首播放" onclick={() => playerController.playNext(song)}><ListEnd /></Button><FavoriteButton {song} onchange={added => removed(index, added)} /><DownloadButton songs={[song]} /></div></Table.Cell>
          </Table.Row>
        {/each}
      </Table.Body>
    </Table.Root>
    {#if pages > 1}<nav aria-label="收藏分页" class="mt-6 flex items-center justify-end gap-3"><Button variant="outline" disabled={currentPage <= 1} onclick={() => turnPage(currentPage - 1)}><ChevronLeft data-icon="inline-start" />上一页</Button><span class="text-xs text-muted-foreground tabular-nums" aria-live="polite">第 {currentPage} / {pages} 页</span><Button variant="outline" disabled={currentPage >= pages} onclick={() => turnPage(currentPage + 1)}>下一页<ChevronRight data-icon="inline-end" /></Button></nav>{/if}
  {/if}
</div>
