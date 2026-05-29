<script lang="ts">
  // Quick-filter chip row for /jots. Stream AA extraction.
  //
  // Two flavours of chip stacked on one wrapping row:
  //   · open tasks / · 7d / · 30d  — orthogonal smart filters
  //   #tag chips                   — AND-combined tag set
  //
  // Visual cue: smart filters lead with a "· " glyph so they read
  // distinctly from tag chips at a glance. The "clear" pill is only
  // rendered when at least one filter is active and shows the
  // visible/total ratio so the user knows what their filters dropped.
  type Timeframe = 'all' | '7d' | '30d';

  type Props = {
    activeTags: string[];
    allTags: string[];
    filterOpenTasks: boolean;
    filterTimeframe: Timeframe;
    hasAnyFilter: boolean;
    visibleCount: number;
    totalCount: number;
    onToggleOpenTasks: () => void;
    onSetTimeframe: (tf: Timeframe) => void;
    onToggleTag: (t: string) => void;
    onClearAll: () => void;
  };

  let {
    activeTags,
    allTags,
    filterOpenTasks,
    filterTimeframe,
    hasAnyFilter,
    visibleCount,
    totalCount,
    onToggleOpenTasks,
    onSetTimeframe,
    onToggleTag,
    onClearAll
  }: Props = $props();

  // Cap the inline tag set to 24 chips; anything beyond gets summarised
  // as "+N more" so the row doesn't blow the page width on tag-heavy
  // accounts. The user can still filter by typing in search.
  const TAG_LIMIT = 24;
</script>

{#if allTags.length > 0 || totalCount > 0}
  <div class="flex flex-wrap items-center gap-1 mt-1.5 text-[11px]">
    {#if hasAnyFilter}
      <button
        type="button"
        onclick={onClearAll}
        class="px-1.5 py-0.5 rounded bg-surface1 text-text hover:bg-surface2"
        title="clear every active filter"
      >clear ({visibleCount}/{totalCount})</button>
    {/if}
    <button
      type="button"
      onclick={onToggleOpenTasks}
      class="px-1.5 py-0.5 rounded {filterOpenTasks ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
      title="only jots whose daily has open tasks"
    >· open tasks</button>
    <button
      type="button"
      onclick={() => onSetTimeframe(filterTimeframe === '7d' ? 'all' : '7d')}
      class="px-1.5 py-0.5 rounded {filterTimeframe === '7d' ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
      title="last 7 days only"
    >· 7d</button>
    <button
      type="button"
      onclick={() => onSetTimeframe(filterTimeframe === '30d' ? 'all' : '30d')}
      class="px-1.5 py-0.5 rounded {filterTimeframe === '30d' ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
      title="last 30 days only"
    >· 30d</button>
    {#if allTags.length > 0}
      <span class="text-dim opacity-50 mx-0.5">|</span>
    {/if}
    {#each allTags.slice(0, TAG_LIMIT) as t}
      <button
        type="button"
        onclick={() => onToggleTag(t)}
        class="px-1.5 py-0.5 rounded {activeTags.includes(t) ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
      >#{t}</button>
    {/each}
    {#if allTags.length > TAG_LIMIT}
      <span class="text-dim">+{allTags.length - TAG_LIMIT} more</span>
    {/if}
  </div>
{/if}
