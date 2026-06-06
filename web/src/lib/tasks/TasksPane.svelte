<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/stores';
  import { auth } from '$lib/stores/auth';
  import { todayISO, type Task } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { installTasksLifecycle } from '$lib/tasks/tasksLifecycle';
  import Kanban from '$lib/tasks/Kanban.svelte';
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
  import TasksInboxView from '$lib/tasks/TasksInboxView.svelte';
  import TasksWeekView from '$lib/tasks/TasksWeekView.svelte';
  import TasksViewToolbar from '$lib/tasks/TasksViewToolbar.svelte';
  import TasksPlanMyDay from '$lib/tasks/TasksPlanMyDay.svelte';
  import TasksShortcutsOverlay from '$lib/tasks/TasksShortcutsOverlay.svelte';
  import TasksCardList from '$lib/tasks/TasksCardList.svelte';
  import TasksQuickAddBar from '$lib/tasks/TasksQuickAddBar.svelte';
  import TasksSwipeHint from '$lib/tasks/TasksSwipeHint.svelte';
  import TasksEmptyStates from '$lib/tasks/TasksEmptyStates.svelte';
  import TasksPresetsBar from '$lib/tasks/TasksPresetsBar.svelte';
  import TasksActiveFilterChips from '$lib/tasks/TasksActiveFilterChips.svelte';
  import { installTasksKeyboardForCtls } from '$lib/tasks/useTasksKeyboard';
  import { createTasksUrlSync } from '$lib/tasks/tasksUrlSync';
  import { createTasksGroupAdd } from '$lib/tasks/tasksGroupAdd.svelte';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';
  import { applyNextPriority, toggleDoneOf } from '$lib/tasks/taskActions';
  import {
    createTriageStore,
    createDeadlineStore,
    createFocusPlanStore
  } from '$lib/tasks/aiAgentStore';
  import { isStale } from '$lib/tasks/tasksHelpers';
  import { createPresetsControllerForCtls } from '$lib/tasks/tasksPresets.svelte';
  import { createTasksFilterState } from '$lib/tasks/tasksFilterState.svelte';
  import { createTasksViewState } from '$lib/tasks/tasksViewState.svelte';
  import { createTasksData } from '$lib/tasks/tasksData.svelte';
  import { createTasksSelection } from '$lib/tasks/tasksSelection.svelte';
  import { createTasksDetail } from '$lib/tasks/tasksDetail.svelte';
  import { createTasksLoader } from '$lib/tasks/tasksLoader.svelte';

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

  // Quick-add bar (input + Plan-day + Ask-tasks triggers) lives in
  // $lib/tasks/TasksQuickAddBar — owns its own input state + parse
  // pipeline + busy flag. The parent still owns aiFocusHours
  // (passed bindable) so TasksPlanMyDay above the bar reads the same
  // value without an extra round-trip.

  // Cursor (j/k navigation) + bulk-selection state lives in
  // $lib/tasks/tasksSelection. The controller owns cursorIdx +
  // selectedIds + focusCursor + selectAllOrClear +
  // openSnoozePickerForCursor; bindings into children pass through
  // its getter/setter pairs the same way viewCtl bindings do.
  const selCtl = createTasksSelection({ getFiltered: () => filterCtl.filtered });
  // Detail drawer + context menu state lives in
  // $lib/tasks/tasksDetail. openDetail also publishes to the
  // workspace context bus for adjacent AI panes.
  const detCtl = createTasksDetail();
  const openDetail = detCtl.openDetail;
  const openContext = detCtl.openContext;

  // Swipe-hint banner + dismissal lives in $lib/tasks/TasksSwipeHint —
  // owns its own localStorage flag + touch-device probe + 8-second
  // auto-dismiss timer. The parent decides `applicable` (list view
  // with at least one card visible) at the render site.

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

  // Data loader — owns the fetch + the filter-driven $effect that
  // re-fires it. See tasksLoader.svelte.ts for the why on the
  // untrack() / void-list pattern.
  const loader = createTasksLoader({
    getAuth: () => $auth,
    filterCtl,
    dataCtl
  });
  const load = loader.load;

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

  // WS coalesce + visibility-aware refresh — set up via
  // installTasksLifecycle so the parent's onMount stays a one-liner.
  // See tasksLifecycle.ts for the why on the 600ms window.
  onMount(() => installTasksLifecycle({ load }));

  // Keyboard shortcut wiring + cursor + selection state lives in
  // selCtl (see tasksSelection.svelte.ts); the install call below
  // bridges selCtl into useTasksKeyboard's refs object.

  async function cyclePriorityOf(t: Task) {
    try {
      await applyNextPriority(t);
    } catch {}
  }

  // VIEW_CYCLE + VIEW_DIGIT_MAP live in $lib/tasks/tasksHelpers — same
  // vocabulary shared with the future workspace shell so the chord
  // walks the same tab order whether tasks lives as a route or a pane.

  // Page-scoped keyboard handler — the convenience factory wires the
  // controller trio directly; only the page-local actions go through
  // explicit callbacks.
  onMount(() =>
    installTasksKeyboardForCtls({
      filterCtl,
      viewCtl,
      selCtl,
      setAgentOpen: (v) => (agentOpen = v),
      toggleDoneFor: (t) => { toggleDoneOf(t).catch(() => {}); },
      openDetailFor: openDetail,
      cyclePriorityFor: (t) => { void cyclePriorityOf(t); }
    })
  );

  // isSnoozed / isStale / isTaskLikePath live in $lib/tasks/tasksHelpers
  // — shared across the page, TaskCard, AIStaleVerdicts, and the
  // future workspace pane.
  //
  // The `filterCtl.filtered` derivation lives in $lib/tasks/tasksFilterState —
  // read it via filterCtl.filtered everywhere below.

  // Swipe-hint applicability — the parent's local share. The
  // component owns the dismissed/touch-device flags + visibility
  // derive; this is the "is the surrounding view one where swipe
  // gestures even apply?" gate.
  let swipeHintApplicable = $derived(
    viewCtl.view === 'list' && filterCtl.filtered.length > 0
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
  // as a one-click chip above the stats row. Persisted to
  // localStorage. The CRUD + starter set + the filterCtl/viewCtl
  // snapshot bridge live in $lib/tasks/tasksPresets.
  const presetCtl = createPresetsControllerForCtls(filterCtl, viewCtl);

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
      <!-- AI Plan-my-day — sequenced 3-7-task plan, accept/skip per row.
           Self-hides when there's no plan state worth showing. -->
      <TasksPlanMyDay {focusPlan} {dataCtl} {aiFocusHours} {load} />

      <!-- Ask Tasks — chat-style Q&A across the currently-visible
           task set. Streams a markdown answer the user can read,
           copy, or dismiss. No mutations; pure analysis. The trigger
           sits in the quick-add bar below; this component handles
           everything once `open` flips true. -->
      <AskTasks bind:open={askTasksOpen} filtered={filterCtl.filtered} />

      <!-- Quick-add bar. Type a single-line task in granit's
           parser-friendly syntax; Enter creates it in today's daily
           note. Single most-impactful "more powerful tasks" change:
           creating a task no longer requires opening a note. -->
      <TasksQuickAddBar
        {focusPlan}
        {dataCtl}
        filteredCount={filterCtl.filtered.length}
        bind:aiFocusHours
        onAdded={load}
        onStartAsk={startAskTasks}
      />
      <!-- Saved filter presets. One-click application of a stored
           filter combo. The "+ save" chip captures the current
           filter state under a name; clicking a preset chip
           re-applies all stored fields. Long-press / right-click to
           delete via the small × on the active chip. -->
      <TasksPresetsBar {presetCtl} />
      <!-- Active-filter chip row. Hidden when no filters are active.
           "Clear all" pill appears once 2+ filters are active. -->
      <TasksActiveFilterChips
        chips={filterCtl.activeFilterChips}
        filteredCount={filterCtl.filtered.length}
        onClearAll={filterCtl.clearAll}
      />
      <!-- Stream N — slim contextual sub-toolbar. Only shown for list
           and kanban views. The visual noise of the previous always-
           on filterCtl.smartCounts row is gone; key counts (done today / week /
           estimate budget / avg priority) live in the slide-out
           filter panel as informational chips. Group/sort/columns
           selectors stay here because they reshape the visible list
           and the user reaches for them frequently. -->
      {#if viewCtl.view === 'list' || viewCtl.view === 'kanban'}
        <TasksViewToolbar {viewCtl} {dataCtl} />
      {/if}
    {/if}

    {#if selCtl.selectedIds.size > 0}
      <BulkBar
        count={selCtl.selectedIds.size}
        ids={Array.from(selCtl.selectedIds)}
        onClear={selCtl.clear}
        onChanged={async () => { selCtl.clear(); await load(); }}
      />
    {/if}

    <div class="flex-1 overflow-auto p-2 sm:p-3">
      {#if dataCtl.loading && dataCtl.tasks.length === 0}
        <div class="text-sm text-dim">loading…</div>
      {:else if filterCtl.filtered.length === 0}
        <TasksEmptyStates
          view={viewCtl.view}
          totalTasks={dataCtl.tasks.length}
          {emptyStateSubtitle}
          onSwitchView={(v) => (viewCtl.view = v)}
          onClearAll={filterCtl.clearAll}
          onQuickCapture={openQuickCapture}
        />
      {:else if viewCtl.view === 'week'}
        <TasksWeekView
          {filterCtl}
          {dataCtl}
          {viewCtl}
          bind:selectedIds={selCtl.selectedIds}
          {load}
          onOpenDetail={openDetail}
          onOpenContext={openContext}
        />
      {:else if viewCtl.view === 'kanban'}
        <Kanban
          tasks={filterCtl.filtered}
          bind:mode={viewCtl.kanbanMode}
          bind:swimlane={viewCtl.kanbanSwimlane}
          bind:selectedIds={selCtl.selectedIds}
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
        <TasksInboxView
          {triage}
          {deadline}
          {filterCtl}
          {dataCtl}
          {viewCtl}
          cursorIdx={selCtl.cursorIdx}
          bind:selectedIds={selCtl.selectedIds}
          {load}
          onOpenDetail={openDetail}
          onOpenContext={openContext}
        />
      {:else if viewCtl.view === 'stale'}
        <div class="max-w-3xl">
          <AIStaleVerdicts
            candidates={filterCtl.filtered.filter(isStale)}
            allTasks={dataCtl.tasks}
            onReload={load}
          />
          <TasksCardList
            {filterCtl}
            {dataCtl}
            {viewCtl}
            cursorIdx={selCtl.cursorIdx}
            bind:selectedIds={selCtl.selectedIds}
            {load}
            onOpenDetail={openDetail}
            onOpenContext={openContext}
          />
        </div>
      {:else if viewCtl.view === 'duplicates'}
        <div class="max-w-3xl">
          <TaskDuplicates onReload={load} />
        </div>
      {:else if viewCtl.view === 'quickwins'}
        <div class="max-w-3xl">
          <p class="text-sm text-dim mb-4">High-priority tasks you can finish in ≤30 min. Pick one, knock it out.</p>
          <TasksCardList
            {filterCtl}
            {dataCtl}
            {viewCtl}
            cursorIdx={selCtl.cursorIdx}
            bind:selectedIds={selCtl.selectedIds}
            {load}
            onOpenDetail={openDetail}
            onOpenContext={openContext}
          />
        </div>
      {:else if viewCtl.view === 'review'}
        <div class="max-w-3xl">
          <p class="text-sm text-dim mb-4">Done in the last week — your retrospective view.</p>
          <TasksCardList
            {filterCtl}
            {dataCtl}
            {viewCtl}
            cursorIdx={selCtl.cursorIdx}
            bind:selectedIds={selCtl.selectedIds}
            dim
            {load}
            onOpenDetail={openDetail}
            onOpenContext={openContext}
          />
        </div>
      {:else}
        <!-- Stream N — smart-section grouped list. Sections (overdue /
             today / tomorrow / this week / later / no date / done)
             carry visual weight via tinted backgrounds and border-l-2
             on the loudest two. Collapse state per section persists.
             SectionList owns rendering; this page owns the data + the
             callbacks. -->
        <TasksSwipeHint applicable={swipeHintApplicable} />
        <SectionList
          groups={viewCtl.listGroups}
          filtered={filterCtl.filtered}
          cursorIdx={selCtl.cursorIdx}
          compactCards={viewCtl.compactCards}
          childCount={dataCtl.childCount}
          collapsedIds={dataCtl.collapsedIds}
          collapsedSections={viewCtl.collapsedSections}
          groupAddKey={groupAddCtl.key}
          bind:groupAddText={groupAddCtl.text}
          groupAddBusy={groupAddCtl.busy}
          bind:selectedIds={selCtl.selectedIds}
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

<TaskDetail bind:open={detCtl.detailOpen} task={detCtl.detailTask} onChanged={async () => {
  await load();
  // Refresh the in-drawer task copy from the freshly-loaded list so subsequent
  // edits see latest state.
  if (detCtl.detailTask) {
    const id = detCtl.detailTask.id;
    detCtl.detailTask = dataCtl.tasks.find((t) => t.id === id) ?? detCtl.detailTask;
  }
}} />

{#if detCtl.ctxTask}
  <TaskContextMenu
    task={detCtl.ctxTask}
    x={detCtl.ctxX}
    y={detCtl.ctxY}
    onClose={detCtl.closeContext}
    onChanged={load}
    onOpenDetail={openDetail}
  />
{/if}

<!-- Keyboard shortcuts overlay. Toggled with '?' or the header button. -->
<TasksShortcutsOverlay bind:open={viewCtl.helpOpen} />

<!-- When the user has bulk-selected dataCtl.tasks, narrow the agent's
     scope to that selection — the explicit selection IS the
     intent. Otherwise fall back to the page's filterCtl.filtered list so
     "agent over what I'm looking at" is the default. -->
<TaskAgent
  open={agentOpen}
  tasks={selCtl.selectedIds.size > 0 ? filterCtl.filtered.filter((t) => selCtl.selectedIds.has(t.id)) : filterCtl.filtered}
  todayISO={todayISO()}
  availableProjects={dataCtl.projects.map((p) => p.name)}
  onClose={() => (agentOpen = false)}
  onChanged={load}
/>
