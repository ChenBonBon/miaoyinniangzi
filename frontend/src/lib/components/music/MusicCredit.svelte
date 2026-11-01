<script lang="ts">
 import { toast } from 'svelte-sonner'
 import { loadSongCredits, type SongCredits } from '$lib/song-credits'
 import { playerController } from '$lib/player-state.svelte'
 import { navigationGuard } from '$lib/navigation.svelte'
 import { Button } from '$lib/components/ui/button'
 import * as Popover from '$lib/components/ui/popover'
 import { openMusicDetail } from '$lib/detail-state.svelte'
 import type { Artist } from '$lib/music'
 let { song, album=false }: {song:{id?:string;artist?:string;artists?:Artist[];album?:string;albumMid?:string};album?:boolean}=$props()
 let open=$state(false),loading=$state(false)
 let resolved=$state<SongCredits|null>(null)
 $effect(()=>{void song.id;resolved=null})
 const albumMid=$derived(resolved?.albumMid ?? song.albumMid)
 const artists=$derived((resolved?.artists??song.artists??[]).filter(artist=>artist.name))
 const linked=$derived(artists.filter(artist=>/^[a-zA-Z0-9]{1,64}$/.test(artist.id)))
 async function resolveCredit(){
  if(!song.id||loading)return
  const id=song.id,current=navigationGuard();loading=true
  try{
   const credits=await loadSongCredits(id)
   playerController.updateCredits(id,credits.artists,credits.albumMid)
   if(!current()||song.id!==id)return
   resolved=credits
   if(album){if(credits.albumMid)openMusicDetail('album',credits.albumMid);else toast.error('该歌曲暂无可用的专辑标识')}
   else {const available=credits.artists.filter(artist=>artist.id);if(available.length===1&&credits.artists.length===1)openMusicDetail('artist',available[0].id);else if(available.length)open=true;else toast.error('该歌曲暂无可用的歌手标识')}
  }catch{if(current())toast.error('无法读取歌曲的歌手或专辑信息，请稍后重试')}
  finally{loading=false}
 }
 const label=$derived(album?song.album||'未知专辑':song.artist||artists.map(artist=>artist.name).join(' / ')||'未知歌手')
</script>
{#if album && albumMid && /^[a-zA-Z0-9]{1,64}$/.test(albumMid)}
 <Button variant="link" class="block h-auto min-w-0 max-w-full shrink truncate p-0 text-left font-normal text-inherit [font-size:inherit]" title={label} aria-label={`查看专辑 ${label}`} onclick={()=>openMusicDetail('album',albumMid!)}>{label}</Button>
{:else if !album && linked.length===1 && artists.length===1}
 <Button variant="link" class="block h-auto min-w-0 max-w-full shrink truncate p-0 text-left font-normal text-inherit [font-size:inherit]" title={label} aria-label={`查看歌手 ${label}`} onclick={()=>openMusicDetail('artist',linked[0].id)}>{label}</Button>
{:else if !album && linked.length>0}
 <Popover.Root bind:open><Popover.Trigger>{#snippet child({props})}<Button {...props} variant="link" class="block h-auto min-w-0 max-w-full shrink truncate p-0 text-left font-normal text-inherit [font-size:inherit]" title={label} aria-label={`选择歌手 ${label}`}>{label}</Button>{/snippet}</Popover.Trigger><Popover.Content aria-label="选择歌手" class="flex w-64 flex-col gap-1 p-2">{#each artists as artist,index (`${artist.id}:${index}`)}<Button variant="ghost" class="justify-start" disabled={!/^[a-zA-Z0-9]{1,64}$/.test(artist.id)} onclick={()=>{open=false;openMusicDetail('artist',artist.id)}}>{artist.name}</Button>{/each}</Popover.Content></Popover.Root>
{:else if song.id && /^[a-zA-Z0-9]{1,64}$/.test(song.id) && (album ? song.album : song.artist)}
 <Button variant="link" class="block h-auto min-w-0 max-w-full shrink truncate p-0 text-left font-normal text-inherit [font-size:inherit]" title={label} disabled={loading} aria-busy={loading} aria-label={`${album?'查看专辑':'查看歌手'} ${label}`} onclick={resolveCredit}>{label}</Button>
{:else}<span class="block truncate" title={label}>{label}</span>{/if}
