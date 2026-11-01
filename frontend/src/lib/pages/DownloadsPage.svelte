<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { Download, RotateCcw, Pause, Play, Trash2, X } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import { Checkbox } from '$lib/components/ui/checkbox'
  import * as Tabs from '$lib/components/ui/tabs'
  import * as Table from '$lib/components/ui/table'
  import * as Alert from '$lib/components/ui/alert'
  import * as AlertDialog from '$lib/components/ui/alert-dialog'
  import * as Empty from '$lib/components/ui/empty'
  import { Skeleton } from '$lib/components/ui/skeleton'
  import { mountNavigationPage, recordBrowse } from '$lib/navigation.svelte'
  import MusicCredit from '$lib/components/music/MusicCredit.svelte'
  import SongCover from '$lib/components/search/SongCover.svelte'
  import { downloadsState, downloadsController } from '$lib/downloads-state.svelte'
  import { downloadLabels, canControlDownload, type DownloadTask, type DownloadAction } from '$lib/downloads-controller'
  let filter = $state('全部'), page = $state(1), deleting = $state<DownloadTask | null>(null)
  const filters = ['全部', '下载中', '下载成功', '下载失败']
  const filtered = $derived(downloadsState.tasks.filter(task => filter === '全部' || (filter === '下载中' ? task.state === 'downloading' || task.state === 'paused' || task.state === 'queued' && (task.bytes > 0 || task.total > 0) : task.state === (filter === '下载成功' ? 'success' : 'failed'))))
  const pages = $derived(Math.max(1, Math.ceil(filtered.length / 50)))
  const currentPage = $derived(Math.min(page, pages))
  const visible = $derived(filtered.slice((currentPage - 1) * 50, currentPage * 50))
  let selected = $state<string[]>([])
  const selection = $derived(downloadsState.tasks.filter(task => selected.includes(task.id)))
  const allSelected = $derived(visible.length > 0 && visible.every(task => selected.includes(task.id)))
  const partlySelected = $derived(!allSelected && visible.some(task => selected.includes(task.id)))
  const bulkBusy = $derived(downloadsState.status !== 'ready' || downloadsState.pending.includes('download-control'))
  const canStartAll = $derived(downloadsState.status === 'ready' && downloadsState.tasks.some(task => canControlDownload(task, 'start')))
  const canPauseAll = $derived(downloadsState.status === 'ready' && downloadsState.tasks.some(task => canControlDownload(task, 'pause')))
  $effect(() => { void filter; page = 1; selected = [] })
  $effect(() => { void currentPage; selected = [] })
  async function controlSelection(action: DownloadAction) {
    const ids = selection.filter(task => canControlDownload(task, action)).map(task => task.id)
    if (ids.length && await downloadsController.control(ids, action)) selected = []
  }
  onMount(() => {
    let disposed = false, timer: ReturnType<typeof setTimeout>
    const poll = async () => { await downloadsController.refresh(); if (!disposed) timer = setTimeout(poll, 1000) }
    const binding = mountNavigationPage('下载', {
      capture: () => ({ filter, page: currentPage }),
      restore: async (data, current) => {
        if (data) filter = data.filter
        await downloadsController.refresh(); await tick()
        if (data && current()) page = data.page
      },
      refresh: () => downloadsController.refresh(),
    })
    void binding.ready.then(() => { if (!disposed) timer = setTimeout(poll, 1000) })
    return () => { disposed = true; binding.dispose(); clearTimeout(timer) }
  })
  function bytes(value: number) { return value >= 1024 * 1024 ? `${(value / 1024 / 1024).toFixed(1)} MiB` : value >= 1024 ? `${(value / 1024).toFixed(1)} KiB` : `${Math.floor(value)} B` }
  function quality(value: string) { return value === 'highest' ? '最高可用' : value.toUpperCase() }
  function progress(task: DownloadTask) { return task.total > 0 ? Math.min(99, Math.floor(task.bytes / task.total * 100)) : 0 }
  function showProgress(task: DownloadTask) { return !['success', 'skipped'].includes(task.state) && (task.total > 0 || task.bytes > 0 || task.state === 'downloading') }
</script>

