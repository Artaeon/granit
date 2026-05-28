<script lang="ts">
  // Quick-filter status chips for /goals. Replaces the old segmented
  // pill bar (All / Active / Paused / Completed / Archived). Each chip
  // is tone-tinted so the user can read the status at a glance:
  //   All       → primary (or neutral when inactive)
  //   Active    → primary
  //   Paused    → warning
  //   Completed → success
  //   Archived  → dim
  // Mobile: horizontal scroll, no wrap.
  type StatusFilter = 'all' | 'active' | 'paused' | 'completed' | 'archived';

  type Props = {
    status: StatusFilter;
    counts: { all: number; active: number; paused: number; completed: number; archived: number };
    onSet: (s: StatusFilter) => void;
  };

  let { status, counts, onSet }: Props = $props();

  // Per-chip visual signature. Class strings are spelled out per
  // tone so Tailwind v4's source scanner picks them up — computed
  // `bg-${tone}` strings would be invisible to the scanner.
  type Chip = {
    key: StatusFilter;
    label: string;
    title: string;
    activeClass: string;   // when this chip is the active filter
    inactiveClass: string; // when another chip is active
  };
  const CHIPS: Chip[] = [
    {
      key: 'all',
      label: 'All',
      title: 'Every goal across all statuses',
      activeClass: 'bg-primary text-on-primary border-primary',
      inactiveClass: 'bg-surface0 text-subtext border-surface1 hover:border-primary hover:text-text'
    },
    {
      key: 'active',
      label: 'Active',
      title: "Goals you're currently moving on",
      activeClass: 'bg-primary text-on-primary border-primary',
      inactiveClass: 'bg-surface0 text-primary border-surface1 hover:border-primary'
    },
    {
      key: 'paused',
      label: 'Paused',
      title: "Goals you've deliberately set aside",
      activeClass: 'bg-warning text-mantle border-warning',
      inactiveClass: 'bg-surface0 text-warning border-surface1 hover:border-warning'
    },
    {
      key: 'completed',
      label: 'Completed',
      title: "Goals you've closed out",
      activeClass: 'bg-success text-mantle border-success',
      inactiveClass: 'bg-surface0 text-success border-surface1 hover:border-success'
    },
    {
      key: 'archived',
      label: 'Archived',
      title: 'Retired goals kept for history',
      activeClass: 'bg-surface2 text-text border-surface2',
      inactiveClass: 'bg-surface0 text-dim border-surface1 hover:border-surface2 hover:text-subtext'
    }
  ];
</script>

<div
  class="flex items-center gap-1.5 overflow-x-auto flex-shrink-0 scrollbar-thin"
  role="toolbar"
  aria-label="Status filter"
>
  {#each CHIPS as c (c.key)}
    {@const n = counts[c.key]}
    <button
      type="button"
      onclick={() => onSet(c.key)}
      aria-pressed={status === c.key}
      title={c.title}
      class="px-2.5 py-1 rounded text-xs font-medium whitespace-nowrap border inline-flex items-center gap-1.5 {status === c.key ? c.activeClass : c.inactiveClass}"
    >
      <span>{c.label}</span>
      {#if n > 0}
        <span class="font-mono tabular-nums text-[10px] opacity-80">{n}</span>
      {/if}
    </button>
  {/each}
</div>

<style>
  .scrollbar-thin::-webkit-scrollbar { height: 0; }
  .scrollbar-thin { scrollbar-width: none; }
</style>
