<script lang="ts">
  import { Disc3 } from '@lucide/svelte'
  import { cn } from '$lib/utils'
  let { src, large = false, compact = false }: { src: string; large?: boolean; compact?: boolean } = $props()
  let failed = $state(false)
  $effect(() => { void src; failed = false })
</script>
<div class={cn('flex shrink-0 items-center justify-center overflow-hidden rounded-md bg-muted text-muted-foreground', large ? 'size-24' : compact ? 'size-10' : 'size-16')}>
  {#if src && !failed}<img {src} alt="" loading="lazy" decoding="async" referrerpolicy="no-referrer" class="size-full object-cover" onerror={() => failed = true} />{:else}<Disc3 class={large ? 'size-10' : compact ? 'size-5' : 'size-7'} aria-hidden="true" />{/if}
</div>
