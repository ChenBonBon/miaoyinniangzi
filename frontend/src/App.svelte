<script lang="ts">
  import { onMount } from 'svelte'
  import { fly } from 'svelte/transition'
  import { Toaster } from '$lib/components/ui/sonner'
  import AppSidebar from '$lib/components/layout/AppSidebar.svelte'
  import PlayerBar from '$lib/components/layout/PlayerBar.svelte'
  import LazyPage from '$lib/components/LazyPage.svelte'
  import { navigation } from '$lib/navigation.svelte'
  import { cn } from '$lib/utils'
  import SearchForm from '$lib/components/search/SearchForm.svelte'
  import NavigationControls from '$lib/components/layout/NavigationControls.svelte'
  import { initializePlayer,playerController } from '$lib/player-state.svelte'
  import { registerWindowLifecycle } from '$lib/window-lifecycle'
  import { initializeDownloads } from '$lib/downloads-state.svelte'
  import { initializeFavorites } from '$lib/favorites-state.svelte'

  const queueLoader = () => import('$lib/components/layout/QueuePanel.svelte')
  const loaders = {
    音乐详情: () => import('$lib/pages/MusicDetailPage.svelte'),
    正在播放: () => import('$lib/pages/NowPlayingPage.svelte'),
    推荐: () => import('$lib/pages/DiscoverPage.svelte'),
    喜欢: () => import('$lib/pages/FavoritesPage.svelte'),
    搜索: () => import('$lib/pages/SearchPage.svelte'),
    下载: () => import('$lib/pages/DownloadsPage.svelte'),
    设置: () => import('$lib/pages/SettingsPage.svelte'),
  }
  let reduced = $state(true)
  let pageSeen: string | undefined
  $effect(() => {
    const page = navigation.page
    if (pageSeen !== undefined && pageSeen !== page && matchMedia('(max-width: 767px)').matches) navigation.queueOpen = false
    pageSeen = page
  })
  onMount(() => {
    initializePlayer()
    initializeFavorites()
    initializeDownloads()
    const lifecycle=registerWindowLifecycle(()=>playerController.prepareQuit())
    const closing=()=>playerController.stop()
    window.addEventListener('beforeunload',closing)
    const media = matchMedia('(prefers-reduced-motion: reduce)')
    const update = () => reduced = media.matches
    update(); media.addEventListener('change', update)
    return () => { lifecycle();media.removeEventListener('change', update);window.removeEventListener('beforeunload',closing) }
  })
</script>

<svelte:head><title>妙音娘子</title><meta name="description" content="专注音乐的本地播放器" /></svelte:head>

<Toaster position="top-right" theme="dark" richColors />
<div class="grid h-dvh grid-cols-[74px_1fr] grid-rows-[1fr_auto] md:grid-cols-[210px_1fr] md:grid-rows-[1fr_100px]">
  <AppSidebar />
  <main class="flex min-w-0 flex-col overflow-hidden">
    <header class="flex min-h-20 shrink-0 items-center gap-3 bg-background px-6 py-4 md:px-12">
      <NavigationControls />
      <SearchForm />
    </header>
    <div class="flex min-h-0 flex-1">
    <div data-content-scroll class={cn('min-h-0 min-w-0 flex-1 overflow-auto', navigation.queueOpen && 'hidden md:block')}>
    {#key navigation.page}
      <section class={cn('mx-auto w-full max-w-7xl px-6 md:px-12', navigation.page === '正在播放' ? 'flex h-full min-h-0 flex-col py-4 md:py-6' : 'pt-8 pb-14')} in:fly={{ y: reduced ? 0 : 10, duration: reduced ? 0 : 220 }}>
        <LazyPage loader={loaders[navigation.page]} />
      </section>
    {/key}
    </div>
    {#if navigation.queueOpen}<div class="flex min-h-0 w-full shrink-0 md:w-80 xl:w-96"><LazyPage loader={queueLoader} /></div>{/if}
    </div>
  </main>
  <PlayerBar />
</div>
