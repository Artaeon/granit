// AI orchestration stores for the tasks page. Extracted from
// tasks/+page.svelte to keep the page concerned with rendering +
// page-local UI state, not with the conversational/agentic AI
// flows that happen to live on it.
//
// Three independent flows live here, each owning its own busy /
// proposals / abort triad:
//
//   - triageStore: /api/v1/ai/inbox-triage → priority + schedule
//     proposals applied per-card.
//   - deadlineStore: /api/v1/ai/deadline-detect → dueDate proposals
//     for open tasks missing a due date.
//   - focusPlanStore: chatStream "plan my day" → ordered PlanItem[]
//     pinned into back-to-back scheduledStart slots.
//
// The conversational TaskAgent and the per-stale-task verdict
// surface (AIStaleVerdicts.svelte) own their own internal state
// and are NOT routed through this module — they're already
// extracted as components and the page passes them props.
//
// Why factory functions instead of module-level singletons:
// the page mounts/unmounts (auth gate, route guards, hot-reload),
// and these flows shouldn't share state across mounts. Each
// factory returns a Svelte store object the page subscribes to
// via `$store` syntax; reassignment in the .svelte file goes
// through the factory's methods, not direct setter writes.
//
// Why Svelte stores instead of $state: $state is component-scoped
// (it relies on Svelte's compiler-injected reactivity context),
// so it can't be reassigned from a .ts file outside that context.
// writable() works the same in plain TS and from any .svelte
// caller, which is the contract we need here.

import { writable, get, type Readable } from 'svelte/store';
import { api, todayISO, fmtDateISO, type Task, type AITriageProposal, type AIDeadlineProposal } from '$lib/api';
import { toast } from '$lib/components/toast';
import { errorMessage } from '$lib/util/errorMessage';
import { saveProposals, loadProposals } from '$lib/util/proposalCache';
import { extractJsonBlock } from '$lib/util/jsonExtract';
import { rafThrottle } from '$lib/util/streamThrottle';
import { isAbortError } from '$lib/util/aiErrors';
import {
  buildPlanDayPrompt,
  roundUpTo15Min,
  validatePlanItems,
  type PlanItem
} from '$lib/tasks/aiPrompts';

// Cache keys live with the stores that own them — the page no
// longer cares where these strings live, only that hydrate() /
// the run methods keep them in sync.
const TRIAGE_KEY = 'granit.ai.triage.proposals';
const DEADLINE_KEY = 'granit.ai.deadlines.proposals';

// ── Shared helpers ─────────────────────────────────────────────────

// Map an AI feature-toggle error into a user-readable hint pointing
// at Settings → AI features. Same wording the inline code used
// before extraction; preserved verbatim so the user's mental model
// of the message doesn't shift.
function aiFeatureToast(action: string, msg: string): void {
  toast.error(
    /disabled in AI preferences/i.test(msg)
      ? `Enable "${action}" in Settings → AI features first.`
      : `${action} failed: ${msg}`
  );
}

// ── Triage store ───────────────────────────────────────────────────

export interface TriageState {
  busy: boolean;
  proposals: AITriageProposal[];
}

export interface TriageStore extends Readable<TriageState> {
  /** Trigger /ai/inbox-triage and cache the resulting proposals. */
  run(): Promise<void>;
  /** Abort an in-flight run. No-op if nothing is in flight. */
  cancel(): void;
  /** Apply a single proposal: patch priority + dueDate + triage state. */
  apply(p: AITriageProposal, onChanged: () => Promise<void> | void): Promise<void>;
  /** Drop one proposal locally without applying it. */
  skip(id: string): void;
  /** Drop every cached proposal at once. */
  discard(): void;
  /** Re-read cached proposals from localStorage (idempotent). */
  hydrate(): void;
}

