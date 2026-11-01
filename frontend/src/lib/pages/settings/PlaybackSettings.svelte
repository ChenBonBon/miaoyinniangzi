<script lang="ts">
  import * as Field from '$lib/components/ui/field'
  import { Slider } from '$lib/components/ui/slider'
  import { Separator } from '$lib/components/ui/separator'
  import SettingRow from '$lib/components/settings/SettingRow.svelte'
  import QualitySelect from '$lib/components/settings/QualitySelect.svelte'
  import { settingsState } from '$lib/settings-state.svelte'
</script>

<h2 class="mb-2 text-xl font-semibold">播放</h2>
{#if settingsState.draft}
  <Field.Group class="gap-0">
    <SettingRow id="playback-quality" label="播放音质" description="优先无损，不可用时逐级降级并提示。播放功能接通后生效。">
      {#snippet control()}<QualitySelect id="playback-quality" bind:value={settingsState.draft!.playbackQuality} />{/snippet}
    </SettingRow>
    <Separator />
    <SettingRow id="preferred-volume" label="默认音量" description="播放功能接入后使用此音量。">
      {#snippet control()}
        <Slider id="preferred-volume" type="single" min={0} max={1} step={0.01} bind:value={settingsState.draft!.volume} aria-label="默认音量" aria-describedby="preferred-volume-description" class="min-w-0 flex-1" />
        <span class="w-10 shrink-0 text-right text-sm text-muted-foreground tabular-nums">{Math.round(settingsState.draft!.volume * 100)}%</span>
      {/snippet}
    </SettingRow>
  </Field.Group>
{/if}
