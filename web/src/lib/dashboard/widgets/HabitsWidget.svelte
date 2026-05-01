<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type HabitsResponse } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import Skeleton from '$lib/components/Skeleton.svelte';

  let data = $state<HabitsResponse | null>(null);
  let loading = $state(false);

  async function load() {
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

  async function toggle(taskId: string | undefined, done: boolean) {
    if (!taskId) return;
    try {
      await api.patchTask(taskId, { done: !done });
      await load();
    } catch {}
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Habits</h2>
    <a href="/habits" class="text-xs text-secondary hover:underline">all →</a>
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
    <ul class="space-y-2">
      {#each data.habits.slice(0, 6) as h (h.name)}
        <li class="flex items-baseline gap-2">
          <button
            onclick={() => toggle(h.taskIdToday, h.doneToday)}
            disabled={!h.taskIdToday}
            class="w-4 h-4 mt-0.5 rounded border flex-shrink-0 flex items-center justify-center disabled:opacity-50
              {h.doneToday ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
            aria-label="toggle"
          >
            {#if h.doneToday}
              <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
            {/if}
          </button>
          <span class="flex-1 text-sm {h.doneToday ? 'text-dim' : 'text-text'} truncate">{h.name}</span>
          <span class="text-xs text-warning flex-shrink-0">🔥 {h.currentStreak}</span>
        </li>
      {/each}
      {#if data.habits.length > 6}
        <li class="text-xs text-dim">+{data.habits.length - 6} more</li>
      {/if}
    </ul>
  {/if}
</section>
