<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type Venture } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';

  // /ventures is the umbrella view above projects + goals. Project.venture
  // and Goal.venture stay free-text strings — a venture record adds
  // description / mission / color on top so a "what's this venture about?"
  // panel can lead with purpose, not metadata.
  //
  // The list page offers three view modes:
  //   - cards: grouped by status, status sections collapsible (default)
  //   - table: dense tabular layout with sortable columns
  //   - kanban: status-as-columns, drag-free (status change via menu)
  // The choice persists to localStorage so navigation back to the page
  // restores the user's preferred shape.

  type ViewMode = 'cards' | 'table' | 'kanban';
  const VIEW_KEY = 'granit.ventures.view';

  let ventures = $state<Venture[]>([]);
  let loading = $state(false);
  let q = $state('');
  let statusFilter = $state<'all' | 'active' | 'paused' | 'archived'>('active');
  let view = $state<ViewMode>(loadStoredString(VIEW_KEY, 'cards') as ViewMode);
  $effect(() => saveStoredString(VIEW_KEY, view));

  let createOpen = $state(false);

  // Create-form state. Mission is the headline differentiator from
  // a plain Project: a venture exists because of a *why*, and the
  // create form leads with that.
  let nName = $state('');
  let nDescription = $state('');
  let nMission = $state('');
  let nColor = $state('blue');
  let nUrl = $state('');
  let saving = $state(false);

  const colorOptions = ['blue', 'green', 'mauve', 'peach', 'red', 'yellow', 'pink', 'lavender', 'teal', 'sapphire'];

  function colorVar(c?: string): string {
    const map: Record<string, string> = {
      red: 'error', yellow: 'warning', orange: 'accent', green: 'success',
      blue: 'secondary', purple: 'primary', cyan: 'info', mauve: 'primary',
      peach: 'accent', teal: 'info', sapphire: 'secondary', pink: 'accent',
      lavender: 'primary', flamingo: 'error'
    };
    return `var(--color-${map[c ?? ''] ?? 'secondary'})`;
  }

  function statusTone(status?: string): string {
    if (status === 'active') return 'success';
    if (status === 'paused') return 'warning';
    if (status === 'archived') return 'subtext';
    return 'subtext';
  }

  // Status icon — purely decorative but adds scanability in dense views.
  function statusIcon(status?: string): string {
    if (status === 'active') return '●';
    if (status === 'paused') return '◐';
    if (status === 'archived') return '◯';
    return '●';
  }

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      const r = await api.listVentures();
      ventures = r.ventures;
    } catch (e) {
      toast.error('failed to load ventures: ' + (errorMessage(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    const unsub = onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/ventures.json') load();
      // project / goal counts on each card depend on those sidecars too,
      // so refresh when they move (cheap: same single endpoint).
      if (ev.type.startsWith('project.')) load();
      if (ev.type === 'state.changed' && ev.path === '.granit/goals.json') load();
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

  let filtered = $derived.by(() => {
    let list = ventures;
    if (statusFilter !== 'all') list = list.filter((v) => (v.status ?? 'active') === statusFilter);
    const term = q.trim().toLowerCase();
    if (term) {
      list = list.filter((v) =>
        v.name.toLowerCase().includes(term) ||
        (v.description ?? '').toLowerCase().includes(term) ||
        (v.mission ?? '').toLowerCase().includes(term) ||
        (v.tags ?? []).some((t) => t.toLowerCase().includes(term))
      );
    }
    return [...list].sort((a, b) => {
      const sa = a.status ?? 'active';
      const sb = b.status ?? 'active';
      if (sa !== sb) {
        const order = { active: 0, paused: 1, archived: 2 } as Record<string, number>;
        return (order[sa] ?? 9) - (order[sb] ?? 9);
      }
      return a.name.localeCompare(b.name);
    });
  });

  // Group filtered list by status for the cards/kanban views. Active
  // first because the most common scan is "what's in motion right now".
  // Empty buckets are kept in the result so kanban renders all columns
  // even when one is empty (it gets an explicit "empty" tile then).
  let grouped = $derived.by(() => {
    const buckets: Record<'active' | 'paused' | 'archived', Venture[]> = {
      active: [],
      paused: [],
      archived: []
    };
    for (const v of filtered) {
      const s = (v.status ?? 'active') as keyof typeof buckets;
      if (buckets[s]) buckets[s].push(v);
      else buckets.active.push(v);
    }
    return buckets;
  });

  function resetCreate() {
    nName = '';
    nDescription = '';
    nMission = '';
    nColor = 'blue';
    nUrl = '';
  }

  // ----- Stat strip -----
  // Compact roll-up of "what does the venture portfolio look like?"
  // Counts use the server-decorated project_count / goal_count fields
  // so we don't fan out to /projects + /goals just to render the
  // strip. Numbers reflect the visible (status-filtered) list rather
  // than every venture in the file — when the user is on the Active
  // tab the strip should answer "what's actually in motion".
  let stats = $derived.by(() => {
    let projectsTotal = 0;
    let goalsTotal = 0;
    for (const v of filtered) {
      projectsTotal += v.project_count ?? 0;
      goalsTotal += v.goal_count ?? 0;
    }
    return {
      ventures: filtered.length,
      projects: projectsTotal,
      goals: goalsTotal
    };
  });

  // Per-status counts for the filter tab badges (always computed off
  // the unfiltered set so the user can see what's behind each tab
  // without flipping between them).
  let statusCounts = $derived.by(() => {
    const c = { all: ventures.length, active: 0, paused: 0, archived: 0 };
    for (const v of ventures) {
      const s = (v.status ?? 'active') as 'active' | 'paused' | 'archived';
      if (s in c) c[s] += 1;
    }
    return c;
  });

  async function submitCreate(e?: SubmitEvent) {
    e?.preventDefault();
    if (!nName.trim()) return;
    saving = true;
    try {
      const v = await api.createVenture({
        name: nName.trim(),
        description: nDescription.trim() || undefined,
        mission: nMission.trim() || undefined,
        color: nColor,
        url: nUrl.trim() || undefined,
        status: 'active'
      });
      // Optimistic prepend so the new venture renders immediately.
      // load() reconciles with server-decorated counts on the next tick.
      if (!ventures.some((x) => x.name === v.name)) {
        ventures = [v, ...ventures];
      }
      resetCreate();
      createOpen = false;
      toast.success(`venture "${v.name}" created`);
      await load();
    } catch (err) {
      toast.error('create failed: ' + (errorMessage(err)));
    } finally {
      saving = false;
    }
  }

  // Inline status change from card / table / kanban tile menus. PATCHes
  // the venture on the server, then optimistically updates the local
  // list so the move feels instant. The WS event will reconcile.
  async function setStatus(v: Venture, status: 'active' | 'paused' | 'archived') {
    if ((v.status ?? 'active') === status) return;
    try {
      await api.patchVenture(v.name, { status });
      ventures = ventures.map((x) => (x.name === v.name ? { ...x, status } : x));
      toast.success(`${v.name} → ${status}`);
    } catch (err) {
      toast.error('update failed: ' + (errorMessage(err)));
    }
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-6xl mx-auto">
    <header class="mb-6 flex flex-col sm:flex-row sm:items-baseline sm:justify-between gap-2">
      <div>
        <h1 class="text-2xl sm:text-3xl font-semibold text-text">Ventures</h1>
        <p class="text-sm text-dim mt-1">
          {ventures.length} venture{ventures.length === 1 ? '' : 's'} · the umbrella above
          <a href="/projects" class="text-secondary hover:underline">projects</a> and
          <a href="/goals" class="text-secondary hover:underline">goals</a>
        </p>
      </div>
      <button
        onclick={() => (createOpen = true)}
        class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90 self-start"
      >+ New venture</button>
    </header>

    <!-- Stat strip — total projects + goals across the (filtered)
         venture set. Hidden until at least one venture is in scope so
         a fresh page stays empty-and-quiet. Reads server-decorated
         counts directly so no extra round-trip is needed. -->
    {#if stats.ventures > 0}
      <div class="flex flex-wrap items-baseline gap-x-4 gap-y-1 mb-4 text-xs">
        <span class="text-text font-medium tabular-nums">
          {stats.ventures} {stats.ventures === 1 ? 'venture' : 'ventures'}
        </span>
        {#if stats.projects > 0}
          <span class="text-secondary tabular-nums">{stats.projects} {stats.projects === 1 ? 'project' : 'projects'}</span>
        {/if}
        {#if stats.goals > 0}
          <span class="text-secondary tabular-nums">{stats.goals} {stats.goals === 1 ? 'goal' : 'goals'}</span>
        {/if}
      </div>
    {/if}

    <!-- Toolbar: status filter + search + view toggle. The view toggle
         is rightmost so it stays out of the user's primary scan path
         (filter/search) but is one tap away when they want to swap
         shape. Hidden on extra-small screens where kanban/table don't
         work well; cards is the only option there. -->
    <div class="flex flex-wrap items-center gap-2 mb-4">
      <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm">
        {#each ['all', 'active', 'paused', 'archived'] as s}
          {@const n = statusCounts[s as keyof typeof statusCounts]}
          <button
            class="px-3 py-1.5 capitalize flex items-center gap-1.5 {statusFilter === s ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (statusFilter = s as typeof statusFilter)}
          >
            <span>{s}</span>
            {#if n > 0}
              <span class="text-[10px] tabular-nums opacity-70">{n}</span>
            {/if}
          </button>
        {/each}
      </div>
      <input
        bind:value={q}
        placeholder="search name, mission, tags…"
        class="flex-1 min-w-0 px-3 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      <div class="hidden sm:flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm" role="tablist" aria-label="View mode">
        {#each [['cards', 'Cards'], ['table', 'Table'], ['kanban', 'Kanban']] as [m, label]}
          <button
            role="tab"
            aria-selected={view === m}
            class="px-3 py-1.5 {view === m ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (view = m as ViewMode)}
            title="{label} view"
          >{label}</button>
        {/each}
      </div>
    </div>

    {#if loading && ventures.length === 0}
      <!-- Skeleton state — three placeholder cards roughly matching the
           real layout so the page doesn't reflow when data arrives. -->
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        {#each [0, 1, 2] as i (i)}
          <div class="bg-surface0 border border-surface1 rounded-lg overflow-hidden">
            <div class="h-1.5 bg-surface1"></div>
            <div class="p-4 space-y-2">
              <Skeleton class="h-5 w-2/3" />
              <Skeleton class="h-3 w-full" />
              <Skeleton class="h-3 w-4/5" />
              <div class="pt-2 flex gap-3">
                <Skeleton class="h-3 w-16" />
                <Skeleton class="h-3 w-16" />
              </div>
            </div>
          </div>
        {/each}
      </div>
    {:else if filtered.length === 0 && !q && statusFilter === 'active'}
      <EmptyState
        icon="🏢"
        title="No ventures yet"
        description="A venture is the umbrella above projects and goals — your company, side hustle, ministry, or research initiative. Create one and link projects/goals to it via their Venture field."
      />
    {:else if filtered.length === 0}
      <div class="bg-surface0 border border-surface1 rounded-lg p-8 text-center">
        <div class="text-3xl mb-2 opacity-60">🔍</div>
        <p class="text-sm text-text">No ventures match this filter.</p>
        {#if q}
          <button
            class="text-xs text-secondary hover:underline mt-2"
            onclick={() => (q = '')}
          >Clear search</button>
        {/if}
      </div>
    {:else if view === 'cards'}
      <!-- Cards view — grouped by status with section headers. The
           headers are decorative (filter tabs already do real filtering)
           but they preserve the visual rhythm of "active stuff first,
           paused below the fold, archived at the bottom" so the user's
           eye lands on what's in motion. Sections are skipped when
           empty (when statusFilter is 'active' only the active section
           renders, etc.). -->
      <div class="space-y-6">
        {#each ['active', 'paused', 'archived'] as status (status)}
          {@const list = grouped[status as keyof typeof grouped]}
          {#if list.length > 0}
            <section>
              {#if statusFilter === 'all'}
                <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-2 flex items-center gap-2">
                  <span style="color: var(--color-{statusTone(status)})">{statusIcon(status)}</span>
                  <span>{status}</span>
                  <span class="tabular-nums opacity-60">{list.length}</span>
                </h2>
              {/if}
              <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
                {#each list as v (v.name)}
                  {@const projectsCount = v.project_count ?? 0}
                  {@const goalsCount = v.goal_count ?? 0}
                  <article
                    class="bg-surface0 border border-surface1 rounded-lg overflow-hidden hover:border-primary/40 transition-colors flex flex-col"
                  >
                    <div class="h-1.5 flex-shrink-0" style="background: {colorVar(v.color)}"></div>
                    <div class="p-4 flex flex-col gap-2 flex-1">
                      <div class="flex items-start gap-2">
                        <a
                          href={`/ventures/${encodeURIComponent(v.name)}`}
                          class="text-base sm:text-lg font-semibold text-text flex-1 min-w-0 truncate hover:text-primary"
                        >{v.name}</a>
                        <span
                          class="text-[10px] uppercase tracking-wider flex-shrink-0 flex items-center gap-1 px-1.5 py-0.5 rounded"
                          style="background: color-mix(in srgb, var(--color-{statusTone(v.status)}) 12%, transparent); color: var(--color-{statusTone(v.status)});"
                        >
                          <span aria-hidden="true">{statusIcon(v.status)}</span>
                          <span>{v.status ?? 'active'}</span>
                        </span>
                      </div>
                      {#if v.mission}
                        <p class="text-xs text-subtext italic line-clamp-2">{v.mission}</p>
                      {/if}
                      {#if v.description}
                        <p class="text-sm text-subtext line-clamp-2">{v.description}</p>
                      {/if}
                      {#if v.tags && v.tags.length > 0}
                        <div class="flex flex-wrap gap-1">
                          {#each v.tags.slice(0, 4) as t}
                            <span class="text-[10px] px-1.5 py-0.5 bg-surface1 text-subtext rounded">#{t}</span>
                          {/each}
                          {#if v.tags.length > 4}
                            <span class="text-[10px] text-dim">+{v.tags.length - 4}</span>
                          {/if}
                        </div>
                      {/if}
                      <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-dim mt-auto pt-2">
                        <a
                          href={`/projects?venture=${encodeURIComponent(v.name)}`}
                          class="hover:text-primary"
                          title="show projects in this venture"
                        >📁 {projectsCount} project{projectsCount === 1 ? '' : 's'}</a>
                        <a
                          href="/goals"
                          class="hover:text-primary"
                          title="goals tracker"
                        >🎯 {goalsCount} goal{goalsCount === 1 ? '' : 's'}</a>
                        <a
                          href={`/deadlines?venture=${encodeURIComponent(v.name)}`}
                          class="hover:text-primary"
                          title="show deadlines for this venture"
                        >⏰ deadlines</a>
                        <a
                          href={`/prayer?venture=${encodeURIComponent(v.name)}`}
                          class="hover:text-primary"
                          title="add a prayer intention for this venture"
                        >🙏 pray</a>
                        {#if v.url}
                          <a
                            href={v.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            class="hover:text-primary truncate font-mono text-[11px]"
                          >↗ {v.url.replace(/^https?:\/\//, '').replace(/\/$/, '')}</a>
                        {/if}
                      </div>
                    </div>
                  </article>
                {/each}
              </div>
            </section>
          {/if}
        {/each}
      </div>
    {:else if view === 'table'}
      <!-- Table view — dense scan layout. Inline status change menu
           per row so the user can re-bucket without navigating to the
           detail page. Hides description/mission on narrow screens
           (the cards view exists for that). -->
      <div class="bg-surface0 border border-surface1 rounded-lg overflow-x-auto">
        <table class="w-full text-sm">
          <thead class="bg-surface1/50 text-xs uppercase tracking-wider text-dim">
            <tr>
              <th class="text-left font-medium px-4 py-2">Name</th>
              <th class="text-left font-medium px-2 py-2 hidden md:table-cell">Mission</th>
              <th class="text-left font-medium px-2 py-2">Status</th>
              <th class="text-right font-medium px-2 py-2 tabular-nums">Projects</th>
              <th class="text-right font-medium px-2 py-2 tabular-nums">Goals</th>
              <th class="text-left font-medium px-2 py-2 hidden lg:table-cell">URL</th>
            </tr>
          </thead>
          <tbody>
            {#each filtered as v (v.name)}
              <tr class="border-t border-surface1 hover:bg-surface1/30">
                <td class="px-4 py-2">
                  <a
                    href={`/ventures/${encodeURIComponent(v.name)}`}
                    class="flex items-center gap-2 text-text hover:text-primary font-medium"
                  >
                    <span class="w-2 h-2 rounded-full flex-shrink-0" style="background: {colorVar(v.color)}"></span>
                    <span class="truncate">{v.name}</span>
                  </a>
                </td>
                <td class="px-2 py-2 hidden md:table-cell text-subtext italic max-w-xs">
                  <span class="line-clamp-1">{v.mission ?? v.description ?? ''}</span>
                </td>
                <td class="px-2 py-2">
                  <select
                    value={v.status ?? 'active'}
                    onchange={(e) => setStatus(v, (e.currentTarget as HTMLSelectElement).value as 'active' | 'paused' | 'archived')}
                    class="text-[11px] uppercase tracking-wider bg-transparent border border-surface1 rounded px-1.5 py-0.5 text-text focus:outline-none focus:border-primary"
                    style="color: var(--color-{statusTone(v.status)});"
                    aria-label="Change status for {v.name}"
                  >
                    <option value="active">active</option>
                    <option value="paused">paused</option>
                    <option value="archived">archived</option>
                  </select>
                </td>
                <td class="px-2 py-2 text-right tabular-nums">
                  <a
                    href={`/projects?venture=${encodeURIComponent(v.name)}`}
                    class="text-secondary hover:underline"
                  >{v.project_count ?? 0}</a>
                </td>
                <td class="px-2 py-2 text-right tabular-nums">
                  <span class="text-text">{v.goal_count ?? 0}</span>
                </td>
                <td class="px-2 py-2 hidden lg:table-cell">
                  {#if v.url}
                    <a
                      href={v.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      class="text-secondary hover:underline truncate font-mono text-[11px]"
                    >↗ {v.url.replace(/^https?:\/\//, '').replace(/\/$/, '')}</a>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <!-- Kanban view — three columns by status. Useful for "I want to
           drag this venture from active to paused" mental moves; the
           drag itself is keyboard-only via the per-tile select. Each
           column is independently scrollable on small viewports. -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        {#each ['active', 'paused', 'archived'] as status (status)}
          {@const list = grouped[status as keyof typeof grouped]}
          <section
            class="bg-surface0 border border-surface1 rounded-lg p-3 flex flex-col"
            aria-label="{status} ventures"
          >
            <header class="flex items-baseline justify-between mb-2 px-1">
              <h2 class="text-xs uppercase tracking-wider font-medium flex items-center gap-1.5">
                <span style="color: var(--color-{statusTone(status)})">{statusIcon(status)}</span>
                <span class="text-text">{status}</span>
              </h2>
              <span class="text-[11px] text-dim tabular-nums">{list.length}</span>
            </header>
            <div class="space-y-2 flex-1 min-h-[4rem]">
              {#if list.length === 0}
                <p class="text-xs text-dim italic text-center py-4">empty</p>
              {:else}
                {#each list as v (v.name)}
                  <article class="bg-mantle border border-surface1 rounded p-2.5 hover:border-primary/40 transition-colors">
                    <div class="flex items-start gap-2">
                      <span class="w-2 h-2 rounded-full flex-shrink-0 mt-1.5" style="background: {colorVar(v.color)}"></span>
                      <a
                        href={`/ventures/${encodeURIComponent(v.name)}`}
                        class="text-sm font-medium text-text hover:text-primary flex-1 min-w-0 truncate"
                      >{v.name}</a>
                    </div>
                    {#if v.mission}
                      <p class="text-[11px] text-subtext italic line-clamp-2 mt-1 ml-4">{v.mission}</p>
                    {/if}
                    <div class="flex items-center gap-2 mt-2 ml-4 text-[11px] text-dim">
                      {#if (v.project_count ?? 0) > 0}
                        <span title="projects">📁 {v.project_count}</span>
                      {/if}
                      {#if (v.goal_count ?? 0) > 0}
                        <span title="goals">🎯 {v.goal_count}</span>
                      {/if}
                      <span class="flex-1"></span>
                      <select
                        value={v.status ?? 'active'}
                        onchange={(e) => setStatus(v, (e.currentTarget as HTMLSelectElement).value as 'active' | 'paused' | 'archived')}
                        class="text-[10px] bg-transparent border border-surface1 rounded px-1 py-0.5 text-dim focus:outline-none focus:border-primary"
                        aria-label="Move {v.name} to another status"
                      >
                        <option value="active">→ active</option>
                        <option value="paused">→ paused</option>
                        <option value="archived">→ archived</option>
                      </select>
                    </div>
                  </article>
                {/each}
              {/if}
            </div>
          </section>
        {/each}
      </div>
    {/if}
  </div>
</div>

<!-- Create modal — kept inline because we don't have many fields to
     justify a separate component yet. If editing surfaces grow we'll
     extract a VentureCreate.svelte sibling to ProjectCreate. -->
{#if createOpen}
  <div
    class="fixed inset-0 z-50 bg-black/40 flex items-end sm:items-center justify-center sm:p-4"
    role="dialog"
    tabindex="-1"
    onclick={() => (createOpen = false)}
    onkeydown={(e) => { if (e.key === 'Escape') createOpen = false; }}
  >
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
      class="w-full max-w-md bg-mantle border border-surface1 rounded-t-lg sm:rounded-lg shadow-xl max-h-[90dvh] overflow-y-auto"
      role="document"
    >
      <header class="px-5 py-3 border-b border-surface1 flex items-center gap-2">
        <h2 class="text-base font-semibold text-text flex-1">New venture</h2>
        <button onclick={() => (createOpen = false)} aria-label="close" class="text-dim hover:text-text">×</button>
      </header>
      <form onsubmit={submitCreate} class="p-5 pb-[calc(1.25rem+env(safe-area-inset-bottom,0px))] sm:pb-5 space-y-3">
        <div>
          <label for="nv-name" class="text-xs uppercase tracking-wider text-dim block mb-1">Name</label>
          <input
            id="nv-name"
            bind:value={nName}
            required
            autofocus
            placeholder="e.g. Stoicera"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
          />
          <p class="text-[11px] text-dim mt-1">This is the string projects/goals reference via their Venture field.</p>
        </div>

        <div>
          <label for="nv-mission" class="text-xs uppercase tracking-wider text-dim block mb-1">Mission</label>
          <textarea
            id="nv-mission"
            bind:value={nMission}
            rows="2"
            placeholder="Why this venture exists — 1-3 sentences."
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
          ></textarea>
        </div>

        <div>
          <label for="nv-desc" class="text-xs uppercase tracking-wider text-dim block mb-1">Description</label>
          <textarea
            id="nv-desc"
            bind:value={nDescription}
            rows="2"
            placeholder="What it does, who it serves."
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
          ></textarea>
        </div>

        <div>
          <label for="nv-url" class="text-xs uppercase tracking-wider text-dim block mb-1">Homepage / URL (optional)</label>
          <input
            id="nv-url"
            type="url"
            bind:value={nUrl}
            placeholder="https://acme.com"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text font-mono focus:outline-none focus:border-primary"
          />
        </div>

        <div>
          <span class="text-xs uppercase tracking-wider text-dim block mb-1">Color</span>
          <div class="flex flex-wrap gap-2">
            {#each colorOptions as c}
              <button
                type="button"
                onclick={() => (nColor = c)}
                aria-label="color {c}"
                class="w-7 h-7 rounded-full border-2 {nColor === c ? 'border-text' : 'border-surface1'}"
                style="background: {colorVar(c)}"
              ></button>
            {/each}
          </div>
        </div>

        <button
          type="submit"
          disabled={!nName.trim() || saving}
          class="w-full px-4 py-2.5 bg-primary text-on-primary rounded font-medium disabled:opacity-50"
        >{saving ? 'creating…' : 'Create venture'}</button>
      </form>
    </div>
  </div>
{/if}
