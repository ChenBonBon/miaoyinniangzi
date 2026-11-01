<script lang="ts">
  import { ChevronLeft, ChevronRight, ListEnd } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import * as Table from '$lib/components/ui/table'
  import MusicCredit from '$lib/components/music/MusicCredit.svelte'
  import SongCover from './SongCover.svelte'
  import ResolveSongAction from './ResolveSongAction.svelte'
  import DownloadButton from '$lib/components/DownloadButton.svelte'
  import FavoriteButton from '$lib/components/favorites/FavoriteButton.svelte'
  import type { SearchResult } from '$lib/music'
  import { playerController, playerState } from '$lib/player-state.svelte'

  let { result, onpage }: { result: SearchResult; onpage: (page: number) => void } = $props()
  function duration(seconds: number) {
    if (!Number.isFinite(seconds) || seconds <= 0) return '未知'
    return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
  }
</script>

<div class="mb-5 flex flex-wrap items-baseline justify-between gap-3">
  <h2 class="min-w-0 break-words text-lg font-semibold">「{result.query}」的搜索结果</h2>
  <div class="flex flex-wrap items-center gap-3"><span class="text-xs text-muted-foreground">{result.total > 0 ? `约 ${result.total.toLocaleString()} 首` : ''}</span><DownloadButton songs={result.songs} label="下载本页" /></div>
</div>
<Table.Root class="table-fixed" aria-label="QQ 音乐搜索结果">
  <Table.Header><Table.Row>
    <Table.Head class="w-10 text-center">#</Table.Head>
    <Table.Head>歌曲</Table.Head>
    <Table.Head class="hidden w-1/5 md:table-cell">歌手</Table.Head>
    <Table.Head class="hidden w-1/4 md:table-cell">专辑</Table.Head>
    <Table.Head class="w-16 text-right">时长</Table.Head>
    <Table.Head class="w-44 text-right"><span class="sr-only">歌曲操作</span></Table.Head>
  </Table.Row></Table.Header>
  <Table.Body>
    {#each result.songs as song, index (song.id)}
      <Table.Row class="cursor-pointer" onclick={event=>{if(!(event.target instanceof Element)||!event.target.closest('button,a'))playerController.playFrom(result.songs,index)}}>
        <Table.Cell class="text-center"><span class="text-xs text-muted-foreground tabular-nums">{(result.page - 1) * result.pageSize + index + 1}</span></Table.Cell>
        <Table.Cell class="py-3">
          <div class="flex min-w-0 items-center gap-3"><SongCover src={song.coverUrl} /><div class="min-w-0">
            <div class="flex min-w-0 items-center gap-1"><Button variant="ghost" class="h-auto min-w-0 max-w-full justify-start p-0" title={song.title} aria-label={`播放 ${song.title}`} onclick={()=>playerController.playFrom(result.songs,index)}><span class="truncate">{song.title}</span></Button></div>
            {#if song.subtitle}<p class="mt-1 truncate text-xs text-muted-foreground" title={song.subtitle}>{song.subtitle}</p>{/if}
            <div class="mt-1 flex min-w-0 items-center gap-1 text-xs text-muted-foreground md:hidden"><MusicCredit {song} />{#if song.album}<span>·</span><MusicCredit {song} album />{/if}</div>
          </div></div>
        </Table.Cell>
        <Table.Cell class="hidden md:table-cell"><div class="min-w-0 text-muted-foreground"><MusicCredit {song} /></div></Table.Cell>
        <Table.Cell class="hidden md:table-cell"><div class="min-w-0 text-muted-foreground"><MusicCredit {song} album /></div></Table.Cell>
        <Table.Cell class="text-right"><span class="text-xs text-muted-foreground tabular-nums">{duration(song.duration)}</span></Table.Cell>
        <Table.Cell class="text-right"><div class="flex items-center justify-end gap-1"><Button variant="ghost" size="icon-sm" disabled={playerState.queue[playerState.index]?.id === song.id} aria-label={`下一首播放 ${song.title}`} title="下一首播放" onclick={() => playerController.playNext(song)}><ListEnd /></Button><FavoriteButton {song} /><DownloadButton songs={[song]} /><ResolveSongAction {song} /></div></Table.Cell>
      </Table.Row>
    {/each}
  </Table.Body>
</Table.Root>
<nav aria-label="搜索结果分页" class="mt-6 flex items-center justify-end gap-3">
  <Button variant="outline" disabled={result.page <= 1} onclick={() => onpage(result.page - 1)}><ChevronLeft data-icon="inline-start" />上一页</Button>
  <span class="text-xs text-muted-foreground tabular-nums" aria-live="polite">第 {result.page} 页</span>
  <Button variant="outline" disabled={!result.hasMore} onclick={() => onpage(result.page + 1)}>下一页<ChevronRight data-icon="inline-end" /></Button>
</nav>
