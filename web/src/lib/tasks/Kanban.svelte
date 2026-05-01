<script lang="ts">
  import { onMount } from 'svelte';
  import type { Task } from '$lib/api';
  import TaskCard from './TaskCard.svelte';

  let { tasks, mode = $bindable('priority'), onChanged }: { tasks: Task[]; mode?: 'priority' | 'due' | 'triage'; onChanged?: () => void } = $props();

  type Column = { key: string; label: string; tasks: Task[]; tone?: string };

  let columns = $derived.by((): Column[] => {
    if (mode === 'priority') {
      const buckets: Record<string, Task[]> = { '1': [], '2': [], '3': [], '0': [], done: [] };
      for (const t of tasks) {
        if (t.done) buckets.done.push(t);
        else buckets[String(t.priority)].push(t);
      }
      return [
        { key: '1', label: 'P1 — high', tasks: buckets['1'], tone: 'text-error' },
        { key: '2', label: 'P2 — medium', tasks: buckets['2'], tone: 'text-warning' },
        { key: '3', label: 'P3 — low', tasks: buckets['3'], tone: 'text-info' },
        { key: '0', label: 'no priority', tasks: buckets['0'], tone: 'text-subtext' },
        { key: 'done', label: 'done', tasks: buckets.done, tone: 'text-success' }
      ];
    }
    if (mode === 'due') {
      const today = new Date().toISOString().slice(0, 10);
      const buckets: Record<string, Task[]> = { overdue: [], today: [], upcoming: [], no_date: [], done: [] };
      for (const t of tasks) {
        if (t.done) buckets.done.push(t);
        else if (!t.dueDate && !t.scheduledStart) buckets.no_date.push(t);
        else {
          const d = t.dueDate ?? (t.scheduledStart ? t.scheduledStart.slice(0, 10) : '');
          if (d < today) buckets.overdue.push(t);
          else if (d === today) buckets.today.push(t);
          else buckets.upcoming.push(t);
        }
      }
      return [
        { key: 'overdue', label: 'overdue', tasks: buckets.overdue, tone: 'text-error' },
        { key: 'today', label: 'today', tasks: buckets.today, tone: 'text-warning' },
        { key: 'upcoming', label: 'upcoming', tasks: buckets.upcoming, tone: 'text-secondary' },
        { key: 'no_date', label: 'no date', tasks: buckets.no_date, tone: 'text-subtext' },
        { key: 'done', label: 'done', tasks: buckets.done, tone: 'text-success' }
      ];
    }
    const buckets: Record<string, Task[]> = { inbox: [], triaged: [], scheduled: [], done: [], dropped: [], snoozed: [] };
    for (const t of tasks) {
      const k = t.triage || (t.done ? 'done' : 'inbox');
      (buckets[k] ??= []).push(t);
    }
    return [
      { key: 'inbox', label: 'inbox', tasks: buckets.inbox, tone: 'text-warning' },
      { key: 'triaged', label: 'triaged', tasks: buckets.triaged, tone: 'text-secondary' },
      { key: 'scheduled', label: 'scheduled', tasks: buckets.scheduled, tone: 'text-info' },
      { key: 'snoozed', label: 'snoozed', tasks: buckets.snoozed, tone: 'text-dim' },
      { key: 'done', label: 'done', tasks: buckets.done, tone: 'text-success' },
      { key: 'dropped', label: 'dropped', tasks: buckets.dropped, tone: 'text-dim' }
    ].filter((c) => c.tasks.length > 0 || c.key !== 'dropped');
  });

  // Mobile: track collapsed state per column. Empty columns auto-collapse.
  let isDesktop = $state(false);
  let collapsed = $state<Record<string, boolean>>({});
  onMount(() => {
    const mq = window.matchMedia('(min-width: 768px)');
    isDesktop = mq.matches;
    const handler = (e: MediaQueryListEvent) => (isDesktop = e.matches);
    mq.addEventListener('change', handler);
    return () => mq.removeEventListener('change', handler);
  });

  // Auto-collapse empty + done columns on mobile by default.
  $effect(() => {
    if (!isDesktop && Object.keys(collapsed).length === 0) {
      const next: Record<string, boolean> = {};
      for (const c of columns) {
        if (c.tasks.length === 0 || c.key === 'done' || c.key === 'dropped' || c.key === 'snoozed') {
          next[c.key] = true;
        }
      }
      collapsed = next;
    }
  });

  function toggle(key: string) {
    if (isDesktop) return;
    collapsed = { ...collapsed, [key]: !collapsed[key] };
  }
</script>

<div class="flex flex-col md:flex-row gap-3 md:overflow-x-auto md:pb-3" style="min-height: 60vh">
  {#each columns as col (col.key)}
    {@const isCollapsed = !isDesktop && !!collapsed[col.key]}
    <div class="bg-mantle/50 border border-surface1 rounded flex flex-col md:flex-shrink-0 md:w-72">
      <button
        type="button"
        onclick={() => toggle(col.key)}
        class="flex items-baseline justify-between gap-2 p-3 md:p-2 md:cursor-default text-left"
      >
        <h3 class="text-xs uppercase tracking-wider font-medium {col.tone ?? 'text-dim'}">{col.label}</h3>
        <span class="text-xs text-dim flex items-center gap-2">
          <span>{col.tasks.length}</span>
          <span class="md:hidden text-base text-subtext leading-none">{isCollapsed ? '▸' : '▾'}</span>
        </span>
      </button>

      {#if !isCollapsed}
        <div class="px-2 pb-2 md:px-2 space-y-2 md:overflow-y-auto md:flex-1">
          {#if col.tasks.length === 0}
            <div class="text-xs text-dim italic px-1 pb-2">empty</div>
          {:else}
            {#each col.tasks as t (t.id)}
              <TaskCard task={t} compact onChanged={() => onChanged?.()} />
            {/each}
          {/if}
        </div>
      {/if}
    </div>
  {/each}
</div>
