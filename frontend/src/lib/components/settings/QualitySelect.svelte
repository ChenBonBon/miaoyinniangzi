<script lang="ts">
  import * as NativeSelect from '$lib/components/ui/native-select'
  import { sourceState } from '$lib/source-state.svelte'
  let { id, value = $bindable('highest') }: { id: string; value: string } = $props()
  const qualities = [{ value:'highest', label:'最高可用' }, { value:'flac', label:'无损 FLAC' }, { value:'320k', label:'320K' }, { value:'128k', label:'128K' }]
  let active = $derived(sourceState.entries.find(entry => entry.active))
  let available = $derived(qualities.filter(quality => quality.value === 'highest' || !active || active.qualities.includes(quality.value)))
  let unavailable = $derived(!available.some(quality => quality.value === value))
</script>

<NativeSelect.Root {id} bind:value aria-describedby={`${id}-description`} class="w-full">
  {#if unavailable}<NativeSelect.Option {value} disabled>{qualities.find(quality => quality.value === value)?.label ?? value}（当前音源不支持）</NativeSelect.Option>{/if}
  {#each available as quality}<NativeSelect.Option value={quality.value}>{quality.label}</NativeSelect.Option>{/each}
</NativeSelect.Root>
