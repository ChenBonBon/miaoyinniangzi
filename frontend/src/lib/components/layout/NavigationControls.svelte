<script lang="ts">
  import { ArrowLeft, ArrowRight, RotateCcw } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import { browsing, goBack, goForward, refreshPage } from '$lib/navigation.svelte'
</script>
<nav class="flex shrink-0 items-center gap-1" aria-label="浏览导航">
  <Button variant="ghost" size="icon-sm" aria-label="后退" title="后退" disabled={!browsing.back} onclick={goBack}><ArrowLeft /></Button>
  <Button variant="ghost" size="icon-sm" aria-label="前进" title="前进" disabled={!browsing.forward} onclick={goForward}><ArrowRight /></Button>
  <Button variant="ghost" size="icon-sm" aria-label="刷新" title="刷新当前页面" disabled={browsing.refreshing || browsing.restoring} onclick={() => refreshPage()}><RotateCcw /></Button>
</nav>
