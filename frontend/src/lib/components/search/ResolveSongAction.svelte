<script lang="ts">
 import { LoaderCircle } from '@lucide/svelte'
 import { Button } from '$lib/components/ui/button'
 import { resolutionController,resolutionState } from '$lib/resolution-state.svelte'
 import type { Song } from '$lib/music'
 let {song}:{song:Song}=$props()
 const busy=$derived(resolutionState.songId===song.id&&resolutionState.status==='loading')
</script>
 <Button variant="ghost" size="sm" aria-label={`${busy?'取消解析':'解析'} ${song.title}`} onclick={()=>busy?resolutionController.cancel():void resolutionController.resolve(song)}>
  {#if busy}<LoaderCircle data-icon="inline-start" class="animate-spin" />取消{:else}解析{/if}
 </Button>
