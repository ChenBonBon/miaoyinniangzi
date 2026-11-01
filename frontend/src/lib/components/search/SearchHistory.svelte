<script lang="ts">
  import { onMount } from 'svelte'
  import { X } from '@lucide/svelte'
  import { Badge } from '$lib/components/ui/badge'
  import { Button } from '$lib/components/ui/button'
  import { Skeleton } from '$lib/components/ui/skeleton'
  import * as Alert from '$lib/components/ui/alert'
  import { historyState, historyController } from '$lib/search-history-state.svelte'
  import { searchForm, searchController } from '$lib/search-state.svelte'
  import { navigate } from '$lib/navigation.svelte'

  function searchAgain(query: string) {
    navigate('搜索', () => { searchForm.query = query; void searchController.search(query) })
  }
  onMount(() => { void historyController.load() })
</script>

{#if historyState.items.length || historyState.busy || historyState.error}
  <section aria-label="历史搜索" class="mb-8">
    <div class="mb-3 flex items-center justify-between gap-3"><h2 class="text-sm font-medium">历史搜索</h2>{#if historyState.items.length}<Button size="xs" variant="ghost" disabled={historyState.busy} onclick={() => { void historyController.clear() }}>清空历史</Button>{/if}</div>
    {#if !historyState.loaded && historyState.busy}
      <div class="flex gap-2" role="status" aria-label="正在读取历史搜索"><Skeleton class="h-7 w-24 rounded-full" /><Skeleton class="h-7 w-32 rounded-full" /></div>
    {:else if historyState.items.length}
      <div class="flex flex-wrap gap-2">
        {#each historyState.items as query (query)}
          <Badge variant="secondary" class="h-auto max-w-full gap-0 overflow-visible p-0.5">
            <Button size="xs" variant="ghost" class="min-w-0 max-w-52" aria-label={`搜索历史 ${query}`} onclick={() => searchAgain(query)}><span class="truncate">{query}</span></Button>
            <Button size="icon-xs" variant="ghost" disabled={historyState.busy} aria-label={`删除历史搜索 ${query}`} onclick={() => { void historyController.remove(query) }}><X /></Button>
          </Badge>
        {/each}
      </div>
    {/if}
    {#if historyState.error}<Alert.Root variant="destructive" class="mt-3"><Alert.Title>历史搜索未更新</Alert.Title><Alert.Description>{historyState.error}</Alert.Description><Button variant="outline" class="mt-2 w-fit" disabled={historyState.busy} onclick={() => { void historyController.load(true) }}>重新读取历史</Button></Alert.Root>{/if}
  </section>
{/if}
