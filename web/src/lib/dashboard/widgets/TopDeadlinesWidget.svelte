<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Deadline, type Goal } from '$lib/api';

  // TopDeadlinesWidget renders the next ≤3 deadlines with a countdown
  // and the linked goal/project. Uses tryListDeadlines() so the widget
  // gracefully renders nothing on environments where the endpoint is
  // unreachable (404 / network), instead of poisoning the dashboard
  // with an error card.
  let deadlines = $state<Deadline[] | null>(null);
  let goalsById = $state<Record<string, string>>({});
  let loaded = $state(false);

  async function load() {
    const [ds, gs] = await Promise.all([
      api.tryListDeadlines(),
      api.listGoals().catch(() => ({ goals: [] as Goal[], total: 0 }))
    ]);
    deadlines = ds;
    const map: Record<string, string> = {};
    for (const g of gs.goals) map[g.id] = g.title;
    goalsById = map;
    loaded = true;
  }
  onMount(load);

  function daysUntil(iso: string): number {
    const [y, m, d] = iso.split('-').map(Number);
    const due = new Date(y, m - 1, d);
    const t = new Date();
    t.setHours(0, 0, 0, 0);
    return Math.round((due.getTime() - t.getTime()) / 86_400_000);
  }
  function tone(days: number): string {
    if (days < 0) return 'error';
    if (days <= 3) return 'error';
    if (days <= 7) return 'warning';
    if (days <= 14) return 'info';
    return 'subtext';
  }
  function relLabel(days: number): string {
    if (days < 0) return `${Math.abs(days)}d overdue`;
    if (days === 0) return 'today';
    if (days === 1) return 'tomorrow';
    if (days < 14) return `in ${days}d`;
    if (days < 60) return `in ${Math.round(days / 7)}w`;
    return `in ${Math.round(days / 30)}mo`;
  }

  let upcoming = $derived.by(() => {
    if (!deadlines) return [];
    // Hide cancelled & met from the dashboard widget — only the active /
    // missed bucket is "what should I worry about right now".
    return deadlines
      .filter((d) => d.status !== 'cancelled' && d.status !== 'met')
      .map((d) => ({ d, days: daysUntil(d.date) }))
      .sort((a, b) => a.days - b.days)
      .slice(0, 3);
  });
</script>

{#if loaded && deadlines !== null}
  <section class="bg-surface0 border border-surface1 rounded-lg p-4">
    <div class="flex items-baseline justify-between mb-3">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Top deadlines</h2>
      <a href="/deadlines" class="text-xs text-secondary hover:underline">all →</a>
    </div>
    {#if upcoming.length === 0}
      <div class="text-sm text-dim italic">no upcoming deadlines</div>
    {:else}
      <ul class="space-y-2.5">
        {#each upcoming as { d, days } (d.id)}
          {@const t = tone(days)}
          <li>
            <div class="flex items-baseline gap-2">
              <span
                class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium tabular-nums whitespace-nowrap"
                style="background: color-mix(in srgb, var(--color-{t}) 14%, transparent); color: var(--color-{t}); border: 1px solid color-mix(in srgb, var(--color-{t}) 30%, transparent);"
              >
                {relLabel(days)}
              </span>
              <span class="text-sm text-text flex-1 truncate">{d.title}</span>
              <span class="text-[11px] text-dim font-mono tabular-nums">{d.date}</span>
            </div>
            {#if (d.goal_id && goalsById[d.goal_id]) || d.project}
              <div class="text-[11px] text-dim mt-0.5 ml-1 truncate">
                {#if d.goal_id && goalsById[d.goal_id]}
                  <a href="/goals" class="hover:text-secondary">🎯 {goalsById[d.goal_id]}</a>
                {:else if d.project}
                  <a href="/projects" class="hover:text-secondary">⏵ {d.project}</a>
                {/if}
              </div>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>
{/if}
