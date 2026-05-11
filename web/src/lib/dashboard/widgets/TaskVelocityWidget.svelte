<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Task } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  // TaskVelocityWidget — 8-week bar chart of completed tasks per
  // ISO week. Pure-CSS bars (no SVG, no chart lib) — height as a
  // percentage of the busiest week, color based on relative
  // intensity. Pulls from the existing /tasks list and aggregates
  // client-side; `completedAt` is set by the task store on every
  // done-flip.
  //
  // Why this matters: the dashboard already shows "today's tasks"
  // and "open tasks", but the user has no read on whether they're
  // actually shipping work over time. A flat 8-week trend signals
  // "stuck"; an upward slope signals momentum.

  const WEEKS = 8;

  let buckets = $state<{ label: string; count: number; isThisWeek: boolean }[]>([]);
  let total = $state(0);
  let loading = $state(true);

  function weekKey(d: Date): string {
    // ISO week date — same scheme the /review page uses, so a
    // user matching this widget's "week 18" tally against their
    // weekly review for week 18 sees the same bucket. Year-week
    // strings sort naturally.
    const t = new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()));
    const day = (t.getUTCDay() + 6) % 7;
    t.setUTCDate(t.getUTCDate() - day + 3);
    const firstThu = new Date(Date.UTC(t.getUTCFullYear(), 0, 4));
    const week = 1 + Math.round((t.getTime() - firstThu.getTime()) / (7 * 24 * 60 * 60 * 1000));
    return `${t.getUTCFullYear()}-W${String(week).padStart(2, '0')}`;
  }

  function startOfIsoWeek(d: Date): Date {
    const t = new Date(d);
    const day = (t.getDay() + 6) % 7; // 0 = Monday
    t.setDate(t.getDate() - day);
    t.setHours(0, 0, 0, 0);
    return t;
  }

  async function load() {
    loading = true;
    try {
      const r = await api.listTasks({ status: 'done' });
      const tasks = r.tasks as Task[];
      const now = new Date();
      const thisWeekKey = weekKey(now);
      const weekStart = startOfIsoWeek(now);

      // Build the last WEEKS week buckets. Pre-seed with zeros so
      // empty weeks still render (a flat zero is information).
      const order: string[] = [];
      const labelByKey = new Map<string, string>();
      for (let i = WEEKS - 1; i >= 0; i--) {
        const d = new Date(weekStart);
        d.setDate(d.getDate() - i * 7);
        const k = weekKey(d);
        order.push(k);
        // Compact label: "W18" for older weeks, "Now" for the
        // current one. Keeps the x-axis legible at narrow widths.
        labelByKey.set(k, k === thisWeekKey ? 'Now' : k.split('W')[1]);
      }
      const counts = new Map<string, number>();
      for (const t of tasks) {
        if (!t.completedAt) continue;
        const d = new Date(t.completedAt);
        if (Number.isNaN(d.getTime())) continue;
        const k = weekKey(d);
        if (!order.includes(k)) continue;
        counts.set(k, (counts.get(k) ?? 0) + 1);
      }
      buckets = order.map((k) => ({
        label: labelByKey.get(k) ?? k,
        count: counts.get(k) ?? 0,
        isThisWeek: k === thisWeekKey
      }));
      total = buckets.reduce((s, b) => s + b.count, 0);
    } catch {
      buckets = [];
      total = 0;
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'task.changed' || ev.type === 'note.changed') void load();
    });
  });

  const max = $derived(buckets.reduce((m, b) => Math.max(m, b.count), 0));
  // Trend = average of last 3 weeks vs the 3 weeks before. >1.0 =
  // accelerating, <1.0 = slowing. Used for the small ↗/→/↘ arrow
  // next to the headline so the user reads momentum at a glance.
  const trend = $derived.by(() => {
    if (buckets.length < 6) return null;
    const recent = buckets.slice(-3).reduce((s, b) => s + b.count, 0) / 3;
    const earlier = buckets.slice(-6, -3).reduce((s, b) => s + b.count, 0) / 3;
    if (recent === 0 && earlier === 0) return null;
    if (earlier === 0) return 'up';
    const ratio = recent / earlier;
    if (ratio > 1.15) return 'up';
    if (ratio < 0.85) return 'down';
    return 'flat';
  });
  const thisWeekCount = $derived(buckets[buckets.length - 1]?.count ?? 0);
</script>

<div class="bg-surface0 border border-surface1 rounded-lg p-4">
  <header class="flex items-baseline gap-2 mb-3">
    <h3 class="text-sm font-medium text-text">Task velocity</h3>
    <span class="flex-1"></span>
    <span class="text-[11px] text-dim">last {WEEKS} weeks</span>
  </header>

  {#if loading}
    <div class="text-xs text-dim italic">Loading…</div>
  {:else if total === 0}
    <p class="text-xs text-dim italic">
      No completed tasks in the last {WEEKS} weeks. Knock one out — the bar shows up here.
    </p>
  {:else}
    <div class="flex items-baseline gap-2 mb-3">
      <span class="text-2xl font-semibold text-text">{thisWeekCount}</span>
      <span class="text-xs text-dim">this week</span>
      {#if trend === 'up'}
        <span class="text-success text-xs" title="3-week avg trending up">↗</span>
      {:else if trend === 'down'}
        <span class="text-warning text-xs" title="3-week avg trending down">↘</span>
      {:else if trend === 'flat'}
        <span class="text-dim text-xs" title="3-week avg holding steady">→</span>
      {/if}
      <span class="flex-1"></span>
      <span class="text-[11px] text-dim font-mono">{total} total</span>
    </div>
    <!-- Bars. Min-height 2px so a zero week still has a visible
         baseline; the user reads a row of nubs instead of empty
         space. Current week gets the primary color so it pops. -->
    <div class="flex items-end gap-1 h-16">
      {#each buckets as b (b.label)}
        {@const pct = max === 0 ? 0 : Math.max(2, Math.round((b.count / max) * 100))}
        <div class="flex-1 flex flex-col items-center justify-end gap-1" title="{b.label}: {b.count} task{b.count === 1 ? '' : 's'}">
          <div
            class="w-full rounded-t {b.isThisWeek ? 'bg-primary' : 'bg-surface2'} transition-all"
            style="height: {pct}%"
          ></div>
          <div class="text-[9px] text-dim font-mono leading-none">{b.label}</div>
        </div>
      {/each}
    </div>
  {/if}
</div>
