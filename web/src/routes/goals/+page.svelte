<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, type Goal, type Project, type Task , todayISO } from '$lib/api';
  import { rafThrottle } from '$lib/util/streamThrottle';
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
  import PageHeader from '$lib/components/PageHeader.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import { daysUntilTarget, targetChip, targetBorderColor } from '$lib/goals/util';
  import { loadStoredString, saveStoredString } from '$lib/util/storage';

  // View modes — `cards` is the rich card layout (the existing UI),
  // `list` is a compact one-line-per-goal table for users with many
  // goals who want density, `kanban` lays goals out in status columns
  // (active / paused / completed / archived) so the user can see the
  // shape of their pipeline at a glance. Persisted in localStorage so
  // the user lands on their preferred mode on every visit.
  type ViewMode = 'cards' | 'list' | 'kanban';
  const VIEW_KEY = 'granit.goals.view';
  let viewMode = $state<ViewMode>(loadStoredString(VIEW_KEY, 'cards') as ViewMode);
  $effect(() => saveStoredString(VIEW_KEY, viewMode));

  let goals = $state<Goal[]>([]);
  // Goal Agent — conversational mutation engine for /goals.
  // Mirrors Task/Project agents; lives on the same audit-gated
  // chatStream pipeline. Operates on the filtered list.
  let agentOpen = $state(false);
  // Linked tasks + projects power the roll-up chips on each goal —
  // "X open tasks · Y projects" so the user can see at a glance
  // which goals have momentum behind them and which are floating
  // dreams. Tasks fetched once with status filter (open + done) so
  // the cards can show "X / Y done" for goal-tagged tasks.
  let openTasks = $state<Task[]>([]);
  let doneTasks = $state<Task[]>([]);
  let projects = $state<Project[]>([]);
  let loading = $state(false);
  // Status values mirror the TUI / internal/goals.Status. The earlier UI
  // rendered a 'done' tab that never matched anything because the TUI
  // writes 'completed'.
  let statusFilter = $state<'all' | 'active' | 'paused' | 'completed' | 'archived'>('all');
  let categoryFilter = $state<string>('');
  let tagFilter = $state<string>('');
  let ventureFilter = $state<string>('');
  let q = $state<string>('');

  let createOpen = $state(false);
  let detailOpen = $state(false);
  let selectedId = $state<string | null>(null);

  // Tracks whether load() has resolved at least once. Drives the
  // skeleton vs empty-state choice — pre-resolution we render
  // shimmer placeholders, post-resolution we render the proper
  // "no goals" empty state. Without this distinction the page
  // briefly flashed "no goals match this filter." on every mount.
  let firstLoaded = $state(false);
  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      // Fetch goals + linked context (tasks + projects) in parallel.
      // The roll-up is purely advisory — failures of the secondary
      // calls shouldn't block the goals list itself, so each is
      // wrapped in its own try and logged-but-ignored on error.
      const [list, openRes, doneRes, projRes] = await Promise.allSettled([
        api.listGoals(),
        api.listTasks({ status: 'open' }),
        api.listTasks({ status: 'done' }),
        api.listProjects()
      ]);
      if (list.status === 'fulfilled') goals = list.value.goals;
      openTasks = openRes.status === 'fulfilled' ? openRes.value.tasks : [];
      doneTasks = doneRes.status === 'fulfilled' ? doneRes.value.tasks : [];
      projects = projRes.status === 'fulfilled' ? projRes.value.projects : [];
    } finally {
      loading = false;
      firstLoaded = true;
    }
  }
  onMount(() => {
    load();
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
      if (ev.type === 'note.changed' || ev.type === 'note.removed') load();
      // Re-fetch when the TUI (or another web tab) writes goals.json.
      // The server broadcasts state.changed with Path=".granit/goals.json".
      if (ev.type === 'state.changed' && ev.path === '.granit/goals.json') load();
    });
    // Visibility-aware refresh: WS connections are suspended when the
    // tab is backgrounded (especially on mobile Safari), so we'd miss
    // any state.changed event fired in that window. Refetching on
    // visibility flip cheaply guarantees the user never returns to a
    // stale list.
    const onVisible = () => {
      if (document.visibilityState === 'visible') load();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', onVisible);
    // 'a' opens the Goal Agent — same hotkey contract as /tasks
    // and /projects. isTypingTarget guard suppresses while the
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
    return () => {
      unsub();
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', onVisible);
      window.removeEventListener('keydown', onKey);
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
    if (goals.length === 0) return; // wait until load() resolves
    const g = goals.find((x) => x.id === focus);
    if (g && selectedId !== focus) {
      selectedId = focus;
      detailOpen = true;
    }
  });

  // Selected goal — derived from id so live edits during a refetch find
  // the new copy without reopening the drawer at a stale state.
  let selected = $derived(goals.find((g) => g.id === selectedId) ?? null);

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
  }

  let filtered = $derived.by(() => {
    let list = goals;
    if (statusFilter !== 'all') list = list.filter((g) => (g.status ?? 'active') === statusFilter);
    if (categoryFilter) list = list.filter((g) => g.category === categoryFilter);
    if (tagFilter) list = list.filter((g) => (g.tags ?? []).includes(tagFilter));
    if (ventureFilter) list = list.filter((g) => (g.venture ?? '') === ventureFilter);
    const term = q.trim().toLowerCase();
    if (term) {
      list = list.filter((g) =>
        g.title.toLowerCase().includes(term) ||
        (g.description ?? '').toLowerCase().includes(term) ||
        (g.notes ?? '').toLowerCase().includes(term) ||
        (g.venture ?? '').toLowerCase().includes(term)
      );
    }
    return list;
  });

  function progress(g: Goal): { done: number; total: number; pct: number } {
    const ms = g.milestones ?? [];
    const total = ms.length;
    if (total === 0) return { done: 0, total: 0, pct: g.status === 'completed' ? 100 : 0 };
    const done = ms.filter((m) => m.done).length;
    return { done, total, pct: Math.round((done / total) * 100) };
  }

  // Per-goal index of linked tasks (by goalId) + linked projects (by
  // matching goal.project against project.name, since the schema is
  // free-text not FK). Computed in a single pass over each list so
  // the per-goal lookups in the render loop stay O(1). Used by the
  // cards view to surface a "5 open · 2 done · 1 project" chip row
  // — the user's signal for which goals actually have execution
  // behind them and which are still abstractions.
  let rollups = $derived.by(() => {
    const byGoalOpen = new Map<string, number>();
    const byGoalDone = new Map<string, number>();
    for (const t of openTasks) {
      if (!t.goalId) continue;
      byGoalOpen.set(t.goalId, (byGoalOpen.get(t.goalId) ?? 0) + 1);
    }
    for (const t of doneTasks) {
      if (!t.goalId) continue;
      byGoalDone.set(t.goalId, (byGoalDone.get(t.goalId) ?? 0) + 1);
    }
    // Projects index by name (lowercased) so a goal.project field
    // matches case-insensitively. We only need a presence check +
    // the matched project's `progress` field for surface-level
    // context, so the value is the project itself.
    const projByName = new Map<string, Project>();
    for (const p of projects) projByName.set(p.name.toLowerCase(), p);
    return { byGoalOpen, byGoalDone, projByName };
  });

  function rollupFor(g: Goal): { open: number; done: number; project: Project | null } {
    const open = rollups.byGoalOpen.get(g.id) ?? 0;
    const done = rollups.byGoalDone.get(g.id) ?? 0;
    const project = g.project ? rollups.projByName.get(g.project.toLowerCase()) ?? null : null;
    return { open, done, project };
  }

  // Status-pill colors. Spec: active=primary, paused=subtext, completed=success, archived=dim.
  function statusColor(s?: string): { bg: string; text: string } {
    switch (s) {
      case 'active':
        return { bg: 'bg-primary/15', text: 'text-primary' };
      case 'paused':
        return { bg: 'bg-surface1', text: 'text-subtext' };
      case 'completed':
        return { bg: 'bg-surface0', text: 'text-success' };
      case 'archived':
        return { bg: 'bg-surface1', text: 'text-dim' };
      default:
        return { bg: 'bg-surface1', text: 'text-subtext' };
    }
  }

  function fmtDate(s: string | undefined): string {
    if (!s) return '';
    const d = new Date(s);
    if (!isNaN(d.getTime())) {
      return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
    }
    return s;
  }

  // Status-aware urgency tone for the goal card's left border.
  // Completed + archived goals stay neutral so a past-target completed
  // goal doesn't shout — its target_date is now historical context,
  // not a call to action. Only `active` and `paused` goals get the
  // urgency treatment so the user's eye lands on living work.
  // The proximity → tone mapping itself lives in $lib/goals/util so
  // /goals and the dashboard widget can't drift.
  function targetTone(g: Goal): string | null {
    if (g.status === 'completed' || g.status === 'archived') return null;
    const days = daysUntilTarget(g.target_date);
    if (days === null) return null;
    if (days < 0) return 'error';
    if (days <= 30) return 'warning';
    if (days <= 90) return 'info';
    return null;
  }

  let counts = $derived({
    all: goals.length,
    active: goals.filter((g) => (g.status ?? 'active') === 'active').length,
    paused: goals.filter((g) => g.status === 'paused').length,
    completed: goals.filter((g) => g.status === 'completed').length,
    archived: goals.filter((g) => g.status === 'archived').length
  });

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
  function staleness(iso: string | undefined): number {
    if (!iso) return Number.POSITIVE_INFINITY;
    const t = Date.parse(iso);
    if (Number.isNaN(t)) return Number.POSITIVE_INFINITY;
    return Math.floor((Date.now() - t) / 86_400_000);
  }
  function recentCompletionForGoal(goalId: string): boolean {
    const fortnight = Date.now() - 14 * 86_400_000;
    for (const t of doneTasks) {
      if (t.goalId !== goalId) continue;
      const ts = Date.parse(t.updatedAt ?? t.createdAt ?? '');
      if (!Number.isNaN(ts) && ts >= fortnight) return true;
    }
    return false;
  }
  let stalledGoals = $derived.by(() =>
    goals.filter((g) => {
      const status = g.status ?? 'active';
      if (status !== 'active') return false;
      const days = staleness(g.updated_at);
      if (days < 30) return false;
      return !recentCompletionForGoal(g.id);
    })
  );
  // When the user clicks the banner action, we filter the list
  // down to just the stalled rows. Re-derive over `filtered` rather
  // than mutating filters so the existing search/category/tag
  // pickers aren't disturbed.
  let stalledFilterOn = $state(false);
  let visibleGoals = $derived.by(() => {
    if (!stalledFilterOn) return filtered;
    const stalledIds = new Set(stalledGoals.map((g) => g.id));
    return filtered.filter((g) => stalledIds.has(g.id));
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
    for (const g of filtered) {
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
  // goals whose status excludes them from the urgency treatment
  // (completed / archived) and skips free-text target_dates that
  // can't be compared to "today". Falls back to null when the user
  // has no dated goals — the hero card simply doesn't render.
  let goalHero = $derived.by((): { goal: Goal; days: number } | null => {
    let best: Goal | null = null;
    let bestDays = Infinity;
    for (const g of goals) {
      const status = g.status ?? 'active';
      if (status !== 'active' && status !== 'paused') continue;
      const days = daysUntilTarget(g.target_date);
      if (days === null) continue;
      // Earliest target wins; overdue goals (negative days) sort
      // ahead of upcoming ones because they need attention more.
      if (days < bestDays) {
        bestDays = days;
        best = g;
      }
    }
    return best ? { goal: best, days: bestDays } : null;
  });

  // ----- Target-proximity stat strip -----
  // Distribution of dated active+paused goals across urgency
  // buckets, surfaced as a one-line summary below the status tabs.
  // Complements the hero (single most-pressing goal) by showing the
  // shape of the whole pipeline at a glance. Free-text target_dates
  // and undated goals are excluded — they have no place on a
  // proximity axis.
  let targetStats = $derived.by(() => {
    let pastTarget = 0, thisMonth = 0, thisQuarter = 0, later = 0;
    for (const g of goals) {
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
    for (const g of goals) {
      const c = (g.category ?? '').trim();
      if (!c) continue;
      m.set(c, (m.get(c) ?? 0) + 1);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([c]) => c);
  });
  let tags = $derived.by(() => {
    const m = new Map<string, number>();
    for (const g of goals) {
      for (const t of g.tags ?? []) m.set(t, (m.get(t) ?? 0) + 1);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([t]) => t);
  });
  let ventures = $derived.by(() => {
    const m = new Map<string, number>();
    for (const g of goals) {
      const v = (g.venture ?? '').trim();
      if (!v) continue;
      m.set(v, (m.get(v) ?? 0) + 1);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([v]) => v);
  });

  async function created(g: Goal) {
    // Optimistic prepend so the new goal renders immediately. The
    // load() below reconciles with the server (auth-stamped CreatedAt,
    // any defaults the server filled in).
    if (!goals.some((x) => x.id === g.id)) {
      goals = [g, ...goals];
    }
    selectedId = g.id;
    detailOpen = true;
    await load();
  }

  async function deleted(_id: string) {
    detailOpen = false;
    selectedId = null;
    await load();
    toast.success('goal deleted');
  }

  // ----- AI "next milestone" suggester -----
  // Per-goal helper that asks the model for the next concrete
  // milestone given the goal's title, description, target_date,
  // existing milestones, and roll-up context. Routed through
  // chatStream → /chat/stream so the audit / Sabbath / redaction /
  // cost guards apply uniformly with every other AI feature in
  // Granit. The page tracks a single in-flight suggestion at a
  // time (only one card can be expanded), keyed by goalId.
  //
  // Output shape we ask the model for: a single concise milestone
  // text (one line, action-oriented). The user can then click
  // "Add as milestone" — which calls api.addGoalMilestone — or
  // dismiss + retry. Streamed so tokens render progressively.
  let aiGoalId = $state<string | null>(null);
  let aiText = $state<string>('');
  let aiBusy = $state(false);
  let aiError = $state<string>('');
  let aiAbort: AbortController | null = null;

  function aiClose() {
    aiAbort?.abort();
    aiAbort = null;
    aiGoalId = null;
    aiText = '';
    aiError = '';
    aiBusy = false;
  }

  // Toggle handler used by the inline ✨ button — opens the panel
  // for a fresh goal, closes it when clicked twice on the same goal.
  function aiToggle(g: Goal) {
    if (aiGoalId === g.id) {
      aiClose();
      return;
    }
    aiSuggest(g);
  }

  async function aiSuggest(g: Goal) {
    // Always (re-)run — the "Try again" button calls this directly
    // for the currently-open goal. The toggle behaviour lives in
    // aiToggle so re-rolling stays a single click.
    aiAbort?.abort();
    aiAbort = null;
    aiGoalId = g.id;
    aiBusy = true;
    aiError = '';
    aiText = '';
    aiAbort = new AbortController();

    const ms = g.milestones ?? [];
    const open = ms.filter((m) => !m.done).map((m) => m.text);
    const done = ms.filter((m) => m.done).map((m) => m.text);
    const roll = rollupFor(g);
    // Compose a structured context block — keep it under ~2KB so
    // the prompt cost stays predictable. Only fields with content
    // are emitted, so a sparse goal yields a sparse prompt.
    const ctx = [
      `Goal: ${g.title}`,
      g.description ? `Description: ${g.description}` : '',
      g.target_date ? `Target date: ${g.target_date}` : '',
      g.venture ? `Venture: ${g.venture}` : '',
      g.project ? `Project: ${g.project}` : '',
      g.category ? `Category: ${g.category}` : '',
      open.length > 0 ? `Open milestones:\n${open.map((m) => `- ${m}`).join('\n')}` : '',
      done.length > 0 ? `Completed milestones:\n${done.map((m) => `- ${m}`).join('\n')}` : '',
      roll.open + roll.done > 0
        ? `Linked tasks: ${roll.open} open, ${roll.done} done`
        : ''
    ].filter(Boolean).join('\n\n');

    const userMessage =
      'Propose ONE concrete next milestone for this goal. Rules:\n' +
      '- One line, max ~12 words.\n' +
      '- Action-oriented, starts with a verb (Draft, Ship, Interview, Outline, …).\n' +
      "- Specific enough to know when it's done.\n" +
      '- Must move the goal forward from where it stands now (avoid restating done milestones).\n' +
      '- Output the milestone text only — no preamble, no quotes, no bullet, no period.\n\n' +
      'Goal context:\n\n' + ctx;

    // rAF throttle — aiText is rendered live.
    const goalT = rafThrottle((full) => { aiText = full; });
    try {
      await api.chatStream(
        [{ role: 'user', content: userMessage }],
        undefined,
        {
          onChunk: goalT.onChunk,
          onDone: () => {
            goalT.flush();
            aiBusy = false;
            aiAbort = null;
            // Trim once at end so the streaming UI shows tokens
            // exactly as they arrive but the final value is clean.
            aiText = aiText.trim().replace(/^["'\-•*]+\s*/, '').replace(/\.\s*$/, '');
            if (!aiText) aiError = 'AI returned an empty suggestion.';
          },
          onError: (err) => {
            goalT.flush();
            aiBusy = false;
            aiAbort = null;
            aiError = err.message;
          }
        },
        aiAbort.signal
      );
    } catch (e) {
      aiBusy = false;
      aiAbort = null;
      aiError = errorMessage(e);
    }
  }

  async function aiAdoptAsMilestone(g: Goal) {
    const text = aiText.trim();
    if (!text) return;
    try {
      await api.addGoalMilestone(g.id, { text });
      toast.success('milestone added');
      aiClose();
      await load();
    } catch (err) {
      toast.error('Add failed: ' + (errorMessage(err)));
    }
  }

  // ─────────────────────────────────────────────────────────────────
  // AI "Weekly check-in" — page-level coaching
  // ─────────────────────────────────────────────────────────────────
  // Walks every active + paused goal and asks the model for a sharp
  // one-line verdict (on-track / drifting / dead) plus one specific
  // question the user could sit with this week. Streams back as a
  // single JSON array so we can render it as a checklist of cards
  // and let the user save individual entries to today's jot or the
  // whole batch as a single "Weekly check-in" block.
  //
  // The system prompt borrows from the Coach (Socrates) + Founder
  // personas — questions over answers, but operator-grade specifics,
  // refusing generic praise. The user message embeds a compact view
  // of each goal's milestones, target_date urgency, and linked-task
  // velocity (open + 4-week done count) so the model has enough to
  // tell drifting from healthy.
  interface CheckinEntry {
    id: string;            // matches Goal.id when the model gets it right
    title: string;         // echoed goal title for safety in the render
    verdict: 'on-track' | 'drifting' | 'dead' | string;
    question: string;
  }
  let checkinOpen = $state(false);
  let checkinBusy = $state(false);
  let checkinError = $state('');
  let checkinEntries = $state<CheckinEntry[]>([]);
  let checkinAbort: AbortController | null = null;
  // Set of goal ids the user has already saved or dismissed in this
  // session — keeps the buttons honest if the user clicks "save" then
  // changes their mind ("dismiss" hides the row).
  let checkinHidden = $state<Set<string>>(new Set());

  function checkinClose() {
    checkinAbort?.abort();
    checkinAbort = null;
    checkinOpen = false;
    checkinBusy = false;
    checkinError = '';
    checkinEntries = [];
    checkinHidden = new Set();
  }

  // Live-goal slice the AI sees — single source of truth so the
  // "AI saw" disclosure below the panel matches the prompt exactly.
  let checkinScope = $derived.by(() =>
    goals.filter((g) => {
      const s = g.status ?? 'active';
      return s === 'active' || s === 'paused';
    })
  );

  // Per-goal velocity in the last 4 weeks — used in the prompt so
  // the model can spot "0 done in 4w" drift without us telling it
  // what to think. Same goalId-on-task indexing the page already
  // uses for the rollup chips, but bucketed by completedAt.
  function recentDoneFor(g: Goal): number {
    const cutoff = Date.now() - 28 * 24 * 3600 * 1000;
    let n = 0;
    for (const t of doneTasks) {
      if (t.goalId !== g.id || !t.completedAt) continue;
      const d = new Date(t.completedAt).getTime();
      if (!Number.isFinite(d)) continue;
      if (d >= cutoff) n++;
    }
    return n;
  }

  async function runCheckin() {
    if (checkinBusy) return;
    if (checkinScope.length === 0) {
      toast.error('No active or paused goals to check in on.');
      return;
    }
    checkinAbort?.abort();
    checkinAbort = new AbortController();
    checkinOpen = true;
    checkinBusy = true;
    checkinError = '';
    checkinEntries = [];
    checkinHidden = new Set();

    // Compact per-goal block. Cap milestone lists at ~6 each so a
    // goal with a long history doesn't dominate the prompt budget.
    const blocks: string[] = [];
    for (const g of checkinScope) {
      const ms = g.milestones ?? [];
      const open = ms.filter((m) => !m.done).slice(0, 6).map((m) => m.text);
      const done = ms.filter((m) => m.done).slice(-6).map((m) => m.text);
      const days = daysUntilTarget(g.target_date);
      const roll = rollupFor(g);
      const recent = recentDoneFor(g);
      const lines: string[] = [
        `[id: ${g.id}] ${g.title}`,
        `status: ${g.status ?? 'active'}`,
      ];
      if (g.target_date) {
        const urgency =
          days === null ? 'no-date'
          : days < 0 ? `${Math.abs(days)}d past target`
          : days <= 7 ? `${days}d left`
          : days <= 30 ? `${days}d left (this month)`
          : days <= 90 ? `${days}d left (this quarter)`
          : `${days}d left`;
        lines.push(`target: ${g.target_date} — ${urgency}`);
      }
      if (g.venture) lines.push(`venture: ${g.venture}`);
      if (g.project) lines.push(`project: ${g.project}`);
      lines.push(`milestones: ${done.length} done, ${open.length} open`);
      if (open.length > 0) lines.push(`open milestones: ${open.map((m) => `"${m}"`).join('; ')}`);
      if (done.length > 0) lines.push(`recent done: ${done.map((m) => `"${m}"`).join('; ')}`);
      lines.push(`linked tasks: ${roll.open} open, ${roll.done} done lifetime, ${recent} done in last 4w`);
      blocks.push(lines.join('\n'));
    }

    const userMessage =
      'You are a Socratic coach with operator-grade specificity.\n' +
      'Refuse generic praise. The user can already pat themselves on the back; your job is to add what they would not naturally see.\n\n' +
      'For EACH goal below, return a one-sentence honest verdict and ONE specific question worth sitting with this week.\n\n' +
      'Verdict rules:\n' +
      '- "on-track" — momentum is real, recent done > 0 OR milestones are closing in cadence with the target_date\n' +
      '- "drifting" — talked about, not moved on; few/no done milestones or tasks in last 4w; target_date pressure rising\n' +
      '- "dead" — past target with no progress, OR open milestones haven\'t been touched and the user has clearly shifted\n' +
      'Be willing to call drifting "drifting". The user wants honesty over kindness.\n\n' +
      'Question rules:\n' +
      '- Specific to THIS goal, not a generic "what is your why".\n' +
      '- Pry at an assumption, a contradiction, or a missing definition the user is probably carrying.\n' +
      '- Sometimes uncomfortable. Always kind.\n' +
      '- One question. No multi-part. No "What if you tried…" — questions, not advice.\n\n' +
      'Return STRICT JSON ONLY (no markdown fences, no preamble), shape:\n' +
      '[{"id": "<echo the goal id>", "title": "<echo the goal title>", "verdict": "on-track|drifting|dead", "question": "..."}, ...]\n' +
      'Order matches input order. Do NOT skip any goal.\n\n' +
      'Goals:\n\n' + blocks.join('\n\n---\n\n');

    let acc = '';
    try {
      await api.chatStream(
        [{ role: 'user', content: userMessage }],
        undefined,
        {
          onChunk: (c) => { acc += c; },
          onDone: () => {
            checkinBusy = false;
            checkinAbort = null;
            // Strip optional code fences and parse.
            let cleaned = acc.trim();
            if (cleaned.startsWith('```')) {
              cleaned = cleaned.replace(/^```(?:json)?\s*/, '').replace(/```\s*$/, '').trim();
            }
            try {
              const parsed = JSON.parse(cleaned);
              if (!Array.isArray(parsed)) throw new Error('expected array');
              checkinEntries = parsed
                .filter((p: unknown) => p && typeof p === 'object')
                .map((p) => p as CheckinEntry)
                .filter((p) => typeof p.question === 'string' && typeof p.verdict === 'string');
              if (checkinEntries.length === 0) {
                checkinError = 'AI returned no entries.';
              }
            } catch (err) {
              checkinError = 'Couldn\'t parse check-in: ' + (errorMessage(err));
            }
          },
          onError: (err) => {
            checkinBusy = false;
            checkinAbort = null;
            checkinError = err.message;
          }
        },
        checkinAbort.signal
      );
    } catch (e) {
      checkinBusy = false;
      checkinAbort = null;
      checkinError = errorMessage(e);
    }
  }

  // Compose the markdown block that will be appended to today's jot
  // (or saved alongside, depending on which button the user clicked).
  // Visible only entries (not dismissed) make it in — that's the
  // user's curation step before persistence.
  function checkinAsMarkdown(entries: CheckinEntry[]): string {
    const today = todayISO();
    const lines: string[] = [`\n\n## Weekly goal check-in — ${today}\n`];
    for (const e of entries) {
      const verdictTag = e.verdict === 'on-track' ? 'ON-TRACK'
        : e.verdict === 'drifting' ? 'DRIFTING'
        : e.verdict === 'dead' ? 'DEAD'
        : e.verdict.toUpperCase();
      lines.push(`### ${e.title} — _${verdictTag}_`);
      lines.push(`> ${e.question}`);
      lines.push('');
    }
    return lines.join('\n');
  }

  async function checkinSaveOne(e: CheckinEntry) {
    try {
      const note = await api.daily('today');
      const block = checkinAsMarkdown([e]);
      const next = (note.body ?? '') + block;
      await api.putNote(note.path, { frontmatter: note.frontmatter, body: next }, undefined);
      checkinHidden = new Set([...checkinHidden, e.id]);
      toast.success('saved to today\'s jot');
    } catch (err) {
      toast.error('save failed: ' + (errorMessage(err)));
    }
  }
  async function checkinSaveAll() {
    const visible = checkinEntries.filter((e) => !checkinHidden.has(e.id));
    if (visible.length === 0) return;
    try {
      const note = await api.daily('today');
      const block = checkinAsMarkdown(visible);
      const next = (note.body ?? '') + block;
      await api.putNote(note.path, { frontmatter: note.frontmatter, body: next }, undefined);
      toast.success(`saved ${visible.length} entr${visible.length === 1 ? 'y' : 'ies'} to today's jot`);
      checkinClose();
    } catch (err) {
      toast.error('save failed: ' + (errorMessage(err)));
    }
  }
  function checkinDismiss(e: CheckinEntry) {
    checkinHidden = new Set([...checkinHidden, e.id]);
  }

  // ─────────────────────────────────────────────────────────────────
  // AI "Goal alignment audit" — strategy/execution drift detector
  // ─────────────────────────────────────────────────────────────────
  // Reads all active goals + open tasks + recently-completed tasks
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
  interface AuditFinding {
    cluster: string;        // human label, e.g. "support / maintenance"
    count: number;          // tasks in this cluster
    sample: string[];       // up to 3 example task texts
    observation: string;    // one sentence — what's happening
    question: string;       // one question — was it intentional?
  }
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
    const orphanOpen = openTasks
      .filter((t) => !t.goalId && (t.text ?? '').trim().length > 0)
      .slice(0, 80);
    const orphanDoneRecent = doneTasks
      .filter((t) => {
        if (t.goalId) return false;
        if (!t.completedAt) return false;
        const d = new Date(t.completedAt).getTime();
        return Number.isFinite(d) && d >= cutoff;
      })
      .slice(0, 80);
    const linkedOpen = openTasks.filter((t) => t.goalId).length;
    const linkedDone14 = doneTasks.filter((t) => {
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
    const activeGoals = goals.filter((g) => (g.status ?? 'active') === 'active');
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
</script>

<div class="h-full overflow-y-auto">
  <!-- Container widens in kanban mode so the four columns have room
       to breathe. Cards / list stay at the original 4xl reading width
       so long titles remain comfortable. -->
  <div class="p-4 sm:p-6 lg:p-8 mx-auto {viewMode === 'kanban' ? 'max-w-7xl' : 'max-w-4xl'}">
    <VisionContextStrip />
    <PageHeader
      title="Goals"
      subtitle="{goals.length} {goals.length === 1 ? 'goal' : 'goals'} · the things you're committing to in this season"
    >
      {#snippet actions()}
        <button
          type="button"
          onclick={() => { if (checkinOpen) checkinClose(); else void runCheckin(); }}
          disabled={checkinBusy}
          class="px-3 py-1.5 border border-surface1 text-subtext rounded text-sm hover:border-primary hover:text-primary disabled:opacity-50"
          title="Honest one-line verdict + a sharp question for each active goal"
        >{checkinOpen ? '✕ check-in' : '✨ Weekly check-in'}</button>
        <button
          type="button"
          onclick={() => { if (auditOpen) auditClose(); else void runAudit(); }}
          disabled={auditBusy}
          class="px-3 py-1.5 border border-surface1 text-subtext rounded text-sm hover:border-warning hover:text-warning disabled:opacity-50"
          title="Surface tasks that don't advance any stated goal"
        >{auditOpen ? '✕ audit' : '🔍 Alignment audit'}</button>
        <!-- Goal Agent button removed; launches from the chat
             sidebar via ?agent=1. -->
        <button
          onclick={() => (createOpen = true)}
          class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90"
        >+ New goal</button>
      {/snippet}
    </PageHeader>

    <!-- Weekly check-in panel — page-level coaching surface that
         reads every active + paused goal and proposes a one-line
         verdict + one sharp question per goal. Streams as JSON; once
         parsed, the user can save individual rows or the full batch
         to today's jot, or just dismiss. -->
    {#if checkinOpen}
      <section class="mb-5 bg-surface0 border border-surface1 rounded-lg overflow-hidden">
        <header class="px-4 py-2.5 border-b border-surface1 flex items-center gap-2">
          <span class="text-sm font-medium text-text">Weekly check-in</span>
          <span class="text-[11px] text-dim">{checkinScope.length} active/paused goal{checkinScope.length === 1 ? '' : 's'}</span>
          <span class="flex-1"></span>
          {#if checkinBusy}
            <button
              type="button"
              onclick={() => checkinAbort?.abort()}
              class="px-2 py-1 text-xs bg-surface1 text-subtext rounded hover:bg-surface2"
            >Stop</button>
          {:else}
            {#if checkinEntries.length > 0 && checkinEntries.some((e) => !checkinHidden.has(e.id))}
              <button
                type="button"
                onclick={checkinSaveAll}
                class="px-2.5 py-1 text-xs bg-primary text-on-primary rounded hover:opacity-90"
                title="Append all visible entries to today's jot"
              >Save all to jot</button>
            {/if}
            <button
              type="button"
              onclick={() => void runCheckin()}
              class="px-2 py-1 text-xs bg-surface1 text-subtext rounded hover:bg-surface2"
              title="re-roll the check-in"
            >↻ retry</button>
          {/if}
          <button
            type="button"
            onclick={checkinClose}
            class="text-xs text-dim hover:text-text px-1"
          >Dismiss</button>
        </header>

        {#if checkinError}
          <div class="px-4 py-2 text-xs text-error bg-surface0 border-b border-error">{checkinError}</div>
        {/if}

        <div class="p-3 space-y-2">
          {#if checkinBusy && checkinEntries.length === 0}
            <div class="text-xs text-dim italic px-2 py-3 flex items-center gap-2">
              <span class="inline-block w-1.5 h-3 bg-primary/60 animate-pulse rounded-sm"></span>
              reading {checkinScope.length} goals — verdict + question coming…
            </div>
          {:else if checkinEntries.length === 0 && !checkinError}
            <div class="text-xs text-dim italic px-2 py-3">No entries yet.</div>
          {:else}
            {#each checkinEntries as e (e.id + e.question)}
              {#if !checkinHidden.has(e.id)}
                {@const verdictTone = e.verdict === 'on-track' ? 'success'
                  : e.verdict === 'drifting' ? 'warning'
                  : e.verdict === 'dead' ? 'error'
                  : 'subtext'}
                <article class="p-3 bg-mantle border border-surface1 rounded">
                  <div class="flex items-baseline gap-2 mb-1">
                    <span class="text-sm font-medium text-text flex-1 min-w-0 break-words">{e.title}</span>
                    <span
                      class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium tabular-nums whitespace-nowrap"
                      style="background: color-mix(in srgb, var(--color-{verdictTone}) 14%, transparent); color: var(--color-{verdictTone});"
                    >{e.verdict}</span>
                  </div>
                  <p class="text-sm text-subtext italic leading-snug">"{e.question}"</p>
                  <div class="flex items-center gap-2 mt-2 text-[11px]">
                    <button
                      type="button"
                      onclick={() => void checkinSaveOne(e)}
                      class="px-2 py-0.5 bg-surface1 text-primary rounded hover:bg-primary hover:text-on-primary"
                      title="Append this entry to today's jot"
                    >Save to jot</button>
                    <button
                      type="button"
                      onclick={() => checkinDismiss(e)}
                      class="text-dim hover:text-text"
                    >Dismiss</button>
                    {#if goals.some((g) => g.id === e.id)}
                      <button
                        type="button"
                        onclick={() => { const g = goals.find((x) => x.id === e.id); if (g) openDetail(g); }}
                        class="ml-auto text-secondary hover:underline"
                      >Open goal →</button>
                    {/if}
                  </div>
                </article>
              {/if}
            {/each}
          {/if}
        </div>

        <!-- "What the AI saw" disclosure — keeps the design rule
             that the user can always inspect the context. -->
        {#if !checkinBusy && checkinEntries.length > 0}
          <details class="px-4 py-2 border-t border-surface1 text-[11px] text-dim">
            <summary class="cursor-pointer hover:text-text">What the AI saw</summary>
            <ul class="mt-1.5 space-y-0.5 list-disc list-inside">
              {#each checkinScope as g (g.id)}
                {@const days = daysUntilTarget(g.target_date)}
                {@const roll = rollupFor(g)}
                {@const recent = recentDoneFor(g)}
                <li>
                  <span class="text-subtext">{g.title}</span>
                  · {g.status ?? 'active'}
                  {#if days !== null}· {days < 0 ? `${Math.abs(days)}d past` : `${days}d left`}{/if}
                  · {(g.milestones ?? []).filter((m) => m.done).length}/{(g.milestones ?? []).length} ms
                  · {roll.open}↗ tasks · {recent} done in 4w
                </li>
              {/each}
            </ul>
          </details>
        {/if}
      </section>
    {/if}

    <!-- Goal alignment audit panel — clusters of unlinked tasks that
         aren't advancing any active goal. Each finding renders as a
         tinted card with cluster label, count, sample tasks, an
         observation and an "intentional?" question. The user can
         dismiss findings, jump to /tasks to re-link, or close the
         whole panel. -->
    {#if auditOpen}
      <section class="mb-5 bg-surface0 border border-surface1 rounded-lg overflow-hidden">
        <header class="px-4 py-2.5 border-b border-surface1 flex items-center gap-2 flex-wrap">
          <span class="text-sm font-medium text-text">Alignment audit</span>
          <span class="text-[11px] text-dim">
            {auditScope.orphanOpen.length} unlinked open ·
            {auditScope.orphanDoneRecent.length} unlinked done in 14d ·
            {auditScope.linkedOpen + auditScope.linkedDone14} linked
          </span>
          <span class="flex-1"></span>
          {#if auditBusy}
            <button
              type="button"
              onclick={() => auditAbort?.abort()}
              class="px-2 py-1 text-xs bg-surface1 text-subtext rounded hover:bg-surface2"
            >Stop</button>
          {:else}
            <button
              type="button"
              onclick={() => void runAudit()}
              class="px-2 py-1 text-xs bg-surface1 text-subtext rounded hover:bg-surface2"
              title="re-roll the audit"
            >↻ retry</button>
          {/if}
          <a
            href="/tasks"
            class="text-xs text-secondary hover:underline"
            title="Open /tasks to re-link work to a goal"
          >Open tasks →</a>
          <button
            type="button"
            onclick={auditClose}
            class="text-xs text-dim hover:text-text px-1"
          >Dismiss</button>
        </header>

        {#if auditError}
          <div class="px-4 py-2 text-xs text-error bg-surface0 border-b border-error">{auditError}</div>
        {/if}

        <div class="p-3 space-y-2">
          {#if auditBusy && auditFindings.length === 0}
            <div class="text-xs text-dim italic px-2 py-3 flex items-center gap-2">
              <span class="inline-block w-1.5 h-3 bg-surface0 animate-pulse rounded-sm"></span>
              clustering {auditScope.orphanOpen.length + auditScope.orphanDoneRecent.length} unlinked tasks against {goals.filter((g) => (g.status ?? 'active') === 'active').length} active goals…
            </div>
          {:else if auditFindings.length === 0 && !auditError}
            <div class="text-xs text-dim italic px-2 py-3">No findings yet.</div>
          {:else}
            {#each auditFindings as f (f.cluster + f.observation)}
              {#if !auditDismissed.has(f.cluster)}
                <article class="p-3 bg-mantle border border-surface1 rounded border-l-4" style="border-left-color: var(--color-warning);">
                  <div class="flex items-baseline gap-2 mb-1 flex-wrap">
                    <span class="text-sm font-medium text-text flex-1 min-w-0 break-words">{f.cluster}</span>
                    <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-surface0 text-warning tabular-nums">
                      {f.count} task{f.count === 1 ? '' : 's'}
                    </span>
                  </div>
                  <p class="text-sm text-subtext leading-snug">{f.observation}</p>
                  {#if f.sample.length > 0}
                    <ul class="mt-1.5 space-y-0.5 text-[11px] text-dim font-mono">
                      {#each f.sample as s}
                        <li class="truncate">— {s}</li>
                      {/each}
                    </ul>
                  {/if}
                  <p class="text-sm text-text italic mt-2">"{f.question}"</p>
                  <div class="flex items-center gap-2 mt-2 text-[11px]">
                    <button
                      type="button"
                      onclick={() => auditDismiss(f)}
                      class="px-2 py-0.5 bg-surface1 text-subtext rounded hover:bg-surface2"
                      title="It was intentional / I've heard you. Hide this finding."
                    >Intentional</button>
                    <a
                      href="/tasks"
                      class="ml-auto text-secondary hover:underline"
                    >Re-link in /tasks →</a>
                  </div>
                </article>
              {/if}
            {/each}
          {/if}
        </div>
      </section>
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
              <span class="font-mono tabular-nums text-subtext">{fmtDate(h.target_date)}</span>
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

    <!-- Status tabs + view-mode toggle. The view toggle is hidden in
         the kanban layout (kanban already implies "by status" so the
         status filter is decorative there — kept anyway so a user
         narrowing to "active only" sees just one column without
         switching back to cards). -->
    <div class="flex flex-wrap items-center gap-2 mb-3">
      <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm self-start flex-wrap">
        {#each ['all', 'active', 'paused', 'completed', 'archived'] as s}
          <button
            class="px-3 py-1.5 capitalize {statusFilter === s ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (statusFilter = s as typeof statusFilter)}
          >
            {s} <span class="text-xs opacity-70">{counts[s as keyof typeof counts]}</span>
          </button>
        {/each}
      </div>
      <div class="ml-auto flex bg-surface0 border border-surface1 rounded overflow-hidden text-xs">
        <button
          class="px-2.5 py-1.5 inline-flex items-center gap-1 {viewMode === 'cards' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (viewMode = 'cards')}
          title="rich card layout"
          aria-label="cards view"
          aria-pressed={viewMode === 'cards'}
        >
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="3" y="4" width="18" height="6" rx="1.5" />
            <rect x="3" y="14" width="18" height="6" rx="1.5" />
          </svg>
          Cards
        </button>
        <button
          class="px-2.5 py-1.5 inline-flex items-center gap-1 {viewMode === 'list' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (viewMode = 'list')}
          title="compact list — denser, one row per goal"
          aria-label="list view"
          aria-pressed={viewMode === 'list'}
        >
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <path d="M3 6h18" /><path d="M3 12h18" /><path d="M3 18h18" />
          </svg>
          List
        </button>
        <button
          class="px-2.5 py-1.5 inline-flex items-center gap-1 {viewMode === 'kanban' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
          onclick={() => (viewMode = 'kanban')}
          title="kanban — columns by status"
          aria-label="kanban view"
          aria-pressed={viewMode === 'kanban'}
        >
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="3" y="4" width="5" height="16" rx="1" />
            <rect x="9.5" y="4" width="5" height="11" rx="1" />
            <rect x="16" y="4" width="5" height="14" rx="1" />
          </svg>
          Kanban
        </button>
      </div>
    </div>

    <!-- Target-proximity stat strip — complements the hero card by
         showing the distribution of dated goals across urgency
         buckets. Hidden when no dated goals exist (the strip would
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

    <!-- Search + filter chips -->
    <div class="mb-4 space-y-2">
      <input
        bind:value={q}
        placeholder="search title, description, notes…"
        class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      {#if categories.length > 0 || tags.length > 0 || ventures.length > 0}
        <div class="flex flex-wrap items-center gap-1.5 text-xs">
          {#if categoryFilter || tagFilter || ventureFilter}
            <button
              onclick={() => { categoryFilter = ''; tagFilter = ''; ventureFilter = ''; }}
              class="px-2 py-0.5 bg-surface1 text-dim rounded hover:text-text"
            >clear filters</button>
          {/if}
          {#each ventures as v}
            <button
              onclick={() => (ventureFilter = ventureFilter === v ? '' : v)}
              class="px-2 py-0.5 rounded {ventureFilter === v ? 'bg-secondary text-on-primary' : 'bg-surface0 text-secondary hover:bg-surface1'}"
              title="filter to this venture"
            >🏢 {v}</button>
          {/each}
          {#each categories as c}
            <button
              onclick={() => (categoryFilter = categoryFilter === c ? '' : c)}
              class="px-2 py-0.5 rounded {categoryFilter === c ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
            >{c}</button>
          {/each}
          {#each tags as t}
            <button
              onclick={() => (tagFilter = tagFilter === t ? '' : t)}
              class="px-2 py-0.5 rounded {tagFilter === t ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
            >#{t}</button>
          {/each}
        </div>
      {/if}
    </div>

    <!-- Stalled-goal banner — surfaces active goals with no
         metadata edit in 30+ days AND no completed task in 14+
         days. Click to filter the list down to just those rows;
         click again to clear. The banner only renders when at
         least one stalled goal exists, so a healthy goal set
         never sees the nag. -->
    {#if firstLoaded && stalledGoals.length > 0}
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
            {stalledGoals.length}
            {stalledGoals.length === 1 ? 'goal looks' : 'goals look'}
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

    {#if !firstLoaded && loading}
      <!-- Shimmer skeletons match the cards-view rhythm so the
           layout doesn't reflow on first load. Three placeholders is
           a reasonable bet — most users have at least that many
           goals once they're past day-one. -->
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
      <!-- Empty state branches: real "no goals at all" vs "filter
           hides everything". The first nudges the user to create
           their first goal; the second offers a clear-filters
           shortcut so they don't have to hunt for the UI. -->
      {#if goals.length === 0}
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
                statusFilter = 'all';
                categoryFilter = '';
                tagFilter = '';
                ventureFilter = '';
                q = '';
                stalledFilterOn = false;
              }}
              class="px-3 py-1.5 bg-surface1 text-text rounded text-sm hover:bg-surface2"
            >Clear all filters</button>
          {/snippet}
        </EmptyState>
      {/if}
    {:else if viewMode === 'cards'}
      <div class="space-y-4">
        {#each visibleGoals as g (g.id)}
          {@const p = progress(g)}
          {@const sc = statusColor(g.status)}
          {@const tone = targetTone(g)}
          {@const chip = targetChip(g.target_date)}
          {@const roll = rollupFor(g)}
          {@const aiOpen = aiGoalId === g.id}
          <article
            class="bg-surface0 border border-surface1 rounded-lg overflow-hidden hover:border-primary transition-colors {tone ? 'border-l-4' : ''}"
            style={tone ? `border-left-color: var(--color-${tone});` : ''}
          >
            <button
              type="button"
              onclick={() => openDetail(g)}
              class="w-full text-left p-4 flex flex-col gap-2"
            >
              <div class="flex items-start gap-3">
                <div class="flex-1 min-w-0">
                  <h2 class="text-base sm:text-lg font-semibold text-text break-words">{@html inlineMd(g.title)}</h2>
                  {#if g.description}
                    <p class="text-sm text-subtext mt-1 break-words">{@html inlineMd(g.description)}</p>
                  {/if}
                </div>
                <div class="flex flex-col items-end gap-1 flex-shrink-0">
                  <span class="text-[10px] uppercase tracking-wider px-2 py-0.5 rounded {sc.bg} {sc.text}">
                    {g.status ?? 'active'}
                  </span>
                  {#if stalledGoals.some((s) => s.id === g.id)}
                    <span class="text-[10px] uppercase tracking-wider px-2 py-0.5 rounded bg-surface0 text-warning" title="No edits in 30+ days, no recent completed tasks">
                      stalled
                    </span>
                  {/if}
                </div>
              </div>

              <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-dim">
                {#if g.target_date}
                  <span class="inline-flex items-baseline gap-1.5">
                    <span>🎯 {fmtDate(g.target_date)}</span>
                    {#if chip && (g.status === 'active' || g.status === 'paused')}
                      <span
                        class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium tabular-nums whitespace-nowrap"
                        style="background: color-mix(in srgb, var(--color-{chip.tone}) 14%, transparent); color: var(--color-{chip.tone});"
                      >{chip.label}</span>
                    {/if}
                  </span>
                {/if}
                {#if g.project}<span>📁 {g.project}</span>{/if}
                {#if g.venture}<span class="text-secondary">🏢 {g.venture}</span>{/if}
                {#if g.category}<span>· {g.category}</span>{/if}
                {#if p.total > 0}<span>{p.done}/{p.total} milestones</span>{/if}
                {#if g.review_frequency}<span>↻ {g.review_frequency}</span>{/if}
              </div>

              {#if g.tags && g.tags.length > 0}
                <div class="flex flex-wrap gap-1">
                  {#each g.tags as t}
                    <span class="text-[10px] px-1.5 py-0.5 bg-surface1 text-subtext rounded">#{t}</span>
                  {/each}
                </div>
              {/if}

              {#if p.total > 0}
                <div class="mt-1">
                  <div class="h-1.5 bg-mantle rounded-full overflow-hidden">
                    <div class="h-full bg-primary transition-all" style="width: {p.pct}%"></div>
                  </div>
                  <div class="text-[10px] text-dim mt-1">{p.pct}% complete</div>
                </div>
              {/if}

              <!-- Roll-up chips: linked tasks (open + done) and the
                   matched project. Renders nothing when a goal has
                   no execution behind it so the cards for orphan
                   goals don't get noisier. The chips look passive
                   (they live inside the card-wide button) but stay
                   visually distinct so they read as "context"
                   rather than "card body". -->
              {#if roll.open + roll.done > 0 || roll.project}
                <div class="flex flex-wrap items-center gap-1.5 pt-1.5 mt-1 border-t border-surface1 text-[11px]">
                  {#if roll.open > 0}
                    <span class="px-1.5 py-0.5 bg-surface1 text-subtext rounded tabular-nums" title="open tasks linked to this goal">
                      {roll.open} open task{roll.open === 1 ? '' : 's'}
                    </span>
                  {/if}
                  {#if roll.done > 0}
                    <span class="px-1.5 py-0.5 bg-surface0 text-success rounded tabular-nums" title="completed tasks linked to this goal">
                      {roll.done} done
                    </span>
                  {/if}
                  {#if roll.project}
                    <span class="px-1.5 py-0.5 bg-surface1 text-secondary rounded truncate max-w-[14rem]" title="linked project">
                      📁 {roll.project.name}
                      {#if typeof roll.project.progress === 'number'}
                        <span class="opacity-70 tabular-nums ml-0.5">{roll.project.progress}%</span>
                      {/if}
                    </span>
                  {/if}
                </div>
              {/if}
            </button>

            <!-- AI "next milestone" affordance — only on living goals
                 (active / paused). Sits in a footer strip outside the
                 card-wide button so the AI button isn't an
                 accidentally-nested <button>. The inline panel opens
                 below it when the user invokes the suggestion. -->
            {#if g.status === 'active' || g.status === 'paused'}
              <div class="border-t border-surface1 px-3 py-1.5 flex items-center justify-end">
                <button
                  type="button"
                  onclick={() => aiToggle(g)}
                  class="inline-flex items-center gap-1 text-[11px] {aiOpen ? 'text-primary' : 'text-dim hover:text-secondary'}"
                  title="ask AI for the next concrete milestone"
                  aria-label="suggest next milestone with AI"
                  aria-expanded={aiOpen}
                  disabled={aiBusy && !aiOpen}
                >
                  {aiOpen ? '✕' : '✨'} {aiOpen ? 'close' : 'suggest next milestone'}
                </button>
              </div>
              {#if aiOpen}
                <div class="border-t border-surface1 bg-mantle p-3">
                  <div class="text-[10px] uppercase tracking-wider text-dim mb-1">AI suggestion</div>
                  {#if aiError}
                    <div class="text-sm text-error mb-2">{aiError}</div>
                  {/if}
                  {#if aiText || aiBusy}
                    <div class="text-sm text-text leading-snug min-h-[1.4em]">
                      {aiText}{#if aiBusy}<span class="inline-block w-1.5 h-3.5 ml-0.5 align-middle bg-primary/60 animate-pulse rounded-sm"></span>{/if}
                    </div>
                  {/if}
                  <div class="flex flex-wrap items-center gap-2 mt-2.5">
                    {#if aiBusy}
                      <button
                        type="button"
                        onclick={() => aiAbort?.abort()}
                        class="px-2 py-1 text-xs bg-surface1 text-subtext rounded hover:bg-surface2"
                      >Stop</button>
                    {:else}
                      <button
                        type="button"
                        onclick={() => aiAdoptAsMilestone(g)}
                        disabled={!aiText}
                        class="px-2.5 py-1 text-xs bg-primary text-on-primary rounded hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
                      >+ Add as milestone</button>
                      <button
                        type="button"
                        onclick={() => aiSuggest(g)}
                        class="px-2 py-1 text-xs bg-surface1 text-subtext rounded hover:bg-surface2"
                        title="re-roll the suggestion"
                      >↻ Try again</button>
                    {/if}
                    <button
                      type="button"
                      onclick={aiClose}
                      class="ml-auto text-xs text-dim hover:text-text"
                    >Dismiss</button>
                  </div>
                </div>
              {/if}
            {/if}
          </article>
        {/each}
      </div>
    {:else if viewMode === 'list'}
      <!-- Compact list — denser layout for users with many goals.
           Each row: title · status pill · countdown chip · progress
           bar inline. Click anywhere on the row opens the detail
           drawer. Same urgency border-left as cards so the visual
           language stays consistent. -->
      <div class="bg-surface0 border border-surface1 rounded-lg overflow-hidden divide-y divide-surface1">
        {#each visibleGoals as g (g.id)}
          {@const p = progress(g)}
          {@const sc = statusColor(g.status)}
          {@const tone = targetTone(g)}
          {@const chip = targetChip(g.target_date)}
          {@const roll = rollupFor(g)}
          <button
            type="button"
            onclick={() => openDetail(g)}
            class="w-full text-left px-3 py-2 flex items-center gap-3 hover:bg-surface1 transition-colors {tone ? 'border-l-4' : 'border-l-4 border-l-transparent'}"
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
                  <span class="font-mono tabular-nums">{fmtDate(g.target_date)}</span>
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
                  {@const p = progress(g)}
                  {@const tone = targetTone(g)}
                  {@const chip = targetChip(g.target_date)}
                  <button
                    type="button"
                    onclick={() => openDetail(g)}
                    class="w-full text-left p-2.5 bg-mantle rounded border border-surface1 hover:border-primary transition-colors {tone ? 'border-l-4' : ''}"
                    style={tone ? `border-left-color: var(--color-${tone});` : ''}
                  >
                    <div class="text-sm font-medium text-text break-words leading-snug">{@html inlineMd(g.title)}</div>
                    <div class="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 mt-1.5 text-[11px] text-dim">
                      {#if g.target_date}
                        <span class="font-mono tabular-nums">{fmtDate(g.target_date)}</span>
                      {/if}
                      {#if chip && (g.status === 'active' || g.status === 'paused')}
                        <span class="tabular-nums" style="color: var(--color-{chip.tone});">{chip.label}</span>
                      {/if}
                    </div>
                    {#if g.venture || g.project}
                      <div class="text-[11px] text-secondary truncate mt-0.5">
                        {#if g.venture}🏢 {g.venture}{/if}
                        {#if g.venture && g.project} · {/if}
                        {#if g.project}📁 {g.project}{/if}
                      </div>
                    {/if}
                    {#if p.total > 0}
                      <div class="mt-1.5">
                        <div class="h-1 bg-surface1 rounded-full overflow-hidden">
                          <div class="h-full bg-primary" style="width: {p.pct}%"></div>
                        </div>
                        <div class="text-[10px] text-dim mt-0.5 tabular-nums">{p.done}/{p.total} · {p.pct}%</div>
                      </div>
                    {/if}
                  </button>
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
  onUpdated={load}
  onDeleted={deleted}
  onOpenDashboard={openDashboard}
/>

{#if dashboardOpen && selected}
  <!-- Goal Dashboard overlay — full-screen visual operating
       picture for the focused goal. URL-persisted via
       ?focus=X&dashboard=1 so a reload keeps it open. Sits above
       the goals page chrome (list/cards/kanban + detail drawer)
       without unmounting them, so closing the dashboard lands the
       user back where they came from. -->
  <GoalDashboardPanel goal={selected} onClose={closeDashboard} />
{/if}

<!-- Goal Agent — operates on the filtered list (whatever the
     current status / search / venture / category scope yields).
     parent's load() reconciles every goal page surface. -->
<GoalAgent
  open={agentOpen}
  goals={filtered}
  todayISO={todayISO()}
  knownVentures={ventures}
  onClose={() => (agentOpen = false)}
  onChanged={load}
/>
