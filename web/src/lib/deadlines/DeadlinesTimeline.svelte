<script lang="ts">
  // Timeline view for /deadlines. Stream BB. Vertical rail of
  // deadlines sorted earliest-first, with month headers breaking the
  // visual rhythm so the eye can pick out clusters. Dot tone driven
  // by urgency (not importance) — the rail communicates "when";
  // importance lives on the row's pill.
  import type { Deadline } from '$lib/api';
  import DeadlinePill from '$lib/deadlines/DeadlinePill.svelte';
  import { daysUntil } from '$lib/deadlines/util';

  type Props = {
    rows: Deadline[];
    countdown: (d: Deadline) => string;
    monthLabel: (key: string) => string;
    monthBucket: (d: Deadline) => string;
    goalTitle: (id?: string) => string;
    onOpen: (d: Deadline) => void;
  };

  let { rows, countdown, monthLabel, monthBucket, goalTitle, onOpen }: Props = $props();

  // First-of-month markers — the first row in each month gets a
  // chunky uppercase header so months read as visual sections.
  let monthHeaders = $derived.by(() => {
    const seen = new Set<string>();
    const out = new Set<string>();
    for (const d of rows) {
      const k = monthBucket(d);
      if (!seen.has(k)) { seen.add(k); out.add(d.id); }
    }
    return out;
  });
</script>

{#if rows.length === 0}
  <div class="text-sm text-dim italic">No deadlines match your filters.</div>
{:else}
  <ol class="relative ml-4 border-l border-surface2 space-y-2 pl-5">
    {#each rows as d (d.id)}
      {@const days = daysUntil(d.date)}
      {@const isMet = d.status === 'met'}
      {@const isCancelled = d.status === 'cancelled'}
      {@const isDone = isMet || isCancelled}
      {@const showMonth = monthHeaders.has(d.id)}
      {@const dotTone = isDone
        ? (isMet ? 'success' : 'dim')
        : days < 0 ? 'error'
        : days <= 3 ? 'error'
        : days <= 7 ? 'warning'
        : days <= 14 ? 'info'
        : 'secondary'}
      {#if showMonth}
        <li class="ml-[-1.25rem] pt-3 first:pt-0 text-[11px] uppercase tracking-wider text-dim font-medium">
          {monthLabel(monthBucket(d))}
        </li>
      {/if}
      <li class="relative">
        <span
          class="absolute -left-[1.55rem] top-3 w-3 h-3 rounded-full ring-2 ring-base"
          style="background: var(--color-{dotTone});"
          aria-hidden="true"
        ></span>
        <div
          role="button"
          tabindex="0"
          onclick={() => onOpen(d)}
          onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onOpen(d); } }}
          class="bg-surface0 border border-surface1 hover:border-primary rounded-lg p-3 transition-colors cursor-pointer {isDone ? 'opacity-60' : ''}"
        >
          <div class="flex items-baseline gap-2">
            <span class="font-mono text-xs text-subtext tabular-nums w-20 flex-shrink-0">{d.date}</span>
            <span class="text-sm font-medium text-text flex-1 min-w-0 truncate {isMet ? 'line-through' : ''}">{d.title}</span>
            <DeadlinePill variant="importance" importance={d.importance} />
          </div>
          <div class="flex flex-wrap items-center gap-x-3 gap-y-0.5 mt-1 text-xs text-dim">
            <span style="color: var(--color-{dotTone});">· {countdown(d)}</span>
            {#if d.venture}<span class="text-secondary">🏢 {d.venture}</span>{/if}
            {#if d.goal_id}<span class="text-secondary">🎯 {goalTitle(d.goal_id)}</span>{/if}
            {#if d.project}<span class="text-secondary">📁 {d.project}</span>{/if}
          </div>
        </div>
      </li>
    {/each}
  </ol>
{/if}
