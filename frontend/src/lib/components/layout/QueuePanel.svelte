<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { ListMusic, ListEnd, LocateFixed, X, Volume2, Pause, LoaderCircle } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import * as AlertDialog from '$lib/components/ui/alert-dialog'
  import * as Empty from '$lib/components/ui/empty'
  import SongCover from '$lib/components/search/SongCover.svelte'
  import { playerController, playerState } from '$lib/player-state.svelte'
  import { closeQueue } from '$lib/navigation.svelte'
  import { cn } from '$lib/utils'

  const rowHeight = 64
  let viewport: HTMLDivElement
  let heading: HTMLHeadingElement
  let scrollTop = $state(0)
  let height = $state(400)
  let confirmClear = $state(false)
  const count = $derived(playerState.queue.length)
  const start = $derived(Math.max(0, Math.min(Math.floor(scrollTop / rowHeight) - 5, count - 1)))
  const end = $derived(Math.min(count, start + Math.ceil(height / rowHeight) + 11))
  const currentID = $derived(playerState.queue[playerState.index]?.id)

  function locateCurrent() {
    if (!viewport || playerState.index < 0) return
    viewport.scrollTop = Math.max(0, playerState.index * rowHeight - (viewport.clientHeight - rowHeight) / 2)
    scrollTop = viewport.scrollTop
  }
  async function focusRow(index: number, action = 'play') {
    if (index < 0 || index >= count) return
    if (index < start || index >= end) {
      viewport.scrollTop = Math.max(0, index * rowHeight - height / 2)
      scrollTop = viewport.scrollTop
    }
    await tick()
    viewport.querySelector<HTMLElement>(`[data-queue-index="${index}"] [data-action="${action}"]`)?.focus({ preventScroll: true })
  }
  async function remove(index: number) {
    playerController.removeQueue(index)
    await tick()
    if (count) await focusRow(Math.min(index, count - 1), 'remove')
    else heading?.focus()
  }
  async function moveNext(index: number) {
    if (playerController.playNext(playerState.queue[index], index)) {
      await tick()
      await focusRow(playerState.index + 1, 'next')
    }
  }
  function keyboard(event: KeyboardEvent) {
    if (event.altKey || event.ctrlKey || event.metaKey) return
    const row = (event.target as HTMLElement).closest<HTMLElement>('[data-queue-index]')
    if (!row || !['ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const index = Number(row.dataset.queueIndex)
    const target = event.key === 'Home' ? 0 : event.key === 'End' ? count - 1 : index + (event.key === 'ArrowDown' ? 1 : -1)
    void focusRow(Math.max(0, Math.min(count - 1, target)))
  }
  $effect(() => {
    void currentID
    void playerState.selectionRevision
    untrack(() => { void tick().then(locateCurrent) })
  })
  onMount(() => {
    if (document.activeElement?.id === 'player-queue-toggle') heading.focus({ preventScroll: true })
    const observer = new ResizeObserver(() => height = viewport.clientHeight)
    observer.observe(viewport)
    return () => observer.disconnect()
  })
</script>

<svelte:window onkeydown={event => {
  if (event.key === 'Escape' && !event.defaultPrevented && !confirmClear) { event.preventDefault(); closeQueue() }
}} />

<aside id="queue-panel" aria-labelledby="queue-heading" class="flex min-h-0 w-full shrink-0 flex-col bg-sidebar md:w-80 md:border-l xl:w-96">
  <div class="flex h-14 shrink-0 items-center justify-between gap-3 px-4">
    <h2 id="queue-heading" bind:this={heading} tabindex="-1" class="min-w-0 text-base font-semibold outline-none">播放队列 <span class="ml-1 text-sm font-normal text-muted-foreground tabular-nums">{count}</span></h2>
    <Button variant="ghost" size="icon-sm" aria-label="关闭播放队列" onclick={closeQueue}><X /></Button>
  </div>
  <div class="flex shrink-0 items-center justify-between gap-2 border-b px-3 pb-2">
    <Button variant="ghost" size="sm" disabled={playerState.index < 0} onclick={() => { locateCurrent(); void focusRow(playerState.index) }}><LocateFixed data-icon="inline-start" />当前歌曲</Button>
    <Button variant="ghost" size="sm" disabled={!count} onclick={() => confirmClear = true}>清空</Button>
  </div>
  <p id="queue-keyboard-help" class="sr-only">歌曲按钮上可用上下方向键浏览，Home 和 End 跳到首尾，Escape 关闭队列。</p>
  <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (Named scroll region supports native keyboard scrolling; row actions remain native buttons.) -->
  <div bind:this={viewport} class="min-h-0 flex-1 overflow-y-auto overscroll-contain focus-visible:outline-2 focus-visible:outline-ring" onscroll={() => scrollTop = viewport.scrollTop} onkeydown={keyboard} role="region" aria-label="队列歌曲" aria-describedby="queue-keyboard-help" tabindex="0">
    {#if count}
      <ol class="m-0 list-none px-2" style:padding-top={`${start * rowHeight}px`} style:padding-bottom={`${(count - end) * rowHeight}px`} start={start + 1}>
        {#each playerState.queue.slice(start, end) as song, offset}
          {@const index = start + offset}
          {@const current = index === playerState.index}
          <li data-queue-index={index} aria-current={current ? 'true' : undefined} aria-setsize={count} aria-posinset={index + 1} class={cn('flex h-16 min-w-0 items-center gap-1 rounded-md', current && 'bg-accent')}>
            <Button data-action="play" variant="ghost" class="h-full min-w-0 flex-1 justify-start gap-3 px-2" aria-label={`播放队列中的 ${song.title}`} onclick={() => playerController.playQueue(index)}>
              <SongCover src={song.coverUrl} />
              <span class="flex min-w-0 flex-1 flex-col gap-1 text-left"><span class={cn('truncate text-sm', current && 'font-semibold text-foreground')}>{song.title}</span><span class="truncate text-xs font-normal text-muted-foreground">{song.artist || '未知歌手'}</span></span>
              {#if current}{#if playerState.status === 'playing'}<Volume2 aria-hidden="true" />{:else if playerState.status === 'resolving' || playerState.status === 'buffering'}<LoaderCircle class="animate-spin" aria-hidden="true" />{:else}<Pause aria-hidden="true" />{/if}{/if}
            </Button>
            <Button data-action="next" variant="ghost" size="icon-sm" disabled={current} aria-label={`下一首播放 ${song.title}`} title="下一首播放" onclick={() => moveNext(index)}><ListEnd /></Button>
            <Button data-action="remove" variant="ghost" size="icon-sm" aria-label={`从队列移除 ${song.title}`} title="从队列移除" onclick={() => remove(index)}><X /></Button>
          </li>
        {/each}
      </ol>
    {:else}
      <Empty.Root class="h-full rounded-none"><Empty.Header><Empty.Media variant="icon"><ListMusic /></Empty.Media><Empty.Title>队列为空</Empty.Title><Empty.Description>从搜索结果选择歌曲，或添加为下一首播放。</Empty.Description></Empty.Header></Empty.Root>
    {/if}
  </div>
</aside>

<AlertDialog.Root bind:open={confirmClear}>
  <AlertDialog.Content onCloseAutoFocus={event => { event.preventDefault(); heading?.focus({ preventScroll: true }) }}>
    <AlertDialog.Header><AlertDialog.Title>清空播放队列？</AlertDialog.Title><AlertDialog.Description>将停止播放并移除队列中的全部歌曲，不影响收藏或本地文件。</AlertDialog.Description></AlertDialog.Header>
    <AlertDialog.Footer><AlertDialog.Cancel>取消</AlertDialog.Cancel><AlertDialog.Action variant="destructive" onclick={() => playerController.clearQueue()}>清空队列</AlertDialog.Action></AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
