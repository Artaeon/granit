<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type Task, type Project } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import TaskCard from '$lib/tasks/TaskCard.svelte';
  import Kanban from '$lib/tasks/Kanban.svelte';
  import TriageBoard from '$lib/tasks/TriageBoard.svelte';
  import BulkBar from '$lib/tasks/BulkBar.svelte';
  import TaskDetail from '$lib/tasks/TaskDetail.svelte';
  import Drawer from '$lib/components/Drawer.svelte';

  type View = 'list' | 'kanban' | 'triage' | 'inbox' | 'stale' | 'quickwins' | 'review';
  type Group = 'due' | 'priority' | 'note' | 'project' | 'tag';

  let tasks = $state<Task[]>([]);
  let projects = $state<Project[]>([]);

  // Persist view + groupBy to localStorage so the user comes back to where they left off.
  const VIEW_KEY = 'granit.tasks.view';
  const GROUP_KEY = 'granit.tasks.groupBy';

  let view = $state<View>(
    (typeof localStorage !== 'undefined' && (localStorage.getItem(VIEW_KEY) as View)) || 'list'
  );
  let groupBy = $state<Group>(
    (typeof localStorage !== 'undefined' && (localStorage.getItem(GROUP_KEY) as Group)) || 'due'
  );
  let kanbanMode = $state<'priority' | 'due' | 'triage'>('priority');
  let status = $state<'open' | 'done' | 'all'>('open');
  let q = $state('');
  let tagFilter = $state('');
  let projectFilter = $state('');
  let priorityFilter = $state<number | ''>('');
  let loading = $state(false);
  let filterDrawerOpen = $state(false);
  let selectedIds = $state<Set<string>>(new Set());
  let detailTask = $state<Task | null>(null);
  let detailOpen = $state(false);

  function openDetail(t: Task) {
    detailTask = t;
    detailOpen = true;
  }

  $effect(() => {
    if (typeof localStorage === 'undefined') return;
    try {
      localStorage.setItem(VIEW_KEY, view);
      localStorage.setItem(GROUP_KEY, groupBy);
    } catch {}
  });

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      const params: { status?: 'open' | 'done'; tag?: string } = {};
      if (status !== 'all') params.status = status;
      if (tagFilter) params.tag = tagFilter;
      const [list, p] = await Promise.all([
        api.listTasks(params),
        projects.length === 0 ? api.listProjects().catch(() => ({ projects: [] as Project[] })) : Promise.resolve({ projects })
      ]);
      tasks = list.tasks;
      projects = p.projects;
    } finally {
      loading = false;
    }
  }

  onMount(load);
  onMount(() =>
    onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
    })
  );

  $effect(() => {
    void status;
    void tagFilter;
    load();
  });

  // Active snooze: a task is "active" if snoozedUntil is empty or in the past.
  function isSnoozed(t: Task): boolean {
    if (!t.snoozedUntil) return false;
    const sn = new Date(t.snoozedUntil);
    if (isNaN(sn.getTime())) return false;
    return sn.getTime() > Date.now();
  }

  function isStale(t: Task): boolean {
    if (t.done) return false;
    const ref = t.updatedAt ?? t.createdAt;
    if (!ref) return false;
    const d = new Date(ref);
    if (isNaN(d.getTime())) return false;
    const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
    return d.getTime() < sevenDaysAgo;
  }

  let filtered = $derived.by(() => {
    let out = tasks;
    if (q.trim()) {
      const ql = q.toLowerCase();
      out = out.filter((t) => t.text.toLowerCase().includes(ql) || t.notePath.toLowerCase().includes(ql));
    }
    if (priorityFilter !== '') out = out.filter((t) => t.priority === priorityFilter);
    if (projectFilter) {
      const proj = projects.find((p) => p.name === projectFilter);
      if (proj) {
        out = out.filter((t) => {
          if (t.projectId === proj.name) return true;
          if (proj.folder && t.notePath.startsWith(proj.folder + '/')) return true;
          if (proj.tags && proj.tags.some((tag) => t.tags?.includes(tag))) return true;
          return false;
        });
      }
    }
    // View-specific filtering
    if (view === 'inbox') {
      out = out.filter((t) => !t.done && (t.triage || 'inbox') === 'inbox');
    } else if (view === 'stale') {
      out = out.filter(isStale);
    } else if (view === 'quickwins') {
      out = out.filter((t) => !t.done && t.priority >= 1 && t.priority <= 2 && t.estimatedMinutes && t.estimatedMinutes <= 30);
    } else if (view === 'review') {
      const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
      out = out.filter((t) => t.done && t.completedAt && new Date(t.completedAt).getTime() > sevenDaysAgo);
    } else {
      // For all non-special views, hide currently-snoozed tasks unless explicitly viewing all/done.
      if (status === 'open') out = out.filter((t) => !isSnoozed(t));
    }
    return out;
  });

  type ListGroup = { key: string; label: string; tasks: Task[] };
  let listGroups = $derived.by((): ListGroup[] => {
    if (groupBy === 'due') {
      const today = new Date().toISOString().slice(0, 10);
      const b: Record<string, Task[]> = { overdue: [], today: [], upcoming: [], no_date: [] };
      for (const t of filtered) {
        if (!t.dueDate && !t.scheduledStart) b.no_date.push(t);
        else {
          const d = t.dueDate ?? (t.scheduledStart ? t.scheduledStart.slice(0, 10) : '');
          if (d < today) b.overdue.push(t);
          else if (d === today) b.today.push(t);
          else b.upcoming.push(t);
        }
      }
      return [
        { key: 'overdue', label: 'Overdue', tasks: b.overdue },
        { key: 'today', label: 'Today', tasks: b.today },
        { key: 'upcoming', label: 'Upcoming', tasks: b.upcoming },
        { key: 'no_date', label: 'No date', tasks: b.no_date }
      ].filter((g) => g.tasks.length > 0);
    }
    if (groupBy === 'priority') {
      const b: Record<string, Task[]> = { '1': [], '2': [], '3': [], '0': [] };
      for (const t of filtered) b[String(t.priority)].push(t);
      return [
        { key: '1', label: 'P1 high', tasks: b['1'] },
        { key: '2', label: 'P2 med', tasks: b['2'] },
        { key: '3', label: 'P3 low', tasks: b['3'] },
        { key: '0', label: 'no priority', tasks: b['0'] }
      ].filter((g) => g.tasks.length > 0);
    }
    if (groupBy === 'tag') {
      const b: Record<string, Task[]> = {};
      for (const t of filtered) {
        const tags = t.tags && t.tags.length ? t.tags : ['(untagged)'];
        for (const tag of tags) (b[tag] ??= []).push(t);
      }
      return Object.entries(b).map(([k, v]) => ({ key: k, label: '#' + k.replace('(untagged)', 'untagged'), tasks: v })).sort((a, b) => b.tasks.length - a.tasks.length);
    }
    if (groupBy === 'project') {
      const b: Record<string, Task[]> = {};
      for (const t of filtered) {
        // Prefer explicit projectId; fall back to membership inferred from
        // matching project's folder; else the top-level folder.
        let key = t.projectId || '';
        if (!key) {
          const matched = projects.find((p) => p.folder && t.notePath.startsWith(p.folder + '/'));
          key = matched?.name ?? (t.notePath.split('/')[0] || '(no project)');
        }
        (b[key] ??= []).push(t);
      }
      return Object.entries(b).map(([k, v]) => ({ key: k, label: k, tasks: v })).sort((a, b) => a.label.localeCompare(b.label));
    }
    const b: Record<string, Task[]> = {};
    for (const t of filtered) (b[t.notePath] ??= []).push(t);
    return Object.entries(b).map(([k, v]) => ({ key: k, label: k, tasks: v })).sort((a, b) => a.label.localeCompare(b.label));
  });

  let allTags = $derived.by(() => {
    const s = new Set<string>();
    for (const t of tasks) for (const tag of t.tags ?? []) s.add(tag);
    return Array.from(s).sort();
  });

  let countOpen = $derived(tasks.filter((t) => !t.done).length);
  let countDone = $derived(tasks.filter((t) => t.done).length);

  let activeFilterCount = $derived(
    (priorityFilter !== '' ? 1 : 0) + (projectFilter ? 1 : 0) + (tagFilter ? 1 : 0)
  );
