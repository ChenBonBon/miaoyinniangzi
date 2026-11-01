<script lang="ts">
  import { Music2 } from '@lucide/svelte'
  let { src }: { src: string } = $props()
  let failed = $state(false)
  $effect(() => { void src; failed = false })
</script>

<div class="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-md bg-muted text-muted-foreground">
  {#if src && !failed}<img {src} alt="" loading="lazy" decoding="async" referrerpolicy="no-referrer" class="size-full object-cover" onerror={() => failed = true} />{:else}<Music2 class="size-5" aria-hidden="true" />{/if}
</div>