export function createTriageStore(): TriageStore {
  const state = writable<TriageState>({ busy: false, proposals: [] });
  let abort: AbortController | null = null;

  async function run(): Promise<void> {
    state.update((s) => ({ ...s, busy: true }));
    abort = new AbortController();
    try {
      const r = await api.aiInboxTriage(abort.signal);
      const proposals = r.proposals ?? [];
      state.update((s) => ({ ...s, proposals }));
      saveProposals(TRIAGE_KEY, proposals);
      if (proposals.length === 0) {
        if (r.warning) toast.warning(r.warning);
        else toast.info('No suggestions returned.');
      }
    } catch (err) {
      const msg = errorMessage(err);
      if (isAbortError(err)) {
        toast.info('Triage cancelled.');
      } else {
        aiFeatureToast('Inbox triage', msg);
      }
    } finally {
      state.update((s) => ({ ...s, busy: false }));
      abort = null;
    }
  }

  function cancel(): void {
    abort?.abort();
  }

  // Translate the AI's `schedule` keyword into a concrete dueDate.
  // Identical to the page-local implementation pre-extraction —
  // any change here is a behaviour change for every accepted
  // proposal, so keep this mapping in lock-step with the prompt's
  // documented schedule vocabulary.
  function scheduleToDueDate(schedule: string): string | undefined {
    const today = new Date();
    switch (schedule) {
      case 'today':
        return fmtDateISO(today);
      case 'tomorrow': {
        const t = new Date(today);
        t.setDate(t.getDate() + 1);
        return fmtDateISO(t);
      }
      case 'this_week': {
        // End of week — Sunday — at the latest.
        const t = new Date(today);
        const dow = t.getDay();
        const daysToSun = (7 - dow) % 7;
        t.setDate(t.getDate() + daysToSun);
        return fmtDateISO(t);
      }
      case 'next_week': {
        const t = new Date(today);
        t.setDate(t.getDate() + 7);
        return fmtDateISO(t);
      }
      // 'no_date' or anything else → leave dueDate alone.
      default:
        return undefined;
    }
  }

  async function apply(
    p: AITriageProposal,
    onChanged: () => Promise<void> | void
  ): Promise<void> {
    state.update((s) => ({ ...s, busy: true }));
    try {
      const patch: Parameters<typeof api.patchTask>[1] = {};
      if (p.priority === 0) {
        patch.done = true;
        patch.triage = 'dropped';
      } else {
        patch.priority = p.priority;
        patch.triage = 'triaged';
      }
      const due = scheduleToDueDate(p.schedule);
      if (due) patch.dueDate = due;
      await api.patchTask(p.id, patch);
      const next = get(state).proposals.filter((x) => x.id !== p.id);
      state.update((s) => ({ ...s, proposals: next }));
      saveProposals(TRIAGE_KEY, next);
      await onChanged();
    } catch (err) {
      toast.error('Apply failed: ' + errorMessage(err));
    } finally {
      state.update((s) => ({ ...s, busy: false }));
    }
  }

  function skip(id: string): void {
    const next = get(state).proposals.filter((p) => p.id !== id);
    state.update((s) => ({ ...s, proposals: next }));
    saveProposals(TRIAGE_KEY, next);
  }

  function discard(): void {
    state.update((s) => ({ ...s, proposals: [] }));
    saveProposals(TRIAGE_KEY, []);
  }

  function hydrate(): void {
    const items = loadProposals<AITriageProposal>(TRIAGE_KEY);
    state.update((s) => ({ ...s, proposals: items }));
  }

  return { subscribe: state.subscribe, run, cancel, apply, skip, discard, hydrate };
}

// ── Deadline store ─────────────────────────────────────────────────

export interface DeadlineState {
  busy: boolean;
  proposals: AIDeadlineProposal[];
}

export interface DeadlineStore extends Readable<DeadlineState> {
  run(): Promise<void>;
  cancel(): void;
  apply(p: AIDeadlineProposal, onChanged: () => Promise<void> | void): Promise<void>;
  skip(id: string): void;
  discard(): void;
  hydrate(): void;
}

export function createDeadlineStore(): DeadlineStore {
  const state = writable<DeadlineState>({ busy: false, proposals: [] });
  let abort: AbortController | null = null;

  async function run(): Promise<void> {
    state.update((s) => ({ ...s, busy: true }));
    abort = new AbortController();
    try {
      const r = await api.aiDeadlineDetect(abort.signal);
      const proposals = r.proposals ?? [];
      state.update((s) => ({ ...s, proposals }));
      saveProposals(DEADLINE_KEY, proposals);
      if (proposals.length === 0) {
        if (r.warning) toast.warning(r.warning);
        else toast.info('No clear deadlines detected.');
      }
    } catch (err) {
      const msg = errorMessage(err);
      if (isAbortError(err)) {
        toast.info('Detect cancelled.');
      } else {
        aiFeatureToast('Deadline detect', msg);
      }
    } finally {
      state.update((s) => ({ ...s, busy: false }));
      abort = null;
    }
  }

  function cancel(): void {
    abort?.abort();
  }

  async function apply(
    p: AIDeadlineProposal,
    onChanged: () => Promise<void> | void
  ): Promise<void> {
    state.update((s) => ({ ...s, busy: true }));
    try {
      await api.patchTask(p.id, { dueDate: p.due_date });
      const next = get(state).proposals.filter((x) => x.id !== p.id);
      state.update((s) => ({ ...s, proposals: next }));
      saveProposals(DEADLINE_KEY, next);
      await onChanged();
    } catch (err) {
      toast.error('Apply failed: ' + errorMessage(err));
    } finally {
      state.update((s) => ({ ...s, busy: false }));
    }
  }

  function skip(id: string): void {
    const next = get(state).proposals.filter((p) => p.id !== id);
    state.update((s) => ({ ...s, proposals: next }));
    saveProposals(DEADLINE_KEY, next);
  }

  function discard(): void {
    state.update((s) => ({ ...s, proposals: [] }));
    saveProposals(DEADLINE_KEY, []);
  }

  function hydrate(): void {
    const items = loadProposals<AIDeadlineProposal>(DEADLINE_KEY);
    state.update((s) => ({ ...s, proposals: items }));
  }

  return { subscribe: state.subscribe, run, cancel, apply, skip, discard, hydrate };
}

