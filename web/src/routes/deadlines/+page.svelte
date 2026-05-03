<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { auth } from '$lib/stores/auth';
  import {
    api,
    type Deadline,
    type DeadlineCreate,
    type DeadlineImportance,
    type DeadlineStatus,
    type Goal,
    type Project,
    type Task
  } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { onWsEvent } from '$lib/ws';
  import Drawer from '$lib/components/Drawer.svelte';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import VisionContextStrip from '$lib/components/VisionContextStrip.svelte';
  import DeadlinePill from '$lib/deadlines/DeadlinePill.svelte';
  import { daysUntil, pickHeroDeadline } from '$lib/deadlines/util';

  // Deadlines page — top-level "this matters by date X" markers backed
  // by .granit/deadlines.json. Distinct from Tasks (no checkbox / not
  // a todo) and from Goals (a goal has milestones / progress; a
  // deadline is a single hard moment in time, possibly linked to a
  // goal). The list is grouped by recency so the user can see at a
  // glance what's slipping vs. what's distant background context.

  let deadlines = $state<Deadline[]>([]);
  let goals = $state<Goal[]>([]);
  let projects = $state<Project[]>([]);
  // Open tasks pool used by the "link to tasks" multi-select. Loaded
  // lazily on drawer open so the page paints fast even on big vaults.
  let openTasks = $state<Task[]>([]);
  let tasksLoaded = $state(false);
  let loading = $state(false);
  let busy = $state(false);

  // Active importance filter — null = show all; otherwise filter to
  // the matching importance value. The toggle bar at the top reads + writes this.
  let importanceFilter = $state<DeadlineImportance | null>(null);

  // Optional URL-driven scope filters (e.g. /deadlines?project=Foo or
  // ?goal_id=G123). Used by the note-page deadline strip to deep-link
  // to "show me everything tied to this thing". Reactive — survives
  // SPA query-only navigations.
  let scopeProject = $derived($page.url.searchParams.get('project') ?? '');
  let scopeGoalId = $derived($page.url.searchParams.get('goal_id') ?? '');

  // Selection / drawer state.
  let drawerOpen = $state(false);
  // null = create form; non-null = editing this deadline.
  let editing = $state<Deadline | null>(null);
  // Form buffers — bound to inputs so the drawer can be cancelled
  // cleanly without mutating the on-disk record until Save.
  let fTitle = $state('');
  let fDate = $state('');
  let fDescription = $state('');
  let fImportance = $state<DeadlineImportance>('normal');
  let fStatus = $state<DeadlineStatus>('active');
  let fGoalId = $state('');
  let fProject = $state('');
  let fTaskIds = $state<string[]>([]);

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      const [dl, gl, pl] = await Promise.all([
        api.listDeadlines(),
        api.listGoals().catch(() => ({ goals: [] as Goal[], total: 0 })),
        api.listProjects().catch(() => ({ projects: [] as Project[], total: 0 }))
      ]);
      deadlines = dl.deadlines;
      goals = gl.goals;
      projects = pl.projects;
    } catch (e) {
      toast.error('load failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  async function ensureTasksLoaded() {
    if (tasksLoaded) return;
    try {
      const r = await api.listTasks({ status: 'open' });
      openTasks = r.tasks;
    } catch {
      openTasks = [];
    } finally {
      tasksLoaded = true;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      // Refetch on the server's deadlines.json signal AND on note/task
      // changes — the latter two refresh the linked task chips when the
      // underlying task gets renamed or completed.
      if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') load();
      if (ev.type === 'task.changed' || ev.type === 'note.changed') load();
    });
  });

  // ----- Importance filter -----
  //
  // Counts roll over the FULL list (so the pill bar shows the global
  // distribution); the list itself is filtered to whatever the user
  // picked. Active filter shows a "Showing N of M" hint with a clear
  // affordance so the user can never forget they're in a filtered view.
  let importanceCounts = $derived.by(() => {
    let critical = 0, high = 0, normal = 0;
    for (const d of scoped) {
      // Hide already-met from the active-importance counts — those
      // belong to the "Met" tail and shouldn't inflate "you have X
      // critical things to worry about".
      if (d.status === 'met' || d.status === 'cancelled') continue;
      if (d.importance === 'critical') critical++;
      else if (d.importance === 'high') high++;
      else normal++;
    }
    return { critical, high, normal };
  });

  // Scope-filter (URL params) is applied BEFORE the importance filter so
  // counts in the importance bar reflect the scoped subset, not the
  // entire vault — matches the user's mental model when they land here
  // via "deadlines for this project".
  let scoped = $derived.by(() => {
    let out = deadlines;
    if (scopeProject) out = out.filter((d) => d.project === scopeProject);
    if (scopeGoalId) out = out.filter((d) => d.goal_id === scopeGoalId);
    return out;
  });

  let filtered = $derived.by(() => {
    if (!importanceFilter) return scoped;
    return scoped.filter((d) => d.importance === importanceFilter);
  });

  // ----- Hero countdown card -----
  // Most-urgent active row (importance critical → high → normal,
  // earliest date as tiebreaker). Uses the SCOPED list (so a
  // deep-link from a project shows the project's hero deadline) but
  // ignores the importance pill — the card represents the user's
  // most pressing commitment regardless of which pill is active.
  let hero = $derived(pickHeroDeadline(scoped));
  let heroDays = $derived(hero ? daysUntil(hero.date) : 0);

  // ----- Grouping for the list -----
  //
  // Buckets: "Overdue" | "This week" | "This month" | "Later" | "Met"
  // | "Cancelled". Met used to fold into "Done" alongside cancelled,
  // but the user explicitly wants past wins to stay visible — so
  // we now render "Met" as its own tail group, and roll cancelled
  // into a separate quieter bucket below it.

  type Bucket = 'overdue' | 'this_week' | 'this_month' | 'later' | 'met' | 'cancelled';
  const bucketOrder: Bucket[] = ['overdue', 'this_week', 'this_month', 'later', 'met', 'cancelled'];
  const bucketLabel: Record<Bucket, string> = {
    overdue: 'Overdue',
    this_week: 'This week',
    this_month: 'This month',
    later: 'Later',
    met: 'Met',
    cancelled: 'Cancelled'
  };

  function bucketOf(d: Deadline): Bucket {
    if (d.status === 'met') return 'met';
    if (d.status === 'cancelled') return 'cancelled';
    const days = daysUntil(d.date);
    if (days < 0) return 'overdue';
    if (days <= 7) return 'this_week';
    if (days <= 31) return 'this_month';
    return 'later';
  }

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

  let grouped = $derived.by(() => {
    const out: Record<Bucket, Deadline[]> = {
      overdue: [],
      this_week: [],
      this_month: [],
      later: [],
      met: [],
      cancelled: []
    };
    // The server returns rows already sorted (active+missed first by
    // date asc), so we can just bucket without re-sorting.
    for (const d of filtered) out[bucketOf(d)].push(d);
    return out;
  });

  // ----- Drawer / form -----

  function openCreate() {
    editing = null;
    fTitle = '';
    fDate = todayISO();
    fDescription = '';
    fImportance = 'normal';
    fStatus = 'active';
    fGoalId = '';
    fProject = '';
    fTaskIds = [];
    drawerOpen = true;
    void ensureTasksLoaded();
  }

  function openEdit(d: Deadline) {
    editing = d;
    fTitle = d.title;
    fDate = d.date;
    fDescription = d.description ?? '';
    fImportance = d.importance ?? 'normal';
    fStatus = d.status ?? 'active';
    fGoalId = d.goal_id ?? '';
    fProject = d.project ?? '';
    fTaskIds = [...(d.task_ids ?? [])];
    drawerOpen = true;
    void ensureTasksLoaded();
  }

  function todayISO(): string {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  }

  function isValidDate(s: string): boolean {
    return /^\d{4}-\d{2}-\d{2}$/.test(s);
  }

  async function save() {
    if (!fTitle.trim()) {
      toast.warning('title is required');
      return;
    }
    if (!isValidDate(fDate)) {
      toast.warning('date must be YYYY-MM-DD');
      return;
    }
    busy = true;
    try {
      const payload: DeadlineCreate = {
        title: fTitle.trim(),
        date: fDate,
        description: fDescription.trim() || undefined,
        importance: fImportance,
        status: fStatus,
        goal_id: fGoalId || undefined,
        project: fProject || undefined,
        task_ids: fTaskIds.length ? fTaskIds : undefined
      };
      if (editing) {
        await api.patchDeadline(editing.id, payload);
        toast.success('saved');
      } else {
        await api.createDeadline(payload);
        toast.success('created');
      }
      drawerOpen = false;
      await load();
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      busy = false;
    }
  }

  async function remove() {
    if (!editing) return;
    if (!confirm(`Delete "${editing.title}"? This can't be undone.`)) return;
    busy = true;
    try {
      await api.deleteDeadline(editing.id);
      toast.success('deleted');
      drawerOpen = false;
      await load();
    } catch (e) {
      toast.error('delete failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      busy = false;
    }
  }

  function toggleTaskLink(id: string) {
    if (fTaskIds.includes(id)) fTaskIds = fTaskIds.filter((x) => x !== id);
    else fTaskIds = [...fTaskIds, id];
  }

  function statusTone(s: DeadlineStatus | string | undefined): string {
    switch (s) {
      case 'met':
        return 'success';
      case 'missed':
        return 'error';
      case 'cancelled':
        return 'subtext';
      default:
        return 'info';
    }
  }

  // Display helpers for the chip row — we look up the linked goal by
  // ID so the chip shows a real title (the on-disk record only stores
  // the ULID). Falls back to "(unknown)" if the goal was deleted but
  // the deadline still references it.
  function goalTitle(id?: string): string {
    if (!id) return '';
    const g = goals.find((x) => x.id === id);
    return g?.title ?? '(unknown)';
  }

  function taskText(id: string): string {
    const t = openTasks.find((x) => x.id === id);
    return t?.text ?? id;
  }

  function setFilter(v: DeadlineImportance | null) {
    importanceFilter = importanceFilter === v ? null : v;
  }

  // Border tint for the hero countdown card — driven by urgency, not
  // importance, since the card already dedicates a big icon + label
  // to importance. This keeps the visual hierarchy: urgency = card
  // glow; importance = icon.
  function heroBorder(days: number): string {
    if (days < 0 || days <= 3) return 'var(--color-error)';
    if (days <= 7) return 'var(--color-warning)';
    if (days <= 14) return 'var(--color-info)';
    return 'var(--color-secondary)';
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-4xl mx-auto">
    <VisionContextStrip />
    <PageHeader
      title="Deadlines"
      subtitle="dated commitments — linked to goals, projects, or tasks"
    >
      {#snippet actions()}
        <button
          onclick={openCreate}
          class="px-3 py-1.5 bg-primary text-on-primary text-sm font-medium rounded hover:opacity-90"
        >+ New deadline</button>
      {/snippet}
    </PageHeader>

    {#if scopeProject || scopeGoalId}
      <div class="mb-4 flex items-center gap-2 text-xs px-3 py-2 bg-secondary/10 border border-secondary/30 rounded">
        <span class="text-secondary">
          {#if scopeProject}📁 Scope: project <strong>{scopeProject}</strong>{/if}
          {#if scopeGoalId}🎯 Scope: goal <strong>{goalTitle(scopeGoalId)}</strong>{/if}
        </span>
        <span class="text-dim">· {scoped.length} {scoped.length === 1 ? 'deadline' : 'deadlines'}</span>
        <a href="/deadlines" class="ml-auto text-dim hover:text-text">× clear scope</a>
      </div>
    {/if}

    {#if loading && deadlines.length === 0}
      <div class="text-sm text-dim">loading…</div>
    {:else if deadlines.length === 0}
      <div class="text-sm text-dim italic">
        no deadlines yet. click "+ New deadline" to add the first one.
      </div>
    {:else}
      <!-- Hero countdown card — most-urgent active row. Visually
           striking so the user can't miss it on first paint. -->
      {#if hero}
        {@const h = hero}
        <button
          type="button"
          onclick={() => openEdit(h)}
          class="w-full text-left block mb-5 p-4 sm:p-5 bg-surface0 border-l-4 rounded-lg hover:border-primary transition-colors"
          style="border-left-color: {heroBorder(heroDays)};"
        >
          <div class="flex items-start gap-3">
            <span class="text-2xl flex-shrink-0 mt-0.5" aria-hidden="true">
              {h.importance === 'critical' ? '🔴' : h.importance === 'high' ? '🟠' : '🟢'}
            </span>
            <div class="flex-1 min-w-0">
              <div class="text-[11px] uppercase tracking-wider text-dim mb-0.5">Up next</div>
              <div class="text-lg sm:text-xl font-semibold text-text leading-tight">
                {#if heroDays < 0}
                  <span class="text-error">{Math.abs(heroDays)} {Math.abs(heroDays) === 1 ? 'day' : 'days'} overdue</span> · {h.title}
                {:else if heroDays === 0}
                  <span class="text-error">Today</span> · {h.title}
                {:else if heroDays === 1}
                  <span class="text-warning">Tomorrow</span> · {h.title}
                {:else}
                  <span class="text-primary">{heroDays} days</span>
                  <span class="text-dim font-normal">until</span>
                  {h.title}
                {/if}
              </div>
              <div class="flex flex-wrap items-center gap-x-3 gap-y-1 mt-1.5 text-xs text-dim">
                <span class="font-mono tabular-nums text-subtext">{h.date}</span>
                {#if h.goal_id}
                  <span class="text-secondary">🎯 {goalTitle(h.goal_id)}</span>
                {/if}
                {#if h.project}
                  <span class="text-secondary">📁 {h.project}</span>
                {/if}
                <span class="ml-auto text-secondary hover:underline">View →</span>
              </div>
            </div>
          </div>
        </button>
      {/if}

      <!-- Importance filter bar — three pills with global counts.
           Click toggles. The hint row below tells the user we're
           filtered (and how to reset). -->
      <div class="flex flex-wrap items-center gap-2 mb-4">
        {#each [
          { v: 'critical' as DeadlineImportance, label: 'Critical', tone: 'error', count: importanceCounts.critical },
          { v: 'high' as DeadlineImportance, label: 'High', tone: 'warning', count: importanceCounts.high },
          { v: 'normal' as DeadlineImportance, label: 'Normal', tone: 'secondary', count: importanceCounts.normal }
        ] as p}
          {@const active = importanceFilter === p.v}
          <button
            type="button"
            onclick={() => setFilter(p.v)}
            class="px-3 py-1.5 rounded-full border text-xs font-medium tabular-nums transition-colors flex items-center gap-1.5
              {active ? 'border-transparent' : 'border-surface1 hover:border-surface2'}"
            style={active
              ? `background: var(--color-${p.tone}); color: var(--color-mantle);`
              : `color: var(--color-${p.tone}); background: color-mix(in srgb, var(--color-${p.tone}) 8%, transparent);`}
          >
            <span>{p.label}</span>
            <span class="text-[10px] opacity-80">{p.count}</span>
          </button>
        {/each}
        {#if importanceFilter}
          <span class="text-xs text-dim">
            Showing {filtered.length} of {scoped.length}
            <button
              type="button"
              onclick={() => (importanceFilter = null)}
              class="ml-1 px-1.5 py-0.5 rounded text-dim hover:text-text hover:bg-surface1"
            >× clear</button>
          </span>
        {/if}
      </div>

      <div class="space-y-6">
        {#each bucketOrder as b}
          {@const rows = grouped[b]}
          {#if rows.length > 0}
            <section>
              <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-2">
                {bucketLabel[b]}
                <span class="text-dim/70 ml-1">·{rows.length}</span>
              </h2>
              <ul class="space-y-1.5">
                {#each rows as d (d.id)}
                  {@const st = statusTone(d.status)}
                  {@const isMet = d.status === 'met'}
                  {@const isCancelled = d.status === 'cancelled'}
                  <li>
                    <button
                      type="button"
                      onclick={() => openEdit(d)}
                      class="w-full text-left bg-surface0 border border-surface1 rounded-lg p-3 hover:border-primary/50 transition-colors flex flex-col gap-1.5
                        {isMet || isCancelled ? 'opacity-60' : ''}"
                    >
                      <!-- Title row — stacks on phone, inline 2-col on sm+
                           so the date pill stays visible on narrow screens. -->
                      <div class="flex flex-col sm:flex-row sm:items-baseline sm:gap-2">
                        <div class="flex items-baseline gap-2 flex-1 min-w-0">
                          <DeadlinePill variant="icon" importance={d.importance} />
                          <span class="text-base text-text font-medium flex-1 min-w-0 truncate {isMet ? 'line-through' : ''}">
                            {d.title}
                          </span>
                        </div>
                        <div class="flex items-center gap-1.5 flex-shrink-0 mt-1 sm:mt-0">
                          <DeadlinePill variant="importance" importance={d.importance} />
                          {#if d.status && d.status !== 'active'}
                            <span
                              class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded whitespace-nowrap"
                              style="background: color-mix(in srgb, var(--color-{st}) 18%, transparent); color: var(--color-{st});"
                            >{d.status}</span>
                          {/if}
                        </div>
                      </div>
                      <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-dim">
                        <span class="font-mono tabular-nums text-subtext">{d.date}</span>
                        <span>· {countdown(d)}</span>
                        {#if d.goal_id}
                          <span class="text-secondary">🎯 {goalTitle(d.goal_id)}</span>
                        {/if}
                        {#if d.project}
                          <span class="text-secondary">📁 {d.project}</span>
                        {/if}
                        {#if d.task_ids && d.task_ids.length > 0}
                          <span>🔗 {d.task_ids.length} task{d.task_ids.length === 1 ? '' : 's'}</span>
                        {/if}
                      </div>
                      {#if d.description}
                        <p class="text-sm text-subtext line-clamp-2">{d.description}</p>
                      {/if}
                    </button>
                  </li>
                {/each}
              </ul>
            </section>
          {/if}
        {/each}
        {#if importanceFilter && filtered.length === 0}
          <div class="text-sm text-dim italic">
            No deadlines match this filter.
            <button
              type="button"
              onclick={() => (importanceFilter = null)}
              class="text-secondary hover:underline ml-1"
            >clear →</button>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>

<Drawer bind:open={drawerOpen} side="right" responsive width="w-full sm:w-96 md:w-[28rem]">
  <div class="h-full flex flex-col overflow-hidden">
    <header class="px-4 py-3 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
      <h2 class="text-sm font-semibold text-text flex-1 truncate">
        {editing ? 'Edit deadline' : 'New deadline'}
      </h2>
      <button
        onclick={() => (drawerOpen = false)}
        aria-label="close"
        class="text-dim hover:text-text text-lg leading-none"
      >×</button>
    </header>

    <div class="flex-1 overflow-y-auto p-4 space-y-4">
      <div>
        <label for="d-title" class="block text-xs uppercase tracking-wider text-dim mb-1">Title</label>
        <input
          id="d-title"
          type="text"
          bind:value={fTitle}
          placeholder="e.g. Bar exam"
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
        />
      </div>

      <div>
        <label for="d-date" class="block text-xs uppercase tracking-wider text-dim mb-1">Date</label>
        <input
          id="d-date"
          type="date"
          bind:value={fDate}
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
        />
      </div>

      <div>
        <label for="d-desc" class="block text-xs uppercase tracking-wider text-dim mb-1">Description</label>
        <textarea
          id="d-desc"
          bind:value={fDescription}
          rows="3"
          placeholder="optional context"
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary resize-y"
        ></textarea>
      </div>

      <div>
        <span class="block text-xs uppercase tracking-wider text-dim mb-1">Importance</span>
        <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm">
          {#each ['critical', 'high', 'normal'] as i}
            <button
              type="button"
              onclick={() => (fImportance = i as DeadlineImportance)}
              class="flex-1 px-3 py-1.5 capitalize {fImportance === i ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            >{i}</button>
          {/each}
        </div>
      </div>

      <div>
        <span class="block text-xs uppercase tracking-wider text-dim mb-1">Status</span>
        <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm">
          {#each ['active', 'missed', 'met', 'cancelled'] as s}
            <button
              type="button"
              onclick={() => (fStatus = s as DeadlineStatus)}
              class="flex-1 px-2 py-1.5 capitalize {fStatus === s ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            >{s}</button>
          {/each}
        </div>
      </div>

      <div>
        <label for="d-goal" class="block text-xs uppercase tracking-wider text-dim mb-1">Linked goal</label>
        <select
          id="d-goal"
          bind:value={fGoalId}
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
        >
          <option value="">— none —</option>
          {#each goals as g (g.id)}
            <option value={g.id}>{g.title}</option>
          {/each}
        </select>
      </div>

      <div>
        <label for="d-project" class="block text-xs uppercase tracking-wider text-dim mb-1">Linked project</label>
        <select
          id="d-project"
          bind:value={fProject}
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
        >
          <option value="">— none —</option>
          {#each projects as p (p.name)}
            <option value={p.name}>{p.name}</option>
          {/each}
        </select>
      </div>

      <div>
        <span class="block text-xs uppercase tracking-wider text-dim mb-1">
          Linked tasks <span class="text-dim/70">({fTaskIds.length} selected)</span>
        </span>
        {#if !tasksLoaded}
          <div class="text-xs text-dim italic">loading tasks…</div>
        {:else if openTasks.length === 0}
          <div class="text-xs text-dim italic">no open tasks to link.</div>
        {:else}
          <div class="max-h-48 overflow-y-auto bg-surface0 border border-surface1 rounded">
            {#each openTasks as t (t.id)}
              {@const checked = fTaskIds.includes(t.id)}
              <label class="flex items-start gap-2 px-2 py-1.5 hover:bg-surface1 cursor-pointer text-xs">
                <input
                  type="checkbox"
                  {checked}
                  onchange={() => toggleTaskLink(t.id)}
                  class="mt-0.5"
                />
                <span class="flex-1 text-text truncate">{t.text}</span>
              </label>
            {/each}
          </div>
        {/if}
      </div>
    </div>

    <footer class="px-4 py-3 border-t border-surface1 flex items-center gap-2 flex-shrink-0">
      {#if editing}
        <button
          onclick={remove}
          disabled={busy}
          class="px-3 py-1.5 text-xs text-error hover:bg-error/10 rounded disabled:opacity-50"
        >Delete</button>
      {/if}
      <span class="flex-1"></span>
      <button
        onclick={() => (drawerOpen = false)}
        disabled={busy}
        class="px-3 py-1.5 text-sm text-subtext hover:bg-surface0 rounded disabled:opacity-50"
      >Cancel</button>
      <button
        onclick={save}
        disabled={busy}
        class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded hover:opacity-90 disabled:opacity-50"
      >{editing ? 'Save' : 'Create'}</button>
    </footer>
  </div>
</Drawer>
