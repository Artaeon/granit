<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { api, type Project, todayISO } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import {
    createProjectsListData,
    installProjectsListLive
  } from '$lib/projects/projectsListData.svelte';
  import { createProjectsListFilter } from '$lib/projects/projectsListFilter.svelte';
  import {
    SPARK_WEEKS,
    buildSparkWeekOrder,
    computeMomentumByProject
  } from '$lib/projects/projectsListMomentum';
  import { createProjectsListStallRadar } from '$lib/projects/projectsListStallRadar.svelte';
  import {
    createProjectsListUrlState,
    type ViewMode
  } from '$lib/projects/projectsListUrlState.svelte';
  import {
    consumeAgentLaunchParam,
    installProjectsListShortcuts
  } from '$lib/projects/projectsListShortcuts';
  import { colorVar, statusTone } from '$lib/util/colors';
  import ProjectDetail from '$lib/projects/ProjectDetail.svelte';
  import ProjectCreate from '$lib/projects/ProjectCreate.svelte';
  import ProjectTimeline from '$lib/projects/ProjectTimeline.svelte';
  import ProjectHeatmap from '$lib/projects/ProjectHeatmap.svelte';
  import ProjectKanban from '$lib/projects/ProjectKanban.svelte';
  import ProjectAgent from '$lib/projects/ProjectAgent.svelte';
  import ProjectDashboardPanel from '$lib/projects/ProjectDashboardPanel.svelte';
  import type { KanbanStatus } from '$lib/projects/kanbanGroup';
  import ProjectStatusBar from '$lib/projects/ProjectStatusBar.svelte';
  import VisionContextStrip from '$lib/components/VisionContextStrip.svelte';

  // Project Agent — conversational mutation engine for /projects.
  // Same architecture as TaskAgent: free-text intent → streamed
  // typed actions → accept per row → run-scoped undo. Shared
  // re-stream merge logic lives in $lib/agents/core.
  let agentOpen = $state(false);

  const dataCtl = createProjectsListData();
  const projects = $derived(dataCtl.projects);
  const tasks = $derived(dataCtl.tasks);
  const loading = $derived(dataCtl.loading);

  // URL-driven view state — selected project, view mode, dashboard
  // overlay, venture scope. The page is bookmarkable: every
  // persisted choice round-trips through the URL.
  const urlCtl = createProjectsListUrlState({
    getSearchParams: () => $page.url.searchParams,
    getProjects: () => projects,
    navigate: (url, opts) => goto(url, opts)
  });
  const selectedName = $derived(urlCtl.selectedName);
  const selected = $derived(urlCtl.selected);
  const viewMode = $derived(urlCtl.viewMode);
  const ventureFilter = $derived(urlCtl.ventureFilter);
  const dashboardOpen = $derived(urlCtl.dashboardOpen);
  const selectProject = (name: string) => urlCtl.selectProject(name);
  const setViewMode = (v: ViewMode) => urlCtl.setViewMode(v);
  const openDashboard = () => urlCtl.openDashboard();
  const closeDashboard = () => urlCtl.closeDashboard();
  const clearVentureFilter = () => urlCtl.clearVentureFilter();

  // ── Per-project momentum derivations ─────────────────────────────
  // Pull all tasks once on the list page so each card can render a
  // tiny 4-week sparkline + "this week" counts. Without this, the
  // ProjectDetail panel (right pane) was the only surface that
  // showed momentum — users browsing the list saw only a flat
  // milestone-progress bar. The data is already loaded for the
  // detail panel; surfacing it on the cards costs zero extra wire
  // calls and answers "which projects are alive" at a glance. The
  // math lives in projectsListMomentum.ts as pure stateless helpers
  // so it round-trips through unit tests; we only wrap it in
  // $derived here to pick up project/task reactivity.
  const sparkWeekOrder = $derived.by(() => buildSparkWeekOrder(new Date()));
  const momentumByProject = $derived.by(() =>
    computeMomentumByProject(projects, tasks, sparkWeekOrder, new Date())
  );
  const filterCtl = createProjectsListFilter({
    getProjects: () => projects,
    getVentureFilter: () => ventureFilter
  });
  const q = $derived(filterCtl.q);
  const statusFilter = $derived(filterCtl.statusFilter);
  const ventures = $derived(filterCtl.ventures);
  const filtered = $derived(filterCtl.filtered);
  const kanbanFeed = $derived(filterCtl.kanbanFeed);
  const grouped = $derived(filterCtl.grouped);
  let createOpen = $state(false);

  // ── Stalled-projects radar ───────────────────────────────────────
  // Local stall detection + AI unblock suggestions, with Stop/Close
  // splitting cancel (keep partial rows) from close (drop rows).
  // The controller owns rows/busy/error/ranAt + runRadar + the
  // optimistic archive flow. Page reads its derives and binds the
  // template buttons directly to the controller methods.
  const radarCtl = createProjectsListStallRadar({
    getProjects: () => projects,
    getTasks: () => tasks,
    reload: () => load()
  });
  const radarOpen = $derived(radarCtl.open);
  const radarBusy = $derived(radarCtl.busy);
  const radarError = $derived(radarCtl.error);
  const radarRows = $derived(radarCtl.rows);
  const radarRanAt = $derived(radarCtl.ranAt);
  const stalledLocally = $derived(radarCtl.stalledLocally);

  // Kanban drag handler. Patches the project's status and refreshes
  // optimistically. Distinct from archiveProject because the kanban
  // is a fluent reclassification surface — no confirm() in the
  // middle of a drag (the user just dragged it, the intent is
  // unambiguous). Failures revert via load().
  async function handleKanbanStatusChange(name: string, status: KanbanStatus) {
    // Optimistic local update so the card "lands" in the new column
    // before the network roundtrip completes.
    dataCtl.projects = projects.map((p) => (p.name === name ? { ...p, status } : p));
    try {
      await api.patchProject(name, { status });
      toast.success(`"${name}" → ${status}`);
    } catch (e) {
      toast.error('status change failed: ' + (e instanceof Error ? e.message : String(e)));
    }
    await load();
  }

  const load = () => dataCtl.load();

  onMount(() => {
    load();
    consumeAgentLaunchParam({
      getSearchParams: () => $page.url.searchParams,
      navigate: (url, opts) => void goto(url, opts),
      openAgent: () => (agentOpen = true)
    });
    const offLive = installProjectsListLive({ reload: load });
    const offKeys = installProjectsListShortcuts({
      openAgent: () => (agentOpen = true)
    });
    return () => {
      offLive();
      offKeys();
    };
  });

  async function created(p: Project) {
    createOpen = false;
    // Optimistic insert so the new project shows up immediately even if
    // the listProjects roundtrip is slow. The await load() below
    // reconciles with server-decorated fields (progress, task counts).
    if (!projects.some((x) => x.name === p.name)) {
      dataCtl.projects = [p, ...projects];
    }
    selectProject(p.name);
    await load();
  }

  async function deleted(name: string) {
    selectProject('');
    await load();
    toast.success(`project "${name}" deleted`);
  }
