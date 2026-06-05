<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/stores';
  import { auth } from '$lib/stores/auth';
  import { api, todayISO, type Task, type Project, type Goal, type Deadline } from '$lib/api';
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
  import TasksFilterDrawer from '$lib/tasks/TasksFilterDrawer.svelte';
  import { installTasksKeyboard } from '$lib/tasks/useTasksKeyboard';
  import { createTasksUrlSync } from '$lib/tasks/tasksUrlSync';
  import { createTasksGroupAdd } from '$lib/tasks/tasksGroupAdd.svelte';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';
  import { applyNextPriority, toggleDoneOf } from '$lib/tasks/taskActions';
  import {
    createTriageStore,
    createDeadlineStore,
    createFocusPlanStore
  } from '$lib/tasks/aiAgentStore';
  import { isStale, fmtEstBudget } from '$lib/tasks/tasksHelpers';
  import { createPresetsController } from '$lib/tasks/tasksPresets.svelte';
  import { createTasksFilterState } from '$lib/tasks/tasksFilterState.svelte';
  import { createTasksViewState } from '$lib/tasks/tasksViewState.svelte';
  import { createTasksData } from '$lib/tasks/tasksData.svelte';
  import { workspaceContext } from '$lib/workspace/workspaceContext.svelte';

  // Loaded data (dataCtl.tasks/dataCtl.projects/dataCtl.goals/dataCtl.deadlines), dataCtl.loading flag,
  // dataCtl.parentMap/dataCtl.childCount/dataCtl.allTags/dataCtl.countOpen/dataCtl.countDone/dataCtl.stats, plus
  // per-subtree collapse state moved into $lib/tasks/tasksData. The
  // page still owns load() + WS subscription; both write into dataCtl.
  const dataCtl = createTasksData();
  // Task Agent — conversational action proposer. Sees the
  // currently-filtered task list, takes a free-text intent, returns
  // a list of typed actions the user accepts per-card. Distinct
  // from Plan-day (schedules) and Stale-review (verdicts) — this
  // is the "do something for me" surface.
  let agentOpen = $state(false);
  // Goals + dataCtl.deadlines drive the new group-by options and the group
  // header titles (so a "Q3 launch (G004)" group reads as the goal's
  // title, not the bare ID). Loaded once, then refreshed alongside
  // the task list on WS events.
  // dataCtl.goals + dataCtl.deadlines moved into dataCtl alongside dataCtl.tasks + dataCtl.projects.

  // View + display state — viewMode, viewCtl.groupBy, viewCtl.sortBy, viewCtl.density,
  // viewCtl.kanbanMode/Swimlane, viewCtl.helpOpen/viewCtl.filterPanelOpen/viewCtl.moreViewsOpen, the
  // per-section collapse map, plus the four screen-shape derivations
  // (viewCtl.listGroups, viewCtl.weekColumns, viewCtl.viewCounts, viewCtl.taskComparator) live in
  // $lib/tasks/tasksViewState. The controller reaches filterCtl
  // through getFiltered, and the data sidecars through the same deps
  // bundle filterCtl uses.
  const viewCtl = createTasksViewState({
    getFiltered: () => filterCtl.filtered,
    getTasks: () => dataCtl.tasks,
    getProjects: () => dataCtl.projects,
    getGoals: () => dataCtl.goals,
    getDeadlines: () => dataCtl.deadlines
  });

  // Every filter dimension + the filtered derivation + smartCounts +
  // activeFilterChips + clearAll live in $lib/tasks/tasksFilterState.
  // The controller reads dataCtl.tasks / dataCtl.projects / view / dataCtl.goals / dataCtl.deadlines /
  // dataCtl.childCount via the deps bundle below; the rest of this file reads
  // its state through filterCtl.X.
  const filterCtl = createTasksFilterState({
    getTasks: () => dataCtl.tasks,
    getProjects: () => dataCtl.projects,
    getGoals: () => dataCtl.goals,
    getDeadlines: () => dataCtl.deadlines,
    getView: () => viewCtl.view,
    getChildCount: () => dataCtl.childCount
  });

  // viewCtl.density + viewCtl.compactCards moved into $lib/tasks/tasksViewState.

  // Inline per-group quick-add. Only one group's input is open at a
  // time. Submitting creates a task with the group's defaults applied
  // (due date, priority, project, etc.) so the new row lands in the
  // SAME bucket the user added it from — no scattering across groups.
  // Distinct from the existing toolbar-level quickAdd: that one parses
  // natural language and dumps everything into today's daily; this one
  // is group-scoped and infers defaults from the bucket.
  // Per-group quick-add — open a row scoped to a section in
  // SectionList, type, submit. Defaults (dueDate / priority / project
  // etc.) are inferred from the group key so a task added to the
  // "this_week" bucket lands due in 3 days, one added to a project
  // group lands tagged with that project. Controller lives in
  // $lib/tasks/tasksGroupAdd.
  const groupAddCtl = createTasksGroupAdd({
    getGroupBy: () => viewCtl.groupBy,
    getProjects: () => dataCtl.projects,
    onAdded: load
  });

  // ── Ask Tasks ────────────────────────────────────────────────────
  // Free-form Q&A against the currently-loaded task set lives in
  // $lib/tasks/AskTasks.svelte — that component owns the question /
  // answer state, the streaming, the dismiss path. The parent only
  // owns the open flag (so its trigger button can flip it) and the
  // "no dataCtl.tasks in current view" guard (filterCtl.filtered is the parent's
  // derivation).
  let askTasksOpen = $state(false);
  function startAskTasks() {
    if (filterCtl.filtered.length === 0) {
      toast.info('No tasks in the current view.');
      return;
    }
    askTasksOpen = true;
  }
  // dataCtl.loading flag moved into dataCtl.

  // URL sync: hydrate filter state from ?status=…&priority=…&… on
  // first load so refresh / shared links keep filters intact, and
  // mirror user-driven changes back into the URL via $effect.
  // Without this, the kanban/list filters were per-tab session state
  // — opening a P1-filtered list in a new tab silently lost the
  // filter and the user blamed "the search box".
  // URL ↔ state sync lives in tasksUrlSync. The factory takes the
  // controllers + a page getter; hydrate() reads URL → controllers
  // once (onMount), sync() writes controllers → URL on every change
  // after that.
  const urlSync = createTasksUrlSync({
    filterCtl,
    viewCtl,
    getPage: () => $page,
    onAgentParam: () => (agentOpen = true)
  });
  const hydrateFromUrl = urlSync.hydrate;
  const syncToUrl = urlSync.sync;
  // Stream N — slide-out filter panel. Replaces the always-on desktop
  // sidebar so the default page is cleaner; one click opens advanced
  // filtering. Persists nothing — open-state is session-only so the
  // panel doesn't pop open on every reload.
  // viewCtl.filterPanelOpen + viewCtl.collapsedSections + viewCtl.toggleSection moved into
  // $lib/tasks/tasksViewState — read via viewCtl.X.

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
  // The `showSwipeHint` derived value reads `filterCtl.filtered`, which is
  // declared later in the file (the page's main filter pipeline).
  // Hoisting THAT declaration up is invasive; we instead lazy-evaluate
  // the gate by declaring `showSwipeHint` after `filterCtl.filtered` near the
  // bottom of the script. See the matching $derived below the
  // `filterCtl.filtered` derivation.
  // Context menu state — driven by TaskCard's onContextMenu hook.
  // The menu mounts at the click position with {ctxTask, ctxX, ctxY}.
  let ctxTask = $state<Task | null>(null);
  let ctxX = $state(0);
  let ctxY = $state(0);

  function openDetail(t: Task) {
    detailTask = t;
    detailOpen = true;
    // Publish to the workspace context bus so an AI pane in the
    // adjacent slot can surface this task as context. Best-effort
    // — if the user isn't running TasksPane inside the workspace
    // shell, nothing reads the bus and the publish is a no-op.
    workspaceContext.publish({
      paneKind: 'tasks',
      itemId: t.id,
      label: t.text,
      excerpt: t.notePath
    });
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
  // re-rendered dataCtl.tasks list would re-open the drawer every load.
  let lastFocusedFromUrl = $state<string | null>(null);
  $effect(() => {
    const focusId = $page.url.searchParams.get('focus');
    if (!focusId || dataCtl.tasks.length === 0) return;
    if (focusId === lastFocusedFromUrl) return;
    const t = dataCtl.tasks.find((x) => x.id === focusId);
    if (t) {
      openDetail(t);
      lastFocusedFromUrl = focusId;
    }
  });

  // view + groupBy persistence handled inside viewCtl.

  async function load() {
    if (!$auth) return;
    dataCtl.loading = true;
    try {
      // Honor every server-side filter we expose. The client-side
      // filterCtl.filtered derivation still re-applies these (so the
      // view-specific logic like inbox/stale stays consistent), but
      // pushing them to the server first means we don't ship the
      // entire task graph over the wire when the user wants P1 only.
      const params: Parameters<typeof api.listTasks>[0] = {};
      if (filterCtl.status !== 'all') params.status = filterCtl.status;
      // The backend endpoint accepts a single tag; for multi-tag
      // filters we pass the first to narrow the server response and
      // AND-narrow the rest client-side in the filterCtl.filtered
      // derivation.
      if (filterCtl.tagFilters.length > 0) params.tag = filterCtl.tagFilters[0];
      if (filterCtl.priorityFilter !== '') params.priority = filterCtl.priorityFilter;
      if (filterCtl.projectFilter) params.project = filterCtl.projectFilter;
      if (filterCtl.goalFilter) params.goal = filterCtl.goalFilter;
      if (filterCtl.deadlineFilter) params.deadline = filterCtl.deadlineFilter;
      if (filterCtl.archivedMode === 'show') params.includeArchived = true;
      if (filterCtl.archivedMode === 'only') params.archived = true;
      const [list, p, gg, dd] = await Promise.all([
        api.listTasks(params),
        dataCtl.projects.length === 0
          ? api.listProjects().catch(() => ({ projects: [] as Project[] }))
          : Promise.resolve({ projects: dataCtl.projects }),
        dataCtl.goals.length === 0
          ? api.listGoals().catch(() => ({ goals: [] as Goal[] }))
          : Promise.resolve({ goals: dataCtl.goals }),
        dataCtl.deadlines.length === 0
          ? api.listDeadlines().catch(() => ({ deadlines: [] as Deadline[] }))
          : Promise.resolve({ deadlines: dataCtl.deadlines })
      ]);
      dataCtl.tasks = list.tasks;
      dataCtl.projects = p.projects;
      dataCtl.goals = gg.goals;
      dataCtl.deadlines = dd.deadlines;
    } catch {
      // 401 (stale auth) and network failures both end up here.
      // Silently leave dataCtl.tasks/dataCtl.projects empty so the empty-state copy
      // renders instead of the indefinite dataCtl.loading spinner. A later
      // WS reconnect or filter change will retry naturally — no
      // toast, no console noise; the comment above is the only
      // documentation we need for the silent branch.
    } finally {
      dataCtl.loading = false;
    }
  }

  // Single load driver: an effect that keys off $auth + filters. When
  // auth resolves (or changes) it fires; when status/filterCtl.tagFilters change
  // it fires. We don't pair it with onMount(load) — that would cause
  // a double-fetch on initial paint and (more importantly) was the
  // source of the "stays dataCtl.loading" bug when an early call set
  // dataCtl.loading=true before $auth was ready.
  //
  // load() is wrapped in untrack() because the function reads
  // dataCtl.projects.length / dataCtl.goals.length / dataCtl.deadlines.length to decide whether
  // to refetch the linkable-entity sidecars, and it reassigns those
  // arrays when fresh data lands. Without untrack, those reads would
  // become deps of THIS effect, and Svelte 5 fires reactivity on
  // $state array reassignment even when contents are equal — turning
  // a single initial fetch into a tight loop (most visible when
  // /api/v1/deadlines returns []: dataCtl.deadlines.length stays 0, so every
  // load() refires load(), saturating the page). The explicit `void`
  // list above is the source-of-truth for what should retrigger load.
  $effect(() => {
    void $auth;
    void filterCtl.status;
    void filterCtl.tagFilters;
    void filterCtl.priorityFilter;
    void filterCtl.projectFilter;
    void filterCtl.goalFilter;
    void filterCtl.deadlineFilter;
    void filterCtl.archivedMode;
    untrack(() => load());
  });

  // URL-state effect — runs whenever a filter changes after hydration.
  // Skipped on the initial render so the URL doesn't get rewritten
  // before we read it back. syncToUrl reads $page.url.pathname and
  // calls goto(); both are reactive surfaces we don't want this effect
  // to depend on, so the call is untracked. The void list above is
  // the explicit dep set.
  $effect(() => {
    void filterCtl.status;
    void filterCtl.q;
    void filterCtl.tagFilters;
    void filterCtl.projectFilter;
    void filterCtl.priorityFilter;
    void filterCtl.goalFilter;
    void filterCtl.deadlineFilter;
    void viewCtl.view;
    void viewCtl.groupBy;
    void filterCtl.smartFilter;
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
  // The cursor is page-local; we only navigate within the current `filterCtl.filtered`
  // list. Discoverable via the '?' button in the header.
  // ---------------------------------------------------------------------------
  let cursorIdx = $state<number>(-1);
  $effect(() => {
    // Reset cursor when the filterCtl.filtered list shrinks past it. We read the
    // whole `filterCtl.filtered` array (not just .length) so any change to the
    // filter pipeline retriggers — a swap that keeps length identical
    // but rearranges items could otherwise leave the cursor pointing
    // at a stale row. The Math.max(0, …) keeps cursorIdx valid (>= 0)
    // even when filterCtl.filtered is empty; cursor-read sites also `?.` against
    // out-of-bounds so a flicker between the effect firing and the
    // render path resolves gracefully.
    void filterCtl.filtered;
    if (cursorIdx >= filterCtl.filtered.length) {
      cursorIdx = Math.max(0, filterCtl.filtered.length - 1);
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
    cursorIdx = Math.max(0, Math.min(filterCtl.filtered.length - 1, idx));
    // Scroll the focused row into view; the data-task-id attr on the
    // wrapper element gives us a stable selector across re-renders.
    const t = filterCtl.filtered[cursorIdx];
    if (!t) return;
    queueMicrotask(() => {
      const el = document.querySelector(`[data-task-id="${t.id}"]`) as HTMLElement | null;
      if (el) el.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    });
  }

  // VIEW_CYCLE + VIEW_DIGIT_MAP live in $lib/tasks/tasksHelpers — same
  // vocabulary shared with the future workspace shell so the chord
  // walks the same tab order whether dataCtl.tasks lives as a route or a pane.

  // Trigger the in-card snooze picker for the cursor task. The picker
  // is owned by TaskCard and anchored to its own snooze button (so the
  // popover positions correctly), so the cleanest cross-component
  // invocation is to .click() the button via the data-task-id wrapper.
  // Falls back silently if the row hasn't rendered yet — the keydown
  // handler's early-return already guarded against an empty filter.
  function openSnoozePickerForCursor() {
    const t = cursorIdx >= 0 ? filterCtl.filtered[cursorIdx] : null;
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
    if (filterCtl.filtered.length === 0) return;
    const allSelected = filterCtl.filtered.every((t) => selectedIds.has(t.id));
    if (allSelected) {
      selectedIds = new Set();
      toast.info('Selection cleared');
      return;
    }
    selectedIds = new Set(filterCtl.filtered.map((t) => t.id));
    toast.success(`Selected ${filterCtl.filtered.length} task${filterCtl.filtered.length === 1 ? '' : 's'}`);
  }

  // Page-scoped keyboard handler lives in useTasksKeyboard. The refs
  // object exposes the parent's controllers + action callbacks; the
  // handler reads through them so this file owns the j/k cursor and
  // selection state without inlining the dispatch tree.
  onMount(() =>
    installTasksKeyboard({
      getView: () => viewCtl.view,
      getFiltered: () => filterCtl.filtered,
      getCursorIdx: () => cursorIdx,
      getSelectionSize: () => selectedIds.size,
      isHelpOpen: () => viewCtl.helpOpen,
      setHelpOpen: (v) => (viewCtl.helpOpen = v),
      isFilterPanelOpen: () => viewCtl.filterPanelOpen,
      setFilterPanelOpen: (v) => (viewCtl.filterPanelOpen = v),
      cycleView: (dir) => viewCtl.cycleView(dir),
      setView: (v) => (viewCtl.view = v),
      setAgentOpen: (v) => (agentOpen = v),
      focusCursor,
      selectAllOrClear,
      toggleSelectedFor: (id) => {
        const next = new Set(selectedIds);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        selectedIds = next;
      },
      toggleDoneFor: (t) => {
        toggleDoneOf(t).catch(() => {});
      },
      openDetailFor: openDetail,
      cyclePriorityFor: (t) => {
        void cyclePriorityOf(t);
      },
      openSnoozeForCursor: openSnoozePickerForCursor,
      clearSelection: () => (selectedIds = new Set())
    })
  );

  // isSnoozed / isStale / isTaskLikePath live in $lib/tasks/tasksHelpers
  // — shared across the page, TaskCard, AIStaleVerdicts, and the
  // future workspace pane.
  //
  // The `filterCtl.filtered` derivation lives in $lib/tasks/tasksFilterState —
  // read it via filterCtl.filtered everywhere below.

  // Swipe-hint visibility. State (dismissed flag + touch probe) lives
  // near the top of the script with the other UI state.
  let showSwipeHint = $derived(
    isTouchDevice && !swipeHintDismissed && viewCtl.view === 'list' && filterCtl.filtered.length > 0
  );

  // viewCtl.weekColumns moved into $lib/tasks/tasksViewState — read via
  // viewCtl.weekColumns.

  // smartCounts moved into $lib/tasks/tasksFilterState — read via
  // filterCtl.smartCounts.

  // At-a-glance dataCtl.stats over the unfiltered open task list. Surfaced
  // as small chips above the list so the user always knows the
  // overall load — even when a filter is hiding most of it. Numbers
  // are debounced through $derived so they don't flicker mid-edit.
  // Subtask collapse state. Stored as a flat set of parent task IDs;
  // a task whose ANY ancestor is in this set is hidden from the
  // rendered list. Persisted to localStorage so collapse state
  // survives a refresh, but only IDs that still exist in the current
  // task list are kept (prevents the set from growing forever).
  // dataCtl.collapsedIds + dataCtl.parentMap + dataCtl.childCount + dataCtl.isHiddenByCollapse +
  // dataCtl.toggleCollapsed moved into dataCtl.

  // Saved filter presets — name a combination of status / q / tag /
  // project / priority / goal / deadline / view / viewCtl.groupBy, pin it
  // as a one-click chip above the dataCtl.stats row. Persisted to
  // localStorage. The CRUD + starter set live in
  // $lib/tasks/tasksPresets; this page reaches them via the snapshot
  // bridge so the controller stays decoupled from the page's let
  // bindings.
  const presetCtl = createPresetsController({
    getSnapshot: () => ({
      status: filterCtl.status,
      q: filterCtl.q,
      tagFilters: [...filterCtl.tagFilters],
      projectFilter: filterCtl.projectFilter,
      priorityFilter: filterCtl.priorityFilter,
      goalFilter: filterCtl.goalFilter,
      deadlineFilter: filterCtl.deadlineFilter,
      view: viewCtl.view,
      groupBy: viewCtl.groupBy,
      sortBy: viewCtl.sortBy,
      sourceFilter: filterCtl.sourceFilter,
      smartFilter: filterCtl.smartFilter,
      archivedMode: filterCtl.archivedMode
    }),
    applySnapshot: (s) => {
      filterCtl.status = s.status;
      filterCtl.q = s.q;
      filterCtl.tagFilters = [...s.tagFilters];
      filterCtl.projectFilter = s.projectFilter;
      filterCtl.priorityFilter = s.priorityFilter;
      filterCtl.goalFilter = s.goalFilter;
      filterCtl.deadlineFilter = s.deadlineFilter;
      viewCtl.view = s.view;
      viewCtl.groupBy = s.groupBy;
      viewCtl.sortBy = s.sortBy;
      filterCtl.sourceFilter = s.sourceFilter;
      filterCtl.smartFilter = s.smartFilter;
      filterCtl.archivedMode = s.archivedMode;
    }
  });

  // dataCtl.stats moved into dataCtl.

  // viewCtl.taskComparator / viewCtl.viewCounts / viewCtl.listGroups moved into
  // $lib/tasks/tasksViewState — read via viewCtl.X.

  // fmtEstBudget lives in $lib/tasks/tasksHelpers.

  // dataCtl.allTags + dataCtl.countOpen + dataCtl.countDone moved into dataCtl.

  // filterCtl.activeFilterCount / filterCtl.activeFilterChips / clearAll moved into
  // $lib/tasks/tasksFilterState — read via filterCtl.

  // viewCtl.selectView lives in viewCtl.selectView.

  // Adaptive subtitle for the "no matches" empty state. Mirrors the
  // active-filter set so the user gets a meaningful read instead of a
  // generic "nothing to see here". Order matches user intent: tag /
  // project / goal / search before generic fallback.
  let emptyStateSubtitle = $derived.by((): string => {
    if (filterCtl.tagFilters.length === 1) return `No tasks tagged #${filterCtl.tagFilters[0]}.`;
    if (filterCtl.tagFilters.length > 1) return `No tasks tagged ${filterCtl.tagFilters.map((t) => '#' + t).join(' + ')}.`;
    if (filterCtl.projectFilter) return `No tasks in project "${filterCtl.projectFilter}".`;
    if (filterCtl.goalFilter) {
      const g = dataCtl.goals.find((x) => x.id === filterCtl.goalFilter);
      return `No tasks linked to goal "${g?.title ?? filterCtl.goalFilter}".`;
    }
    if (filterCtl.priorityFilter !== '') return `No P${filterCtl.priorityFilter} tasks here.`;
    if (filterCtl.q.trim()) return `No tasks match "${filterCtl.q.trim()}".`;
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


<div class="flex h-full">
  <!-- Stream N — slide-out filter panel (right side, responsive). The
       previous always-on desktop sidebar is gone; advanced filtering
       is one click away from the header's Filter button. The drawer
       renders its content in the DOM at all times (just translated
       off-screen) so global `/` page-search can still focus the
       embedded search input. -->
  <Drawer bind:open={viewCtl.filterPanelOpen} side="right" responsive={true} width="w-80 sm:w-96">
    <TasksFilterDrawer
      {filterCtl}
      {dataCtl}
      onClose={() => (viewCtl.filterPanelOpen = false)}
    />
  </Drawer>

  <div class="flex-1 flex flex-col min-w-0">
    <!-- Stream N — single-row slim page header. Title + counts on the
         left, view-switcher segmented control + More-views dropdown
         + viewCtl.density + filter + capture + help on the right. Saves ~50%
         vertical space vs the previous two-row layout. -->
    <TasksPageHeader
      view={viewCtl.view}
      totalCount={dataCtl.tasks.length}
      filteredCount={filterCtl.filtered.length}
      activeFilterCount={filterCtl.activeFilterCount}
      density={viewCtl.density}
      todayLoad={dataCtl.stats.overdue + dataCtl.stats.todayCount}
      todayOverdue={dataCtl.stats.overdue}
      inboxLoad={viewCtl.viewCounts.inbox}
      moreViewsOpen={viewCtl.moreViewsOpen}
      activeOverflowLabel={viewCtl.activeOverflowLabel}
      onSelectView={viewCtl.selectView}
      onToggleMoreViews={() => (viewCtl.moreViewsOpen = !viewCtl.moreViewsOpen)}
      onPickOverflowView={viewCtl.pickOverflowView}
      onMoreViewsKey={viewCtl.onMoreViewsKey}
      onToggleDensity={() => (viewCtl.density = viewCtl.density === 'compact' ? 'normal' : 'compact')}
      onToggleFilterPanel={() => (viewCtl.filterPanelOpen = !viewCtl.filterPanelOpen)}
      onQuickCapture={openQuickCapture}
      onToggleHelp={() => (viewCtl.helpOpen = !viewCtl.helpOpen)}
    />

    <!-- Stream N — quick-filter chip row, always visible. 6 chips
         (All · Today · Overdue · P1 · No date · Done) — the
         single-click smart filters that let the user re-shape the
         list without opening the filter panel. Horizontal scroll on
         mobile (no wrap) so the row stays one line. -->
    <QuickFilterChips
      smartFilter={filterCtl.smartFilter}
      status={filterCtl.status}
      counts={{
        overdue: filterCtl.smartCounts.overdue,
        today: filterCtl.smartCounts.today,
        noDue: filterCtl.smartCounts.noDue,
        highPriority: filterCtl.smartCounts.highPriority
      }}
      doneCount={dataCtl.countDone}
      activeFilterCount={filterCtl.activeFilterCount}
      onSetSmart={(s) => (filterCtl.smartFilter = s)}
      onSetStatus={(s) => (filterCtl.status = s)}
      onClearAll={filterCtl.clearAll}
    />

    {#if viewCtl.view === 'list' || viewCtl.view === 'kanban' || viewCtl.view === 'today'}
      <!-- AI Plan-my-day. Different agent from triage/
           deadline-detect: those operate on UNTRIAGED dataCtl.tasks;
           this one looks across ALL open dataCtl.tasks and produces a
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
                <button onclick={() => void focusPlan.acceptAll(dataCtl.tasks, load)} class="text-[11px] text-success hover:underline" title="Pin every remaining plan item back-to-back starting now">accept all</button>
              {/if}
              <button onclick={() => void focusPlan.run(dataCtl.tasks, aiFocusHours)} class="text-[11px] text-secondary hover:underline">↻ regenerate</button>
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
                {@const t = dataCtl.tasks.find((x) => x.id === p.taskId)}
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
                      onclick={() => void focusPlan.acceptItem(p, dataCtl.tasks, load)}
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
            <p class="text-[10px] text-dim mt-2">Context: {dataCtl.tasks.filter((t) => !t.done).slice(0, 30).length} open dataCtl.tasks shown · {aiFocusHours}h focus budget</p>
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
      <AskTasks bind:open={askTasksOpen} filtered={filterCtl.filtered} />

      <!-- Quick-add bar. Type a single-line task in granit's
           parser-friendly syntax; Enter creates it in today's daily
           note. Single most-impactful "more powerful dataCtl.tasks" change:
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
          onclick={() => void focusPlan.run(dataCtl.tasks, aiFocusHours)}
          disabled={$focusPlan.busy || dataCtl.tasks.filter((t) => !t.done).length === 0}
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
          disabled={filterCtl.filtered.length === 0}
          title="Ask AI a question about your current task view"
          class="inline-flex px-3 py-2 text-sm bg-surface1 border border-surface2 text-primary rounded hover:border-primary disabled:opacity-50 flex-shrink-0 items-center gap-1.5"
        >
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="9"/>
            <path d="M9.5 9a2.5 2.5 0 0 1 5 0c0 1.5-2.5 2-2.5 3.5"/>
            <path d="M12 17h0"/>
          </svg>
          <span>Ask dataCtl.tasks</span>
        </button>
      </div>
      <!-- Saved filter presets. One-click application of a stored
           filter combo. The "+ save" chip captures the current
           filter state under a name; clicking a preset chip
           re-applies all stored fields. Long-press / right-click to
           delete via the small × on the active chip. -->
      {#if presetCtl.visiblePresets.length > 0 || true}
        <div class="px-3 py-1.5 border-b border-surface1 flex items-center gap-1.5 text-xs flex-shrink-0 flex-wrap">
          <span class="text-dim font-mono uppercase tracking-wider">presets</span>
          {#if presetCtl.isShowingStarters}
            <span class="text-[10px] text-dim italic font-mono" title="Built-in starter presets — save your own and these go away">starter</span>
          {/if}
          {#each presetCtl.visiblePresets as p (p.name)}
            {@const active = presetCtl.matches(p)}
            {@const isStarter = presetCtl.isShowingStarters}
            <span
              class="inline-flex items-center rounded overflow-hidden border
                {active ? 'border-primary bg-surface1 text-primary' : 'border-surface1 bg-surface0 text-subtext hover:border-primary'}"
            >
              <button
                onclick={() => presetCtl.apply(p)}
                class="px-2 py-0.5"
              >{p.name}</button>
              {#if active && !isStarter}
                <button
                  onclick={() => presetCtl.remove(p.name)}
                  title="Remove preset"
                  class="px-1.5 py-0.5 text-dim hover:text-error border-l border-surface1"
                >×</button>
              {/if}
            </span>
          {/each}
          <button
            onclick={() => presetCtl.capture()}
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
      {#if filterCtl.activeFilterChips.length > 0}
        <div class="px-3 py-1.5 border-b border-surface1 flex items-center gap-1 text-[11px] flex-shrink-0 flex-wrap bg-surface0/40">
          <span class="text-[10px] uppercase tracking-wider text-dim mr-1 select-none">Filters</span>
          {#each filterCtl.activeFilterChips as chip (chip.key)}
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
          {#if filterCtl.activeFilterChips.length >= 2}
            <button
              type="button"
              onclick={filterCtl.clearAll}
              title="Reset every active filter to its default"
              class="ml-1 px-1.5 py-0.5 text-[10px] uppercase tracking-wider text-warning hover:text-error border border-dashed border-warning hover:border-error"
            >clear all</button>
          {/if}
          <span class="flex-1"></span>
          <span class="text-[10px] text-dim font-mono tabular-nums select-none">{filterCtl.filtered.length} match{filterCtl.filtered.length === 1 ? '' : 'es'}</span>
        </div>
      {/if}
      <!-- Stream N — slim contextual sub-toolbar. Only shown for list
           and kanban views. The visual noise of the previous always-
           on filterCtl.smartCounts row is gone; key counts (done today / week /
           estimate budget / avg priority) live in the slide-out
           filter panel as informational chips. Group/sort/columns
           selectors stay here because they reshape the visible list
           and the user reaches for them frequently. -->
      {#if viewCtl.view === 'list' || viewCtl.view === 'kanban'}
        <div class="px-3 py-1.5 border-b border-surface1 flex items-center gap-2 text-xs flex-shrink-0 flex-wrap bg-mantle">
          {#if viewCtl.view === 'list'}
            <span class="text-dim font-mono uppercase tracking-wider select-none">group</span>
            <select
              bind:value={viewCtl.groupBy}
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
              bind:value={viewCtl.sortBy}
              title="How to order dataCtl.tasks inside each group"
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
            <select bind:value={viewCtl.kanbanMode} class="bg-surface0 border border-surface1 rounded px-2 py-0.5 text-text">
              <option value="priority">priority</option>
              <option value="due">due</option>
              <option value="triage">triage (granit)</option>
              <option value="config">config</option>
            </select>
          {/if}
          <span class="flex-1"></span>
          <!-- Tiny passive dataCtl.stats — done today / done 7d / est budget.
               Live next to the group/sort selectors so the user has
               a one-line at-a-glance signal without the previous
               14-chip stat row. Other dataCtl.stats (noEstCount, avgPriority,
               snoozed) moved to the filter panel. -->
          {#if dataCtl.stats.doneToday > 0}
            <span class="text-success font-mono tabular-nums select-none" title="Completed today">✓ {dataCtl.stats.doneToday}</span>
          {/if}
          {#if dataCtl.stats.doneWeek > 0}
            <span class="text-success/80 font-mono tabular-nums select-none" title="Completed in the last 7 days">7d ✓ {dataCtl.stats.doneWeek}</span>
          {/if}
          {#if dataCtl.stats.sumEstMin > 0}
            <span class="text-secondary font-mono tabular-nums select-none" title="Total estimated minutes across open non-snoozed dataCtl.tasks. 8h = one day-block.">Σ {fmtEstBudget(dataCtl.stats.sumEstMin)}</span>
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
      {#if dataCtl.loading && dataCtl.tasks.length === 0}
        <div class="text-sm text-dim">dataCtl.loading…</div>
      {:else if filterCtl.filtered.length === 0 && viewCtl.view === 'today'}
        <!-- Today view inbox-zero message. Different from a true empty
             state — the user has dataCtl.tasks, just none for today. The
             tone is calm-celebratory rather than the cobwebbed
             "get to work" used by the Review view. -->
        <div class="max-w-md mx-auto py-6 text-center">
          <div class="text-4xl mb-3 opacity-50">🌤</div>
          <h2 class="text-base font-medium text-text mb-1">Today is clear</h2>
          <p class="text-sm text-dim">
            Nothing overdue, nothing due today, nothing scheduled. Take the open space — or pick something from
            <button class="text-primary hover:underline" onclick={() => (viewCtl.view = 'list')}>the full list</button>.
          </p>
        </div>
      {:else if filterCtl.filtered.length === 0 && viewCtl.view === 'review'}
        <div class="bg-surface0 border border-surface1 rounded-lg p-5 text-center">
          <p class="text-sm text-text mb-1">No dataCtl.tasks completed in the last 7 days.</p>
          <p class="text-xs text-dim mb-3">The review tab shows what you've finished — once a few dataCtl.tasks roll through, this is where you'll spot patterns.</p>
          <button
            type="button"
            onclick={() => (viewCtl.view = 'list')}
            class="text-xs px-3 py-1.5 bg-primary text-on-primary rounded font-medium hover:opacity-90"
          >Open task list →</button>
        </div>
      {:else if filterCtl.filtered.length === 0 && viewCtl.view === 'inbox'}
        <div class="bg-surface0 border border-surface1 rounded-lg p-5 text-center">
          <p class="text-sm text-success mb-1">Inbox empty.</p>
          <p class="text-xs text-dim mb-3">Nothing waiting to be triaged. Captured dataCtl.tasks land here for sorting before they hit the main list.</p>
          <button
            type="button"
            onclick={() => (viewCtl.view = 'list')}
            class="text-xs px-3 py-1.5 bg-surface1 border border-surface2 text-text rounded font-medium hover:border-primary"
          >Open task list →</button>
        </div>
      {:else if filterCtl.filtered.length === 0 && viewCtl.view === 'stale'}
        <div class="bg-surface0 border border-surface1 rounded-lg p-5 text-center">
          <p class="text-sm text-success mb-1">No stale dataCtl.tasks.</p>
          <p class="text-xs text-dim">Everything's been touched in the last week — nothing rotting in the backlog.</p>
        </div>
      {:else if filterCtl.filtered.length === 0 && viewCtl.view === 'quickwins'}
        <p class="text-sm text-dim italic">No quick wins available. Add an estimate (e.g. <code class="text-secondary">est:30m</code>) to high-priority dataCtl.tasks.</p>
      {:else if filterCtl.filtered.length === 0 && dataCtl.tasks.length === 0}
        <!-- True empty: no dataCtl.tasks anywhere. Onboarding-style hint
             pointing at the quick-add bar. -->
        <div class="max-w-md mx-auto py-6 text-center">
          <div class="text-5xl mb-3 opacity-30">✓</div>
          <h2 class="text-lg font-semibold text-text mb-2">No dataCtl.tasks yet</h2>
          <p class="text-sm text-dim mb-1">
            Type your first task in the bar above. Examples:
          </p>
          <ul class="text-sm text-subtext font-mono mt-3 space-y-1.5 inline-block text-left">
            <li>fix login bug <span class="text-error">!1</span> <span class="text-secondary">due:tomorrow</span></li>
            <li>buy groceries <span class="text-info">#errands</span></li>
            <li>review PR <span class="text-warning">!2</span> <span class="text-secondary">due:fri</span></li>
          </ul>
        </div>
      {:else if filterCtl.filtered.length === 0}
        <!-- Tasks exist but the active filter masks them all. The
             subtitle adapts to which filter is the dominant signal
             (tag / project / goal / priority / search) so the user
             reads "No dataCtl.tasks tagged #X" instead of a generic "no
             matches". Two CTAs: Quick capture for fast entry and
             Clear filters for the reset path. min-w-0 keeps the
             card from overrunning the sidebar on narrow viewports. -->
        <div class="min-w-0">
          <div class="max-w-md mx-auto py-6 text-center">
            <div class="text-4xl mb-3 opacity-30">🔍</div>
            <h2 class="text-base font-medium text-text mb-2">No dataCtl.tasks here</h2>
            <p class="text-sm text-dim mb-1">{emptyStateSubtitle}</p>
            <p class="text-xs text-dim mb-4">
              {dataCtl.tasks.length} {dataCtl.tasks.length === 1 ? 'task is' : 'tasks are'} hidden by the current filters.
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
                onclick={filterCtl.clearAll}
                class="px-3 py-1.5 bg-surface0 border border-surface1 hover:border-primary rounded text-sm text-subtext"
              >Clear filters</button>
            </div>
          </div>
        </div>
      {:else if viewCtl.view === 'week'}
        <!-- Week view — 8 columns: Unscheduled + 7 rolling days from
             today. Overdue dataCtl.tasks bubble up as a striped strip pinned
             above today's column so the user doesn't have to hunt
             them across past dates. Each column header is clickable:
             pressing one drops the user into List view filterCtl.filtered to
             that day so they can drill in. -->
        <div class="flex flex-col gap-2">
          {#if viewCtl.weekColumns.overdue.length > 0}
            <div class="bg-surface0 border border-error rounded p-2">
              <div class="flex items-baseline gap-2 mb-1.5">
                <h3 class="text-xs uppercase tracking-wider text-error font-medium">overdue</h3>
                <span class="text-[10px] font-mono text-dim">{viewCtl.weekColumns.overdue.length}</span>
                <button
                  type="button"
                  onclick={() => { filterCtl.smartFilter = 'overdue'; view = 'list'; }}
                  class="ml-auto text-[10px] text-error hover:underline font-mono"
                >open in list →</button>
              </div>
              <div class="space-y-1">
                {#each viewCtl.weekColumns.overdue.slice(0, 5) as t (t.id)}
                  <TaskCard task={t} compact={viewCtl.compactCards} hasChildren={(dataCtl.childCount.get(t.id) ?? 0) > 0} childCount={dataCtl.childCount.get(t.id) ?? 0} collapsed={dataCtl.collapsedIds.has(t.id)} onToggleCollapse={() => dataCtl.toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
                {/each}
                {#if viewCtl.weekColumns.overdue.length > 5}
                  <p class="text-[11px] text-dim italic px-1">…{viewCtl.weekColumns.overdue.length - 5} more</p>
                {/if}
              </div>
            </div>
          {/if}
          <div class="grid grid-cols-[minmax(10rem,1fr)_repeat(7,minmax(0,1fr))] gap-2 min-h-[20rem]">
            <!-- Unscheduled column — capture surface for dataCtl.tasks with
                 no date. The "+ add" button at the bottom kicks off a
                 quick-add that lands without a date so the user can
                 then drag (or click) it into a day column. -->
            <div class="bg-surface0 border border-surface1 rounded p-2 flex flex-col min-h-0">
              <div class="flex items-baseline gap-2 mb-1.5 sticky top-0 bg-surface0 pb-1 border-b border-surface1">
                <h3 class="text-xs uppercase tracking-wider text-dim font-medium">unscheduled</h3>
                <span class="text-[10px] font-mono text-dim">{viewCtl.weekColumns.unscheduled.length}</span>
              </div>
              <div class="flex-1 overflow-y-auto space-y-1">
                {#each viewCtl.weekColumns.unscheduled.slice(0, 50) as t (t.id)}
                  <TaskCard task={t} compact hasChildren={(dataCtl.childCount.get(t.id) ?? 0) > 0} childCount={dataCtl.childCount.get(t.id) ?? 0} collapsed={dataCtl.collapsedIds.has(t.id)} onToggleCollapse={() => dataCtl.toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
                {/each}
                {#if viewCtl.weekColumns.unscheduled.length > 50}
                  <p class="text-[11px] text-dim italic px-1">…{viewCtl.weekColumns.unscheduled.length - 50} more</p>
                {/if}
                {#if viewCtl.weekColumns.unscheduled.length === 0}
                  <p class="text-[11px] text-dim italic px-1">nothing untaken — good shape.</p>
                {/if}
              </div>
            </div>
            <!-- Seven day columns. The today column gets a primary
                 border so the user's eye lands on it first. -->
            {#each viewCtl.weekColumns.days as col (col.date)}
              <div class="bg-surface0 border {col.isToday ? 'border-primary' : 'border-surface1'} rounded p-2 flex flex-col min-h-0">
                <div class="flex items-baseline gap-1.5 mb-1.5 sticky top-0 bg-surface0 pb-1 border-b border-surface1">
                  <button
                    type="button"
                    onclick={() => { view = 'list'; filterCtl.q = ''; filterCtl.smartFilter = col.isToday ? 'today' : (col.date === viewCtl.weekColumns.days[1]?.date ? 'tomorrow' : ''); }}
                    class="text-xs uppercase tracking-wider {col.isToday ? 'text-primary' : 'text-text'} font-medium hover:underline"
                    title="open this day in the list view"
                  >{col.label}</button>
                  <span class="text-[10px] text-dim font-mono">{col.sublabel}</span>
                  <span class="ml-auto text-[10px] font-mono text-dim">{col.tasks.length}</span>
                </div>
                <div class="flex-1 overflow-y-auto space-y-1">
                  {#each col.tasks as t (t.id)}
                    <TaskCard task={t} compact hasChildren={(dataCtl.childCount.get(t.id) ?? 0) > 0} childCount={dataCtl.childCount.get(t.id) ?? 0} collapsed={dataCtl.collapsedIds.has(t.id)} onToggleCollapse={() => dataCtl.toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
                  {/each}
                  {#if col.tasks.length === 0}
                    <p class="text-[11px] text-dim italic px-1">—</p>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        </div>
      {:else if viewCtl.view === 'kanban'}
        <Kanban
          tasks={filterCtl.filtered}
          bind:mode={viewCtl.kanbanMode}
          bind:swimlane={viewCtl.kanbanSwimlane}
          bind:selectedIds
          onChanged={load}
          onOpenDetail={openDetail}
          onContextMenu={openContext}
        />
      {:else if viewCtl.view === 'eisenhower'}
        <EisenhowerView
          tasks={filterCtl.filtered}
          onOpenDetail={openDetail}
          onContextMenu={openContext}
          onChanged={load}
        />
      {:else if viewCtl.view === 'triage'}
        <TriageBoard tasks={filterCtl.filtered} onChanged={load} />
      {:else if viewCtl.view === 'inbox'}
        <div class="max-w-3xl">
          <div class="flex items-baseline gap-3 mb-4">
            <p class="text-sm text-dim flex-1">
              Untriaged dataCtl.tasks. Decide for each: schedule, prioritize, drop, or snooze.
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
                disabled={filterCtl.filtered.length === 0}
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
                title="Scan all open dataCtl.tasks without a due date — propose ones whose title implies a clear deadline"
              >✨ Detect dataCtl.deadlines</button>
            {/if}
          </div>

          {#if $deadline.proposals.length > 0}
            <!-- Deadline proposals — operates across ALL open dataCtl.tasks
                 without a due_date, not just inbox. Server already
                 filterCtl.filtered out blanks, so every row is a confident
                 suggestion. Apply patches dueDate; skip just dismisses. -->
            <div class="mb-5 p-3 bg-surface0 border border-warning rounded">
              <div class="flex items-center mb-2">
                <div class="text-xs uppercase tracking-wider text-warning font-semibold flex-1">Detected dataCtl.deadlines ({$deadline.proposals.length})</div>
                <button
                  onclick={() => deadline.discard()}
                  class="text-[10px] text-dim hover:text-error"
                  title="Drop all proposals without applying any"
                >discard</button>
              </div>
              <ul class="space-y-2">
                {#each $deadline.proposals as p (p.id)}
                  {@const t = dataCtl.tasks.find((x) => x.id === p.id)}
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
                  {@const t = dataCtl.tasks.find((x) => x.id === p.id)}
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
            {#each filterCtl.filtered.filter((tt) => !dataCtl.isHiddenByCollapse(tt.id, dataCtl.collapsedIds)) as t (t.id)}
              <div data-task-id={t.id} class={cursorIdx >= 0 && filterCtl.filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                <TaskCard task={t} compact={viewCtl.compactCards} hasChildren={(dataCtl.childCount.get(t.id) ?? 0) > 0} childCount={dataCtl.childCount.get(t.id) ?? 0} collapsed={dataCtl.collapsedIds.has(t.id)} onToggleCollapse={() => dataCtl.toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
              </div>
            {/each}
          </div>
        </div>
      {:else if viewCtl.view === 'stale'}
        <div class="max-w-3xl">
          <AIStaleVerdicts
            candidates={filterCtl.filtered.filter(isStale)}
            allTasks={dataCtl.tasks}
            onReload={load}
          />

          <div class="space-y-2">
            {#each filterCtl.filtered.filter((tt) => !dataCtl.isHiddenByCollapse(tt.id, dataCtl.collapsedIds)) as t (t.id)}
              <div data-task-id={t.id} class={cursorIdx >= 0 && filterCtl.filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                <TaskCard task={t} compact={viewCtl.compactCards} hasChildren={(dataCtl.childCount.get(t.id) ?? 0) > 0} childCount={dataCtl.childCount.get(t.id) ?? 0} collapsed={dataCtl.collapsedIds.has(t.id)} onToggleCollapse={() => dataCtl.toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
              </div>
            {/each}
          </div>
        </div>
      {:else if viewCtl.view === 'duplicates'}
        <div class="max-w-3xl">
          <TaskDuplicates onReload={load} />
        </div>
      {:else if viewCtl.view === 'quickwins'}
        <div class="max-w-3xl">
          <p class="text-sm text-dim mb-4">High-priority dataCtl.tasks you can finish in ≤30 min. Pick one, knock it out.</p>
          <div class="space-y-2">
            {#each filterCtl.filtered.filter((tt) => !dataCtl.isHiddenByCollapse(tt.id, dataCtl.collapsedIds)) as t (t.id)}
              <div data-task-id={t.id} class={cursorIdx >= 0 && filterCtl.filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                <TaskCard task={t} compact={viewCtl.compactCards} hasChildren={(dataCtl.childCount.get(t.id) ?? 0) > 0} childCount={dataCtl.childCount.get(t.id) ?? 0} collapsed={dataCtl.collapsedIds.has(t.id)} onToggleCollapse={() => dataCtl.toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
              </div>
            {/each}
          </div>
        </div>
      {:else if viewCtl.view === 'review'}
        <div class="max-w-3xl">
          <p class="text-sm text-dim mb-4">Done in the last week — your retrospective view.</p>
          <div class="space-y-2 opacity-80">
            {#each filterCtl.filtered.filter((tt) => !dataCtl.isHiddenByCollapse(tt.id, dataCtl.collapsedIds)) as t (t.id)}
              <div data-task-id={t.id} class={cursorIdx >= 0 && filterCtl.filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                <TaskCard task={t} compact={viewCtl.compactCards} hasChildren={(dataCtl.childCount.get(t.id) ?? 0) > 0} childCount={dataCtl.childCount.get(t.id) ?? 0} collapsed={dataCtl.collapsedIds.has(t.id)} onToggleCollapse={() => dataCtl.toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
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
          groups={viewCtl.listGroups}
          filtered={filterCtl.filtered}
          cursorIdx={cursorIdx}
          compactCards={viewCtl.compactCards}
          childCount={dataCtl.childCount}
          collapsedIds={dataCtl.collapsedIds}
          collapsedSections={viewCtl.collapsedSections}
          groupAddKey={groupAddCtl.key}
          bind:groupAddText={groupAddCtl.text}
          groupAddBusy={groupAddCtl.busy}
          bind:selectedIds
          isHiddenByCollapse={dataCtl.isHiddenByCollapse}
          onToggleSection={viewCtl.toggleSection}
          onToggleCollapse={dataCtl.toggleCollapsed}
          onChanged={load}
          onOpenDetail={openDetail}
          onContextMenu={openContext}
          onStartGroupAdd={(key) => (groupAddCtl.key === key ? groupAddCtl.cancel() : groupAddCtl.open(key))}
          onCancelGroupAdd={groupAddCtl.cancel}
          onSubmitGroupAdd={groupAddCtl.submit}
          onGroupAddTextChange={(v) => (groupAddCtl.text = v)}
        />
      {/if}
    </div>
  </div>
</div>

<TaskDetail bind:open={detailOpen} task={detailTask} onChanged={async () => {
  await load();
  // Refresh the in-drawer task copy from the freshly-loaded list so subsequent
  // edits see latest state.
  if (detailTask) detailTask = dataCtl.tasks.find((t) => t.id === detailTask!.id) ?? detailTask;
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
{#if viewCtl.helpOpen}
  <div
    class="fixed inset-0 bg-mantle z-50 flex items-center justify-center p-4"
    onclick={() => (viewCtl.helpOpen = false)}
    role="presentation"
  >
    <!-- max-h with dvh keeps the dialog from bleeding behind mobile
         browser chrome / keyboards; overflow-y-auto lets the user
         scroll the shortcut list when the keyboard takes half the
         screen. -->
    <div
      class="bg-surface0 border border-surface1 rounded-lg p-5 max-w-md w-full max-h-[90dvh] overflow-y-auto shadow-xl"
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => { if (e.key === 'Escape') viewCtl.helpOpen = false; }}
      role="dialog"
      aria-modal="true"
      aria-label="Keyboard shortcuts"
      tabindex="-1"
    >
      <div class="flex items-center justify-between mb-3">
        <h2 class="text-base font-semibold text-text">Keyboard shortcuts</h2>
        <button onclick={() => (viewCtl.helpOpen = false)} class="text-dim hover:text-text">esc</button>
      </div>
      <div class="grid grid-cols-2 gap-y-2 gap-x-4 text-sm">
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">j / k</kbd>
        <span class="text-subtext">navigate up / down</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">x</kbd>
        <span class="text-subtext">toggle bulk-select</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">Shift+A</kbd>
        <span class="text-subtext">select / clear all filterCtl.filtered</span>
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
        <span class="text-subtext">open AI agent (operates on filterCtl.filtered list or bulk-selection)</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">[ / ]</kbd>
        <span class="text-subtext">previous / next view mode</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">1‥4</kbd>
        <span class="text-subtext">jump to today / list / kanban / matrix</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">?</kbd>
        <span class="text-subtext">toggle this overlay</span>
      </div>
      <div class="mt-4 pt-3 border-t border-surface1 text-xs text-dim">
        <strong class="text-subtext">Kanban:</strong> drag cards between columns. Drag while a
        bulk-selection is active to move all selected dataCtl.tasks at once.
      </div>
    </div>
  </div>
{/if}

<!-- When the user has bulk-selected dataCtl.tasks, narrow the agent's
     scope to that selection — the explicit selection IS the
     intent. Otherwise fall back to the page's filterCtl.filtered list so
     "agent over what I'm looking at" is the default. -->
<TaskAgent
  open={agentOpen}
  tasks={selectedIds.size > 0 ? filterCtl.filtered.filter((t) => selectedIds.has(t.id)) : filterCtl.filtered}
  todayISO={todayISO()}
  availableProjects={dataCtl.projects.map((p) => p.name)}
  onClose={() => (agentOpen = false)}
  onChanged={load}
/>
