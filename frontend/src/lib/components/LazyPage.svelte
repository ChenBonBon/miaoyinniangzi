<script lang="ts">
  import type { Component } from 'svelte'
  import { Button } from '$lib/components/ui/button'
  import { Skeleton } from '$lib/components/ui/skeleton'
  import * as Alert from '$lib/components/ui/alert'

  let { loader }: { loader: () => Promise<{ default: Component }> } = $props()
  let attempt = $state(0)
  let pending = $derived.by(() => { void attempt; return loader() })
</script>

{#await pending}
  <div class="flex flex-col gap-5" role="status" aria-label="正在加载页面"><Skeleton class="h-8 w-40" /><Skeleton class="h-32 w-full" /></div>
{:then module}
  <module.default />
{:catch}
  <Alert.Root variant="destructive"><Alert.Title>页面加载失败</Alert.Title><Alert.Description>请重新加载此页面。<Button variant="outline" onclick={() => attempt++}>重试加载</Button></Alert.Description></Alert.Root>
{/await}
