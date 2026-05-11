<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Deadline } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { daysUntil } from '$lib/deadlines/util';

  // EntityDeadlines — compact list of deadlines linked to a project,
  // goal, or venture. Drops into any detail panel as a "Deadlines"
  // section with a quick-add button. Single source of truth for what
  // a "deadline section" looks like — ProjectDetail, GoalDetail, and
  // any future venture detail page render the same component so the
  // visual language stays consistent across the app.
  //
  // The component is read-mostly: editing a deadline still happens on
  // /deadlines (the full drawer with status/importance controls). We
  // provide an inline "+" jump that pre-fills the create form via URL
  // params, so the user goes "I'm in this project, I need a deadline
  // for it" → click + → /deadlines drawer with project pre-set.

  type Scope =
    | { kind: 'project'; name: string }
    | { kind: 'goal'; id: string; title?: string }
    | { kind: 'venture'; name: string };

  let { scope, max = 5 }: { scope: Scope; max?: number } = $props();

  let deadlines = $state<Deadline[]>([]);
  let loading = $state(false);
  let loaded = $state(false);

  // Match-by-scope. Mirrors the server-side sort — active+missed first
  // by date asc — but applied client-side so a single /deadlines fetch
  // serves every entity-detail panel on the page (no per-mount roundtrip).
  function matches(d: Deadline): boolean {
    if (scope.kind === 'project') return d.project === scope.name;
    if (scope.kind === 'goal') return d.goal_id === scope.id;
    if (scope.kind === 'venture') return d.venture === scope.name;
    return false;
  }

  async function load() {
    loading = true;
    try {
      const list = await api.tryListDeadlines();
      deadlines = (list ?? []).filter(matches);
    } finally {
      loading = false;
      loaded = true;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') load();
    });
  });

  // Re-filter when the scope prop changes — a parent panel can swap
  // entities (e.g. picking a different project in the list) without
  // unmounting the component.
  $effect(() => {
    void scope;
    if (loaded) load();
  });

  // Hide met + cancelled in the inline view — they belong to the past
  // and would crowd the active picture. The /deadlines page shows them
  // explicitly under their own buckets.
  let active = $derived(deadlines.filter((d) => d.status !== 'met' && d.status !== 'cancelled'));
  let recentlyMet = $derived(
    deadlines
      .filter((d) => d.status === 'met')
      .slice(0, 3)
  );

  function tone(days: number): string {
    if (days < 0) return 'error';
    if (days <= 3) return 'error';
    if (days <= 7) return 'warning';
    if (days <= 30) return 'info';
    return 'subtext';
  }
  function countdown(days: number): string {
    if (days < 0) return `${-days}d ago`;
    if (days === 0) return 'today';
    if (days === 1) return 'tomorrow';
    if (days < 14) return `in ${days}d`;
    if (days < 60) return `in ${Math.round(days / 7)}w`;
    return `in ${Math.round(days / 30)}mo`;
  }

  let scopeQuery = $derived.by(() => {
    if (scope.kind === 'project') return `project=${encodeURIComponent(scope.name)}`;
    if (scope.kind === 'goal') return `goal_id=${encodeURIComponent(scope.id)}`;
    if (scope.kind === 'venture') return `venture=${encodeURIComponent(scope.name)}`;
    return '';
  });
  // The `new=1` param on /deadlines opens the create drawer with the
  // scope pre-filled. The deadlines page hydrates from URL on mount.
  let addHref = $derived(`/deadlines?${scopeQuery}&new=1`);
  let viewAllHref = $derived(`/deadlines?${scopeQuery}`);
</script>

{#if loaded}
  {#if active.length > 0 || recentlyMet.length > 0}
    <section>
      <div class="flex items-baseline justify-between mb-2">
        <h3 class="text-xs uppercase tracking-wider text-dim font-medium">
          Deadlines · {active.length}{#if recentlyMet.length > 0}<span class="text-success/70 ml-1">+ {recentlyMet.length} met</span>{/if}
        </h3>
        <div class="flex items-center gap-2 text-xs">
          <a href={addHref} class="text-secondary hover:underline" title="add a deadline for this">+ add</a>
          {#if active.length > 0 || recentlyMet.length > 0}
            <a href={viewAllHref} class="text-dim hover:text-text">all →</a>
          {/if}
        </div>
      </div>
      {#if active.length > 0}
        <ul class="space-y-1">
          {#each active.slice(0, max) as d (d.id)}
            {@const days = daysUntil(d.date)}
            {@const t = tone(days)}
            <li>
              <a
                href={`/deadlines?${scopeQuery}#${d.id}`}
                class="flex items-baseline gap-2 px-2.5 py-1.5 rounded hover:bg-surface0 group"
                style="border-left: 2px solid var(--color-{t});"
              >
                <span class="text-sm text-text flex-1 truncate group-hover:text-primary">{d.title}</span>
                {#if d.importance === 'critical'}
                  <span class="text-[9px] uppercase tracking-wider px-1 py-0.5 rounded bg-surface0 text-error flex-shrink-0">crit</span>
                {:else if d.importance === 'high'}
                  <span class="text-[9px] uppercase tracking-wider px-1 py-0.5 rounded bg-surface0 text-warning flex-shrink-0">high</span>
                {/if}
                <span class="text-xs tabular-nums flex-shrink-0" style="color: var(--color-{t});">{countdown(days)}</span>
              </a>
            </li>
          {/each}
        </ul>
        {#if active.length > max}
          <a href={viewAllHref} class="block mt-1 px-2.5 py-1 text-[11px] text-dim hover:text-text">+{active.length - max} more</a>
        {/if}
      {/if}
      {#if recentlyMet.length > 0}
        <ul class="space-y-0.5 mt-2 pl-2 border-l border-success">
          {#each recentlyMet as d (d.id)}
            <li class="text-[11px] text-success/80 truncate">✓ {d.title}</li>
          {/each}
        </ul>
      {/if}
    </section>
  {:else}
    <section>
      <div class="flex items-baseline justify-between mb-1.5">
        <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Deadlines</h3>
        <a href={addHref} class="text-xs text-secondary hover:underline">+ add</a>
      </div>
      <p class="text-xs text-dim italic px-2.5">No deadlines yet.</p>
    </section>
  {/if}
{/if}
