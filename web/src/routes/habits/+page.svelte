<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type HabitInfo, type HabitsResponse } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';

  let data = $state<HabitsResponse | null>(null);
  let loading = $state(false);
  let busy = $state<string | null>(null);

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      data = await api.listHabits();
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    });
  });

  async function toggleToday(h: HabitInfo) {
    busy = h.name;
    try {
      // If the habit task already exists in today's daily, just toggle.
      if (h.taskIdToday) {
        await api.patchTask(h.taskIdToday, { done: !h.doneToday });
        await load();
        return;
      }
      // Otherwise, materialize the habit in today's daily note as a new
      // task line under "## Habits", then mark it done.
      if (!h.notePathToday) return;
      const created = await api.createTask({
        notePath: h.notePathToday,
        text: h.name,
        section: '## Habits'
      });
      await api.patchTask(created.id, { done: true });
      await load();
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      (await import('$lib/components/toast')).toast.error(`couldn't toggle: ${msg}`);
    } finally {
      busy = null;
    }
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-5xl mx-auto">
    <header class="mb-6">
      <h1 class="text-2xl sm:text-3xl font-semibold text-text">Habits</h1>
      <p class="text-sm text-dim mt-1">
        derived from the <code class="text-xs">## Habits</code> section of each daily note · last {data?.days ?? 90} days
      </p>
    </header>

    {#if loading && !data}
      <div class="space-y-4">
        {#each Array(3) as _}
          <div class="bg-surface0 border border-surface1 rounded-lg p-4 space-y-3">
            <div class="flex items-start gap-3">
              <Skeleton class="h-6 w-6 rounded flex-shrink-0" />
              <div class="flex-1 space-y-2">
                <Skeleton class="h-5 w-1/2" />
                <Skeleton class="h-3 w-2/3" />
              </div>
            </div>
            <Skeleton class="h-12 w-full" />
          </div>
        {/each}
      </div>
    {:else if data && data.habits.length === 0}
      <div class="bg-surface0 border border-surface1 rounded-lg p-5 sm:p-6 leading-relaxed">
        <div class="text-4xl mb-3 opacity-60">◈</div>
        <h2 class="text-base font-medium text-text">Track habits in your daily notes</h2>
        <p class="text-sm text-dim mt-2 max-w-prose">
          Add a <code class="text-xs">## Habits</code> section to any daily note with checkbox lines.
          The web dashboard scans the last 90 days, computes streaks, and shows a dot grid like GitHub contributions.
        </p>
        <pre class="mt-4 p-3 bg-mantle rounded text-xs text-secondary font-mono overflow-x-auto">## Habits

- [ ] morning movement
- [ ] read 20 pages
- [ ] no doomscrolling</pre>
        <p class="mt-3 text-xs text-dim">
          The same checkboxes show up as tasks in the TUI — both views stay in sync via the markdown.
        </p>
      </div>
    {:else if data}
      <div class="space-y-4">
        {#each data.habits as h (h.name)}
          <article class="bg-surface0 border border-surface1 rounded-lg p-4">
            <div class="flex items-start gap-3 mb-3">
              <button
                onclick={() => toggleToday(h)}
                disabled={busy === h.name}
                title={h.taskIdToday ? (h.doneToday ? 'mark not done today' : 'mark done today') : 'open daily note to add this habit'}
                class="w-6 h-6 mt-0.5 rounded border flex-shrink-0 flex items-center justify-center transition-colors disabled:opacity-50
                  {h.doneToday ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
                aria-label="toggle today"
              >
                {#if h.doneToday}
                  <svg viewBox="0 0 12 12" class="w-4 h-4 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
                {/if}
              </button>
              <div class="flex-1 min-w-0">
                <h2 class="text-base font-medium text-text break-words">{h.name}</h2>
                <div class="flex flex-wrap items-baseline gap-x-4 gap-y-0.5 text-xs text-dim mt-0.5">
                  <span>🔥 {h.currentStreak}-day streak</span>
                  <span>longest: {h.longestStreak}</span>
                  <span>last 7: {h.last7Pct}%</span>
                  <span>last 30: {h.last30Pct}%</span>
                </div>
              </div>
            </div>

            <!-- Dot grid: 90 days, oldest→newest -->
            <div class="grid grid-flow-col grid-rows-7 gap-0.5" style="grid-auto-columns: minmax(0, 1fr);">
              {#each h.days as d (d.date)}
                {@const isToday = d.date === data.today}
                <div
                  class="aspect-square rounded-[2px] {d.done ? 'bg-success' : 'bg-surface1'}"
                  class:ring-1={isToday}
                  class:ring-primary={isToday}
                  title="{d.date}{d.done ? ' · done' : ''}"
                ></div>
              {/each}
            </div>
          </article>
        {/each}
      </div>
    {/if}
  </div>
</div>
