<script lang="ts">
  import { onDestroy, onMount, tick, untrack } from 'svelte'
  import { ArrowLeft, Music2, RotateCcw } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import { Skeleton } from '$lib/components/ui/skeleton'
  import * as Empty from '$lib/components/ui/empty'
  import MusicCredit from '$lib/components/music/MusicCredit.svelte'
  import FavoriteButton from '$lib/components/favorites/FavoriteButton.svelte'
  import { closeNowPlaying, mountNavigationPage } from '$lib/navigation.svelte'
  import { playerState, playerController } from '$lib/player-state.svelte'
  import { LyricsController, currentLyric, type LyricsState, type LyricsAPI } from '$lib/lyrics-controller'
  const song = $derived(playerState.queue[playerState.index])
  let view = $state<LyricsState>({ songId: '', lines: [], text: '', status: 'idle', error: '' })
  let follow = $state(true), reduced = $state(true), failedCover = $state(false)
  let pane: HTMLDivElement | undefined = $state()
  let disposed = false
  function bridge() {
    const api = (window as Window & { go?: { main?: { App?: { GetLyrics: LyricsAPI['get']; CancelLyrics: LyricsAPI['cancel'] } } } }).go?.main?.App
    if (!api?.GetLyrics) throw Error('请在桌面应用中读取歌词')
    return api
  }
  const controller = new LyricsController({ get: (mid,id) => bridge().GetLyrics(mid,id), cancel: async (id) => bridge().CancelLyrics(id) }, state => Object.assign(view,state))
  const active = $derived(currentLyric(view.lines,playerState.position))
  const songId = $derived(song?.id ?? '')
  let displayedSong: string | undefined
  $effect(() => {
    const id = songId
    if (id === displayedSong) return
    displayedSong = id
    untrack(() => {
      follow = true
      failedCover = false
      if (pane) pane.scrollTop = 0
      void controller.select(id)
    })
  })
  $effect(() => {
    const index = active, tracking = follow, container = pane
    if (!tracking || index < 0 || !container) return
    void tick().then(() => {
      if (disposed || active !== index || !follow) return
      const line = container.querySelector<HTMLElement>(`[data-lyric-index="${index}"]`)
      if (line) {
        const target = Math.max(0, line.getBoundingClientRect().top - container.getBoundingClientRect().top + container.scrollTop - container.clientHeight / 2 + line.offsetHeight / 2)
        // WKWebView versions differ on the nonstandard 'instant' enum. A direct
        // scrollTop assignment is immediate and never starts a competing animation.
        if (reduced) container.scrollTop = target
        else container.scrollTo({ top: target, behavior: 'smooth' })
      }
    })
  })
  onMount(() => {
    const media = matchMedia('(prefers-reduced-motion: reduce)')
    const update = () => reduced = media.matches
    update();media.addEventListener('change',update)
    const binding = mountNavigationPage('正在播放', {
      capture: () => ({ songId: song?.id, follow, scroll: pane?.scrollTop ?? 0 }),
      restore: async (data, current) => { if (data && data.songId === song?.id) { follow = data.follow; await tick(); if (current() && pane && !follow) pane.scrollTop = data.scroll } },
      refresh: () => controller.select(song?.id ?? '', true),
    })
    return () => { binding.dispose(); media.removeEventListener('change',update) }
  })
  onDestroy(() => { disposed = true; controller.destroy() })
  function stopFollowing() {
    follow = false
    if (pane) pane.scrollTo({ top: pane.scrollTop, behavior: 'auto' })
  }
  function jump(time: number) { if (!playerState.seekable) return; follow = true; playerController.seek(time) }
</script>

