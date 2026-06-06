<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, type Goal, type Project, type Task , todayISO } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { inlineMd } from '$lib/util/inlineMd';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import GoalCreate from '$lib/goals/GoalCreate.svelte';
  import GoalDetail from '$lib/goals/GoalDetail.svelte';
  import GoalDashboardPanel from '$lib/goals/GoalDashboardPanel.svelte';
  import GoalAgent from '$lib/goals/GoalAgent.svelte';
  import { isTypingTarget } from '$lib/util/isTypingTarget';
  import VisionContextStrip from '$lib/components/VisionContextStrip.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import {
    daysUntilTarget,
    targetChip,
    targetBorderColor,
    statusColor,
    fmtTargetDate,
    goalTargetTone
  } from '$lib/goals/util';
  import GoalKanbanCard from '$lib/goals/GoalKanbanCard.svelte';
  import GoalCard from '$lib/goals/GoalCard.svelte';
  import GoalsPageHeader from '$lib/goals/GoalsPageHeader.svelte';
  import GoalsStatusChips from '$lib/goals/GoalsStatusChips.svelte';
  import GoalsAICheckinPanel, { type CheckinEntry } from '$lib/goals/GoalsAICheckinPanel.svelte';
  import GoalsAIAuditPanel, { type AuditFinding } from '$lib/goals/GoalsAIAuditPanel.svelte';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';
  import {
    createGoalsFilterState,
    type GoalsViewMode,
    type GoalsStatusFilter
  } from '$lib/goals/goalsFilterState.svelte';
  import { createGoalsData } from '$lib/goals/goalsData.svelte';
  import { createGoalsAiSuggest } from '$lib/goals/goalsAiSuggest.svelte';
  import { createGoalsCheckin } from '$lib/goals/goalsCheckin.svelte';
  import { workspaceContext } from '$lib/workspace/workspaceContext.svelte';

  // Loaded data (dataCtl.goals + dataCtl.openTasks/dataCtl.doneTasks/dataCtl.projects sidecars) +
  // dataCtl.loading flags + dataCtl.load() + per-goal dataCtl.rollups + stalled detection
  // all live in $lib/goals/goalsData. Read via dataCtl.X.
  const dataCtl = createGoalsData({ isAuthed: () => !!$auth });

  // View + filter state. Reads dataCtl.goals through dataCtl.
  const filterCtl = createGoalsFilterState({
    getGoals: () => dataCtl.goals
  });

  // Goal Agent — conversational mutation engine for /goals.
  // Mirrors Task/Project agents; lives on the same audit-gated
  // chatStream pipeline. Operates on the filtered list.
  let agentOpen = $state(false);

  let createOpen = $state(false);
  let detailOpen = $state(false);
  let selectedId = $state<string | null>(null);

  // "More" dropdown — collapsed AI surface launcher. Reflects the
  // open-state of either AI panel so the header tints the trigger.
  let moreOpen = $state(false);

  // Tracks whether dataCtl.load() has resolved at least once. Drives the
  // skeleton vs empty-state choice — pre-resolution we render
  // shimmer placeholders, post-resolution we render the proper
  // "no dataCtl.goals" empty state. Without this distinction the page
  // briefly flashed "no dataCtl.goals match this filter." on every mount.
  // dataCtl.firstLoaded + dataCtl.load() moved into dataCtl.
  onMount(() => {
    dataCtl.load();
    // ?agent=1 launches the Goal Agent — used by the chat sidebar.
    if ($page.url.searchParams.get('agent') === '1') {
      agentOpen = true;
      const params = new URLSearchParams($page.url.searchParams);
      params.delete('agent');
      void goto(`/goals${params.toString() ? '?' + params : ''}`, {
        replaceState: true,
        keepFocus: true
      });
    }
    const unsub = onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') dataCtl.load();
      // Re-fetch when the TUI (or another web tab) writes goals.json.
      // The server broadcasts state.changed with Path=".granit/goals.json".
      if (ev.type === 'state.changed' && ev.path === '.granit/goals.json') dataCtl.load();
    });
    // Visibility-aware refresh: WS connections are suspended when the
    // tab is backgrounded (especially on mobile Safari), so we'd miss
    // any state.changed event fired in that window. Refetching on
    // visibility flip cheaply guarantees the user never returns to a
    // stale list.
    const onVisible = () => {
      if (document.visibilityState === 'visible') dataCtl.load();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', onVisible);
    // 'a' opens the Goal Agent — same hotkey contract as /tasks
    // and /dataCtl.projects. isTypingTarget guard suppresses while the
    // user is typing anywhere on the page.
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (isTypingTarget(e.target)) return;
      if (e.key === 'a') {
        agentOpen = true;
        e.preventDefault();
      }
    };
    window.addEventListener('keydown', onKey);
    // Click-outside dismiss for the More dropdown. Mirrors the
    // /tasks page's pattern with the data-* marker on the trigger.
    const onDocClick = (e: MouseEvent) => {
      if (!moreOpen) return;
      const t = e.target as HTMLElement | null;
      if (t && t.closest('[data-goals-more]')) return;
      moreOpen = false;
    };
    document.addEventListener('click', onDocClick);
    return () => {
      unsub();
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', onVisible);
      window.removeEventListener('keydown', onKey);
      document.removeEventListener('click', onDocClick);
    };
  });

  // ?focus=<goalId> auto-opens the matching detail drawer. Used by the
  // /tasks page deepLink when a goal-group "open ↗" is clicked. Without
  // this the deepLink used to point at /goals/<id> — a route that
  // didn't exist, so SvelteKit served the SPA shell and the client
  // router fell through silently. The user perceived it as a freeze.
  $effect(() => {
    const focus = $page.url.searchParams.get('focus');
    if (!focus) return;
    if (dataCtl.goals.length === 0) return; // wait until dataCtl.load() resolves
    const g = dataCtl.goals.find((x) => x.id === focus);
    if (g && selectedId !== focus) {
      selectedId = focus;
      detailOpen = true;
    }
  });

  // Selected goal — derived from id so live edits during a refetch find
  // the new copy without reopening the drawer at a stale state.
  let selected = $derived(dataCtl.goals.find((g) => g.id === selectedId) ?? null);

  // Dashboard overlay — full-screen GoalDashboardPanel for the focused
  // goal. State persists in the URL (?focus=X&dashboard=1) so a reload
  // or shared link keeps it open. Pure presentation flag — the panel
  // does its own data load.
  let dashboardOpen = $derived(
    $page.url.searchParams.get('dashboard') === '1' && !!selected
  );
  function openDashboard() {
    if (!selectedId) return;
    const params = new URLSearchParams($page.url.searchParams);
    params.set('focus', selectedId);
    params.set('dashboard', '1');
    goto(`/goals?${params.toString()}`, { replaceState: true, keepFocus: true });
  }
  function closeDashboard() {
    const params = new URLSearchParams($page.url.searchParams);
    params.delete('dashboard');
    goto(`/goals?${params.toString()}`, { replaceState: true, keepFocus: true });
  }

  function openDetail(g: Goal) {
    selectedId = g.id;
    detailOpen = true;
    workspaceContext.publish({
      paneKind: 'goals',
      itemId: g.id,
      label: g.title,
      excerpt: g.description ?? undefined
    });
  }
  function openDetailById(id: string) {
    const g = dataCtl.goals.find((x) => x.id === id);
    if (g) openDetail(g);
  }

  // filterCtl.filtered moved into filterCtl.

  function progress(g: Goal): { done: number; total: number; pct: number } {
    const ms = g.milestones ?? [];
    const total = ms.length;
    if (total === 0) return { done: 0, total: 0, pct: g.status === 'completed' ? 100 : 0 };
    const done = ms.filter((m) => m.done).length;
    return { done, total, pct: Math.round((done / total) * 100) };
  }

  // Per-goal index of linked tasks (by goalId) + linked dataCtl.projects (by
  // matching goal.project against project.name, since the schema is
  // free-text not FK). Computed in a single pass over each list so
  // the per-goal lookups in the render loop stay O(1). Used by the
  // cards view to surface a "5 open · 2 done · 1 project" chip row
  // — the user's signal for which dataCtl.goals actually have execution
  // behind them and which are still abstractions.
  // dataCtl.rollups + rollupFor moved into dataCtl.

  // Status-pill colors, friendly date formatting, and urgency-tone
  // routing all live in $lib/goals/util now — single source of truth
  // for every goal surface (kanban, cards, list, dashboard widgets).
  // The local copies that used to live here are gone; call sites
  // route through the imports above.

  // filterCtl.counts moved into filterCtl.

  // ── Stalled-goal detection ─────────────────────────────────────
  //
  // A goal is "stalled" when:
  //   - its status is active (we don't nag paused / completed /
  //     archived rows — those are deliberate states)
  //   - the goal record itself hasn't been touched in 30+ days
  //     (no metadata edit, no milestone tick)
  //   - AND no linked task has been completed in the last 14 days
  //     (because the goal might be quiet on its own record while
  //     real progress is happening in the task list)
  //
  // The banner exists to convert "I forgot about this" into a
  // visible signal without auto-archiving — the user decides
  // whether to update the goal, log a milestone, or move it to
  // paused/archived.
  // staleness + recentCompletionForGoal + dataCtl.stalledGoals moved into
  // dataCtl.
  // When the user clicks the banner action, we filter the list
  // down to just the stalled rows. Re-derive over `filterCtl.filtered` rather
  // than mutating filters so the existing search/category/tag
  // pickers aren't disturbed.
  let stalledFilterOn = $state(false);
  let visibleGoals = $derived.by(() => {
    if (!stalledFilterOn) return filterCtl.filtered;
    const stalledIds = new Set(dataCtl.stalledGoals.map((g) => g.id));
    return filterCtl.filtered.filter((g) => stalledIds.has(g.id));
  });

  // Kanban grouping — same status order as the tabs so the column
  // order matches the user's mental model. Filtering still applies
  // (search / category / venture / tag); the status filter is only
  // honoured when it isn't 'all', otherwise every column renders so
  // the kanban surfaces the full pipeline. Sort within each column:
  // imminent target_date first, then by title for stability.
  type KanbanCol = 'active' | 'paused' | 'completed' | 'archived';
  const kanbanColumns: KanbanCol[] = ['active', 'paused', 'completed', 'archived'];
  let kanbanGroups = $derived.by((): Record<KanbanCol, Goal[]> => {
    const out: Record<KanbanCol, Goal[]> = {
      active: [], paused: [], completed: [], archived: []
    };
    for (const g of filterCtl.filtered) {
      const s = (g.status ?? 'active') as KanbanCol;
      if (out[s]) out[s].push(g);
    }
    const sortKey = (g: Goal): number => {
      const d = daysUntilTarget(g.target_date);
      // Goals with no parseable date sink to the bottom; among the
      // dated, smaller (closer / overdue) days come first.
      return d === null ? Number.POSITIVE_INFINITY : d;
    };
    for (const col of kanbanColumns) {
      out[col].sort((a, b) => {
        const sa = sortKey(a), sb = sortKey(b);
        if (sa !== sb) return sa - sb;
        return a.title.localeCompare(b.title);
      });
    }
    return out;
  });

  // ----- Hero "next target" -----
  // Picks the most-imminent active or paused goal with a parseable
  // target_date and surfaces it as a hero card above the list. Skips
  // dataCtl.goals whose status excludes them from the urgency treatment
  // (completed / archived) and skips free-text target_dates that
  // can't be compared to "today". Falls back to null when the user
  // has no dated dataCtl.goals — the hero card simply doesn't render.
  let goalHero = $derived.by((): { goal: Goal; days: number } | null => {
    let best: Goal | null = null;
    let bestDays = Infinity;
    for (const g of dataCtl.goals) {
      const status = g.status ?? 'active';
      if (status !== 'active' && status !== 'paused') continue;
      const days = daysUntilTarget(g.target_date);
      if (days === null) continue;
      // Earliest target wins; overdue dataCtl.goals (negative days) sort
      // ahead of upcoming ones because they need attention more.
      if (days < bestDays) {
        bestDays = days;
        best = g;
      }
    }
    return best ? { goal: best, days: bestDays } : null;
  });

  // ----- Target-proximity stat strip -----
  // Distribution of dated active+paused dataCtl.goals across urgency
  // buckets, surfaced as a one-line summary below the status tabs.
  // Complements the hero (single most-pressing goal) by showing the
  // shape of the whole pipeline at a glance. Free-text target_dates
  // and undated dataCtl.goals are excluded — they have no place on a
  // proximity axis.
  let targetStats = $derived.by(() => {
    let pastTarget = 0, thisMonth = 0, thisQuarter = 0, later = 0;
    for (const g of dataCtl.goals) {
      const status = g.status ?? 'active';
      if (status !== 'active' && status !== 'paused') continue;
      const days = daysUntilTarget(g.target_date);
      if (days === null) continue;
      if (days < 0) pastTarget++;
      else if (days <= 30) thisMonth++;
      else if (days <= 90) thisQuarter++;
      else later++;
    }
    return { pastTarget, thisMonth, thisQuarter, later };
  });

  // Distinct category + tag chips, sorted by frequency desc so the most
  // common chip surfaces first.
  let categories = $derived.by(() => {
    const m = new Map<string, number>();
    for (const g of dataCtl.goals) {
      const c = (g.category ?? '').trim();
      if (!c) continue;
      m.set(c, (m.get(c) ?? 0) + 1);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([c]) => c);
  });
  let tags = $derived.by(() => {
    const m = new Map<string, number>();
    for (const g of dataCtl.goals) {
      for (const t of g.tags ?? []) m.set(t, (m.get(t) ?? 0) + 1);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([t]) => t);
  });
  let ventures = $derived.by(() => {
    const m = new Map<string, number>();
    for (const g of dataCtl.goals) {
      const v = (g.venture ?? '').trim();
      if (!v) continue;
      m.set(v, (m.get(v) ?? 0) + 1);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([v]) => v);
  });

  async function created(g: Goal) {
    // Optimistic prepend so the new goal renders immediately. The
    // dataCtl.load() below reconciles with the server (auth-stamped CreatedAt,
    // any defaults the server filled in).
    if (!dataCtl.goals.some((x) => x.id === g.id)) {
      dataCtl.goals = [g, ...goals];
    }
    selectedId = g.id;
    detailOpen = true;
    await dataCtl.load();
  }

  async function deleted(_id: string) {
    detailOpen = false;
    selectedId = null;
    await dataCtl.load();
    toast.success('goal deleted');
  }

  // AI "next milestone" suggester lives in goalsAiSuggest.svelte.
  // The controller owns the open card id, streaming text, busy / error
  // state, and the abort lifecycle. The page just calls toggle / close
  // / adoptAsMilestone and reads via aiCtl.X.
  const aiCtl = createGoalsAiSuggest({ dataCtl });

  // AI "Weekly check-in" controller — lives in goalsCheckin.svelte.
  // Owns the open/busy/error/entries/hidden state, the live scope
  // derive, run/saveOne/saveAll/dismiss/close methods, and the prompt
  // composition. Read via checkinCtl.X.
  const checkinCtl = createGoalsCheckin({ dataCtl });

  // ─────────────────────────────────────────────────────────────────
  // AI "Goal alignment audit" — strategy/execution drift detector
  // ─────────────────────────────────────────────────────────────────
  // Reads all active dataCtl.goals + open tasks + recently-completed tasks
  // (last 14 days, no goalId) and asks the model: which clusters of
  // tasks are NOT advancing any stated goal? Surfaces the gap that
  // goal-setters typically can't see for themselves: the busywork
  // that fills the day without moving the season.
  //
  // The audit is honest and non-judgmental — the user may be
  // intentionally working off-goal (urgent maintenance, paid work,
  // family emergency). The model's job is to surface the pattern,
  // not to scold. The user can dismiss findings, mark a finding as
  // "intentional" (no action), or jump to /tasks to re-link.
  let auditOpen = $state(false);
  let auditBusy = $state(false);
  let auditError = $state('');
  let auditFindings = $state<AuditFinding[]>([]);
  let auditAbort: AbortController | null = null;
  let auditDismissed = $state<Set<string>>(new Set());

  // Tasks the audit looks at — in-flight + recently-done, both
  // unlinked from any goal. Cap at 80 each so the prompt stays
  // bounded; the model sees representative behaviour, not a full
  // dump. Recently-done is limited to the last 14 days because
  // older history isn't actionable for a "this season" check.
  let auditScope = $derived.by(() => {
    const cutoff = Date.now() - 14 * 24 * 3600 * 1000;
    const orphanOpen = dataCtl.openTasks
      .filter((t) => !t.goalId && (t.text ?? '').trim().length > 0)
      .slice(0, 80);
    const orphanDoneRecent = dataCtl.doneTasks
      .filter((t) => {
        if (t.goalId) return false;
        if (!t.completedAt) return false;
        const d = new Date(t.completedAt).getTime();
        return Number.isFinite(d) && d >= cutoff;
      })
      .slice(0, 80);
    const linkedOpen = dataCtl.openTasks.filter((t) => t.goalId).length;
    const linkedDone14 = dataCtl.doneTasks.filter((t) => {
      if (!t.goalId || !t.completedAt) return false;
      const d = new Date(t.completedAt).getTime();
      return Number.isFinite(d) && d >= cutoff;
    }).length;
    return { orphanOpen, orphanDoneRecent, linkedOpen, linkedDone14 };
  });

  function auditClose() {
    auditAbort?.abort();
    auditAbort = null;
    auditOpen = false;
    auditBusy = false;
    auditError = '';
    auditFindings = [];
    auditDismissed = new Set();
  }

  async function runAudit() {
    if (auditBusy) return;
    const activeGoals = dataCtl.goals.filter((g) => (g.status ?? 'active') === 'active');
    if (activeGoals.length === 0) {
      toast.error('No active goals to audit against.');
      return;
    }
    const totalOrphan = auditScope.orphanOpen.length + auditScope.orphanDoneRecent.length;
    if (totalOrphan === 0) {
      toast.success('Every recent task is linked to a goal — nothing to audit.');
      return;
    }
    auditAbort?.abort();
    auditAbort = new AbortController();
    auditOpen = true;
    auditBusy = true;
    auditError = '';
    auditFindings = [];
    auditDismissed = new Set();

    const goalLines = activeGoals
      .map((g) => `- ${g.title}${g.target_date ? ` (target ${g.target_date})` : ''}${g.venture ? ` [${g.venture}]` : ''}`)
      .join('\n');
    const orphanOpenLines = auditScope.orphanOpen.map((t) => `- ${t.text}`).join('\n');
    const orphanDoneLines = auditScope.orphanDoneRecent.map((t) => `- ${t.text}`).join('\n');

    const userMessage =
      'You are an honest, non-judgmental auditor of where the user\'s actual work is going.\n' +
      'Compare the user\'s ACTIVE GOALS to their TASKS that are NOT linked to any goal. ' +
      'Find 2-5 clusters of unlinked tasks that share a theme. For each cluster, surface what is happening and ask whether it was intentional.\n\n' +
      'Rules:\n' +
      '- Be specific. "You worked on support" beats "you worked on miscellaneous things".\n' +
      '- Cluster by theme (e.g. "support / maintenance", "finances / admin", "client work for X", "household").\n' +
      '- Off-goal work is NOT inherently bad — paid work, urgent maintenance, family. Your job is to NAME the pattern, not to scold.\n' +
      '- Include the rough count of tasks in each cluster and 2-3 representative task texts (verbatim).\n' +
      '- The "question" should be honest and useful: "Was this week\'s 12 tasks on X the right call given goal Y is overdue?" — never generic.\n' +
      '- Skip clusters with fewer than 2 tasks. Don\'t pad to hit a number; 2 sharp findings beat 5 mush ones.\n\n' +
      'Return STRICT JSON ONLY (no markdown fences, no preamble), shape:\n' +
      '[{"cluster": "...", "count": N, "sample": ["...", "..."], "observation": "...", "question": "..."}, ...]\n\n' +
      'ACTIVE GOALS:\n' + (goalLines || '(none)') + '\n\n' +
      `UNLINKED OPEN TASKS (${auditScope.orphanOpen.length}):\n` + (orphanOpenLines || '(none)') + '\n\n' +
      `UNLINKED TASKS COMPLETED IN LAST 14 DAYS (${auditScope.orphanDoneRecent.length}):\n` + (orphanDoneLines || '(none)') + '\n\n' +
      'For context the user already has linked work too: ' +
      `${auditScope.linkedOpen} open tasks tied to goals, ${auditScope.linkedDone14} goal-linked tasks completed in 14d. ` +
      'Don\'t mention this in your output unless it changes the verdict.';

    let acc = '';
    try {
      await api.chatStream(
        [{ role: 'user', content: userMessage }],
        undefined,
        {
          onChunk: (c) => { acc += c; },
          onDone: () => {
            auditBusy = false;
            auditAbort = null;
            let cleaned = acc.trim();
            if (cleaned.startsWith('```')) {
              cleaned = cleaned.replace(/^```(?:json)?\s*/, '').replace(/```\s*$/, '').trim();
            }
            try {
              const parsed = JSON.parse(cleaned);
              if (!Array.isArray(parsed)) throw new Error('expected array');
              auditFindings = parsed
                .filter((p: unknown) => p && typeof p === 'object')
                .map((p) => p as AuditFinding)
                .filter((p) => typeof p.cluster === 'string' && typeof p.observation === 'string' && typeof p.question === 'string')
                .map((p) => ({
                  cluster: p.cluster,
                  count: typeof p.count === 'number' ? p.count : (Array.isArray(p.sample) ? p.sample.length : 0),
                  sample: Array.isArray(p.sample) ? p.sample.filter((s): s is string => typeof s === 'string').slice(0, 3) : [],
                  observation: p.observation,
                  question: p.question
                }));
              if (auditFindings.length === 0) {
                auditError = 'AI returned no clusters — the work may already be aligned, or the parse failed.';
              }
            } catch (err) {
              auditError = 'Couldn\'t parse audit: ' + (errorMessage(err));
            }
          },
          onError: (err) => {
            auditBusy = false;
            auditAbort = null;
            auditError = err.message;
          }
        },
        auditAbort.signal
      );
    } catch (e) {
      auditBusy = false;
      auditAbort = null;
      auditError = errorMessage(e);
    }
  }

  function auditDismiss(f: AuditFinding) {
    auditDismissed = new Set([...auditDismissed, f.cluster]);
  }

  // Header callback wiring — keeps the More dropdown closing when
  // the user picks something inside it.
  function onToggleMore() { moreOpen = !moreOpen; }
  function onToggleCheckin() {
    moreOpen = false;
    if (checkinCtl.checkinOpen) checkinCtl.close();
    else void checkinCtl.run();
  }
  function onToggleAudit() {
    moreOpen = false;
    if (auditOpen) auditClose();
    else void runAudit();
  }

  // Active-dataCtl.goals count is reused by the audit panel header.
  let activeGoalsCount = $derived(dataCtl.goals.filter((g) => (g.status ?? 'active') === 'active').length);
