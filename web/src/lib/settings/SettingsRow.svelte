<script lang="ts">
  // SettingsRow — single consistent row layout used inside every tab.
  // Left: label (+ optional helper text below).
  // Right: control snippet (the input/select/toggle/etc).
  // This keeps the visual grid identical across rows so the eye can
  // scan a tab without re-aligning on each section.
  import type { Snippet } from 'svelte';

  let {
    label,
    help,
    control,
    align = 'baseline'
  }: {
    label: string;
    help?: string;
    control: Snippet;
    align?: 'baseline' | 'start';
  } = $props();
</script>

<div
  class="flex flex-col sm:flex-row gap-1.5 sm:gap-3 sm:items-{align === 'start' ? 'start' : 'baseline'} py-2 border-b border-surface1 last:border-b-0"
>
  <div class="flex-1 min-w-0">
    <div class="text-sm text-text">{label}</div>
    {#if help}
      <div class="text-[11px] text-dim mt-0.5 leading-snug">{help}</div>
    {/if}
  </div>
  <div class="sm:flex-shrink-0 sm:max-w-[55%] flex items-center gap-1.5 flex-wrap">
    {@render control()}
  </div>
</div>