<div class="flex min-h-0 flex-1 flex-col gap-4 md:gap-6" data-now-playing>
  <div><Button variant="ghost" size="sm" onclick={closeNowPlaying}><ArrowLeft data-icon="inline-start"/>返回</Button></div>
  {#if !song}
    <Empty.Root><Empty.Header><Empty.Media variant="icon"><Music2/></Empty.Media><Empty.Title>尚未选择歌曲</Empty.Title><Empty.Description>选择歌曲后，可在这里查看歌词。</Empty.Description></Empty.Header></Empty.Root>
  {:else}
    <div class="grid min-h-0 min-w-0 flex-1 grid-rows-[auto_minmax(0,1fr)] items-start gap-6 md:grid-cols-[minmax(0,.85fr)_minmax(0,1.2fr)] md:grid-rows-1 md:gap-12 lg:gap-20">
      <div class="flex min-w-0 items-center gap-5 md:sticky md:top-8 md:flex-col md:items-start md:gap-7">
        <div class="flex size-24 shrink-0 items-center justify-center overflow-hidden rounded-lg bg-muted text-muted-foreground md:aspect-square md:size-auto md:w-full md:max-w-[min(20rem,calc(100dvh-27rem))]">
          {#if song.coverUrl && !failedCover}<img src={song.coverUrl} alt="" referrerpolicy="no-referrer" decoding="async" class="size-full object-cover" onerror={() => failedCover = true}/>{:else}<Music2 class="size-10 md:size-20" aria-hidden="true"/>{/if}
        </div>
        <div class="flex min-w-0 flex-col gap-3">
          <div class="flex min-w-0 items-center gap-2"><h1 title={song.title} class="min-w-0 line-clamp-2 break-words text-2xl leading-tight font-semibold md:text-3xl">{song.title}</h1><FavoriteButton {song} /></div>
          <div class="min-w-0 text-base text-muted-foreground"><MusicCredit {song} /></div>
          {#if song.album}<div class="min-w-0 text-sm text-muted-foreground"><MusicCredit {song} album /></div>{/if}
        </div>
      </div>
      <div class="relative min-h-0 min-w-0 self-stretch">
        {#if !follow && view.lines.length}<div class="absolute top-1 right-2 z-10"><Button variant="secondary" size="sm" onclick={() => follow = true}><RotateCcw data-icon="inline-start"/>回到当前歌词</Button></div>{/if}
        <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (Named scroll region is focusable for keyboard scrolling; its actions are native buttons.) -->
        <div bind:this={pane} role="region" aria-label="歌词" tabindex="0" class="lyrics-pane absolute inset-0 min-h-0 overflow-y-auto overscroll-contain rounded-md focus-visible:outline-2 focus-visible:outline-ring" onfocusin={stopFollowing} onpointerdown={(event) => { if (event.target === pane) stopFollowing() }} onwheel={stopFollowing} ontouchstart={stopFollowing} onkeydown={(event) => { if (['ArrowUp','ArrowDown','PageUp','PageDown','Home','End',' '].includes(event.key)) stopFollowing() }}>
          {#if view.status === 'loading'}
            <div class="flex flex-col gap-6 px-3 pt-14" role="status" aria-label="正在加载歌词">{#each [72,90,58,84,65,78] as width}<Skeleton class="h-7" style={`width:${width}%`}/>{/each}</div>
          {:else if view.status === 'error'}
            <Empty.Root class="min-h-64"><Empty.Header><Empty.Title>歌词暂时无法加载</Empty.Title></Empty.Header><Empty.Content><Button variant="outline" onclick={() => controller.select(song.id,true)}>重试歌词</Button></Empty.Content></Empty.Root>
          {:else if view.status === 'empty'}
            <Empty.Root class="min-h-64"><Empty.Header><Empty.Media variant="icon"><Music2/></Empty.Media><Empty.Title>暂无歌词</Empty.Title></Empty.Header></Empty.Root>
          {:else if view.lines.length}
            <div class="flex flex-col gap-1 py-[6dvh] md:py-[15dvh]">
              {#each view.lines as line,index (line.time)}
                <Button variant="ghost" class="lyric-line h-auto min-h-12 w-full justify-start rounded-md px-3 py-3 text-left whitespace-pre-wrap" data-lyric-index={index} aria-current={active === index ? 'true' : undefined} aria-label={`${Math.floor(line.time/60)}分${Math.floor(line.time%60)}秒 ${line.text || '间奏'}`} disabled={!playerState.seekable} onclick={() => jump(line.time)}><span class="block min-w-0 break-words text-xl leading-relaxed font-semibold md:text-2xl">{line.text || '♪'}</span></Button>
              {/each}
            </div>
          {:else if view.text}
            <div class="flex flex-col gap-5 px-3 py-8"><p class="text-sm text-muted-foreground">歌词暂无时间轴</p><p class="text-xl leading-loose whitespace-pre-wrap">{view.text}</p></div>
          {/if}
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  :global(.lyric-line) { color: var(--muted-foreground); transition: color 180ms ease-out, background-color 180ms ease-out; }
  :global(.lyric-line[aria-current="true"]) { color: var(--foreground); }
  :global(.lyric-line:disabled) { opacity: 1; }
  @media (prefers-reduced-motion: reduce) { :global(.lyric-line) { transition: none; } }
</style>