</script>

<div class="h-full overflow-y-auto">
  <!-- Slim page header — title, count chip, view picker, More menu,
       primary "+ New goal" button. Replaces the prior PageHeader
       strip + view-mode toolbar. -->
  <GoalsPageHeader
    view={filterCtl.viewMode}
    totalCount={dataCtl.goals.length}
    filteredCount={visibleGoals.length}
    checkinOpen={checkinCtl.checkinOpen}
    checkinBusy={checkinCtl.checkinBusy}
    auditOpen={auditOpen}
    auditBusy={auditBusy}
    moreOpen={moreOpen}
    onSelectView={(v) => (filterCtl.viewMode = v)}
    onToggleMore={onToggleMore}
    onToggleCheckin={onToggleCheckin}
    onToggleAudit={onToggleAudit}
    onCreate={() => (createOpen = true)}
  />

  <!-- Container widens in kanban mode so the four columns have room
       to breathe. Cards / list stay at the original 4xl reading width
       so long titles remain comfortable. -->
  <div class="p-4 sm:p-6 lg:p-8 mx-auto {filterCtl.viewMode === 'kanban' ? 'max-w-7xl' : 'max-w-4xl'}">
    <VisionContextStrip />

    <!-- Weekly check-in + Alignment audit panels — toggled from the
         More dropdown in the slim header. State machines stay on
         this page; the components own the chrome. -->
    {#if checkinCtl.checkinOpen}
      <GoalsAICheckinPanel
        scope={checkinCtl.checkinScope}
        entries={checkinCtl.checkinEntries}
        hidden={checkinCtl.checkinHidden}
        busy={checkinCtl.checkinBusy}
        error={checkinCtl.checkinError}
        goals={dataCtl.goals}
        rollupFor={checkinCtl.rollupFor}
        recentDoneFor={checkinCtl.recentDoneFor}
        onAbort={checkinCtl.stop}
        onRetry={() => void checkinCtl.run()}
        onClose={checkinCtl.close}
        onSaveOne={(e) => void checkinCtl.saveOne(e)}
        onSaveAll={checkinCtl.saveAll}
        onDismiss={checkinCtl.dismiss}
        onOpenGoal={openDetailById}
      />
    {/if}

    {#if auditOpen}
      <GoalsAIAuditPanel
        findings={auditFindings}
        dismissed={auditDismissed}
        busy={auditBusy}
        error={auditError}
        orphanOpenCount={auditScope.orphanOpen.length}
        orphanDoneCount={auditScope.orphanDoneRecent.length}
        linkedCount={auditScope.linkedOpen + auditScope.linkedDone14}
        activeGoalsCount={activeGoalsCount}
        onAbort={() => auditAbort?.abort()}
        onRetry={() => void runAudit()}
        onClose={auditClose}
        onDismissFinding={auditDismiss}
      />
    {/if}

    <!-- Hero "next target" card — surfaces the most-imminent active
         or paused goal with a parseable target_date so the user lands
         on what's most pressing without scanning the list. Click jumps
         to the goal's detail drawer (same affordance as the cards
         below). Border tint = urgency, mirroring the deadlines hero. -->
    {#if goalHero}
      {@const h = goalHero.goal}
      {@const days = goalHero.days}
      {@const p = progress(h)}
      <button
        type="button"
        onclick={() => openDetail(h)}
        class="w-full text-left block mb-5 p-4 sm:p-5 bg-surface0 border-l-4 rounded-lg hover:border-primary transition-colors"
        style="border-left-color: {targetBorderColor(days)};"
      >
        <div class="flex items-start gap-3">
          <span class="text-2xl flex-shrink-0 mt-0.5" aria-hidden="true">🎯</span>
          <div class="flex-1 min-w-0">
            <div class="text-[11px] uppercase tracking-wider text-dim mb-0.5">Next target</div>
            <div class="text-lg sm:text-xl font-semibold text-text leading-tight break-words">
              {#if days < 0}
                <span class="text-error">{Math.abs(days)} {Math.abs(days) === 1 ? 'day' : 'days'} past target</span> · {h.title}
              {:else if days === 0}
                <span class="text-error">Today</span> · {h.title}
              {:else if days === 1}
                <span class="text-warning">Tomorrow</span> · {h.title}
              {:else}
                <span class="text-primary">{days} days</span>
                <span class="text-dim font-normal">until</span>
                {h.title}
              {/if}
            </div>
            <div class="flex flex-wrap items-center gap-x-3 gap-y-1 mt-1.5 text-xs text-dim">
              <span class="font-mono tabular-nums text-subtext">{fmtTargetDate(h.target_date)}</span>
              {#if h.venture}
                <span class="text-secondary">🏢 {h.venture}</span>
              {/if}
              {#if h.project}
                <span class="text-secondary">📁 {h.project}</span>
              {/if}
              {#if p.total > 0}
                <span class="tabular-nums">{p.done}/{p.total} milestones · {p.pct}%</span>
              {/if}
              <span class="ml-auto text-secondary hover:underline">View →</span>
            </div>
          </div>
        </div>
      </button>
    {/if}

    <!-- Status quick-filter chips — replaces the old segmented pill
         row with tone-tinted chips so the active filter reads off
         the colour (active=primary, paused=warning, completed=
         success, archived=dim). -->
    <div class="mb-3">
      <GoalsStatusChips
        status={filterCtl.statusFilter}
        counts={filterCtl.counts}
        onSet={(s) => (filterCtl.statusFilter = s)}
      />
    </div>

    <!-- Target-proximity stat strip — complements the hero card by
         showing the distribution of dated dataCtl.goals across urgency
         buckets. Hidden when no dated dataCtl.goals exist (the strip would
         just read "0 0 0 0"). -->
    {#if targetStats.pastTarget + targetStats.thisMonth + targetStats.thisQuarter + targetStats.later > 0}
      <div class="flex flex-wrap items-baseline gap-x-4 gap-y-1 mb-4 text-xs">
        {#if targetStats.pastTarget > 0}
          <span class="text-error font-medium tabular-nums">{targetStats.pastTarget} past target</span>
        {/if}
        {#if targetStats.thisMonth > 0}
          <span class="text-warning tabular-nums">{targetStats.thisMonth} this month</span>
        {/if}
        {#if targetStats.thisQuarter > 0}
          <span class="text-info tabular-nums">{targetStats.thisQuarter} this quarter</span>
        {/if}
        {#if targetStats.later > 0}
          <span class="text-dim tabular-nums">{targetStats.later} later</span>
        {/if}
      </div>
    {/if}

    <!-- Search + category/tag/venture chips. The chip row only
         renders when at least one dimension has values; an empty
         dataCtl.goals set sees just the search bar. -->
    <div class="mb-4 space-y-2">
      <input
        bind:value={filterCtl.q}
        placeholder="search title, description, notes…"
        class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      {#if categories.length > 0 || tags.length > 0 || ventures.length > 0}
        <div class="flex flex-wrap items-center gap-1.5 text-xs">
          {#if filterCtl.categoryFilter || filterCtl.tagFilter || filterCtl.ventureFilter}
            <button
              onclick={() => { filterCtl.categoryFilter = ''; filterCtl.tagFilter = ''; filterCtl.ventureFilter = ''; }}
              class="px-2 py-0.5 bg-surface1 text-dim rounded hover:text-text"
            >clear filters</button>
          {/if}
          {#each ventures as v}
            <button
              onclick={() => (filterCtl.ventureFilter = filterCtl.ventureFilter === v ? '' : v)}
              class="px-2 py-0.5 rounded {filterCtl.ventureFilter === v ? 'bg-secondary text-on-primary' : 'bg-surface0 text-secondary hover:bg-surface1'}"
              title="filter to this venture"
            >🏢 {v}</button>
          {/each}
          {#each categories as c}
            <button
              onclick={() => (filterCtl.categoryFilter = filterCtl.categoryFilter === c ? '' : c)}
              class="px-2 py-0.5 rounded {filterCtl.categoryFilter === c ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
            >{c}</button>
          {/each}
          {#each tags as t}
            <button
              onclick={() => (filterCtl.tagFilter = filterCtl.tagFilter === t ? '' : t)}
              class="px-2 py-0.5 rounded {filterCtl.tagFilter === t ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
            >#{t}</button>
          {/each}
        </div>
      {/if}
    </div>

    <!-- Stalled-goal banner — surfaces active dataCtl.goals with no
         metadata edit in 30+ days AND no completed task in 14+
         days. Click to filter the list down to just those rows;
         click again to clear. The banner only renders when at
         least one stalled goal exists, so a healthy goal set
         never sees the nag. -->
    {#if dataCtl.firstLoaded && dataCtl.stalledGoals.length > 0}
      <button
        type="button"
        onclick={() => (stalledFilterOn = !stalledFilterOn)}
        class="w-full text-left mb-4 px-3 py-2.5 rounded-lg border flex items-center gap-3 transition-colors {stalledFilterOn ? 'bg-surface0 border-warning' : 'bg-surface0 border-warning hover:bg-surface1'}"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-4 h-4 text-warning flex-shrink-0">
          <circle cx="12" cy="12" r="9"/>
          <path d="M12 7v5l3 2"/>
        </svg>
        <div class="flex-1 text-sm">
          <span class="text-text font-medium">
            {dataCtl.stalledGoals.length}
            {dataCtl.stalledGoals.length === 1 ? 'goal looks' : 'goals look'}
            stalled
          </span>
          <span class="text-dim ml-1">
            — no edits in 30+ days, no recent completed tasks
          </span>
        </div>
        <span class="text-xs text-subtext flex-shrink-0">
          {stalledFilterOn ? 'showing only stalled · click to clear' : 'click to focus'}
        </span>
      </button>
    {/if}

    {#if !dataCtl.firstLoaded && dataCtl.loading}
      <!-- Shimmer skeletons match the cards-view rhythm so the
           layout doesn't reflow on first load. Three placeholders is
           a reasonable bet — most users have at least that many
           dataCtl.goals once they're past day-one. -->
      <div class="space-y-4">
        {#each [0, 1, 2] as i (i)}
          <div class="bg-surface0 border border-surface1 rounded-lg p-3 space-y-2.5">
            <div class="flex items-start gap-3">
              <div class="flex-1 space-y-1.5">
                <Skeleton class="h-5 w-2/3" />
                <Skeleton class="h-3.5 w-full" />
              </div>
              <Skeleton class="h-4 w-14 rounded-full" />
            </div>
            <Skeleton class="h-3 w-1/2" />
            <Skeleton class="h-1.5 w-full" />
          </div>
        {/each}
      </div>
    {:else if visibleGoals.length === 0}
      <!-- Empty state branches: real "no dataCtl.goals at all" vs "filter
           hides everything". The first nudges the user to create
           their first goal; the second offers a clear-filters
           shortcut so they don't have to hunt for the UI. -->
      {#if dataCtl.goals.length === 0}
        <EmptyState
          icon="🎯"
          title="No goals yet"
          description="Goals are the long-term targets you're committing to in this season — quarterly, annual, lifetime. Add your first one to get started."
        >
          {#snippet action()}
            <button
              onclick={() => (createOpen = true)}
              class="px-4 py-2 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90"
            >+ Create a goal</button>
          {/snippet}
        </EmptyState>
      {:else}
        <EmptyState
          icon="🔍"
          title="No goals match this filter"
          description={stalledFilterOn ? "Stalled-only filter is on but every goal looks fresh — click the banner to clear it." : "Try a different status tab, clear the search, or drop your category / tag filters."}
        >
          {#snippet action()}
            <button
              onclick={() => {
                filterCtl.statusFilter = 'all';
                filterCtl.categoryFilter = '';
                filterCtl.tagFilter = '';
                filterCtl.ventureFilter = '';
                filterCtl.q = '';
                stalledFilterOn = false;
              }}
              class="px-3 py-1.5 bg-surface1 text-text rounded text-sm hover:bg-surface2"
            >Clear all filters</button>
          {/snippet}
        </EmptyState>
      {/if}
    {:else if filterCtl.viewMode === 'cards'}
      <div class="space-y-4">
        {#each visibleGoals as g (g.id)}
          {@const p = progress(g)}
          {@const tone = goalTargetTone(g.status, g.target_date)}
          {@const roll = dataCtl.rollupFor(g)}
          {@const aiOpen = aiCtl.aiGoalId === g.id}
          {@const isArchived = g.status === 'archived'}
          {@const isCompleted = g.status === 'completed'}
          {@const isStalled = dataCtl.stalledGoals.some((s) => s.id === g.id)}
          <article
            class="bg-surface0 rounded-lg overflow-hidden transition-colors {tone ? 'border-l-4' : ''} {isArchived ? 'border border-dashed border-surface1' : 'border border-surface1 hover:border-primary'} {isStalled ? 'ring-1 ring-warning/30' : ''} {isCompleted ? 'opacity-90' : ''}"
            style={tone ? `border-left-color: var(--color-${tone});` : ''}
          >
            <GoalCard
              goal={g}
              progress={p}
              rollup={roll}
              stalled={isStalled}
              onClick={() => openDetail(g)}
            />

            <!-- AI "next milestone" affordance — only on living dataCtl.goals
                 (active / paused). Sits in a footer strip outside the
                 card-wide button so the AI button isn't an
                 accidentally-nested <button>. The inline panel opens
                 below it when the user invokes the suggestion. -->
            {#if g.status === 'active' || g.status === 'paused'}
              <div class="border-t border-surface1 px-3 py-1.5 flex items-center justify-end">
                <button
                  type="button"
                  onclick={() => aiCtl.toggle(g)}
                  class="inline-flex items-center gap-1 text-[11px] {aiOpen ? 'text-primary' : 'text-dim hover:text-secondary'}"
                  title="ask AI for the next concrete milestone"
                  aria-label="suggest next milestone with AI"
                  aria-expanded={aiOpen}
                  disabled={aiCtl.aiBusy && !aiOpen}
                >
                  {aiOpen ? '✕' : '✨'} {aiOpen ? 'close' : 'suggest next milestone'}
                </button>
              </div>
              {#if aiOpen}
                <div class="border-t border-surface1 bg-mantle p-3">
                  <div class="text-[10px] uppercase tracking-wider text-dim mb-1">AI suggestion</div>
                  {#if aiCtl.aiError}
                    <div class="text-sm text-error mb-2">{aiCtl.aiError}</div>
                  {/if}
                  {#if aiCtl.aiText || aiCtl.aiBusy}
                    <div class="text-sm text-text leading-snug min-h-[1.4em]">
                      {aiCtl.aiText}{#if aiCtl.aiBusy}<span class="inline-block w-1.5 h-3.5 ml-0.5 align-middle bg-primary/60 animate-pulse rounded-sm"></span>{/if}
                    </div>
                  {/if}
                  <div class="flex flex-wrap items-center gap-2 mt-2.5">
                    {#if aiCtl.aiBusy}
                      <button
                        type="button"
                        onclick={aiCtl.stop}
                        class="px-2 py-1 text-xs bg-surface1 text-subtext rounded hover:bg-surface2"
                      >Stop</button>
                    {:else}
                      <button
                        type="button"
                        onclick={() => aiCtl.adoptAsMilestone(g)}
                        disabled={!aiCtl.aiText}
                        class="px-2.5 py-1 text-xs bg-primary text-on-primary rounded hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
                      >+ Add as milestone</button>
                      <button
                        type="button"
                        onclick={() => aiCtl.suggest(g)}
                        class="px-2 py-1 text-xs bg-surface1 text-subtext rounded hover:bg-surface2"
                        title="re-roll the suggestion"
                      >↻ Try again</button>
                    {/if}
                    <button
                      type="button"
                      onclick={aiCtl.close}
                      class="ml-auto text-xs text-dim hover:text-text"
                    >Dismiss</button>
                  </div>
                </div>
              {/if}
            {/if}
          </article>
        {/each}
      </div>
    {:else if filterCtl.viewMode === 'list'}
      <!-- Compact list — denser layout for users with many dataCtl.goals.
           Each row: title · status pill · countdown chip · progress
           bar inline. Click anywhere on the row opens the detail
           drawer. Same urgency border-left as cards so the visual
           language stays consistent. Completed/archived rows dim
           to half-opacity so the eye stays on living work. -->
      <div class="bg-surface0 border border-surface1 rounded-lg overflow-hidden divide-y divide-surface1">
        {#each visibleGoals as g (g.id)}
          {@const p = progress(g)}
          {@const sc = statusColor(g.status)}
          {@const tone = goalTargetTone(g.status, g.target_date)}
          {@const chip = targetChip(g.target_date)}
          {@const roll = dataCtl.rollupFor(g)}
          {@const rowDim = g.status === 'completed' ? 'opacity-70' : g.status === 'archived' ? 'opacity-55' : ''}
          <button
            type="button"
            onclick={() => openDetail(g)}
            class="w-full text-left px-3 py-2 flex items-center gap-3 hover:bg-surface1 transition-colors {tone ? 'border-l-4' : 'border-l-4 border-l-transparent'} {rowDim}"
            style={tone ? `border-left-color: var(--color-${tone});` : ''}
          >
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <span class="text-sm font-medium text-text truncate">{@html inlineMd(g.title)}</span>
                {#if g.venture}
                  <span class="text-[10px] text-secondary truncate hidden sm:inline">🏢 {g.venture}</span>
                {/if}
              </div>
              <div class="flex items-center gap-x-3 gap-y-0.5 text-[11px] text-dim flex-wrap">
                {#if g.target_date}
                  <span class="font-mono tabular-nums">{fmtTargetDate(g.target_date)}</span>
                {/if}
                {#if chip && (g.status === 'active' || g.status === 'paused')}
                  <span
                    class="text-[10px] tabular-nums"
                    style="color: var(--color-{chip.tone});"
                  >{chip.label}</span>
                {/if}
                {#if p.total > 0}
                  <span class="tabular-nums">{p.done}/{p.total} ms</span>
                {/if}
                {#if roll.open > 0}
                  <span class="tabular-nums" title="open tasks linked to this goal">{roll.open} task{roll.open === 1 ? '' : 's'}</span>
                {/if}
                {#if g.category}<span>· {g.category}</span>{/if}
              </div>
            </div>
            {#if p.total > 0}
              <div class="hidden sm:flex flex-col items-end gap-0.5 w-24 flex-shrink-0">
                <div class="text-[10px] text-dim tabular-nums">{p.pct}%</div>
                <div class="h-1 w-full bg-mantle rounded-full overflow-hidden">
                  <div class="h-full bg-primary" style="width: {p.pct}%"></div>
                </div>
              </div>
            {/if}
            <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded {sc.bg} {sc.text} flex-shrink-0 hidden sm:inline">
              {g.status ?? 'active'}
            </span>
          </button>
        {/each}
      </div>
    {:else}
      <!-- Kanban-by-status. Four columns side-by-side on desktop,
           horizontally scrollable on mobile. Each column header
           carries its own count so the user can see the shape of
           their pipeline at a glance. Empty columns render a faint
           "—" placeholder so the column shape stays visible. -->
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
        {#each kanbanColumns as col (col)}
          {@const colGoals = kanbanGroups[col]}
          {@const colColor = statusColor(col)}
          <section class="bg-surface0 border border-surface1 rounded-lg flex flex-col min-h-[120px]">
            <header class="flex items-center justify-between px-3 py-2 border-b border-surface1">
              <div class="flex items-center gap-2">
                <span class="text-[10px] uppercase tracking-wider px-2 py-0.5 rounded {colColor.bg} {colColor.text}">
                  {col}
                </span>
              </div>
              <span class="text-xs text-dim tabular-nums">{colGoals.length}</span>
            </header>
            <div class="flex-1 p-2 space-y-2">
              {#if colGoals.length === 0}
                <div class="text-[11px] text-dim italic text-center py-4">—</div>
              {:else}
                {#each colGoals as g (g.id)}
                  <GoalKanbanCard
                    goal={g}
                    progress={progress(g)}
                    onClick={() => openDetail(g)}
                  />
                {/each}
              {/if}
            </div>
          </section>
        {/each}
      </div>
    {/if}
  </div>
</div>

<GoalCreate bind:open={createOpen} ventures={ventures} onCreated={created} />
<GoalDetail
  bind:open={detailOpen}
  goal={selected}
  onUpdated={() => dataCtl.load()}
  onDeleted={deleted}
  onOpenDashboard={openDashboard}
/>

{#if dashboardOpen && selected}
  <!-- Goal Dashboard overlay — full-screen visual operating
       picture for the focused goal. URL-persisted via
       ?focus=X&dashboard=1 so a reload keeps it open. Sits above
       the dataCtl.goals page chrome (list/cards/kanban + detail drawer)
       without unmounting them, so closing the dashboard lands the
       user back where they came from. -->
  <GoalDashboardPanel goal={selected} onClose={closeDashboard} />
{/if}

<!-- Goal Agent — operates on the filterCtl.filtered list (whatever the
     current status / search / venture / category scope yields).
     parent's dataCtl.load() reconciles every goal page surface. -->
<GoalAgent
  open={agentOpen}
  goals={filterCtl.filtered}
  todayISO={todayISO()}
  knownVentures={ventures}
  onClose={() => (agentOpen = false)}
  onChanged={() => dataCtl.load()}
/>
