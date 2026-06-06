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
    type Task,
    type Venture
  } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { onWsEvent } from '$lib/ws';
  import { createCoalescedReload } from '$lib/util/coalesce';
  import VisionContextStrip from '$lib/components/VisionContextStrip.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import DeadlinesPageHeader from '$lib/deadlines/DeadlinesPageHeader.svelte';
  import DeadlinesQuickFilters from '$lib/deadlines/DeadlinesQuickFilters.svelte';
  import DeadlinesListSections from '$lib/deadlines/DeadlinesListSections.svelte';
  import DeadlinesComingUp from '$lib/deadlines/DeadlinesComingUp.svelte';
  import DeadlinesTimeline from '$lib/deadlines/DeadlinesTimeline.svelte';
  import DeadlinesCalendar from '$lib/deadlines/DeadlinesCalendar.svelte';
  import DeadlineDrawer from '$lib/deadlines/DeadlineDrawer.svelte';
  import { daysUntil } from '$lib/deadlines/util';
  import {
    todayISO,
    isValidDate,
    addDaysISO,
    countdown,
    projectHref,
    goalHref,
    ventureHref
  } from '$lib/deadlines/deadlinesPageHelpers';
  import { loadStored, saveStored, loadStoredString, saveStoredString } from '$lib/util/storage';

  // Deadlines page — top-level "this matters by date X" markers backed
  // by .granit/deadlines.json. Distinct from Tasks (no checkbox / not
  // a todo) and from Goals (a goal has milestones / progress; a
  // deadline is a single hard moment in time, possibly linked to a
  // goal). Three view modes:
  //   list      — flat, group-by toggle (urgency / status / month)
  //   timeline  — vertical year-spanning rail of bands
  //   calendar  — month-grid heat view
  // Active view + group-by are persisted in localStorage so the
  // user's last layout reopens with them.

  type ViewMode = 'list' | 'timeline' | 'calendar';
  type GroupBy = 'urgency' | 'status' | 'month';

  const VIEW_KEY = 'granit.deadlines.view';
  const GROUP_KEY = 'granit.deadlines.groupby';
  const COLLAPSE_KEY = 'granit.deadlines.collapsedSections';

  // Validate the persisted string against the union — tolerate a stale
  // key from an older version that no longer maps to a real view.
  const VIEW_VALUES = ['list', 'timeline', 'calendar'] as const;
  const GROUP_VALUES = ['urgency', 'status', 'month'] as const;
  const loadView = (): ViewMode => {
    const v = loadStoredString(VIEW_KEY, 'list');
    return (VIEW_VALUES as readonly string[]).includes(v) ? (v as ViewMode) : 'list';
  };
  const loadGroup = (): GroupBy => {
    const v = loadStoredString(GROUP_KEY, 'urgency');
    return (GROUP_VALUES as readonly string[]).includes(v) ? (v as GroupBy) : 'urgency';
  };

  let viewMode = $state<ViewMode>(loadView());
  let groupBy = $state<GroupBy>(loadGroup());
  // Section collapse state — Record<bucketKey, boolean>. Persisted so
  // a user who collapsed "Met" once never has to do it again on
  // reload. Absence falls through to the bucket-tone default in the
  // DeadlinesListSections helper (live buckets open, archive buckets
  // collapsed).
  let collapsedSections = $state<Record<string, boolean>>(
    loadStored<Record<string, boolean>>(COLLAPSE_KEY, {})
  );

  $effect(() => saveStoredString(VIEW_KEY, viewMode));
  $effect(() => saveStoredString(GROUP_KEY, groupBy));
  $effect(() => saveStored(COLLAPSE_KEY, collapsedSections));

  let deadlines = $state<Deadline[]>([]);
  let goals = $state<Goal[]>([]);
  let projects = $state<Project[]>([]);
  let ventures = $state<Venture[]>([]);
  // Open tasks pool used by the "link to tasks" multi-select. Loaded
  // lazily on drawer open so the page paints fast even on big vaults.
  let openTasks = $state<Task[]>([]);
  let tasksLoaded = $state(false);
  let loading = $state(false);
  let busy = $state(false);

  // Active importance filter — null = show all; otherwise filter to
  // the matching importance value. The chip row at the top reads + writes this.
  let importanceFilter = $state<DeadlineImportance | null>(null);

  // Title-substring quick filter — typing in the search box narrows
  // the visible set. Cheap on the client; deadlines.json rarely runs
  // to thousands of rows.
  let q = $state('');

  // Optional URL-driven scope filters (e.g. /deadlines?project=Foo or
  // ?goal_id=G123). Used by the note-page deadline strip to deep-link
  // to "show me everything tied to this thing". Reactive — survives
  // SPA query-only navigations.
  let scopeProject = $derived($page.url.searchParams.get('project') ?? '');
  let scopeGoalId = $derived($page.url.searchParams.get('goal_id') ?? '');
  let scopeVenture = $derived($page.url.searchParams.get('venture') ?? '');

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
  let fVenture = $state('');
  let fTaskIds = $state<string[]>([]);

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      const [dl, gl, pl, vl] = await Promise.all([
        api.listDeadlines(),
        api.listGoals().catch(() => ({ goals: [] as Goal[], total: 0 })),
        api.listProjects().catch(() => ({ projects: [] as Project[], total: 0 })),
        api.listVentures().catch(() => ({ ventures: [] as Venture[], total: 0 }))
      ]);
      deadlines = dl.deadlines;
      goals = gl.goals;
      projects = pl.projects;
      ventures = vl.ventures;
    } catch (e) {
      toast.error('load failed: ' + (errorMessage(e)));
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

  // Coalesced reload — load() runs four parallel API calls
  // (deadlines, tasks, projects, goals) and rebuilds the derived
  // scoping/grouping views. A triage burst that fires many
  // task.changed events in a row used to kick off a full refetch
  // per event; one trailing-edge reload per window suffices.
  const reload = createCoalescedReload(() => load(), 600);

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      // Refetch on the server's deadlines.json signal AND on note/task
      // changes — the latter two refresh the linked task chips when the
      // underlying task gets renamed or completed.
      if (ev.type === 'state.changed' && ev.path === '.granit/deadlines.json') reload.trigger();
      if (ev.type === 'task.changed' || ev.type === 'note.changed') reload.trigger();
    });
  });

  // ----- Importance filter -----
  //
  // Counts roll over the FULL list (so the chip row shows the global
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
  // counts in the chip row reflect the scoped subset, not the entire
  // vault — matches the user's mental model when they land here via
  // "deadlines for this project".
  let scoped = $derived.by(() => {
    let out = deadlines;
    if (scopeProject) out = out.filter((d) => d.project === scopeProject);
    if (scopeGoalId) out = out.filter((d) => d.goal_id === scopeGoalId);
    if (scopeVenture) out = out.filter((d) => d.venture === scopeVenture);
    return out;
  });

  let filtered = $derived.by(() => {
    let out = scoped;
    if (importanceFilter) out = out.filter((d) => d.importance === importanceFilter);
    const term = q.trim().toLowerCase();
    if (term) {
      out = out.filter((d) =>
        d.title.toLowerCase().includes(term) ||
        (d.description ?? '').toLowerCase().includes(term) ||
        (d.project ?? '').toLowerCase().includes(term) ||
        (d.venture ?? '').toLowerCase().includes(term)
      );
    }
    return out;
  });

  // ----- Grouping for the list -----
  //
  // Group-by 'urgency' (default): "Overdue" | "This week" | "This
  // month" | "Later" | "Met" | "Cancelled". Met used to fold into
  // "Done" alongside cancelled, but the user explicitly wants past
  // wins to stay visible — so we render "Met" as its own tail group
  // and roll cancelled into a quieter bucket below it.
  //
  // Group-by 'status': active / missed / met / cancelled — useful
  // when triaging "what's still open" across all dates.
  //
  // Group-by 'month': "May 2026", "Jun 2026", etc — calendar-year
  // mental model, matches how birthdays and exams are usually
  // mentally bucketed.

  type Bucket = string;
  const urgencyOrder: Bucket[] = ['overdue', 'this_week', 'this_month', 'later', 'met', 'cancelled'];
  const urgencyLabel: Record<string, string> = {
    overdue: 'Overdue',
    this_week: 'This week',
    this_month: 'This month',
    later: 'Later',
    met: 'Met',
    cancelled: 'Cancelled'
  };
  const statusOrder: Bucket[] = ['active', 'missed', 'met', 'cancelled'];
  const statusLabel: Record<string, string> = {
    active: 'Active',
    missed: 'Missed',
    met: 'Met',
    cancelled: 'Cancelled'
  };

  function urgencyBucket(d: Deadline): Bucket {
    if (d.status === 'met') return 'met';
    if (d.status === 'cancelled') return 'cancelled';
    const days = daysUntil(d.date);
    if (days < 0) return 'overdue';
    if (days <= 7) return 'this_week';
    if (days <= 31) return 'this_month';
    return 'later';
  }

  function statusBucket(d: Deadline): Bucket {
    return d.status ?? 'active';
  }

  function monthBucket(d: Deadline): Bucket {
    // 'YYYY-MM' as the bucket key; rendered with the localised month name.
    return d.date.slice(0, 7);
  }
  function monthLabel(key: string): string {
    const [y, m] = key.split('-').map(Number);
    if (!y || !m) return key;
    const dt = new Date(y, m - 1, 1);
    return dt.toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
  }

  // Each bucket is a Map of [key, rows] preserving insertion order
  // so the section render order matches our intended display order.
  let grouped = $derived.by(() => {
    const out = new Map<Bucket, Deadline[]>();
    if (groupBy === 'urgency') {
      for (const k of urgencyOrder) out.set(k, []);
      for (const d of filtered) out.get(urgencyBucket(d))!.push(d);
    } else if (groupBy === 'status') {
      for (const k of statusOrder) out.set(k, []);
      for (const d of filtered) {
        const b = statusBucket(d);
        if (!out.has(b)) out.set(b, []);
        out.get(b)!.push(d);
      }
    } else {
      // month — keys are the YYYY-MM in chronological order. We
      // collect first then sort, which is cheap (deadlines.json is
      // small enough that O(n log n) once isn't worth optimising).
      const tmp = new Map<string, Deadline[]>();
      for (const d of filtered) {
        const k = monthBucket(d);
        if (!tmp.has(k)) tmp.set(k, []);
        tmp.get(k)!.push(d);
      }
      const keys = Array.from(tmp.keys()).sort();
      for (const k of keys) out.set(k, tmp.get(k)!);
    }
    return out;
  });

  function bucketTitle(b: Bucket): string {
    if (groupBy === 'urgency') return urgencyLabel[b] ?? b;
    if (groupBy === 'status') return statusLabel[b] ?? b;
    return monthLabel(b);
  }

  // Bucket header tint — drives the section heading color so the eye
  // lands on Overdue / This week first under urgency, on Active under
  // status, and on the urgency of the bucket's first row under month.
  function bucketTone(b: Bucket): string {
    if (groupBy === 'urgency') {
      switch (b) {
        case 'overdue': return 'error';
        case 'this_week': return 'warning';
        case 'this_month': return 'info';
        case 'met': return 'success';
        default: return 'dim';
      }
    }
    if (groupBy === 'status') {
      switch (b) {
        case 'active': return 'info';
        case 'missed': return 'error';
        case 'met': return 'success';
        default: return 'dim';
      }
    }
    // month — tint by how close the bucket is to today.
    const [y, m] = b.split('-').map(Number);
    if (!y || !m) return 'dim';
    const now = new Date();
    const monthsAhead = (y - now.getFullYear()) * 12 + (m - 1 - now.getMonth());
    if (monthsAhead < 0) return 'dim';
    if (monthsAhead === 0) return 'warning';
    if (monthsAhead <= 1) return 'info';
    return 'secondary';
  }

  function toggleSection(key: string) {
    // Three-state toggle: undefined → true → false → undefined. The
    // undefined state means "use the default for this bucket tone" so
    // the user can return to defaults by clicking twice.
    const cur = collapsedSections[key];
    if (cur === undefined) collapsedSections = { ...collapsedSections, [key]: true };
    else if (cur === true) collapsedSections = { ...collapsedSections, [key]: false };
    else {
      const { [key]: _drop, ...rest } = collapsedSections;
      collapsedSections = rest;
    }
  }

  // ----- Stat strip -----
  // One-line "shape of your deadlines" summary, computed from the
  // SCOPED list (so deep-links from a project show project-only
  // counts) but BEFORE the importance filter — the strip is a global
  // glance, not a filtered view. Reads from `grouped` would be
  // wrong: that's already filtered. Recompute from `scoped` directly.
  let stats = $derived.by(() => {
    let overdue = 0, thisWeek = 0, thisMonth = 0, later = 0, met = 0;
    for (const d of scoped) {
      if (d.status === 'cancelled') continue;
      if (d.status === 'met') { met++; continue; }
      const days = daysUntil(d.date);
      if (days < 0) overdue++;
      else if (days <= 7) thisWeek++;
      else if (days <= 31) thisMonth++;
      else later++;
    }
    return { overdue, thisWeek, thisMonth, later, met };
  });

  // ----- Drawer / form -----

  function openCreate() {
    editing = null;
    fTitle = '';
    fDate = todayISO();
    fDescription = '';
    fImportance = 'normal';
    fStatus = 'active';
    // Pre-fill linkage from URL scope so the entity-detail "+ add"
    // jump lands here with project/goal/venture already populated.
    fGoalId = scopeGoalId;
    fProject = scopeProject;
    fVenture = scopeVenture;
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
    fVenture = d.venture ?? '';
    fTaskIds = [...(d.task_ids ?? [])];
    drawerOpen = true;
    void ensureTasksLoaded();
  }

  // Hydrate the create drawer from ?new=1 on mount so the entity
  // detail "+ add" buttons land in the open state. Done as an effect
  // (not in load) so a subsequent navigation back here picks up the
  // param the second time too.
  let urlNewHandled = $state(false);
  $effect(() => {
    if (urlNewHandled) return;
    if (!loading && deadlines !== undefined) {
      const sp = $page.url.searchParams;
      if (sp.get('new') === '1') {
        urlNewHandled = true;
        openCreate();
      }
    }
  });

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
        venture: fVenture.trim() || undefined,
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
      toast.error('save failed: ' + (errorMessage(e)));
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
      toast.error('delete failed: ' + (errorMessage(e)));
    } finally {
      busy = false;
    }
  }

  // Inline quick-actions on a row. Don't open the drawer — fire-and-
  // forget patches that the WS broadcast will reconcile on the next
  // refetch. We optimistically toast on success/failure so the user
  // gets feedback without a layout shift.
  async function markMet(d: Deadline, e: MouseEvent) {
    e.stopPropagation();
    try {
      await api.patchDeadline(d.id, { status: 'met' });
      toast.success(`✓ ${d.title}`);
      await load();
    } catch (err) {
      toast.error('mark met failed: ' + (errorMessage(err)));
    }
  }
  async function snooze(d: Deadline, days: number, e: MouseEvent) {
    e.stopPropagation();
    try {
      await api.patchDeadline(d.id, { date: addDaysISO(d.date, days) });
      toast.success(`snoozed +${days}d`);
      await load();
    } catch (err) {
      toast.error('snooze failed: ' + (errorMessage(err)));
    }
  }
  async function reopen(d: Deadline, e: MouseEvent) {
    e.stopPropagation();
    try {
      await api.patchDeadline(d.id, { status: 'active' });
      toast.success('reopened');
      await load();
    } catch (err) {
      toast.error('reopen failed: ' + (errorMessage(err)));
    }
  }

  function toggleTaskLink(id: string) {
    if (fTaskIds.includes(id)) fTaskIds = fTaskIds.filter((x) => x !== id);
    else fTaskIds = [...fTaskIds, id];
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

  function setFilter(v: DeadlineImportance | null) {
    importanceFilter = importanceFilter === v ? null : v;
  }

  // ----- "Coming up" hero strip -----
  // Three most-urgent active rows (critical→high→normal, then
  // earliest date). Replaces the single hero card on first paint
  // because the user often has multiple critical things stacked and
  // showing only one hides the next-on-deck. Falls back to whatever's
  // available when fewer than three are upcoming.
  let comingUp = $derived.by(() => {
    const active = scoped.filter((d) => d.status !== 'met' && d.status !== 'cancelled');
    const importanceRank: Record<DeadlineImportance, number> = { critical: 0, high: 1, normal: 2 };
    return [...active]
      .sort((a, b) => {
        const ra = importanceRank[a.importance] ?? 2;
        const rb = importanceRank[b.importance] ?? 2;
        if (ra !== rb) return ra - rb;
        return a.date.localeCompare(b.date);
      })
      .slice(0, 3);
  });

  // ----- Timeline view -----
  // Vertical rail of all (filtered) deadlines, sorted earliest-first,
  // visually grouped by month. Row position on the rail communicates
  // urgency; gap height between rows is proportional to days-between
  // (clamped) so distant deadlines visibly drift apart from clustered ones.
  let timelineRows = $derived.by(() => {
    return [...filtered].sort((a, b) => a.date.localeCompare(b.date));
  });

  // ----- Keyboard shortcuts -----
  //
  //   n        new deadline
  //   /        focus title-search box
  //   1/2/3    toggle Critical / High / Normal chip
  //   v        cycle view mode (list → timeline → calendar → list)
  //   g        cycle group-by (urgency → status → month → urgency)
  //   esc      clear filter / blur search (when search has focus)
  //
  // We bail when the drawer is open or focus is inside any input,
  // textarea or select — global single-letter binds otherwise eat
  // typing. The search box has its own key handler to intercept Esc
  // before it reaches us.
  let searchInput = $state<HTMLInputElement | null>(null);
  // Shortcuts-help popover. Discoverability for the keybinds — most
  // users don't read tooltips, but a "?" chip in the toolbar is a
  // well-established affordance (Linear, GitHub, Notion all use it).
  let shortcutsOpen = $state(false);
  function onPageKey(e: KeyboardEvent) {
    if (drawerOpen) return;
    const t = e.target as HTMLElement | null;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable)) {
      return;
    }
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    switch (e.key) {
      case '?':
        e.preventDefault();
        shortcutsOpen = !shortcutsOpen;
        break;
      case 'n':
        e.preventDefault();
        openCreate();
        break;
      case '/':
        e.preventDefault();
        searchInput?.focus();
        searchInput?.select();
        break;
      case '1':
        e.preventDefault();
        setFilter('critical');
        break;
      case '2':
        e.preventDefault();
        setFilter('high');
        break;
      case '3':
        e.preventDefault();
        setFilter('normal');
        break;
      case 'v': {
        e.preventDefault();
        const order: ViewMode[] = ['list', 'timeline', 'calendar'];
        viewMode = order[(order.indexOf(viewMode) + 1) % order.length];
        break;
      }
      case 'g': {
        e.preventDefault();
        const order: GroupBy[] = ['urgency', 'status', 'month'];
        groupBy = order[(order.indexOf(groupBy) + 1) % order.length];
        break;
      }
      case 'Escape':
        if (shortcutsOpen) {
          e.preventDefault();
          shortcutsOpen = false;
        } else if (importanceFilter || q) {
          e.preventDefault();
          importanceFilter = null;
          q = '';
        }
        break;
    }
  }

  // Search box keydown — intercepts Escape to blur + clear before the
  // global handler sees it (so Esc-in-search means "leave search" not
  // "clear filters and bring me out").
  function onSearchKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      (e.target as HTMLInputElement).blur();
      q = '';
    }
  }
