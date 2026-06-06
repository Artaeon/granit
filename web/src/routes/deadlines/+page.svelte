<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { auth } from '$lib/stores/auth';
  import {
    api,
    type Deadline,
    type DeadlineCreate,
    type DeadlineImportance,
    type DeadlineStatus
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
  import {
    monthBucket,
    monthLabel,
    bucketTitle as bucketTitleOf,
    bucketTone as bucketToneOf,
    buildGrouped
  } from '$lib/deadlines/deadlinesBuckets';
  import { createDeadlinesData } from '$lib/deadlines/deadlinesData.svelte';
  import { createDeadlinesViewState } from '$lib/deadlines/deadlinesViewState.svelte';
  import { createDeadlinesFilterState } from '$lib/deadlines/deadlinesFilterState.svelte';

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

  // viewMode + groupBy + collapsedSections (plus their localStorage
  // round-trip and the three-state section toggle) all live in the
  // view-state controller. Read via viewCtl.X.
  const viewCtl = createDeadlinesViewState();
  let viewMode = $derived(viewCtl.viewMode);
  let groupBy = $derived(viewCtl.groupBy);
  let collapsedSections = $derived(viewCtl.collapsedSections);
  const toggleSection = viewCtl.toggleSection;

  // Loaded sidecars (deadlines + goals/projects/ventures), the lazy
  // open-tasks pool, and the loading / busy flags live in the data
  // controller. Read via dataCtl.X; the $derived aliases below keep
  // the rest of the script body terse where it works against rows.
  const dataCtl = createDeadlinesData({ isAuthed: () => !!$auth });
  let deadlines = $derived(dataCtl.deadlines);
  let goals = $derived(dataCtl.goals);
  let projects = $derived(dataCtl.projects);
  let ventures = $derived(dataCtl.ventures);
  let openTasks = $derived(dataCtl.openTasks);
  let tasksLoaded = $derived(dataCtl.tasksLoaded);
  let loading = $derived(dataCtl.loading);
  // load() + ensureTasksLoaded() proxy onto the controller. Keeping
  // the local names lets the rest of the page (onMount, save, quick-
  // actions, openCreate/openEdit) call them without touching dataCtl
  // and avoids a ripple-rewrite of every call site.
  const load = () => dataCtl.load();
  const ensureTasksLoaded = () => dataCtl.ensureTasksLoaded();

  // Optional URL-driven scope filters (e.g. /deadlines?project=Foo or
  // ?goal_id=G123). Used by the note-page deadline strip to deep-link
  // to "show me everything tied to this thing". Reactive — survives
  // SPA query-only navigations.
  let scopeProject = $derived($page.url.searchParams.get('project') ?? '');
  let scopeGoalId = $derived($page.url.searchParams.get('goal_id') ?? '');
  let scopeVenture = $derived($page.url.searchParams.get('venture') ?? '');

  // Importance chip + free-text search + the scoped/filtered/counts
  // derivations all live in the filter controller. Read via filterCtl;
  // the $derived aliases below keep the rest of the script terse.
  const filterCtl = createDeadlinesFilterState({
    getDeadlines: () => dataCtl.deadlines,
    getScopeProject: () => scopeProject,
    getScopeGoalId: () => scopeGoalId,
    getScopeVenture: () => scopeVenture
  });
  let importanceFilter = $derived(filterCtl.importanceFilter);
  let q = $derived(filterCtl.q);
  let scoped = $derived(filterCtl.scoped);
  let filtered = $derived(filterCtl.filtered);
  let importanceCounts = $derived(filterCtl.importanceCounts);
  const setFilter = filterCtl.setFilter;

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

  // Bucket helpers live in $lib/deadlines/deadlinesBuckets. The two
  // closures read the live `groupBy` $state via the call site so they
  // re-evaluate naturally when the user cycles group-by.
  let grouped = $derived(buildGrouped(filtered, groupBy));
  function bucketTitle(b: string): string {
    return bucketTitleOf(b, groupBy);
  }
  function bucketTone(b: string): string {
    return bucketToneOf(b, groupBy);
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
    dataCtl.busy = true;
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
      dataCtl.busy = false;
    }
  }

  async function remove() {
    if (!editing) return;
    if (!confirm(`Delete "${editing.title}"? This can't be undone.`)) return;
    dataCtl.busy = true;
    try {
      await api.deleteDeadline(editing.id);
      toast.success('deleted');
      drawerOpen = false;
      await load();
    } catch (e) {
      toast.error('delete failed: ' + (errorMessage(e)));
    } finally {
      dataCtl.busy = false;
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
      case 'v':
        e.preventDefault();
        viewCtl.cycleView();
        break;
      case 'g':
        e.preventDefault();
        viewCtl.cycleGroup();
        break;
      case 'Escape':
        if (shortcutsOpen) {
          e.preventDefault();
          shortcutsOpen = false;
        } else if (importanceFilter || q) {
          e.preventDefault();
          filterCtl.clearAll();
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
      filterCtl.q = '';
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
    onSelectView={(v) => (viewCtl.viewMode = v)}
    onSelectGroup={(g) => (viewCtl.groupBy = g)}
    onSearchChange={(v) => (filterCtl.q = v)}
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
          onClearAll={() => filterCtl.clearAll()}
        />
      </div>

      {#if importanceFilter || q}
        <div class="mb-3 text-xs text-dim flex items-center gap-1.5">
          <span>Showing {filtered.length} of {scoped.length}</span>
          <button
            type="button"
            onclick={() => filterCtl.clearAll()}
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
              onclick={() => filterCtl.clearAll()}
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
  busy={dataCtl.busy}
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

