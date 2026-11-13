<script lang="ts">
 import {onDestroy,untrack} from 'svelte'
 import {Music2,Play,RefreshCw} from '@lucide/svelte'
 import {toast} from 'svelte-sonner'
 import {Button} from '$lib/components/ui/button'
 import {Skeleton} from '$lib/components/ui/skeleton'
 import * as Empty from '$lib/components/ui/empty'
 import {forYouState as view,forYouController as controller} from '$lib/for-you-state.svelte'
 import {favoritesState,favoritesController} from '$lib/favorites-state.svelte'
 import {playerController,playerState} from '$lib/player-state.svelte'
 import {openMusicDetail} from '$lib/detail-state.svelte'
 let failed=$state(new Set<string>())
 $effect(()=>{void view.songs;failed=new Set()})
 let favoriteErrorSeen=false
 $effect(()=>{
  const status=favoritesState.status,songs=favoritesState.songs
  untrack(()=>{
   if(status==='ready'){favoriteErrorSeen=false;controller.setFavorites(songs);void controller.activate()}
   else if(status==='error'){controller.suspend();if(!favoriteErrorSeen){favoriteErrorSeen=true;toast.error('无法读取收藏，暂时不能生成推荐',{id:'for-you'})}}
  })
 })
 $effect(()=>controller.setQueue(playerState.queue))
 onDestroy(()=>controller.suspend())
 function next(){if(favoritesState.status==='error')void favoritesController.load(true);else controller.next()}
</script>

<section aria-labelledby="for-you-heading" class="@container/for-you mb-10" aria-busy={view.status==='loading'||view.status==='idle'&&favoritesState.status!=='error'||view.refreshing}>
 <div class="mb-4 flex items-center justify-between gap-4"><h2 id="for-you-heading" class="text-lg font-semibold">猜你喜欢</h2><Button variant="outline" size="sm" disabled={view.status==='loading'||view.refreshing||(favoritesState.status==='loading'||favoritesState.status==='idle')} onclick={next}><RefreshCw data-icon="inline-start" />换一批</Button></div>
 {#if (view.status==='idle'||view.status==='loading')&&favoritesState.status!=='error'}
  <div class="recommendation-grid grid grid-cols-3 gap-3 md:gap-4" role="status" aria-label="正在根据收藏寻找相似歌曲">{#each Array.from({length:12},(_,index)=>index) as index (index)}<Skeleton class="aspect-square w-full rounded-lg" />{/each}</div>
 {:else if view.songs.length}
  {#if view.refreshing}<span class="sr-only" role="status">正在更新推荐，当前内容仍可使用</span>{/if}
  <span class="sr-only" role="status">第 {view.batch} 批，共 {view.songs.length} 首猜你喜欢</span>
  <ul class="recommendation-grid grid grid-cols-3 gap-3 md:gap-4" aria-label="猜你喜欢曲目">
   {#each view.songs as song,index (song.id)}
    <li class="relative min-w-0" data-for-you-song={song.id}>
     <Button variant="ghost" class="relative aspect-square h-auto w-full overflow-hidden rounded-lg bg-muted p-0 text-left whitespace-normal" aria-label={`查看专辑 ${song.album||song.title}`} onclick={()=>openMusicDetail('album',song.albumMid)}>
      {#if song.coverUrl&&!failed.has(song.id)}<img src={song.coverUrl} alt="" width="240" height="240" class="absolute inset-0 size-full object-cover" loading="lazy" decoding="async" referrerpolicy="no-referrer" onerror={()=>failed=new Set([...failed,song.id])} />{:else}<Music2 class="size-16 text-muted-foreground" />{/if}
      <span class="absolute inset-0 bg-gradient-to-t from-black/90 via-black/60 to-black/10" aria-hidden="true"></span>
      <span class="absolute right-14 bottom-3 left-3 flex min-w-0 flex-col gap-1 text-white md:right-16 md:bottom-4 md:left-4"><span data-for-you-title class="truncate text-base font-semibold md:text-lg" title={song.title}>{song.title}</span><span class="truncate text-xs font-normal text-white/90 md:text-sm" title={song.artist||'未知歌手'}>{song.artist||'未知歌手'}</span></span>
     </Button>
     <Button size="icon-sm" class="absolute right-3 bottom-3 rounded-full md:right-4 md:bottom-4" title="播放" aria-label={`播放 ${song.title}`} onclick={()=>playerController.playFrom(view.songs,index)}><Play /></Button>
    </li>
   {/each}
  </ul>
 {:else if view.status==='error'||favoritesState.status==='error'}<span class="sr-only">猜你喜欢读取失败，可点击换一批重试。</span>
 {:else}
  <Empty.Root class="py-10"><Empty.Header><Empty.Media variant="icon"><Music2 /></Empty.Media><Empty.Title>{view.reason==='no_favorites'?'从喜欢的歌开始':view.reason==='missing_song_ids'?'收藏歌曲缺少推荐信息':'暂时没有可用的相似歌曲'}</Empty.Title><Empty.Description>{view.reason==='no_favorites'?'收藏几首歌曲后，会在这里为你寻找相似音乐。':view.reason==='missing_song_ids'?'请重新收藏几首歌曲，再来试试。':'可以再收藏几首不同歌手的歌曲。'}</Empty.Description></Empty.Header></Empty.Root>
 {/if}
</section>
<style>
 @container for-you (min-width:52rem){.recommendation-grid{grid-template-columns:repeat(4,minmax(0,1fr))}}
</style>
