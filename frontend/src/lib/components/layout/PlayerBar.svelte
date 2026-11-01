<script lang="ts">
  import { Music2, Play, Pause, SkipBack, SkipForward, Volume2, VolumeX, LoaderCircle, ListMusic, Shuffle, Repeat1, Repeat } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import { Slider } from '$lib/components/ui/slider'
  import FavoriteButton from '$lib/components/favorites/FavoriteButton.svelte'
  import SongCover from '$lib/components/search/SongCover.svelte'
  import PlaybackProgress from './PlaybackProgress.svelte'
  import { playerState,playerController,setPlayerVolume,togglePlayerMute } from '$lib/player-state.svelte'
  import MusicCredit from '$lib/components/music/MusicCredit.svelte'
  import { openNowPlaying, navigation, closeQueue } from '$lib/navigation.svelte'
  const song=$derived(playerState.queue[playerState.index])
  const busy=$derived(playerState.status==='resolving'||playerState.status==='buffering')
  const playing=$derived(playerState.status==='playing')
  const modeName=$derived(playerState.mode==='shuffle'?'随机播放':playerState.mode==='repeat-one'?'单曲循环':'顺序播放')
</script>

<footer aria-label="播放控制" class="col-span-full grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-4 border-t bg-sidebar px-4 py-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)_minmax(0,1fr)] md:px-7">
  <div class="flex min-w-0 flex-1 items-center gap-3">
    {#if song}<Button variant="ghost" size="icon" class="size-12 shrink-0 p-0" aria-label="打开播放详情" onclick={openNowPlaying}><SongCover src={song.coverUrl}/></Button>{:else}<div class="flex size-12 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground"><Music2 class="size-6" /></div>{/if}
    <div class="min-w-0 flex-1">{#if song}<div class="flex min-w-0 items-center gap-2"><Button variant="link" class="block h-auto min-w-0 max-w-full shrink truncate p-0 text-left text-foreground" aria-label={`查看 ${song.title} 的播放详情`} onclick={openNowPlaying}>{song.title}</Button><FavoriteButton {song} /></div>{:else}<strong class="block truncate text-sm font-medium">尚未播放</strong>{/if}<div class="mt-2 hidden min-w-0 text-xs text-muted-foreground md:block">{#if playerState.error || playerState.status==='resolving'}<span class="block truncate">{playerState.error || '正在准备播放'}</span>{:else if song}<MusicCredit {song} />{:else}选一首喜欢的音乐{/if}</div></div>
  </div>
  <div class="w-full max-w-lg justify-self-center">
    <div class="flex items-center justify-center gap-3 md:gap-5">
      <Button variant="ghost" size="icon" aria-label={`播放模式：${modeName}`} disabled={!song} onclick={()=>playerController.setMode(playerState.mode==='sequential'?'shuffle':playerState.mode==='shuffle'?'repeat-one':'sequential')}>
        {#if playerState.mode==='shuffle'}<Shuffle/>{:else if playerState.mode==='repeat-one'}<Repeat1/>{:else}<Repeat/>{/if}
      </Button>
      <Button variant="ghost" size="icon" disabled={!song || !playerState.canPrevious} aria-label="上一首" onclick={()=>playerController.previous()}><SkipBack /></Button>
      <Button size="icon" class="rounded-full" disabled={!song} aria-label={busy?'取消播放':playing?'暂停':'播放'} onclick={()=>playerController.toggle()}>
        {#if busy}<LoaderCircle class="animate-spin"/>{:else if playing}<Pause/>{:else}<Play/>{/if}
      </Button>
      <Button variant="ghost" size="icon" disabled={!song} aria-label="下一首" onclick={()=>playerController.next()}><SkipForward /></Button>
    </div>
    <PlaybackProgress/>
  </div>
  <div class="flex items-center justify-self-end gap-2">
    <div class="hidden w-32 items-center gap-2 md:flex">
    <Button variant="ghost" size="icon-sm" aria-label={playerState.volume===0?'恢复音量':'静音'} aria-pressed={playerState.volume===0} onclick={togglePlayerMute}>{#if playerState.volume===0}<VolumeX/>{:else}<Volume2/>{/if}</Button>
    <Slider type="single" value={playerState.volume} min={0} max={1} step={.01} onValueChange={setPlayerVolume} aria-label="音量" aria-valuetext={playerState.volume===0?'静音':`${Math.round(playerState.volume*100)}%`}/>
    </div>
    <Button id="player-queue-toggle" variant="ghost" size="icon" aria-label="播放队列" aria-expanded={navigation.queueOpen} aria-controls="queue-panel" onclick={() => navigation.queueOpen ? closeQueue() : navigation.queueOpen = true}><ListMusic /></Button>
  </div>
</footer>
