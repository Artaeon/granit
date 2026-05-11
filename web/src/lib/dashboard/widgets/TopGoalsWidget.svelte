<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Goal } from '$lib/api';
  import { daysUntilTarget, targetBorderColor, targetChip } from '$lib/goals/util';

  // TopGoalsWidget — "next ≤3 goal targets" dashboard tile, parallel
  // to TopDeadlinesWidget. Surfaces active/paused goals with a
  // parseable target_date sorted by proximity (overdue first), so the
  // user sees the most-imminent commitments without clicking into
  // /goals. Goals without a date are deliberately excluded — the
  // widget is for "by-when" pressure, not aspirational tracking.
  //
  // Urgency math (daysUntilTarget / targetBorderColor / targetChip)
  // lives in $lib/goals/util so this widget and the /goals page stay
  // in lockstep — drift here used to be a real risk.

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

  function progressPct(g: Goal): number | null {
    const ms = g.milestones ?? [];
    if (ms.length === 0) return null;
    const done = ms.filter((m) => m.done).length;
    return Math.round((done / ms.length) * 100);
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
          {@const chip = targetChip(goal.target_date)}
          <li
            class="flex items-start gap-2 pl-2.5 pr-1 py-1.5 bg-mantle rounded border-l-2"
            style="border-left-color: {targetBorderColor(days)};"
          >
            <a href="/goals?focus={encodeURIComponent(goal.id)}" class="flex-1 min-w-0 hover:text-primary">
              <div class="flex items-baseline gap-1.5 flex-wrap">
                <span class="text-sm text-text font-medium flex-1 min-w-0 truncate">{goal.title}</span>
                {#if chip}
                  <span
                    class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium tabular-nums whitespace-nowrap"
                    style="background: color-mix(in srgb, var(--color-{chip.tone}) 14%, transparent); color: var(--color-{chip.tone});"
                  >{chip.label}</span>
                {/if}
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
