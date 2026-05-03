<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Deadline, type Goal } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import DeadlinePill from '$lib/deadlines/DeadlinePill.svelte';
  import { daysUntil } from '$lib/deadlines/util';

  // TopDeadlinesWidget renders the next ≤3 deadlines with a countdown
  // pill, importance icon, and a one-tap "Done" button. Uses
  // tryListDeadlines() so the widget gracefully renders nothing on
  // environments where the endpoint is unreachable (404 / network)
  // instead of poisoning the dashboard with an error card.
  let deadlines = $state<Deadline[] | null>(null);
  let goalsById = $state<Record<string, string>>({});
  let loaded = $state(false);
  // Tracks IDs we've optimistically marked done so the row hides
  // immediately while the PATCH is in flight.
  let dismissing = $state<Set<string>>(new Set());

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

  // Border-color heuristic for the row: red ≤3 days, orange ≤7 days,
  // neutral later. Exposed as a CSS variable so the rule survives
  // Tailwind's purger (no dynamic utility class names).
  function borderColor(days: number): string {
    if (days <= 3) return 'var(--color-error)';
    if (days <= 7) return 'var(--color-warning)';
    return 'var(--color-surface2)';
  }

  let upcoming = $derived.by(() => {
    if (!deadlines) return [];
    // Hide cancelled & met from the dashboard widget — only the active /
    // missed bucket is "what should I worry about right now".
    return deadlines
      .filter((d) => d.status !== 'cancelled' && d.status !== 'met' && !dismissing.has(d.id))
      .map((d) => ({ d, days: daysUntil(d.date) }))
      .sort((a, b) => a.days - b.days);
  });

  let visible = $derived(upcoming.slice(0, 3));
  let extra = $derived(Math.max(0, upcoming.length - 3));

  // Mark a deadline met. Optimistic — we hide the row first, then
  // PATCH; on failure we revert + toast. The widget reuses
  // api.patchDeadline (already exported), so no new endpoint is needed.
  async function markDone(d: Deadline) {
    dismissing = new Set([...dismissing, d.id]);
    try {
      await api.patchDeadline(d.id, { status: 'met' });
      // Mutate the local cache so a re-render after WS doesn't put it back.
      if (deadlines) {
        deadlines = deadlines.map((x) => (x.id === d.id ? { ...x, status: 'met' } : x));
      }
      toast.success(`marked "${d.title}" met`);
    } catch (e) {
      const next = new Set(dismissing);
      next.delete(d.id);
      dismissing = next;
      toast.error('mark done failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
</script>

{#if loaded && deadlines !== null}
  <section class="bg-surface0 border border-surface1 rounded-lg p-4">
    <div class="flex items-baseline justify-between mb-3">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Top deadlines</h2>
      <a href="/deadlines" class="text-xs text-secondary hover:underline">all →</a>
    </div>
    {#if visible.length === 0}
      <div class="text-sm text-dim italic">
        No deadlines coming up — <a href="/deadlines" class="text-secondary hover:underline">set one →</a>
      </div>
    {:else}
      <ul class="space-y-2">
        {#each visible as { d, days } (d.id)}
          <li
            class="flex items-start gap-2 pl-2.5 pr-1 py-1.5 bg-mantle/30 rounded border-l-2"
            style="border-left-color: {borderColor(days)};"
          >
            <div class="flex-1 min-w-0">
              <div class="flex items-baseline gap-1.5 flex-wrap">
                <DeadlinePill variant="icon" importance={d.importance} />
                <span class="text-sm text-text font-medium flex-1 min-w-0 truncate">{d.title}</span>
                <DeadlinePill variant="countdown" {days} status={d.status} />
              </div>
              {#if (d.goal_id && goalsById[d.goal_id]) || d.project}
                <div class="text-[11px] text-dim mt-0.5 truncate">
                  {#if d.goal_id && goalsById[d.goal_id]}
                    <a href="/goals" class="hover:text-secondary">🎯 {goalsById[d.goal_id]}</a>
                  {:else if d.project}
                    <a href="/projects" class="hover:text-secondary">⏵ {d.project}</a>
                  {/if}
                </div>
              {/if}
            </div>
            <button
              type="button"
              onclick={() => markDone(d)}
              aria-label="mark done"
              title="mark done"
              class="flex-shrink-0 w-7 h-7 flex items-center justify-center rounded text-dim hover:text-success hover:bg-success/10 transition-colors"
            >
              <svg viewBox="0 0 12 12" class="w-3.5 h-3.5"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
            </button>
          </li>
        {/each}
      </ul>
      {#if extra > 0}
        <a href="/deadlines" class="block text-xs text-secondary hover:underline mt-2.5 text-center">
          + {extra} more →
        </a>
      {/if}
    {/if}
  </section>
{/if}
