<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, todayISO, fmtDateISO, type Task, type Project, type Goal, type Deadline } from '$lib/api';
  import { parseTaskInput, smartDate } from '$lib/util/taskParse';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { onWsEvent } from '$lib/ws';
  import { createCoalescedReload } from '$lib/util/coalesce';
  import TaskCard from '$lib/tasks/TaskCard.svelte';
  import Kanban from '$lib/tasks/Kanban.svelte';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import TriageBoard from '$lib/tasks/TriageBoard.svelte';
  import BulkBar from '$lib/tasks/BulkBar.svelte';
  import TaskDetail from '$lib/tasks/TaskDetail.svelte';
  import TaskContextMenu from '$lib/tasks/TaskContextMenu.svelte';
  import Drawer from '$lib/components/Drawer.svelte';
  import EisenhowerView from '$lib/tasks/EisenhowerView.svelte';
  import TaskAgent from '$lib/tasks/TaskAgent.svelte';
  import AIStaleVerdicts from '$lib/tasks/AIStaleVerdicts.svelte';
  import AskTasks from '$lib/tasks/AskTasks.svelte';
  import TaskDuplicates from '$lib/tasks/TaskDuplicates.svelte';
  // Stream N — slim page chrome split into three small sub-components
  // so this file doesn't re-grow into a god-template after the recent
  // AI-store extraction. Header + chips + sections all live as their
  // own files; this page wires them together and owns the state.
  import TasksPageHeader from '$lib/tasks/TasksPageHeader.svelte';
  import QuickFilterChips from '$lib/tasks/QuickFilterChips.svelte';
  import SectionList from '$lib/tasks/SectionList.svelte';
  import { isTypingTarget } from '$lib/util/isTypingTarget';
  import { loadStored, loadStoredString, saveStored, saveStoredString } from '$lib/util/storage';
  import { focusOnMount } from '$lib/util/focusOnMount';
  import { applyNextPriority, toggleDoneOf } from '$lib/tasks/taskActions';
  import {
    createTriageStore,
    createDeadlineStore,
    createFocusPlanStore
  } from '$lib/tasks/aiAgentStore';

  type View = 'list' | 'kanban' | 'today' | 'week' | 'triage' | 'inbox' | 'stale' | 'duplicates' | 'quickwins' | 'review' | 'eisenhower';
  type Group = 'due' | 'priority' | 'note' | 'project' | 'tag' | 'goal' | 'deadline';
  // Explicit sort overrides the per-group "auto" sort (which sorts
  // every bucket by due-then-priority). Set to anything other than
  // 'auto' and the same sort applies inside every group so the user
  // gets consistent ordering regardless of which group-by is active.
  // 'age' is by createdAt ascending (oldest first — the
  // procrastination tell); the others are obvious from the label.
  type SortBy = 'auto' | 'priority' | 'due' | 'age' | 'alpha' | 'estimate';

  let tasks = $state<Task[]>([]);
  let projects = $state<Project[]>([]);
  // Task Agent — conversational action proposer. Sees the
  // currently-filtered task list, takes a free-text intent, returns
  // a list of typed actions the user accepts per-card. Distinct
  // from Plan-day (schedules) and Stale-review (verdicts) — this
  // is the "do something for me" surface.
  let agentOpen = $state(false);
  // Goals + deadlines drive the new group-by options and the group
  // header titles (so a "Q3 launch (G004)" group reads as the goal's
  // title, not the bare ID). Loaded once, then refreshed alongside
  // the task list on WS events.
  let goals = $state<Goal[]>([]);
  let deadlines = $state<Deadline[]>([]);

  // Persist view + groupBy to localStorage so the user comes back to where they left off.
  const VIEW_KEY = 'granit.tasks.view';
  const GROUP_KEY = 'granit.tasks.groupBy';
  const SORT_KEY = 'granit.tasks.sortBy';

  let view = $state<View>(loadStoredString(VIEW_KEY, 'list') as View);
  let groupBy = $state<Group>(loadStoredString(GROUP_KEY, 'due') as Group);
  let sortBy = $state<SortBy>(loadStoredString(SORT_KEY, 'auto') as SortBy);
  $effect(() => saveStoredString(SORT_KEY, sortBy));
  let kanbanMode = $state<'priority' | 'due' | 'triage' | 'config'>('priority');
  let kanbanSwimlane = $state<'none' | 'project' | 'tag' | 'priority'>('none');
  let helpOpen = $state(false);
  // Overflow dropdown for the secondary view-mode cluster. Stream H
  // collapsed 11 tabs into 5 primary (Today/List/Kanban/Matrix/Week)
  // plus a "More views" dropdown for Triage/Inbox/Stale/Duplicates/
  // Quick wins/Review so the strip stops scrolling sideways at narrow
  // viewports. The actual View identifiers are unchanged — the `[`/
  // `]` cycle and URL hydration still see all 11.
  let moreViewsOpen = $state(false);
  // Labels for the overflow set — shared between the dropdown items
  // AND the "More: <label>" button text so the user can see which
  // overflow view is currently active without opening the menu.
  const OVERFLOW_VIEWS: { key: View; label: string; title: string }[] = [
    { key: 'triage', label: 'Triage', title: 'AI-driven inbox triage proposals' },
    { key: 'inbox', label: 'Inbox', title: 'untriaged tasks awaiting categorisation' },
    { key: 'stale', label: 'Stale', title: 'not touched in 7+ days — needs a decision' },
    { key: 'duplicates', label: 'Duplicates', title: 'near-duplicate task pairs by text similarity — deterministic scan, no AI' },
    { key: 'quickwins', label: 'Quick wins', title: 'high priority + ≤30 min — tackle a few before lunch' },
    { key: 'review', label: 'Review', title: 'completed in the last 7 days — celebrate the wins' }
  ];
  let activeOverflowLabel = $derived(
    OVERFLOW_VIEWS.find((v) => v.key === view)?.label ?? ''
  );
  function pickOverflowView(v: View) {
    view = v;
    moreViewsOpen = false;
  }
  function onMoreViewsKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      moreViewsOpen = false;
      e.stopPropagation();
    }
  }
  // Click-outside dismiss for the overflow menu. We install a
  // window-level listener only while the menu is open so the rest
  // of the page doesn't pay for it. The menu+button live inside an
  // element marked with data-more-views; any click outside that
  // subtree closes the menu. Keep the install/teardown inside an
  // $effect so Svelte handles cleanup across HMR + unmount.
  $effect(() => {
    if (!moreViewsOpen) return;
    function onDocClick(e: MouseEvent) {
      const target = e.target as HTMLElement | null;
      if (target && target.closest('[data-more-views]')) return;
      moreViewsOpen = false;
    }
    window.addEventListener('mousedown', onDocClick);
    return () => window.removeEventListener('mousedown', onDocClick);
  });
  let status = $state<'open' | 'done' | 'all'>('open');
  let q = $state('');
  // tagFilters — multi-tag filter with AND semantics. Clicking a tag
  // chip toggles its membership; the visible list shrinks to tasks
  // that carry EVERY active tag. URL serialization is comma-separated
  // (?tag=foo,bar) so shared links round-trip; the backend listTasks
  // call passes only the first tag (the endpoint supports a single
  // tag param) and the rest are AND-narrowed client-side.
  let tagFilters = $state<string[]>([]);
  let projectFilter = $state('');
  let priorityFilter = $state<number | ''>('');
  let goalFilter = $state('');
  let deadlineFilter = $state('');
  // Archived view modes:
  //   'hide'  — default. archived tasks are hidden from every list (server-side filter).
  //   'show'  — show archived tasks alongside active so the user can see the full picture.
  //   'only'  — show ONLY archived (the "archive drawer" view).
  // Persisted to localStorage like the other filters.
  let archivedMode = $state<'hide' | 'show' | 'only'>('hide');

  // Smart filter chip — a single-select quick predicate on top of the
  // existing filter dimensions. Replaces the passive stats chips with
  // one-click filters so the user can jump from "I see I have 4
  // overdue" to "showing 4 overdue" in one click without opening any
  // dialog or learning a new control. Persisted to URL hash so
  // refreshes / shared links carry the same focus.
  type SmartFilter =
    | ''
    | 'overdue'
    | 'today'
    | 'tomorrow'
    | 'thisWeek'
    | 'noDue'
    | 'noPriority'
    | 'highPriority'
    | 'hasSubtasks'
    | 'hasEstimate'
    | 'noEstimate';
  let smartFilter = $state<SmartFilter>('');
  // Source filter — separates "tasks the user actually wrote as tasks"
  // from "stray `- [ ]` bullets in reading notes / brainstorm pages".
  // Default is 'all' (every `- [ ]` in the vault shows up, matching the
  // README's promise and Amplenote-style task capture from arbitrary
  // notes). Flipping to 'task-notes' narrows to notes that look like
  // dedicated task surfaces — daily notes, anything under Tasks/,
  // Projects/, or Daily/.
  //
  // Storage key bumped from .source to .source.v2 so existing users
  // who had been silently defaulted to the old 'task-notes' get the
  // new behaviour once (and can re-pick strict mode from the sidebar
  // if that's actually what they want).
  const SOURCE_KEY = 'granit.tasks.source.v2';
  let sourceFilter = $state<'task-notes' | 'all'>(
    loadStoredString(SOURCE_KEY, 'all') === 'task-notes' ? 'task-notes' : 'all'
  );
  $effect(() => saveStoredString(SOURCE_KEY, sourceFilter));

  // Compact density — flips every TaskCard into its `compact` mode so
  // more rows fit above the fold. Power users with hundreds of open
  // tasks lean on this; casual users keep the comfortable default.
  // Persisted to localStorage like every other view preference.
  const DENSITY_KEY = 'granit.tasks.density';
  let density = $state<'normal' | 'compact'>(
    loadStoredString(DENSITY_KEY, 'normal') === 'compact' ? 'compact' : 'normal'
  );
  $effect(() => saveStoredString(DENSITY_KEY, density));
  let compactCards = $derived(density === 'compact');

  // Inline per-group quick-add. Only one group's input is open at a
  // time. Submitting creates a task with the group's defaults applied
  // (due date, priority, project, etc.) so the new row lands in the
  // SAME bucket the user added it from — no scattering across groups.
  // Distinct from the existing toolbar-level quickAdd: that one parses
  // natural language and dumps everything into today's daily; this one
  // is group-scoped and infers defaults from the bucket.
  let groupAddKey = $state<string | null>(null);
  let groupAddText = $state('');
  let groupAddBusy = $state(false);

  // Translate a (groupBy, group-key) pair into the createTask defaults
  // for that bucket. Keeps every group-add landing in the SAME bucket
  // the user added it from — no scattering across groups.
  function groupAddDefaults(group: string): {
    dueDate?: string;
    priority?: number;
    projectId?: string;
    tags?: string[];
    goalId?: string;
    deadlineId?: string;
    notePathHint?: string;
  } {
    const today = todayISO();
    if (groupBy === 'due') {
      switch (group) {
        case 'overdue':
        case 'today':
          return { dueDate: today };
        case 'tomorrow': {
          const d = new Date(today + 'T00:00:00');
          d.setDate(d.getDate() + 1);
          return { dueDate: fmtDateISO(d) };
        }
        case 'this_week': {
          const d = new Date(today + 'T00:00:00');
          d.setDate(d.getDate() + 3);
          return { dueDate: fmtDateISO(d) };
        }
        case 'later': {
          const d = new Date(today + 'T00:00:00');
          d.setDate(d.getDate() + 14);
          return { dueDate: fmtDateISO(d) };
        }
        case 'no_date':
        default:
          return {};
      }
    }
    if (groupBy === 'priority') {
      const p = Number(group);
      return p >= 1 && p <= 3 ? { priority: p } : {};
    }
    if (groupBy === 'tag') {
      return group === '(untagged)' ? {} : { tags: [group] };
    }
    if (groupBy === 'project') {
      const proj = projects.find((p) => p.name === group);
      if (!proj) return {};
      return { projectId: proj.name };
    }
    if (groupBy === 'goal') return group === '(no goal)' ? {} : { goalId: group };
    if (groupBy === 'deadline') return group === '(no deadline)' ? {} : { deadlineId: group };
    if (groupBy === 'note') return { notePathHint: group };
    return {};
  }

  async function submitGroupAdd(group: string) {
    const text = groupAddText.trim();
    if (!text || groupAddBusy) return;
    groupAddBusy = true;
    try {
      const defaults = groupAddDefaults(group);
      // notePath fallback chain:
      //   1. note-grouped key IS the notePath
      //   2. otherwise today's daily — the safe capture target
      let notePath = defaults.notePathHint ?? '';
      if (!notePath) {
        try {
          const daily = await api.daily('today');
          notePath = daily.path;
        } catch {
          notePath = `${todayISO()}.md`;
        }
      }
      const body: Parameters<typeof api.createTask>[0] = { notePath, text };
      if (defaults.dueDate) body.dueDate = defaults.dueDate;
      if (defaults.priority !== undefined) body.priority = defaults.priority;
      if (defaults.tags && defaults.tags.length > 0) body.tags = defaults.tags;
      if (defaults.projectId) body.projectId = defaults.projectId;
      if (defaults.goalId) body.goalId = defaults.goalId;
      if (defaults.deadlineId) body.deadlineId = defaults.deadlineId;
      await api.createTask(body);
      groupAddText = '';
      await load();
      toast.success('task added');
      // Leave the input open so the user can keep capturing without
      // re-opening the row. Esc / blur dismisses it.
    } catch (e) {
      toast.error('add failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      groupAddBusy = false;
    }
  }
  function cancelGroupAdd() {
    groupAddKey = null;
    groupAddText = '';
  }

  // ── Ask Tasks ────────────────────────────────────────────────────
  // Free-form Q&A against the currently-loaded task set lives in
  // $lib/tasks/AskTasks.svelte — that component owns the question /
  // answer state, the streaming, the dismiss path. The parent only
  // owns the open flag (so its trigger button can flip it) and the
  // "no tasks in current view" guard (filtered is the parent's
  // derivation).
  let askTasksOpen = $state(false);
  function startAskTasks() {
    if (filtered.length === 0) {
      toast.info('No tasks in the current view.');
      return;
    }
    askTasksOpen = true;
  }
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
    if (sp.has('tag')) {
      // Comma-separated list. Empty entries (leading/trailing comma,
      // accidental double comma) get filtered out so a stale URL
      // doesn't ghost in an empty-string "tag".
      tagFilters = get('tag').split(',').map((s) => s.trim()).filter(Boolean);
    }
    if (sp.has('project')) projectFilter = get('project');
    if (sp.has('priority')) {
      const n = Number(get('priority'));
      priorityFilter = n >= 1 && n <= 3 ? n : '';
    }
    if (sp.has('goal')) goalFilter = get('goal');
    if (sp.has('deadline')) deadlineFilter = get('deadline');
    if (sp.has('view')) {
      const v = get('view') as View;
      if (['list', 'kanban', 'today', 'week', 'triage', 'inbox', 'stale', 'duplicates', 'quickwins', 'review', 'eisenhower'].includes(v)) view = v;
    }
    if (sp.has('group')) {
      const g = get('group') as Group;
      if (['due', 'priority', 'note', 'project', 'tag', 'goal', 'deadline'].includes(g)) groupBy = g;
    }
    if (sp.has('smart')) {
      const v = get('smart') as SmartFilter;
      if (['overdue', 'today', 'tomorrow', 'thisWeek', 'noDue', 'noPriority', 'highPriority', 'hasSubtasks', 'hasEstimate', 'noEstimate'].includes(v)) {
        smartFilter = v;
      }
    }
    // ?agent=1 launches the Task Agent directly — the sidebar's
    // "Run Task Agent" entry uses this to open the agent from
    // outside the page without a global ref. Consumed once: we
    // clear the param on hydrate so a hash-refresh doesn't keep
    // re-popping the dialog.
    if (sp.get('agent') === '1') {
      agentOpen = true;
      const next = new URLSearchParams(sp);
      next.delete('agent');
      const qs = next.toString();
      void goto(qs ? `${$page.url.pathname}?${qs}` : $page.url.pathname, {
        replaceState: true,
        noScroll: true,
        keepFocus: true
      });
    }
    urlHydrated = true;
  }
  function syncToUrl() {
    if (!urlHydrated) return;
    if (typeof window === 'undefined') return;
    const sp = new URLSearchParams();
    if (status !== 'open') sp.set('status', status);
    if (q) sp.set('q', q);
    if (tagFilters.length > 0) sp.set('tag', tagFilters.join(','));
    if (projectFilter) sp.set('project', projectFilter);
    if (priorityFilter !== '') sp.set('priority', String(priorityFilter));
    if (goalFilter) sp.set('goal', goalFilter);
    if (deadlineFilter) sp.set('deadline', deadlineFilter);
    if (view !== 'list') sp.set('view', view);
    if (groupBy !== 'due') sp.set('group', groupBy);
    if (smartFilter) sp.set('smart', smartFilter);
    const qs = sp.toString();
    const next = qs ? `${$page.url.pathname}?${qs}` : $page.url.pathname;
    // replaceState (not goto) — we don't want every keystroke in the
    // search box adding to browser history.
    void goto(next, { replaceState: true, noScroll: true, keepFocus: true });
  }
  // Stream N — slide-out filter panel. Replaces the always-on desktop
  // sidebar so the default page is cleaner; one click opens advanced
  // filtering. Persists nothing — open-state is session-only so the
  // panel doesn't pop open on every reload.
  let filterPanelOpen = $state(false);

  // Stream N — per-section collapse state for the smart-section list.
  // Keyed by section key ('overdue' / 'today' / 'tomorrow' / 'this_week'
  // / 'later' / 'no_date' / 'done'). Value 'true' = collapsed, 'false'
  // = explicitly expanded; missing keys fall through to the per-section
  // default (later/no_date/done collapsed; everything else open).
  const SECTION_COLLAPSE_KEY = 'granit.tasks.collapsedSections';
  let collapsedSections = $state<Record<string, boolean>>(
    loadStored<Record<string, boolean>>(SECTION_COLLAPSE_KEY, {})
  );
  $effect(() => saveStored(SECTION_COLLAPSE_KEY, collapsedSections));
  function toggleSection(key: string) {
    // Read the same per-section default the SectionList uses so the
    // toggle flips against the effective state (a 'later' section that
    // looks collapsed because of the default but has no explicit entry
    // should record 'false' on first toggle, not 'true').
    const defaultCollapsed =
      key === 'later' || key === 'no_date' || key === 'done';
    const current = collapsedSections[key];
    const effective = current === undefined ? defaultCollapsed : current;
    collapsedSections = { ...collapsedSections, [key]: !effective };
  }

  // AI orchestration stores — busy / proposals / abort state for
  // inbox-triage, deadline-detect, and plan-my-day all live in
  // $lib/tasks/aiAgentStore. The page subscribes via $triage /
  // $deadline / $focusPlan auto-stores and calls the methods on
  // each. The page-local hydrate-from-cache happens in onMount.
  //
  // The AI Stale-task verdict surface (✨ AI verdicts button + the
  // accept/defer/archive panel) is its own component
  // ($lib/tasks/AIStaleVerdicts.svelte) and owns its own state;
  // the page just passes candidates + onReload.
  const triage = createTriageStore();
  const deadline = createDeadlineStore();
  const focusPlan = createFocusPlanStore();

  // Focus-hours input lives on the page (it's a single
  // localStorage-persisted number bound to the toolbar's <input>),
  // and gets passed into focusPlan.run() at call-time so the
  // store snapshots it at start, not per-render. Defaults to 4
  // (a realistic deep-work day for most knowledge workers).
  const FOCUS_HOURS_KEY = 'granit.tasks.focusHours';
  let aiFocusHours = $state<number>(
    Number(loadStoredString(FOCUS_HOURS_KEY, '4')) || 4
  );
  $effect(() => saveStoredString(FOCUS_HOURS_KEY, String(aiFocusHours)));

  // Quick-add bar at the top of the page. The user types a single
  // line in the syntax granit's parser understands ("buy milk !2
  // due:2026-05-15 #errand" — also "due:tomorrow" / "due:fri" via
  // smartDate) and pressing Enter creates the task in today's
  // daily note. Keeps the user's hands on the keyboard and turns
  // task creation into a 2-second flow without opening any modal.
  let quickAdd = $state('');
  let quickAddBusy = $state(false);
  async function submitQuickAdd() {
    const raw = quickAdd.trim();
    if (!raw || quickAddBusy) return;
    quickAddBusy = true;
    try {
      // Pre-parse to extract priority / due / tags. parseTaskInput
      // accepts ISO dates; smartDate translates "tomorrow" / "fri"
      // / "next mon" so the user can type natural language.
      const parsed = parseTaskInput(raw);
      // If the user typed `due:<word>` (non-ISO), retry via smartDate.
      // The pre-parse leaves the original raw visible, so we slice
      // it ourselves by re-matching.
      let dueDate = parsed.dueDate;
      if (!dueDate) {
        const m = raw.match(/(?:^|\s)due:([\w-]+)(?=\s|$)/);
        if (m) {
          const sd = smartDate(m[1]);
          if (sd) {
            dueDate = sd;
            // Strip the original `due:<word>` from the text we
            // pre-parsed (parseTaskInput only stripped ISO matches).
            parsed.text = parsed.text.replace(/(?:^|\s)due:[\w-]+(?=\s|$)/, ' ').trim().replace(/\s+/g, ' ');
          }
        }
      }
      if (!parsed.text) {
        toast.error('Empty task — type a description first.');
        return;
      }
      // Create in today's daily note. If today's daily doesn't exist
      // yet, granit's GET /api/v1/daily/today auto-creates it from
      // the template.
      const daily = await api.daily('today');
      await api.createTask({
        notePath: daily.path,
        text: parsed.text,
        priority: parsed.priority || undefined,
        dueDate: dueDate || undefined,
        tags: parsed.tags.length > 0 ? parsed.tags : undefined
      });
      toast.success(`Added: ${parsed.text}`);
      quickAdd = '';
      await load();
    } catch (e) {
      toast.error('Failed to add task: ' + (errorMessage(e)));
    } finally {
      quickAddBusy = false;
    }
  }
  let selectedIds = $state<Set<string>>(new Set());
  let detailTask = $state<Task | null>(null);
  let detailOpen = $state(false);

  // Swipe-hint dismissal. The first time a touch-device user lands
  // on the list view, show a small "‹ swipe ›" banner above the first
  // card so they know snooze (left) / done (right) gestures exist.
  // Dismiss on tap or after 8 seconds — pick the simpler path rather
  // than wiring "swipe detected" back from TaskCard. Once dismissed,
  // localStorage holds the flag so the hint never reappears.
  const SWIPE_HINT_KEY = 'granit.tasks.swipe-hint-dismissed';
  let swipeHintDismissed = $state(
    typeof window !== 'undefined' && window.localStorage.getItem(SWIPE_HINT_KEY) === '1'
  );
  // Only show on touch devices. The matchMedia probe is best-effort —
  // if the API isn't available (very old browser) we err on the side
  // of NOT showing the hint, which is the conservative path.
  let isTouchDevice = $state(false);
  onMount(() => {
    try {
      isTouchDevice = window.matchMedia('(hover: none) and (pointer: coarse)').matches;
    } catch {
      isTouchDevice = false;
    }
    // Auto-dismiss after 8 seconds — the hint is meant as a one-time
    // nudge, not a permanent fixture. Saves a write to localStorage so
    // the dismissal sticks across the next refresh too.
    if (!swipeHintDismissed && isTouchDevice) {
      const handle = setTimeout(() => dismissSwipeHint(), 8000);
      return () => clearTimeout(handle);
    }
  });
  function dismissSwipeHint() {
    swipeHintDismissed = true;
    try { window.localStorage.setItem(SWIPE_HINT_KEY, '1'); } catch {}
  }
  // The `showSwipeHint` derived value reads `filtered`, which is
  // declared later in the file (the page's main filter pipeline).
  // Hoisting THAT declaration up is invasive; we instead lazy-evaluate
  // the gate by declaring `showSwipeHint` after `filtered` near the
  // bottom of the script. See the matching $derived below the
  // `filtered` derivation.
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

  // Deep-link `?focus=<task-id>` opens the detail drawer for that
  // task on load. The dashboard's TodayStream widget links here so
  // a click on a scheduled / due task lands directly on its detail
  // instead of the user having to scroll-and-find. Only fires once
  // per change in the URL+task-list pairing — without that guard a
  // re-rendered tasks list would re-open the drawer every load.
  let lastFocusedFromUrl = $state<string | null>(null);
  $effect(() => {
    const focusId = $page.url.searchParams.get('focus');
    if (!focusId || tasks.length === 0) return;
    if (focusId === lastFocusedFromUrl) return;
    const t = tasks.find((x) => x.id === focusId);
    if (t) {
      openDetail(t);
      lastFocusedFromUrl = focusId;
    }
  });

  $effect(() => saveStoredString(VIEW_KEY, view));
  $effect(() => saveStoredString(GROUP_KEY, groupBy));

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
      // The backend endpoint accepts a single tag; for multi-tag
      // filters we pass the first to narrow the server response and
      // AND-narrow the rest client-side in the `filtered` derivation.
      if (tagFilters.length > 0) params.tag = tagFilters[0];
      if (priorityFilter !== '') params.priority = priorityFilter;
      if (projectFilter) params.project = projectFilter;
      if (goalFilter) params.goal = goalFilter;
      if (deadlineFilter) params.deadline = deadlineFilter;
      if (archivedMode === 'show') params.includeArchived = true;
      if (archivedMode === 'only') params.archived = true;
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
    } catch {
      // 401 (stale auth) and network failures both end up here.
      // Silently leave tasks/projects empty so the empty-state copy
      // renders instead of the indefinite loading spinner. A later
      // WS reconnect or filter change will retry naturally — no toast,
      // no console noise; the comment above is the only documentation
      // we need for the silent branch.
    } finally {
      loading = false;
    }
  }

  // Single load driver: an effect that keys off $auth + filters. When
  // auth resolves (or changes) it fires; when status/tagFilters change
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
    void tagFilters;
    void priorityFilter;
    void projectFilter;
    void goalFilter;
    void deadlineFilter;
    void archivedMode;
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
    void tagFilters;
    void projectFilter;
    void priorityFilter;
    void goalFilter;
    void deadlineFilter;
    void view;
    void groupBy;
    void smartFilter;
    untrack(() => syncToUrl());
  });

  onMount(() => {
    hydrateFromUrl();
    // Rehydrate any unprocessed AI proposals so a refresh / nav-away
    // doesn't burn the call. TTL-stale entries are dropped silently
    // inside each store's hydrate().
    triage.hydrate();
    deadline.hydrate();
  });

  // Coalesced reload — bulk operations (multi-select triage, plan
  // apply, drag-drop kanban moves) can fire dozens of task.changed
  // events in a row. Each one used to refetch the entire list,
  // which froze the page during a 50-item triage. One trailing-edge
  // reload per window suffices; the visibility-change handler still
  // bypasses the coalesce so a returning tab feels instantly fresh.
  const reload = createCoalescedReload(() => load(), 600);

  onMount(() => {
    const unsub = onWsEvent((ev) => {
      // task.changed fires after every patchTask, including drag-drops
      // from the kanban — without it, moves would only show up on a
      // manual refresh (or the next note write coincidentally). Match
      // the same set the calendar/inbox widgets honor.
      if (ev.type === 'note.changed' || ev.type === 'note.removed' || ev.type === 'task.changed') {
        reload.trigger();
      }
    });
    // Visibility-aware refresh: a backgrounded tab won't get WS events,
    // so a task ticked off on the phone while the desktop tab was
    // hidden would otherwise stay open here until reload. Catches the
    // cross-device case at zero recurring cost. Bypass the coalesce
    // so the user sees fresh data immediately on tab return.
    const onVisible = () => {
      if (document.visibilityState === 'visible') reload.flush();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', onVisible);
    return () => {
      unsub();
      reload.cancel();
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', onVisible);
    };
  });

  // ---------------------------------------------------------------------------
  // Keyboard shortcuts (j/k navigate, x select, e edit, d done, p priority).
  // Mirrors the TUI's task manager bindings as far as the web allows. Skipped
  // when the user is typing into an input so we don't eat letters mid-search.
  // The cursor is page-local; we only navigate within the current `filtered`
  // list. Discoverable via the '?' button in the header.
  // ---------------------------------------------------------------------------
  let cursorIdx = $state<number>(-1);
  $effect(() => {
    // Reset cursor when the filtered list shrinks past it. We read the
    // whole `filtered` array (not just .length) so any change to the
    // filter pipeline retriggers — a swap that keeps length identical
    // but rearranges items could otherwise leave the cursor pointing
    // at a stale row. The Math.max(0, …) keeps cursorIdx valid (>= 0)
    // even when filtered is empty; cursor-read sites also `?.` against
    // out-of-bounds so a flicker between the effect firing and the
    // render path resolves gracefully.
    void filtered;
    if (cursorIdx >= filtered.length) {
      cursorIdx = Math.max(0, filtered.length - 1);
    }
  });

  // isTypingTarget lives in $lib/util/isTypingTarget — shared with
  // /projects and /goals page-level hotkey handlers.

  async function cyclePriorityOf(t: Task) {
    try {
      await applyNextPriority(t);
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

  // View-mode cycle order. Mirrors the visible-tab order in the
  // segmented pills (primary cluster then smart-filter cluster) so
  // `[` / `]` walks the user through the same tabs their eye sees,
  // left-to-right. Includes every shape so the cycle is exhaustive;
  // narrow-viewport users still hit hidden tabs (stale / quickwins /
  // duplicates / review / triage) via the chord even when the buttons
  // are visually hidden — which is the point of having a chord.
  const VIEW_CYCLE: View[] = [
    'today', 'list', 'week', 'kanban', 'eisenhower',
    'inbox', 'quickwins', 'stale', 'duplicates', 'review', 'triage'
  ];
  // Numeric direct-jump map — Stream F shortcuts `1`-`5`.
  // Maps to the primary tab cluster (Today / List / Kanban / Matrix /
  // Week) in the same left-to-right order the segmented pill renders
  // after Stream H consolidated 11 view modes into 5 primary + 6
  // overflow. Overflow views (Triage / Inbox / Stale / Duplicates /
  // Quick wins / Review) are still reachable via the "More views"
  // dropdown and the `[` / `]` cycle — no digit binding so power
  // users learn to thumb the primary cluster by number.
  const VIEW_DIGIT_MAP: Record<string, View> = {
    '1': 'today',
    '2': 'list',
    '3': 'kanban',
    '4': 'eisenhower',
    '5': 'week'
  };

  // Trigger the in-card snooze picker for the cursor task. The picker
  // is owned by TaskCard and anchored to its own snooze button (so the
  // popover positions correctly), so the cleanest cross-component
  // invocation is to .click() the button via the data-task-id wrapper.
  // Falls back silently if the row hasn't rendered yet — the keydown
  // handler's early-return already guarded against an empty filter.
  function openSnoozePickerForCursor() {
    const t = cursorIdx >= 0 ? filtered[cursorIdx] : null;
    if (!t) return;
    const row = document.querySelector(`[data-task-id="${t.id}"]`);
    if (!row) return;
    const btn = row.querySelector('button[aria-label="snooze"]') as HTMLButtonElement | null;
    if (btn) btn.click();
  }

  // Bulk select-all toggle. Distinct from BulkBar's checkbox: this
  // operates on every item in the currently-filtered list (including
  // groups collapsed by the user — selection IS the union). Re-firing
  // with everything already selected clears, so the chord doubles as
  // an escape hatch without needing a separate Esc press.
  function selectAllOrClear() {
    if (filtered.length === 0) return;
    const allSelected = filtered.every((t) => selectedIds.has(t.id));
    if (allSelected) {
      selectedIds = new Set();
      toast.info('Selection cleared');
      return;
    }
    selectedIds = new Set(filtered.map((t) => t.id));
    toast.success(`Selected ${filtered.length} task${filtered.length === 1 ? '' : 's'}`);
  }

  function cycleView(direction: 1 | -1) {
    const i = VIEW_CYCLE.indexOf(view);
    const base = i >= 0 ? i : 0;
    const next = (base + direction + VIEW_CYCLE.length) % VIEW_CYCLE.length;
    view = VIEW_CYCLE[next];
  }

  onMount(() => {
    function onKey(e: KeyboardEvent) {
      if (isTypingTarget(e.target)) return;
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const k = e.key;
      // Help overlay — works on every view (including kanban/triage/
      // eisenhower, which otherwise short-circuit below).
      if (k === '?') {
        helpOpen = !helpOpen;
        e.preventDefault();
        return;
      }
      if (helpOpen && k === 'Escape') {
        helpOpen = false;
        return;
      }
      // Stream N — `/` opens the slide-out filter panel so the global
      // page-search handler in +layout.svelte finds the embedded
      // search input visible. The panel's content renders in DOM at
      // all times (Drawer translates off-screen), so the global
      // focus() call still works; without opening the panel the user
      // would type into an invisible field. We DON'T preventDefault —
      // the global handler still runs and focuses the input.
      if (k === '/' && !filterPanelOpen) {
        filterPanelOpen = true;
        // Fall through; the layout's onKey will focus the input next.
      }
      // Esc closes the filter panel before falling through to the
      // selection-clear branch lower down.
      if (k === 'Escape' && filterPanelOpen) {
        filterPanelOpen = false;
        e.preventDefault();
        return;
      }
      // View cycling + direct-jump work on EVERY view (so the user can
      // bounce out of kanban → list with `]`, then back with `[`).
      // Must run before the kanban/triage/eisenhower early-return.
      if (k === '[') {
        cycleView(-1);
        e.preventDefault();
        return;
      }
      if (k === ']') {
        cycleView(1);
        e.preventDefault();
        return;
      }
      if (k in VIEW_DIGIT_MAP) {
        view = VIEW_DIGIT_MAP[k];
        e.preventDefault();
        return;
      }
      // Kanban / TriageBoard / EisenhowerView each install their own
      // window-level handler with a column-aware cursor. Suppressing
      // the page-level handler in those views avoids double-firing
      // j/k/x/d/e/p (which would move two cursors and patch twice).
      if (view === 'kanban' || view === 'triage' || view === 'eisenhower') return;
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
      // 'a' opens the Task Agent. Distinct from per-task shortcuts
      // below — no cursor task required, the agent operates on the
      // filtered list (or the bulk-selection if one is active).
      if (k === 'a') {
        agentOpen = true;
        e.preventDefault();
        return;
      }
      // Shift+A — bulk select-all / clear toggle. Different from `a`
      // (which opens the agent) because of the explicit modifier;
      // event.key on Shift+A reports "A" uppercase, which is what we
      // match here. e.shiftKey check disambiguates from the unlikely
      // case of a hardware-locked caps-lock typist hitting plain "A".
      if (k === 'A' && e.shiftKey) {
        selectAllOrClear();
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
        toggleDoneOf(t).catch(() => {});
        e.preventDefault();
      } else if (k === 'e') {
        openDetail(t);
        e.preventDefault();
      } else if (k === 'p') {
        cyclePriorityOf(t);
        e.preventDefault();
      } else if (k === 's') {
        // Open the snooze popover on the cursor task. The popover
        // owns the date picker; this just triggers the in-card
        // button so positioning + outside-click dismiss behave the
        // same as a mouse click.
        openSnoozePickerForCursor();
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

  // isTaskLikePath: heuristic for "this notePath came from a note the
  // user clearly meant as a task surface, not a reading list that
  // happens to use - [ ] for visual bullets". Pure path-based so we
  // don't need to fetch frontmatter for every task.
  //
  // Match rules (any one is enough to count as task-like):
  //   - filename is YYYY-MM-DD.md anywhere → daily note
  //   - path begins with Daily/, Tasks/, or Projects/ at any depth
  //   - notePath empty → tasks created via the API w/o a host note
  //     (we keep them visible because they were explicit)
  //
  // The folder list below intentionally does NOT include arbitrary
  // user folders; the user can still see those by flipping the source
  // filter to 'all' from the UI. Folder names are case-insensitive on
  // the prefix to be friendly to mac/windows-originated vaults.
  const taskFolderPrefixes = ['daily/', 'tasks/', 'projects/'];
  const reDailyName = /(?:^|\/)\d{4}-\d{2}-\d{2}\.md$/;
  function isTaskLikePath(p: string): boolean {
    if (!p) return true;
    if (reDailyName.test(p)) return true;
    const lower = p.toLowerCase();
    for (const prefix of taskFolderPrefixes) {
      if (lower.startsWith(prefix)) return true;
    }
    return false;
  }

  let filtered = $derived.by(() => {
    let out = tasks;
    if (sourceFilter === 'task-notes') {
      out = out.filter((t) => isTaskLikePath(t.notePath));
    }
    if (q.trim()) {
      const ql = q.toLowerCase();
      out = out.filter((t) => t.text.toLowerCase().includes(ql) || t.notePath.toLowerCase().includes(ql));
    }
    if (priorityFilter !== '') out = out.filter((t) => t.priority === priorityFilter);
    // Multi-tag AND filter — the backend already narrowed by the first
    // tag (if any), so we only need to re-check the rest here. Doing
    // it client-side keeps the filter UI snappy: clicking a second
    // tag chip doesn't force a refetch + re-render of the whole list.
    if (tagFilters.length > 1) {
      out = out.filter((t) => {
        const tags = t.tags ?? [];
        // Skip index 0 since the server already filtered by it.
        for (let i = 1; i < tagFilters.length; i++) {
          if (!tags.includes(tagFilters[i])) return false;
        }
        return true;
      });
    }
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
    if (view === 'today') {
      // Today view = open tasks that have a date signal pointing at
      // today: due_date today, scheduled_start today, OR overdue
      // (anything past-due needs to be addressed today by default).
      // Snoozed tasks excluded — if you snoozed a task to tomorrow,
      // it shouldn't crowd today's list.
      const today = todayISO();
      out = out.filter((t) => {
        if (t.done || isSnoozed(t)) return false;
        const due = t.dueDate ?? '';
        const sched = t.scheduledStart ? t.scheduledStart.slice(0, 10) : '';
        return due === today || sched === today || (!!due && due < today);
      });
    } else if (view === 'inbox') {
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
    // Smart filter chip — applied last so it always operates on the
    // result of every other dimension. Predicates kept inline since
    // they're tiny; the predicate-by-key map is built outside the
    // derivation to avoid re-allocating it on every refilter.
    if (smartFilter) {
      out = out.filter((t) => smartPredicate(smartFilter, t));
    }
    return out;
  });

  // Swipe-hint visibility — derived here (after `filtered`) so the
  // reactive read order is valid. State (dismissed flag + touch
  // probe) lives near the top of the script with the other UI state.
  let showSwipeHint = $derived(
    isTouchDevice && !swipeHintDismissed && view === 'list' && filtered.length > 0
  );

  // Week-view columns. 7 day columns rolling from today + an
  // "unscheduled" column on the left for open tasks with no date
  // signal, and an "overdue" callout pinned to today's column. The
  // user scans the week, sees their commitments at a glance, and
  // can click any column header to set the smart filter to that day.
  type DayColumn = { date: string; label: string; sublabel: string; isToday: boolean; tasks: Task[] };
  let weekColumns = $derived.by((): { unscheduled: Task[]; overdue: Task[]; days: DayColumn[] } => {
    const today = todayISO();
    const todayD = new Date(today + 'T00:00:00');
    const days: DayColumn[] = [];
    const byDate = new Map<string, Task[]>();
    const unscheduled: Task[] = [];
    const overdue: Task[] = [];
    for (let i = 0; i < 7; i++) {
      const d = new Date(todayD);
      d.setDate(d.getDate() + i);
      const iso = fmtDateISO(d);
      days.push({
        date: iso,
        label: i === 0 ? 'Today' : i === 1 ? 'Tomorrow' : d.toLocaleDateString(undefined, { weekday: 'short' }),
        sublabel: d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }),
        isToday: i === 0,
        tasks: []
      });
      byDate.set(iso, days[i].tasks);
    }
    for (const t of filtered) {
      if (t.done) continue;
      const due = t.dueDate ?? '';
      const sched = t.scheduledStart ? t.scheduledStart.slice(0, 10) : '';
      const anchor = due || sched;
      if (!anchor) {
        unscheduled.push(t);
        continue;
      }
      if (anchor < today) {
        overdue.push(t);
        continue;
      }
      const bucket = byDate.get(anchor);
      if (bucket) bucket.push(t);
      // Tasks beyond +6 days fall off the grid; the user can switch
      // to list view with a future-week filter for those.
    }
    // Sort each column by scheduled time (if any), then priority.
    const cmp = (a: Task, b: Task) => {
      const at = a.scheduledStart ? a.scheduledStart.slice(11) : '99:99';
      const bt = b.scheduledStart ? b.scheduledStart.slice(11) : '99:99';
      if (at !== bt) return at.localeCompare(bt);
      return (a.priority || 9) - (b.priority || 9);
    };
    for (const col of days) col.tasks.sort(cmp);
    unscheduled.sort((a, b) => (a.priority || 9) - (b.priority || 9));
    overdue.sort((a, b) => (a.dueDate ?? '').localeCompare(b.dueDate ?? ''));
    return { unscheduled, overdue, days };
  });

  // Smart-filter live counts — how many tasks would each chip show
  // given the OTHER active filters. We don't recompute the full filter
  // chain; we just take the pre-smart `filtered` array (which is the
  // result of applying every other dimension) and run each predicate.
  //
  // The trick: this $derived reads `filtered` AND `smartFilter`, so
  // when the chip is set, `filtered` is narrowed by it and the counts
  // would all sum to the visible total — useless. We use a separate
  // counter that walks `tasks` directly with the non-smart filters
  // duplicated. To avoid duplicating that whole predicate, we just
  // count against the loaded tasks (no filters applied) — simple and
  // gives the user "across the whole vault, how many overdue exist?".
  // Other active filters are still applied to the visible list.
  let smartCounts = $derived.by(() => {
    const counts: Record<string, number> = {};
    const filters: SmartFilter[] = ['overdue', 'today', 'tomorrow', 'thisWeek', 'noDue', 'noPriority', 'highPriority', 'hasSubtasks', 'hasEstimate', 'noEstimate'];
    for (const f of filters) counts[f] = 0;
    for (const t of tasks) {
      if (t.archived && archivedMode === 'hide') continue;
      if (isSnoozed(t) && status === 'open') continue;
      for (const f of filters) {
        if (smartPredicate(f, t)) counts[f]++;
      }
    }
    return counts;
  });

  // Smart-filter predicates. Each takes a task and returns true if
  // the task belongs in that smart-filter bucket. Computed against
  // today's date (re-derived per call so a long-lived session that
  // crosses midnight rolls over without a reload).
  function smartPredicate(sf: SmartFilter, t: Task): boolean {
    if (!sf) return true;
    const today = todayISO();
    const tomorrow = (() => {
      const d = new Date(today + 'T00:00:00');
      d.setDate(d.getDate() + 1);
      return fmtDateISO(d);
    })();
    const weekEnd = (() => {
      const d = new Date(today + 'T00:00:00');
      d.setDate(d.getDate() + 7);
      return fmtDateISO(d);
    })();
    const due = t.dueDate ?? '';
    const sched = t.scheduledStart ? t.scheduledStart.slice(0, 10) : '';
    const dateSignal = due || sched;
    switch (sf) {
      case 'overdue':
        return !t.done && !!due && due < today;
      case 'today':
        return !t.done && (due === today || sched === today);
      case 'tomorrow':
        return !t.done && (due === tomorrow || sched === tomorrow);
      case 'thisWeek':
        return !t.done && !!dateSignal && dateSignal >= today && dateSignal <= weekEnd;
      case 'noDue':
        return !t.done && !due && !sched;
      case 'noPriority':
        return !t.done && (t.priority === 0 || t.priority === 4);
      case 'highPriority':
        return !t.done && t.priority === 1;
      case 'hasSubtasks':
        return !t.done && (childCount.get(t.id) ?? 0) > 0;
      case 'hasEstimate':
        return !t.done && !!t.estimatedMinutes && t.estimatedMinutes > 0;
      case 'noEstimate':
        return !t.done && (!t.estimatedMinutes || t.estimatedMinutes === 0);
      default:
        return true;
    }
  }

  // At-a-glance stats over the unfiltered open task list. Surfaced
  // as small chips above the list so the user always knows the
  // overall load — even when a filter is hiding most of it. Numbers
  // are debounced through $derived so they don't flicker mid-edit.
  // Subtask collapse state. Stored as a flat set of parent task IDs;
  // a task whose ANY ancestor is in this set is hidden from the
  // rendered list. Persisted to localStorage so collapse state
  // survives a refresh, but only IDs that still exist in the current
  // task list are kept (prevents the set from growing forever).
  const COLLAPSE_KEY = 'granit.tasks.collapsed';
  let collapsedIds = $state<Set<string>>(new Set(loadStored<string[]>(COLLAPSE_KEY, [])));
  $effect(() => saveStored(COLLAPSE_KEY, Array.from(collapsedIds)));

  // Parent map: for every task with indent > 0, finds its parent in
  // the same notePath (the nearest preceding task with smaller
  // indent). Built once per task-list update so the collapse logic
  // doesn't recompute O(N²) on every render.
  let parentMap = $derived.by(() => {
    const m = new Map<string, string>();
    // Group by notePath then walk in line order so the parent search
    // is bounded to within a note.
    const byNote: Record<string, Task[]> = {};
    for (const t of tasks) (byNote[t.notePath] ??= []).push(t);
    for (const list of Object.values(byNote)) {
      list.sort((a, b) => a.lineNum - b.lineNum);
      const stack: Task[] = [];
      for (const t of list) {
        const ind = t.indent ?? 0;
        while (stack.length > 0 && (stack[stack.length - 1].indent ?? 0) >= ind) {
          stack.pop();
        }
        if (stack.length > 0) m.set(t.id, stack[stack.length - 1].id);
        stack.push(t);
      }
    }
    return m;
  });

  // Inverse: parentId -> child IDs. Used to know whether a task
  // even HAS children (so we can show the chevron) and to count
  // them in the toggle label.
  let childCount = $derived.by(() => {
    const c = new Map<string, number>();
    for (const childId of parentMap.keys()) {
      const parent = parentMap.get(childId)!;
      c.set(parent, (c.get(parent) ?? 0) + 1);
    }
    return c;
  });

  // Walk ancestry; returns true if any ancestor is collapsed.
  function isHiddenByCollapse(taskId: string, collapsed: Set<string>): boolean {
    let cur: string | undefined = parentMap.get(taskId);
    while (cur) {
      if (collapsed.has(cur)) return true;
      cur = parentMap.get(cur);
    }
    return false;
  }

  function toggleCollapsed(taskId: string) {
    const next = new Set(collapsedIds);
    if (next.has(taskId)) next.delete(taskId);
    else next.add(taskId);
    collapsedIds = next;
  }

  // Saved filter presets — name a combination of status / q / tag /
  // project / priority / goal / deadline / view / groupBy, pin it
  // as a one-click chip above the stats row. Persisted to
  // localStorage. Useful for "P1 this week", "Inbox", "Project X —
  // open", etc — the kind of saved-views feature power users rely
  // on.
  type FilterPreset = {
    name: string;
    status: 'open' | 'done' | 'all';
    q: string;
    // Legacy string `tag` was a single tag; newer presets persist
    // the multi-tag array directly. captureCurrentAsPreset writes
    // both fields so older code paths reading `tag` still work,
    // and applyPreset prefers the array when present.
    tag: string;
    tags?: string[];
    project: string;
    priority: number | '';
    goal: string;
    deadline: string;
    view: View;
    groupBy: Group;
    // Newer fields — old presets without them load with falsy
    // defaults via the `?? ''` reads in applyPreset.
    sortBy?: SortBy;
    sourceFilter?: 'all' | 'task-notes';
    smartFilter?: SmartFilter;
    archivedMode?: 'hide' | 'show' | 'only';
  };
  const PRESETS_KEY = 'granit.tasks.presets';
  let presets = $state<FilterPreset[]>(loadStored<FilterPreset[]>(PRESETS_KEY, []));
  function persistPresets() {
    saveStored(PRESETS_KEY, presets);
  }
  function captureCurrentAsPreset() {
    const name = prompt('Name this filter preset:', '');
    if (!name || !name.trim()) return;
    const trimmed = name.trim();
    const next = presets.filter((p) => p.name !== trimmed);
    next.unshift({
      name: trimmed,
      status, q, tag: tagFilters[0] ?? '', tags: [...tagFilters], project: projectFilter,
      priority: priorityFilter, goal: goalFilter, deadline: deadlineFilter,
      view, groupBy,
      sortBy, sourceFilter, smartFilter, archivedMode
    });
    presets = next;
    persistPresets();
    toast.success(`Saved preset "${trimmed}"`);
  }
  function applyPreset(p: FilterPreset) {
    status = p.status; q = p.q;
    tagFilters = Array.isArray(p.tags) ? [...p.tags] : (p.tag ? [p.tag] : []);
    projectFilter = p.project;
    priorityFilter = p.priority; goalFilter = p.goal; deadlineFilter = p.deadline;
    view = p.view; groupBy = p.groupBy;
    sortBy = p.sortBy ?? 'auto';
    sourceFilter = p.sourceFilter ?? 'all';
    smartFilter = p.smartFilter ?? '';
    archivedMode = p.archivedMode ?? 'hide';
  }
  function deletePreset(name: string) {
    presets = presets.filter((p) => p.name !== name);
    persistPresets();
  }
  function presetMatches(p: FilterPreset): boolean {
    const presetTags = (p.tags && Array.isArray(p.tags)) ? p.tags : (p.tag ? [p.tag] : []);
    if (presetTags.length !== tagFilters.length) return false;
    if (presetTags.some((t, i) => t !== tagFilters[i])) return false;
    return p.status === status && p.q === q
      && p.project === projectFilter && p.priority === priorityFilter
      && p.goal === goalFilter && p.deadline === deadlineFilter
      && p.view === view && p.groupBy === groupBy
      && (p.sortBy ?? 'auto') === sortBy
      && (p.sourceFilter ?? 'all') === sourceFilter
      && (p.smartFilter ?? '') === smartFilter
      && (p.archivedMode ?? 'hide') === archivedMode;
  }

  // Built-in starter presets. Surface a few well-named common filter
  // combos so the presets row isn't empty for first-time users. Only
  // shown when the user has zero saved presets; once they save their
  // own, the starter set hides. Clicking applies the combo; from
  // there the user can tweak and "save current" to make it their own.
  const STARTER_PRESETS: FilterPreset[] = [
    { name: 'P1 this week', status: 'open', q: '', tag: '', project: '', priority: 1, goal: '', deadline: '', view: 'list', groupBy: 'due', smartFilter: 'thisWeek' },
    { name: 'Inbox', status: 'open', q: '', tag: '', project: '', priority: '', goal: '', deadline: '', view: 'inbox', groupBy: 'priority' },
    { name: 'Overdue', status: 'open', q: '', tag: '', project: '', priority: '', goal: '', deadline: '', view: 'list', groupBy: 'priority', smartFilter: 'overdue' },
    { name: 'Quick wins', status: 'open', q: '', tag: '', project: '', priority: '', goal: '', deadline: '', view: 'quickwins', groupBy: 'priority' },
    { name: 'Recently done', status: 'done', q: '', tag: '', project: '', priority: '', goal: '', deadline: '', view: 'review', groupBy: 'due' }
  ];
  let visiblePresets = $derived(presets.length > 0 ? presets : STARTER_PRESETS);

  let stats = $derived.by(() => {
    const today = todayISO();
    // Week boundary: completedAt within the last 7 calendar days
    // (Sunday-relative would surprise users mid-week, so we keep it
    // rolling-7d). Used by the "Done · 7d" chip.
    const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
    let open = 0,
      overdue = 0,
      todayCount = 0,
      doneToday = 0,
      doneWeek = 0,
      snoozed = 0,
      // sumEstMin accumulates estimatedMinutes across OPEN, non-
      // snoozed tasks. Power users budget their day in minutes —
      // surfacing "1280m queued" tells them at a glance whether
      // the filtered list is doable today (a typical focus day is
      // ~5h = 300m). Tasks with no estimate contribute 0 — those
      // get a separate "untouched" counter the user can act on.
      sumEstMin = 0,
      // Count of open non-snoozed tasks with no estimate — power-
      // UI nudge to add estimates so the time budget chip becomes
      // accurate. Shown only when > 0.
      noEstCount = 0,
      // priority accumulator for the average. Skips P0 (unset)
      // because mixing "no priority" with P1/P2/P3 would skew
      // the mean toward 0 and read as falsely-low urgency.
      prioritySum = 0,
      priorityCount = 0;
    for (const t of tasks) {
      const sn = isSnoozed(t);
      if (!t.done) {
        open++;
        if (sn) snoozed++;
        else {
          const d = t.dueDate ?? (t.scheduledStart ? t.scheduledStart.slice(0, 10) : '');
          if (d && d < today) overdue++;
          else if (d === today) todayCount++;
        }
        if (t.priority >= 1 && t.priority <= 3) {
          prioritySum += t.priority;
          priorityCount++;
        }
        if (t.estimatedMinutes && t.estimatedMinutes > 0) {
          sumEstMin += t.estimatedMinutes;
        } else {
          noEstCount++;
        }
      } else if (t.completedAt) {
        const day = t.completedAt.slice(0, 10);
        if (day === today) doneToday++;
        if (new Date(t.completedAt).getTime() > sevenDaysAgo) doneWeek++;
      }
    }
    const avgPriority = priorityCount > 0 ? prioritySum / priorityCount : 0;
    return {
      open,
      overdue,
      todayCount,
      doneToday,
      doneWeek,
      snoozed,
      sumEstMin,
      noEstCount,
      avgPriority
    };
  });

  // Per-bucket task comparator. Routed through a derived so every
  // group-by branch can sort buckets through one place — pick a
  // different sortBy and the entire list reshapes consistently.
  // 'auto' preserves the historical due-then-priority shape so
  // existing users aren't surprised on first load.
  let taskComparator = $derived.by(() => {
    const dueOf = (t: Task) => t.dueDate ?? (t.scheduledStart?.slice(0, 10) ?? '');
    const prioOf = (t: Task) => t.priority || 99;
    const ageOf = (t: Task) => t.createdAt ?? '';
    const estOf = (t: Task) => t.estimatedMinutes ?? 0;
    const textOf = (t: Task) => t.text.toLowerCase();
    switch (sortBy) {
      case 'priority':
        // P1 → P2 → P3 → no-priority. Stable tiebreaker on due to
        // keep "same priority" tasks in a sensible order.
        return (a: Task, x: Task) => {
          const d = prioOf(a) - prioOf(x);
          if (d !== 0) return d;
          const ad = dueOf(a), xd = dueOf(x);
          return ad === xd ? 0 : ad < xd ? -1 : 1;
        };
      case 'due':
        // Earliest due first; no-date pushed to the end.
        return (a: Task, x: Task) => {
          const ad = dueOf(a), xd = dueOf(x);
          if (!ad && xd) return 1;
          if (ad && !xd) return -1;
          if (ad !== xd) return ad < xd ? -1 : 1;
          return prioOf(a) - prioOf(x);
        };
      case 'age':
        // Oldest first — surfaces tasks that have been sitting.
        return (a: Task, x: Task) => {
          const aa = ageOf(a), xa = ageOf(x);
          if (aa !== xa) return aa < xa ? -1 : 1;
          return prioOf(a) - prioOf(x);
        };
      case 'alpha':
        // Title A→Z. Locale-aware so accented characters land in
        // the spot a native speaker expects.
        return (a: Task, x: Task) => textOf(a).localeCompare(textOf(x));
      case 'estimate':
        // Smallest estimate first — pair with the Quick-wins view
        // to surface a list of fast tasks for a fragmented hour.
        return (a: Task, x: Task) => {
          const d = estOf(a) - estOf(x);
          if (d !== 0) return d;
          return prioOf(a) - prioOf(x);
        };
      default:
        // 'auto' — date asc, then priority asc. Matches the previous
        // hardcoded sort so a user upgrading from before this option
        // sees identical output.
        return (a: Task, x: Task) => {
          const ad = dueOf(a), xd = dueOf(x);
          if (ad !== xd) return ad < xd ? -1 : 1;
          return prioOf(a) - prioOf(x);
        };
    }
  });

  // Format minutes as a compact human-readable budget — "45m",
  // "3h 20m", "1d 4h". 8h is one "day-block" by convention; the
  // chip stays scannable even on overflowing backlogs.
  function fmtEstBudget(mins: number): string {
    if (mins < 60) return `${mins}m`;
    if (mins < 8 * 60) {
      const h = Math.floor(mins / 60);
      const m = mins - h * 60;
      return m === 0 ? `${h}h` : `${h}h ${m}m`;
    }
    const d = Math.floor(mins / (8 * 60));
    const remH = Math.floor((mins - d * 8 * 60) / 60);
    return remH === 0 ? `${d}d` : `${d}d ${remH}h`;
  }

  // Per-smart-filter counts so the view tabs can show badges.
  // Derived from the unfiltered open task list (sourceFilter +
  // search are applied independently per view; the badge reflects
  // 'is this view worth visiting right now', not 'after filters').
  // Cheap O(n) — recomputes on tasks change but the cache hits often
  // because tasks rarely change while typing.
  let viewCounts = $derived.by(() => {
    const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
    let inbox = 0, stale = 0, quickwins = 0, review = 0;
    for (const t of tasks) {
      if (!t.done) {
        if ((t.triage || 'inbox') === 'inbox') inbox++;
        if (isStale(t)) stale++;
        if (t.priority >= 1 && t.priority <= 2 && t.estimatedMinutes && t.estimatedMinutes <= 30) quickwins++;
      } else if (t.completedAt && new Date(t.completedAt).getTime() > sevenDaysAgo) {
        review++;
      }
    }
    return { inbox, stale, quickwins, review };
  });

  type ListGroup = { key: string; label: string; tasks: Task[]; deepLink?: string };
  let listGroups = $derived.by((): ListGroup[] => {
    if (groupBy === 'due') {
      // Smart-groups: split the previous "Upcoming" bucket into
      // Tomorrow / This week / Later so a user with a long backlog
      // sees the upcoming-week's work without scrolling. Boundaries:
      //   today          — date == today
      //   tomorrow       — date == today+1
      //   this_week      — within next 7 days but past tomorrow
      //   later          — beyond 7 days
      // Each group only renders when non-empty.
      const now = new Date();
      const today = fmtDateISO(now);
      const tmw = new Date(now);
      tmw.setDate(tmw.getDate() + 1);
      const tomorrow = fmtDateISO(tmw);
      const wk = new Date(now);
      wk.setDate(wk.getDate() + 7);
      const weekEnd = fmtDateISO(wk);
      // Stream N — added a 'done' bucket so completed tasks visible
      // via status='done' or status='all' land in their own dedicated
      // section (collapsible, success-tinted) instead of sharing
      // 'today'/'overdue' bins with open tasks. SectionList renders
      // empty buckets as muted single-line headers so the user still
      // sees the structure even when a bucket has zero items.
      const b: Record<string, Task[]> = {
        overdue: [], today: [], tomorrow: [], this_week: [], later: [], no_date: [], done: []
      };
      for (const t of filtered) {
        if (t.done) {
          b.done.push(t);
          continue;
        }
        if (!t.dueDate && !t.scheduledStart) {
          b.no_date.push(t);
          continue;
        }
        const d = t.dueDate ?? (t.scheduledStart ? t.scheduledStart.slice(0, 10) : '');
        if (d < today) b.overdue.push(t);
        else if (d === today) b.today.push(t);
        else if (d === tomorrow) b.tomorrow.push(t);
        else if (d < weekEnd) b.this_week.push(t);
        else b.later.push(t);
      }
      // Per-bucket ordering: 'auto' uses the legacy "date asc, then
      // priority asc" rule; an explicit sortBy choice applies the
      // selected criterion to EVERY bucket so the user gets the
      // same shape regardless of which group they look at.
      Object.values(b).forEach((arr) => arr.sort(taskComparator));
      return [
        { key: 'overdue',   label: 'Overdue',   tasks: b.overdue },
        { key: 'today',     label: 'Today',     tasks: b.today },
        { key: 'tomorrow',  label: 'Tomorrow',  tasks: b.tomorrow },
        { key: 'this_week', label: 'This week', tasks: b.this_week },
        { key: 'later',     label: 'Later',     tasks: b.later },
        { key: 'no_date',   label: 'No date',   tasks: b.no_date },
        { key: 'done',      label: 'Done',      tasks: b.done }
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
      tagFilters.length +
      (goalFilter ? 1 : 0) +
      (deadlineFilter ? 1 : 0) +
      (q ? 1 : 0) +
      (status !== 'open' ? 1 : 0) +
      (sourceFilter !== 'all' ? 1 : 0)
  );

  // Active-filter chip row. Each filter that's not at its default
  // surfaces as a removable chip above the stats row. Lets power
  // users see + clear filters at a glance without opening the
  // filter drawer (mobile) or scrolling the sidebar (desktop).
  // The chips share one shape so the user can dismiss any of them
  // with the same gesture (click the ×). Includes a final "clear
  // all" pill when 2+ filters are active so a stuck-in-narrow-view
  // power user can reset in one click.
  type FilterChip = { key: string; label: string; clear: () => void; tone?: string };
  let activeFilterChips = $derived.by((): FilterChip[] => {
    const out: FilterChip[] = [];
    if (q) {
      out.push({
        key: 'q',
        label: `search: "${q.length > 18 ? q.slice(0, 17) + '…' : q}"`,
        clear: () => (q = '')
      });
    }
    if (status !== 'open') {
      out.push({
        key: 'status',
        label: `status: ${status}`,
        clear: () => (status = 'open')
      });
    }
    if (priorityFilter !== '') {
      const tone =
        priorityFilter === 1 ? 'text-error'
        : priorityFilter === 2 ? 'text-warning'
        : 'text-info';
      out.push({
        key: 'priority',
        label: `P${priorityFilter}`,
        clear: () => (priorityFilter = ''),
        tone
      });
    }
    if (projectFilter) {
      out.push({
        key: 'project',
        label: `project: ${projectFilter.length > 16 ? projectFilter.slice(0, 15) + '…' : projectFilter}`,
        clear: () => (projectFilter = '')
      });
    }
    // One filter chip per active tag — clicking × removes that
    // single tag, not the whole multi-tag filter set.
    for (const t of tagFilters) {
      out.push({
        key: `tag:${t}`,
        label: `#${t.replace(/^#/, '')}`,
        clear: () => (tagFilters = tagFilters.filter((x) => x !== t))
      });
    }
    if (goalFilter) {
      const g = goals.find((x) => x.id === goalFilter);
      out.push({
        key: 'goal',
        label: `goal: ${g?.title ?? goalFilter}`,
        clear: () => (goalFilter = '')
      });
    }
    if (deadlineFilter) {
      const d = deadlines.find((x) => x.id === deadlineFilter);
      out.push({
        key: 'deadline',
        label: `deadline: ${d?.title ?? deadlineFilter}`,
        clear: () => (deadlineFilter = '')
      });
    }
    if (sourceFilter !== 'all') {
      out.push({
        key: 'source',
        label: 'task notes only',
        clear: () => (sourceFilter = 'all')
      });
    }
    if (smartFilter) {
      const labels: Record<string, string> = {
        overdue: 'overdue',
        today: 'today',
        tomorrow: 'tomorrow',
        thisWeek: 'this week',
        noDue: 'no due date',
        noPriority: 'no priority',
        highPriority: 'high priority',
        hasSubtasks: 'has subtasks',
        hasEstimate: 'has estimate',
        noEstimate: 'no estimate'
      };
      out.push({
        key: 'smart',
        label: labels[smartFilter] ?? smartFilter,
        clear: () => (smartFilter = '')
      });
    }
    return out;
  });
  function clearAllFilters() {
    q = '';
    status = 'open';
    priorityFilter = '';
    projectFilter = '';
    tagFilters = [];
    goalFilter = '';
    deadlineFilter = '';
    sourceFilter = 'all';
    smartFilter = '';
  }

  // Stream N — primary view selection from the new TasksPageHeader.
  // Wraps the bare `view = v` assignment so any future view-change
  // side effects (e.g. resetting cursor, closing the More-views menu)
  // route through one place.
  function selectView(v: View) {
    view = v;
    moreViewsOpen = false;
  }

  // Adaptive subtitle for the "no matches" empty state. Mirrors the
  // active-filter set so the user gets a meaningful read instead of a
  // generic "nothing to see here". Order matches user intent: tag /
  // project / goal / search before generic fallback.
  let emptyStateSubtitle = $derived.by((): string => {
    if (tagFilters.length === 1) return `No tasks tagged #${tagFilters[0]}.`;
    if (tagFilters.length > 1) return `No tasks tagged ${tagFilters.map((t) => '#' + t).join(' + ')}.`;
    if (projectFilter) return `No tasks in project "${projectFilter}".`;
    if (goalFilter) {
      const g = goals.find((x) => x.id === goalFilter);
      return `No tasks linked to goal "${g?.title ?? goalFilter}".`;
    }
    if (priorityFilter !== '') return `No P${priorityFilter} tasks here.`;
    if (q.trim()) return `No tasks match "${q.trim()}".`;
    return 'Nothing to do right now.';
  });

  // Trigger the global QuickCaptureFab (Mod-Shift-N opens it). We
  // dispatch a synthetic keystroke rather than expose a new global
  // store, so the existing handler in QuickCaptureFab.svelte owns
  // open-state. Falls through gracefully if the fab isn't mounted.
  function openQuickCapture() {
    const evt = new KeyboardEvent('keydown', {
      key: 'N',
      code: 'KeyN',
      metaKey: true,
      shiftKey: true,
      bubbles: true
    });
    window.dispatchEvent(evt);
  }
</script>

{#snippet filterContent()}
  <div class="p-4 space-y-4">
    <!-- Stream N — slide-out filter panel header. Title + close hint
         so the user knows this is the same surface they opened from
         the toolbar's Filter button. -->
    <div class="flex items-center justify-between border-b border-surface1 pb-2 -mt-1">
      <h2 class="text-sm font-semibold text-text">Filters</h2>
      <button
        type="button"
        onclick={() => (filterPanelOpen = false)}
        aria-label="close filter panel"
        title="Close (Esc)"
        class="text-dim hover:text-text text-xs px-1.5 py-0.5"
      >esc</button>
    </div>

    <!-- Search. data-page-search="1" lets the global `/` shortcut
         focus this input — the slide-out panel renders its content
         in DOM at all times (translated off-screen when closed), so
         the global handler can still find + focus it. -->
    <div>
      <label class="text-xs uppercase tracking-wider text-dim mb-1 block" for="tasks-search">Search</label>
      <input
        id="tasks-search"
        bind:value={q}
        placeholder="search task text or path…"
        data-page-search="1"
        class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
      />
    </div>

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

    <!-- Archived view toggle. Default hides archived tasks (soft-
         deleted via the TaskDetail Archive button). 'Show' includes
         them in the active list, dimmed + dashed-border so the user
         can tell archived from live. 'Only' is the archive drawer
         view — used to find what to restore. -->
    <div>
      <div class="text-xs uppercase tracking-wider text-dim mb-2">Archived</div>
      <div class="flex flex-col gap-1 text-sm">
        <button
          class="text-left px-3 py-2 rounded {archivedMode === 'hide' ? 'bg-surface1 text-text' : 'text-subtext hover:bg-surface0'}"
          onclick={() => (archivedMode = 'hide')}
          title="Hide archived tasks (default)"
        >Hide</button>
        <button
          class="text-left px-3 py-2 rounded {archivedMode === 'show' ? 'bg-surface1 text-text' : 'text-subtext hover:bg-surface0'}"
          onclick={() => (archivedMode = 'show')}
          title="Show active + archived together"
        >Show all</button>
        <button
          class="text-left px-3 py-2 rounded {archivedMode === 'only' ? 'bg-surface1 text-warning' : 'text-warning hover:bg-surface0'}"
          onclick={() => (archivedMode = 'only')}
          title="Only archived — used for restore"
        >Archived only</button>
      </div>
    </div>

    <!-- Source filter — 'all' (default) surfaces every `- [ ]` line
         in the vault, including checkboxes inline in arbitrary notes
         (Amplenote-style capture). 'Task notes only' narrows to notes
         that look like dedicated task surfaces: daily notes and
         anything under Daily/, Tasks/, Projects/. Flip when reading
         notes' visual bullets pollute the view. -->
    <div>
      <div class="text-xs uppercase tracking-wider text-dim mb-2">Source</div>
      <div class="flex flex-col gap-1 text-sm">
        <button
          class="text-left px-3 py-2 rounded {sourceFilter === 'all' ? 'bg-surface1 text-text' : 'text-subtext hover:bg-surface0'}"
          onclick={() => (sourceFilter = 'all')}
          title="Show every - [ ] checkbox the parser found in the vault"
        >
          All notes
        </button>
        <button
          class="text-left px-3 py-2 rounded {sourceFilter === 'task-notes' ? 'bg-surface1 text-text' : 'text-subtext hover:bg-surface0'}"
          onclick={() => (sourceFilter = 'task-notes')}
          title="Daily notes, Tasks/, Projects/, Daily/ — skip bullets in arbitrary notes"
        >
          Task notes only
        </button>
      </div>
    </div>

    <div>
      <div class="text-xs uppercase tracking-wider text-dim mb-2">Priority</div>
      <div class="flex flex-col gap-1 text-sm">
        <button class="text-left px-3 py-2 rounded {priorityFilter === '' ? 'bg-surface1' : 'hover:bg-surface0'} text-subtext" onclick={() => (priorityFilter = '')}>any</button>
        <button class="text-left px-3 py-2 rounded {priorityFilter === 1 ? 'bg-surface0 text-error' : 'hover:bg-surface1 text-error'}" onclick={() => (priorityFilter = 1)}>P1 high</button>
        <button class="text-left px-3 py-2 rounded {priorityFilter === 2 ? 'bg-surface0 text-warning' : 'hover:bg-surface1 text-warning'}" onclick={() => (priorityFilter = 2)}>P2 medium</button>
        <button class="text-left px-3 py-2 rounded {priorityFilter === 3 ? 'bg-surface0 text-info' : 'hover:bg-surface1 text-info'}" onclick={() => (priorityFilter = 3)}>P3 low</button>
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
            {@const active = tagFilters.includes(t)}
            <button
              class="text-xs px-2 py-1 rounded {active ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
              onclick={() => (tagFilters = active ? tagFilters.filter((x) => x !== t) : [...tagFilters, t])}
              title={active ? `Remove #${t} from filter` : `Add #${t} to filter (AND-combine with current)`}
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
              class="text-left px-3 py-2 rounded text-sm truncate {goalFilter === g.id ? 'bg-surface0 text-info' : 'text-subtext hover:bg-surface1'}"
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
              class="text-left px-3 py-2 rounded text-sm truncate {deadlineFilter === d.id ? 'bg-surface0 text-warning' : 'text-subtext hover:bg-surface1'}"
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
      onclick={() => { priorityFilter = ''; projectFilter = ''; tagFilters = []; goalFilter = ''; deadlineFilter = ''; q = ''; }}
      class="w-full text-xs text-dim hover:text-text underline pt-2"
    >
      reset filters
    </button>

    <!-- Stream N — passive stats at the bottom of the panel. The
         previous always-on stat row is gone; advanced users who want
         these live numbers find them here. avgPriority / noEstCount
         / snoozed all live here so the main chrome stays calm. -->
    <div class="border-t border-surface1 pt-3">
      <div class="text-xs uppercase tracking-wider text-dim mb-2">Stats</div>
      <div class="grid grid-cols-2 gap-1.5 text-xs">
        <div class="flex items-baseline justify-between px-2 py-1.5 bg-surface0 rounded font-mono tabular-nums">
          <span class="text-dim">open</span>
          <span class="text-text font-semibold">{stats.open}</span>
        </div>
        {#if stats.snoozed > 0}
          <div class="flex items-baseline justify-between px-2 py-1.5 bg-surface0 rounded font-mono tabular-nums" title="Currently snoozed">
            <span class="text-dim">snoozed</span>
            <span class="text-dim font-semibold">{stats.snoozed}</span>
          </div>
        {/if}
        {#if stats.doneToday > 0}
          <div class="flex items-baseline justify-between px-2 py-1.5 bg-surface0 rounded font-mono tabular-nums" title="Completed today">
            <span class="text-dim">done today</span>
            <span class="text-success font-semibold">{stats.doneToday}</span>
          </div>
        {/if}
        {#if stats.doneWeek > 0}
          <div class="flex items-baseline justify-between px-2 py-1.5 bg-surface0 rounded font-mono tabular-nums" title="Completed in the last 7 days — rolling weekly velocity">
            <span class="text-dim">done · 7d</span>
            <span class="text-success font-semibold">{stats.doneWeek}</span>
          </div>
        {/if}
        {#if stats.sumEstMin > 0}
          <div class="flex items-baseline justify-between px-2 py-1.5 bg-surface0 rounded font-mono tabular-nums" title="Total estimated minutes across open non-snoozed tasks. 8h = one day-block.">
            <span class="text-dim">Σ est</span>
            <span class="text-secondary font-semibold">{fmtEstBudget(stats.sumEstMin)}</span>
          </div>
        {/if}
        {#if stats.noEstCount > 0}
          <div class="flex items-baseline justify-between px-2 py-1.5 bg-surface0 rounded font-mono tabular-nums" title="Open tasks with no time estimate — add est:30m to make Σ accurate">
            <span class="text-dim">no estimate</span>
            <span class="text-dim font-semibold">{stats.noEstCount}</span>
          </div>
        {/if}
        {#if stats.avgPriority > 0}
          {@const ap = stats.avgPriority}
          {@const apTone = ap < 1.5 ? 'text-error' : ap < 2.5 ? 'text-warning' : 'text-info'}
          <div class="flex items-baseline justify-between px-2 py-1.5 bg-surface0 rounded font-mono tabular-nums" title="Average priority across prioritised open tasks (1=high, 3=low)">
            <span class="text-dim">avg pri</span>
            <span class="{apTone} font-semibold">P{ap.toFixed(1)}</span>
          </div>
        {/if}
      </div>
    </div>
  </div>
{/snippet}

<div class="flex h-full">
  <!-- Stream N — slide-out filter panel (right side, responsive). The
       previous always-on desktop sidebar is gone; advanced filtering
       is one click away from the header's Filter button. The drawer
       renders its content in the DOM at all times (just translated
       off-screen) so global `/` page-search can still focus the
       embedded search input. -->
  <Drawer bind:open={filterPanelOpen} side="right" responsive={true} width="w-80 sm:w-96">
    {@render filterContent()}
  </Drawer>

  <div class="flex-1 flex flex-col min-w-0">
    <!-- Stream N — single-row slim page header. Title + counts on the
         left, view-switcher segmented control + More-views dropdown
         + density + filter + capture + help on the right. Saves ~50%
         vertical space vs the previous two-row layout. -->
    <TasksPageHeader
      view={view}
      totalCount={tasks.length}
      filteredCount={filtered.length}
      activeFilterCount={activeFilterCount}
      density={density}
      todayLoad={stats.overdue + stats.todayCount}
      todayOverdue={stats.overdue}
      inboxLoad={viewCounts.inbox}
      moreViewsOpen={moreViewsOpen}
      activeOverflowLabel={activeOverflowLabel}
      onSelectView={selectView}
      onToggleMoreViews={() => (moreViewsOpen = !moreViewsOpen)}
      onPickOverflowView={pickOverflowView}
      onMoreViewsKey={onMoreViewsKey}
      onToggleDensity={() => (density = density === 'compact' ? 'normal' : 'compact')}
      onToggleFilterPanel={() => (filterPanelOpen = !filterPanelOpen)}
      onQuickCapture={openQuickCapture}
      onToggleHelp={() => (helpOpen = !helpOpen)}
    />

    <!-- Stream N — quick-filter chip row, always visible. 6 chips
         (All · Today · Overdue · P1 · No date · Done) — the
         single-click smart filters that let the user re-shape the
         list without opening the filter panel. Horizontal scroll on
         mobile (no wrap) so the row stays one line. -->
    <QuickFilterChips
      smartFilter={smartFilter}
      status={status}
      counts={{
        overdue: smartCounts.overdue,
        today: smartCounts.today,
        noDue: smartCounts.noDue,
        highPriority: smartCounts.highPriority
      }}
      doneCount={countDone}
      activeFilterCount={activeFilterCount}
      onSetSmart={(s) => (smartFilter = s)}
      onSetStatus={(s) => (status = s)}
      onClearAll={clearAllFilters}
    />

    {#if view === 'list' || view === 'kanban' || view === 'today'}
      <!-- AI Plan-my-day. Different agent from triage/
           deadline-detect: those operate on UNTRIAGED tasks;
           this one looks across ALL open tasks and produces a
           sequenced 3-7-task plan budgeted to the user's stated
           focus hours. Returns strict JSON so each row gets its
           own accept/skip controls — accepting pins the task into
           a back-to-back time slot via scheduledStart. Falls back
           to streamed prose if JSON parse fails. Always-visible
           regardless of view so it's reachable from any task
           context. -->
      {#if $focusPlan.busy || $focusPlan.response || $focusPlan.error || $focusPlan.plan.length > 0}
        <div class="px-3 py-3 border-b border-surface1 flex-shrink-0 bg-surface1">
          <div class="flex items-baseline gap-2 mb-2 flex-wrap">
            <span class="text-xs uppercase tracking-wider text-secondary font-semibold">Plan my day</span>
            {#if $focusPlan.plan.length > 0 && !$focusPlan.busy}
              {@const totalEst = $focusPlan.plan.reduce((s, p) => s + Math.max(15, p.estimateMinutes || 30), 0)}
              <span class="text-[11px] text-dim font-mono tabular-nums">{$focusPlan.plan.length} task{$focusPlan.plan.length === 1 ? '' : 's'} · {totalEst}m</span>
            {/if}
            <span class="flex-1"></span>
            {#if $focusPlan.busy}
              <button onclick={() => focusPlan.cancel()} class="text-[11px] text-warning hover:underline">cancel</button>
            {:else}
              {#if $focusPlan.plan.length > 0}
                <button onclick={() => void focusPlan.acceptAll(tasks, load)} class="text-[11px] text-success hover:underline" title="Pin every remaining plan item back-to-back starting now">accept all</button>
              {/if}
              <button onclick={() => void focusPlan.run(tasks, aiFocusHours)} class="text-[11px] text-secondary hover:underline">↻ regenerate</button>
              <button onclick={() => focusPlan.dismiss()} class="text-[11px] text-dim hover:text-error">dismiss</button>
            {/if}
          </div>
          {#if $focusPlan.error}
            <div class="text-xs text-error">{$focusPlan.error}</div>
          {:else if $focusPlan.plan.length > 0}
            <!-- Structured plan view. Each row has its own accept/skip,
                 so the user can take 4 of 5 suggestions without burning
                 the call. -->
            <ol class="space-y-1.5">
              {#each $focusPlan.plan as p (p.taskId)}
                {@const t = tasks.find((x) => x.id === p.taskId)}
                {#if t}
                  <li class="flex items-start gap-2 text-xs">
                    <span class="font-mono text-secondary tabular-nums w-5 flex-shrink-0 mt-0.5">{p.order}.</span>
                    <div class="flex-1 min-w-0">
                      <div class="text-text">
                        <span class="font-medium">{t.text}</span>
                        <span class="text-dim ml-2 font-mono tabular-nums">{Math.max(15, p.estimateMinutes || 30)}m</span>
                      </div>
                      {#if p.rationale}
                        <div class="text-dim mt-0.5 italic">{p.rationale}</div>
                      {/if}
                    </div>
                    <button
                      onclick={() => void focusPlan.acceptItem(p, tasks, load)}
                      class="px-2 py-0.5 bg-surface0 text-success rounded hover:bg-surface1 flex-shrink-0"
                      title="Pin this task into a time slot today"
                    >accept</button>
                    <button
                      onclick={() => focusPlan.skipItem(p.taskId)}
                      class="px-2 py-0.5 text-dim hover:text-text flex-shrink-0"
                    >skip</button>
                  </li>
                {/if}
              {/each}
            </ol>
            {#if $focusPlan.skipped}
              <p class="text-[11px] text-dim italic mt-2 pt-2 border-t border-surface1">Skipped: {$focusPlan.skipped}</p>
            {/if}
            <p class="text-[10px] text-dim mt-2">Context: {tasks.filter((t) => !t.done).slice(0, 30).length} open tasks shown · {aiFocusHours}h focus budget</p>
          {:else}
            <!-- Streaming/fallback view: show the raw model output while
                 we wait for the JSON to close, OR if parsing fails. -->
            <div class="prose prose-sm max-w-none text-sm">
              <MarkdownRenderer body={$focusPlan.response || '_thinking…_'} />
            </div>
          {/if}
        </div>
      {/if}

      <!-- Ask Tasks — chat-style Q&A across the currently-visible
           task set. Streams a markdown answer the user can read,
           copy, or dismiss. No mutations; pure analysis. The trigger
           sits in the quick-add bar below; this component handles
           everything once `open` flips true. -->
      <AskTasks bind:open={askTasksOpen} filtered={filtered} />

      <!-- Quick-add bar. Type a single-line task in granit's
           parser-friendly syntax; Enter creates it in today's daily
           note. Single most-impactful "more powerful tasks" change:
           creating a task no longer requires opening a note. -->
      <div class="px-3 py-2 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
        <span class="text-xl text-primary leading-none flex-shrink-0" aria-hidden="true">＋</span>
        <input
          bind:value={quickAdd}
          onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); void submitQuickAdd(); } }}
          placeholder="Quick-add a task — e.g. fix login bug !1 due:tomorrow #frontend"
          aria-label="Quick-add task"
          disabled={quickAddBusy}
          class="flex-1 min-w-0 px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary disabled:opacity-60"
        />
        <button
          onclick={() => void submitQuickAdd()}
          disabled={!quickAdd.trim() || quickAddBusy}
          class="px-3 py-2 bg-primary text-on-primary rounded text-sm disabled:opacity-50 flex-shrink-0"
        >{quickAddBusy ? '…' : 'Add'}</button>
        <!-- Focus-hours input + Plan-my-day trigger. The hours value
             feeds the AI's budget so it doesn't propose 8h of work
             when the user has 2h available. Persisted in
             localStorage so the user only sets it once.
             Step is 0.5; clamps 0.5-12h. -->
        <label class="hidden sm:inline-flex items-center gap-1 text-xs text-dim flex-shrink-0" title="Focus hours available today — feeds the Plan-my-day budget">
          <input
            type="number"
            min="0.5"
            max="12"
            step="0.5"
            bind:value={aiFocusHours}
            class="w-12 px-1 py-1 bg-surface0 border border-surface1 rounded text-text text-xs tabular-nums text-center focus:outline-none focus:border-primary"
            aria-label="Focus hours available today"
          />
          <span>h</span>
        </label>
        <button
          onclick={() => void focusPlan.run(tasks, aiFocusHours)}
          disabled={$focusPlan.busy || tasks.filter((t) => !t.done).length === 0}
          title="AI builds a sequenced day-plan budgeted to your focus hours"
          class="hidden sm:inline-flex px-3 py-2 text-sm bg-surface1 border border-surface2 text-primary rounded hover:border-primary disabled:opacity-50 flex-shrink-0 items-center gap-1.5"
        >
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5L12 3z"/>
          </svg>
          <span>{$focusPlan.busy ? 'planning…' : 'Plan day'}</span>
        </button>
        <!-- Ask Tasks — opens a Q&A panel above the list. The model
             answers from the loaded task set as context. No mutations
             — pure read surface for "which P1 has no due date?" /
             "what's blocked?" / "summarize today's commitments" -->
        <button
          onclick={startAskTasks}
          disabled={filtered.length === 0}
          title="Ask AI a question about your current task view"
          class="inline-flex px-3 py-2 text-sm bg-surface1 border border-surface2 text-primary rounded hover:border-primary disabled:opacity-50 flex-shrink-0 items-center gap-1.5"
        >
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="9"/>
            <path d="M9.5 9a2.5 2.5 0 0 1 5 0c0 1.5-2.5 2-2.5 3.5"/>
            <path d="M12 17h0"/>
          </svg>
          <span>Ask tasks</span>
        </button>
      </div>
      <!-- Saved filter presets. One-click application of a stored
           filter combo. The "+ save" chip captures the current
           filter state under a name; clicking a preset chip
           re-applies all stored fields. Long-press / right-click to
           delete via the small × on the active chip. -->
      {#if visiblePresets.length > 0 || true}
        <div class="px-3 py-1.5 border-b border-surface1 flex items-center gap-1.5 text-xs flex-shrink-0 flex-wrap">
          <span class="text-dim font-mono uppercase tracking-wider">presets</span>
          {#if presets.length === 0}
            <span class="text-[10px] text-dim italic font-mono" title="Built-in starter presets — save your own and these go away">starter</span>
          {/if}
          {#each visiblePresets as p (p.name)}
            {@const active = presetMatches(p)}
            {@const isStarter = presets.length === 0}
            <span
              class="inline-flex items-center rounded overflow-hidden border
                {active ? 'border-primary bg-surface1 text-primary' : 'border-surface1 bg-surface0 text-subtext hover:border-primary'}"
            >
              <button
                onclick={() => applyPreset(p)}
                class="px-2 py-0.5"
              >{p.name}</button>
              {#if active && !isStarter}
                <button
                  onclick={() => deletePreset(p.name)}
                  title="Remove preset"
                  class="px-1.5 py-0.5 text-dim hover:text-error border-l border-surface1"
                >×</button>
              {/if}
            </span>
          {/each}
          <button
            onclick={captureCurrentAsPreset}
            title="Save the current filters as a named preset"
            class="px-2 py-0.5 text-dim hover:text-primary border border-dashed border-surface1 hover:border-primary rounded"
          >+ save current</button>
        </div>
      {/if}
      <!-- Active-filter chip row. Surfaces every non-default filter
           as an x-removable chip so the user can SEE what's filtering
           the visible list and dismiss any single one in one click —
           no need to open the filter drawer (mobile) or hunt the
           sidebar (desktop). Hidden when no filters are active.
           "Clear all" pill appears once 2+ filters are active. -->
      {#if activeFilterChips.length > 0}
        <div class="px-3 py-1.5 border-b border-surface1 flex items-center gap-1 text-[11px] flex-shrink-0 flex-wrap bg-surface0/40">
          <span class="text-[10px] uppercase tracking-wider text-dim mr-1 select-none">Filters</span>
          {#each activeFilterChips as chip (chip.key)}
            <span class="inline-flex items-center gap-1 px-1.5 py-0.5 bg-surface0 border border-surface1 font-mono tabular-nums {chip.tone ?? 'text-subtext'}">
              <span class="select-none">{chip.label}</span>
              <button
                type="button"
                onclick={chip.clear}
                aria-label="clear {chip.key} filter"
                title="Remove this filter"
                class="text-dim hover:text-error leading-none px-1 -mx-1"
              >×</button>
            </span>
          {/each}
          {#if activeFilterChips.length >= 2}
            <button
              type="button"
              onclick={clearAllFilters}
              title="Reset every active filter to its default"
              class="ml-1 px-1.5 py-0.5 text-[10px] uppercase tracking-wider text-warning hover:text-error border border-dashed border-warning hover:border-error"
            >clear all</button>
          {/if}
          <span class="flex-1"></span>
          <span class="text-[10px] text-dim font-mono tabular-nums select-none">{filtered.length} match{filtered.length === 1 ? '' : 'es'}</span>
        </div>
      {/if}
      <!-- Stream N — slim contextual sub-toolbar. Only shown for list
           and kanban views. The visual noise of the previous always-
           on smartCounts row is gone; key counts (done today / week /
           estimate budget / avg priority) live in the slide-out
           filter panel as informational chips. Group/sort/columns
           selectors stay here because they reshape the visible list
           and the user reaches for them frequently. -->
      {#if view === 'list' || view === 'kanban'}
        <div class="px-3 py-1.5 border-b border-surface1 flex items-center gap-2 text-xs flex-shrink-0 flex-wrap bg-mantle">
          {#if view === 'list'}
            <span class="text-dim font-mono uppercase tracking-wider select-none">group</span>
            <select
              bind:value={groupBy}
              title="How to split the list into sections"
              class="bg-surface0 border border-surface1 rounded px-2 py-0.5 text-text"
            >
              <option value="due">due date</option>
              <option value="priority">priority</option>
              <option value="tag">tag</option>
              <option value="project">project</option>
              <option value="goal">goal</option>
              <option value="deadline">deadline</option>
              <option value="note">note</option>
            </select>
            <span class="text-dim font-mono uppercase tracking-wider select-none">sort</span>
            <select
              bind:value={sortBy}
              title="How to order tasks inside each group"
              class="bg-surface0 border border-surface1 rounded px-2 py-0.5 text-text"
            >
              <option value="auto">auto</option>
              <option value="priority">priority</option>
              <option value="due">due</option>
              <option value="age">age (oldest first)</option>
              <option value="alpha">A → Z</option>
              <option value="estimate">estimate (smallest)</option>
            </select>
          {:else}
            <span class="text-dim font-mono uppercase tracking-wider select-none">columns</span>
            <select bind:value={kanbanMode} class="bg-surface0 border border-surface1 rounded px-2 py-0.5 text-text">
              <option value="priority">priority</option>
              <option value="due">due</option>
              <option value="triage">triage (granit)</option>
              <option value="config">config</option>
            </select>
          {/if}
          <span class="flex-1"></span>
          <!-- Tiny passive stats — done today / done 7d / est budget.
               Live next to the group/sort selectors so the user has
               a one-line at-a-glance signal without the previous
               14-chip stat row. Other stats (noEstCount, avgPriority,
               snoozed) moved to the filter panel. -->
          {#if stats.doneToday > 0}
            <span class="text-success font-mono tabular-nums select-none" title="Completed today">✓ {stats.doneToday}</span>
          {/if}
          {#if stats.doneWeek > 0}
            <span class="text-success/80 font-mono tabular-nums select-none" title="Completed in the last 7 days">7d ✓ {stats.doneWeek}</span>
          {/if}
          {#if stats.sumEstMin > 0}
            <span class="text-secondary font-mono tabular-nums select-none" title="Total estimated minutes across open non-snoozed tasks. 8h = one day-block.">Σ {fmtEstBudget(stats.sumEstMin)}</span>
          {/if}
        </div>
      {/if}
    {/if}

    {#if selectedIds.size > 0}
      <BulkBar
        count={selectedIds.size}
        ids={Array.from(selectedIds)}
        onClear={() => (selectedIds = new Set())}
        onChanged={async () => { selectedIds = new Set(); await load(); }}
      />
    {/if}

    <div class="flex-1 overflow-auto p-2 sm:p-3">
      {#if loading && tasks.length === 0}
        <div class="text-sm text-dim">loading…</div>
      {:else if filtered.length === 0 && view === 'today'}
        <!-- Today view inbox-zero message. Different from a true empty
             state — the user has tasks, just none for today. The
             tone is calm-celebratory rather than the cobwebbed
             "get to work" used by the Review view. -->
        <div class="max-w-md mx-auto py-6 text-center">
          <div class="text-4xl mb-3 opacity-50">🌤</div>
          <h2 class="text-base font-medium text-text mb-1">Today is clear</h2>
          <p class="text-sm text-dim">
            Nothing overdue, nothing due today, nothing scheduled. Take the open space — or pick something from
            <button class="text-primary hover:underline" onclick={() => (view = 'list')}>the full list</button>.
          </p>
        </div>
      {:else if filtered.length === 0 && view === 'review'}
        <div class="bg-surface0 border border-surface1 rounded-lg p-5 text-center">
          <p class="text-sm text-text mb-1">No tasks completed in the last 7 days.</p>
          <p class="text-xs text-dim mb-3">The review tab shows what you've finished — once a few tasks roll through, this is where you'll spot patterns.</p>
          <button
            type="button"
            onclick={() => (view = 'list')}
            class="text-xs px-3 py-1.5 bg-primary text-on-primary rounded font-medium hover:opacity-90"
          >Open task list →</button>
        </div>
      {:else if filtered.length === 0 && view === 'inbox'}
        <div class="bg-surface0 border border-surface1 rounded-lg p-5 text-center">
          <p class="text-sm text-success mb-1">Inbox empty.</p>
          <p class="text-xs text-dim mb-3">Nothing waiting to be triaged. Captured tasks land here for sorting before they hit the main list.</p>
          <button
            type="button"
            onclick={() => (view = 'list')}
            class="text-xs px-3 py-1.5 bg-surface1 border border-surface2 text-text rounded font-medium hover:border-primary"
          >Open task list →</button>
        </div>
      {:else if filtered.length === 0 && view === 'stale'}
        <div class="bg-surface0 border border-surface1 rounded-lg p-5 text-center">
          <p class="text-sm text-success mb-1">No stale tasks.</p>
          <p class="text-xs text-dim">Everything's been touched in the last week — nothing rotting in the backlog.</p>
        </div>
      {:else if filtered.length === 0 && view === 'quickwins'}
        <p class="text-sm text-dim italic">No quick wins available. Add an estimate (e.g. <code class="text-secondary">est:30m</code>) to high-priority tasks.</p>
      {:else if filtered.length === 0 && tasks.length === 0}
        <!-- True empty: no tasks anywhere. Onboarding-style hint
             pointing at the quick-add bar. -->
        <div class="max-w-md mx-auto py-6 text-center">
          <div class="text-5xl mb-3 opacity-30">✓</div>
          <h2 class="text-lg font-semibold text-text mb-2">No tasks yet</h2>
          <p class="text-sm text-dim mb-1">
            Type your first task in the bar above. Examples:
          </p>
          <ul class="text-sm text-subtext font-mono mt-3 space-y-1.5 inline-block text-left">
            <li>fix login bug <span class="text-error">!1</span> <span class="text-secondary">due:tomorrow</span></li>
            <li>buy groceries <span class="text-info">#errands</span></li>
            <li>review PR <span class="text-warning">!2</span> <span class="text-secondary">due:fri</span></li>
          </ul>
        </div>
      {:else if filtered.length === 0}
        <!-- Tasks exist but the active filter masks them all. The
             subtitle adapts to which filter is the dominant signal
             (tag / project / goal / priority / search) so the user
             reads "No tasks tagged #X" instead of a generic "no
             matches". Two CTAs: Quick capture for fast entry and
             Clear filters for the reset path. min-w-0 keeps the
             card from overrunning the sidebar on narrow viewports. -->
        <div class="min-w-0">
          <div class="max-w-md mx-auto py-6 text-center">
            <div class="text-4xl mb-3 opacity-30">🔍</div>
            <h2 class="text-base font-medium text-text mb-2">No tasks here</h2>
            <p class="text-sm text-dim mb-1">{emptyStateSubtitle}</p>
            <p class="text-xs text-dim mb-4">
              {tasks.length} {tasks.length === 1 ? 'task is' : 'tasks are'} hidden by the current filters.
            </p>
            <div class="flex items-center justify-center gap-2 flex-wrap">
              <button
                type="button"
                onclick={openQuickCapture}
                class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90 inline-flex items-center gap-1.5"
                title="Open the global capture modal (Cmd-Shift-N)"
              >
                <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>
                Quick capture
              </button>
              <button
                type="button"
                onclick={clearAllFilters}
                class="px-3 py-1.5 bg-surface0 border border-surface1 hover:border-primary rounded text-sm text-subtext"
              >Clear filters</button>
            </div>
          </div>
        </div>
      {:else if view === 'week'}
        <!-- Week view — 8 columns: Unscheduled + 7 rolling days from
             today. Overdue tasks bubble up as a striped strip pinned
             above today's column so the user doesn't have to hunt
             them across past dates. Each column header is clickable:
             pressing one drops the user into List view filtered to
             that day so they can drill in. -->
        <div class="flex flex-col gap-2">
          {#if weekColumns.overdue.length > 0}
            <div class="bg-surface0 border border-error rounded p-2">
              <div class="flex items-baseline gap-2 mb-1.5">
                <h3 class="text-xs uppercase tracking-wider text-error font-medium">overdue</h3>
                <span class="text-[10px] font-mono text-dim">{weekColumns.overdue.length}</span>
                <button
                  type="button"
                  onclick={() => { smartFilter = 'overdue'; view = 'list'; }}
                  class="ml-auto text-[10px] text-error hover:underline font-mono"
                >open in list →</button>
              </div>
              <div class="space-y-1">
                {#each weekColumns.overdue.slice(0, 5) as t (t.id)}
                  <TaskCard task={t} compact={compactCards} hasChildren={(childCount.get(t.id) ?? 0) > 0} childCount={childCount.get(t.id) ?? 0} collapsed={collapsedIds.has(t.id)} onToggleCollapse={() => toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
                {/each}
                {#if weekColumns.overdue.length > 5}
                  <p class="text-[11px] text-dim italic px-1">…{weekColumns.overdue.length - 5} more</p>
                {/if}
              </div>
            </div>
          {/if}
          <div class="grid grid-cols-[minmax(10rem,1fr)_repeat(7,minmax(0,1fr))] gap-2 min-h-[20rem]">
            <!-- Unscheduled column — capture surface for tasks with
                 no date. The "+ add" button at the bottom kicks off a
                 quick-add that lands without a date so the user can
                 then drag (or click) it into a day column. -->
            <div class="bg-surface0 border border-surface1 rounded p-2 flex flex-col min-h-0">
              <div class="flex items-baseline gap-2 mb-1.5 sticky top-0 bg-surface0 pb-1 border-b border-surface1">
                <h3 class="text-xs uppercase tracking-wider text-dim font-medium">unscheduled</h3>
                <span class="text-[10px] font-mono text-dim">{weekColumns.unscheduled.length}</span>
              </div>
              <div class="flex-1 overflow-y-auto space-y-1">
                {#each weekColumns.unscheduled.slice(0, 50) as t (t.id)}
                  <TaskCard task={t} compact hasChildren={(childCount.get(t.id) ?? 0) > 0} childCount={childCount.get(t.id) ?? 0} collapsed={collapsedIds.has(t.id)} onToggleCollapse={() => toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
                {/each}
                {#if weekColumns.unscheduled.length > 50}
                  <p class="text-[11px] text-dim italic px-1">…{weekColumns.unscheduled.length - 50} more</p>
                {/if}
                {#if weekColumns.unscheduled.length === 0}
                  <p class="text-[11px] text-dim italic px-1">nothing untaken — good shape.</p>
                {/if}
              </div>
            </div>
            <!-- Seven day columns. The today column gets a primary
                 border so the user's eye lands on it first. -->
            {#each weekColumns.days as col (col.date)}
              <div class="bg-surface0 border {col.isToday ? 'border-primary' : 'border-surface1'} rounded p-2 flex flex-col min-h-0">
                <div class="flex items-baseline gap-1.5 mb-1.5 sticky top-0 bg-surface0 pb-1 border-b border-surface1">
                  <button
                    type="button"
                    onclick={() => { view = 'list'; q = ''; smartFilter = col.isToday ? 'today' : (col.date === weekColumns.days[1]?.date ? 'tomorrow' : ''); }}
                    class="text-xs uppercase tracking-wider {col.isToday ? 'text-primary' : 'text-text'} font-medium hover:underline"
                    title="open this day in the list view"
                  >{col.label}</button>
                  <span class="text-[10px] text-dim font-mono">{col.sublabel}</span>
                  <span class="ml-auto text-[10px] font-mono text-dim">{col.tasks.length}</span>
                </div>
                <div class="flex-1 overflow-y-auto space-y-1">
                  {#each col.tasks as t (t.id)}
                    <TaskCard task={t} compact hasChildren={(childCount.get(t.id) ?? 0) > 0} childCount={childCount.get(t.id) ?? 0} collapsed={collapsedIds.has(t.id)} onToggleCollapse={() => toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
                  {/each}
                  {#if col.tasks.length === 0}
                    <p class="text-[11px] text-dim italic px-1">—</p>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        </div>
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
      {:else if view === 'eisenhower'}
        <EisenhowerView
          tasks={filtered}
          onOpenDetail={openDetail}
          onContextMenu={openContext}
          onChanged={load}
        />
      {:else if view === 'triage'}
        <TriageBoard tasks={filtered} onChanged={load} />
      {:else if view === 'inbox'}
        <div class="max-w-3xl">
          <div class="flex items-baseline gap-3 mb-4">
            <p class="text-sm text-dim flex-1">
              Untriaged tasks. Decide for each: schedule, prioritize, drop, or snooze.
            </p>
            {#if $triage.busy}
              <button
                onclick={() => triage.cancel()}
                class="px-3 py-1.5 text-xs bg-surface0 text-warning rounded hover:bg-surface1 flex-shrink-0"
                title="Cancel the in-flight triage call"
              >✨ thinking… cancel</button>
            {:else}
              <button
                onclick={() => void triage.run()}
                disabled={filtered.length === 0}
                class="px-3 py-1.5 text-xs bg-surface1 text-secondary rounded hover:bg-surface2 disabled:opacity-50 flex-shrink-0"
                title="Ask AI to suggest priority + schedule for each untriaged task"
              >✨ AI triage</button>
            {/if}
            {#if $deadline.busy}
              <button
                onclick={() => deadline.cancel()}
                class="px-3 py-1.5 text-xs bg-surface0 text-warning rounded hover:bg-surface1 flex-shrink-0"
                title="Cancel the in-flight deadline scan"
              >✨ thinking… cancel</button>
            {:else}
              <button
                onclick={() => void deadline.run()}
                class="px-3 py-1.5 text-xs bg-surface1 text-secondary rounded hover:bg-surface2 disabled:opacity-50 flex-shrink-0"
                title="Scan all open tasks without a due date — propose ones whose title implies a clear deadline"
              >✨ Detect deadlines</button>
            {/if}
          </div>

          {#if $deadline.proposals.length > 0}
            <!-- Deadline proposals — operates across ALL open tasks
                 without a due_date, not just inbox. Server already
                 filtered out blanks, so every row is a confident
                 suggestion. Apply patches dueDate; skip just dismisses. -->
            <div class="mb-5 p-3 bg-surface0 border border-warning rounded">
              <div class="flex items-center mb-2">
                <div class="text-xs uppercase tracking-wider text-warning font-semibold flex-1">Detected deadlines ({$deadline.proposals.length})</div>
                <button
                  onclick={() => deadline.discard()}
                  class="text-[10px] text-dim hover:text-error"
                  title="Drop all proposals without applying any"
                >discard</button>
              </div>
              <ul class="space-y-2">
                {#each $deadline.proposals as p (p.id)}
                  {@const t = tasks.find((x) => x.id === p.id)}
                  {#if t}
                    <li class="flex items-start gap-2 text-xs">
                      <div class="flex-1 min-w-0">
                        <div class="text-text">{t.text}</div>
                        <div class="text-dim mt-0.5">
                          due <span class="text-warning font-medium">{p.due_date}</span>
                          {#if p.rationale}<span class="italic"> — {p.rationale}</span>{/if}
                        </div>
                      </div>
                      <button
                        onclick={() => void deadline.apply(p, load)}
                        disabled={$deadline.busy}
                        class="px-2 py-0.5 bg-surface0 text-success rounded hover:bg-surface1"
                      >accept</button>
                      <button
                        onclick={() => deadline.skip(p.id)}
                        class="px-2 py-0.5 text-dim hover:text-text"
                      >skip</button>
                    </li>
                  {/if}
                {/each}
              </ul>
            </div>
          {/if}

          {#if $triage.proposals.length > 0}
            <!-- AI suggestions panel. Each proposal has Accept /
                 Skip; accepting applies the suggested priority +
                 schedule to the matching task. -->
            <div class="mb-5 p-3 bg-surface1 border border-surface2 rounded">
              <div class="flex items-center mb-2">
                <div class="text-xs uppercase tracking-wider text-secondary font-semibold flex-1">AI suggestions ({$triage.proposals.length})</div>
                <button
                  onclick={() => triage.discard()}
                  class="text-[10px] text-dim hover:text-error"
                  title="Drop all proposals without applying any"
                >discard</button>
              </div>
              <ul class="space-y-2">
                {#each $triage.proposals as p (p.id)}
                  {@const t = tasks.find((x) => x.id === p.id)}
                  {#if t}
                    <li class="flex items-start gap-2 text-xs">
                      <div class="flex-1 min-w-0">
                        <div class="text-text">{t.text}</div>
                        <div class="text-dim mt-0.5">
                          {p.priority === 0 ? 'drop' : `P${p.priority}`} · {p.schedule}
                          {#if p.rationale}<span class="italic"> — {p.rationale}</span>{/if}
                        </div>
                      </div>
                      <button
                        onclick={() => void triage.apply(p, load)}
                        disabled={$triage.busy}
                        class="px-2 py-0.5 bg-surface0 text-success rounded hover:bg-surface1"
                      >accept</button>
                      <button
                        onclick={() => triage.skip(p.id)}
                        class="px-2 py-0.5 text-dim hover:text-text"
                      >skip</button>
                    </li>
                  {/if}
                {/each}
              </ul>
            </div>
          {/if}

          <div class="space-y-2">
            {#each filtered.filter((tt) => !isHiddenByCollapse(tt.id, collapsedIds)) as t (t.id)}
              <div data-task-id={t.id} class={cursorIdx >= 0 && filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                <TaskCard task={t} compact={compactCards} hasChildren={(childCount.get(t.id) ?? 0) > 0} childCount={childCount.get(t.id) ?? 0} collapsed={collapsedIds.has(t.id)} onToggleCollapse={() => toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
              </div>
            {/each}
          </div>
        </div>
      {:else if view === 'stale'}
        <div class="max-w-3xl">
          <AIStaleVerdicts
            candidates={filtered.filter(isStale)}
            allTasks={tasks}
            onReload={load}
          />

          <div class="space-y-2">
            {#each filtered.filter((tt) => !isHiddenByCollapse(tt.id, collapsedIds)) as t (t.id)}
              <div data-task-id={t.id} class={cursorIdx >= 0 && filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                <TaskCard task={t} compact={compactCards} hasChildren={(childCount.get(t.id) ?? 0) > 0} childCount={childCount.get(t.id) ?? 0} collapsed={collapsedIds.has(t.id)} onToggleCollapse={() => toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
              </div>
            {/each}
          </div>
        </div>
      {:else if view === 'duplicates'}
        <div class="max-w-3xl">
          <TaskDuplicates onReload={load} />
        </div>
      {:else if view === 'quickwins'}
        <div class="max-w-3xl">
          <p class="text-sm text-dim mb-4">High-priority tasks you can finish in ≤30 min. Pick one, knock it out.</p>
          <div class="space-y-2">
            {#each filtered.filter((tt) => !isHiddenByCollapse(tt.id, collapsedIds)) as t (t.id)}
              <div data-task-id={t.id} class={cursorIdx >= 0 && filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                <TaskCard task={t} compact={compactCards} hasChildren={(childCount.get(t.id) ?? 0) > 0} childCount={childCount.get(t.id) ?? 0} collapsed={collapsedIds.has(t.id)} onToggleCollapse={() => toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
              </div>
            {/each}
          </div>
        </div>
      {:else if view === 'review'}
        <div class="max-w-3xl">
          <p class="text-sm text-dim mb-4">Done in the last week — your retrospective view.</p>
          <div class="space-y-2 opacity-80">
            {#each filtered.filter((tt) => !isHiddenByCollapse(tt.id, collapsedIds)) as t (t.id)}
              <div data-task-id={t.id} class={cursorIdx >= 0 && filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                <TaskCard task={t} compact={compactCards} hasChildren={(childCount.get(t.id) ?? 0) > 0} childCount={childCount.get(t.id) ?? 0} collapsed={collapsedIds.has(t.id)} onToggleCollapse={() => toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
              </div>
            {/each}
          </div>
        </div>
      {:else}
        <!-- Stream N — smart-section grouped list. Sections (overdue /
             today / tomorrow / this week / later / no date / done)
             carry visual weight via tinted backgrounds and border-l-2
             on the loudest two. Collapse state per section persists.
             SectionList owns rendering; this page owns the data + the
             callbacks. -->
        {#if showSwipeHint}
          <button
            type="button"
            onclick={dismissSwipeHint}
            class="w-full max-w-3xl text-center text-[11px] text-dim bg-surface0 border border-surface1 rounded py-2 px-3 flex items-center justify-center gap-2 active:bg-surface1 mb-3"
            aria-label="Dismiss swipe hint"
          >
            <span class="text-warning" aria-hidden="true">‹</span>
            <span>swipe left to snooze</span>
            <span class="text-dim">·</span>
            <span>swipe right for done</span>
            <span class="text-success" aria-hidden="true">›</span>
            <span class="text-dim ml-1">(tap to dismiss)</span>
          </button>
        {/if}
        <SectionList
          groups={listGroups}
          filtered={filtered}
          cursorIdx={cursorIdx}
          compactCards={compactCards}
          childCount={childCount}
          collapsedIds={collapsedIds}
          collapsedSections={collapsedSections}
          groupAddKey={groupAddKey}
          bind:groupAddText
          groupAddBusy={groupAddBusy}
          bind:selectedIds
          isHiddenByCollapse={isHiddenByCollapse}
          onToggleSection={toggleSection}
          onToggleCollapse={toggleCollapsed}
          onChanged={load}
          onOpenDetail={openDetail}
          onContextMenu={openContext}
          onStartGroupAdd={(key) => { groupAddKey = groupAddKey === key ? null : key; groupAddText = ''; }}
          onCancelGroupAdd={cancelGroupAdd}
          onSubmitGroupAdd={submitGroupAdd}
          onGroupAddTextChange={(v) => (groupAddText = v)}
        />
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
    class="fixed inset-0 bg-mantle z-50 flex items-center justify-center p-4"
    onclick={() => (helpOpen = false)}
    role="presentation"
  >
    <!-- max-h with dvh keeps the dialog from bleeding behind mobile
         browser chrome / keyboards; overflow-y-auto lets the user
         scroll the shortcut list when the keyboard takes half the
         screen. -->
    <div
      class="bg-surface0 border border-surface1 rounded-lg p-5 max-w-md w-full max-h-[90dvh] overflow-y-auto shadow-xl"
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
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">Shift+A</kbd>
        <span class="text-subtext">select / clear all filtered</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">e</kbd>
        <span class="text-subtext">open task detail</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">d</kbd>
        <span class="text-subtext">toggle done</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">p</kbd>
        <span class="text-subtext">cycle priority (P0→P3)</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">s</kbd>
        <span class="text-subtext">snooze cursor task</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">esc</kbd>
        <span class="text-subtext">clear selection</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">a</kbd>
        <span class="text-subtext">open AI agent (operates on filtered list or bulk-selection)</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">[ / ]</kbd>
        <span class="text-subtext">previous / next view mode</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">1‥4</kbd>
        <span class="text-subtext">jump to today / list / kanban / matrix</span>
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

<!-- When the user has bulk-selected tasks, narrow the agent's
     scope to that selection — the explicit selection IS the
     intent. Otherwise fall back to the page's filtered list so
     "agent over what I'm looking at" is the default. -->
<TaskAgent
  open={agentOpen}
  tasks={selectedIds.size > 0 ? filtered.filter((t) => selectedIds.has(t.id)) : filtered}
  todayISO={todayISO()}
  availableProjects={projects.map((p) => p.name)}
  onClose={() => (agentOpen = false)}
  onChanged={load}
/>
