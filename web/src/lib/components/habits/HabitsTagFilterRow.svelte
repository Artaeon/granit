<!--
  HabitsTagFilterRow — chip row that narrows the habits list by tag.
  Reads/writes a TagFilterController; renders one chip per known tag
  plus an "All" reset on the left.

  OR semantics: any selected tag matches. Hidden entirely when no
  habit has any tags so the header doesn't waste a row on empty.
-->
<script lang="ts">
  import type { TagFilterController } from '$lib/habits/habitsTagFilter.svelte';

  type Props = { ctl: TagFilterController };
  let { ctl }: Props = $props();
</script>

{#if ctl.knownTags.length > 0}
  <div class="flex items-center gap-1.5 overflow-x-auto -mx-1 px-1 pb-1 mb-3">
    <button
      type="button"
      onclick={() => ctl.clear()}
      class="flex-shrink-0 px-2 py-0.5 rounded-full text-[10px] uppercase tracking-wider border transition-colors
        {ctl.isActive
          ? 'bg-surface1 text-dim border-surface2 hover:text-text'
          : 'bg-secondary text-on-secondary border-secondary'}"
      title="show every tag"
    >All</button>
    {#each ctl.knownTags as t (t)}
      {@const on = ctl.selected.has(t)}
      <button
        type="button"
        onclick={() => ctl.toggle(t)}
        class="flex-shrink-0 px-2 py-0.5 rounded-full text-[10px] uppercase tracking-wider border transition-colors
          {on
            ? 'bg-secondary text-on-secondary border-secondary'
            : 'bg-surface0 text-subtext border-surface2 hover:bg-surface1'}"
      >#{t}</button>
    {/each}
  </div>
{/if}
