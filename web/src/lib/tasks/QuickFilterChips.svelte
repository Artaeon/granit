<script lang="ts">
  import Chip from '$lib/components/Chip.svelte';
  // Quick-filter chips — always-visible smart filters that compose with
  // the slide-out filter panel. Stream N. Replaces the previous always-
  // scrolling smartCounts row with a deliberate 6-chip set the user
  // can scan in one glance:
  //   All · Today · Overdue · P1 · No date · Done
  // Active chip uses primary background (or status-tinted bg for
  // overdue/today) so the user sees the active focus at a glance.
  // Mobile: horizontal scroll, no wrap.
  type SmartFilter =
    | ''
    | 'overdue'
    | 'today'
    | 'tomorrow'
    | 'thisWeek'
    | 'noDue'
    | 'noPriority'
    | 'highPriority'
    | 'hasSubtasks'
    | 'hasEstimate'
    | 'noEstimate';

  type Props = {
    smartFilter: SmartFilter;
    status: 'open' | 'done' | 'all';
    counts: { overdue: number; today: number; noDue: number; highPriority: number };
    doneCount: number;
    activeFilterCount: number;
    onSetSmart: (s: SmartFilter) => void;
    onSetStatus: (s: 'open' | 'done' | 'all') => void;
    onClearAll: () => void;
  };

  let {
    smartFilter,
    status,
    counts,
    doneCount,
    activeFilterCount,
    onSetSmart,
    onSetStatus,
    onClearAll
  }: Props = $props();

  // "All" is the cleared-state pill — it ALSO clears any in-panel
  // filters so the chip-row labels its own behaviour honestly. A chip
  // labelled "All" that leaves a P1 filter on would be a lie.
  let isAllActive = $derived(
    !smartFilter && status === 'open' && activeFilterCount === 0
  );
  let isDoneActive = $derived(status === 'done' || status === 'all');
</script>

<div
  class="flex items-center gap-1.5 px-3 py-2 border-b border-surface1 overflow-x-auto flex-shrink-0 bg-mantle scrollbar-thin"
  role="toolbar"
  aria-label="Quick filters"
>
  <!-- All — clears every smart filter + opens-only status + active
       sidebar filters. The visual primary state is "everything
       neutral". -->
  <Chip tone="neutral" active={isAllActive} onclick={onClearAll} title="Show every open task (clear all filters)">All</Chip>

  <!-- Today — due/scheduled today. Amber tone. -->
  <Chip
    tone="warning"
    active={smartFilter === 'today'}
    onclick={() => onSetSmart(smartFilter === 'today' ? '' : 'today')}
    title="Due or scheduled today"
  >
    <span class="w-1.5 h-1.5 rounded-full {smartFilter === 'today' ? 'bg-mantle' : 'bg-warning'}" aria-hidden="true"></span>
    Today
    {#if counts.today > 0}
      <span class="font-mono tabular-nums text-[10px] {smartFilter === 'today' ? 'text-mantle/80' : 'text-warning/80'}">{counts.today}</span>
    {/if}
  </Chip>

  <!-- Overdue — past due. Red — the single loudest chip. -->
  <Chip
    tone="error"
    active={smartFilter === 'overdue'}
    onclick={() => onSetSmart(smartFilter === 'overdue' ? '' : 'overdue')}
    title="Tasks past their due date"
    disabled={counts.overdue === 0 && smartFilter !== 'overdue'}
  >
    <span class="w-1.5 h-1.5 rounded-full {smartFilter === 'overdue' ? 'bg-mantle' : 'bg-error'}" aria-hidden="true"></span>
    Overdue
    {#if counts.overdue > 0}
      <span class="font-mono tabular-nums text-[10px] font-semibold {smartFilter === 'overdue' ? 'text-mantle/90' : 'text-error'}">{counts.overdue}</span>
    {/if}
  </Chip>

  <!-- P1 — highest priority. Red tone (matches the P1 colour app-wide). -->
  <Chip
    tone="error"
    active={smartFilter === 'highPriority'}
    onclick={() => onSetSmart(smartFilter === 'highPriority' ? '' : 'highPriority')}
    title="P1 (highest priority) tasks"
  >
    <span class="font-mono text-[10px] {smartFilter === 'highPriority' ? 'text-mantle' : 'text-error'}">!1</span>
    P1
    {#if counts.highPriority > 0}
      <span class="font-mono tabular-nums text-[10px] {smartFilter === 'highPriority' ? 'text-mantle/80' : 'text-error/80'}">{counts.highPriority}</span>
    {/if}
  </Chip>

  <!-- No date — no due / no scheduled. Blue tone — encourages triage. -->
  <Chip
    tone="info"
    active={smartFilter === 'noDue'}
    onclick={() => onSetSmart(smartFilter === 'noDue' ? '' : 'noDue')}
    title="No due date and no scheduled time — needs a decision"
  >
    No date
    {#if counts.noDue > 0}
      <span class="font-mono tabular-nums text-[10px] ml-1 {smartFilter === 'noDue' ? 'text-mantle/80' : 'text-info/80'}">{counts.noDue}</span>
    {/if}
  </Chip>

  <!-- Done — flips status to done. Green tone. -->
  <Chip
    tone="success"
    active={isDoneActive}
    onclick={() => onSetStatus(isDoneActive ? 'open' : 'done')}
    title="Show completed tasks"
  >
    <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
      <path d="M5 12l5 5 9-11" />
    </svg>
    Done
    {#if doneCount > 0 && !isDoneActive}
      <span class="font-mono tabular-nums text-[10px] text-success/80">{doneCount}</span>
    {/if}
  </Chip>
</div>

<style>
  .scrollbar-thin::-webkit-scrollbar { height: 0; }
  .scrollbar-thin { scrollbar-width: none; }
</style>
