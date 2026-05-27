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
  import { isTypingTarget } from '$lib/util/isTypingTarget';
  import { loadStored, loadStoredString, saveStored, saveStoredString } from '$lib/util/storage';
  import { rafThrottle } from '$lib/util/streamThrottle';
  import { saveProposals, loadProposals } from '$lib/util/proposalCache';
  import { extractJsonBlock } from '$lib/util/jsonExtract';
  import { focusOnMount } from '$lib/util/focusOnMount';
  import {
    buildPlanDayPrompt,
    roundUpTo15Min,
    validatePlanItems,
    type PlanItem
  } from '$lib/tasks/aiPrompts';

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
  let filterDrawerOpen = $state(false);

  // AI inbox-triage state. The button on the inbox view kicks off
  // /api/v1/ai/inbox-triage; the response is a list of proposals
  // {id, priority, schedule, rationale} that render as accept/skip
  // chips. Accept applies the suggested priority + a derived
  // scheduledStart based on the schedule keyword.
  let aiTriageBusy = $state(false);
  let aiTriageProposals = $state<{
    id: string;
    priority: number;
    schedule: string;
    rationale: string;
  }[]>([]);
  // One controller per in-flight triage call. Holding a reference
  // lets the Cancel button abort the fetch (which the server picks
  // up via r.Context() and short-circuits before billing tokens).
  let aiTriageAbort: AbortController | null = null;

  // Cached AI proposals — see $lib/util/proposalCache. Stored under
  // these feature keys with a 24 h TTL so a refresh / SW update
  // doesn't lose the suggestions the user already paid tokens for.
  const TRIAGE_KEY = 'granit.ai.triage.proposals';
  const DEADLINE_KEY = 'granit.ai.deadlines.proposals';

  // ── AI Plan-my-day ───────────────────────────────────────────────
  // Different agent than triage: triage processes UNTRIAGED tasks;
  // this one looks across the WHOLE open task set and produces a
  // sequenced plan of 3-7 tasks for TODAY, gated by the user's
  // declared focus hours. Returns strict JSON so each row gets an
  // "accept" button that pins the task into a time slot
  // (scheduledStart = now + cumulative minutes, dueDate = today).
  // Falls back to the streamed text if JSON parse fails — old
  // text-only display remains available so a malformed model reply
  // never leaves the user with nothing visible.
  //
  // Goes through chatStream so it joins the audit rollup, sabbath
  // gate, redaction, cost tracking. NEVER bypass.
  let aiFocusBusy = $state(false);
  let aiFocusError = $state('');
  let aiFocusResponse = $state('');
  let aiFocusPlan = $state<PlanItem[]>([]);
  let aiFocusSkipped = $state('');
  let aiFocusAbort: AbortController | null = null;
  // Persist the user's typical focus-hours so they don't retype "4"
  // every morning. localStorage-backed; defaults to 4 (a realistic
  // deep-work day for most knowledge workers).
  const FOCUS_HOURS_KEY = 'granit.tasks.focusHours';
  let aiFocusHours = $state<number>(
    Number(loadStoredString(FOCUS_HOURS_KEY, '4')) || 4
  );
  $effect(() => saveStoredString(FOCUS_HOURS_KEY, String(aiFocusHours)));

  async function runAIFocus() {
    if (aiFocusBusy) return;
    aiFocusBusy = true;
    aiFocusError = '';
    aiFocusResponse = '';
    aiFocusPlan = [];
    aiFocusSkipped = '';
    aiFocusAbort = new AbortController();
    const { system, user: userMessage } = buildPlanDayPrompt(tasks, todayISO(), aiFocusHours);
    // rAF throttle — aiFocusResponse is rendered live through
    // MarkdownRenderer in the streaming branch (1871). Pre-fix this
    // re-parsed the whole growing buffer per chunk → main thread
    // choke. Throttle commits the latest buffer + tries the JSON
    // parse at most once per frame.
    const t = rafThrottle((full) => {
      aiFocusResponse = full;
      const block = extractJsonBlock(full);
      if (!block) return;
      try {
        const parsed = JSON.parse(block) as { plan?: PlanItem[]; skipped_reasons?: string };
        if (Array.isArray(parsed.plan)) {
          aiFocusPlan = validatePlanItems(parsed.plan, tasks);
          aiFocusSkipped = typeof parsed.skipped_reasons === 'string' ? parsed.skipped_reasons : '';
        }
      } catch {
        // Partial JSON — wait for more chunks.
      }
    });
    try {
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: userMessage }
        ],
        undefined,
        {
          onChunk: t.onChunk,
          onDone: () => { t.flush(); },
          onError: (err) => { t.flush(); aiFocusError = err.message; }
        },
        aiFocusAbort.signal
      );
    } finally {
      aiFocusBusy = false;
      aiFocusAbort = null;
    }
  }
  function cancelAIFocus() { aiFocusAbort?.abort(); }
  function dismissAIFocus() {
    aiFocusResponse = '';
    aiFocusError = '';
    aiFocusPlan = [];
    aiFocusSkipped = '';
  }

  // Accept a single plan item: schedule the task into a time slot
  // starting from now + the cumulative estimate of all previously
  // accepted slots. We can't know which OTHER plan items the user
  // already accepted from a previous session, so we anchor the
  // first accept at "now rounded up to the next 15 min" and lay
  // subsequent accepts back-to-back. dueDate set to today so the
  // task surfaces in the Today view.
  async function acceptPlanItem(p: PlanItem) {
    const t = tasks.find((x) => x.id === p.taskId);
    if (!t) return;
    // Find the cumulative offset for this item: sum estimateMinutes
    // of preceding plan items (by .order). We don't know which the
    // user already pinned, so we treat the FULL plan as the schedule
    // skeleton — accepting #2 alone still places it after #1's slot
    // (so the day reads coherently if the user accepts more later).
    const earlier = aiFocusPlan
      .filter((x) => (x.order ?? 99) < (p.order ?? 99))
      .reduce((sum, x) => sum + Math.max(15, x.estimateMinutes || 30), 0);
    const start = roundUpTo15Min(new Date());
    start.setMinutes(start.getMinutes() + earlier);
    const today = todayISO();
    try {
      await api.patchTask(p.taskId, {
        scheduledStart: start.toISOString(),
        dueDate: t.dueDate ?? today,
        durationMinutes: Math.max(15, p.estimateMinutes || 30)
      });
      // Drop the accepted item so the panel reflects what's left.
      aiFocusPlan = aiFocusPlan.filter((x) => x.taskId !== p.taskId);
      await load();
      toast.success(`Pinned: ${t.text.slice(0, 40)}${t.text.length > 40 ? '…' : ''}`);
    } catch (e) {
      toast.error('Pin failed: ' + (errorMessage(e)));
    }
  }
  function skipPlanItem(taskId: string) {
    aiFocusPlan = aiFocusPlan.filter((x) => x.taskId !== taskId);
  }
  // Accept all remaining plan items in order. Pins them back-to-back
  // starting from "now rounded to next 15 min". Useful when the user
  // trusts the plan and just wants to commit.
  async function acceptAllPlanItems() {
    const items = [...aiFocusPlan].sort((a, b) => (a.order ?? 99) - (b.order ?? 99));
    for (const p of items) {
      await acceptPlanItem(p);
    }
  }

  async function runAITriage() {
    aiTriageBusy = true;
    aiTriageAbort = new AbortController();
    try {
      const r = await api.aiInboxTriage(aiTriageAbort.signal);
      aiTriageProposals = r.proposals ?? [];
      saveProposals(TRIAGE_KEY, aiTriageProposals);
      if ((r.proposals?.length ?? 0) === 0) {
        if (r.warning) toast.warning(r.warning);
        else toast.info('No suggestions returned.');
      }
    } catch (err) {
      const msg = errorMessage(err);
      // AbortError surfaces as a DOMException with name "AbortError";
      // when fetch is aborted on Chromium-based engines the message
      // can also be "BodyStreamBuffer was aborted" — match both.
      if (err instanceof DOMException && err.name === 'AbortError') {
        toast.info('Triage cancelled.');
      } else {
        toast.error(/disabled in AI preferences/i.test(msg)
          ? 'Enable "Inbox triage" in Settings → AI features first.'
          : 'AI triage failed: ' + msg);
      }
    } finally {
      aiTriageBusy = false;
      aiTriageAbort = null;
    }
  }
  function cancelAITriage() { aiTriageAbort?.abort(); }

  function skipTriageProposal(id: string) {
    aiTriageProposals = aiTriageProposals.filter((p) => p.id !== id);
    saveProposals(TRIAGE_KEY, aiTriageProposals);
  }
  function discardTriageProposals() {
    aiTriageProposals = [];
    saveProposals(TRIAGE_KEY, []);
  }

  // The AI Stale-task verdict surface (✨ AI verdicts button + the
  // accept/defer/archive panel) lives in $lib/tasks/AIStaleVerdicts.svelte.
  // The parent passes the candidate set + the full task list and reloads
  // when a verdict is applied.

  // AI deadline-detect — sister feature to triage. Scans every open
  // task with no due_date and proposes one (or stays silent) based on
  // title/note context. Lower-pressure than triage: blanks are
  // filtered server-side so the UI only shows confident proposals.
  let aiDeadlineBusy = $state(false);
  let aiDeadlineProposals = $state<{ id: string; due_date: string; rationale: string }[]>([]);
  let aiDeadlineAbort: AbortController | null = null;
  async function runAIDeadlineDetect() {
    aiDeadlineBusy = true;
    aiDeadlineAbort = new AbortController();
    try {
      const r = await api.aiDeadlineDetect(aiDeadlineAbort.signal);
      aiDeadlineProposals = r.proposals ?? [];
      saveProposals(DEADLINE_KEY, aiDeadlineProposals);
      if ((r.proposals?.length ?? 0) === 0) {
        if (r.warning) toast.warning(r.warning);
        else toast.info('No clear deadlines detected.');
      }
    } catch (err) {
      const msg = errorMessage(err);
      if (err instanceof DOMException && err.name === 'AbortError') {
        toast.info('Detect cancelled.');
      } else {
        toast.error(/disabled in AI preferences/i.test(msg)
          ? 'Enable "Deadline detect" in Settings → AI features first.'
          : 'Detect failed: ' + msg);
      }
    } finally {
      aiDeadlineBusy = false;
      aiDeadlineAbort = null;
    }
  }
  function cancelAIDeadline() { aiDeadlineAbort?.abort(); }
  function skipDeadlineProposal(id: string) {
    aiDeadlineProposals = aiDeadlineProposals.filter((p) => p.id !== id);
    saveProposals(DEADLINE_KEY, aiDeadlineProposals);
  }
  function discardDeadlineProposals() {
    aiDeadlineProposals = [];
    saveProposals(DEADLINE_KEY, []);
  }
  async function applyDeadlineProposal(p: { id: string; due_date: string }) {
    aiDeadlineBusy = true;
    try {
      await api.patchTask(p.id, { dueDate: p.due_date });
      aiDeadlineProposals = aiDeadlineProposals.filter((x) => x.id !== p.id);
      saveProposals(DEADLINE_KEY, aiDeadlineProposals);
      await load();
    } catch (err) {
      toast.error('Apply failed: ' + (errorMessage(err)));
    } finally {
      aiDeadlineBusy = false;
    }
  }

  // Apply a proposal: patch priority + (when applicable) compute a
  // dueDate from the schedule keyword. "drop" sets done = true.
  // Move triage state out of "inbox" so the same task doesn't keep
  // showing up.
  async function applyTriageProposal(p: { id: string; priority: number; schedule: string }) {
    aiTriageBusy = true;
    try {
      const patch: Parameters<typeof api.patchTask>[1] = {};
      if (p.priority === 0) {
        patch.done = true;
        patch.triage = 'dropped';
      } else {
        patch.priority = p.priority;
        patch.triage = 'triaged';
      }
      const today = new Date();
      switch (p.schedule) {
        case 'today': {
          patch.dueDate = fmtDateISO(today);
          break;
        }
        case 'tomorrow': {
          const t = new Date(today);
          t.setDate(t.getDate() + 1);
          patch.dueDate = fmtDateISO(t);
          break;
        }
        case 'this_week': {
          // End of week — Sunday — at the latest.
          const t = new Date(today);
          const dow = t.getDay();
          const daysToSun = (7 - dow) % 7;
          t.setDate(t.getDate() + daysToSun);
          patch.dueDate = fmtDateISO(t);
          break;
        }
        case 'next_week': {
          const t = new Date(today);
          t.setDate(t.getDate() + 7);
          patch.dueDate = fmtDateISO(t);
          break;
        }
        // 'no_date' or anything else → leave dueDate alone.
      }
      await api.patchTask(p.id, patch);
      aiTriageProposals = aiTriageProposals.filter((x) => x.id !== p.id);
      saveProposals(TRIAGE_KEY, aiTriageProposals);
      await load();
    } catch (err) {
      toast.error('Apply failed: ' + (errorMessage(err)));
    } finally {
      aiTriageBusy = false;
    }
  }

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
    // by loadProposals.
    aiTriageProposals = loadProposals(TRIAGE_KEY);
    aiDeadlineProposals = loadProposals(DEADLINE_KEY);
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
    // Reset cursor when the filtered list shrinks past it.
    if (cursorIdx >= filtered.length) cursorIdx = filtered.length - 1;
  });

  // isTypingTarget lives in $lib/util/isTypingTarget — shared with
  // /projects and /goals page-level hotkey handlers.

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
      // Kanban / TriageBoard / EisenhowerView each install their own
      // window-level handler with a column-aware cursor. Suppressing
      // the page-level handler in those views avoids double-firing
      // j/k/x/d/e/p (which would move two cursors and patch twice).
      if (view === 'kanban' || view === 'triage' || view === 'eisenhower') return;
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
      // 'a' opens the Task Agent. Distinct from per-task shortcuts
      // below — no cursor task required, the agent operates on the
      // filtered list (or the bulk-selection if one is active).
      if (k === 'a') {
        agentOpen = true;
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
      const b: Record<string, Task[]> = {
        overdue: [], today: [], tomorrow: [], this_week: [], later: [], no_date: []
      };
      for (const t of filtered) {
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
        { key: 'this_week', label: 'This Week', tasks: b.this_week },
        { key: 'later',     label: 'Later',     tasks: b.later },
        { key: 'no_date',   label: 'No date',   tasks: b.no_date }
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
  </div>
{/snippet}

<div class="flex h-full">
  <!-- Desktop sidebar -->
  <aside class="hidden md:block md:w-56 lg:w-64 border-r border-surface1 bg-mantle flex-shrink-0 overflow-y-auto">
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
      <!-- View tabs split into two clusters: primary (Today / List /
           Kanban) renders as one segmented pill so the user always
           sees the three main shapes; smart-filter views (Inbox,
           Stale, Quick wins, Review) sit in a second pill with
           live count badges so the user knows which ones are
           worth visiting at a glance. Tabs with zero count read
           as muted so they don't pull attention. -->
      <div class="flex items-center gap-1.5 flex-wrap">
        <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-xs sm:text-sm">
          <button
            class="px-2 sm:px-3 py-1.5 inline-flex items-center gap-1 {view === 'today' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (view = 'today')}
            title="overdue + due today + scheduled today"
          >
            Today
            {#if stats.overdue + stats.todayCount > 0 && view !== 'today'}
              <span class="text-[10px] tabular-nums {stats.overdue > 0 ? 'text-error' : 'text-warning'}">{stats.overdue + stats.todayCount}</span>
            {/if}
          </button>
          <button
            class="px-2 sm:px-3 py-1.5 {view === 'list' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (view = 'list')}
          >List</button>
          <button
            class="px-2 sm:px-3 py-1.5 {view === 'week' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (view = 'week')}
            title="7-day grid — see what's scheduled or due each day this week"
          >Week</button>
          <button
            class="px-2 sm:px-3 py-1.5 {view === 'kanban' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (view = 'kanban')}
          >Kanban</button>
          <button
            class="px-2 sm:px-3 py-1.5 {view === 'eisenhower' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (view = 'eisenhower')}
            title="2×2 matrix: urgent × important — Covey / GTD style prioritisation"
          >Matrix</button>
        </div>
        <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-xs sm:text-sm">
          <button
            class="px-2 sm:px-3 py-1.5 inline-flex items-center gap-1 {view === 'inbox' ? 'bg-primary text-on-primary' : viewCounts.inbox > 0 ? 'text-text hover:bg-surface1' : 'text-dim hover:bg-surface2'}"
            onclick={() => (view = 'inbox')}
            title="untriaged tasks awaiting categorisation"
          >
            Inbox
            {#if viewCounts.inbox > 0 && view !== 'inbox'}
              <span class="text-[10px] tabular-nums text-secondary font-mono">{viewCounts.inbox}</span>
            {/if}
          </button>
          <button
            class="px-2 sm:px-3 py-1.5 hidden sm:inline-flex items-center gap-1 {view === 'quickwins' ? 'bg-primary text-on-primary' : viewCounts.quickwins > 0 ? 'text-text hover:bg-surface1' : 'text-dim hover:bg-surface2'}"
            onclick={() => (view = 'quickwins')}
            title="high priority + ≤30 min — tackle a few before lunch"
          >
            Quick wins
            {#if viewCounts.quickwins > 0 && view !== 'quickwins'}
              <span class="text-[10px] tabular-nums text-success font-mono">{viewCounts.quickwins}</span>
            {/if}
          </button>
          <button
            class="px-2 sm:px-3 py-1.5 hidden sm:inline-flex items-center gap-1 {view === 'stale' ? 'bg-primary text-on-primary' : viewCounts.stale > 0 ? 'text-text hover:bg-surface1' : 'text-dim hover:bg-surface2'}"
            onclick={() => (view = 'stale')}
            title="not touched in 7+ days — needs a decision"
          >
            Stale
            {#if viewCounts.stale > 0 && view !== 'stale'}
              <span class="text-[10px] tabular-nums text-warning font-mono">{viewCounts.stale}</span>
            {/if}
          </button>
          <button
            class="px-2 sm:px-3 py-1.5 hidden sm:inline-block {view === 'duplicates' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (view = 'duplicates')}
            title="near-duplicate task pairs by text similarity — deterministic scan, no AI"
          >Duplicates</button>
          <button
            class="px-2 sm:px-3 py-1.5 hidden sm:inline-flex items-center gap-1 {view === 'review' ? 'bg-primary text-on-primary' : viewCounts.review > 0 ? 'text-text hover:bg-surface1' : 'text-dim hover:bg-surface2'}"
            onclick={() => (view = 'review')}
            title="completed in the last 7 days — celebrate the wins"
          >
            Review
            {#if viewCounts.review > 0 && view !== 'review'}
              <span class="text-[10px] tabular-nums text-success font-mono">{viewCounts.review}</span>
            {/if}
          </button>
          <button
            class="px-2 sm:px-3 py-1.5 hidden sm:inline-block {view === 'triage' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (view = 'triage')}
            title="AI-driven inbox triage proposals"
          >Triage</button>
        </div>
      </div>
      <!-- Task Agent button removed from the page header — the
           agent now launches from the chat sidebar (Cmd+J →
           "Run Task Agent" chip) so AI work has one consistent
           entry point across the app. The dialog component still
           lives here and opens via ?agent=1 URL param from the
           sidebar's nav-and-open shim. -->

      <button
        onclick={() => (helpOpen = !helpOpen)}
        aria-label="keyboard shortcuts"
        title="keyboard shortcuts (?)"
        class="hidden sm:flex w-7 h-7 items-center justify-center text-dim hover:text-text border border-surface1 rounded text-sm"
      >?</button>
    </header>

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
      {#if aiFocusBusy || aiFocusResponse || aiFocusError || aiFocusPlan.length > 0}
        <div class="px-3 py-3 border-b border-surface1 flex-shrink-0 bg-surface1">
          <div class="flex items-baseline gap-2 mb-2 flex-wrap">
            <span class="text-xs uppercase tracking-wider text-secondary font-semibold">Plan my day</span>
            {#if aiFocusPlan.length > 0 && !aiFocusBusy}
              {@const totalEst = aiFocusPlan.reduce((s, p) => s + Math.max(15, p.estimateMinutes || 30), 0)}
              <span class="text-[11px] text-dim font-mono tabular-nums">{aiFocusPlan.length} task{aiFocusPlan.length === 1 ? '' : 's'} · {totalEst}m</span>
            {/if}
            <span class="flex-1"></span>
            {#if aiFocusBusy}
              <button onclick={cancelAIFocus} class="text-[11px] text-warning hover:underline">cancel</button>
            {:else}
              {#if aiFocusPlan.length > 0}
                <button onclick={() => void acceptAllPlanItems()} class="text-[11px] text-success hover:underline" title="Pin every remaining plan item back-to-back starting now">accept all</button>
              {/if}
              <button onclick={() => void runAIFocus()} class="text-[11px] text-secondary hover:underline">↻ regenerate</button>
              <button onclick={dismissAIFocus} class="text-[11px] text-dim hover:text-error">dismiss</button>
            {/if}
          </div>
          {#if aiFocusError}
            <div class="text-xs text-error">{aiFocusError}</div>
          {:else if aiFocusPlan.length > 0}
            <!-- Structured plan view. Each row has its own accept/skip,
                 so the user can take 4 of 5 suggestions without burning
                 the call. -->
            <ol class="space-y-1.5">
              {#each aiFocusPlan as p (p.taskId)}
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
                      onclick={() => void acceptPlanItem(p)}
                      class="px-2 py-0.5 bg-surface0 text-success rounded hover:bg-surface1 flex-shrink-0"
                      title="Pin this task into a time slot today"
                    >accept</button>
                    <button
                      onclick={() => skipPlanItem(p.taskId)}
                      class="px-2 py-0.5 text-dim hover:text-text flex-shrink-0"
                    >skip</button>
                  </li>
                {/if}
              {/each}
            </ol>
            {#if aiFocusSkipped}
              <p class="text-[11px] text-dim italic mt-2 pt-2 border-t border-surface1">Skipped: {aiFocusSkipped}</p>
            {/if}
            <p class="text-[10px] text-dim mt-2">Context: {tasks.filter((t) => !t.done).slice(0, 30).length} open tasks shown · {aiFocusHours}h focus budget</p>
          {:else}
            <!-- Streaming/fallback view: show the raw model output while
                 we wait for the JSON to close, OR if parsing fails. -->
            <div class="prose prose-sm max-w-none text-sm">
              <MarkdownRenderer body={aiFocusResponse || '_thinking…_'} />
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
          onclick={() => void runAIFocus()}
          disabled={aiFocusBusy || tasks.filter((t) => !t.done).length === 0}
          title="AI builds a sequenced day-plan budgeted to your focus hours"
          class="hidden sm:inline-flex px-3 py-2 text-sm bg-surface1 border border-surface2 text-primary rounded hover:border-primary disabled:opacity-50 flex-shrink-0 items-center gap-1.5"
        >
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5L12 3z"/>
          </svg>
          <span>{aiFocusBusy ? 'planning…' : 'Plan day'}</span>
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
      <!-- Smart filter chip bar — every chip is a one-click filter
           that narrows the list to its predicate. The chip lights up
           when active; clicking again clears (toggle). Counts are
           live across the loaded tasks so the user always sees how
           many would be revealed before clicking. The done/velocity
           chips remain passive (no filter behaviour — they're
           informational) but live in the same row for a single
           "at-a-glance" surface. -->
      <div class="px-3 py-2 border-b border-surface1 flex items-center gap-1.5 text-xs flex-shrink-0 flex-wrap">
        <span class="px-2 py-1 rounded bg-surface0 text-subtext font-mono tabular-nums">
          <span class="text-text font-semibold">{stats.open}</span> open
        </span>
        {#if smartCounts.overdue > 0}
          <button
            type="button"
            onclick={() => (smartFilter = smartFilter === 'overdue' ? '' : 'overdue')}
            aria-pressed={smartFilter === 'overdue'}
            title="Tasks past their due date — click to filter"
            class="px-2 py-1 rounded font-mono tabular-nums {smartFilter === 'overdue' ? 'bg-error text-on-primary' : 'bg-surface0 text-error hover:bg-surface1'}"
          ><span class="font-semibold">{smartCounts.overdue}</span> overdue</button>
        {/if}
        {#if smartCounts.today > 0}
          <button
            type="button"
            onclick={() => (smartFilter = smartFilter === 'today' ? '' : 'today')}
            aria-pressed={smartFilter === 'today'}
            title="Due or scheduled today — click to filter"
            class="px-2 py-1 rounded font-mono tabular-nums {smartFilter === 'today' ? 'bg-warning text-on-primary' : 'bg-surface0 text-warning hover:bg-surface1'}"
          ><span class="font-semibold">{smartCounts.today}</span> today</button>
        {/if}
        {#if smartCounts.tomorrow > 0}
          <button
            type="button"
            onclick={() => (smartFilter = smartFilter === 'tomorrow' ? '' : 'tomorrow')}
            aria-pressed={smartFilter === 'tomorrow'}
            title="Due or scheduled tomorrow — click to filter"
            class="px-2 py-1 rounded font-mono tabular-nums {smartFilter === 'tomorrow' ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
          ><span class="font-semibold">{smartCounts.tomorrow}</span> tmrw</button>
        {/if}
        {#if smartCounts.thisWeek > 0}
          <button
            type="button"
            onclick={() => (smartFilter = smartFilter === 'thisWeek' ? '' : 'thisWeek')}
            aria-pressed={smartFilter === 'thisWeek'}
            title="Due or scheduled in the next 7 days — click to filter"
            class="px-2 py-1 rounded font-mono tabular-nums {smartFilter === 'thisWeek' ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
          ><span class="font-semibold">{smartCounts.thisWeek}</span> 7d</button>
        {/if}
        {#if smartCounts.noDue > 0}
          <button
            type="button"
            onclick={() => (smartFilter = smartFilter === 'noDue' ? '' : 'noDue')}
            aria-pressed={smartFilter === 'noDue'}
            title="No due date and no scheduled time — click to filter and prioritize"
            class="px-2 py-1 rounded font-mono tabular-nums {smartFilter === 'noDue' ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'}"
          ><span class="font-semibold">{smartCounts.noDue}</span> no-date</button>
        {/if}
        {#if smartCounts.highPriority > 0}
          <button
            type="button"
            onclick={() => (smartFilter = smartFilter === 'highPriority' ? '' : 'highPriority')}
            aria-pressed={smartFilter === 'highPriority'}
            title="P1 (highest priority) tasks — click to filter"
            class="px-2 py-1 rounded font-mono tabular-nums {smartFilter === 'highPriority' ? 'bg-primary text-on-primary' : 'bg-surface0 text-text hover:bg-surface1'}"
          >!<span class="font-semibold">{smartCounts.highPriority}</span></button>
        {/if}
        {#if smartCounts.noPriority > 0}
          <button
            type="button"
            onclick={() => (smartFilter = smartFilter === 'noPriority' ? '' : 'noPriority')}
            aria-pressed={smartFilter === 'noPriority'}
            title="No priority set — click to filter and triage"
            class="px-2 py-1 rounded font-mono tabular-nums {smartFilter === 'noPriority' ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'}"
          ><span class="font-semibold">{smartCounts.noPriority}</span> no-pri</button>
        {/if}
        {#if smartCounts.hasSubtasks > 0}
          <button
            type="button"
            onclick={() => (smartFilter = smartFilter === 'hasSubtasks' ? '' : 'hasSubtasks')}
            aria-pressed={smartFilter === 'hasSubtasks'}
            title="Tasks with children — click to filter to parent-only"
            class="px-2 py-1 rounded font-mono tabular-nums {smartFilter === 'hasSubtasks' ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
          >⫶<span class="font-semibold">{smartCounts.hasSubtasks}</span></button>
        {/if}
        {#if stats.doneToday > 0}
          <span class="px-2 py-1 rounded bg-surface0 text-success font-mono tabular-nums" title="Completed today">
            ✓ <span class="font-semibold">{stats.doneToday}</span> today
          </span>
        {/if}
        {#if stats.doneWeek > 0}
          <span class="px-2 py-1 rounded bg-surface0 text-success font-mono tabular-nums" title="Completed in the last 7 days — rolling weekly velocity">
            ✓ <span class="font-semibold">{stats.doneWeek}</span> 7d
          </span>
        {/if}
        {#if stats.sumEstMin > 0}
          <span class="px-2 py-1 rounded bg-surface0 text-secondary font-mono tabular-nums" title="Total estimated minutes across open, non-snoozed tasks ({stats.sumEstMin}m). 8h = one day-block.">
            Σ <span class="font-semibold">{fmtEstBudget(stats.sumEstMin)}</span>
          </span>
        {/if}
        {#if stats.noEstCount > 0}
          <span class="px-2 py-1 rounded bg-surface0 text-dim font-mono tabular-nums" title="Open tasks with no time estimate — add `est:30m` so the total chip becomes accurate">
            ? <span class="font-semibold">{stats.noEstCount}</span>
          </span>
        {/if}
        {#if stats.avgPriority > 0}
          {@const ap = stats.avgPriority}
          {@const apTone = ap < 1.5 ? 'text-error' : ap < 2.5 ? 'text-warning' : 'text-info'}
          <span class="px-2 py-1 rounded bg-surface0 font-mono tabular-nums {apTone}" title="Average priority across prioritised open tasks (1=high, 3=low). Lower = more urgent overall load.">
            avg P<span class="font-semibold">{ap.toFixed(1)}</span>
          </span>
        {/if}
        {#if stats.snoozed > 0}
          <span class="px-2 py-1 rounded bg-surface0 text-dim font-mono tabular-nums" title="Currently snoozed">
            zZ {stats.snoozed}
          </span>
        {/if}
        <span class="flex-1"></span>
        {#if view === 'list'}
          <span class="text-dim select-none">group</span>
          <select
            bind:value={groupBy}
            title="How to split the list into groups"
            class="bg-surface0 border border-surface1 px-2 py-1 text-text"
          >
            <option value="due">due date</option>
            <option value="priority">priority</option>
            <option value="tag">tag</option>
            <option value="project">project</option>
            <option value="goal">goal</option>
            <option value="deadline">deadline</option>
            <option value="note">note</option>
          </select>
          <span class="text-dim select-none">sort</span>
          <select
            bind:value={sortBy}
            title="How to order tasks inside each group. 'auto' uses due-then-priority (the historical default); other choices apply the same rule across every group."
            class="bg-surface0 border border-surface1 px-2 py-1 text-text"
          >
            <option value="auto">auto</option>
            <option value="priority">priority</option>
            <option value="due">due</option>
            <option value="age">age (oldest first)</option>
            <option value="alpha">A → Z</option>
            <option value="estimate">estimate (smallest)</option>
          </select>
          <!-- Density toggle — compact mode strips visual breathing
               room from every TaskCard so power users see ~40% more
               rows above the fold. Persisted to localStorage. -->
          <button
            type="button"
            onclick={() => (density = density === 'compact' ? 'normal' : 'compact')}
            aria-pressed={density === 'compact'}
            title={density === 'compact' ? 'Compact density — click for comfortable spacing' : 'Comfortable density — click for compact rows'}
            class="px-1.5 py-1 text-[10px] uppercase tracking-wider font-mono {density === 'compact' ? 'bg-primary text-on-primary' : 'bg-surface0 text-dim hover:bg-surface1 hover:text-text'} border border-surface1 rounded"
          >{density === 'compact' ? '≡' : '≣'}</button>
        {:else}
          <span class="text-dim">columns</span>
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
        <!-- Tasks exist but the active filter masks them all. Offer
             a "Clear filters" reset so the user isn't stuck. -->
        <div class="max-w-md mx-auto py-6 text-center">
          <div class="text-4xl mb-3 opacity-30">🔍</div>
          <h2 class="text-base font-medium text-text mb-2">No tasks match these filters</h2>
          <p class="text-sm text-dim mb-3">
            {tasks.length} {tasks.length === 1 ? 'task is' : 'tasks are'} hidden by the current filters.
          </p>
          <button
            onclick={() => {
              q = ''; tagFilters = []; projectFilter = ''; priorityFilter = '';
              goalFilter = ''; deadlineFilter = ''; status = 'open';
            }}
            class="px-3 py-1.5 bg-surface0 border border-surface1 hover:border-primary rounded text-sm text-subtext"
          >Clear filters</button>
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
            {#if aiTriageBusy}
              <button
                onclick={cancelAITriage}
                class="px-3 py-1.5 text-xs bg-surface0 text-warning rounded hover:bg-surface1 flex-shrink-0"
                title="Cancel the in-flight triage call"
              >✨ thinking… cancel</button>
            {:else}
              <button
                onclick={() => void runAITriage()}
                disabled={filtered.length === 0}
                class="px-3 py-1.5 text-xs bg-surface1 text-secondary rounded hover:bg-surface2 disabled:opacity-50 flex-shrink-0"
                title="Ask AI to suggest priority + schedule for each untriaged task"
              >✨ AI triage</button>
            {/if}
            {#if aiDeadlineBusy}
              <button
                onclick={cancelAIDeadline}
                class="px-3 py-1.5 text-xs bg-surface0 text-warning rounded hover:bg-surface1 flex-shrink-0"
                title="Cancel the in-flight deadline scan"
              >✨ thinking… cancel</button>
            {:else}
              <button
                onclick={() => void runAIDeadlineDetect()}
                class="px-3 py-1.5 text-xs bg-surface1 text-secondary rounded hover:bg-surface2 disabled:opacity-50 flex-shrink-0"
                title="Scan all open tasks without a due date — propose ones whose title implies a clear deadline"
              >✨ Detect deadlines</button>
            {/if}
          </div>

          {#if aiDeadlineProposals.length > 0}
            <!-- Deadline proposals — operates across ALL open tasks
                 without a due_date, not just inbox. Server already
                 filtered out blanks, so every row is a confident
                 suggestion. Apply patches dueDate; skip just dismisses. -->
            <div class="mb-5 p-3 bg-surface0 border border-warning rounded">
              <div class="flex items-center mb-2">
                <div class="text-xs uppercase tracking-wider text-warning font-semibold flex-1">Detected deadlines ({aiDeadlineProposals.length})</div>
                <button
                  onclick={discardDeadlineProposals}
                  class="text-[10px] text-dim hover:text-error"
                  title="Drop all proposals without applying any"
                >discard</button>
              </div>
              <ul class="space-y-2">
                {#each aiDeadlineProposals as p (p.id)}
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
                        onclick={() => void applyDeadlineProposal(p)}
                        disabled={aiDeadlineBusy}
                        class="px-2 py-0.5 bg-surface0 text-success rounded hover:bg-surface1"
                      >accept</button>
                      <button
                        onclick={() => skipDeadlineProposal(p.id)}
                        class="px-2 py-0.5 text-dim hover:text-text"
                      >skip</button>
                    </li>
                  {/if}
                {/each}
              </ul>
            </div>
          {/if}

          {#if aiTriageProposals.length > 0}
            <!-- AI suggestions panel. Each proposal has Accept /
                 Skip; accepting applies the suggested priority +
                 schedule to the matching task. -->
            <div class="mb-5 p-3 bg-surface1 border border-surface2 rounded">
              <div class="flex items-center mb-2">
                <div class="text-xs uppercase tracking-wider text-secondary font-semibold flex-1">AI suggestions ({aiTriageProposals.length})</div>
                <button
                  onclick={discardTriageProposals}
                  class="text-[10px] text-dim hover:text-error"
                  title="Drop all proposals without applying any"
                >discard</button>
              </div>
              <ul class="space-y-2">
                {#each aiTriageProposals as p (p.id)}
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
                        onclick={() => void applyTriageProposal(p)}
                        disabled={aiTriageBusy}
                        class="px-2 py-0.5 bg-surface0 text-success rounded hover:bg-surface1"
                      >accept</button>
                      <button
                        onclick={() => skipTriageProposal(p.id)}
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
        <div class="space-y-4 max-w-3xl">
          {#each listGroups as g (g.key)}
            {@const dotColor = (
              g.key === 'overdue' ? 'bg-error' :
              g.key === 'today' ? 'bg-warning' :
              g.key === 'tomorrow' ? 'bg-secondary' :
              g.key === 'this_week' ? 'bg-success' :
              'bg-surface2'
            )}
            {@const labelColor = (
              g.key === 'overdue' ? 'text-error' :
              g.key === 'today' ? 'text-warning' :
              'text-text'
            )}
            <section>
              <h2 class="text-xs uppercase tracking-wider mb-2 font-semibold border-b border-surface1 pb-1.5 flex items-center gap-2">
                <!-- Color dot keyed to urgency tone: overdue red,
                     today amber, tomorrow blue, this-week green,
                     anything else muted. Quick scan signal. -->
                <span class="w-2 h-2 rounded-full {dotColor}" aria-hidden="true"></span>
                <span class={labelColor}>{g.label}</span>
                <span class="text-dim font-mono tabular-nums text-[11px]">{g.tasks.length}</span>
                {#if g.key === 'overdue' && g.tasks.length > 0}
                  <span class="ml-1 px-1.5 py-0.5 bg-surface0 text-error text-[10px] tracking-wider rounded uppercase font-bold animate-pulse" title="These tasks are past their due date">
                    overdue
                  </span>
                {/if}
                {#if g.deepLink}
                  <a
                    href={g.deepLink}
                    class="text-[10px] text-secondary hover:underline normal-case tracking-normal"
                    title="open {g.label}"
                  >open ↗</a>
                {/if}
                <!-- Per-group quick-add. Opens an inline input that
                     applies the group's defaults (due/priority/tag/
                     project/goal/deadline) to the new task. -->
                <button
                  type="button"
                  onclick={() => { groupAddKey = groupAddKey === g.key ? null : g.key; groupAddText = ''; }}
                  class="ml-auto text-[10px] text-dim hover:text-text normal-case tracking-normal font-mono"
                  title="add a task to this group ({g.label})"
                >+ add</button>
              </h2>
              {#if groupAddKey === g.key}
                <!-- Pre-render the inline input above the group's
                     task list so the new row appears right where the
                     user expects it. Input auto-focuses; Enter saves,
                     Esc dismisses. The input stays open after save so
                     the user can keep capturing without re-opening. -->
                <div class="mb-1.5 flex items-center gap-1.5">
                  <input
                    type="text"
                    bind:value={groupAddText}
                    onkeydown={(e) => {
                      if (e.key === 'Enter') { e.preventDefault(); submitGroupAdd(g.key); }
                      else if (e.key === 'Escape') { e.preventDefault(); cancelGroupAdd(); }
                    }}
                    onblur={() => { if (!groupAddText.trim() && !groupAddBusy) cancelGroupAdd(); }}
                    placeholder="new task in {g.label}…"
                    use:focusOnMount
                    disabled={groupAddBusy}
                    class="flex-1 bg-surface0 border border-surface1 rounded px-2 py-1 text-sm text-text placeholder-dim focus:outline-none focus:border-primary disabled:opacity-50"
                  />
                  <button
                    type="button"
                    onclick={() => submitGroupAdd(g.key)}
                    disabled={groupAddBusy || !groupAddText.trim()}
                    class="text-[11px] px-2 py-1 rounded bg-primary text-on-primary font-medium hover:opacity-90 disabled:opacity-40"
                  >{groupAddBusy ? '…' : 'add'}</button>
                </div>
              {/if}
              <div class="space-y-1.5">
                {#each g.tasks.filter((tt) => !isHiddenByCollapse(tt.id, collapsedIds)) as t (t.id)}
                  <div data-task-id={t.id} class={cursorIdx >= 0 && filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                    <TaskCard task={t} compact={compactCards} hasChildren={(childCount.get(t.id) ?? 0) > 0} childCount={childCount.get(t.id) ?? 0} collapsed={collapsedIds.has(t.id)} onToggleCollapse={() => toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
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
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">e</kbd>
        <span class="text-subtext">open task detail</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">d</kbd>
        <span class="text-subtext">toggle done</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">p</kbd>
        <span class="text-subtext">cycle priority (P0→P3)</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">esc</kbd>
        <span class="text-subtext">clear selection</span>
        <kbd class="font-mono text-xs px-1.5 py-0.5 bg-surface1 rounded text-subtext">a</kbd>
        <span class="text-subtext">open AI agent (operates on filtered list or bulk-selection)</span>
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
