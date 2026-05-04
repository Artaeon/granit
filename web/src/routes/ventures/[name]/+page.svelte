<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import {
    api,
    type Venture,
    type Project,
    type Goal,
    type Deadline,
    type PrayerIntention
  } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import { daysUntil } from '$lib/deadlines/util';

  // Venture detail page — single aggregation view answering "what's
  // the current state of this venture, all in one place?". The
  // /ventures list page is the catalogue; this is the detail.
  //
  // Aggregation is client-side (multiple parallel API calls + filter)
  // because:
  //   - all four sources are already loaded for other pages, so the
  //     browser usually serves them from cache;
  //   - a server-side rollup endpoint would be one more thing to keep
  //     in sync with the underlying schemas;
  //   - the cost is one extra network round-trip vs. four — not the
  //     bottleneck for the data sizes we're dealing with.
  // If perf becomes a problem we can collapse to a /ventures/{name}
  // server-decorated response without touching this page's render
  // logic.

  let venture = $state<Venture | null>(null);
  let projects = $state<Project[]>([]);
  let goals = $state<Goal[]>([]);
  let deadlines = $state<Deadline[]>([]);
  let intentions = $state<PrayerIntention[]>([]);
  let loading = $state(false);
  let notFound = $state(false);

  // The route param is the raw venture name (URL-encoded). We
  // case-insensitive-match on the server (ventures.Find) but keep
  // the user's original casing in the displayed name; the lookup
  // here is exact-equality on the decoded segment.
  let name = $derived(decodeURIComponent($page.params.name ?? ''));

  async function load() {
    if (!name) return;
    loading = true;
    notFound = false;
    try {
      // Fetch venture + the four linked-entity lists in parallel.
      // Each list endpoint we already use elsewhere, so the browser
      // typically has the response in cache. Promise.allSettled so
      // a single failing module (deadlines disabled, prayer disabled)
      // doesn't take the whole page down.
      const [vRes, pRes, gRes, dRes, iRes] = await Promise.allSettled([
        api.getVenture(name),
        api.listProjects(),
        api.listGoals(),
        api.tryListDeadlines(),
        api.listPrayer()
      ]);

      if (vRes.status !== 'fulfilled') {
        notFound = true;
        return;
      }
      venture = vRes.value;

      // Filter each list by venture (case-insensitive — the server
      // does the same on Find, so a project tagged with lowercase
      // venture name still rolls up to the canonical record).
      const lowerName = name.toLowerCase();
      const allProjects = pRes.status === 'fulfilled' ? pRes.value.projects : [];
      const allGoals = gRes.status === 'fulfilled' ? gRes.value.goals : [];
      const allDeadlines = dRes.status === 'fulfilled' ? (dRes.value ?? []) : [];
      const allIntentions = iRes.status === 'fulfilled' ? iRes.value.intentions : [];

      projects = allProjects.filter((p) => (p.venture ?? '').toLowerCase() === lowerName);
      goals = allGoals.filter((g) => (g.venture ?? '').toLowerCase() === lowerName);
      deadlines = allDeadlines.filter((d) => (d.venture ?? '').toLowerCase() === lowerName);
      intentions = allIntentions.filter((p) => (p.venture ?? '').toLowerCase() === lowerName);
    } catch (e) {
      toast.error('failed to load venture: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    const unsub = onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/ventures.json') load();
      if (ev.type.startsWith('project.')) load();
      if (ev.type === 'state.changed' && ev.path === '.granit/goals.json') load();
      if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') load();
      if (ev.type === 'state.changed' && ev.path === '.granit/prayer/intentions.json') load();
    });
    const onVisible = () => {
      if (document.visibilityState === 'visible') load();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', onVisible);
    return () => {
      unsub();
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', onVisible);
    };
  });

  // Re-fetch when the URL param changes (e.g. user navigates from one
  // venture to another).
  $effect(() => {
    void name;
    load();
  });

  // ----- Derived rollups -----

  let activeProjects = $derived(projects.filter((p) => (p.status ?? 'active') === 'active'));
  let pausedProjects = $derived(projects.filter((p) => p.status === 'paused'));
  let activeGoals = $derived(goals.filter((g) => (g.status ?? 'active') === 'active'));
  let activeDeadlines = $derived(
    deadlines.filter((d) => d.status !== 'met' && d.status !== 'cancelled')
  );
  let activeIntentions = $derived(intentions.filter((p) => p.status === 'praying'));

  // Aggregate task counts across the venture's projects — a single
  // "23 open · 7 done" line at the top of the page is the fastest
  // read on overall momentum.
  let aggregateTasksOpen = $derived(
    projects.reduce((acc, p) => acc + ((p.tasksTotal ?? 0) - (p.tasksDone ?? 0)), 0)
  );
  let aggregateTasksDone = $derived(projects.reduce((acc, p) => acc + (p.tasksDone ?? 0), 0));

  // Average progress across active projects (each project's progress
  // is already derived server-side from its goals + tasks).
  let aggregateProgress = $derived.by(() => {
    if (activeProjects.length === 0) return 0;
    const sum = activeProjects.reduce((acc, p) => acc + (p.progress ?? 0), 0);
    return sum / activeProjects.length;
  });

  function colorVar(c?: string): string {
    const map: Record<string, string> = {
      red: 'error', yellow: 'warning', orange: 'accent', green: 'success',
      blue: 'secondary', purple: 'primary', cyan: 'info', mauve: 'primary',
      peach: 'accent', teal: 'info', sapphire: 'secondary', pink: 'accent',
      lavender: 'primary', flamingo: 'error'
    };
    return `var(--color-${map[c ?? ''] ?? 'secondary'})`;
  }

  function statusTone(s?: string): string {
    if (s === 'active') return 'success';
    if (s === 'paused') return 'warning';
    if (s === 'completed') return 'info';
    if (s === 'archived') return 'subtext';
    return 'subtext';
  }

  // Deadline countdown — short-form for the sidebar list. Mirrors the
  // /deadlines page formatter so the language is consistent across surfaces.
  function countdown(d: Deadline): string {
    if (d.status === 'met') return 'met';
    if (d.status === 'cancelled') return 'cancelled';
    const n = daysUntil(d.date);
    if (n === 0) return 'today';
    if (n === 1) return 'tomorrow';
    if (n === -1) return 'yesterday';
    if (n > 1) return `in ${n}d`;
    return `${-n}d ago`;
  }
  function deadlineTone(d: Deadline): string {
    if (d.status === 'met') return 'success';
    if (d.status === 'cancelled') return 'subtext';
    const n = daysUntil(d.date);
    if (n < 0) return 'error';
    if (n <= 3) return 'error';
    if (n <= 7) return 'warning';
    if (n <= 30) return 'info';
    return 'subtext';
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-5xl mx-auto">
    {#if loading && !venture}
      <div class="text-sm text-dim">loading…</div>
    {:else if notFound}
      <div class="bg-surface0 border border-error/30 rounded-lg p-6 text-center">
        <p class="text-sm text-text mb-2">No venture named <strong>{name}</strong> found.</p>
        <a href="/ventures" class="text-sm text-secondary hover:underline">← back to ventures</a>
      </div>
    {:else if venture}
      <!-- Header / hero -->
      <header class="mb-6 flex items-start gap-3">
        <a
          href="/ventures"
          aria-label="back to ventures"
          class="flex-shrink-0 w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded -ml-1"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </a>
        <span
          class="w-3 h-3 rounded-full flex-shrink-0 mt-3"
          style="background: {colorVar(venture.color)}"
        ></span>
        <div class="flex-1 min-w-0">
          <h1 class="text-2xl sm:text-3xl font-semibold text-text break-words">{venture.name}</h1>
          {#if venture.mission}
            <p class="text-sm sm:text-base text-subtext italic mt-1 break-words">{venture.mission}</p>
          {/if}
          {#if venture.description}
            <p class="text-sm text-subtext mt-2 break-words">{venture.description}</p>
          {/if}
          <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-dim mt-3">
            <span
              class="px-2 py-0.5 rounded uppercase tracking-wider text-[10px]"
              style="background: color-mix(in srgb, var(--color-{statusTone(venture.status)}) 14%, transparent); color: var(--color-{statusTone(venture.status)});"
            >{venture.status ?? 'active'}</span>
            {#if venture.url}
              <a
                href={venture.url}
                target="_blank"
                rel="noopener noreferrer"
                class="text-secondary hover:underline truncate font-mono text-[11px]"
              >↗ {venture.url.replace(/^https?:\/\//, '').replace(/\/$/, '')}</a>
            {/if}
            {#if venture.tags && venture.tags.length > 0}
              <span class="flex items-center gap-1">
                {#each venture.tags as t}
                  <span class="text-[10px] px-1.5 py-0.5 bg-surface1 text-subtext rounded">#{t}</span>
                {/each}
              </span>
            {/if}
          </div>
        </div>
      </header>

      <!-- Aggregate row — at-a-glance momentum signal. Active goals
           count, active deadlines, prayer count, project task rollup. -->
      <section class="grid grid-cols-2 sm:grid-cols-4 gap-2 mb-6">
        <a
          href={`/projects?venture=${encodeURIComponent(venture.name)}`}
          class="block px-3 py-3 bg-surface0 border border-surface1 rounded hover:border-primary/40 transition-colors"
        >
          <div class="text-2xl font-semibold text-text tabular-nums leading-none">{activeProjects.length}</div>
          <div class="text-[11px] text-dim mt-1">Active projects</div>
        </a>
        <a
          href="/goals"
          class="block px-3 py-3 bg-surface0 border border-surface1 rounded hover:border-primary/40 transition-colors"
        >
          <div class="text-2xl font-semibold text-text tabular-nums leading-none">{activeGoals.length}</div>
          <div class="text-[11px] text-dim mt-1">Active goals</div>
        </a>
        <a
          href={`/deadlines?venture=${encodeURIComponent(venture.name)}`}
          class="block px-3 py-3 bg-surface0 border border-surface1 rounded hover:border-primary/40 transition-colors"
        >
          <div
            class="text-2xl font-semibold tabular-nums leading-none"
            style="color: {activeDeadlines.length > 0 ? 'var(--color-warning)' : 'var(--color-text)'};"
          >{activeDeadlines.length}</div>
          <div class="text-[11px] text-dim mt-1">Open deadlines</div>
        </a>
        <a
          href={`/prayer?venture=${encodeURIComponent(venture.name)}`}
          class="block px-3 py-3 bg-surface0 border border-surface1 rounded hover:border-primary/40 transition-colors"
        >
          <div
            class="text-2xl font-semibold tabular-nums leading-none"
            style="color: {activeIntentions.length > 0 ? 'var(--color-secondary)' : 'var(--color-text)'};"
          >{activeIntentions.length}</div>
          <div class="text-[11px] text-dim mt-1">Praying for</div>
        </a>
      </section>

      <!-- Progress bar — averaged across active projects. Single
           anchor for "how are we doing on this venture". -->
      {#if activeProjects.length > 0}
        <section class="mb-6">
          <div class="flex items-baseline justify-between mb-1.5">
            <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Overall progress</h2>
            <span class="text-xs text-subtext font-mono">
              {Math.round(aggregateProgress * 100)}%
              {#if aggregateTasksOpen + aggregateTasksDone > 0}
                · <span class="text-dim">{aggregateTasksDone}/{aggregateTasksDone + aggregateTasksOpen} tasks</span>
              {/if}
            </span>
          </div>
          <div class="h-2 rounded-full bg-surface0 overflow-hidden">
            <div
              class="h-full transition-all"
              style="width: {Math.round(aggregateProgress * 100)}%; background: {colorVar(venture.color)}"
            ></div>
          </div>
        </section>
      {/if}

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <!-- Projects -->
        <section>
          <div class="flex items-baseline justify-between mb-2">
            <h2 class="text-sm font-medium text-text">Projects · {activeProjects.length}</h2>
            <a
              href={`/projects?venture=${encodeURIComponent(venture.name)}`}
              class="text-xs text-secondary hover:underline"
            >open list →</a>
          </div>
          {#if activeProjects.length === 0 && pausedProjects.length === 0}
            <p class="text-xs text-dim italic px-2.5">No projects linked yet.</p>
          {:else}
            <ul class="space-y-1.5">
              {#each activeProjects as p (p.name)}
                {@const progress = Math.round((p.progress ?? 0) * 100)}
                <li>
                  <a
                    href={`/projects?p=${encodeURIComponent(p.name)}`}
                    class="block px-3 py-2 bg-surface0 border border-surface1 rounded hover:border-primary/40 transition-colors group"
                  >
                    <div class="flex items-baseline gap-2">
                      <span class="w-2 h-2 rounded-full flex-shrink-0" style="background: {colorVar(p.color)}"></span>
                      <span class="text-sm text-text flex-1 min-w-0 truncate group-hover:text-primary">{p.name}</span>
                      {#if p.kind}
                        <span class="text-[9px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-primary/10 text-primary flex-shrink-0">{p.kind}</span>
                      {/if}
                    </div>
                    {#if p.description}
                      <p class="text-xs text-subtext line-clamp-1 mt-0.5">{p.description}</p>
                    {/if}
                    <div class="flex items-center gap-2 mt-1.5">
                      <div class="flex-1 h-1 rounded-full bg-mantle overflow-hidden">
                        <div class="h-full" style="width: {progress}%; background: {colorVar(p.color)}"></div>
                      </div>
                      <span class="text-[10px] text-dim font-mono w-9 text-right">{progress}%</span>
                    </div>
                  </a>
                </li>
              {/each}
              {#if pausedProjects.length > 0}
                <li class="text-[11px] text-dim italic px-2 pt-1">
                  + {pausedProjects.length} paused
                </li>
              {/if}
            </ul>
          {/if}
        </section>

        <!-- Goals -->
        <section>
          <div class="flex items-baseline justify-between mb-2">
            <h2 class="text-sm font-medium text-text">Goals · {activeGoals.length}</h2>
            <a href="/goals" class="text-xs text-secondary hover:underline">open list →</a>
          </div>
          {#if activeGoals.length === 0}
            <p class="text-xs text-dim italic px-2.5">No goals linked yet.</p>
          {:else}
            <ul class="space-y-1.5">
              {#each activeGoals as g (g.id)}
                {@const ms = g.milestones ?? []}
                {@const total = ms.length}
                {@const done = ms.filter((m) => m.done).length}
                {@const pct = total === 0 ? (g.status === 'completed' ? 100 : 0) : Math.round((done / total) * 100)}
                <li>
                  <a
                    href={`/goals?focus=${encodeURIComponent(g.id)}`}
                    class="block px-3 py-2 bg-surface0 border border-surface1 rounded hover:border-primary/40 transition-colors group"
                  >
                    <div class="flex items-baseline gap-2">
                      <span class="text-sm text-text flex-1 min-w-0 truncate group-hover:text-primary">{g.title}</span>
                      {#if g.target_date}
                        <span class="text-[10px] text-dim font-mono flex-shrink-0">🎯 {g.target_date}</span>
                      {/if}
                    </div>
                    {#if total > 0}
                      <div class="flex items-center gap-2 mt-1.5">
                        <div class="flex-1 h-1 rounded-full bg-mantle overflow-hidden">
                          <div class="h-full bg-primary" style="width: {pct}%"></div>
                        </div>
                        <span class="text-[10px] text-dim font-mono">{done}/{total}</span>
                      </div>
                    {/if}
                  </a>
                </li>
              {/each}
            </ul>
          {/if}
        </section>

        <!-- Deadlines -->
        <section>
          <div class="flex items-baseline justify-between mb-2">
            <h2 class="text-sm font-medium text-text">Deadlines · {activeDeadlines.length}</h2>
            <div class="flex items-center gap-2 text-xs">
              <a
                href={`/deadlines?venture=${encodeURIComponent(venture.name)}&new=1`}
                class="text-secondary hover:underline"
              >+ add</a>
              <a
                href={`/deadlines?venture=${encodeURIComponent(venture.name)}`}
                class="text-dim hover:text-text"
              >all →</a>
            </div>
          </div>
          {#if activeDeadlines.length === 0}
            <p class="text-xs text-dim italic px-2.5">No active deadlines.</p>
          {:else}
            <ul class="space-y-1">
              {#each activeDeadlines.slice(0, 6) as d (d.id)}
                {@const tone = deadlineTone(d)}
                <li>
                  <a
                    href={`/deadlines?venture=${encodeURIComponent(venture.name)}#${d.id}`}
                    class="flex items-baseline gap-2 px-2.5 py-1.5 rounded hover:bg-surface0 group"
                    style="border-left: 2px solid var(--color-{tone});"
                  >
                    <span class="text-sm text-text flex-1 truncate group-hover:text-primary">{d.title}</span>
                    {#if d.importance === 'critical'}
                      <span class="text-[9px] uppercase tracking-wider px-1 py-0.5 rounded bg-error/15 text-error flex-shrink-0">crit</span>
                    {:else if d.importance === 'high'}
                      <span class="text-[9px] uppercase tracking-wider px-1 py-0.5 rounded bg-warning/15 text-warning flex-shrink-0">high</span>
                    {/if}
                    <span class="text-xs tabular-nums flex-shrink-0" style="color: var(--color-{tone});">{countdown(d)}</span>
                  </a>
                </li>
              {/each}
              {#if activeDeadlines.length > 6}
                <li>
                  <a
                    href={`/deadlines?venture=${encodeURIComponent(venture.name)}`}
                    class="block px-2.5 py-1 text-[11px] text-dim hover:text-text"
                  >+ {activeDeadlines.length - 6} more</a>
                </li>
              {/if}
            </ul>
          {/if}
        </section>

        <!-- Prayer intentions -->
        <section>
          <div class="flex items-baseline justify-between mb-2">
            <h2 class="text-sm font-medium text-text">Praying for · {activeIntentions.length}</h2>
            <div class="flex items-center gap-2 text-xs">
              <a
                href={`/prayer?venture=${encodeURIComponent(venture.name)}`}
                class="text-secondary hover:underline"
              >+ add</a>
              <a
                href={`/prayer?venture=${encodeURIComponent(venture.name)}`}
                class="text-dim hover:text-text"
              >all →</a>
            </div>
          </div>
          {#if activeIntentions.length === 0}
            <p class="text-xs text-dim italic px-2.5">
              <a href={`/prayer?venture=${encodeURIComponent(venture.name)}`} class="text-secondary hover:underline">Bring it before God</a> — what are you asking Him for in this venture?
            </p>
          {:else}
            <ul class="space-y-1.5">
              {#each activeIntentions.slice(0, 6) as p (p.id)}
                <li class="px-2.5 py-1.5 bg-surface0 rounded">
                  <div class="text-sm text-text break-words">{p.text}</div>
                  {#if p.passage_ref || p.started_at}
                    <div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 mt-0.5 text-[11px] text-dim">
                      {#if p.passage_ref}<span>📖 {p.passage_ref}</span>{/if}
                      {#if p.started_at}<span>since {p.started_at}</span>{/if}
                    </div>
                  {/if}
                </li>
              {/each}
              {#if activeIntentions.length > 6}
                <li class="text-[11px] text-dim px-2 italic">+ {activeIntentions.length - 6} more</li>
              {/if}
            </ul>
          {/if}
        </section>
      </div>
    {/if}
  </div>
</div>
