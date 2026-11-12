<script lang="ts">
 import {onMount} from 'svelte'
 import {ArrowLeft,Play} from '@lucide/svelte'
 import {toast} from 'svelte-sonner'
 import {Button} from '$lib/components/ui/button'
 import * as Table from '$lib/components/ui/table'
 import {previewSongs} from '$lib/for-you-preview'
 import {forYouPreview as view} from '$lib/for-you-preview-state.svelte'
 import {mountNavigationPage,goBack,navigate,browsing} from '$lib/navigation.svelte'
 const song=$derived(previewSongs.find(song=>song.id===view.albumID)??previewSongs[0])
 function play(){toast.info('静态预览，暂不播放音乐',{id:'for-you-preview'})}
 onMount(()=>{
  const binding=mountNavigationPage('专辑预览',{
   capture:()=>view.albumID,
   restore:id=>{if(Number.isInteger(id)&&previewSongs.some(song=>song.id===id))view.albumID=id},
   refresh:()=>{},
  })
  return ()=>binding.dispose()
 })
</script>

<div data-preview-album={song.id}>
 <div class="mb-6"><Button variant="ghost" size="sm" onclick={()=>browsing.back?goBack():navigate('推荐')}><ArrowLeft data-icon="inline-start" />返回推荐</Button></div>
 <div class="mb-8 flex min-w-0 items-center gap-5">
  <img src={song.artwork} alt="" width="240" height="240" class="size-28 shrink-0 rounded-lg object-cover md:size-40" />
  <div class="flex min-w-0 flex-col gap-2"><h1 class="break-words text-2xl font-semibold md:text-3xl">{song.albumTitle}</h1><p class="text-sm text-muted-foreground">{song.artist}</p><p class="text-sm text-muted-foreground">静态专辑预览 · 1 首</p></div>
 </div>
 <Table.Root aria-label="专辑预览曲目"><Table.Header><Table.Row><Table.Head>歌曲</Table.Head><Table.Head>歌手</Table.Head><Table.Head class="w-12"><span class="sr-only">播放</span></Table.Head></Table.Row></Table.Header><Table.Body><Table.Row><Table.Cell>{song.title}</Table.Cell><Table.Cell class="text-muted-foreground">{song.artist}</Table.Cell><Table.Cell><Button variant="ghost" size="icon-sm" aria-label={`播放 ${song.title}`} onclick={play}><Play /></Button></Table.Cell></Table.Row></Table.Body></Table.Root>
</div>
