<script lang="ts">
  import { cn } from '$lib/utils'
  import { ListEnd } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import * as Table from '$lib/components/ui/table'
  import MusicCredit from './MusicCredit.svelte'
  import SongCover from '$lib/components/search/SongCover.svelte'
  import FavoriteButton from '$lib/components/favorites/FavoriteButton.svelte'
  import DownloadButton from '$lib/components/DownloadButton.svelte'
  import { playerController, playerState } from '$lib/player-state.svelte'
  import type { Song } from '$lib/music'
  let { songs, offset = 0, onplay, label = '推荐曲目', artistView = false }: { artistView?: boolean; songs: Song[]; offset?: number; onplay: (index: number) => void; label?: string } = $props()
  function duration(value: number) {
    const seconds = Math.floor(value)
    return Number.isFinite(seconds) && seconds > 0 ? `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}` : '未知'
  }
</script>

<Table.Root class="table-fixed" aria-label={label}>
  <Table.Header><Table.Row><Table.Head class="w-10 text-center">#</Table.Head><Table.Head>歌曲</Table.Head>{#if !artistView}<Table.Head class="hidden w-1/5 lg:table-cell">歌手</Table.Head>{/if}<Table.Head class={cn('hidden w-1/5',artistView ? 'md:table-cell' : 'xl:table-cell')}>专辑</Table.Head><Table.Head class="hidden w-16 text-right sm:table-cell">时长</Table.Head><Table.Head class="w-28 text-right"><span class="sr-only">歌曲操作</span></Table.Head></Table.Row></Table.Header>
  <Table.Body>
    {#each songs as song, index (`${song.id}:${offset+index}`)}
      <Table.Row class="cursor-pointer" onclick={event => { if (!(event.target instanceof Element) || !event.target.closest('button,a')) onplay(index) }}>
        <Table.Cell class="text-center"><span class="text-xs text-muted-foreground tabular-nums">{offset + index + 1}</span></Table.Cell>
        <Table.Cell class="py-3"><div class="flex min-w-0 items-center gap-3"><SongCover src={song.coverUrl} /><div class="min-w-0"><Button variant="ghost" class="h-auto max-w-full justify-start p-0" title={song.title} aria-label={`播放 ${song.title}`} onclick={() => onplay(index)}><span class="truncate">{song.title}</span></Button>{#if song.subtitle}<p class="mt-1 truncate text-xs text-muted-foreground" title={song.subtitle}>{song.subtitle}</p>{/if}<div class={cn('mt-1 min-w-0 text-xs text-muted-foreground',artistView ? 'md:hidden' : 'lg:hidden')}><MusicCredit {song} album={artistView} /></div></div></div></Table.Cell>
        {#if !artistView}<Table.Cell class="hidden lg:table-cell"><div class="min-w-0 text-muted-foreground"><MusicCredit {song} /></div></Table.Cell>{/if}
        <Table.Cell class={cn('hidden',artistView ? 'md:table-cell' : 'xl:table-cell')}><div class="min-w-0 text-muted-foreground"><MusicCredit {song} album /></div></Table.Cell>
        <Table.Cell class="hidden text-right sm:table-cell"><span class="text-xs text-muted-foreground tabular-nums">{duration(song.duration)}</span></Table.Cell>
        <Table.Cell><div class="flex items-center justify-end gap-1"><Button variant="ghost" size="icon-sm" disabled={playerState.queue[playerState.index]?.id === song.id} aria-label={`下一首播放 ${song.title}`} title="下一首播放" onclick={() => playerController.playNext(song)}><ListEnd /></Button><FavoriteButton {song} /><DownloadButton songs={[song]} /></div></Table.Cell>
      </Table.Row>
    {/each}
  </Table.Body>
</Table.Root>
