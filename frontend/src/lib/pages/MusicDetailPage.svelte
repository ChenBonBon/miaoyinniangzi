<script lang="ts">
 import { onMount, tick } from 'svelte'
 import { Play, ChevronLeft, ChevronRight, Disc3 } from '@lucide/svelte'
 import { Button } from '$lib/components/ui/button'
 import { Skeleton } from '$lib/components/ui/skeleton'
 import * as Alert from '$lib/components/ui/alert'
 import * as Empty from '$lib/components/ui/empty'
 import CollectionCover from '$lib/components/music/CollectionCover.svelte'
 import SongTable from '$lib/components/music/SongTable.svelte'
 import MusicCredit from '$lib/components/music/MusicCredit.svelte'
 import DownloadButton from '$lib/components/DownloadButton.svelte'
 import { detailSelection as selection, detailState as view, detailController as controller, openMusicDetail } from '$lib/detail-state.svelte'
 import { mountNavigationPage, recordBrowse } from '$lib/navigation.svelte'
 import { playerController } from '$lib/player-state.svelte'
 const data=$derived(view.value),songs=$derived(data?.songs??[]),pages=$derived(Math.max(1,Math.ceil(songs.length/50))),page=$derived(Math.min(selection.songPage,pages)),offset=$derived((page-1)*50)
 let root:HTMLDivElement
 let albumsRoot=$state<HTMLElement>()
 $effect(()=>{if(view.status==='ready'&&selection.songPage>pages)selection.songPage=pages})
 onMount(()=>{
  const binding=mountNavigationPage('音乐详情',{
   capture:()=>({...selection}),
   restore:async value=>{if(value)Object.assign(selection,value);await controller.open(selection.kind,selection.id,selection.albumPage)},
   refresh:async current=>{const key=`${selection.kind}:${selection.id}:${selection.albumPage}`,pane=root.closest<HTMLElement>('[data-content-scroll]'),scroll=pane?.scrollTop??0;await controller.open(selection.kind,selection.id,selection.albumPage,true);await tick();if(current()&&root.isConnected&&key===`${selection.kind}:${selection.id}:${selection.albumPage}`&&pane)pane.scrollTop=scroll},
  })
  return()=>{binding.dispose();controller.suspend()}
 })
 async function turnSongs(value:number){recordBrowse(()=>selection.songPage=value);await tick();root.closest<HTMLElement>('[data-content-scroll]')?.scrollTo({top:0,behavior:'instant'})}
 async function turnAlbums(value:number){const id=selection.id;let pending=Promise.resolve();recordBrowse(()=>{selection.albumsExpanded=true;selection.albumPage=value;pending=controller.open('artist',id,value)});await pending;await tick();if(view.status==='ready'&&selection.kind==='artist'&&selection.id===id&&selection.albumPage===value)albumsRoot?.scrollIntoView({block:'start',behavior:'instant'})}
