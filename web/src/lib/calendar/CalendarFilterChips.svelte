<script lang="ts">
  // Quick-filter chips for the calendar — Stream R polish. Drives the
  // same `hidden` Set<EventFilterKey> as the sidebar Filters section,
  // surfaced as an always-visible pill row so a user can toggle
  // Events / ICS / Tasks / Deadlines without opening the sidebar.
  //
  // Active chip = tinted fill in the type's tone (Tasks = primary,
  // ICS = info, Deadlines = error, etc) so the user reads both the
  // state AND the type at a glance. Counts show how many of each
  // type are in the loaded window so empty buckets are obvious and
  // the user doesn't waste a tap toggling something with nothing to
  // show.
  //
  // Mobile: horizontal scroll, no wrap.

  // Single source of truth — calendarFilterState owns EventFilterKey +
  // ChipDef. Importing them (instead of re-declaring a copy that drifted
  // out of sync, e.g. missing 'content_event') keeps the chip row's
  // types aligned with the filter state that drives it.
  import type { EventFilterKey, ChipDef } from './calendarFilterState.svelte';

  let {
    chips,
    hidden,
    typeCounts,
    onToggle,
    onClearAll
  }: {
    chips: ReadonlyArray<ChipDef>;
    hidden: Set<EventFilterKey>;
    typeCounts: Record<string, number>;
    onToggle: (k: EventFilterKey) => void;
    onClearAll: () => void;
  } = $props();

  // "All" is the cleared-state pill — primary tint when nothing is
  // hidden, since that's the canonical "show everything" mode.
  let isAllActive = $derived(hidden.size === 0);
</script>

<div
  class="flex items-center gap-1.5 px-2 sm:px-3 py-1.5 border-b border-surface1 overflow-x-auto flex-shrink-0 bg-mantle scrollbar-thin"
  role="toolbar"
  aria-label="Quick filters"
>
  <!-- All — clears every type filter. Matches the Tasks page chip
       pattern: when nothing is hidden, the All pill is the loud one. -->
  <button
    type="button"
    onclick={onClearAll}
    aria-pressed={isAllActive}
    title="Show every event type"
    class="px-2.5 py-1 rounded text-xs font-medium whitespace-nowrap border {isAllActive ? 'bg-primary text-on-primary border-primary' : 'bg-surface0 text-subtext border-surface1 hover:border-primary hover:text-text'}"
  >All</button>

  {#each chips as f (f.key)}
    {@const visible = !hidden.has(f.key)}
    {@const count = typeCounts[f.key] ?? 0}
    <button
      type="button"
      onclick={() => onToggle(f.key)}
      aria-pressed={visible}
      title="{visible ? 'Hide' : 'Show'} {f.label} ({count} in window)"
      class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded text-xs font-medium whitespace-nowrap border transition-colors {count === 0 && visible ? 'opacity-50' : ''}"
      style:background={visible ? `var(--color-${f.tone})` : undefined}
      style:color={visible ? 'var(--color-mantle)' : undefined}
      style:border-color={visible ? `var(--color-${f.tone})` : undefined}
      class:bg-surface0={!visible}
      class:border-surface1={!visible}
    >
      <span
        class="w-1.5 h-1.5 rounded-full flex-shrink-0"
        style:background={visible ? 'var(--color-mantle)' : `var(--color-${f.tone})`}
        aria-hidden="true"
      ></span>
      <span style:color={visible ? undefined : `var(--color-${f.tone})`}>{f.label}</span>
      {#if count > 0}
        <span
          class="font-mono tabular-nums text-[10px]"
          style:color={visible ? 'color-mix(in srgb, var(--color-mantle) 80%, transparent)' : `color-mix(in srgb, var(--color-${f.tone}) 80%, transparent)`}
        >{count}</span>
      {/if}
    </button>
  {/each}

  {#if hidden.size > 0}
    <button
      type="button"
      onclick={onClearAll}
      class="ml-1 px-1.5 py-0.5 text-[10px] text-warning hover:text-error whitespace-nowrap"
      title="Show all hidden types"
    >clear ({hidden.size})</button>
  {/if}
</div>

<style>
  .scrollbar-thin::-webkit-scrollbar { height: 0; }
  .scrollbar-thin { scrollbar-width: none; }
</style>
