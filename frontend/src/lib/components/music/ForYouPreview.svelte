<script lang="ts">
 import { Heart,ListEnd,Play,RefreshCw } from '@lucide/svelte'
 import { toast } from 'svelte-sonner'
 import { Button } from '$lib/components/ui/button'
 import * as Table from '$lib/components/ui/table'
 import SongCover from '$lib/components/search/SongCover.svelte'
 import { samplePreviewSongs } from '$lib/for-you-preview'
 import { cn } from '$lib/utils'
 let songs=$state(samplePreviewSongs())
 let favorites=$state(new Set<number>())
 let batch=$state(1)
 function replace(){songs=samplePreviewSongs();batch++}
 function favorite(id:number){const next=new Set(favorites);if(next.has(id))next.delete(id);else next.add(id);favorites=next}
</script>

<section aria-labelledby="for-you-heading" class="mb-10">
 <div class="mb-4 flex items-center justify-between gap-4">
  <h2 id="for-you-heading" class="text-lg font-semibold">猜你喜欢</h2>
  <Button variant="outline" size="sm" onclick={replace}><RefreshCw data-icon="inline-start" />换一批</Button>
 </div>
 <p id="for-you-preview-description" class="sr-only">静态预览，播放和下一首暂不接入播放器，收藏仅在本次预览中切换。</p>
 <span class="sr-only" role="status">第 {batch} 批，共 10 首歌曲</span>
 <Table.Root class="table-fixed" aria-label="猜你喜欢曲目">
  <Table.Header><Table.Row><Table.Head>歌曲</Table.Head><Table.Head class="hidden w-1/3 sm:table-cell">歌手</Table.Head><Table.Head class="w-28 text-right">操作</Table.Head></Table.Row></Table.Header>
  <Table.Body>
   {#each songs as song (song.id)}
    <Table.Row>
     <Table.Cell class="py-3"><div class="flex min-w-0 items-center gap-3"><SongCover src="" /><div class="min-w-0"><span class="block truncate font-medium">{song.title}</span><span class="mt-1 block truncate text-xs text-muted-foreground sm:hidden">{song.artist}</span></div></div></Table.Cell>
     <Table.Cell class="hidden text-muted-foreground sm:table-cell"><span class="block truncate">{song.artist}</span></Table.Cell>
     <Table.Cell><div class="flex justify-end gap-1">
      <Button variant="ghost" size="icon-sm" title="播放" aria-label={`播放 ${song.title}`} aria-describedby="for-you-preview-description" onclick={()=>toast.info('静态预览，暂不播放音乐',{id:'for-you-preview'})}><Play /></Button>
      <Button variant="ghost" size="icon-sm" title="下一首播放" aria-label={`下一首播放 ${song.title}`} aria-describedby="for-you-preview-description" onclick={()=>toast.info('静态预览，暂不加入播放队列',{id:'for-you-preview'})}><ListEnd /></Button>
      <Button variant="ghost" size="icon-sm" title={favorites.has(song.id)?'取消收藏':'收藏'} aria-label={`${favorites.has(song.id)?'取消收藏':'收藏'} ${song.title}`} aria-pressed={favorites.has(song.id)} aria-describedby="for-you-preview-description" onclick={()=>favorite(song.id)}><Heart class={cn(favorites.has(song.id)&&'text-destructive')} fill={favorites.has(song.id)?'currentColor':'none'} /></Button>
     </div></Table.Cell>
    </Table.Row>
   {/each}
  </Table.Body>
 </Table.Root>
</section>