</script>

<div class="h-full flex flex-col">
  <!-- Vision strip sits above the projects layout (sidebar + detail
       split), so the user always sees their season focus without it
       competing with horizontal space. Hidden on mobile when the
       detail pane is open to keep the chrome quiet. -->
  <div class="px-3 sm:px-4 pt-3 flex-shrink-0 {selectedName && viewMode === 'list' ? 'hidden md:block' : ''}">
    <VisionContextStrip />
  </div>

  <!-- Top-level view-mode toggle. Lives outside the sidebar so the
       buttons stay visible in timeline mode (where the sidebar is
       hidden). The list mode is the default — picked by users who
       want the sidebar+detail browsing flow. Timeline gives a
       Gantt-ish whole-portfolio plan view, heatmap a workload grid.
       In chart modes the sidebar's status pills aren't visible, so
       a compact status select rides next to the toggle so the user
       can still scope the chart without bouncing back to list view. -->
  <div class="px-3 sm:px-4 pt-2 flex-shrink-0 flex items-center gap-1.5 flex-wrap {selectedName && viewMode === 'list' ? 'hidden md:flex' : 'flex'}">
    <div class="inline-flex rounded border border-surface1 bg-surface0 overflow-hidden text-xs" role="tablist" aria-label="view mode">
      <button
        role="tab"
        aria-selected={viewMode === 'list'}
        onclick={() => setViewMode('list')}
        class="px-2.5 py-1.5 sm:py-1 min-h-[32px] {viewMode === 'list' ? 'bg-surface1 text-text' : 'text-dim hover:text-text'}"
        title="List + detail (default)"
      >☰ List</button>
      <button
        role="tab"
        aria-selected={viewMode === 'kanban'}
        onclick={() => setViewMode('kanban')}
        class="px-2.5 py-1.5 sm:py-1 min-h-[32px] border-l border-surface1 {viewMode === 'kanban' ? 'bg-surface1 text-text' : 'text-dim hover:text-text'}"
        title="Kanban — drag cards to change status"
      >▤ Board</button>
      <button
        role="tab"
        aria-selected={viewMode === 'timeline'}
        onclick={() => setViewMode('timeline')}
        class="px-2.5 py-1.5 sm:py-1 min-h-[32px] border-l border-surface1 {viewMode === 'timeline' ? 'bg-surface1 text-text' : 'text-dim hover:text-text'}"
        title="Gantt-ish timeline across all projects"
      >▭ Timeline</button>
      <button
        role="tab"
        aria-selected={viewMode === 'heatmap'}
        onclick={() => setViewMode('heatmap')}
        class="px-2.5 py-1.5 sm:py-1 min-h-[32px] border-l border-surface1 {viewMode === 'heatmap' ? 'bg-surface1 text-text' : 'text-dim hover:text-text'}"
        title="Per-project completion volume by week"
      >▦ Heatmap</button>
    </div>
    {#if viewMode === 'timeline' || viewMode === 'heatmap' || viewMode === 'kanban'}
      <!-- The list view's sidebar carries the search box; the chart
           and board views hide the sidebar, so a compact mirror sits
           in the toolbar so the user isn't search-blind here. -->
      <input
        bind:value={filterCtl.q}
        placeholder="filter…"
        class="text-xs px-2 py-1 bg-surface0 border border-surface1 rounded text-text placeholder:text-dim focus:outline-none focus:border-primary min-h-[32px] w-32 sm:w-40"
        aria-label="filter projects"
      />
      <!-- Kanban already splits by status (one column per state),
           so the status select would just empty three columns —
           hide it there. Timeline/heatmap still need it because
           those views show every project on a single canvas. -->
      {#if viewMode !== 'kanban'}
        <select
          value={statusFilter}
          onchange={(e) => (filterCtl.statusFilter = (e.target as HTMLSelectElement).value as typeof statusFilter)}
          class="text-xs px-2 py-1 bg-surface0 border border-surface1 rounded text-subtext min-h-[32px]"
          aria-label="filter by status"
        >
          <option value="active">active</option>
          <option value="paused">paused</option>
          <option value="completed">completed</option>
          <option value="archived">archived</option>
          <option value="all">all</option>
        </select>
      {/if}
      {#if ventureFilter}
        <button
          onclick={clearVentureFilter}
          class="text-xs px-2 py-1 rounded bg-surface1 text-secondary hover:bg-surface2 min-h-[32px]"
          title="clear venture filter"
        >🏢 {ventureFilter} ×</button>
      {/if}
      <!-- Project Agent button — removed; launches from the chat
           sidebar via ?agent=1 instead. -->
      <button
        onclick={() => (createOpen = true)}
        class="ml-auto px-2.5 py-1.5 sm:py-1 min-h-[32px] text-xs bg-primary text-on-primary rounded hover:opacity-90"
      >+ new</button>
    {/if}
  </div>

  <div class="flex-1 min-h-0 flex">
  {#if viewMode === 'list'}
  <!-- List -->
  <aside class="w-full md:w-72 lg:w-80 xl:w-96 flex-shrink-0 border-r border-surface1 bg-mantle flex flex-col {selectedName ? 'hidden md:flex' : ''}">
    <header class="px-3 py-2.5 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
      <h2 class="text-sm font-medium text-text flex-1">Projects</h2>
      <!-- Project Agent button removed; launches from the chat
           sidebar via ?agent=1. -->

      <button
        onclick={() => (createOpen = true)}
        class="px-2.5 py-1 text-xs bg-primary text-on-primary rounded hover:opacity-90"
      >+ new</button>
    </header>
    <div class="px-3 py-2 space-y-2 flex-shrink-0">
      <input
        bind:value={filterCtl.q}
        placeholder="filter… (name, kind, venture, tag)"
        class="w-full px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      <!-- Five status pills. py-1.5 on mobile gives a ~32px tap row;
           desktop tightens back to py-0.5 to keep the dense sidebar
           feel. flex-wrap lets the row break on narrow phones rather
           than crushing each label below readable width. -->
      <div class="flex flex-wrap gap-1 text-xs">
        {#each ['active', 'paused', 'completed', 'archived', 'all'] as s}
          <button
            class="flex-1 min-w-[3.5rem] px-1 py-1.5 sm:py-0.5 rounded {statusFilter === s ? 'bg-surface1 text-text' : 'text-dim hover:text-text hover:bg-surface0'}"
            onclick={() => (filterCtl.statusFilter = s as typeof statusFilter)}
          >{s}</button>
        {/each}
      </div>
      {#if ventureFilter}
        <button
          onclick={clearVentureFilter}
          class="w-full text-left px-2 py-1 text-xs rounded bg-surface1 text-secondary hover:bg-surface2 flex items-center gap-1.5"
          title="clear venture filter"
        >
          <span>🏢 {ventureFilter}</span>
          <span class="ml-auto text-dim hover:text-text">×</span>
        </button>
      {/if}

      <!-- Stalled-projects radar — collapsible. Local heuristic
           detects active projects with no completion / no edit in
           STALL_DAYS days; the AI adds a one-line unblock per row.
           One toggle button to open + scan; once open, results
           stay visible and the user can rerun. The badge on the
           closed button shows the local count so the user knows
           "the radar would find 4 things" without opening it. -->
      {#if !radarOpen}
        <button
          onclick={() => radarCtl.openAndScan()}
          class="w-full text-left px-2 py-1.5 text-xs rounded bg-surface0 border border-surface1 hover:border-primary text-subtext flex items-center gap-1.5"
          title="Scan active projects for stalled work"
        >
          <span>📡 Stalled radar</span>
          {#if stalledLocally.length > 0}
            <span class="ml-auto px-1.5 py-0 rounded bg-surface0 text-warning font-mono text-[10px]">{stalledLocally.length}</span>
          {:else}
            <span class="ml-auto text-dim font-mono text-[10px]">—</span>
          {/if}
        </button>
      {:else}
        <div class="border border-warning bg-surface0 rounded">
          <div class="px-2 py-1.5 flex items-center gap-1.5 text-xs border-b border-warning">
            <span class="text-warning font-medium flex-1">📡 Stalled radar</span>
            {#if radarBusy}
              <button onclick={() => radarCtl.cancelRadar()} class="text-[10px] text-dim hover:text-error">cancel</button>
            {:else}
              <button
                onclick={() => void radarCtl.runRadar()}
                class="text-[10px] text-secondary hover:underline"
                title="rerun the scan"
              >rerun</button>
            {/if}
            <button
              onclick={() => radarCtl.close()}
              class="text-[10px] text-dim hover:text-text"
              aria-label="close radar"
            >×</button>
          </div>
          <p class="px-2 pt-1.5 text-[10px] text-dim font-mono">
            scanned {projects.filter((p) => (p.status ?? 'active') === 'active').length} active
            {#if radarRanAt} · ran {radarRanAt}{/if}
          </p>
          {#if radarError}
            <p class="px-2 py-1 text-[10px] text-error">{radarError}</p>
          {/if}
          {#if radarRows.length === 0 && !radarBusy}
            <p class="px-2 py-2 text-xs text-success">Nothing stalled. Active projects all show recent work.</p>
          {:else}
            <ul class="divide-y divide-warning/15">
              {#each radarRows as r (r.name)}
                <li class="px-2 py-1.5 text-xs">
                  <div class="flex items-baseline gap-1.5 mb-0.5">
                    <span class="w-1.5 h-1.5 rounded-full flex-shrink-0" style="background: {colorVar(r.color)}"></span>
                    <button
                      onclick={() => selectProject(r.name)}
                      class="text-text hover:text-primary truncate flex-1 text-left font-medium"
                      title="open {r.name}"
                    >{r.name}</button>
                    <span class="text-[9px] text-dim font-mono flex-shrink-0">
                      {#if r.daysSinceCompletion === null}never done{:else}{r.daysSinceCompletion}d{/if}
                    </span>
                  </div>
                  <p class="text-[10px] text-dim mb-1">
                    {r.openTasks} open
                    {#if r.overdueTasks > 0} · <span class="text-error">{r.overdueTasks} overdue</span>{/if}
                    {#if r.daysSinceUpdate !== null} · edited {r.daysSinceUpdate}d ago{/if}
                  </p>
                  {#if r.unblock}
                    <p class="text-[11px] text-text/90 italic mb-1.5">→ {r.unblock}</p>
                  {:else if radarBusy}
                    <p class="text-[10px] text-dim italic mb-1.5">…</p>
                  {/if}
                  <div class="flex gap-1">
                    <a
                      href={`/calendar?plan=1&project=${encodeURIComponent(r.name)}`}
                      class="text-[10px] px-1.5 py-0.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary"
                      title="open the calendar in plan mode for a 30-min unstick session"
                    >schedule unstick →</a>
                    <button
                      onclick={() => void radarCtl.archiveProject(r.name)}
                      class="text-[10px] px-1.5 py-0.5 rounded bg-surface0 border border-surface1 text-dim hover:border-error hover:text-error"
                      title="archive this project"
                    >archive</button>
                  </div>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}
    </div>
    <div class="flex-1 overflow-y-auto">
      {#if loading && projects.length === 0}
        <div class="p-4 text-sm text-dim">loading…</div>
      {:else if filtered.length === 0}
        <div class="p-4 text-sm text-dim italic">no projects</div>
      {:else}
        {#each grouped as g (g.venture)}
          {#if !ventureFilter && grouped.length > 1}
            <div class="px-3 pt-3 pb-1 sticky top-0 bg-mantle z-10 flex items-center gap-2 border-b border-surface1">
              <span class="text-[10px] uppercase tracking-wider text-dim font-medium flex-1 truncate">
                {g.venture === '—' ? 'no venture' : g.venture}
              </span>
              <span class="text-[10px] text-dim font-mono">{g.projects.length}</span>
            </div>
          {/if}
          <ul class="divide-y divide-surface1">
            {#each g.projects as p (p.name)}
              {@const active = p.name === selectedName}
              {@const progress = p.progress ?? 0}
              <li>
                <button
                  onclick={() => selectProject(p.name)}
                  class="w-full text-left px-3 py-2.5 hover:bg-surface0 {active ? 'bg-surface1' : ''}"
                >
                  <div class="flex items-baseline gap-2 mb-1">
                    <span class="w-2 h-2 rounded-full flex-shrink-0" style="background: {colorVar(p.color)}"></span>
                    <span class="text-sm font-medium text-text flex-1 truncate">{p.name}</span>
                    {#if p.kind}
                      <span class="text-[9px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-surface1 text-primary flex-shrink-0">{p.kind}</span>
                    {/if}
                    <span
                      class="text-[10px] uppercase tracking-wider flex-shrink-0"
                      style="color: var(--color-{statusTone(p.status ?? 'active')})"
                    >{p.status ?? 'active'}</span>
                  </div>
                  {#if p.description}
                    <p class="text-xs text-subtext line-clamp-2 mb-1.5">{p.description}</p>
                  {/if}
                  <div class="flex items-center gap-2 text-[10px]">
                    <div class="flex-1 h-1 rounded-full bg-surface0 overflow-hidden">
                      <div
                        class="h-full"
                        style="width: {Math.round(progress * 100)}%; background: {colorVar(p.color)}"
                      ></div>
                    </div>
                    <span class="text-dim font-mono w-10 text-right">{Math.round(progress * 100)}%</span>
                    {#if p.tasksTotal != null && p.tasksTotal > 0}
                      <span class="text-dim">{p.tasksDone}/{p.tasksTotal}</span>
                    {/if}
                  </div>

                  <!-- Task-by-status mini-bar — open / scheduled-this-week
                       / done split. The progress bar above answers
                       "how far?", this answers "is the open work being
                       picked up?" — a project with 30 open tasks and
                       0 scheduled tells a different story from one with
                       30 open and 8 scheduled. -->
                  <ProjectStatusBar project={p} tasks={tasks} />

                  {#if momentumByProject.get(p.name)}
                    {@const m = momentumByProject.get(p.name)!}
                    {@const sparkMax = Math.max(...m.spark, 1)}
                    {@const sparkTotal = m.spark.reduce((s, v) => s + v, 0)}
                    {#if sparkTotal > 0 || m.scheduledThisWeek > 0}
                      <!-- 4-week mini-sparkline + this-week count.
                           The list now answers "is this project alive"
                           at a glance — the user can spot stalled
                           projects (flat zero bars) without clicking
                           into each detail panel. Same ISO-week
                           bucketing as the detail burn-up so the
                           per-card view and the per-project view
                           agree. -->
                      <div class="flex items-center gap-2 mt-1.5 text-[10px]">
                        <div class="flex items-end gap-0.5 h-3 flex-shrink-0" aria-hidden="true">
                          {#each m.spark as count, i (i)}
                            {@const isThisWeek = i === SPARK_WEEKS - 1}
                            {@const pct = sparkMax === 0 ? 0 : Math.max(15, Math.round((count / sparkMax) * 100))}
                            <div
                              class="w-1 rounded-sm {isThisWeek ? 'bg-primary' : 'bg-surface2'}"
                              style="height: {pct}%"
                            ></div>
                          {/each}
                        </div>
                        {#if sparkTotal > 0}
                          <span class="text-dim font-mono">{sparkTotal} done · 4w</span>
                        {/if}
                        {#if m.scheduledThisWeek > 0}
                          <span class="flex-1"></span>
                          <span class="text-secondary font-mono" title="Tasks scheduled this week">📅 {m.scheduledThisWeek}</span>
                        {/if}
                      </div>
                    {/if}
                  {/if}
                </button>
              </li>
            {/each}
          </ul>
        {/each}
      {/if}
    </div>
  </aside>

  <!-- Detail -->
  <main class="flex-1 min-w-0 {selectedName ? 'block' : 'hidden md:block'}">
    {#if selected}
      <ProjectDetail
        project={selected}
        onClose={() => selectProject('')}
        onUpdated={load}
        onDeleted={deleted}
        onOpenDashboard={openDashboard}
      />
    {:else}
      <div class="h-full flex items-center justify-center text-dim text-sm">
        Select a project from the list, or create a new one.
      </div>
    {/if}
  </main>
  {:else if viewMode === 'kanban'}
    <!-- Kanban — drag-to-change-status board. Sidebar collapses
         (the four columns ARE the navigation). Detail pane opens
         in the same drawer pattern as timeline so clicking a card
         doesn't leave the board. -->
    <main class="flex-1 min-w-0 flex flex-col {selectedName ? 'hidden md:flex' : ''}">
      <ProjectKanban
        projects={kanbanFeed}
        tasks={tasks}
        onSelect={selectProject}
        onStatusChange={handleKanbanStatusChange}
        colorVar={colorVar}
        statusTone={statusTone}
        selectedName={selectedName}
      />
    </main>
    {#if selected}
      <aside class="w-full md:w-[28rem] lg:w-[32rem] flex-shrink-0 border-l border-surface1 bg-base">
        <ProjectDetail
          project={selected}
          onClose={() => selectProject('')}
          onUpdated={load}
          onDeleted={deleted}
        />
      </aside>
    {/if}
  {:else if viewMode === 'timeline'}
    <!-- Timeline view — full-width Gantt-ish chart. Clicking a bar
         flips the URL to ?p=<name>, which keeps the project drawer
         opening on top so the timeline stays the active surface. -->
    <main class="flex-1 min-w-0 flex flex-col {selectedName ? 'hidden md:flex' : ''}">
      <ProjectTimeline
        projects={filtered}
        tasks={tasks}
        onSelect={selectProject}
        colorVar={colorVar}
        statusTone={statusTone}
      />
    </main>
    {#if selected}
      <!-- On desktop the detail pane sits beside the timeline; on
           mobile it covers (the timeline is too dense to share with
           a side pane). -->
      <aside class="w-full md:w-[28rem] lg:w-[32rem] flex-shrink-0 border-l border-surface1 bg-base">
        <ProjectDetail
          project={selected}
          onClose={() => selectProject('')}
          onUpdated={load}
          onDeleted={deleted}
        />
      </aside>
    {/if}
  {:else if viewMode === 'heatmap'}
    <!-- Heatmap — projects × weeks completion grid. Same overlay
         pattern as timeline: full surface chart, optional detail
         pane on the right when a project is selected. -->
    <main class="flex-1 min-w-0 flex flex-col {selectedName ? 'hidden md:flex' : ''}">
      <ProjectHeatmap
        projects={filtered}
        tasks={tasks}
        onSelect={selectProject}
        colorVar={colorVar}
      />
    </main>
    {#if selected}
      <aside class="w-full md:w-[28rem] lg:w-[32rem] flex-shrink-0 border-l border-surface1 bg-base">
        <ProjectDetail
          project={selected}
          onClose={() => selectProject('')}
          onUpdated={load}
          onDeleted={deleted}
        />
      </aside>
    {/if}
  {/if}
  </div>
</div>

{#if dashboardOpen && selected}
  <!-- Project Dashboard overlay — full-screen visual operating
       picture for the selected project. URL-persisted via
       ?dashboard=1 so a reload keeps it open. Sits above every
       other layout (list/kanban/timeline/heatmap) without
       unmounting them, so closing the dashboard lands the user
       back where they came from. -->
  <ProjectDashboardPanel project={selected} onClose={closeDashboard} />
{/if}

<ProjectCreate bind:open={createOpen} ventures={ventures} onCreated={created} />

<!-- Project Agent — operates on whatever's currently visible
     (filtered list, including venture/search/status scope). The
     parent reloads via load() so the kanban + list + timeline
     all reflect the agent's changes immediately. -->
<ProjectAgent
  open={agentOpen}
  projects={filtered}
  todayISO={todayISO()}
  knownVentures={ventures}
  onClose={() => (agentOpen = false)}
  onChanged={load}
/>
