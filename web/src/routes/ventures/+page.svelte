<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type Venture } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import EmptyState from '$lib/components/EmptyState.svelte';

  // /ventures is the umbrella view above projects + goals. Project.venture
  // and Goal.venture stay free-text strings — a venture record adds
  // description / mission / color on top so a "what's this venture about?"
  // panel can lead with purpose, not metadata.

  let ventures = $state<Venture[]>([]);
  let loading = $state(false);
  let q = $state('');
  let statusFilter = $state<'all' | 'active' | 'paused' | 'archived'>('active');
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

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      const r = await api.listVentures();
      ventures = r.ventures;
    } catch (e) {
      toast.error('failed to load ventures: ' + (e instanceof Error ? e.message : String(e)));
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
      toast.error('create failed: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      saving = false;
    }
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-5xl mx-auto">
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

    <!-- Status tabs + search -->
    <div class="flex flex-wrap items-center gap-2 mb-4">
      <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm">
        {#each ['all', 'active', 'paused', 'archived'] as s}
          <button
            class="px-3 py-1.5 capitalize {statusFilter === s ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (statusFilter = s as typeof statusFilter)}
          >{s}</button>
        {/each}
      </div>
      <input
        bind:value={q}
        placeholder="search name, mission, tags…"
        class="flex-1 min-w-0 px-3 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
      />
    </div>

    {#if loading && ventures.length === 0}
      <div class="text-sm text-dim">loading…</div>
    {:else if filtered.length === 0 && !q && statusFilter === 'active'}
      <EmptyState
        icon="🏢"
        title="No ventures yet"
        description="A venture is the umbrella above projects and goals — your company, side hustle, ministry, or research initiative. Create one and link projects/goals to it via their Venture field."
      />
    {:else if filtered.length === 0}
      <div class="text-sm text-dim italic">no ventures match this filter.</div>
    {:else}
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        {#each filtered as v (v.name)}
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
                  class="text-[10px] uppercase tracking-wider flex-shrink-0"
                  style="color: var(--color-{statusTone(v.status)})"
                >{v.status ?? 'active'}</span>
              </div>
              {#if v.mission}
                <p class="text-xs text-subtext italic line-clamp-2">{v.mission}</p>
              {/if}
              {#if v.description}
                <p class="text-sm text-subtext line-clamp-2">{v.description}</p>
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
            placeholder="https://stoicera.com"
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
