<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, type HabitsResponse } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { habitTargets } from '$lib/habits/targets';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import { toast } from '$lib/components/toast';
  import { createCoalescedReload } from '$lib/util/coalesce';

  // HabitsWidget — at-a-glance daily ticks with an aggregate progress
  // bar at the top + per-habit weekly target chips (when set on the
  // /habits page). One-click "tick all" handles the morning rhythm
  // case where the user wants to mark everything done in one go.

  let data = $state<HabitsResponse | null>(null);
  let loading = $state(false);
  let bulkBusy = $state(false);

  async function load() {
    loading = true;
    try {
      data = await api.listHabits();
    } finally {
      loading = false;
    }
  }
  const reload = createCoalescedReload(load, 600);
  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') reload.trigger();
    });
  });
  onDestroy(() => reload.cancel());

  async function toggle(taskId: string | undefined, done: boolean) {
    if (!taskId) return;
    try {
      await api.patchTask(taskId, { done: !done });
      await load();
    } catch (e) {
      // Surface the failure — a silent fail used to leave the user
      // wondering why their tick didn't stick. Toast names what
      // failed so they can act (network, auth, server).
      toast.error('habit toggle failed: ' + (e instanceof Error ? e.message : String(e)));
      await load();
    }
  }

  // Tick every undone habit at once. Only enabled when at least one
  // habit can be toggled (some require the daily note's `## Habits`
  // section to exist first — those get skipped). Optimistic flip for
  // each, single load() reconciles.
  async function tickAll() {
    if (!data || bulkBusy) return;
    const targets = data.habits.filter((h) => !h.doneToday && h.taskIdToday);
    if (targets.length === 0) return;
    bulkBusy = true;
    const today = data.today;
    for (const h of targets) {
      const habit = data.habits.find((x) => x.name === h.name);
      const day = habit?.days.find((d) => d.date === today);
      if (day) day.done = true;
      if (habit) habit.doneToday = true;
    }
    data = { ...data };
    const failed: string[] = [];
    await Promise.all(
      targets.map(async (h) => {
        try { await api.toggleHabit(h.name, today, true); } catch { failed.push(h.name); }
      })
    );
    bulkBusy = false;
    await load();
    if (failed.length > 0) toast.error(`couldn't tick: ${failed.join(', ')}`);
  }

  // Aggregate progress for the bar at the top. Counts done-today
  // across the visible 6 + the rest. Single number, glanceable.
  let doneCount = $derived(data?.habits.filter((h) => h.doneToday).length ?? 0);
  let totalCount = $derived(data?.habits.length ?? 0);
  let pct = $derived(totalCount === 0 ? 0 : Math.round((doneCount / totalCount) * 100));

  function last7Done(days: { date: string; done: boolean }[]): number {
    return days.slice(-7).filter((d) => d.done).length;
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg shadow-sm p-3">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs text-dim font-semibold">Habits</h2>
    <span class="flex-1"></span>
    {#if data && totalCount > 0}
      <span class="text-[11px] text-dim font-mono tabular-nums">{doneCount}/{totalCount}</span>
    {/if}
    <a href="/habits" class="text-xs text-secondary hover:underline ml-2">all →</a>
  </div>

  {#if loading && !data}
    <div class="space-y-2">
      {#each Array(4) as _, i}
        <div class="flex items-center gap-2">
          <Skeleton class="h-4 w-4 rounded flex-shrink-0" />
          <Skeleton class="h-4 {i % 2 === 0 ? 'w-3/5' : 'w-1/2'}" />
        </div>
      {/each}
    </div>
  {:else if !data || data.habits.length === 0}
    <div class="text-sm text-dim italic leading-relaxed">
      add a <code class="text-xs">## Habits</code> section to your daily note to track streaks here.
    </div>
  {:else}
    <!-- Aggregate progress bar — full bar when all done; rests at 0
         when nothing's ticked. The bar uses success when full,
         primary while in progress so the user reads "near done" vs
         "just starting" at a glance. -->
    <div class="mb-3">
      <div class="h-1.5 bg-surface1 rounded-full overflow-hidden">
        <div
          class="h-full transition-all duration-300 {pct === 100 ? 'bg-success' : 'bg-primary'}"
          style="width: {pct}%"
        ></div>
      </div>
    </div>

    <ul class="space-y-1.5">
      {#each data.habits.slice(0, 6) as h (h.name)}
        {@const target = $habitTargets[h.name]}
        {@const last7 = last7Done(h.days)}
        {@const onTarget = target && last7 >= target}
        <li class="flex items-baseline gap-2">
          <button
            onclick={() => toggle(h.taskIdToday, h.doneToday)}
            disabled={!h.taskIdToday}
            title={h.taskIdToday ? '' : 'add this habit to today\'s daily note first'}
            class="w-4 h-4 mt-0.5 rounded border flex-shrink-0 flex items-center justify-center transition-colors disabled:opacity-50
              {h.doneToday ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
            aria-label="toggle"
          >
            {#if h.doneToday}
              <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
            {/if}
          </button>
          <span class="flex-1 text-sm {h.doneToday ? 'text-dim line-through decoration-dim/40' : 'text-text'} truncate">{h.name}</span>
          {#if target}
            <span
              class="text-[10px] px-1.5 py-0.5 rounded border tabular-nums flex-shrink-0
                {onTarget ? 'bg-surface0 text-success border-success' : 'bg-surface0 text-warning border-warning'}"
              title="weekly target"
            >🎯 {last7}/{target}</span>
          {/if}
          <span class="text-xs text-warning flex-shrink-0 tabular-nums">🔥 {h.currentStreak}</span>
        </li>
      {/each}
      {#if data.habits.length > 6}
        <li class="text-xs text-dim">+{data.habits.length - 6} more</li>
      {/if}
    </ul>

    <!-- Tick-all shortcut. Only renders when there's something to
         tick; once everything's done the row collapses to keep the
         widget tight. -->
    {#if doneCount < totalCount}
      <button
        type="button"
        onclick={tickAll}
        disabled={bulkBusy}
        class="mt-3 w-full text-[11px] py-1.5 rounded border bg-surface0 text-success border-success hover:bg-surface1 disabled:opacity-50"
      >
        {bulkBusy ? '…' : `Tick all (${totalCount - doneCount})`}
      </button>
    {/if}
  {/if}
</section>
