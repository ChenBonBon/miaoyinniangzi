<script lang="ts">
  import { toast } from 'svelte-sonner'
  import { Button } from '$lib/components/ui/button'
  import * as AlertDialog from '$lib/components/ui/alert-dialog'
  import * as Alert from '$lib/components/ui/alert'
  import { deleteSource, type SourceInfo } from '$lib/sources'
  import { errorMessage } from '$lib/settings'

  let { source, onclose, onremoved, returnFocus }: {
    source: SourceInfo | null
    onclose: () => void
    onremoved: (id: string) => void
    returnFocus?: HTMLElement
  } = $props()
  let deleting = $state(false)
  let error = $state('')
  $effect(() => { void source?.id; error = '' })

  async function confirm() {
    if (!source || deleting) return
    deleting = true; error = ''
    const id = source.id
    try {
      if (!await deleteSource(id)) throw new Error('音源未删除，请重试。')
      onremoved(id)
      onclose()
      toast.success('音源已删除')
    } catch (cause) { error = errorMessage(cause); toast.error('音源删除失败') }
    finally { deleting = false }
  }
</script>

<AlertDialog.Root open={source !== null} onOpenChange={(open) => { if (!open && !deleting) onclose() }}>
  <AlertDialog.Content onEscapeKeydown={(event) => { if (deleting) event.preventDefault() }} onCloseAutoFocus={(event) => { event.preventDefault(); (returnFocus?.isConnected ? returnFocus : document.getElementById('source-import'))?.focus() }}>
    <AlertDialog.Header>
      <AlertDialog.Title>删除音源？</AlertDialog.Title>
      <AlertDialog.Description>将删除「{source?.name}」在应用内的加密副本，不影响原始脚本文件。</AlertDialog.Description>
    </AlertDialog.Header>
    {#if error}<Alert.Root variant="destructive"><Alert.Title>删除失败</Alert.Title><Alert.Description>{error}</Alert.Description></Alert.Root>{/if}
    <AlertDialog.Footer>
      <AlertDialog.Cancel disabled={deleting}>取消</AlertDialog.Cancel>
      <Button variant="destructive" disabled={deleting} onclick={confirm}>{deleting ? '删除中…' : '删除音源'}</Button>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
