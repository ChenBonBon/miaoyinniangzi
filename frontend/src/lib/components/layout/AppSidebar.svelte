<script lang="ts">
  import { Compass, Heart, Download, Settings2 } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import { cn } from '$lib/utils'
  import { navigation, navigate } from '$lib/navigation.svelte'

  const pages = [{ name:'推荐', icon:Compass }, { name:'喜欢', icon:Heart }, { name:'下载', icon:Download }, { name:'设置', icon:Settings2 }] as const
</script>

<aside class="flex flex-col bg-sidebar px-2 py-8 md:px-4">
  <a class="mb-10 flex items-center gap-3 rounded-lg px-2 focus-visible:ring-2 focus-visible:ring-ring md:px-3" href="#推荐" onclick={() => navigate('推荐')} aria-label="妙音娘子首页"><span class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary font-serif text-primary-foreground">妙</span><span class="hidden text-lg font-semibold md:block">妙音娘子</span></a>
  <nav class="flex flex-col gap-2" aria-label="主导航">
    {#each pages as item}
      <Button variant={navigation.page === item.name ? 'secondary' : 'ghost'} class={cn('h-12 justify-center gap-4 md:justify-start md:px-4', item.name === '设置' && 'mt-7')} aria-label={item.name} aria-current={navigation.page === item.name ? 'page' : undefined} onclick={() => navigate(item.name)}><item.icon data-icon="inline-start" /><span class="hidden md:inline">{item.name}</span></Button>
    {/each}
  </nav>
  <div class="mt-auto hidden px-4 pt-16 text-xs text-muted-foreground md:block">仅 QQ 音乐<p class="mt-3">本地版 · macOS</p></div>
</aside>
