<!--
  HabitsCategoryFilterRow — chip row that narrows the habits list by
  category. Reads/writes a CategoryFilterController; renders one chip
  per known category, an "Uncategorized" chip when at least one
  habit has no category, and an "All" reset on the left.

  Horizontally scrollable on mobile so a dozen categories never blow
  out the header. POWERUI defaults: small chips, hover feedback, no
  modal — every change is a single click.
-->
<script lang="ts">
  import {
    UNCATEGORIZED,
    type CategoryFilterController
  } from '$lib/habits/habitsCategoryFilter.svelte';

  type Props = { ctl: CategoryFilterController };
  let { ctl }: Props = $props();
</script>

{#if ctl.knownCategories.length > 0 || ctl.hasUncategorized}
  <div class="flex items-center gap-1.5 overflow-x-auto -mx-1 px-1 pb-1 mb-2">
    <button
      type="button"
      onclick={() => ctl.clear()}
      class="flex-shrink-0 px-2 py-0.5 rounded-full text-[11px] uppercase tracking-wider border transition-colors
        {ctl.isActive
          ? 'bg-surface1 text-dim border-surface2 hover:text-text'
          : 'bg-primary text-on-primary border-primary'}"
      title="show every category"
    >All</button>
    {#each ctl.knownCategories as c (c)}
      {@const on = ctl.selected.has(c)}
      <button
        type="button"
        onclick={() => ctl.toggle(c)}
        class="flex-shrink-0 px-2 py-0.5 rounded-full text-[11px] uppercase tracking-wider border transition-colors
          {on
            ? 'bg-primary text-on-primary border-primary'
            : 'bg-surface0 text-subtext border-surface2 hover:bg-surface1'}"
      >{c}</button>
    {/each}
    {#if ctl.hasUncategorized}
      {@const on = ctl.selected.has(UNCATEGORIZED)}
      <button
        type="button"
        onclick={() => ctl.toggle(UNCATEGORIZED)}
        class="flex-shrink-0 px-2 py-0.5 rounded-full text-[11px] uppercase tracking-wider border transition-colors
          {on
            ? 'bg-primary text-on-primary border-primary'
            : 'bg-surface0 text-dim border-surface2 hover:bg-surface1'}"
        title="habits without a category set"
      >Uncategorized</button>
    {/if}
  </div>
{/if}
