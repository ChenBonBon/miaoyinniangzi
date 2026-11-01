<script lang="ts">
  import { FolderOpen } from '@lucide/svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Separator } from '$lib/components/ui/separator'
  import * as Field from '$lib/components/ui/field'
  import SettingRow from '$lib/components/settings/SettingRow.svelte'
  import QualitySelect from '$lib/components/settings/QualitySelect.svelte'
  import { settingsState, selectDownloadDirectory } from '$lib/settings-state.svelte'
</script>

<h2 class="mb-2 text-xl font-semibold">下载</h2>
{#if settingsState.draft}
  <fieldset disabled={settingsState.choosing} class="min-w-0">
    <Field.Group class="gap-0">
      <SettingRow id="download-directory" label="下载目录" description="仅在此目录核对已有歌曲；切换目录不会移动已下载的文件。">
        {#snippet control()}
          <Input id="download-directory" value={settingsState.draft!.downloadDirectory} title={settingsState.draft!.downloadDirectory} readonly aria-describedby="download-directory-description" class="min-w-0 flex-1 truncate" />
          <Button variant="outline" size="icon" onclick={selectDownloadDirectory} aria-label={settingsState.choosing ? '选择中…' : '选择目录'} title="选择目录"><FolderOpen /></Button>
        {/snippet}
      </SettingRow>
      <Separator />
      <SettingRow id="download-quality" label="下载音质" description="下载功能接通后生效。">
        {#snippet control()}<QualitySelect id="download-quality" bind:value={settingsState.draft!.downloadQuality} />{/snippet}
      </SettingRow>
    </Field.Group>
  </fieldset>
{/if}
