<script lang="ts">
 import * as Field from '$lib/components/ui/field'
 import * as NativeSelect from '$lib/components/ui/native-select'
 import { Button } from '$lib/components/ui/button'
 import SettingRow from '$lib/components/settings/SettingRow.svelte'
 import { appearanceState as view,appearanceController as controller } from '$lib/appearance-state.svelte'
</script>

<h2 class="mb-2 text-xl font-semibold">外观</h2>
<Field.Group class="gap-0">
 <SettingRow id="appearance-mode" label="主题" description="跟随系统时，随系统外观自动切换。" disabled={!view.loaded}>
  {#snippet control()}
   <NativeSelect.Root id="appearance-mode" class="w-full" value={view.mode} disabled={!view.loaded} aria-describedby="appearance-mode-description" onchange={event=>controller.select(event.currentTarget.value)}>
    <NativeSelect.Option value="dark">深色</NativeSelect.Option>
    <NativeSelect.Option value="light">浅色</NativeSelect.Option>
    <NativeSelect.Option value="system">跟随系统</NativeSelect.Option>
   </NativeSelect.Root>
  {/snippet}
 </SettingRow>
</Field.Group>
{#if view.loading}<span class="sr-only" role="status">正在读取外观设置</span>{:else if view.status==='saving'}<span class="sr-only" role="status">正在保存外观设置</span>{/if}
{#if view.error}<div class="flex items-center gap-3"><p class="text-sm text-destructive" role="alert">{view.error}</p><Button variant="outline" size="sm" onclick={()=>controller.retry()}>{view.loaded?'重试保存外观':'重试读取外观'}</Button></div>{/if}
