<!--
  TasksActiveFilterChips — the row of x-removable chips under the
  presets bar that surfaces every non-default filter. Lets the user
  SEE what's filtering the visible list and dismiss any single one in
  one click — no need to open the filter drawer (mobile) or hunt the
  sidebar (desktop).

  Hidden when no filters are active. The "Clear all" pill appears
  once at least two filters are active, so a single accidental chip
  doesn't get a redundant reset CTA next to its own × button.
-->
<script lang="ts">
  import type { FilterChip } from './tasksFilterState.svelte';

  type Props = {
    chips: FilterChip[];
    filteredCount: number;
    onClearAll: () => void;
  };

  let { chips, filteredCount, onClearAll }: Props = $props();
</script>

{#if chips.length > 0}
  <div class="px-3 py-1.5 border-b border-surface1 flex items-center gap-1 text-[11px] flex-shrink-0 flex-wrap bg-surface0/40">
    <span class="text-[10px] uppercase tracking-wider text-dim mr-1 select-none">Filters</span>
    {#each chips as chip (chip.key)}
      <span class="inline-flex items-center gap-1 px-1.5 py-0.5 bg-surface0 border border-surface1 font-mono tabular-nums {chip.tone ?? 'text-subtext'}">
        <span class="select-none">{chip.label}</span>
        <button
          type="button"
          onclick={chip.clear}
          aria-label="clear {chip.key} filter"
          title="Remove this filter"
          class="text-dim hover:text-error leading-none px-1 -mx-1"
        >×</button>
      </span>
    {/each}
    {#if chips.length >= 2}
      <button
        type="button"
        onclick={onClearAll}
        title="Reset every active filter to its default"
        class="ml-1 px-1.5 py-0.5 text-[10px] uppercase tracking-wider text-warning hover:text-error border border-dashed border-warning hover:border-error"
      >clear all</button>
    {/if}
    <span class="flex-1"></span>
    <span class="text-[10px] text-dim font-mono tabular-nums select-none">{filteredCount} match{filteredCount === 1 ? '' : 'es'}</span>
  </div>
{/if}
