<script lang="ts">
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
  <button
    type="button"
    onclick={onClearAll}
    aria-pressed={isAllActive}
    title="Show every open task (clear all filters)"
    class="px-2.5 py-1 rounded text-xs font-medium whitespace-nowrap border {isAllActive ? 'bg-primary text-on-primary border-primary' : 'bg-surface0 text-subtext border-surface1 hover:border-primary hover:text-text'}"
  >All</button>

  <!-- Today — due/scheduled today. Amber when active so the urgency
       reads off the chip itself. -->
  <button
    type="button"
    onclick={() => onSetSmart(smartFilter === 'today' ? '' : 'today')}
    aria-pressed={smartFilter === 'today'}
    title="Due or scheduled today"
    class="px-2.5 py-1 rounded text-xs font-medium whitespace-nowrap border inline-flex items-center gap-1.5 {smartFilter === 'today' ? 'bg-warning text-mantle border-warning' : 'bg-surface0 text-warning border-surface1 hover:border-warning'}"
  >
    <span class="w-1.5 h-1.5 rounded-full {smartFilter === 'today' ? 'bg-mantle' : 'bg-warning'}" aria-hidden="true"></span>
    Today
    {#if counts.today > 0}
      <span class="font-mono tabular-nums text-[10px] {smartFilter === 'today' ? 'text-mantle/80' : 'text-warning/80'}">{counts.today}</span>
    {/if}
  </button>

  <!-- Overdue — past due. Red. The single loudest chip; user-research
       cliché but it works: red owes you a decision. -->
  <button
    type="button"
    onclick={() => onSetSmart(smartFilter === 'overdue' ? '' : 'overdue')}
    aria-pressed={smartFilter === 'overdue'}
    title="Tasks past their due date"
    class="px-2.5 py-1 rounded text-xs font-medium whitespace-nowrap border inline-flex items-center gap-1.5 {smartFilter === 'overdue' ? 'bg-error text-mantle border-error' : counts.overdue > 0 ? 'bg-surface0 text-error border-error/40 hover:border-error' : 'bg-surface0 text-dim border-surface1'}"
    disabled={counts.overdue === 0 && smartFilter !== 'overdue'}
  >
    <span class="w-1.5 h-1.5 rounded-full {smartFilter === 'overdue' ? 'bg-mantle' : 'bg-error'}" aria-hidden="true"></span>
    Overdue
    {#if counts.overdue > 0}
      <span class="font-mono tabular-nums text-[10px] font-semibold {smartFilter === 'overdue' ? 'text-mantle/90' : 'text-error'}">{counts.overdue}</span>
    {/if}
  </button>

  <!-- P1 — highest priority. Tinted to match the priority colour used
       throughout the app (text-error for P1). -->
  <button
    type="button"
    onclick={() => onSetSmart(smartFilter === 'highPriority' ? '' : 'highPriority')}
    aria-pressed={smartFilter === 'highPriority'}
    title="P1 (highest priority) tasks"
    class="px-2.5 py-1 rounded text-xs font-medium whitespace-nowrap border inline-flex items-center gap-1.5 {smartFilter === 'highPriority' ? 'bg-error text-mantle border-error' : 'bg-surface0 text-error border-surface1 hover:border-error'}"
  >
    <span class="font-mono text-[10px] {smartFilter === 'highPriority' ? 'text-mantle' : 'text-error'}">!1</span>
    P1
    {#if counts.highPriority > 0}
      <span class="font-mono tabular-nums text-[10px] {smartFilter === 'highPriority' ? 'text-mantle/80' : 'text-error/80'}">{counts.highPriority}</span>
    {/if}
  </button>

  <!-- No date — no due / no scheduled. Encourages triage. -->
  <button
    type="button"
    onclick={() => onSetSmart(smartFilter === 'noDue' ? '' : 'noDue')}
    aria-pressed={smartFilter === 'noDue'}
    title="No due date and no scheduled time — needs a decision"
    class="px-2.5 py-1 rounded text-xs font-medium whitespace-nowrap border {smartFilter === 'noDue' ? 'bg-info text-mantle border-info' : 'bg-surface0 text-info border-surface1 hover:border-info'}"
  >
    No date
    {#if counts.noDue > 0}
      <span class="font-mono tabular-nums text-[10px] ml-1 {smartFilter === 'noDue' ? 'text-mantle/80' : 'text-info/80'}">{counts.noDue}</span>
    {/if}
  </button>

  <!-- Done — flips status to done. Distinct from the smart-filter
       chips (which compose with status); clicking this toggles which
       tasks are loaded entirely. -->
  <button
    type="button"
    onclick={() => onSetStatus(isDoneActive ? 'open' : 'done')}
    aria-pressed={isDoneActive}
    title="Show completed tasks"
    class="px-2.5 py-1 rounded text-xs font-medium whitespace-nowrap border inline-flex items-center gap-1.5 {isDoneActive ? 'bg-success text-mantle border-success' : 'bg-surface0 text-success border-surface1 hover:border-success'}"
  >
    <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
      <path d="M5 12l5 5 9-11" />
    </svg>
    Done
    {#if doneCount > 0 && !isDoneActive}
      <span class="font-mono tabular-nums text-[10px] text-success/80">{doneCount}</span>
    {/if}
  </button>
</div>

<style>
  .scrollbar-thin::-webkit-scrollbar { height: 0; }
  .scrollbar-thin { scrollbar-width: none; }
</style>
