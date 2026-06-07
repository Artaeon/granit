<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, todayISO, type HabitInfo } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { createCoalescedReload } from '$lib/util/coalesce';
  import { onLocalMidnight } from '$lib/util/midnightTick';

  // StreaksWidget — top habits by current streak with a 14-day mini
  // strip per habit (today's cell highlighted). The user reads
  // "what's my best chain" + "did I do it today" in one row.
  //
  // Sort: doneToday OR currentStreak >= 7 first (the chains worth
  // protecting), then by currentStreak desc. A long-streak habit
  // not yet ticked today gets a subtle warning ring on its today
  // cell so the user notices the chain is at risk.

  let habits = $state<HabitInfo[]>([]);
  let loading = $state(false);
  // Reactive so the "today" cell in the 14-day strip migrates forward
  // when the dashboard's been open over a day-boundary.
  let today = $state(todayISO());

  async function load() {
    loading = true;
    try {
      const r = await api.listHabits();
      habits = [...r.habits].sort((a, b) => {
        // At-risk-today (long streak, not done yet) sorts to top so
        // the user sees what needs protecting first.
        const aRisk = a.currentStreak >= 3 && !a.doneToday ? 1 : 0;
        const bRisk = b.currentStreak >= 3 && !b.doneToday ? 1 : 0;
        if (aRisk !== bRisk) return bRisk - aRisk;
        return (b.currentStreak ?? 0) - (a.currentStreak ?? 0);
      });
    } catch {
      habits = [];
    } finally {
      loading = false;
    }
  }

  const reload = createCoalescedReload(load, 600);
  let stopMidnight: (() => void) | null = null;
  onMount(() => {
    void load();
    stopMidnight = onLocalMidnight(() => {
      today = todayISO();
      void load();
    });
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') reload.trigger();
    });
  });
  onDestroy(() => {
    reload.cancel();
    if (stopMidnight) stopMidnight();
  });

  function fireGlyph(streak: number): { glyph: string; tone: string } {
    if (streak >= 30) return { glyph: '🔥', tone: 'text-warning' };
    if (streak >= 7) return { glyph: '✨', tone: 'text-secondary' };
    if (streak >= 1) return { glyph: '·', tone: 'text-dim' };
    return { glyph: '○', tone: 'text-dim/50' };
  }

  // 14-day strip width. Hidden on extra-narrow widgets via flex
  // shrink, kept visible from sm: up. Today's cell gets a primary
  // ring so the user can see "this is the live cell".
  const STRIP_DAYS = 14;
</script>

<section class="bg-surface0 border border-surface1 rounded-lg shadow-sm p-3">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs text-dim font-semibold">Streaks</h2>
    {#if !loading && habits.length > 0}
      {@const atRisk = habits.filter((h) => h.currentStreak >= 3 && !h.doneToday).length}
      {#if atRisk > 0}
        <span class="text-[10px] text-warning font-medium" title="long streaks not yet ticked today">
          {atRisk} at risk
        </span>
      {/if}
    {/if}
    <span class="flex-1"></span>
    <a href="/habits" class="text-xs text-secondary hover:underline">all →</a>
  </div>

  {#if loading && habits.length === 0}
    <ul class="space-y-2">
      {#each [0, 1, 2] as i (i)}
        <li class="flex items-center gap-2">
          <span class="w-5 h-3 bg-surface1 rounded animate-pulse"></span>
          <span class="flex-1 h-3 bg-surface1 rounded animate-pulse {i === 1 ? 'w-3/4' : ''}"></span>
          <span class="w-10 h-3 bg-surface1 rounded animate-pulse"></span>
        </li>
      {/each}
    </ul>
  {:else if habits.length === 0}
    <p class="text-sm text-dim italic">
      No habits tracked yet — <a href="/habits" class="text-secondary hover:underline">add one →</a>
    </p>
  {:else}
    <ul class="space-y-2">
      {#each habits.slice(0, 6) as h (h.name)}
        {@const fire = fireGlyph(h.currentStreak)}
        {@const atRisk = h.currentStreak >= 3 && !h.doneToday}
        <li class="flex items-center gap-2">
          <span class="text-base w-5 flex-shrink-0 {fire.tone}" aria-hidden="true">{fire.glyph}</span>
          <span class="text-sm text-text flex-1 truncate {h.doneToday ? '' : 'opacity-70'}">{h.name}</span>
          <span class="text-xs font-mono text-dim w-12 text-right tabular-nums">
            {h.currentStreak}d
          </span>
          <!-- 14-day strip — one cell per day. Today cell gets a
               primary ring (or warning ring if the chain's at risk
               and not yet ticked). Hidden on the smallest widths
               so the row stays single-line on a narrow widget. -->
          <div class="hidden sm:flex gap-px flex-shrink-0">
            {#each h.days.slice(-STRIP_DAYS) as d (d.date)}
              {@const isToday = d.date === today}
              <span
                class="w-1.5 h-3 rounded-sm
                  {d.done ? 'bg-success' : 'bg-surface1'}
                  {isToday && atRisk ? 'ring-1 ring-warning' : isToday ? 'ring-1 ring-primary/60' : ''}"
                title={`${d.date}${d.done ? ' · done' : ''}`}
              ></span>
            {/each}
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</section>
