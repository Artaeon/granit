<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, type Task, type Project, type Goal, type Deadline } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import TaskCard from '$lib/tasks/TaskCard.svelte';
  import Kanban from '$lib/tasks/Kanban.svelte';
  import TriageBoard from '$lib/tasks/TriageBoard.svelte';
  import BulkBar from '$lib/tasks/BulkBar.svelte';
  import TaskDetail from '$lib/tasks/TaskDetail.svelte';
  import TaskContextMenu from '$lib/tasks/TaskContextMenu.svelte';
  import Drawer from '$lib/components/Drawer.svelte';

  type View = 'list' | 'kanban' | 'triage' | 'inbox' | 'stale' | 'quickwins' | 'review';
  type Group = 'due' | 'priority' | 'note' | 'project' | 'tag' | 'goal' | 'deadline';

  let tasks = $state<Task[]>([]);
  let projects = $state<Project[]>([]);
  // Goals + deadlines drive the new group-by options and the group
  // header titles (so a "Q3 launch (G004)" group reads as the goal's
  // title, not the bare ID). Loaded once, then refreshed alongside
  // the task list on WS events.
  let goals = $state<Goal[]>([]);
  let deadlines = $state<Deadline[]>([]);

  // Persist view + groupBy to localStorage so the user comes back to where they left off.
  const VIEW_KEY = 'granit.tasks.view';
  const GROUP_KEY = 'granit.tasks.groupBy';

  let view = $state<View>(
    (typeof localStorage !== 'undefined' && (localStorage.getItem(VIEW_KEY) as View)) || 'list'
  );
  let groupBy = $state<Group>(
    (typeof localStorage !== 'undefined' && (localStorage.getItem(GROUP_KEY) as Group)) || 'due'
  );
  let kanbanMode = $state<'priority' | 'due' | 'triage' | 'config'>('priority');
  let kanbanSwimlane = $state<'none' | 'project' | 'tag' | 'priority'>('none');
  let helpOpen = $state(false);
  let status = $state<'open' | 'done' | 'all'>('open');
  let q = $state('');
  let tagFilter = $state('');
  let projectFilter = $state('');
  let priorityFilter = $state<number | ''>('');
  let goalFilter = $state('');
  let deadlineFilter = $state('');
  let loading = $state(false);
  // URL sync: hydrate filter state from ?status=…&priority=…&… on
  // first load so refresh / shared links keep filters intact, and
  // mirror user-driven changes back into the URL via $effect.
  // Without this, the kanban/list filters were per-tab session state
  // — opening a P1-filtered list in a new tab silently lost the
  // filter and the user blamed "the search box".
  let urlHydrated = false;
  function hydrateFromUrl() {
    if (typeof window === 'undefined') return;
    const sp = new URL(window.location.href).searchParams;
    const get = (k: string) => sp.get(k) ?? '';
    if (sp.has('status')) {
      const s = get('status');
      if (s === 'open' || s === 'done' || s === 'all') status = s;
    }
    if (sp.has('q')) q = get('q');
    if (sp.has('tag')) tagFilter = get('tag');
    if (sp.has('project')) projectFilter = get('project');
    if (sp.has('priority')) {
      const n = Number(get('priority'));
      priorityFilter = n >= 1 && n <= 3 ? n : '';
    }
    if (sp.has('goal')) goalFilter = get('goal');
    if (sp.has('deadline')) deadlineFilter = get('deadline');
    if (sp.has('view')) {
      const v = get('view') as View;
      if (['list', 'kanban', 'triage', 'inbox', 'stale', 'quickwins', 'review'].includes(v)) view = v;
    }
    if (sp.has('group')) {
      const g = get('group') as Group;
      if (['due', 'priority', 'note', 'project', 'tag', 'goal', 'deadline'].includes(g)) groupBy = g;
    }
    urlHydrated = true;
  }
  function syncToUrl() {
    if (!urlHydrated) return;
    if (typeof window === 'undefined') return;
    const sp = new URLSearchParams();
    if (status !== 'open') sp.set('status', status);
    if (q) sp.set('q', q);
    if (tagFilter) sp.set('tag', tagFilter);
    if (projectFilter) sp.set('project', projectFilter);
    if (priorityFilter !== '') sp.set('priority', String(priorityFilter));
    if (goalFilter) sp.set('goal', goalFilter);
    if (deadlineFilter) sp.set('deadline', deadlineFilter);
    if (view !== 'list') sp.set('view', view);
    if (groupBy !== 'due') sp.set('group', groupBy);
    const qs = sp.toString();
    const next = qs ? `${$page.url.pathname}?${qs}` : $page.url.pathname;
    // replaceState (not goto) — we don't want every keystroke in the
    // search box adding to browser history.
    void goto(next, { replaceState: true, noScroll: true, keepFocus: true });
  }
  let filterDrawerOpen = $state(false);
  let selectedIds = $state<Set<string>>(new Set());
  let detailTask = $state<Task | null>(null);
  let detailOpen = $state(false);
  // Context menu state — driven by TaskCard's onContextMenu hook.
  // The menu mounts at the click position with {ctxTask, ctxX, ctxY}.
  let ctxTask = $state<Task | null>(null);
  let ctxX = $state(0);
  let ctxY = $state(0);

  function openDetail(t: Task) {
    detailTask = t;
    detailOpen = true;
  }
  function openContext(t: Task, x: number, y: number) {
    ctxTask = t;
    ctxX = x;
    ctxY = y;
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
      // Honor every server-side filter we expose. The client-side
      // `filtered` derivation still re-applies these (so view-specific
      // logic like inbox/stale stays consistent), but pushing them to
      // the server first means we don't ship the entire task graph
      // over the wire when the user wants P1 only.
      const params: Parameters<typeof api.listTasks>[0] = {};
      if (status !== 'all') params.status = status;
      if (tagFilter) params.tag = tagFilter;
      if (priorityFilter !== '') params.priority = priorityFilter;
      if (projectFilter) params.project = projectFilter;
      if (goalFilter) params.goal = goalFilter;
      if (deadlineFilter) params.deadline = deadlineFilter;
      const [list, p, gg, dd] = await Promise.all([
        api.listTasks(params),
        projects.length === 0 ? api.listProjects().catch(() => ({ projects: [] as Project[] })) : Promise.resolve({ projects }),
        goals.length === 0 ? api.listGoals().catch(() => ({ goals: [] as Goal[] })) : Promise.resolve({ goals }),
        deadlines.length === 0
          ? api.listDeadlines().catch(() => ({ deadlines: [] as Deadline[] }))
          : Promise.resolve({ deadlines })
      ]);
      tasks = list.tasks;
      projects = p.projects;
      goals = gg.goals;
      deadlines = dd.deadlines;
    } catch (e) {
      // 401 (stale auth) and network failures both end up here.
      // Silently leave tasks/projects empty so the empty-state copy
      // renders instead of the indefinite loading spinner. A later
      // WS reconnect or filter change will retry naturally.
      console.error('tasks: load failed', e);
    } finally {
      loading = false;
    }
  }

  // Single load driver: an effect that keys off $auth + filters. When
  // auth resolves (or changes) it fires; when status/tagFilter change
  // it fires. We don't pair it with onMount(load) — that would cause
  // a double-fetch on initial paint and (more importantly) was the
  // source of the "stays loading" bug when an early call set
  // loading=true before $auth was ready.
  //
  // load() is wrapped in untrack() because the function reads
  // projects.length / goals.length / deadlines.length to decide whether
  // to refetch the linkable-entity sidecars, and it reassigns those
  // arrays when fresh data lands. Without untrack, those reads would
  // become deps of THIS effect, and Svelte 5 fires reactivity on
  // $state array reassignment even when contents are equal — turning
  // a single initial fetch into a tight loop (most visible when
  // /api/v1/deadlines returns []: deadlines.length stays 0, so every
  // load() refires load(), saturating the page). The explicit `void`
  // list above is the source-of-truth for what should retrigger load.
  $effect(() => {
    void $auth;
    void status;
    void tagFilter;
    void priorityFilter;
    void projectFilter;
    void goalFilter;
    void deadlineFilter;
    untrack(() => load());
  });

  // URL-state effect — runs whenever a filter changes after hydration.
  // Skipped on the initial render so the URL doesn't get rewritten
  // before we read it back. syncToUrl reads $page.url.pathname and
  // calls goto(); both are reactive surfaces we don't want this effect
  // to depend on, so the call is untracked. The void list above is
  // the explicit dep set.
  $effect(() => {
    void status;
    void q;
    void tagFilter;
    void projectFilter;
    void priorityFilter;
    void goalFilter;
    void deadlineFilter;
    void view;
    void groupBy;
    untrack(() => syncToUrl());
  });

  onMount(() => {
    hydrateFromUrl();
  });

  onMount(() =>
    onWsEvent((ev) => {
      // task.changed fires after every patchTask, including drag-drops
      // from the kanban — without it, moves would only show up on a
      // manual refresh (or the next note write coincidentally). Match
      // the same set the calendar/inbox widgets honor.
      if (ev.type === 'note.changed' || ev.type === 'note.removed' || ev.type === 'task.changed') load();
    })
  );

  // ---------------------------------------------------------------------------
  // Keyboard shortcuts (j/k navigate, x select, e edit, d done, p priority).
  // Mirrors the TUI's task manager bindings as far as the web allows. Skipped
  // when the user is typing into an input so we don't eat letters mid-search.
  // The cursor is page-local; we only navigate within the current `filtered`
  // list. Discoverable via the '?' button in the header.
  // ---------------------------------------------------------------------------
  let cursorIdx = $state<number>(-1);
  $effect(() => {
    // Reset cursor when the filtered list shrinks past it.
    if (cursorIdx >= filtered.length) cursorIdx = filtered.length - 1;
  });

  function isTypingTarget(el: EventTarget | null): boolean {
    if (!(el instanceof HTMLElement)) return false;
    const tag = el.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true;
    if (el.isContentEditable) return true;
    return false;
  }

  async function cyclePriorityOf(t: Task) {
    const next = ((t.priority || 0) + 1) % 4; // 0,1,2,3 cycle
    try {
      await api.patchTask(t.id, { priority: next });
    } catch {}
  }

  function focusCursor(idx: number) {
    cursorIdx = Math.max(0, Math.min(filtered.length - 1, idx));
    // Scroll the focused row into view; the data-task-id attr on the
    // wrapper element gives us a stable selector across re-renders.
    const t = filtered[cursorIdx];
    if (!t) return;
    queueMicrotask(() => {
      const el = document.querySelector(`[data-task-id="${t.id}"]`) as HTMLElement | null;
      if (el) el.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    });
  }

  onMount(() => {
    function onKey(e: KeyboardEvent) {
      if (isTypingTarget(e.target)) return;
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const k = e.key;
      // Help overlay
      if (k === '?') {
        helpOpen = !helpOpen;
        e.preventDefault();
        return;
      }
      if (helpOpen && k === 'Escape') {
        helpOpen = false;
        return;
      }
      // j/k navigation
      if (k === 'j') {
        focusCursor((cursorIdx < 0 ? 0 : cursorIdx + 1));
        e.preventDefault();
        return;
      }
      if (k === 'k') {
        focusCursor((cursorIdx < 0 ? 0 : cursorIdx - 1));
        e.preventDefault();
        return;
      }
      const t = cursorIdx >= 0 ? filtered[cursorIdx] : null;
      if (!t) return;
      if (k === 'x') {
        // Toggle selection on cursor
        const next = new Set(selectedIds);
        if (next.has(t.id)) next.delete(t.id);
        else next.add(t.id);
        selectedIds = next;
        e.preventDefault();
      } else if (k === 'd') {
        api.patchTask(t.id, { done: !t.done }).catch(() => {});
        e.preventDefault();
      } else if (k === 'e') {
        openDetail(t);
        e.preventDefault();
      } else if (k === 'p') {
        cyclePriorityOf(t);
        e.preventDefault();
      } else if (k === 'Escape') {
        if (selectedIds.size > 0) {
          selectedIds = new Set();
          e.preventDefault();
        }
      }
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
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
    if (goalFilter) out = out.filter((t) => t.goalId === goalFilter);
    if (deadlineFilter) out = out.filter((t) => t.deadlineId === deadlineFilter);
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

  type ListGroup = { key: string; label: string; tasks: Task[]; deepLink?: string };
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
      return Object.entries(b)
        .map(([k, v]) => ({
          key: k,
          label: k,
          tasks: v,
          deepLink: projects.find((p) => p.name === k)
            ? `/projects/${encodeURIComponent(k)}`
            : undefined
        }))
        .sort((a, b) => a.label.localeCompare(b.label));
    }
    if (groupBy === 'goal') {
      const b: Record<string, Task[]> = {};
      for (const t of filtered) {
        const key = t.goalId || '(no goal)';
        (b[key] ??= []).push(t);
      }
      return Object.entries(b)
        .map(([k, v]) => {
          const g = goals.find((x) => x.id === k);
          return {
            key: k,
            label: g ? `🎯 ${g.title} (${g.id})` : k,
            tasks: v,
            // /goals/[id] doesn't exist as a route — the SPA shell
            // matched but the client router fell through, looking like
            // a freeze on click. Use the same-page focus param the
            // /goals page already understands.
            deepLink: g ? `/goals?focus=${encodeURIComponent(g.id)}` : undefined
          };
        })
        .sort((a, b) => {
          // Pin (no goal) to the bottom so the named buckets are surfaced first.
          if (a.key === '(no goal)') return 1;
          if (b.key === '(no goal)') return -1;
          return a.label.localeCompare(b.label);
        });
    }
    if (groupBy === 'deadline') {
      const b: Record<string, Task[]> = {};
      for (const t of filtered) {
        const key = t.deadlineId || '(no deadline)';
        (b[key] ??= []).push(t);
      }
      return Object.entries(b)
        .map(([k, v]) => {
          const d = deadlines.find((x) => x.id === k);
          return {
            key: k,
            label: d ? `⏰ ${d.title} · ${d.date}` : k,
            tasks: v,
            deepLink: d ? `/deadlines?focus=${encodeURIComponent(d.id)}` : undefined
          };
        })
        .sort((a, b) => {
          if (a.key === '(no deadline)') return 1;
          if (b.key === '(no deadline)') return -1;
          // Sort by deadline date ascending — soonest first.
          const da = deadlines.find((x) => x.id === a.key)?.date ?? '';
          const db = deadlines.find((x) => x.id === b.key)?.date ?? '';
          return da.localeCompare(db);
        });
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
    (priorityFilter !== '' ? 1 : 0) +
      (projectFilter ? 1 : 0) +
      (tagFilter ? 1 : 0) +
      (goalFilter ? 1 : 0) +
      (deadlineFilter ? 1 : 0)
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

    {#if goals.length > 0}
      <div>
        <div class="text-xs uppercase tracking-wider text-dim mb-2">Goals</div>
        <div class="flex flex-col gap-1 text-sm">
          <button
            class="text-left px-3 py-2 rounded {goalFilter === '' ? 'bg-surface1' : 'hover:bg-surface0'} text-subtext"
            onclick={() => (goalFilter = '')}
          >all</button>
          {#each goals.slice(0, 12) as g}
            <button
              class="text-left px-3 py-2 rounded text-sm truncate {goalFilter === g.id ? 'bg-info/20 text-info' : 'text-subtext hover:bg-surface0'}"
              onclick={() => (goalFilter = goalFilter === g.id ? '' : g.id)}
              title={g.description}
            >
              <span class="font-mono text-[10px] text-dim mr-1">{g.id}</span>
              {g.title}
            </button>
          {/each}
        </div>
      </div>
    {/if}

    {#if deadlines.length > 0}
      <div>
        <div class="text-xs uppercase tracking-wider text-dim mb-2">Deadlines</div>
        <div class="flex flex-col gap-1 text-sm">
          <button
            class="text-left px-3 py-2 rounded {deadlineFilter === '' ? 'bg-surface1' : 'hover:bg-surface0'} text-subtext"
            onclick={() => (deadlineFilter = '')}
          >all</button>
          {#each deadlines.slice(0, 12) as d}
            <button
              class="text-left px-3 py-2 rounded text-sm truncate {deadlineFilter === d.id ? 'bg-warning/20 text-warning' : 'text-subtext hover:bg-surface0'}"
              onclick={() => (deadlineFilter = deadlineFilter === d.id ? '' : d.id)}
              title={d.description}
            >
              <span class="font-mono text-[10px] text-dim mr-1">{d.date}</span>
              {d.title}
            </button>
          {/each}
        </div>
      </div>
    {/if}

    <button
      onclick={() => { priorityFilter = ''; projectFilter = ''; tagFilter = ''; goalFilter = ''; deadlineFilter = ''; q = ''; }}
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
          <span class="absolute -top-0.5 -right-0.5 w-4 h-4 bg-primary text-on-primary text-[10px] rounded-full flex items-center justify-center">{activeFilterCount}</span>
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
        <button class="px-2 sm:px-3 py-1.5 {view === 'list' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}" onclick={() => (view = 'list')}>List</button>
        <button class="px-2 sm:px-3 py-1.5 {view === 'kanban' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}" onclick={() => (view = 'kanban')}>Kanban</button>
        <button class="px-2 sm:px-3 py-1.5 {view === 'inbox' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}" onclick={() => (view = 'inbox')} title="untriaged tasks">Inbox</button>
        <button class="px-2 sm:px-3 py-1.5 hidden sm:inline-block {view === 'triage' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}" onclick={() => (view = 'triage')}>Triage</button>
        <button class="px-2 sm:px-3 py-1.5 hidden sm:inline-block {view === 'quickwins' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}" onclick={() => (view = 'quickwins')} title="high priority + ≤30 min">Quick wins</button>
        <button class="px-2 sm:px-3 py-1.5 hidden sm:inline-block {view === 'stale' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}" onclick={() => (view = 'stale')} title="not touched in 7+ days">Stale</button>
        <button class="px-2 sm:px-3 py-1.5 hidden sm:inline-block {view === 'review' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}" onclick={() => (view = 'review')} title="completed in last 7 days">Review</button>
      </div>
      <button
        onclick={() => (helpOpen = !helpOpen)}
        aria-label="keyboard shortcuts"
        title="keyboard shortcuts (?)"
        class="hidden sm:flex w-7 h-7 items-center justify-center text-dim hover:text-text border border-surface1 rounded text-sm"
      >?</button>
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
            <option value="goal">goal</option>
            <option value="deadline">deadline</option>
            <option value="note">note</option>
          </select>
        {:else}
          <span>columns</span>
          <select bind:value={kanbanMode} class="bg-surface0 border border-surface1 rounded px-2 py-1 text-text">
            <option value="priority">priority</option>
            <option value="due">due</option>
            <option value="triage">triage (granit)</option>
            <option value="config">config</option>
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
        <Kanban
          tasks={filtered}
          bind:mode={kanbanMode}
          bind:swimlane={kanbanSwimlane}
          bind:selectedIds
          onChanged={load}
          onOpenDetail={openDetail}
          onContextMenu={openContext}
        />
      {:else if view === 'triage'}
        <TriageBoard tasks={filtered} onChanged={load} />
      {:else if view === 'inbox'}
        <div class="max-w-3xl">
          <p class="text-sm text-dim mb-4">
            Untriaged tasks. Decide for each: schedule, prioritize, drop, or snooze.
          </p>
          <div class="space-y-2">
            {#each filtered as t (t.id)}
              <div data-task-id={t.id} class={cursorIdx >= 0 && filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                <TaskCard task={t} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
              </div>
            {/each}
          </div>
        </div>
      {:else if view === 'stale'}
        <div class="max-w-3xl">
          <p class="text-sm text-dim mb-4">Tasks that haven't been touched in 7+ days. Drop, snooze, or do them.</p>
          <div class="space-y-2">
            {#each filtered as t (t.id)}
              <div data-task-id={t.id} class={cursorIdx >= 0 && filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                <TaskCard task={t} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
              </div>
            {/each}
          </div>
        </div>
      {:else if view === 'quickwins'}
        <div class="max-w-3xl">
          <p class="text-sm text-dim mb-4">High-priority tasks you can finish in ≤30 min. Pick one, knock it out.</p>
          <div class="space-y-2">
            {#each filtered as t (t.id)}
              <div data-task-id={t.id} class={cursorIdx >= 0 && filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                <TaskCard task={t} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
              </div>
            {/each}
          </div>
        </div>
      {:else if view === 'review'}
        <div class="max-w-3xl">
          <p class="text-sm text-dim mb-4">Done in the last week — your retrospective view.</p>
          <div class="space-y-2 opacity-80">
            {#each filtered as t (t.id)}
              <div data-task-id={t.id} class={cursorIdx >= 0 && filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                <TaskCard task={t} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
              </div>
            {/each}
          </div>
        </div>
      {:else}
        <div class="space-y-6 max-w-3xl">
          {#each listGroups as g (g.key)}
            <section>
              <h2 class="text-xs uppercase tracking-wider text-dim mb-2 font-medium border-b border-surface1 pb-1 flex items-baseline gap-2">
                <span>{g.label} · {g.tasks.length}</span>
                {#if g.deepLink}
                  <a
                    href={g.deepLink}
                    class="ml-auto text-[10px] text-secondary hover:underline normal-case tracking-normal"
                    title="open {g.label}"
                  >open ↗</a>
                {/if}
              </h2>
              <div class="space-y-2">
                {#each g.tasks as t (t.id)}
                  <div data-task-id={t.id} class={cursorIdx >= 0 && filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                    <TaskCard task={t} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
                  </div>
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

{#if ctxTask}
  <TaskContextMenu
    task={ctxTask}
    x={ctxX}
    y={ctxY}
    onClose={() => (ctxTask = null)}
    onChanged={load}
    onOpenDetail={openDetail}
  />
{/if}

<!-- Keyboard shortcuts overlay. Toggled with '?' or the header button. -->
{#if helpOpen}
  <div
    class="fixed inset-0 bg-mantle/80 z-50 flex items-center justify-center p-4"
    onclick={() => (helpOpen = false)}
    role="presentation"
  >
    <div
      class="bg-surface0 border border-surface1 rounded-lg p-5 max-w-md w-full shadow-xl"
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => { if (e.key === 'Escape') helpOpen = false; }}
      role="dialog"
      aria-modal="true"
      aria-label="Keyboard shortcuts"
      tabindex="-1"
    >
      <div class="flex items-center justify-between mb-3">
        <h2 class="text-base font-semibold text-text">Keyboard shortcuts</h2>
        <button onclick={() => (helpOpen = false)} class="text-dim hover:text-text">esc</button>
      </div>
      <div class="grid grid-cols-2 gap-y-2 gap-x-4 text-sm">
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">j / k</kbd>
        <span class="text-subtext">navigate up / down</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">x</kbd>
        <span class="text-subtext">toggle bulk-select</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">e</kbd>
        <span class="text-subtext">open task detail</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">d</kbd>
        <span class="text-subtext">toggle done</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">p</kbd>
        <span class="text-subtext">cycle priority (P0→P3)</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">esc</kbd>
        <span class="text-subtext">clear selection</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">?</kbd>
        <span class="text-subtext">toggle this overlay</span>
      </div>
      <div class="mt-4 pt-3 border-t border-surface1 text-xs text-dim">
        <strong class="text-subtext">Kanban:</strong> drag cards between columns. Drag while a
        bulk-selection is active to move all selected tasks at once.
      </div>
    </div>
  </div>
{/if}
