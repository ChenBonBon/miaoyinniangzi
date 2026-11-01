<script lang="ts">
  import { onMount } from 'svelte'
  import { toast } from 'svelte-sonner'
  import { Trash2 } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import * as RadioGroup from '$lib/components/ui/radio-group'
  import { Badge } from '$lib/components/ui/badge'
  import { Skeleton } from '$lib/components/ui/skeleton'
  import { Separator } from '$lib/components/ui/separator'
  import * as Field from '$lib/components/ui/field'
  import * as Alert from '$lib/components/ui/alert'
  import SettingRow from '$lib/components/settings/SettingRow.svelte'
  import { importSource, checkSource, activateSource, disableSource, type SourceInfo } from '$lib/sources'
  import { sourceState, refreshSources } from '$lib/source-state.svelte'
  import DeleteSourceDialog from '$lib/components/settings/DeleteSourceDialog.svelte'
  import { errorMessage } from '$lib/settings'

  let entries = $derived(sourceState.entries)
  let activeId = $derived(entries.find(entry => entry.active)?.id ?? '')
  let loading = $state(true)
  let busy = $state(false)
  let importing = $state(false)
  let checkingId = $state('')
  let error = $state('')
  let ready = $state(false)
  let pendingDelete = $state<SourceInfo | null>(null)
  let returnFocus = $state<HTMLElement>()
  async function load() {
    loading = true; error = ''
    try { await refreshSources(); ready = true }
    catch (cause) { error = errorMessage(cause) }
    finally { loading = false }
  }
  async function importScript() {
    if (busy) return
    busy = true; importing = true; error = ''
    try {
      const result = await importSource()
      if (!result) return
      sourceState.entries = entries.some(entry => entry.id === result.source.id)
        ? entries.map(entry => entry.id === result.source.id ? result.source : entry)
        : [result.source, ...entries]
      if (result.validationError) {
        error = `音源已保存，但校验未完成：${result.validationError}`
        toast.error('音源已保存，校验未完成')
      } else if (result.source.checkStatus !== 'ready') toast.error('音源已保存，校验未通过')
      else toast.success(result.duplicate ? '音源已存在，校验通过' : '音源已导入并通过校验')
    } catch (cause) { error = errorMessage(cause); toast.error('音源导入失败') }
    finally { busy = false; importing = false }
  }
  async function run(kind: 'check' | 'activate' | 'disable', id = '') {
    if (busy) return
    busy = true; error = ''; checkingId = kind === 'check' ? id : ''
    try {
      sourceState.entries = await (kind === 'check' ? checkSource(id) : kind === 'activate' ? activateSource(id) : disableSource())
      if (kind === 'check' && entries.find(entry => entry.id === id)?.checkStatus !== 'ready') toast.error('音源兼容性校验未通过')
      else toast.success(kind === 'check' ? '本地兼容性校验通过' : kind === 'activate' ? '音源已启用' : '音源已禁用')
    } catch (cause) {
      error = errorMessage(cause)
      // A failed re-check can invalidate the old selection in the backend.
      try { await refreshSources() } catch { /* Preserve the original error. */ }
      toast.error(kind === 'check' ? '音源校验失败' : '音源状态更新失败')
    } finally { busy = false; checkingId = '' }
  }
  onMount(() => { void load() })
</script>

<DeleteSourceDialog source={pendingDelete} {returnFocus} onclose={() => pendingDelete = null} onremoved={(id) => sourceState.entries = entries.filter(entry => entry.id !== id)} />

<h2 class="mb-2 text-xl font-semibold">音源</h2>
<Field.Group class="gap-0">
  <SettingRow id="source-import" label="本地音源" description="导入洛雪兼容 .js 音源，加密保存后自动离线校验。原文件删除不影响副本，密钥由 macOS 钥匙串保管。导入后自动校验，通过后单选启用，同一时间只启用一个。校验仅验证本地 QQ 音乐兼容性，不联网验证音源服务。" labelFor={false}>
    {#snippet control()}<Button id="source-import" disabled={busy || loading || !ready} variant="outline" aria-describedby="source-import-description" onclick={importScript}>{importing ? '导入并校验中…' : busy ? '处理中…' : '导入脚本'}</Button>{/snippet}
  </SettingRow>
  {#if error}<Alert.Root variant="destructive" class="mb-4"><Alert.Title>音源操作未完成</Alert.Title><Alert.Description>{error}</Alert.Description></Alert.Root>{/if}
  {#if loading}
    <Skeleton class="mb-6 h-14 w-full" aria-label="正在读取音源" />
  {:else if !ready}
    <Button class="mb-6 w-fit" variant="outline" onclick={load}>重新读取音源</Button>
  {:else if entries.length}
    <RadioGroup.Root bind:value={() => activeId, (id) => { if (id && id !== activeId) void run('activate', id) }} aria-label="已导入音源" aria-describedby="source-import-description" class="mb-4 gap-0">
      {#each entries as entry (entry.id)}
        <div class="flex items-start gap-3 py-4">
          <RadioGroup.Item value={entry.id} disabled={busy || entry.checkStatus !== 'ready'} aria-label={`启用音源 ${entry.name}`} class="mt-1" />
          <div class="min-w-0 flex-1">
            <p class="break-words text-sm font-medium">{entry.name}</p>
            <p class="mt-1 break-words text-xs text-muted-foreground">{[entry.version, entry.author].filter(Boolean).join(' · ') || '未提供版本及作者'}</p>
            <div class="mt-3 flex flex-wrap items-center gap-2">
              <Badge variant={entry.active ? 'default' : entry.checkStatus === 'failed' ? 'destructive' : 'secondary'}>{entry.active ? '已启用' : entry.checkStatus === 'ready' ? '校验通过' : entry.checkStatus === 'failed' ? '校验未通过' : '待兼容性校验'}</Badge>
              <Button size="sm" variant="outline" disabled={busy} aria-label={`校验音源 ${entry.name}`} onclick={() => run('check', entry.id)}>{checkingId === entry.id ? '校验中…' : entry.checkStatus === 'unchecked' ? '校验' : '重新校验'}</Button>
              {#if entry.active}<Button size="sm" variant="outline" disabled={busy} onclick={() => run('disable')}>禁用</Button>{/if}
            </div>
            {#if entry.checkError}<p class="mt-2 text-xs leading-6 text-destructive">{entry.checkError}</p>{/if}
          </div>
          <Button variant="ghost" size="icon" disabled={busy} aria-label={`删除音源 ${entry.name}`} title="删除音源" onclick={(event) => { returnFocus = event.currentTarget; pendingDelete = entry }}><Trash2 /></Button>
        </div>
        <Separator />
      {/each}
    </RadioGroup.Root>
  {:else}<p class="mb-6 text-sm text-muted-foreground">尚未导入音源。仅导入你信任的脚本。</p>{/if}
</Field.Group>
