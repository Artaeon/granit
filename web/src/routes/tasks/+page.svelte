<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import { api, type Task, type Project, type Goal, type Deadline } from '$lib/api';
  import { parseTaskInput, smartDate } from '$lib/util/taskParse';
  import { toast } from '$lib/components/toast';
  import { onWsEvent } from '$lib/ws';
  import TaskCard from '$lib/tasks/TaskCard.svelte';
  import Kanban from '$lib/tasks/Kanban.svelte';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import TriageBoard from '$lib/tasks/TriageBoard.svelte';
  import BulkBar from '$lib/tasks/BulkBar.svelte';
  import TaskDetail from '$lib/tasks/TaskDetail.svelte';
  import TaskContextMenu from '$lib/tasks/TaskContextMenu.svelte';
  import Drawer from '$lib/components/Drawer.svelte';

  type View = 'list' | 'kanban' | 'today' | 'triage' | 'inbox' | 'stale' | 'quickwins' | 'review';
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
  // Source filter — separates "tasks the user actually wrote as tasks"
  // from "stray `- [ ]` bullets in reading notes / brainstorm pages".
  // Default is 'task-notes' (only notes that look like task surfaces:
  // daily notes, anything under Tasks/Projects/Daily, or notes with a
  // type:task/project/daily frontmatter declared via path patterns we
  // can detect without a frontmatter fetch). Flipping to 'all' shows
  // every checkbox in the vault — same behaviour as before this filter
  // shipped. Persisted in localStorage so the user's preference sticks.
  const SOURCE_KEY = 'granit.tasks.source';
  let sourceFilter = $state<'task-notes' | 'all'>(
    typeof localStorage !== 'undefined' && localStorage.getItem(SOURCE_KEY) === 'all'
      ? 'all'
      : 'task-notes'
  );
  $effect(() => {
    if (typeof localStorage === 'undefined') return;
    try { localStorage.setItem(SOURCE_KEY, sourceFilter); } catch {}
  });
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
      if (['list', 'kanban', 'today', 'triage', 'inbox', 'stale', 'quickwins', 'review'].includes(v)) view = v;
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

  // Persist proposals to localStorage so navigating away (or a
  // refresh / SW update) doesn't burn the AI work the user just
  // paid for. Tag with the date they were generated; we drop them
  // on load if they're older than 24h since the underlying tasks
  // may have moved on. Same shape used for deadline proposals below.
  const TRIAGE_KEY = 'granit.ai.triage.proposals';
  const DEADLINE_KEY = 'granit.ai.deadlines.proposals';
  const PROPOSAL_TTL_MS = 24 * 60 * 60 * 1000;
  function saveProposals<T>(key: string, items: T[]) {
    try {
      if (items.length === 0) localStorage.removeItem(key);
      else localStorage.setItem(key, JSON.stringify({ at: Date.now(), items }));
    } catch {}
  }
  function loadProposals<T>(key: string): T[] {
    try {
      const raw = localStorage.getItem(key);
      if (!raw) return [];
      const parsed = JSON.parse(raw) as { at?: number; items?: T[] };
      if (!parsed.at || Date.now() - parsed.at > PROPOSAL_TTL_MS) {
        localStorage.removeItem(key);
        return [];
      }
      return parsed.items ?? [];
    } catch {
      return [];
    }
  }

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
  type PlanItem = {
    taskId: string;
    order: number;
    estimateMinutes: number;
    rationale: string;
  };
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
    typeof localStorage !== 'undefined'
      ? Number(localStorage.getItem(FOCUS_HOURS_KEY) || '4') || 4
      : 4
  );
  $effect(() => {
    if (typeof localStorage === 'undefined') return;
    try { localStorage.setItem(FOCUS_HOURS_KEY, String(aiFocusHours)); } catch {}
  });

  // Extract the first {...} JSON object from a streaming reply.
  // Models occasionally wrap JSON in ```json fences or add a
  // sentence of commentary; we strip both before parsing.
  function extractJsonBlock(s: string): string | null {
    if (!s) return null;
    const fence = s.match(/```(?:json)?\s*([\s\S]*?)```/);
    const candidate = fence ? fence[1] : s;
    // Find first '{' and matching '}' by counting depth.
    const start = candidate.indexOf('{');
    if (start < 0) return null;
    let depth = 0;
    for (let i = start; i < candidate.length; i++) {
      const c = candidate[i];
      if (c === '{') depth++;
      else if (c === '}') {
        depth--;
        if (depth === 0) return candidate.slice(start, i + 1);
      }
    }
    return null;
  }

  async function runAIFocus() {
    if (aiFocusBusy) return;
    aiFocusBusy = true;
    aiFocusError = '';
    aiFocusResponse = '';
    aiFocusPlan = [];
    aiFocusSkipped = '';
    aiFocusAbort = new AbortController();
    // Compose a context blob with up to ~30 open tasks. Cap so a
    // huge backlog doesn't blow the prompt size; the AI's job is
    // to pick from the slice we feed it, not to read the whole
    // graph. Today's date threads in so "due tomorrow" lines up.
    const open = tasks.filter((t) => !t.done).slice(0, 30);
    const today = new Date().toISOString().slice(0, 10);
    const focusMinutes = Math.max(30, Math.round(aiFocusHours * 60));
    const lines = open.map((t) => {
      const bits: string[] = [`id:${t.id} — ${t.text}`];
      if (t.priority) bits.push(`p${t.priority}`);
      if (t.dueDate) bits.push(`due ${t.dueDate}`);
      if (t.scheduledStart) bits.push(`scheduled ${t.scheduledStart.slice(0, 10)}`);
      if (t.estimatedMinutes) bits.push(`est ${t.estimatedMinutes}m`);
      return bits.join(' · ');
    }).join('\n');
    // SHARP system prompt. The point: refuse the "everything is
    // important" trap. Force prioritisation. Name what to do FIRST.
    const system =
      'You are a calm, ruthless planning partner. Your job: build a realistic plan for ONE day, not a wishlist. ' +
      'Hard rules: ' +
      '(1) Pick 3-7 tasks max. Fewer is better when the user has limited focus. ' +
      '(2) Sum of estimateMinutes MUST fit within the focus_minutes budget. If the budget is tight, drop tasks — do not shrink estimates to fake fit. ' +
      '(3) Order by what unlocks the day: anything overdue or due-today goes first, then the highest-leverage deep-work item while attention is fresh, admin/quick-wins last. ' +
      '(4) Each rationale must be ONE sentence under 18 words, naming WHY this task NOW (not generic praise). Examples of GOOD rationales: "Overdue two days — close the loop before the standup at 10."; "Deep-work block while you\'re fresh — the report is the bottleneck for Friday\'s review."; "30-min admin task — slot at the energy dip after lunch." ' +
      '(5) If a task lacks an estimate, give your best 15/30/60 min guess based on the title. ' +
      '(6) Output STRICT JSON ONLY, no markdown fences, no preamble. Schema: ' +
      '{"plan":[{"taskId":"<exact id from list>","order":1,"estimateMinutes":30,"rationale":"…"}],"skipped_reasons":"<one sentence on what you cut and why, or empty>"}.';
    const userMessage =
      `Today is ${today}. The user has roughly ${aiFocusHours} hour${aiFocusHours === 1 ? '' : 's'} (~${focusMinutes} minutes) of focus time today. ` +
      'Build a plan from their open tasks below. Use the EXACT taskId values in the JSON; do not invent new ones.\n\n' +
      'Open tasks:\n\n' + lines;
    try {
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: userMessage }
        ],
        undefined,
        {
          onChunk: (c) => {
            aiFocusResponse += c;
            // Try to parse the structured plan as it streams. Most
            // models emit the closing brace late, so this only
            // succeeds toward the end — but it lets a fast local
            // model populate the panel before the stream officially
            // finishes.
            const block = extractJsonBlock(aiFocusResponse);
            if (block) {
              try {
                const parsed = JSON.parse(block) as { plan?: PlanItem[]; skipped_reasons?: string };
                if (Array.isArray(parsed.plan)) {
                  // Validate each item has a taskId that exists in
                  // the open list. Drop hallucinated IDs silently.
                  const valid = parsed.plan
                    .filter((p) => p && typeof p.taskId === 'string' && tasks.some((t) => t.id === p.taskId))
                    .sort((a, b) => (a.order ?? 99) - (b.order ?? 99));
                  aiFocusPlan = valid;
                  aiFocusSkipped = typeof parsed.skipped_reasons === 'string' ? parsed.skipped_reasons : '';
                }
              } catch {
                // Partial JSON — wait for more chunks.
              }
            }
          },
          onError: (err) => { aiFocusError = err.message; }
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
    const start = new Date();
    // Round up to the next 15-minute boundary so the schedule reads
    // as 09:15 / 09:30 / 09:45 etc rather than awkward 09:07 stamps.
    const m = start.getMinutes();
    const remainder = m % 15;
    if (remainder !== 0) start.setMinutes(m + (15 - remainder), 0, 0);
    else start.setSeconds(0, 0);
    start.setMinutes(start.getMinutes() + earlier);
    const today = new Date().toISOString().slice(0, 10);
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
      toast.error('Pin failed: ' + (e instanceof Error ? e.message : String(e)));
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
      const msg = err instanceof Error ? err.message : String(err);
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

  // ── AI Stale-task verdict ───────────────────────────────────────
  // For tasks the user hasn't touched in 7+ days, ask the model:
  // "is this still real, or is it dead weight?" Returns a verdict
  // per task — keep / defer / archive — with a one-line rationale.
  // The user can accept-archive inline (sets done=true, triage='dropped')
  // or apply-defer (resets the updatedAt by patching nothing
  // material — actually, better: bump the snoozedUntil one week
  // forward so the task drops out of the stale view) or just keep
  // it (no-op).
  //
  // Goes through chatStream → /chat/stream so audit / sabbath /
  // redaction / cost all apply.
  type StaleVerdict = {
    taskId: string;
    verdict: 'keep' | 'defer' | 'archive';
    rationale: string;
  };
  let aiStaleBusy = $state(false);
  let aiStaleError = $state('');
  let aiStaleRaw = $state('');
  let aiStaleVerdicts = $state<StaleVerdict[]>([]);
  let aiStaleAbort: AbortController | null = null;
  let aiStaleApplyingId = $state<string>('');

  async function runAIStaleVerdict() {
    if (aiStaleBusy) return;
    aiStaleBusy = true;
    aiStaleError = '';
    aiStaleRaw = '';
    aiStaleVerdicts = [];
    aiStaleAbort = new AbortController();
    // Build the candidate list: every stale task in the current
    // filtered view, capped at 25 to keep prompt size sane. The
    // user is on the Stale view when they click this; the
    // filter set is what they see.
    const candidates = filtered.filter(isStale).slice(0, 25);
    if (candidates.length === 0) {
      aiStaleBusy = false;
      toast.info('No stale tasks to evaluate.');
      return;
    }
    const today = new Date().toISOString().slice(0, 10);
    const lines = candidates.map((t) => {
      const ageRef = t.updatedAt ?? t.createdAt ?? '';
      const ageDays = ageRef
        ? Math.floor((Date.now() - new Date(ageRef).getTime()) / 86_400_000)
        : 0;
      const bits: string[] = [`id:${t.id} — ${t.text}`];
      bits.push(`untouched ${ageDays}d`);
      if (t.priority) bits.push(`p${t.priority}`);
      if (t.dueDate) bits.push(`due ${t.dueDate}`);
      if (t.notes) bits.push(`notes:"${t.notes.slice(0, 80).replace(/\n/g, ' ')}"`);
      return bits.join(' · ');
    }).join('\n');
    const system =
      'You are an honest accountability partner reviewing a user\'s neglected tasks. ' +
      'For each task, return ONE verdict: "keep" (still real, schedule it), "defer" (real but not now — push out), or "archive" (dead weight — drop it). ' +
      'Hard rules: ' +
      '(1) Do not be polite. If a task has been ignored for 30+ days with no due date and no priority, it is almost certainly archive material — say so. ' +
      '(2) "keep" is for tasks where the rationale is "this still matters and the user is avoiding it" — you must say WHY it should be done. ' +
      '(3) "defer" is for real tasks that aren\'t time-critical right now (e.g. seasonal, blocked on someone else, premature). ' +
      '(4) "archive" is the default for anything vague, abandoned, or originating in a brainstorm that never went anywhere. ' +
      '(5) Each rationale is ONE sentence under 16 words. Examples of GOOD rationales: "Mentioned in 3 daily notes but never started — you\'re avoiding the hard conversation."; "Idea from a January brainstorm; nothing else attached. Dead weight."; "Real, but blocked until Q3 budget closes — defer to August." ' +
      '(6) Output STRICT JSON ONLY, no fences, no preamble. Schema: ' +
      '{"verdicts":[{"taskId":"<exact id>","verdict":"keep|defer|archive","rationale":"…"}]}.';
    const user =
      `Today is ${today}. Review these stale tasks. Use the EXACT taskId values; do not invent IDs.\n\n` +
      `Stale tasks (${candidates.length}):\n${lines}`;
    try {
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: user }
        ],
        undefined,
        {
          onChunk: (c) => {
            aiStaleRaw += c;
            const block = extractJsonBlock(aiStaleRaw);
            if (block) {
              try {
                const parsed = JSON.parse(block) as { verdicts?: StaleVerdict[] };
                if (Array.isArray(parsed.verdicts)) {
                  aiStaleVerdicts = parsed.verdicts.filter((v) =>
                    v && typeof v.taskId === 'string'
                    && (v.verdict === 'keep' || v.verdict === 'defer' || v.verdict === 'archive')
                    && tasks.some((t) => t.id === v.taskId)
                  );
                }
              } catch {}
            }
          },
          onError: (err) => { aiStaleError = err.message; }
        },
        aiStaleAbort.signal
      );
    } finally {
      aiStaleBusy = false;
      aiStaleAbort = null;
    }
  }
  function cancelAIStale() { aiStaleAbort?.abort(); }
  function dismissAIStale() {
    aiStaleRaw = '';
    aiStaleError = '';
    aiStaleVerdicts = [];
  }

  // Apply a verdict:
  //   archive → done=true + triage='dropped' (matches existing
  //             "drop this task" semantics from triage flow)
  //   defer   → snoozedUntil = today + 14 days (so it drops out of
  //             the stale view AND the open list until then)
  //   keep    → no-op; remove the verdict so the user can move on
  async function applyStaleVerdict(v: StaleVerdict) {
    aiStaleApplyingId = v.taskId;
    try {
      if (v.verdict === 'archive') {
        await api.patchTask(v.taskId, { done: true, triage: 'dropped' });
      } else if (v.verdict === 'defer') {
        const t = new Date();
        t.setDate(t.getDate() + 14);
        await api.patchTask(v.taskId, {
          snoozedUntil: t.toISOString(),
          triage: 'snoozed'
        });
      }
      // Drop the verdict regardless of action — the user has decided.
      aiStaleVerdicts = aiStaleVerdicts.filter((x) => x.taskId !== v.taskId);
      if (v.verdict !== 'keep') await load();
    } catch (e) {
      toast.error('Apply failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      aiStaleApplyingId = '';
    }
  }
  function skipStaleVerdict(taskId: string) {
    aiStaleVerdicts = aiStaleVerdicts.filter((x) => x.taskId !== taskId);
  }
  // Bulk archive counter for the panel header. Derived so the
  // "archive all 5" button label updates as the user accepts /
  // skips individual rows.
  let staleArchiveCount = $derived(
    aiStaleVerdicts.filter((v) => v.verdict === 'archive').length
  );
  async function archiveAllStaleVerdicts() {
    const items = aiStaleVerdicts.filter((v) => v.verdict === 'archive');
    for (const v of items) await applyStaleVerdict(v);
  }

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
      const msg = err instanceof Error ? err.message : String(err);
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
      toast.error('Apply failed: ' + (err instanceof Error ? err.message : String(err)));
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
          patch.dueDate = today.toISOString().slice(0, 10);
          break;
        }
        case 'tomorrow': {
          const t = new Date(today);
          t.setDate(t.getDate() + 1);
          patch.dueDate = t.toISOString().slice(0, 10);
          break;
        }
        case 'this_week': {
          // End of week — Sunday — at the latest.
          const t = new Date(today);
          const dow = t.getDay();
          const daysToSun = (7 - dow) % 7;
          t.setDate(t.getDate() + daysToSun);
          patch.dueDate = t.toISOString().slice(0, 10);
          break;
        }
        case 'next_week': {
          const t = new Date(today);
          t.setDate(t.getDate() + 7);
          patch.dueDate = t.toISOString().slice(0, 10);
          break;
        }
        // 'no_date' or anything else → leave dueDate alone.
      }
      await api.patchTask(p.id, patch);
      aiTriageProposals = aiTriageProposals.filter((x) => x.id !== p.id);
      saveProposals(TRIAGE_KEY, aiTriageProposals);
      await load();
    } catch (err) {
      toast.error('Apply failed: ' + (err instanceof Error ? err.message : String(err)));
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
      toast.error('Failed to add task: ' + (e instanceof Error ? e.message : String(e)));
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
    // Rehydrate any unprocessed AI proposals so a refresh / nav-away
    // doesn't burn the call. TTL-stale entries are dropped silently
    // by loadProposals.
    aiTriageProposals = loadProposals(TRIAGE_KEY);
    aiDeadlineProposals = loadProposals(DEADLINE_KEY);
  });

  onMount(() => {
    const unsub = onWsEvent((ev) => {
      // task.changed fires after every patchTask, including drag-drops
      // from the kanban — without it, moves would only show up on a
      // manual refresh (or the next note write coincidentally). Match
      // the same set the calendar/inbox widgets honor.
      if (ev.type === 'note.changed' || ev.type === 'note.removed' || ev.type === 'task.changed') load();
    });
    // Visibility-aware refresh: a backgrounded tab won't get WS events,
    // so a task ticked off on the phone while the desktop tab was
    // hidden would otherwise stay open here until reload. Catches the
    // cross-device case at zero recurring cost.
    const onVisible = () => {
      if (document.visibilityState === 'visible') load();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', onVisible);
    return () => {
      unsub();
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
      const today = new Date().toISOString().slice(0, 10);
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
    return out;
  });

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
  let collapsedIds = $state<Set<string>>(new Set());
  onMount(() => {
    try {
      const raw = localStorage.getItem(COLLAPSE_KEY);
      if (raw) {
        const arr = JSON.parse(raw) as string[];
        if (Array.isArray(arr)) collapsedIds = new Set(arr);
      }
    } catch {}
  });
  $effect(() => {
    void collapsedIds;
    try { localStorage.setItem(COLLAPSE_KEY, JSON.stringify(Array.from(collapsedIds))); } catch {}
  });

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
    tag: string;
    project: string;
    priority: number | '';
    goal: string;
    deadline: string;
    view: View;
    groupBy: Group;
  };
  const PRESETS_KEY = 'granit.tasks.presets';
  let presets = $state<FilterPreset[]>([]);
  onMount(() => {
    try {
      const raw = localStorage.getItem(PRESETS_KEY);
      if (raw) {
        const parsed = JSON.parse(raw);
        if (Array.isArray(parsed)) presets = parsed;
      }
    } catch {}
  });
  function persistPresets() {
    try { localStorage.setItem(PRESETS_KEY, JSON.stringify(presets)); } catch {}
  }
  function captureCurrentAsPreset() {
    const name = prompt('Name this filter preset:', '');
    if (!name || !name.trim()) return;
    const trimmed = name.trim();
    const next = presets.filter((p) => p.name !== trimmed);
    next.unshift({
      name: trimmed,
      status, q, tag: tagFilter, project: projectFilter,
      priority: priorityFilter, goal: goalFilter, deadline: deadlineFilter,
      view, groupBy
    });
    presets = next;
    persistPresets();
    toast.success(`Saved preset "${trimmed}"`);
  }
  function applyPreset(p: FilterPreset) {
    status = p.status; q = p.q; tagFilter = p.tag; projectFilter = p.project;
    priorityFilter = p.priority; goalFilter = p.goal; deadlineFilter = p.deadline;
    view = p.view; groupBy = p.groupBy;
  }
  function deletePreset(name: string) {
    presets = presets.filter((p) => p.name !== name);
    persistPresets();
  }
  function presetMatches(p: FilterPreset): boolean {
    return p.status === status && p.q === q && p.tag === tagFilter
      && p.project === projectFilter && p.priority === priorityFilter
      && p.goal === goalFilter && p.deadline === deadlineFilter
      && p.view === view && p.groupBy === groupBy;
  }

  let stats = $derived.by(() => {
    const today = new Date().toISOString().slice(0, 10);
    let open = 0, overdue = 0, todayCount = 0, doneToday = 0, snoozed = 0;
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
      } else if (t.completedAt && t.completedAt.slice(0, 10) === today) {
        doneToday++;
      }
    }
    return { open, overdue, todayCount, doneToday, snoozed };
  });

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
      const today = now.toISOString().slice(0, 10);
      const tmw = new Date(now);
      tmw.setDate(tmw.getDate() + 1);
      const tomorrow = tmw.toISOString().slice(0, 10);
      const wk = new Date(now);
      wk.setDate(wk.getDate() + 7);
      const weekEnd = wk.toISOString().slice(0, 10);
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
      // Sort each bucket by date asc, then priority asc — so the
      // user sees the most urgent / most important task first.
      const sortByDateThenPrio = (a: Task, x: Task) => {
        const ad = a.dueDate ?? (a.scheduledStart?.slice(0, 10) ?? '');
        const xd = x.dueDate ?? (x.scheduledStart?.slice(0, 10) ?? '');
        if (ad !== xd) return ad < xd ? -1 : 1;
        const ap = a.priority || 99;
        const xp = x.priority || 99;
        return ap - xp;
      };
      Object.values(b).forEach((arr) => arr.sort(sortByDateThenPrio));
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

    <!-- Source filter — hides `- [ ]` bullets that live in reading
         notes / brainstorm pages so the global task view doesn't get
         polluted by visual list bullets the user never meant as
         tasks. Default 'task-notes' looks at notes that look like
         task surfaces (daily notes, anything under Daily/, Tasks/,
         Projects/). Flip to 'all' to see every checkbox in the vault. -->
    <div>
      <div class="text-xs uppercase tracking-wider text-dim mb-2">Source</div>
      <div class="flex flex-col gap-1 text-sm">
        <button
          class="text-left px-3 py-2 rounded {sourceFilter === 'task-notes' ? 'bg-surface1 text-text' : 'text-subtext hover:bg-surface0'}"
          onclick={() => (sourceFilter = 'task-notes')}
          title="Daily notes, Tasks/, Projects/, Daily/ — skip bullets in arbitrary notes"
        >
          Task notes only
        </button>
        <button
          class="text-left px-3 py-2 rounded {sourceFilter === 'all' ? 'bg-surface1 text-text' : 'text-subtext hover:bg-surface0'}"
          onclick={() => (sourceFilter = 'all')}
          title="Show every - [ ] checkbox the parser found in the vault"
        >
          All notes
        </button>
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
            class="px-2 sm:px-3 py-1.5 {view === 'kanban' ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (view = 'kanban')}
          >Kanban</button>
        </div>
        <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-xs sm:text-sm">
          <button
            class="px-2 sm:px-3 py-1.5 inline-flex items-center gap-1 {view === 'inbox' ? 'bg-primary text-on-primary' : viewCounts.inbox > 0 ? 'text-text hover:bg-surface1' : 'text-dim hover:bg-surface1'}"
            onclick={() => (view = 'inbox')}
            title="untriaged tasks awaiting categorisation"
          >
            Inbox
            {#if viewCounts.inbox > 0 && view !== 'inbox'}
              <span class="text-[10px] tabular-nums text-secondary font-mono">{viewCounts.inbox}</span>
            {/if}
          </button>
          <button
            class="px-2 sm:px-3 py-1.5 hidden sm:inline-flex items-center gap-1 {view === 'quickwins' ? 'bg-primary text-on-primary' : viewCounts.quickwins > 0 ? 'text-text hover:bg-surface1' : 'text-dim hover:bg-surface1'}"
            onclick={() => (view = 'quickwins')}
            title="high priority + ≤30 min — tackle a few before lunch"
          >
            Quick wins
            {#if viewCounts.quickwins > 0 && view !== 'quickwins'}
              <span class="text-[10px] tabular-nums text-success font-mono">{viewCounts.quickwins}</span>
            {/if}
          </button>
          <button
            class="px-2 sm:px-3 py-1.5 hidden sm:inline-flex items-center gap-1 {view === 'stale' ? 'bg-primary text-on-primary' : viewCounts.stale > 0 ? 'text-text hover:bg-surface1' : 'text-dim hover:bg-surface1'}"
            onclick={() => (view = 'stale')}
            title="not touched in 7+ days — needs a decision"
          >
            Stale
            {#if viewCounts.stale > 0 && view !== 'stale'}
              <span class="text-[10px] tabular-nums text-warning font-mono">{viewCounts.stale}</span>
            {/if}
          </button>
          <button
            class="px-2 sm:px-3 py-1.5 hidden sm:inline-flex items-center gap-1 {view === 'review' ? 'bg-primary text-on-primary' : viewCounts.review > 0 ? 'text-text hover:bg-surface1' : 'text-dim hover:bg-surface1'}"
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
        <div class="px-3 py-3 border-b border-surface1 flex-shrink-0 bg-gradient-to-r from-primary/5 via-secondary/5 to-primary/5">
          <div class="flex items-baseline gap-2 mb-2 flex-wrap">
            <span class="text-xs uppercase tracking-wider text-secondary font-semibold">✨ Plan my day</span>
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
                      class="px-2 py-0.5 bg-success/15 text-success rounded hover:bg-success/25 flex-shrink-0"
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
          class="hidden sm:inline-flex px-3 py-2 text-sm bg-gradient-to-r from-primary/15 to-secondary/15 border border-primary/30 text-primary rounded hover:border-primary/60 disabled:opacity-50 flex-shrink-0 items-center gap-1"
        >
          <span>✨</span>
          <span>{aiFocusBusy ? 'planning…' : 'Plan day'}</span>
        </button>
      </div>
      <!-- Saved filter presets. One-click application of a stored
           filter combo. The "+ save" chip captures the current
           filter state under a name; clicking a preset chip
           re-applies all stored fields. Long-press / right-click to
           delete via the small × on the active chip. -->
      {#if presets.length > 0 || true}
        <div class="px-3 py-1.5 border-b border-surface1 flex items-center gap-1.5 text-xs flex-shrink-0 flex-wrap">
          <span class="text-dim font-mono uppercase tracking-wider">presets</span>
          {#each presets as p (p.name)}
            {@const active = presetMatches(p)}
            <span
              class="inline-flex items-center rounded overflow-hidden border
                {active ? 'border-primary bg-primary/10 text-primary' : 'border-surface1 bg-surface0 text-subtext hover:border-primary/40'}"
            >
              <button
                onclick={() => applyPreset(p)}
                class="px-2 py-0.5"
              >{p.name}</button>
              {#if active}
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
      <!-- Stats summary chips. Always reflect the unfiltered set so
           the user knows total load even with active filters. The
           overdue / today chips have urgency coloring; doneToday is
           a positive-tone affirmation; snoozed is muted. -->
      <div class="px-3 py-2 border-b border-surface1 flex items-center gap-1.5 text-xs flex-shrink-0 flex-wrap">
        <span class="px-2 py-1 rounded bg-surface0 text-subtext font-mono tabular-nums">
          <span class="text-text font-semibold">{stats.open}</span> open
        </span>
        {#if stats.overdue > 0}
          <span class="px-2 py-1 rounded bg-error/15 text-error font-mono tabular-nums" title="Tasks past their due date">
            <span class="font-semibold">{stats.overdue}</span> overdue
          </span>
        {/if}
        {#if stats.todayCount > 0}
          <span class="px-2 py-1 rounded bg-warning/15 text-warning font-mono tabular-nums" title="Tasks due today">
            <span class="font-semibold">{stats.todayCount}</span> today
          </span>
        {/if}
        {#if stats.doneToday > 0}
          <span class="px-2 py-1 rounded bg-success/15 text-success font-mono tabular-nums" title="Completed today">
            ✓ <span class="font-semibold">{stats.doneToday}</span>
          </span>
        {/if}
        {#if stats.snoozed > 0}
          <span class="px-2 py-1 rounded bg-surface0 text-dim font-mono tabular-nums" title="Currently snoozed">
            💤 {stats.snoozed}
          </span>
        {/if}
        <span class="flex-1"></span>
        {#if view === 'list'}
          <span class="text-dim">group</span>
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

    <div class="flex-1 overflow-auto p-3 sm:p-4">
      {#if loading && tasks.length === 0}
        <div class="text-sm text-dim">loading…</div>
      {:else if filtered.length === 0 && view === 'today'}
        <!-- Today view inbox-zero message. Different from a true empty
             state — the user has tasks, just none for today. The
             tone is calm-celebratory rather than the cobwebbed
             "get to work" used by the Review view. -->
        <div class="max-w-md mx-auto py-10 text-center">
          <div class="text-4xl mb-3 opacity-50">🌤</div>
          <h2 class="text-base font-medium text-text mb-1">Today is clear</h2>
          <p class="text-sm text-dim">
            Nothing overdue, nothing due today, nothing scheduled. Take the open space — or pick something from
            <button class="text-primary hover:underline" onclick={() => (view = 'list')}>the full list</button>.
          </p>
        </div>
      {:else if filtered.length === 0 && view === 'review'}
        <div class="text-sm text-dim italic">No tasks completed in the last 7 days. Get to work!</div>
      {:else if filtered.length === 0 && view === 'inbox'}
        <p class="text-sm text-success">Inbox empty 🎉 nothing waiting to be triaged.</p>
      {:else if filtered.length === 0 && view === 'stale'}
        <p class="text-sm text-success">No stale tasks — everything's been touched in the last week.</p>
      {:else if filtered.length === 0 && view === 'quickwins'}
        <p class="text-sm text-dim italic">No quick wins available. Add an estimate (e.g. <code class="text-secondary">est:30m</code>) to high-priority tasks.</p>
      {:else if filtered.length === 0 && tasks.length === 0}
        <!-- True empty: no tasks anywhere. Onboarding-style hint
             pointing at the quick-add bar. -->
        <div class="max-w-md mx-auto py-12 text-center">
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
        <div class="max-w-md mx-auto py-12 text-center">
          <div class="text-4xl mb-3 opacity-30">🔍</div>
          <h2 class="text-base font-medium text-text mb-2">No tasks match these filters</h2>
          <p class="text-sm text-dim mb-3">
            {tasks.length} {tasks.length === 1 ? 'task is' : 'tasks are'} hidden by the current filters.
          </p>
          <button
            onclick={() => {
              q = ''; tagFilter = ''; projectFilter = ''; priorityFilter = '';
              goalFilter = ''; deadlineFilter = ''; status = 'open';
            }}
            class="px-3 py-1.5 bg-surface0 border border-surface1 hover:border-primary rounded text-sm text-subtext"
          >Clear filters</button>
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
                class="px-3 py-1.5 text-xs bg-warning/15 text-warning rounded hover:bg-warning/25 flex-shrink-0"
                title="Cancel the in-flight triage call"
              >✨ thinking… cancel</button>
            {:else}
              <button
                onclick={() => void runAITriage()}
                disabled={filtered.length === 0}
                class="px-3 py-1.5 text-xs bg-secondary/15 text-secondary rounded hover:bg-secondary/25 disabled:opacity-50 flex-shrink-0"
                title="Ask AI to suggest priority + schedule for each untriaged task"
              >✨ AI triage</button>
            {/if}
            {#if aiDeadlineBusy}
              <button
                onclick={cancelAIDeadline}
                class="px-3 py-1.5 text-xs bg-warning/15 text-warning rounded hover:bg-warning/25 flex-shrink-0"
                title="Cancel the in-flight deadline scan"
              >✨ thinking… cancel</button>
            {:else}
              <button
                onclick={() => void runAIDeadlineDetect()}
                class="px-3 py-1.5 text-xs bg-secondary/15 text-secondary rounded hover:bg-secondary/25 disabled:opacity-50 flex-shrink-0"
                title="Scan all open tasks without a due date — propose ones whose title implies a clear deadline"
              >✨ Detect deadlines</button>
            {/if}
          </div>

          {#if aiDeadlineProposals.length > 0}
            <!-- Deadline proposals — operates across ALL open tasks
                 without a due_date, not just inbox. Server already
                 filtered out blanks, so every row is a confident
                 suggestion. Apply patches dueDate; skip just dismisses. -->
            <div class="mb-5 p-3 bg-warning/5 border border-warning/30 rounded">
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
                        class="px-2 py-0.5 bg-success/15 text-success rounded hover:bg-success/25"
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
            <div class="mb-5 p-3 bg-secondary/5 border border-secondary/30 rounded">
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
                        class="px-2 py-0.5 bg-success/15 text-success rounded hover:bg-success/25"
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
                <TaskCard task={t} hasChildren={(childCount.get(t.id) ?? 0) > 0} childCount={childCount.get(t.id) ?? 0} collapsed={collapsedIds.has(t.id)} onToggleCollapse={() => toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
              </div>
            {/each}
          </div>
        </div>
      {:else if view === 'stale'}
        <div class="max-w-3xl">
          <div class="flex items-baseline gap-3 mb-4">
            <p class="text-sm text-dim flex-1">Tasks that haven't been touched in 7+ days. Drop, snooze, or do them.</p>
            {#if aiStaleBusy}
              <button
                onclick={cancelAIStale}
                class="px-3 py-1.5 text-xs bg-warning/15 text-warning rounded hover:bg-warning/25 flex-shrink-0"
                title="Cancel the in-flight verdict scan"
              >✨ thinking… cancel</button>
            {:else if aiStaleVerdicts.length > 0 || aiStaleError || aiStaleRaw}
              <button
                onclick={() => void runAIStaleVerdict()}
                class="px-3 py-1.5 text-xs bg-secondary/15 text-secondary rounded hover:bg-secondary/25 flex-shrink-0"
                title="Re-evaluate stale tasks"
              >↻ re-scan</button>
              <button
                onclick={dismissAIStale}
                class="px-3 py-1.5 text-xs text-dim hover:text-error flex-shrink-0"
              >dismiss</button>
            {:else}
              <button
                onclick={() => void runAIStaleVerdict()}
                disabled={filtered.filter(isStale).length === 0}
                class="px-3 py-1.5 text-xs bg-secondary/15 text-secondary rounded hover:bg-secondary/25 disabled:opacity-50 flex-shrink-0"
                title="AI verdict on each stale task: keep, defer 2 weeks, or archive"
              >✨ AI verdicts</button>
            {/if}
          </div>

          {#if aiStaleError}
            <div class="mb-5 p-3 bg-error/5 border border-error/30 rounded text-xs text-error">
              {aiStaleError}
            </div>
          {/if}

          {#if aiStaleVerdicts.length > 0}
            <!-- Verdict panel. Each row: keep / defer / archive with
                 a one-line rationale. Accept-archive sets the task
                 done + triage='dropped'; defer snoozes 14 days; keep
                 is a no-op (just dismisses the row). User stays in
                 control — every action goes through applyStaleVerdict
                 which round-trips through patchTask + load. -->
            <div class="mb-5 p-3 bg-secondary/5 border border-secondary/30 rounded">
              <div class="flex items-center mb-2">
                <div class="text-xs uppercase tracking-wider text-secondary font-semibold flex-1">AI verdicts ({aiStaleVerdicts.length})</div>
                {#if staleArchiveCount > 1}
                  <button
                    onclick={() => void archiveAllStaleVerdicts()}
                    disabled={!!aiStaleApplyingId}
                    class="text-[10px] text-error hover:underline mr-2 disabled:opacity-50"
                    title="Archive all {staleArchiveCount} dead-weight tasks"
                  >archive all {staleArchiveCount}</button>
                {/if}
                <button
                  onclick={dismissAIStale}
                  class="text-[10px] text-dim hover:text-error"
                  title="Drop verdicts without applying"
                >discard</button>
              </div>
              <ul class="space-y-2">
                {#each aiStaleVerdicts as v (v.taskId)}
                  {@const t = tasks.find((x) => x.id === v.taskId)}
                  {#if t}
                    <!-- Static class lookup so Tailwind's purge keeps them.
                         Dynamic `text-{x}` would survive in dev but get
                         tree-shaken in prod. -->
                    {@const verdictClass = v.verdict === 'archive' ? 'text-error' : v.verdict === 'defer' ? 'text-warning' : 'text-success'}
                    <li class="flex items-start gap-2 text-xs">
                      <div class="flex-1 min-w-0">
                        <div class="text-text">{t.text}</div>
                        <div class="text-dim mt-0.5">
                          <span class="font-mono uppercase tracking-wider {verdictClass}">{v.verdict}</span>
                          {#if v.rationale}<span class="italic"> — {v.rationale}</span>{/if}
                        </div>
                      </div>
                      <button
                        onclick={() => void applyStaleVerdict(v)}
                        disabled={aiStaleApplyingId === v.taskId}
                        class="px-2 py-0.5 rounded flex-shrink-0
                          {v.verdict === 'archive' ? 'bg-error/15 text-error hover:bg-error/25' :
                           v.verdict === 'defer' ? 'bg-warning/15 text-warning hover:bg-warning/25' :
                           'bg-success/15 text-success hover:bg-success/25'}
                          disabled:opacity-50"
                        title={v.verdict === 'archive'
                          ? 'Drop the task — done=true, triage=dropped'
                          : v.verdict === 'defer'
                          ? 'Snooze 2 weeks'
                          : 'Acknowledge — keep on the list'}
                      >{aiStaleApplyingId === v.taskId ? '…' :
                        v.verdict === 'archive' ? 'archive' :
                        v.verdict === 'defer' ? 'defer 2w' : 'acknowledge'}</button>
                      <button
                        onclick={() => skipStaleVerdict(v.taskId)}
                        disabled={!!aiStaleApplyingId}
                        class="px-2 py-0.5 text-dim hover:text-text flex-shrink-0 disabled:opacity-50"
                      >skip</button>
                    </li>
                  {/if}
                {/each}
              </ul>
              <p class="text-[10px] text-dim italic mt-2 pt-2 border-t border-surface1">
                Verdicts are advisory — every action requires your accept. Defer = snooze 14 days; archive = done + dropped.
              </p>
            </div>
          {/if}

          <div class="space-y-2">
            {#each filtered.filter((tt) => !isHiddenByCollapse(tt.id, collapsedIds)) as t (t.id)}
              <div data-task-id={t.id} class={cursorIdx >= 0 && filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                <TaskCard task={t} hasChildren={(childCount.get(t.id) ?? 0) > 0} childCount={childCount.get(t.id) ?? 0} collapsed={collapsedIds.has(t.id)} onToggleCollapse={() => toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
              </div>
            {/each}
          </div>
        </div>
      {:else if view === 'quickwins'}
        <div class="max-w-3xl">
          <p class="text-sm text-dim mb-4">High-priority tasks you can finish in ≤30 min. Pick one, knock it out.</p>
          <div class="space-y-2">
            {#each filtered.filter((tt) => !isHiddenByCollapse(tt.id, collapsedIds)) as t (t.id)}
              <div data-task-id={t.id} class={cursorIdx >= 0 && filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                <TaskCard task={t} hasChildren={(childCount.get(t.id) ?? 0) > 0} childCount={childCount.get(t.id) ?? 0} collapsed={collapsedIds.has(t.id)} onToggleCollapse={() => toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
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
                <TaskCard task={t} hasChildren={(childCount.get(t.id) ?? 0) > 0} childCount={childCount.get(t.id) ?? 0} collapsed={collapsedIds.has(t.id)} onToggleCollapse={() => toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
              </div>
            {/each}
          </div>
        </div>
      {:else}
        <div class="space-y-6 max-w-3xl">
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
                  <span class="ml-1 px-1.5 py-0.5 bg-error/15 text-error text-[10px] tracking-wider rounded uppercase font-bold animate-pulse" title="These tasks are past their due date">
                    overdue
                  </span>
                {/if}
                {#if g.deepLink}
                  <a
                    href={g.deepLink}
                    class="ml-auto text-[10px] text-secondary hover:underline normal-case tracking-normal"
                    title="open {g.label}"
                  >open ↗</a>
                {/if}
              </h2>
              <div class="space-y-2">
                {#each g.tasks.filter((tt) => !isHiddenByCollapse(tt.id, collapsedIds)) as t (t.id)}
                  <div data-task-id={t.id} class={cursorIdx >= 0 && filtered[cursorIdx]?.id === t.id ? 'ring-2 ring-primary/40 rounded' : ''}>
                    <TaskCard task={t} hasChildren={(childCount.get(t.id) ?? 0) > 0} childCount={childCount.get(t.id) ?? 0} collapsed={collapsedIds.has(t.id)} onToggleCollapse={() => toggleCollapsed(t.id)} onChanged={load} bind:selectedIds onOpenDetail={openDetail} onContextMenu={openContext} />
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
