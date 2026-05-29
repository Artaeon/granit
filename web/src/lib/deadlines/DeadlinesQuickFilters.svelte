<script lang="ts">
  // Quick-filter chips for /deadlines. Stream BB. Tone-tinted pills
  // for importance (Critical / High / Normal) — replaces the round
  // border-pill row with the standard chip shape used on /tasks and
  // /goals so the page chrome reads as one family.
  //
  //   All       → clears importance filter (and search)
  //   Critical  → error tint
  //   High      → warning tint
  //   Normal    → secondary tint
  //
  // Each chip surfaces the global count for its bucket so the user
  // can see "I have 2 critical, 5 high" without scrolling.
  import type { DeadlineImportance } from '$lib/api';

  type Props = {
    importance: DeadlineImportance | null;
    q: string;
    counts: { critical: number; high: number; normal: number };
    onSet: (v: DeadlineImportance | null) => void;
    onClearAll: () => void;
  };

  let { importance, q, counts, onSet, onClearAll }: Props = $props();

  // "All" is the cleared-state pill — it ALSO clears the search so
  // the chip labels its own behaviour honestly. A chip labelled "All"
  // that left a search term on would be a lie.
  let isAllActive = $derived(importance === null && !q);

  // Per-chip activeClass / inactiveClass strings are spelled out so
  // Tailwind v4's source scanner picks them up — a computed
  // `bg-${tone}` wouldn't make it into the generated CSS.
  type Chip = {
    key: DeadlineImportance;
    label: string;
    keyHint: string;
    title: string;
    dot: string;        // dot colour when inactive
    activeClass: string;
    inactiveClass: string;
    count: number;
  };
  let CHIPS = $derived<Chip[]>([
    {
      key: 'critical',
      label: 'Critical',
      keyHint: '1',
      title: 'Drop-everything deadlines (1)',
      dot: 'bg-error',
      activeClass: 'bg-error text-mantle border-error',
      inactiveClass: 'bg-surface0 text-error border-surface1 hover:border-error',
      count: counts.critical
    },
    {
      key: 'high',
      label: 'High',
      keyHint: '2',
      title: 'High-importance deadlines (2)',
      dot: 'bg-warning',
      activeClass: 'bg-warning text-mantle border-warning',
      inactiveClass: 'bg-surface0 text-warning border-surface1 hover:border-warning',
      count: counts.high
    },
    {
      key: 'normal',
      label: 'Normal',
      keyHint: '3',
      title: 'Normal-importance deadlines (3)',
      dot: 'bg-secondary',
      activeClass: 'bg-secondary text-mantle border-secondary',
      inactiveClass: 'bg-surface0 text-secondary border-surface1 hover:border-secondary',
      count: counts.normal
    }
  ]);
</script>

<div
  class="flex items-center gap-1.5 overflow-x-auto flex-shrink-0 scrollbar-thin"
  role="toolbar"
  aria-label="Importance filter"
>
  <!-- All — clears importance filter + search. Reads as "empty" until
       a more specific chip is active. -->
  <button
    type="button"
    onclick={onClearAll}
    aria-pressed={isAllActive}
    title="Show every deadline (clear filters)"
    class="px-2.5 py-1 rounded text-xs font-medium whitespace-nowrap border {isAllActive ? 'bg-primary text-on-primary border-primary' : 'bg-surface0 text-subtext border-surface1 hover:border-primary hover:text-text'}"
  >All</button>

  {#each CHIPS as c (c.key)}
    {@const active = importance === c.key}
    <button
      type="button"
      onclick={() => onSet(active ? null : c.key)}
      aria-pressed={active}
      title={c.title}
      class="px-2.5 py-1 rounded text-xs font-medium whitespace-nowrap border inline-flex items-center gap-1.5 {active ? c.activeClass : c.inactiveClass}"
    >
      <span class="w-1.5 h-1.5 rounded-full {active ? 'bg-mantle' : c.dot}" aria-hidden="true"></span>
      <span>{c.label}</span>
      {#if c.count > 0}
        <span class="font-mono tabular-nums text-[10px] opacity-80">{c.count}</span>
      {/if}
    </button>
  {/each}
</div>

<style>
  .scrollbar-thin::-webkit-scrollbar { height: 0; }
  .scrollbar-thin { scrollbar-width: none; }
</style>

