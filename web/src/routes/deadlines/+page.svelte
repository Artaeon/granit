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
  import Drawer from '$lib/components/Drawer.svelte';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import VisionContextStrip from '$lib/components/VisionContextStrip.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import DeadlinePill from '$lib/deadlines/DeadlinePill.svelte';
  import { daysUntil, pickHeroDeadline } from '$lib/deadlines/util';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';

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

  $effect(() => saveStoredString(VIEW_KEY, viewMode));
  $effect(() => saveStoredString(GROUP_KEY, groupBy));

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
  // the matching importance value. The toggle bar at the top reads + writes this.
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

  function todayISO(): string {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  }

  function isValidDate(s: string): boolean {
    return /^\d{4}-\d{2}-\d{2}$/.test(s);
  }

  // Add N days to an ISO YYYY-MM-DD string and return the same format.
  // Used by snooze quick-actions. Local-time arithmetic (matches
  // daysUntil's local-midnight semantics).
  function addDaysISO(iso: string, n: number): string {
    const [y, m, d] = iso.split('-').map(Number);
    const dt = new Date(y, m - 1, d);
    dt.setDate(dt.getDate() + n);
    return `${dt.getFullYear()}-${String(dt.getMonth() + 1).padStart(2, '0')}-${String(dt.getDate()).padStart(2, '0')}`;
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

  // ----- Calendar (month-grid) view -----
  // Cursor is the first-of-month being shown. ←/→ buttons step it.
  let calCursor = $state(new Date(new Date().getFullYear(), new Date().getMonth(), 1));
  function calStep(n: number) {
    const dt = new Date(calCursor);
    dt.setMonth(dt.getMonth() + n);
    calCursor = dt;
  }
  function calLabel(): string {
    return calCursor.toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
  }
  let calCells = $derived.by(() => {
    // 6 weeks x 7 days, aligned to Monday-start. Each cell holds the
    // ISO date string + the deadlines (post-filter) due that day.
    const first = new Date(calCursor);
    // Monday-of-grid = the Monday on or before the 1st. JS Sunday=0 → shift.
    const dow = (first.getDay() + 6) % 7;
    const start = new Date(first);
    start.setDate(first.getDate() - dow);
    const cells: { iso: string; date: Date; rows: Deadline[]; inMonth: boolean }[] = [];
    const byDate = new Map<string, Deadline[]>();
    for (const d of filtered) {
      if (!byDate.has(d.date)) byDate.set(d.date, []);
      byDate.get(d.date)!.push(d);
    }
    for (let i = 0; i < 42; i++) {
      const dt = new Date(start);
      dt.setDate(start.getDate() + i);
      const iso = `${dt.getFullYear()}-${String(dt.getMonth() + 1).padStart(2, '0')}-${String(dt.getDate()).padStart(2, '0')}`;
      cells.push({
        iso,
        date: dt,
        rows: byDate.get(iso) ?? [],
        inMonth: dt.getMonth() === calCursor.getMonth()
      });
    }
    return cells;
  });
  const weekdayLabels = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
  function isTodayISO(iso: string): boolean {
    return iso === todayISO();
  }
  function calRowTone(d: Deadline): string {
    if (d.status === 'met') return 'success';
    if (d.status === 'cancelled') return 'dim';
    if (d.importance === 'critical') return 'error';
    if (d.importance === 'high') return 'warning';
    return 'info';
  }

  // ----- Timeline view -----
  // Vertical rail of all (filtered) deadlines, sorted earliest-first,
  // visually grouped by month. Row position on the rail communicates
  // urgency; gap height between rows is proportional to days-between
  // (clamped) so distant deadlines visibly drift apart from clustered ones.
  let timelineRows = $derived.by(() => {
    return [...filtered].sort((a, b) => a.date.localeCompare(b.date));
  });

  // ----- Cross-link helpers -----
  // The deadline's `project` / `goal_id` / `venture` chips on the row
  // become real links to the corresponding entity-detail page so the
  // user can pivot from a deadline to its parent context in one click.
  // We stopPropagation in the handler so the chip click doesn't also
  // open the deadline drawer.
  function projectHref(name: string): string {
    return `/projects?p=${encodeURIComponent(name)}`;
  }
  function goalHref(id: string): string {
    return `/goals?focus=${encodeURIComponent(id)}`;
  }
  function ventureHref(name: string): string {
    return `/ventures?v=${encodeURIComponent(name)}`;
  }

  // ----- Keyboard shortcuts -----
  //
  //   n        new deadline
  //   /        focus title-search box
  //   1/2/3    toggle Critical / High / Normal pill
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
</script>

<svelte:window on:keydown={onPageKey} />

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-5xl mx-auto">
    <VisionContextStrip />
    <PageHeader
      title="Deadlines"
      subtitle="dated commitments — linked to goals, projects, or tasks"
    >
      {#snippet actions()}
        <button
          onclick={openCreate}
          class="px-3 py-1.5 bg-primary text-on-primary text-sm font-medium rounded hover:opacity-90"
          title="New deadline (n)"
        >+ New deadline</button>
      {/snippet}
    </PageHeader>

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
           cards on desktop, stacks on mobile. Each card mirrors the
           old single-hero layout but tighter so three fit. -->
      {#if comingUp.length > 0}
        <div class="mb-5">
          <div class="text-[11px] uppercase tracking-wider text-dim mb-2">Coming up</div>
          <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
            {#each comingUp as h (h.id)}
              {@const days = daysUntil(h.date)}
              <button
                type="button"
                onclick={() => openEdit(h)}
                class="text-left block p-3 sm:p-4 bg-surface0 border-l-4 rounded-lg hover:border-primary transition-colors"
                style="border-left-color: {heroBorder(days)};"
              >
                <div class="flex items-start gap-2.5">
                  <span class="text-xl flex-shrink-0 mt-0.5" aria-hidden="true">
                    {h.importance === 'critical' ? '🔴' : h.importance === 'high' ? '🟠' : '🟢'}
                  </span>
                  <div class="flex-1 min-w-0">
                    <div class="text-sm sm:text-base font-semibold text-text leading-tight truncate" title={h.title}>
                      {h.title}
                    </div>
                    <div class="mt-1 text-xs">
                      {#if days < 0}
                        <span class="text-error font-medium">{Math.abs(days)}d overdue</span>
                      {:else if days === 0}
                        <span class="text-error font-medium">Today</span>
                      {:else if days === 1}
                        <span class="text-warning font-medium">Tomorrow</span>
                      {:else if days <= 7}
                        <span class="text-warning font-medium">in {days} days</span>
                      {:else if days <= 31}
                        <span class="text-info font-medium">in {days} days</span>
                      {:else}
                        <span class="text-secondary font-medium">in {days} days</span>
                      {/if}
                      <span class="text-dim ml-1">· {h.date}</span>
                    </div>
                    {#if h.project || h.goal_id || h.venture}
                      <div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 mt-1.5 text-[11px] text-dim">
                        {#if h.venture}<span class="text-secondary truncate">🏢 {h.venture}</span>{/if}
                        {#if h.goal_id}<span class="text-secondary truncate">🎯 {goalTitle(h.goal_id)}</span>{/if}
                        {#if h.project}<span class="text-secondary truncate">📁 {h.project}</span>{/if}
                      </div>
                    {/if}
                  </div>
                </div>
              </button>
            {/each}
          </div>
        </div>
      {/if}

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

      <!-- Toolbar — view-mode segment, group-by segment (only in list
           view), search box, importance pills. Wraps cleanly on mobile.
           Keyboard hints in title attrs match the on-page binds. -->
      <div class="flex flex-wrap items-center gap-2 mb-4">
        <!-- View mode -->
        <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-xs" role="tablist" aria-label="View mode">
          {#each [
            { v: 'list' as ViewMode, label: 'List', icon: '☰' },
            { v: 'timeline' as ViewMode, label: 'Timeline', icon: '┊' },
            { v: 'calendar' as ViewMode, label: 'Calendar', icon: '▦' }
          ] as o}
            <button
              type="button"
              role="tab"
              aria-selected={viewMode === o.v}
              onclick={() => (viewMode = o.v)}
              class="px-2.5 py-1.5 transition-colors {viewMode === o.v
                ? 'bg-primary text-on-primary'
                : 'text-subtext hover:bg-surface1'}"
              title="{o.label} view (v to cycle)"
            ><span class="font-mono mr-1">{o.icon}</span>{o.label}</button>
          {/each}
        </div>

        <!-- Group-by — only meaningful in list view -->
        {#if viewMode === 'list'}
          <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-xs" role="tablist" aria-label="Group by">
            <span class="px-2 py-1.5 text-dim self-center">Group:</span>
            {#each [
              { v: 'urgency' as GroupBy, label: 'Urgency' },
              { v: 'status' as GroupBy, label: 'Status' },
              { v: 'month' as GroupBy, label: 'Month' }
            ] as o}
              <button
                type="button"
                role="tab"
                aria-selected={groupBy === o.v}
                onclick={() => (groupBy = o.v)}
                class="px-2.5 py-1.5 transition-colors {groupBy === o.v
                  ? 'bg-primary text-on-primary'
                  : 'text-subtext hover:bg-surface1'}"
                title="Group by {o.label.toLowerCase()} (g to cycle)"
              >{o.label}</button>
            {/each}
          </div>
        {/if}

        <!-- Search — narrows by title/description/project/venture -->
        <div class="relative flex-1 min-w-[12rem] max-w-sm">
          <input
            bind:this={searchInput}
            bind:value={q}
            type="text"
            placeholder="Search title… (/)"
            class="w-full pl-7 pr-2 py-1.5 bg-surface0 border border-surface1 rounded text-xs text-text focus:outline-none focus:border-primary"
            onkeydown={(e) => { if (e.key === 'Escape') { (e.target as HTMLInputElement).blur(); q = ''; } }}
          />
          <span class="absolute left-2 top-1/2 -translate-y-1/2 text-dim text-xs" aria-hidden="true">⌕</span>
          {#if q}
            <button
              type="button"
              onclick={() => (q = '')}
              class="absolute right-1.5 top-1/2 -translate-y-1/2 text-dim hover:text-text text-xs"
              aria-label="clear search"
            >×</button>
          {/if}
        </div>

        <!-- Shortcuts help — "?" chip opens an inline popover listing
             page-scoped keybinds. Closes on Escape or click-outside (we
             attach a global click capture only while open). -->
        <div class="relative">
          <button
            type="button"
            onclick={() => (shortcutsOpen = !shortcutsOpen)}
            aria-expanded={shortcutsOpen}
            aria-label="Keyboard shortcuts"
            title="Keyboard shortcuts (?)"
            class="w-7 h-7 flex items-center justify-center rounded border border-surface1 text-dim hover:text-text hover:border-surface2 text-xs"
          >?</button>
          {#if shortcutsOpen}
            <button
              type="button"
              aria-label="close shortcuts"
              onclick={() => (shortcutsOpen = false)}
              class="fixed inset-0 z-30 cursor-default"
            ></button>
            <div
              role="dialog"
              aria-label="Keyboard shortcuts"
              class="absolute right-0 mt-1 z-40 w-64 bg-surface0 border border-surface1 rounded-lg shadow-lg p-3 text-xs"
            >
              <div class="text-[10px] uppercase tracking-wider text-dim font-medium mb-2">Keyboard shortcuts</div>
              <ul class="space-y-1.5">
                {#each [
                  { k: 'n', label: 'New deadline' },
                  { k: '/', label: 'Focus search' },
                  { k: '1 / 2 / 3', label: 'Filter Critical / High / Normal' },
                  { k: 'v', label: 'Cycle view (list / timeline / calendar)' },
                  { k: 'g', label: 'Cycle group-by (urgency / status / month)' },
                  { k: 'Esc', label: 'Clear filters / close' },
                  { k: '?', label: 'Toggle this help' }
                ] as row}
                  <li class="flex items-baseline gap-2">
                    <kbd class="font-mono text-[10px] bg-surface1 px-1.5 py-0.5 rounded text-text whitespace-nowrap">{row.k}</kbd>
                    <span class="text-subtext">{row.label}</span>
                  </li>
                {/each}
              </ul>
            </div>
          {/if}
        </div>
      </div>

      <!-- Importance filter bar — three pills with global counts.
           Click toggles. The hint row below tells the user we're
           filtered (and how to reset). -->
      <div class="flex flex-wrap items-center gap-2 mb-4">
        {#each [
          { v: 'critical' as DeadlineImportance, label: 'Critical', tone: 'error', count: importanceCounts.critical, key: '1' },
          { v: 'high' as DeadlineImportance, label: 'High', tone: 'warning', count: importanceCounts.high, key: '2' },
          { v: 'normal' as DeadlineImportance, label: 'Normal', tone: 'secondary', count: importanceCounts.normal, key: '3' }
        ] as p}
          {@const active = importanceFilter === p.v}
          <button
            type="button"
            onclick={() => setFilter(p.v)}
            title="{p.label} ({p.key})"
            class="px-3 py-1.5 rounded-full border text-xs font-medium tabular-nums transition-colors flex items-center gap-1.5
              {active ? 'border-transparent' : 'border-surface1 hover:border-surface2'}"
            style={active
              ? `background: var(--color-${p.tone}); color: #ffffff;`
              : `color: var(--color-${p.tone}); background: var(--color-surface0);`}
          >
            <span>{p.label}</span>
            <span class="text-[10px] opacity-80">{p.count}</span>
          </button>
        {/each}
        {#if importanceFilter || q}
          <span class="text-xs text-dim">
            Showing {filtered.length} of {scoped.length}
            <button
              type="button"
              onclick={() => { importanceFilter = null; q = ''; }}
              class="ml-1 px-1.5 py-0.5 rounded text-dim hover:text-text hover:bg-surface1"
            >× clear</button>
          </span>
        {/if}
      </div>

      <!-- Body — view-mode switch -->
      {#if viewMode === 'list'}
        <div class="space-y-6">
          {#each Array.from(grouped.entries()) as [b, rows] (b)}
            {#if rows.length > 0}
              {@const tone = bucketTone(b)}
              <section>
                <!-- Bucket header tinted by tone — overdue/this_week
                     pop visually under urgency, active does under
                     status, etc. -->
                <h2
                  class="text-xs uppercase tracking-wider font-medium mb-2"
                  style={tone === 'dim'
                    ? 'color: var(--color-dim);'
                    : `color: var(--color-${tone});`}
                >
                  {bucketTitle(b)}
                  <span class="opacity-60 ml-1 tabular-nums">· {rows.length}</span>
                </h2>
                <ul class="space-y-1.5">
                  {#each rows as d (d.id)}
                    {@const st = statusTone(d.status)}
                    {@const isMet = d.status === 'met'}
                    {@const isCancelled = d.status === 'cancelled'}
                    {@const isDone = isMet || isCancelled}
                    <li>
                      <div
                        role="button"
                        tabindex="0"
                        onclick={() => openEdit(d)}
                        onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openEdit(d); } }}
                        class="group w-full text-left bg-surface0 border border-surface1 rounded-lg p-3 hover:border-primary transition-colors flex flex-col gap-1.5 cursor-pointer
                          {isDone ? 'opacity-60' : ''}"
                      >
                        <!-- Title row — title takes the lead, importance
                             pill anchors the right. The icon variant is
                             dropped (the colored importance pill already
                             encodes severity) and date+countdown moves
                             into the meta row to declutter the title. -->
                        <div class="flex items-baseline gap-2">
                          <span class="text-base text-text font-medium flex-1 min-w-0 truncate {isMet ? 'line-through' : ''}">
                            {d.title}
                          </span>
                          <div class="flex items-center gap-1.5 flex-shrink-0">
                            <DeadlinePill variant="importance" importance={d.importance} />
                            {#if d.status && d.status !== 'active'}
                              <span
                                class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded whitespace-nowrap"
                                style="background: var(--color-{st}); color: #ffffff;"
                              >{d.status}</span>
                            {/if}
                          </div>
                        </div>
                        <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-dim">
                          <span class="font-mono tabular-nums text-subtext">{d.date}</span>
                          <span class="text-subtext">· {countdown(d)}</span>
                          {#if d.venture}
                            <a
                              href={ventureHref(d.venture)}
                              onclick={(e) => e.stopPropagation()}
                              class="text-secondary hover:underline"
                            >🏢 {d.venture}</a>
                          {/if}
                          {#if d.goal_id}
                            <a
                              href={goalHref(d.goal_id)}
                              onclick={(e) => e.stopPropagation()}
                              class="text-secondary hover:underline"
                            >🎯 {goalTitle(d.goal_id)}</a>
                          {/if}
                          {#if d.project}
                            <a
                              href={projectHref(d.project)}
                              onclick={(e) => e.stopPropagation()}
                              class="text-secondary hover:underline"
                            >📁 {d.project}</a>
                          {/if}
                          {#if d.task_ids && d.task_ids.length > 0}
                            <span>🔗 {d.task_ids.length} task{d.task_ids.length === 1 ? '' : 's'}</span>
                          {/if}
                          <!-- Inline quick-actions, revealed on row
                               hover/focus to stay calm on first paint
                               but discoverable. Active rows: ✓ done +
                               snooze. Done rows: ↺ reopen. -->
                          <span class="ml-auto flex items-center gap-1 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity">
                            {#if !isDone}
                              <button
                                type="button"
                                onclick={(e) => markMet(d, e)}
                                class="px-1.5 py-0.5 text-success hover:bg-surface0 rounded"
                                title="Mark met"
                                aria-label="Mark {d.title} as met"
                              >✓</button>
                              <button
                                type="button"
                                onclick={(e) => snooze(d, 1, e)}
                                class="px-1.5 py-0.5 text-info hover:bg-surface0 rounded"
                                title="Snooze 1 day"
                                aria-label="Snooze {d.title} 1 day"
                              >+1d</button>
                              <button
                                type="button"
                                onclick={(e) => snooze(d, 7, e)}
                                class="px-1.5 py-0.5 text-info hover:bg-surface0 rounded"
                                title="Snooze 1 week"
                                aria-label="Snooze {d.title} 1 week"
                              >+7d</button>
                            {:else}
                              <button
                                type="button"
                                onclick={(e) => reopen(d, e)}
                                class="px-1.5 py-0.5 text-warning hover:bg-surface0 rounded"
                                title="Reopen"
                                aria-label="Reopen {d.title}"
                              >↺</button>
                            {/if}
                          </span>
                        </div>
                        {#if d.description}
                          <p class="text-sm text-subtext line-clamp-2">{d.description}</p>
                        {/if}
                      </div>
                    </li>
                  {/each}
                </ul>
              </section>
            {/if}
          {/each}
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
        </div>
      {:else if viewMode === 'timeline'}
        <!-- Timeline view — vertical rail. Each row is a dot on the
             rail with the date + title to the right. Month dividers
             break the visual rhythm so the eye can pick out clusters. -->
        {#if timelineRows.length === 0}
          <div class="text-sm text-dim italic">No deadlines match your filters.</div>
        {:else}
          {@const monthHeaders = (() => {
            const seen = new Set<string>();
            const out = new Set<string>();
            for (const d of timelineRows) {
              const k = monthBucket(d);
              if (!seen.has(k)) { seen.add(k); out.add(d.id); }
            }
            return out;
          })()}
          <ol class="relative ml-4 border-l border-surface2 space-y-2 pl-5">
            {#each timelineRows as d (d.id)}
              {@const days = daysUntil(d.date)}
              {@const isMet = d.status === 'met'}
              {@const isCancelled = d.status === 'cancelled'}
              {@const isDone = isMet || isCancelled}
              {@const showMonth = monthHeaders.has(d.id)}
              {@const dotTone = isDone
                ? (isMet ? 'success' : 'dim')
                : days < 0 ? 'error'
                : days <= 3 ? 'error'
                : days <= 7 ? 'warning'
                : days <= 14 ? 'info'
                : 'secondary'}
              {#if showMonth}
                <li class="ml-[-1.25rem] pt-3 first:pt-0 text-[11px] uppercase tracking-wider text-dim font-medium">
                  {monthLabel(monthBucket(d))}
                </li>
              {/if}
              <li class="relative">
                <span
                  class="absolute -left-[1.55rem] top-3 w-3 h-3 rounded-full ring-2 ring-base"
                  style="background: var(--color-{dotTone});"
                  aria-hidden="true"
                ></span>
                <div
                  role="button"
                  tabindex="0"
                  onclick={() => openEdit(d)}
                  onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openEdit(d); } }}
                  class="bg-surface0 border border-surface1 hover:border-primary rounded-lg p-3 transition-colors cursor-pointer {isDone ? 'opacity-60' : ''}"
                >
                  <div class="flex items-baseline gap-2">
                    <span class="font-mono text-xs text-subtext tabular-nums w-20 flex-shrink-0">{d.date}</span>
                    <span class="text-sm font-medium text-text flex-1 min-w-0 truncate {isMet ? 'line-through' : ''}">{d.title}</span>
                    <DeadlinePill variant="importance" importance={d.importance} />
                  </div>
                  <div class="flex flex-wrap items-center gap-x-3 gap-y-0.5 mt-1 text-xs text-dim">
                    <span style="color: var(--color-{dotTone});">· {countdown(d)}</span>
                    {#if d.venture}<span class="text-secondary">🏢 {d.venture}</span>{/if}
                    {#if d.goal_id}<span class="text-secondary">🎯 {goalTitle(d.goal_id)}</span>{/if}
                    {#if d.project}<span class="text-secondary">📁 {d.project}</span>{/if}
                  </div>
                </div>
              </li>
            {/each}
          </ol>
        {/if}
      {:else if viewMode === 'calendar'}
        <!-- Calendar view — month grid. Days with deadlines render a
             stacked list of compact rows (max 3 visible, "+N more" if
             over). Click a row to open it; click an empty cell to
             create-on-that-date. -->
        <div class="flex items-center gap-2 mb-3">
          <button
            type="button"
            onclick={() => calStep(-1)}
            class="px-2 py-1 text-sm text-subtext hover:bg-surface1 rounded"
            aria-label="Previous month"
          >‹</button>
          <span class="text-sm font-medium text-text tabular-nums">{calLabel()}</span>
          <button
            type="button"
            onclick={() => calStep(1)}
            class="px-2 py-1 text-sm text-subtext hover:bg-surface1 rounded"
            aria-label="Next month"
          >›</button>
          <button
            type="button"
            onclick={() => (calCursor = new Date(new Date().getFullYear(), new Date().getMonth(), 1))}
            class="px-2 py-1 text-xs text-dim hover:text-text rounded"
          >Today</button>
        </div>
        <div class="grid grid-cols-7 gap-px bg-surface1 border border-surface1 rounded overflow-hidden">
          {#each weekdayLabels as w}
            <div class="bg-mantle text-[10px] uppercase tracking-wider text-dim font-medium py-1.5 text-center">{w}</div>
          {/each}
          {#each calCells as c (c.iso)}
            {@const today = isTodayISO(c.iso)}
            <div
              class="bg-base min-h-[5rem] p-1.5 flex flex-col gap-1 {c.inMonth ? '' : 'opacity-40'}"
              style={today ? 'box-shadow: inset 0 0 0 2px var(--color-primary);' : ''}
            >
              <div class="text-[11px] text-dim tabular-nums flex items-center gap-1">
                <span class={today ? 'text-primary font-semibold' : ''}>{c.date.getDate()}</span>
                {#if c.rows.length === 0 && c.inMonth}
                  <button
                    type="button"
                    onclick={() => { fDate = c.iso; openCreate(); fDate = c.iso; }}
                    class="ml-auto opacity-0 hover:opacity-100 focus:opacity-100 text-dim hover:text-primary"
                    aria-label="Create on {c.iso}"
                    title="Create on {c.iso}"
                  >+</button>
                {/if}
              </div>
              {#each c.rows.slice(0, 3) as d (d.id)}
                {@const tone = calRowTone(d)}
                {@const isMet = d.status === 'met'}
                <button
                  type="button"
                  onclick={() => openEdit(d)}
                  class="text-left text-[11px] truncate px-1 py-0.5 rounded hover:opacity-80 {isMet ? 'line-through opacity-60' : ''}"
                  style="background: var(--color-{tone}); color: #ffffff;"
                  title={d.title}
                >{d.title}</button>
              {/each}
              {#if c.rows.length > 3}
                <span class="text-[10px] text-dim">+ {c.rows.length - 3} more</span>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
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
        <label for="d-venture" class="block text-xs uppercase tracking-wider text-dim mb-1">Linked venture</label>
        <input
          id="d-venture"
          bind:value={fVenture}
          list="d-ventures-list"
          placeholder="venture name (optional)"
          class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
        />
        {#if ventures.length > 0}
          <datalist id="d-ventures-list">
            {#each ventures as v}<option value={v.name}></option>{/each}
          </datalist>
        {/if}
        <p class="text-[11px] text-dim mt-1">
          Free-text — links the deadline to a Venture record so it shows up on /ventures and the venture's project rollup.
        </p>
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
          class="px-3 py-1.5 text-xs text-error hover:bg-surface0 rounded disabled:opacity-50"
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
