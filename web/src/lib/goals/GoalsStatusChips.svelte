<script lang="ts">
  // Quick-filter status chips for /goals (All / Active / Paused /
  // Completed / Archived). Each maps to a semantic Chip tone so the
  // status reads at a glance. Mobile: horizontal scroll, no wrap.
  import Chip from '$lib/components/Chip.svelte';

  type StatusFilter = 'all' | 'active' | 'paused' | 'completed' | 'archived';
  type Tone = 'neutral' | 'warning' | 'success' | 'muted';

  type Props = {
    status: StatusFilter;
    counts: { all: number; active: number; paused: number; completed: number; archived: number };
    onSet: (s: StatusFilter) => void;
  };

  let { status, counts, onSet }: Props = $props();

  const CHIPS: { key: StatusFilter; label: string; title: string; tone: Tone }[] = [
    { key: 'all', label: 'All', title: 'Every goal across all statuses', tone: 'neutral' },
    { key: 'active', label: 'Active', title: "Goals you're currently moving on", tone: 'neutral' },
    { key: 'paused', label: 'Paused', title: "Goals you've deliberately set aside", tone: 'warning' },
    { key: 'completed', label: 'Completed', title: "Goals you've closed out", tone: 'success' },
    { key: 'archived', label: 'Archived', title: 'Retired goals kept for history', tone: 'muted' }
  ];
</script>

<div
  class="flex items-center gap-1.5 overflow-x-auto flex-shrink-0 scrollbar-thin"
  role="toolbar"
  aria-label="Status filter"
>
  {#each CHIPS as c (c.key)}
    {@const n = counts[c.key]}
    <Chip tone={c.tone} active={status === c.key} onclick={() => onSet(c.key)} title={c.title}>
      <span>{c.label}</span>
      {#if n > 0}
        <span class="font-mono tabular-nums text-[10px] opacity-80">{n}</span>
      {/if}
    </Chip>
  {/each}
</div>

<style>
  .scrollbar-thin::-webkit-scrollbar { height: 0; }
  .scrollbar-thin { scrollbar-width: none; }
</style>