</script>

<svelte:window on:keydown={onPageKey} />

<div class="h-full overflow-y-auto">
  <!-- Slim page header — title, count chip, view picker, group-by,
       search, help, primary "+ New deadline" button. Replaces the
       prior PageHeader strip + view-mode toolbar + group-by toolbar
       + search row, collapsing four rows of chrome into one. -->
  <DeadlinesPageHeader
    view={viewMode}
    {groupBy}
    totalCount={scoped.length}
    filteredCount={filtered.length}
    {q}
    bind:searchEl={searchInput}
    {shortcutsOpen}
    onSelectView={(v) => (viewMode = v)}
    onSelectGroup={(g) => (groupBy = g)}
    onSearchChange={(v) => (q = v)}
    onSearchKey={onSearchKey}
    onToggleShortcuts={() => (shortcutsOpen = !shortcutsOpen)}
    onCloseShortcuts={() => (shortcutsOpen = false)}
    onCreate={openCreate}
  />

  <div class="p-4 sm:p-6 lg:p-8 max-w-5xl mx-auto">
    <VisionContextStrip />

    {#if scopeProject || scopeGoalId || scopeVenture}
      <div class="mb-4 flex items-center gap-2 text-xs px-3 py-2 bg-surface1 border border-surface2 rounded">
        <span class="text-secondary">
          {#if scopeProject}📁 Scope: project <strong>{scopeProject}</strong>{/if}
          {#if scopeGoalId}🎯 Scope: goal <strong>{goalTitle(scopeGoalId)}</strong>{/if}
          {#if scopeVenture}🏢 Scope: venture <strong>{scopeVenture}</strong>{/if}
        </span>
        <span class="text-dim">· {scoped.length} {scoped.length === 1 ? 'deadline' : 'deadlines'}</span>
        <a href="/deadlines" class="ml-auto text-dim hover:text-text">× clear scope</a>
      </div>
    {/if}

    {#if loading && deadlines.length === 0}
      <!-- Skeleton — three rectangles approximating the hero strip
           plus a stack of list rows. Keeps the page from looking
           "broken" on a slow network without bouncing the layout when
           real content arrives. -->
      <div class="space-y-5">
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <Skeleton class="h-24 w-full" />
          <Skeleton class="h-24 w-full" />
          <Skeleton class="h-24 w-full" />
        </div>
        <div class="space-y-2">
          {#each Array(5) as _, i (i)}
            <Skeleton class="h-14 w-full" />
          {/each}
        </div>
      </div>
    {:else if deadlines.length === 0}
      <EmptyState
        icon="📅"
        title="No deadlines yet"
        description="Capture the next exam, launch, birthday, or contract date. Deadlines are calm anchors — Granit keeps the slipping ones at the top so you can never miss the one that matters."
      >
        {#snippet action()}
          <button
            onclick={openCreate}
            class="px-4 py-2 bg-primary text-on-primary text-sm font-medium rounded hover:opacity-90"
          >+ Add your first deadline</button>
        {/snippet}
      </EmptyState>
    {:else}
      <!-- Coming-up strip — top 3 most-urgent active rows. Shows three
           cards on desktop, stacks on mobile. Mirrors the old single-
           hero layout but tighter so three fit. -->
      <DeadlinesComingUp rows={comingUp} {goalTitle} onOpen={openEdit} />

      <!-- Stat strip — one-line "shape of today's commitments" so the
           user sees what's slipping vs. distant in a single glance.
           Hides zero buckets to stay tidy on a young vault. -->
      {#if stats.overdue + stats.thisWeek + stats.thisMonth + stats.later + stats.met > 0}
        <div class="flex flex-wrap items-baseline gap-x-4 gap-y-1 mb-3 text-xs">
          {#if stats.overdue > 0}
            <span class="text-error font-medium tabular-nums">{stats.overdue} overdue</span>
          {/if}
          {#if stats.thisWeek > 0}
            <span class="text-warning tabular-nums">{stats.thisWeek} this week</span>
          {/if}
          {#if stats.thisMonth > 0}
            <span class="text-info tabular-nums">{stats.thisMonth} this month</span>
          {/if}
          {#if stats.later > 0}
            <span class="text-dim tabular-nums">{stats.later} later</span>
          {/if}
          {#if stats.met > 0}
            <span class="text-success/80 tabular-nums">{stats.met} met</span>
          {/if}
        </div>
      {/if}

      <!-- Importance quick-filter chips — All / Critical / High /
           Normal. Tone-tinted so the active filter reads off the
           colour. Standalone row (no view-mode-conditional layout)
           because importance applies to every view. -->
      <div class="mb-3">
        <DeadlinesQuickFilters
          importance={importanceFilter}
          {q}
          counts={importanceCounts}
          onSet={setFilter}
          onClearAll={() => { importanceFilter = null; q = ''; }}
        />
      </div>

      {#if importanceFilter || q}
        <div class="mb-3 text-xs text-dim flex items-center gap-1.5">
          <span>Showing {filtered.length} of {scoped.length}</span>
          <button
            type="button"
            onclick={() => { importanceFilter = null; q = ''; }}
            class="px-1.5 py-0.5 rounded text-dim hover:text-text hover:bg-surface1"
          >× clear</button>
        </div>
      {/if}

      <!-- Body — view-mode switch -->
      {#if viewMode === 'list'}
        <DeadlinesListSections
          {grouped}
          {groupBy}
          {bucketTitle}
          {bucketTone}
          {countdown}
          {goalTitle}
          {projectHref}
          {goalHref}
          {ventureHref}
          {collapsedSections}
          onToggleSection={toggleSection}
          onOpen={openEdit}
          onMarkMet={markMet}
          onSnooze={snooze}
          onReopen={reopen}
        />
        {#if filtered.length === 0}
          <div class="text-sm text-dim italic">
            No deadlines match your filters.
            <button
              type="button"
              onclick={() => { importanceFilter = null; q = ''; }}
              class="text-secondary hover:underline ml-1"
            >clear →</button>
          </div>
        {/if}
      {:else if viewMode === 'timeline'}
        <DeadlinesTimeline
          rows={timelineRows}
          {countdown}
          {monthLabel}
          {monthBucket}
          {goalTitle}
          onOpen={openEdit}
        />
      {:else if viewMode === 'calendar'}
        <DeadlinesCalendar
          {filtered}
          {todayISO}
          onOpen={openEdit}
          onCreateOn={(iso) => { openCreate(); fDate = iso; }}
        />
      {/if}
    {/if}
  </div>
</div>

<DeadlineDrawer
  bind:open={drawerOpen}
  {editing}
  {busy}
  bind:fTitle
  bind:fDate
  bind:fDescription
  bind:fImportance
  bind:fStatus
  bind:fGoalId
  bind:fProject
  bind:fVenture
  {fTaskIds}
  {goals}
  {projects}
  {ventures}
  {openTasks}
  {tasksLoaded}
  onSave={save}
  onDelete={remove}
  onClose={() => (drawerOpen = false)}
  onToggleTaskLink={toggleTaskLink}
/>

