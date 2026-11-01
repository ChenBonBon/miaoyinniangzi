<script lang="ts">
 import { Download, LoaderCircle } from '@lucide/svelte'
 import { Button } from '$lib/components/ui/button'
 import { downloadsController } from '$lib/downloads-state.svelte'
 import type { Song } from '$lib/music'
 let {songs,label}: {songs:Song[];label?:string}=$props()
 let pending=$state(false)
</script>
<Button variant={label?'outline':'ghost'} size={label?'default':'icon-sm'} aria-label={label || `下载 ${songs[0]?.title || '歌曲'}`} title={label || '下载'} disabled={pending || !songs.length} aria-busy={pending} onclick={async event=>{event.stopPropagation();pending=true;try{await downloadsController.enqueue(songs)}finally{pending=false}}}>
 {#if pending}<LoaderCircle class="animate-spin" />{:else}<Download />{/if}{#if label}{label}{/if}
</Button>
