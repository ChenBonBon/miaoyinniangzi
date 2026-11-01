<script lang="ts">
  import { Heart, LoaderCircle, RotateCcw } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import { favoritesState, favoritesController } from '$lib/favorites-state.svelte'
  import { cn } from '$lib/utils'
  import type { Song } from '$lib/music'
  let { song, showLabel = false, onchange }: { song: Song; showLabel?: boolean; onchange?: (added: boolean) => void } = $props()
  const liked = $derived(favoritesState.songs.some(entry => entry.id === song.id))
  const saving = $derived(favoritesState.pending.includes(song.id))
  const loading = $derived(favoritesState.status === 'idle' || favoritesState.status === 'loading')
  const failed = $derived(favoritesState.status === 'error')
  const label = $derived(failed ? `重试读取 ${song.title} 的收藏状态` : `${liked ? '取消收藏' : '收藏'} ${song.title}`)
</script>

<Button variant="ghost" size={showLabel ? 'default' : 'icon-sm'} aria-label={label} aria-pressed={failed || loading ? undefined : liked} aria-busy={saving || loading} title={failed ? '重试读取收藏' : liked ? '取消收藏' : '收藏'} disabled={saving || loading} onclick={event => {
  event.stopPropagation()
  if (failed) void favoritesController.load(true)
  else {
    const adding = !liked
    void favoritesController.toggle(song).then(saved => { if (saved) onchange?.(adding) })
  }
}}>
  {#if saving || loading}<LoaderCircle class="animate-spin" />{:else if failed}<RotateCcw />{:else}<Heart class={cn(liked && 'text-destructive')} fill={liked ? 'currentColor' : 'none'} />{/if}
  {#if showLabel}<span>{failed ? '重试读取收藏' : saving ? '正在保存' : liked ? '已收藏' : '收藏'}</span>{/if}
</Button>
