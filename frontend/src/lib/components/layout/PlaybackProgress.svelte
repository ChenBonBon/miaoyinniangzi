<script lang="ts">
 import { Slider } from '$lib/components/ui/slider'
 import { playerState,playerController } from '$lib/player-state.svelte'
 let scrubbing=$state(false)
 let draft=$state(0)
 const position=$derived(scrubbing?draft:playerState.seekTarget??playerState.position)
 const disabled=$derived(playerState.index<0||!playerState.seekable)
 let previousSong:string|undefined
 $effect(()=>{const id=playerState.queue[playerState.index]?.id;if(id!==previousSong){previousSong=id;scrubbing=false}})
 function begin(){if(disabled)return;scrubbing=true;draft=playerState.seekTarget??playerState.position}
 function commit(value:number){if(!scrubbing)return;scrubbing=false;playerController.seek(value)}
 function time(value:number){if(!Number.isFinite(value)||value<0)return '0:00';return `${Math.floor(value/60)}:${Math.floor(value%60).toString().padStart(2,'0')}`}
</script>

<svelte:window onpointercancel={()=>scrubbing=false} onblur={()=>scrubbing=false}/>
<div class="mt-2 flex items-center gap-3">
 <span class="min-w-7 text-[10px] text-muted-foreground tabular-nums">{time(position)}</span>
 <!-- Capture begins preview before Bits UI changes its controlled value. A
      bubbling pointerdown would overwrite the first click/drag with old time. -->
 <div class="min-w-0 flex-1" onpointerdowncapture={begin} onkeydowncapture={event=>{if(['ArrowLeft','ArrowRight','ArrowUp','ArrowDown','Home','End','PageUp','PageDown'].includes(event.key))begin()}}>
  <Slider data-playback type="single" value={position} max={playerState.duration||1} step={.1} {disabled} aria-label="播放进度" aria-valuetext={`${time(position)} / ${time(playerState.duration)}`} aria-busy={playerState.seekTarget!==null} onValueChange={value=>{if(scrubbing)draft=value}} onValueCommit={commit}/>
 </div>
 <span class="min-w-7 text-right text-[10px] text-muted-foreground tabular-nums">{time(playerState.duration)}</span>
</div>
