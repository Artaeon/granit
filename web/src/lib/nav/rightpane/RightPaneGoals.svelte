<script lang="ts">
  // RightPaneGoals — top 10 active goals with simple progress bars.
  // Mirrors the GoalsProgressWidget's sort (target_date proximity
  // first; undated by recent updated_at) so users get a consistent
  // ordering between the dashboard widget and the pane.
  //
  // Progress is computed from milestone completion: 0% when no
  // milestones exist (the goal exists but hasn't been broken down
  // yet). A target_date pill on the right gives the deadline tone
  // (overdue / soon / far).

  import { onMount, onDestroy } from 'svelte';
  import { api, todayISO, type Goal } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { createCoalescedReload } from '$lib/util/coalesce';
  import { onLocalMidnight } from '$lib/util/midnightTick';
  import RightPaneSection from './RightPaneSection.svelte';

  let goals = $state<Goal[]>([]);
  let loading = $state(true);
  let error = $state(false);
  let today = $state(todayISO());

  // Gen counter guards against stale resolves after rapid content
  // switching — see TaskDetail.svelte's loadSubtasks for the pattern.
  let loadGen = 0;
  let destroyed = false;

  async function load() {
    const myGen = ++loadGen;
    try {
      const list = await api.listGoals();
      if (destroyed || myGen !== loadGen) return;
      const active = list.goals.filter((g) => (g.status ?? 'active') === 'active');
      active.sort((a, b) => {
        const aT = a.target_date ?? '';
        const bT = b.target_date ?? '';
        if (aT && bT) return aT.localeCompare(bT);
        if (aT) return -1;
        if (bT) return 1;
        return (b.updated_at ?? '').localeCompare(a.updated_at ?? '');
      });
      goals = active.slice(0, 10);
      error = false;
    } catch {
      if (destroyed || myGen !== loadGen) return;
      error = true;
    } finally {
      if (!destroyed && myGen === loadGen) loading = false;
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
      // Goals persist to .granit/goals.json; the server broadcasts
      // state.changed for that path on every write (same signal the
      // GoalsProgressWidget listens to).
      if (ev.type === 'state.changed' && ev.path === '.granit/goals.json') reload.trigger();
    });
  });
  onDestroy(() => {
    destroyed = true;
    reload.cancel();
    if (stopMidnight) stopMidnight();
  });

  function progress(g: Goal): number {
    const ms = g.milestones ?? [];
    if (ms.length === 0) return 0;
    return Math.round((ms.filter((m) => m.done).length / ms.length) * 100);
  }

  function daysToTarget(g: Goal): number | null {
    if (!g.target_date) return null;
    if (!/^\d{4}-\d{2}-\d{2}$/.test(g.target_date)) return null;
    const a = new Date(g.target_date + 'T00:00:00');
    const b = new Date(today + 'T00:00:00');
    return Math.round((a.getTime() - b.getTime()) / 86400000);
  }

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
    if (days === 1) return '1d';
    if (days < 30) return `${days}d`;
    if (days < 90) return `${Math.round(days / 7)}w`;
    return `${Math.round(days / 30)}mo`;
  }
</script>

<RightPaneSection
  title="Goals"
  badge={`${goals.length} active`}
  footerLabel="Open goals"
  footerHref="/goals"
>
  {#if loading}
    <ul class="space-y-2.5 px-1">
      {#each [0, 1, 2, 3] as i (i)}
        <li>
          <div class="h-3 w-3/4 bg-surface1 rounded animate-pulse"></div>
          <div class="h-1.5 bg-surface1 rounded-full animate-pulse mt-1.5"></div>
        </li>
      {/each}
    </ul>
  {:else if error}
    <p class="px-2 text-dim italic text-xs">Couldn't load goals.</p>
  {:else if goals.length === 0}
    <p class="px-2 text-dim italic text-xs">
      No active goals — <a href="/goals" class="text-secondary hover:underline">set one →</a>
    </p>
  {:else}
    <ul class="space-y-2.5 px-1">
      {#each goals as g (g.id)}
        {@const p = progress(g)}
        {@const days = daysToTarget(g)}
        {@const overdue = days !== null && days < 0}
        {@const tone = dateTone(days)}
        <li class="border-l-2 pl-2.5 py-0.5" style="border-left-color: var(--color-{tone === 'dim' ? 'surface2' : tone});">
          <a href="/goals?focus={encodeURIComponent(g.id)}" class="block hover:opacity-90 group">
            <div class="flex items-baseline gap-2">
              <span class="text-[13px] text-text truncate flex-1 group-hover:text-primary transition-colors" title={g.title}>{g.title}</span>
              {#if g.target_date}
                <span class="text-[10px] tabular-nums flex-shrink-0" style="color: var(--color-{tone});">
                  {days !== null ? fmtDays(days) : g.target_date}
                </span>
              {/if}
            </div>
            {#if g.milestones && g.milestones.length > 0}
              <div class="flex items-baseline gap-2 mt-0.5">
                <span class="text-[10px] text-dim tabular-nums ml-auto flex-shrink-0">
                  {p}% · {g.milestones.filter((m) => m.done).length}/{g.milestones.length}
                </span>
              </div>
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
</RightPaneSection>
