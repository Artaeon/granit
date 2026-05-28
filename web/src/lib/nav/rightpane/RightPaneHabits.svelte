<script lang="ts">
  // RightPaneHabits — today's habit check-ins as a tap-to-tick list.
  // Mirrors the HabitsWidget data shape (api.listHabits) so a tick
  // here updates the dashboard widget through the same WS round-trip.
  //
  // Each row: a checkbox + name + a small streak indicator. Tapping
  // toggles via api.toggleHabit; an optimistic flip avoids the
  // round-trip flicker the user complained about on the first pass.
  // A tick-all CTA appears when at least one habit is still undone.

  import { onMount, onDestroy } from 'svelte';
  import { api, type HabitsResponse } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { createCoalescedReload } from '$lib/util/coalesce';
  import { toast } from '$lib/components/toast';

  let data = $state<HabitsResponse | null>(null);
  let loading = $state(true);
  let error = $state(false);
  let bulkBusy = $state(false);

  async function load() {
    try {
      data = await api.listHabits();
      error = false;
    } catch {
      error = true;
    } finally {
      loading = false;
    }
  }
  const reload = createCoalescedReload(load, 600);
  onMount(() => {
    void load();
    return onWsEvent((ev) => {
      // /habits derives from daily notes, so note.changed is what
      // signals a tick (no dedicated habit.changed event in ws.ts).
      if (ev.type === 'note.changed' || ev.type === 'note.removed') reload.trigger();
    });
  });
  onDestroy(() => reload.cancel());

  async function toggle(name: string, taskId: string | undefined, done: boolean) {
    if (!data || !taskId) return;
    const habit = data.habits.find((h) => h.name === name);
    if (!habit) return;
    // Optimistic flip — find today's day entry + bump doneToday.
    const day = habit.days.find((d) => d.date === data!.today);
    if (day) day.done = !done;
    habit.doneToday = !done;
    data = { ...data };
    try {
      await api.patchTask(taskId, { done: !done });
    } catch (e) {
      toast.error('habit toggle failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      await load();
    }
  }

  async function tickAll() {
    if (!data || bulkBusy) return;
    const targets = data.habits.filter((h) => !h.doneToday && h.taskIdToday);
    if (targets.length === 0) return;
    bulkBusy = true;
    const today = data.today;
    for (const h of targets) {
      const day = h.days.find((d) => d.date === today);
      if (day) day.done = true;
      h.doneToday = true;
    }
    data = { ...data };
    const failed: string[] = [];
    await Promise.all(
      targets.map(async (h) => {
        try {
          await api.toggleHabit(h.name, today, true);
        } catch {
          failed.push(h.name);
        }
      })
    );
    bulkBusy = false;
    await load();
    if (failed.length > 0) toast.error(`couldn't tick: ${failed.join(', ')}`);
  }

  let doneCount = $derived(data?.habits.filter((h) => h.doneToday).length ?? 0);
  let totalCount = $derived(data?.habits.length ?? 0);
</script>

<div class="flex flex-col h-full text-sm min-h-0">
  <header class="flex items-baseline gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0">
    <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Habits today</h3>
    {#if data && totalCount > 0}
      <span class="text-[10px] tabular-nums text-dim">{doneCount}/{totalCount}</span>
    {/if}
  </header>

  <div class="flex-1 overflow-y-auto px-2 py-2 min-h-0">
    {#if loading}
      <ul class="space-y-2 px-1">
        {#each [0, 1, 2, 3, 4] as i (i)}
          <li class="flex items-center gap-2">
            <div class="h-4 w-4 bg-surface1 rounded animate-pulse"></div>
            <div class="h-3 {i % 2 === 0 ? 'w-3/5' : 'w-1/2'} bg-surface1 rounded animate-pulse"></div>
          </li>
        {/each}
      </ul>
    {:else if error}
      <p class="px-2 text-dim italic text-xs">Couldn't load habits.</p>
    {:else if !data || data.habits.length === 0}
      <p class="px-2 text-dim italic text-xs">
        Add a <code class="text-xs">## Habits</code> section to your daily note to track here.
      </p>
    {:else}
      <ul class="space-y-1 px-1">
        {#each data.habits as h (h.name)}
          <li class="flex items-baseline gap-2">
            <button
              onclick={() => toggle(h.name, h.taskIdToday, h.doneToday)}
              disabled={!h.taskIdToday}
              title={h.taskIdToday ? '' : "add this habit to today's daily note first"}
              class="w-4 h-4 mt-0.5 rounded border flex-shrink-0 flex items-center justify-center transition-colors disabled:opacity-50
                {h.doneToday ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
              aria-label="toggle {h.name}"
            >
              {#if h.doneToday}
                <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle" aria-hidden="true"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
              {/if}
            </button>
            <span class="flex-1 text-[13px] {h.doneToday ? 'text-dim line-through decoration-dim/40' : 'text-text'} truncate" title={h.name}>{h.name}</span>
            {#if h.currentStreak > 0}
              <span class="text-[10px] text-warning flex-shrink-0 tabular-nums" title="current streak">{h.currentStreak}d</span>
            {/if}
          </li>
        {/each}
      </ul>

      {#if doneCount < totalCount}
        <button
          type="button"
          onclick={tickAll}
          disabled={bulkBusy}
          class="mt-3 w-[calc(100%-0.5rem)] mx-1 text-[11px] py-1.5 rounded border bg-surface0 text-success border-success hover:bg-surface1 disabled:opacity-50"
        >
          {bulkBusy ? '…' : `Tick all (${totalCount - doneCount})`}
        </button>
      {/if}
    {/if}
  </div>

  <footer class="border-t border-surface1 px-3 py-1.5 flex-shrink-0">
    <a href="/habits" class="text-xs text-secondary hover:underline">Open habits →</a>
  </footer>
</div>
