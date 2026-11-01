<script lang="ts">
  import { onMount } from 'svelte'
  import { mountNavigationPage, recordBrowse } from '$lib/navigation.svelte'
  import { Search, SearchX } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import { Skeleton } from '$lib/components/ui/skeleton'
  import * as Alert from '$lib/components/ui/alert'
  import * as Empty from '$lib/components/ui/empty'
  import SearchHistory from '$lib/components/search/SearchHistory.svelte'
  import SearchResults from '$lib/components/search/SearchResults.svelte'
  import { searchState, searchController, searchForm } from '$lib/search-state.svelte'
  onMount(() => {
    const binding = mountNavigationPage('搜索', {
      capture: () => ({ snapshot: { ...searchState }, form: searchState.query || searchForm.query }),
      restore: async data => { if (data) { searchForm.query = data.form; await searchController.restore(data.snapshot) } },
      refresh: () => searchState.query ? searchController.retry() : undefined,
    })
    return binding.dispose
  })
  let resultsRoot: HTMLDivElement
  function turnPage(page: number) {
    recordBrowse(() => { void searchController.search(searchState.query, page) })
    const main = resultsRoot.closest('[data-content-scroll]')
    if (main && getComputedStyle(main).overflowY === 'auto') main.scrollTo({ top:0, behavior:'instant' })
    else window.scrollTo({ top:0, behavior:'instant' })
  }
</script>

<h1 class="sr-only">歌曲搜索</h1>
<SearchHistory />
<div bind:this={resultsRoot} aria-busy={searchState.status === 'loading'}>
  {#if searchState.status === 'loading'}
    <div role="status" aria-label="正在搜索 QQ 音乐" class="flex flex-col gap-5">
      <Skeleton class="h-6 w-48" />
      {#each [0,1,2,3,4,5] as row (row)}<div class="flex items-center gap-3"><Skeleton class="size-10" /><div class="flex flex-1 flex-col gap-2"><Skeleton class="h-4 w-2/3" /><Skeleton class="h-3 w-1/3" /></div></div>{/each}
    </div>
  {:else if searchState.status === 'error'}
    <Alert.Root variant="destructive"><Alert.Title>搜索未完成</Alert.Title><Alert.Description>{searchState.error}</Alert.Description><Button variant="outline" class="mt-3 w-fit" onclick={() => { void searchController.retry() }}>重试搜索</Button></Alert.Root>
  {:else if searchState.status === 'success' && searchState.result}
    {#if searchState.result.songs.length}
      <SearchResults result={searchState.result} onpage={turnPage} />
    {:else}
      <Empty.Root class="py-14"><Empty.Header><Empty.Media variant="icon"><SearchX /></Empty.Media><Empty.Title>没有找到相关歌曲</Empty.Title><Empty.Description>试试其他歌名、歌手或专辑名称。</Empty.Description></Empty.Header></Empty.Root>
    {/if}
  {:else if searchState.status === 'canceled'}
    <Empty.Root class="py-14"><Empty.Header><Empty.Title>搜索已取消</Empty.Title></Empty.Header><Empty.Content><Button variant="outline" onclick={() => { void searchController.retry() }}>重新搜索</Button></Empty.Content></Empty.Root>
  {:else}
    <Empty.Root class="py-14"><Empty.Header><Empty.Media variant="icon"><Search /></Empty.Media><Empty.Title>输入关键词开始搜索</Empty.Title><Empty.Description>仅搜索 QQ 音乐歌曲，无需启用音源。</Empty.Description></Empty.Header></Empty.Root>
  {/if}
</div>