</script>

{#snippet filterContent()}
  <div class="p-4 space-y-4">
    <div>
      <div class="text-xs uppercase tracking-wider text-dim mb-2">Status</div>
      <div class="flex flex-col gap-1 text-sm">
        {#each ['open', 'done', 'all'] as v}
          <button
            class="text-left px-3 py-2 rounded {status === v ? 'bg-surface1 text-text' : 'text-subtext hover:bg-surface0'}"
            onclick={() => (status = v as typeof status)}
          >
            <span class="capitalize">{v}</span>
            {#if v === 'open'}<span class="text-xs text-dim ml-1">{countOpen}</span>{/if}
            {#if v === 'done'}<span class="text-xs text-dim ml-1">{countDone}</span>{/if}
          </button>
        {/each}
      </div>
    </div>

    <div>
      <div class="text-xs uppercase tracking-wider text-dim mb-2">Priority</div>
      <div class="flex flex-col gap-1 text-sm">
        <button class="text-left px-3 py-2 rounded {priorityFilter === '' ? 'bg-surface1' : 'hover:bg-surface0'} text-subtext" onclick={() => (priorityFilter = '')}>any</button>
        <button class="text-left px-3 py-2 rounded {priorityFilter === 1 ? 'bg-error/20 text-error' : 'hover:bg-surface0 text-error'}" onclick={() => (priorityFilter = 1)}>P1 high</button>
        <button class="text-left px-3 py-2 rounded {priorityFilter === 2 ? 'bg-warning/20 text-warning' : 'hover:bg-surface0 text-warning'}" onclick={() => (priorityFilter = 2)}>P2 medium</button>
        <button class="text-left px-3 py-2 rounded {priorityFilter === 3 ? 'bg-info/20 text-info' : 'hover:bg-surface0 text-info'}" onclick={() => (priorityFilter = 3)}>P3 low</button>
      </div>
    </div>

    {#if projects.length > 0}
      <div>
        <div class="text-xs uppercase tracking-wider text-dim mb-2">Projects</div>
        <div class="flex flex-col gap-1 text-sm">
          <button class="text-left px-3 py-2 rounded {projectFilter === '' ? 'bg-surface1' : 'hover:bg-surface0'} text-subtext" onclick={() => (projectFilter = '')}>all</button>
          {#each projects.slice(0, 12) as p}
            <button
              class="text-left px-3 py-2 rounded text-sm truncate {projectFilter === p.name ? 'bg-surface1 text-text' : 'text-subtext hover:bg-surface0'}"
              onclick={() => (projectFilter = projectFilter === p.name ? '' : p.name)}
              title={p.description}
            >
              {p.name}
            </button>
          {/each}
        </div>
      </div>
    {/if}

    {#if allTags.length > 0}
      <div>
        <div class="text-xs uppercase tracking-wider text-dim mb-2">Tags</div>
        <div class="flex flex-wrap gap-1">
          {#each allTags.slice(0, 24) as t}
            <button
              class="text-xs px-2 py-1 rounded {tagFilter === t ? 'bg-primary/30 text-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
              onclick={() => (tagFilter = tagFilter === t ? '' : t)}
            >
              #{t}
            </button>
          {/each}
        </div>
      </div>
    {/if}

    <button
      onclick={() => { priorityFilter = ''; projectFilter = ''; tagFilter = ''; q = ''; }}
      class="w-full text-xs text-dim hover:text-text underline pt-2"
    >
      reset filters
    </button>
  </div>
{/snippet}

<div class="flex h-full">
  <!-- Desktop sidebar -->
  <aside class="hidden md:block md:w-56 lg:w-64 border-r border-surface1 bg-mantle/50 flex-shrink-0 overflow-y-auto">
    {@render filterContent()}
  </aside>

  <!-- Mobile drawer -->
  <Drawer bind:open={filterDrawerOpen} side="left">
    {@render filterContent()}
  </Drawer>

  <div class="flex-1 flex flex-col min-w-0">
    <header class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0 flex-wrap">
      <button
        onclick={() => (filterDrawerOpen = true)}
        aria-label="filters"
        class="md:hidden w-9 h-9 flex items-center justify-center text-subtext hover:bg-surface0 rounded relative"
      >
        <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M3 6h18M6 12h12M9 18h6" stroke-linecap="round" />
        </svg>
        {#if activeFilterCount > 0}
          <span class="absolute -top-0.5 -right-0.5 w-4 h-4 bg-primary text-mantle text-[10px] rounded-full flex items-center justify-center">{activeFilterCount}</span>
        {/if}
      </button>
      <h1 class="text-base sm:text-lg font-semibold text-text">Tasks</h1>
      <span class="text-xs text-dim">{filtered.length}/{tasks.length}</span>
      <input
        bind:value={q}
        placeholder="search…"
        class="flex-1 min-w-0 px-3 py-2 bg-surface0 border border-surface1 rounded text-base sm:text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-xs sm:text-sm flex-wrap">
        <button class="px-2 sm:px-3 py-1.5 {view === 'list' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}" onclick={() => (view = 'list')}>List</button>
        <button class="px-2 sm:px-3 py-1.5 {view === 'kanban' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}" onclick={() => (view = 'kanban')}>Kanban</button>
        <button class="px-2 sm:px-3 py-1.5 {view === 'inbox' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}" onclick={() => (view = 'inbox')} title="untriaged tasks">Inbox</button>
        <button class="px-2 sm:px-3 py-1.5 hidden sm:inline-block {view === 'triage' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}" onclick={() => (view = 'triage')}>Triage</button>
        <button class="px-2 sm:px-3 py-1.5 hidden sm:inline-block {view === 'quickwins' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}" onclick={() => (view = 'quickwins')} title="high priority + ≤30 min">Quick wins</button>
        <button class="px-2 sm:px-3 py-1.5 hidden sm:inline-block {view === 'stale' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}" onclick={() => (view = 'stale')} title="not touched in 7+ days">Stale</button>
        <button class="px-2 sm:px-3 py-1.5 hidden sm:inline-block {view === 'review' ? 'bg-primary text-mantle' : 'text-subtext hover:bg-surface1'}" onclick={() => (view = 'review')} title="completed in last 7 days">Review</button>
      </div>
    </header>

    {#if view === 'list' || view === 'kanban'}
      <div class="px-3 py-2 border-b border-surface1 flex items-center gap-2 text-xs text-dim flex-shrink-0">
        {#if view === 'list'}
          <span>group</span>
          <select bind:value={groupBy} class="bg-surface0 border border-surface1 rounded px-2 py-1 text-text">
            <option value="due">due date</option>
            <option value="priority">priority</option>
            <option value="tag">tag</option>
            <option value="project">project</option>
            <option value="note">note</option>
          </select>
        {:else}
          <span>columns</span>
          <select bind:value={kanbanMode} class="bg-surface0 border border-surface1 rounded px-2 py-1 text-text">
            <option value="priority">priority</option>
            <option value="due">due</option>
            <option value="triage">triage (granit)</option>
          </select>
        {/if}
      </div>
    {/if}

    {#if selectedIds.size > 0}
      <BulkBar
        count={selectedIds.size}
        ids={Array.from(selectedIds)}
        onClear={() => (selectedIds = new Set())}
        onChanged={async () => { selectedIds = new Set(); await load(); }}
      />
    {/if}

    <div class="flex-1 overflow-auto p-3 sm:p-4">
      {#if loading && tasks.length === 0}
        <div class="text-sm text-dim">loading…</div>
      {:else if filtered.length === 0 && view === 'review'}
        <div class="text-sm text-dim italic">No tasks completed in the last 7 days. Get to work!</div>
      {:else if filtered.length === 0 && view === 'inbox'}
        <p class="text-sm text-success">Inbox empty 🎉 nothing waiting to be triaged.</p>
      {:else if filtered.length === 0 && view === 'stale'}
        <p class="text-sm text-success">No stale tasks — everything's been touched in the last week.</p>
      {:else if filtered.length === 0 && view === 'quickwins'}
        <p class="text-sm text-dim italic">No quick wins available. Add an estimate (e.g. <code class="text-secondary">est:30m</code>) to high-priority tasks.</p>
      {:else if filtered.length === 0}
        <div class="text-sm text-dim italic">no tasks match these filters.</div>
      {:else if view === 'kanban'}
        <Kanban tasks={filtered} bind:mode={kanbanMode} onChanged={load} />
      {:else if view === 'triage'}
        <TriageBoard tasks={filtered} onChanged={load} />
      {:else if view === 'inbox'}
        <div class="max-w-3xl">
          <p class="text-sm text-dim mb-4">
            Untriaged tasks. Decide for each: schedule, prioritize, drop, or snooze.
          </p>
          <div class="space-y-2">
            {#each filtered as t (t.id)}
              <TaskCard task={t} onChanged={load} bind:selectedIds onOpenDetail={openDetail} />
            {/each}
          </div>
        </div>
      {:else if view === 'stale'}
        <div class="max-w-3xl">
          <p class="text-sm text-dim mb-4">Tasks that haven't been touched in 7+ days. Drop, snooze, or do them.</p>
          <div class="space-y-2">
            {#each filtered as t (t.id)}
              <TaskCard task={t} onChanged={load} bind:selectedIds onOpenDetail={openDetail} />
            {/each}
          </div>
        </div>
      {:else if view === 'quickwins'}
        <div class="max-w-3xl">
          <p class="text-sm text-dim mb-4">High-priority tasks you can finish in ≤30 min. Pick one, knock it out.</p>
          <div class="space-y-2">
            {#each filtered as t (t.id)}
              <TaskCard task={t} onChanged={load} bind:selectedIds onOpenDetail={openDetail} />
            {/each}
          </div>
        </div>
      {:else if view === 'review'}
        <div class="max-w-3xl">
          <p class="text-sm text-dim mb-4">Done in the last week — your retrospective view.</p>
          <div class="space-y-2 opacity-80">
            {#each filtered as t (t.id)}
              <TaskCard task={t} onChanged={load} bind:selectedIds onOpenDetail={openDetail} />
            {/each}
          </div>
        </div>
      {:else}
        <div class="space-y-6 max-w-3xl">
          {#each listGroups as g (g.key)}
            <section>
              <h2 class="text-xs uppercase tracking-wider text-dim mb-2 font-medium border-b border-surface1 pb-1">
                {g.label} · {g.tasks.length}
              </h2>
              <div class="space-y-2">
                {#each g.tasks as t (t.id)}
                  <TaskCard task={t} onChanged={load} bind:selectedIds onOpenDetail={openDetail} />
                {/each}
              </div>
            </section>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>

<TaskDetail bind:open={detailOpen} task={detailTask} onChanged={async () => {
  await load();
  // Refresh the in-drawer task copy from the freshly-loaded list so subsequent
  // edits see latest state.
  if (detailTask) detailTask = tasks.find((t) => t.id === detailTask!.id) ?? detailTask;
}} />
