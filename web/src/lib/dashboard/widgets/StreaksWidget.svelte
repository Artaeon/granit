<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type HabitInfo } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  // StreaksWidget surfaces the habit streaks people actually care about:
  // "what's my longest active chain right now". Sorted by current streak
  // descending so the user sees their best work first.

  let habits = $state<HabitInfo[]>([]);

  async function load() {
    try {
      const r = await api.listHabits();
      habits = [...r.habits].sort((a, b) => (b.currentStreak ?? 0) - (a.currentStreak ?? 0));
    } catch {
      habits = [];
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    });
  });

  function fireEmoji(streak: number): string {
    if (streak >= 30) return '🔥';
    if (streak >= 7) return '✨';
    if (streak >= 1) return '·';
    return '';
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Streaks</h2>
    <a href="/habits" class="text-xs text-secondary hover:underline">all →</a>
  </div>

  {#if habits.length === 0}
    <div class="text-sm text-dim italic">No habits tracked yet.</div>
  {:else}
    <ul class="space-y-2">
      {#each habits.slice(0, 6) as h (h.name)}
        <li class="flex items-center gap-2">
          <span class="text-lg w-5 flex-shrink-0">{fireEmoji(h.currentStreak)}</span>
          <span class="text-sm text-text flex-1 truncate {h.doneToday ? '' : 'opacity-70'}">{h.name}</span>
          <span class="text-xs font-mono text-dim w-12 text-right tabular-nums">
            {h.currentStreak}d
          </span>
          <!-- Last 7 mini-strip — one cell per day, filled = done -->
          <div class="hidden sm:flex gap-px flex-shrink-0">
            {#each h.days.slice(-7) as d}
              <span
                class="w-1.5 h-3 rounded-sm"
                style="background: {d.done ? 'var(--color-success)' : 'var(--color-surface1)'}"
              ></span>
            {/each}
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</section>
