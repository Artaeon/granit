<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, todayISO, type Goal } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { createCoalescedReload } from '$lib/util/coalesce';
  import { onLocalMidnight } from '$lib/util/midnightTick';
  import { inlineMd } from '$lib/util/inlineMd';

  // Goals progress — top 3 active goals with a progress bar +
  // urgency tinting on the target_date. Click goes to the goal
  // detail. Reads the milestone list to compute progress; falls
  // back to days-to-target when the user hasn't broken the goal
  // into milestones yet.
  //
  // Sort: target_date proximity (closest first), then alphabetic.
  // Goals without a target_date sort to the bottom — explicit
  // deadlines pull urgency forward.

  let goals = $state<Goal[]>([]);
  let loading = $state(false);
  // Reactive `today` — daysToTarget compares against this; without
  // re-evaluating past midnight the "in 3d" pills lie by a day.
  let today = $state(todayISO());

  async function load() {
    loading = true;
    try {
      const list = await api.listGoals();
      const active = list.goals.filter((g) => (g.status ?? 'active') === 'active');
      // Sort: dated goals first (by closest target_date), undated by
      // recently-updated. The first 3 surface here; the user clicks
      // through for the full list.
      const sorted = active.sort((a, b) => {
        const aT = a.target_date ?? '';
        const bT = b.target_date ?? '';
        if (aT && bT) return aT.localeCompare(bT);
        if (aT) return -1;
        if (bT) return 1;
        return (b.updated_at ?? '').localeCompare(a.updated_at ?? '');
      });
      goals = sorted.slice(0, 3);
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
      if (ev.type === 'state.changed' && ev.path === '.granit/goals.json') reload.trigger();
    });
  });
  onDestroy(() => {
    reload.cancel();
    if (stopMidnight) stopMidnight();
  });

  function progress(g: Goal): number {
    const ms = g.milestones ?? [];
    if (ms.length === 0) return 0;
    return Math.round((ms.filter((m) => m.done).length / ms.length) * 100);
  }

  // Days until target. Negative when overdue. Returns null when no
  // target_date or when the date is unparseable (free-text targets
  // like "Q4 2026" — we leave those as a chip, not a countdown).
  function daysToTarget(g: Goal): number | null {
    if (!g.target_date) return null;
    if (!/^\d{4}-\d{2}-\d{2}$/.test(g.target_date)) return null;
    const a = new Date(g.target_date + 'T00:00:00');
    const b = new Date(today + 'T00:00:00');
    return Math.round((a.getTime() - b.getTime()) / 86400000);
  }

  // Tone for the target date — error if overdue, warning ≤14 days,
  // dim otherwise. Drives the date chip's color.
  function dateTone(days: number | null): string {
    if (days === null) return 'dim';
    if (days < 0) return 'error';
    if (days <= 14) return 'warning';
    return 'dim';
  }

  function fmtDays(days: number | null): string {
    if (days === null) return '';
    if (days < 0) return `${Math.abs(days)}d overdue`;
    if (days === 0) return 'today';
    if (days === 1) return 'tomorrow';
    if (days < 30) return `in ${days}d`;
    if (days < 90) return `in ${Math.round(days / 7)}w`;
    return `in ${Math.round(days / 30)}mo`;
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-3">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Goals</h2>
    <a href="/goals" class="text-xs text-secondary hover:underline">all →</a>
  </div>
  {#if loading && goals.length === 0}
    <ul class="space-y-2.5">
      {#each [0, 1, 2] as i (i)}
        <li>
          <div class="h-3 bg-surface1 rounded animate-pulse {i === 1 ? 'w-3/4' : ''}"></div>
          <div class="h-1.5 bg-surface1 rounded-full animate-pulse mt-1.5"></div>
        </li>
      {/each}
    </ul>
  {:else if goals.length === 0}
    <p class="text-sm text-dim italic">
      No active goals — <a href="/goals" class="text-secondary hover:underline">set one →</a>
    </p>
  {:else}
    <ul class="space-y-2.5">
      {#each goals as g (g.id)}
        {@const p = progress(g)}
        {@const days = daysToTarget(g)}
        {@const overdue = days !== null && days < 0}
        {@const tone = dateTone(days)}
        <li class="border-l-2 pl-2.5 py-0.5" style="border-left-color: var(--color-{tone === 'dim' ? 'surface2' : tone});">
          <a href="/goals?focus={encodeURIComponent(g.id)}" class="block hover:opacity-90 group">
            <div class="flex items-baseline gap-2">
              <span class="text-sm text-text truncate flex-1 group-hover:text-primary transition-colors">{@html inlineMd(g.title)}</span>
              {#if g.target_date}
                <span class="text-[10px] tabular-nums flex-shrink-0" style="color: var(--color-{tone});">
                  {days !== null ? fmtDays(days) : g.target_date}
                </span>
              {/if}
            </div>
            <div class="flex items-baseline gap-2 mt-0.5">
              {#if g.venture}
                <span class="text-[10px] text-secondary truncate">🏢 {g.venture}</span>
              {:else if g.project}
                <span class="text-[10px] text-secondary truncate">📁 {g.project}</span>
              {/if}
              {#if g.milestones && g.milestones.length > 0}
                <span class="text-[10px] text-dim tabular-nums ml-auto flex-shrink-0">
                  {p}% · {g.milestones.filter((m) => m.done).length}/{g.milestones.length}
                </span>
              {/if}
            </div>
            {#if g.milestones && g.milestones.length > 0}
              <div class="h-1 bg-mantle rounded-full overflow-hidden mt-1">
                <div
                  class="h-full transition-all duration-300 {p === 100 ? 'bg-success' : overdue ? 'bg-surface2' : 'bg-primary'}"
                  style="width: {p}%"
                ></div>
              </div>
            {/if}
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</section>