// ── Focus-plan store (Plan my day) ─────────────────────────────────

export interface FocusPlanState {
  busy: boolean;
  error: string;
  /** Raw streamed model response — shown verbatim while JSON is partial. */
  response: string;
  /** Validated, ordered plan items the user accepts/skips per row. */
  plan: PlanItem[];
  /** Free-text "skipped_reasons" the model returns alongside the plan. */
  skipped: string;
}

export interface FocusPlanStore extends Readable<FocusPlanState> {
  /**
   * Build + send the plan-my-day prompt. Snapshots `tasks` and
   * `focusHours` at call-time, so caller re-invokes to get a
   * fresh plan against an updated task list / focus budget.
   */
  run(tasks: Task[], focusHours: number): Promise<void>;
  cancel(): void;
  /** Clear all four fields back to their pristine state. */
  dismiss(): void;
  /**
   * Accept a single plan item: schedule it at "now + cumulative
   * estimates of earlier items" rounded to 15 min. Removes the
   * accepted item from the local plan so the panel shrinks as the
   * user works through it.
   */
  acceptItem(p: PlanItem, tasks: Task[], onChanged: () => Promise<void> | void): Promise<void>;
  /** Drop one plan item without scheduling anything. */
  skipItem(taskId: string): void;
  /** Accept every remaining item in order, back-to-back. */
  acceptAll(tasks: Task[], onChanged: () => Promise<void> | void): Promise<void>;
}

export function createFocusPlanStore(): FocusPlanStore {
  const state = writable<FocusPlanState>({
    busy: false,
    error: '',
    response: '',
    plan: [],
    skipped: ''
  });
  let abort: AbortController | null = null;

  async function run(tasks: Task[], focusHours: number): Promise<void> {
    if (get(state).busy) return;
    state.set({ busy: true, error: '', response: '', plan: [], skipped: '' });
    abort = new AbortController();
    const { system, user: userMessage } = buildPlanDayPrompt(tasks, todayISO(), focusHours);
    // rAF throttle — response is rendered live through
    // MarkdownRenderer in the streaming branch. Pre-extraction this
    // re-parsed the whole growing buffer per chunk → main thread
    // choke. Throttle commits the latest buffer + tries the JSON
    // parse at most once per frame.
    const t = rafThrottle((full) => {
      state.update((s) => ({ ...s, response: full }));
      const block = extractJsonBlock(full);
      if (!block) return;
      try {
        const parsed = JSON.parse(block) as { plan?: PlanItem[]; skipped_reasons?: string };
        if (Array.isArray(parsed.plan)) {
          const validated = validatePlanItems(parsed.plan, tasks);
          const skipped = typeof parsed.skipped_reasons === 'string' ? parsed.skipped_reasons : '';
          state.update((s) => ({ ...s, plan: validated, skipped }));
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
          onError: (err) => { t.flush(); state.update((s) => ({ ...s, error: err.message })); }
        },
        abort.signal
      );
    } finally {
      state.update((s) => ({ ...s, busy: false }));
      abort = null;
    }
  }

  function cancel(): void {
    abort?.abort();
  }

  function dismiss(): void {
    state.update((s) => ({ ...s, response: '', error: '', plan: [], skipped: '' }));
  }

  async function acceptItem(
    p: PlanItem,
    tasks: Task[],
    onChanged: () => Promise<void> | void
  ): Promise<void> {
    const t = tasks.find((x) => x.id === p.taskId);
    if (!t) return;
    // Find the cumulative offset for this item: sum estimateMinutes
    // of preceding plan items (by .order). We don't know which the
    // user already pinned, so we treat the FULL plan as the schedule
    // skeleton — accepting #2 alone still places it after #1's slot
    // (so the day reads coherently if the user accepts more later).
    const currentPlan = get(state).plan;
    const earlier = currentPlan
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
      state.update((s) => ({ ...s, plan: s.plan.filter((x) => x.taskId !== p.taskId) }));
      await onChanged();
      toast.success(`Pinned: ${t.text.slice(0, 40)}${t.text.length > 40 ? '…' : ''}`);
    } catch (e) {
      toast.error('Pin failed: ' + errorMessage(e));
    }
  }

  function skipItem(taskId: string): void {
    state.update((s) => ({ ...s, plan: s.plan.filter((x) => x.taskId !== taskId) }));
  }

  async function acceptAll(
    tasks: Task[],
    onChanged: () => Promise<void> | void
  ): Promise<void> {
    // Snapshot + sort before iterating: acceptItem mutates the
    // store's plan array, so a "live" iteration would drop entries
    // mid-flight. The cumulative-offset math inside acceptItem
    // still reads the current plan, which is fine — earlier items
    // are removed as they're accepted, so the offset shrinks
    // monotonically the same way it did pre-extraction.
    const items = [...get(state).plan].sort((a, b) => (a.order ?? 99) - (b.order ?? 99));
    for (const p of items) {
      await acceptItem(p, tasks, onChanged);
    }
  }

  return { subscribe: state.subscribe, run, cancel, dismiss, acceptItem, skipItem, acceptAll };
}