<div class="mb-6 flex flex-wrap items-center justify-between gap-3">
  <h1 class="text-2xl font-semibold">下载</h1>
  <div class="flex items-center gap-2">
    <Button variant="outline" size="sm" disabled={bulkBusy || !canStartAll} onclick={() => downloadsController.control([], 'start', true)}><Play data-icon="inline-start" />全部开始</Button>
    <Button variant="outline" size="sm" disabled={bulkBusy || !canPauseAll} onclick={() => downloadsController.control([], 'pause', true)}><Pause data-icon="inline-start" />全部暂停</Button>
    <Button variant="ghost" size="icon" aria-label="刷新下载记录" title="刷新下载记录" onclick={() => downloadsController.refresh()}><RotateCcw /></Button>
  </div>
</div>
<Tabs.Root value={filter} onValueChange={value => recordBrowse(() => { filter = value; page = 1 })}>
  <div class="flex flex-wrap items-center justify-between gap-3">
    <Tabs.List aria-label="下载分类">{#each filters as name}<Tabs.Trigger value={name}>{name}</Tabs.Trigger>{/each}</Tabs.List>
    {#if selection.length}<div class="flex flex-wrap items-center gap-2" role="group" aria-label="所选下载任务操作"><span class="text-xs text-muted-foreground">已选 {selection.length} 项</span><Button variant="outline" size="sm" disabled={bulkBusy || !selection.some(task => canControlDownload(task, 'start'))} onclick={() => controlSelection('start')}>开始</Button><Button variant="outline" size="sm" disabled={bulkBusy || !selection.some(task => canControlDownload(task, 'pause'))} onclick={() => controlSelection('pause')}>暂停</Button><Button variant="outline" size="sm" disabled={bulkBusy || !selection.some(task => canControlDownload(task, 'cancel'))} onclick={() => controlSelection('cancel')}>取消</Button></div>{/if}
  </div>
  <Tabs.Content value={filter} class="mt-6">
    {#if downloadsState.status === 'loading' || downloadsState.status === 'idle'}
      <div role="status" aria-label="正在读取下载记录" class="flex flex-col gap-5">{#each [0, 1, 2, 3] as row (row)}<Skeleton class="h-16 w-full" />{/each}</div>
    {:else if downloadsState.status === 'error'}
      <Alert.Root variant="destructive"><Alert.Title>无法读取下载记录</Alert.Title><Alert.Description>请重试，不会清空已有任务或文件。</Alert.Description><Button variant="outline" class="mt-3" onclick={() => downloadsController.refresh()}>重试读取下载</Button></Alert.Root>
    {:else if !filtered.length}
      <Empty.Root class="py-12"><Empty.Header><Empty.Media variant="icon"><Download /></Empty.Media><Empty.Title>{filter === '全部' ? '还没有下载' : '暂无对应任务'}</Empty.Title><Empty.Description>{filter === '全部' ? '从搜索结果或喜欢的音乐中选择下载。' : '等待中和已跳过的任务可在全部中查看。'}</Empty.Description></Empty.Header></Empty.Root>
    {:else}
      <Table.Root class="table-fixed" aria-label="下载任务">
        <Table.Header><Table.Row><Table.Head class="w-10"><Checkbox aria-label="全选本页下载任务" disabled={bulkBusy} checked={allSelected} indeterminate={partlySelected} onCheckedChange={checked => selected = checked ? visible.map(task => task.id) : []} /></Table.Head><Table.Head>歌曲</Table.Head><Table.Head class="hidden w-20 md:table-cell">音质</Table.Head><Table.Head class="w-52 sm:w-64">状态 / 进度</Table.Head><Table.Head class="w-24 text-right"><span class="sr-only">下载操作</span></Table.Head></Table.Row></Table.Header>
        <Table.Body>
          {#each visible as task (task.id)}
            {@const pending = bulkBusy || downloadsState.pending.includes(task.id)}
            {@const active = task.state === 'downloading' || task.state === 'queued'}
            <Table.Row data-download-id={task.id} data-state={selected.includes(task.id) ? 'selected' : undefined}>
              <Table.Cell><Checkbox aria-label={`选择下载任务 ${task.song.title}`} disabled={bulkBusy} checked={selected.includes(task.id)} onCheckedChange={checked => selected = checked ? [...selected, task.id] : selected.filter(id => id !== task.id)} /></Table.Cell>
              <Table.Cell class="py-3"><div class="flex min-w-0 items-center gap-3"><SongCover src={task.song.coverUrl || ''} /><div class="min-w-0"><p class="truncate text-sm font-medium" title={`${task.song.title}\n${task.directory}${task.file ? ` / ${task.file}` : ''}`}>{task.song.title}</p><div class="mt-1 min-w-0 text-xs text-muted-foreground"><MusicCredit song={task.song} /></div></div></div></Table.Cell>
              <Table.Cell class="hidden md:table-cell"><span class="text-xs text-muted-foreground">{quality(task.quality || task.requested)}</span></Table.Cell>
              <Table.Cell>
                <div class="flex min-w-0 flex-col gap-1.5"><span class="text-xs text-muted-foreground">{task.state === 'queued' && task.message === '正在准备下载' ? '准备中' : downloadLabels[task.state]}</span>
                  {#if showProgress(task)}<progress class="h-1 w-full" value={progress(task)} max={100} aria-label={`${task.song.title} 下载进度`}></progress><div class="flex flex-wrap items-baseline gap-x-2 gap-y-1 text-xs text-muted-foreground tabular-nums"><span data-download-size class="whitespace-nowrap">{bytes(task.bytes)}{task.total > 0 ? ` / ${bytes(task.total)}` : ''}</span>{#if task.state === 'downloading'}<span data-download-speed class="whitespace-nowrap" aria-label={`${task.song.title} 下载速度`}>{downloadsState.speeds[task.id] === undefined ? '测速中' : `${bytes(downloadsState.speeds[task.id])}/s`}</span>{/if}</div>{/if}
                  {#if task.message && task.message !== '正在准备下载' && task.message !== '等待手动继续' && task.message !== '无法确认同一媒体实体，已从头下载'}<span class="line-clamp-2 text-xs leading-5 text-muted-foreground" title={task.message}>{task.message}</span>{/if}
                </div>
              </Table.Cell>
              <Table.Cell><div class="flex items-center justify-end gap-1">
                {#if active}<Button variant="ghost" size="icon-sm" disabled={pending} aria-label={`暂停下载 ${task.song.title}`} title="暂停下载" onclick={() => downloadsController.pause(task.id)}><Pause /></Button>{:else if task.state === 'paused' || task.state === 'failed'}<Button variant="ghost" size="icon-sm" disabled={pending} aria-label={`${task.state === 'failed' ? '重试下载' : '继续下载'} ${task.song.title}`} title={task.state === 'failed' ? '重试下载' : '继续下载'} onclick={() => downloadsController.resume(task.id)}>{#if task.state === 'failed'}<RotateCcw />{:else}<Play />{/if}</Button>{/if}
                {#if task.state === 'success'}<Button variant="ghost" size="icon-sm" disabled={pending} aria-label={`删除本地文件 ${task.song.title}`} title="删除本地文件" onclick={() => deleting = task}><Trash2 /></Button>{/if}
                <Button variant="ghost" size="icon-sm" disabled={pending || active} aria-label={`移除下载记录 ${task.song.title}`} title="移除记录，不删除已下载文件" onclick={() => downloadsController.remove(task.id)}><X /></Button>
              </div></Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
      {#if pages > 1}<nav aria-label="下载记录分页" class="mt-6 flex items-center justify-end gap-3"><Button variant="outline" disabled={currentPage <= 1} onclick={() => recordBrowse(() => { page = currentPage - 1 })}>上一页</Button><span class="text-xs text-muted-foreground">{currentPage}/{pages}</span><Button variant="outline" disabled={currentPage >= pages} onclick={() => recordBrowse(() => { page = currentPage + 1 })}>下一页</Button></nav>{/if}
    {/if}
  </Tabs.Content>
</Tabs.Root>
<AlertDialog.Root open={deleting !== null} onOpenChange={open => { if (!open) deleting = null }}>
  <AlertDialog.Content><AlertDialog.Header><AlertDialog.Title>删除本地文件？</AlertDialog.Title><AlertDialog.Description>将删除「{deleting?.file}」及其下载记录。此操作不可撤销，文件已发生变化时不会删除。</AlertDialog.Description></AlertDialog.Header><AlertDialog.Footer><AlertDialog.Cancel>取消</AlertDialog.Cancel><AlertDialog.Action variant="destructive" onclick={async () => { const task = deleting; if (task) await downloadsController.remove(task.id, true); deleting = null }}>删除文件</AlertDialog.Action></AlertDialog.Footer></AlertDialog.Content>
</AlertDialog.Root>

<style>
  progress { appearance: none; border: none; border-radius: 999px; overflow: hidden; background: var(--muted); color: var(--primary); }
  progress::-webkit-progress-bar { background: var(--muted); }
  progress::-webkit-progress-value { background: var(--primary); }
  progress::-moz-progress-bar { background: var(--primary); }
</style>
