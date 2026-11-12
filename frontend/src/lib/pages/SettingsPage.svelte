<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { Music2, Download, Radio, Palette } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import { Skeleton } from '$lib/components/ui/skeleton'
  import { Separator } from '$lib/components/ui/separator'
  import * as Alert from '$lib/components/ui/alert'
  import AppearanceSettings from './settings/AppearanceSettings.svelte'
  import PlaybackSettings from './settings/PlaybackSettings.svelte'
  import DownloadSettings from './settings/DownloadSettings.svelte'
  import SourceSettings from './settings/SourceSettings.svelte'
  import { navigation, mountNavigationPage, recordBrowse, type SettingsSection } from '$lib/navigation.svelte'
  import { settingsState, ensureSettings, retrySettingsSave, refreshSettings } from '$lib/settings-state.svelte'

  import { refreshSources } from '$lib/source-state.svelte'

  const categories = [{ name:'外观', id:'settings-appearance', icon:Palette }, { name:'播放', id:'settings-playback', icon:Music2 }, { name:'下载', id:'settings-download', icon:Download }, { name:'音源', id:'settings-source', icon:Radio }] as const
  let content: HTMLDivElement
  let menu: HTMLElement

  function goTo(name: SettingsSection, smooth = true) {
    const category = categories.find(item => item.name === name)!
    const target = content.querySelector<HTMLElement>(`#${category.id}`)
    navigation.settingsSection = name
    target?.scrollIntoView({ block:'start', behavior: smooth && !matchMedia('(prefers-reduced-motion: reduce)').matches ? 'smooth' : 'instant' })
    if (smooth) target?.focus({ preventScroll:true })
  }

  onMount(() => {
    let disposed = false
    let frame = 0
    let cleanup = () => {}
    const binding = mountNavigationPage('设置', {
      capture: () => ({ section: navigation.settingsSection }),
      restore: async (data, current) => { await ensureSettings(); await tick(); if (disposed || !current()) return; if (data) navigation.settingsSection = data.section; goTo(navigation.settingsSection, false) },
      refresh: async () => { await refreshSettings(); await refreshSources() },
    })
    // Wait for persisted fields to settle before observing section positions.
    void (async () => {
      await binding.ready
      await tick()
      if (disposed) return
      const main = content.closest('[data-content-scroll]')!
      const sections = categories.map(category => content.querySelector<HTMLElement>(`#${category.id}`)!)
      function update() {
        const containedScroll = getComputedStyle(main).overflowY === 'auto'
        const top = containedScroll ? main.getBoundingClientRect().top : content.closest('main')!.querySelector('header')!.getBoundingClientRect().bottom
        const anchorLine = window.innerWidth >= 1024 ? top + 40 : Math.max(top, menu.getBoundingClientRect().bottom) + 8
        const scroller = containedScroll ? main : document.documentElement
        const atBottom = scroller.scrollTop > 0 && scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - 2
        let active = 0
        sections.forEach((section, index) => { if (section.getBoundingClientRect().top <= anchorLine) active = index })
        navigation.settingsSection = categories[atBottom ? categories.length - 1 : active].name
      }
      function schedule() { cancelAnimationFrame(frame); frame = requestAnimationFrame(update) }
      main.addEventListener('scroll', schedule, { passive:true })
      window.addEventListener('scroll', schedule, { passive:true })
      window.addEventListener('resize', schedule)
      cleanup = () => {
        main.removeEventListener('scroll', schedule)
        window.removeEventListener('scroll', schedule)
        window.removeEventListener('resize', schedule)
      }
    })()
    return () => { disposed = true; binding.dispose(); cleanup(); cancelAnimationFrame(frame) }
  })
</script>

<div class="flex flex-col gap-8 lg:flex-row lg:gap-12">
  <nav bind:this={menu} class="sticky top-0 z-10 flex shrink-0 flex-wrap gap-2 bg-background py-2 lg:top-8 lg:w-36 lg:flex-col lg:self-start" aria-label="设置分类">
    {#each categories as category}
      <Button href={`#${category.id}`} variant={navigation.settingsSection === category.name ? 'secondary' : 'ghost'} class="justify-start gap-3" aria-current={navigation.settingsSection === category.name ? 'location' : undefined} onclick={(event) => { event.preventDefault(); recordBrowse(() => goTo(category.name)) }}><category.icon data-icon="inline-start" />{category.name}</Button>
    {/each}
  </nav>
  <div bind:this={content} class="@container/settings-content min-w-0 flex-1 max-w-3xl">
    {#if settingsState.error}
      <Alert.Root variant="destructive" class="mb-6"><Alert.Title>设置未完成</Alert.Title><Alert.Description>{settingsState.error}</Alert.Description></Alert.Root>
    {/if}
    {#if !settingsState.draft && !settingsState.loading}<Button variant="outline" class="mb-6" onclick={ensureSettings}>重新读取设置</Button>{/if}
    <section id="settings-appearance" tabindex="-1" aria-label="外观设置" class="scroll-mt-14 rounded-lg outline-none lg:scroll-mt-8"><AppearanceSettings /></section>
    <Separator class="my-12" />
    <section id="settings-playback" tabindex="-1" aria-label="播放设置" class="scroll-mt-14 rounded-lg outline-none lg:scroll-mt-8">
      {#if settingsState.draft}<PlaybackSettings />{:else}<h2 class="mb-8 text-xl font-semibold">播放</h2><Skeleton class="h-44 w-full" aria-label="播放设置尚未读取" />{/if}
    </section>
    <Separator class="my-12" />
    <section id="settings-download" tabindex="-1" aria-label="下载设置" class="scroll-mt-14 rounded-lg outline-none lg:scroll-mt-8">
      {#if settingsState.draft}<DownloadSettings />{:else}<h2 class="mb-8 text-xl font-semibold">下载</h2><Skeleton class="h-44 w-full" aria-label="下载设置尚未读取" />{/if}
    </section>
    <Separator class="my-12" />
    <section id="settings-source" tabindex="-1" aria-label="音源设置" class="scroll-mt-14 rounded-lg outline-none lg:scroll-mt-8"><SourceSettings /></section>
    {#if settingsState.status === 'error'}
      <Button class="mt-6" variant="outline" onclick={retrySettingsSave}>重试保存</Button>
    {/if}
  </div>
</div>
