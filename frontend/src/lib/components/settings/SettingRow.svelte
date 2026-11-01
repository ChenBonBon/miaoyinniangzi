<script lang="ts">
  import type { Snippet } from 'svelte'
  import * as Field from '$lib/components/ui/field'

  let { id, label, description, control, disabled = false, labelFor = true }: {
    id: string
    label: string
    description: string
    control: Snippet
    disabled?: boolean
    labelFor?: boolean
  } = $props()
</script>

<Field.Field orientation="horizontal" class="setting-row grid gap-4 py-6" data-disabled={disabled || undefined}>
  <Field.Content class="min-w-0 gap-2">
    {#if labelFor}<Field.Label for={id}>{label}</Field.Label>{:else}<Field.Title>{label}</Field.Title>{/if}
    <Field.Description id={`${id}-description`}>{description}</Field.Description>
  </Field.Content>
  <div class="setting-control flex min-w-0 items-center gap-3">{@render control()}</div>
</Field.Field>

<style>
  /* Respond to available settings content width, not the whole window. */
  @container settings-content (min-width: 36rem) {
    :global(.setting-row) { grid-template-columns: minmax(0, 1fr) 16rem; column-gap: 2rem; align-items: center; }
    .setting-control { justify-content: flex-end; }
  }
</style>
