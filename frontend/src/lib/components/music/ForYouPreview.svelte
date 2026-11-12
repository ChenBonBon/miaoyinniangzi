<script lang="ts">
 import { Play,RefreshCw } from '@lucide/svelte'
 import { toast } from 'svelte-sonner'
 import { Button } from '$lib/components/ui/button'
 import { previewBatchSize } from '$lib/for-you-preview'
 import { forYouPreview as view,replacePreviewBatch,openPreviewAlbum } from '$lib/for-you-preview-state.svelte'
</script>

<section aria-labelledby="for-you-heading" class="mb-10">
 <div class="mb-4 flex items-center justify-between gap-4">
  <h2 id="for-you-heading" class="text-lg font-semibold">猜你喜欢</h2>
  <Button variant="outline" size="sm" onclick={replacePreviewBatch}><RefreshCw data-icon="inline-start" />换一批</Button>
 </div>
 <p id="for-you-preview-description" class="sr-only">静态预览，点击封面查看专辑，播放按钮暂不播放真实音乐。</p>
 <span class="sr-only" role="status">第 {view.batch} 批，共 {previewBatchSize} 首歌曲</span>
 <ul class="grid grid-cols-3 gap-3 md:gap-4" aria-label="猜你喜欢曲目">
  {#each view.songs as song (song.id)}
   <li class="relative min-w-0" data-for-you-song={song.id}>
    <Button variant="ghost" class="relative aspect-square h-auto w-full overflow-hidden rounded-lg p-0 text-left whitespace-normal" aria-label={`查看${song.albumTitle}`} aria-describedby="for-you-preview-description" onclick={()=>openPreviewAlbum(song.id)}>
     <img src={song.artwork} alt="" width="240" height="240" class="absolute inset-0 size-full object-cover" decoding="async" />
     <span class="absolute inset-0 bg-gradient-to-t from-black/90 via-black/60 to-black/10" aria-hidden="true"></span>
     <span class="absolute right-14 bottom-3 left-3 flex min-w-0 flex-col gap-1 text-white md:right-16 md:bottom-4 md:left-4">
      <span data-preview-title class="truncate text-base font-semibold md:text-lg">{song.title}</span>
      <span class="truncate text-xs font-normal text-white/90 md:text-sm">{song.artist}</span>
     </span>
    </Button>
    <Button size="icon-sm" class="absolute right-3 bottom-3 rounded-full md:right-4 md:bottom-4" title="播放" aria-label={`播放 ${song.title}`} aria-describedby="for-you-preview-description" onclick={()=>toast.info('静态预览，暂不播放音乐',{id:'for-you-preview'})}><Play /></Button>
   </li>
  {/each}
 </ul>
</section>
