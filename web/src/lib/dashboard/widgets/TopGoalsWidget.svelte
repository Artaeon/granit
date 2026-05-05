<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Goal } from '$lib/api';

  // TopGoalsWidget — "next ≤3 goal targets" dashboard tile, parallel
  // to TopDeadlinesWidget. Surfaces active/paused goals with a
  // parseable target_date sorted by proximity (overdue first), so the
  // user sees the most-imminent commitments without clicking into
  // /goals. Goals without a date are deliberately excluded — the
  // widget is for "by-when" pressure, not aspirational tracking.

  let goals = $state<Goal[] | null>(null);
  let loaded = $state(false);

  async function load() {
    try {
      const r = await api.listGoals();
      goals = r.goals;
    } catch {
      goals = [];
    } finally {
      loaded = true;
    }
  }
  onMount(load);

  // Days until target_date in local time. Returns null when the
  // string isn't a real date (some legacy goals stash free-text like
  // "Q4 2026"); those goals are excluded from the widget. Same shape
  // as the helper on /goals so the two surfaces agree on what "days
  // until" means.
  function daysUntilTarget(s: string | undefined): number | null {
    if (!s) return null;
    const d = new Date(s);
    if (isNaN(d.getTime())) return null;
    const t = new Date();
    const aMid = new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
    const bMid = new Date(t.getFullYear(), t.getMonth(), t.getDate()).getTime();
    return Math.round((aMid - bMid) / (24 * 3600 * 1000));
  }

  function progressPct(g: Goal): number | null {
    const ms = g.milestones ?? [];
    if (ms.length === 0) return null;
    const done = ms.filter((m) => m.done).length;
    return Math.round((done / ms.length) * 100);
  }

  // Same border palette as TopDeadlinesWidget so the two widgets look
  // visually homogeneous when stacked on the dashboard.
  function borderColor(days: number): string {
    if (days < 0 || days <= 7) return 'var(--color-error)';
    if (days <= 30) return 'var(--color-warning)';
    if (days <= 90) return 'var(--color-info)';
    return 'var(--color-surface2)';
  }

  function chipLabel(days: number): string {
    if (days < 0) return `${Math.abs(days)}d past target`;
    if (days === 0) return 'today';
    if (days === 1) return 'tomorrow';
    if (days < 14) return `in ${days}d`;
    if (days < 60) return `in ${Math.round(days / 7)}w`;
    return `in ${Math.round(days / 30)}mo`;
  }

  function chipTone(days: number): string {
    if (days < 0 || days <= 7) return 'error';
    if (days <= 30) return 'warning';
    if (days <= 90) return 'info';
    return 'subtext';
  }

  let upcoming = $derived.by(() => {
    if (!goals) return [];
    const out: { goal: Goal; days: number }[] = [];
    for (const g of goals) {
      const status = g.status ?? 'active';
      if (status !== 'active' && status !== 'paused') continue;
      const days = daysUntilTarget(g.target_date);
      if (days === null) continue;
      out.push({ goal: g, days });
    }
    out.sort((a, b) => a.days - b.days);
    return out;
  });

  let visible = $derived(upcoming.slice(0, 3));
  let extra = $derived(Math.max(0, upcoming.length - 3));
</script>

{#if loaded && goals !== null}
  <section class="bg-surface0 border border-surface1 rounded-lg p-4">
    <div class="flex items-baseline justify-between mb-3">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Next goal targets</h2>
      <a href="/goals" class="text-xs text-secondary hover:underline">all →</a>
    </div>
    {#if visible.length === 0}
      <div class="text-sm text-dim italic">
        No dated goals — <a href="/goals" class="text-secondary hover:underline">set a target →</a>
      </div>
    {:else}
      <ul class="space-y-2">
        {#each visible as { goal, days } (goal.id)}
          {@const pct = progressPct(goal)}
          <li
            class="flex items-start gap-2 pl-2.5 pr-1 py-1.5 bg-mantle/30 rounded border-l-2"
            style="border-left-color: {borderColor(days)};"
          >
            <a href="/goals?focus={encodeURIComponent(goal.id)}" class="flex-1 min-w-0 hover:text-primary">
              <div class="flex items-baseline gap-1.5 flex-wrap">
                <span class="text-sm text-text font-medium flex-1 min-w-0 truncate">{goal.title}</span>
                <span
                  class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium tabular-nums whitespace-nowrap"
                  style="background: color-mix(in srgb, var(--color-{chipTone(days)}) 14%, transparent); color: var(--color-{chipTone(days)});"
                >{chipLabel(days)}</span>
              </div>
              {#if goal.venture || goal.project || pct !== null}
                <div class="text-[11px] text-dim mt-0.5 flex items-center gap-2 flex-wrap">
                  {#if goal.venture}
                    <span class="text-secondary truncate">🏢 {goal.venture}</span>
                  {:else if goal.project}
                    <span class="text-secondary truncate">📁 {goal.project}</span>
                  {/if}
                  {#if pct !== null}
                    <span class="tabular-nums">{pct}%</span>
                  {/if}
                </div>
              {/if}
            </a>
          </li>
        {/each}
      </ul>
      {#if extra > 0}
        <a href="/goals" class="block text-xs text-secondary hover:underline mt-2.5 text-center">
          + {extra} more →
        </a>
      {/if}
    {/if}
  </section>
{/if}