</script>
<div bind:this={root} data-music-detail={selection.kind}>
 {#if view.status==='loading'||view.status==='idle'}
  <div role="status" aria-label="正在读取音乐详情" class="flex flex-col gap-6"><div class="flex items-center gap-5"><Skeleton class="size-24" /><Skeleton class="h-8 w-1/2" /></div>{#each [0,1,2,3,4] as row (row)}<Skeleton class="h-14 w-full" />{/each}</div>
 {:else if view.status==='error'}
  <Alert.Root variant="destructive"><Alert.Title>暂时无法读取详情</Alert.Title><Alert.Description>{view.error}</Alert.Description><Button variant="outline" class="mt-3" onclick={()=>controller.open(selection.kind,selection.id,selection.albumPage,true)}>重试详情</Button></Alert.Root>
 {:else if data}
  <div class="mb-6 flex min-w-0 items-center gap-5"><div class={data.kind==='artist'?'shrink-0 overflow-hidden rounded-full':'shrink-0'}><CollectionCover src={data.coverUrl} large /></div><div class="flex min-w-0 flex-col gap-2"><h1 class="break-words text-2xl leading-tight font-semibold md:text-3xl">{data.title}</h1>{#if data.artists?.length}<div class="min-w-0 text-sm text-muted-foreground"><MusicCredit song={{artists:data.artists,artist:data.artists.map(artist=>artist.name).join(' / ')}} /></div>{/if}<p class="text-sm text-muted-foreground">{data.date ? `${data.date} · ` : ''}{songs.length} 首{data.kind==='artist'?'热门歌曲':''}</p></div></div>
  {#if data.description}<details class="mb-6 max-w-prose"><summary class="w-fit cursor-pointer text-sm text-muted-foreground focus-visible:outline-2 focus-visible:outline-ring">歌手简介</summary><p class="mt-3 text-sm leading-7 text-muted-foreground">{data.description}</p></details>{/if}
  <div class="mb-8 flex flex-wrap items-center gap-3"><Button disabled={!songs.length} onclick={()=>playerController.playFrom(songs,0)}><Play data-icon="inline-start" />{data.kind==='artist'?'播放热门':'播放全部'}</Button><DownloadButton songs={songs} label={data.kind==='artist'?'下载热门':'下载全部'} /></div>
  <section aria-label={data.kind==='artist'?'热门歌曲':'专辑曲目'}>
  {#if data.kind==='artist'}<h2 class="mb-4 text-lg font-semibold">热门歌曲</h2>{/if}
  {#if songs.length}<SongTable artistView={data.kind==='artist'} songs={songs.slice(offset,offset+(data.kind==='artist'&&!selection.hotExpanded?20:50))} {offset} onplay={index=>playerController.playFrom(songs,offset+index)} />
  {#if data.kind==='artist'&&!selection.hotExpanded&&songs.length>20}<div class="mt-4 flex justify-center"><Button variant="ghost" onclick={()=>recordBrowse(()=>selection.hotExpanded=true)}>查看更多</Button></div>{/if}
  {#if pages>1}<nav aria-label="详情曲目分页" class="mt-6 flex items-center justify-end gap-3"><Button variant="outline" disabled={page<=1} onclick={()=>turnSongs(page-1)}><ChevronLeft data-icon="inline-start" />上一页</Button><span class="text-xs text-muted-foreground">第 {page} / {pages} 页</span><Button variant="outline" disabled={page>=pages} onclick={()=>turnSongs(page+1)}>下一页<ChevronRight data-icon="inline-end" /></Button></nav>{/if}
  {:else}<Empty.Root class="py-10"><Empty.Header><Empty.Media variant="icon"><Disc3 /></Empty.Media><Empty.Title>暂无曲目</Empty.Title></Empty.Header></Empty.Root>{/if}
  </section>
  {#if data.kind==='artist'}
   <section bind:this={albumsRoot} class="mt-10 scroll-mt-6" aria-label="歌手专辑"><h2 class="mb-4 text-lg font-semibold">专辑 <span class="ml-1 text-sm font-normal text-muted-foreground">{data.totalAlbums}</span></h2>
   {#if data.albums.length}<div class="grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-2 xl:grid-cols-3">{#each (selection.albumsExpanded ? data.albums : data.albums.slice(0,10)) as album (album.id)}<Button variant="ghost" class="h-auto min-w-0 justify-start gap-4 px-2 py-3 text-left" aria-label={`查看专辑 ${album.title}`} onclick={()=>openMusicDetail('album',album.id)}><CollectionCover src={album.coverUrl} /><span class="flex min-w-0 flex-col gap-2"><span class="truncate text-base font-semibold" title={album.title}>{album.title}</span>{#if album.date}<span class="text-xs font-normal text-muted-foreground">{album.date}</span>{/if}</span></Button>{/each}</div>
   {:else}<p class="py-6 text-sm text-muted-foreground">暂无专辑</p>{/if}
   {#if !selection.albumsExpanded&&(data.albums.length>10||data.hasMoreAlbums)}<div class="mt-4 flex justify-center"><Button variant="ghost" onclick={()=>{if(data.albums.length>10)recordBrowse(()=>selection.albumsExpanded=true);else void turnAlbums(data.albumPage+1)}}>查看更多</Button></div>{/if}
   {#if selection.albumsExpanded&&(data.albumPage>1||data.hasMoreAlbums)}<nav aria-label="歌手专辑分页" class="mt-6 flex items-center justify-end gap-3"><Button variant="outline" disabled={data.albumPage<=1} onclick={()=>turnAlbums(data.albumPage-1)}>上一页</Button><span class="text-xs text-muted-foreground">第 {data.albumPage} 页</span><Button variant="outline" disabled={!data.hasMoreAlbums} onclick={()=>turnAlbums(data.albumPage+1)}>下一页</Button></nav>{/if}
   </section>
  {/if}
 {/if}
</div>
