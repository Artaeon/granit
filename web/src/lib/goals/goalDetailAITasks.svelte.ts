// AI-proposed-this-week-tasks controller for GoalDetail.
//
// Asks the model for 3-5 concrete tasks the user could DO THIS WEEK
// to advance the goal. Distinct from the milestone suggester:
// milestones are the structural breakdown ("Ship v1 to 5 beta
// users"); tasks are the concrete work ("Email 3 candidates for
// beta", "Draft onboarding email"). A goal with rich milestones still
// needs this — the user knows the milestone ladder but freezes on
// what to do Monday morning.
//
// Accepted proposals are persisted as real tasks under today's
// daily note (same convention /tasks quick-add uses) with goalId
// set so they show up in the goal's roll-up + burn-up immediately.

import { api, fmtDateISO, type Goal } from '$lib/api';
import { errorMessage } from '$lib/util/errorMessage';
import { toast } from '$lib/components/toast';

export interface TaskProposal {
  text: string;
  dueDate?: string;
  /** Local UI state — proposal is in edit mode. The template binds
   *  the row's input to proposal.text directly so an edit persists
   *  to the proposal in place. */
  edit: boolean;
}

export interface GoalDetailAITasksController {
  readonly busy: boolean;
  readonly proposals: TaskProposal[];
  readonly error: string;
  /** Fire a fresh suggestion stream against the live goal. */
  suggest(): Promise<void>;
  /** Abort the in-flight stream — keeps any existing proposals
   *  visible (Stop convention). */
  cancel(): void;
  /** Persist a proposal as a real task in today's daily note, then
   *  drop it from the proposal list. */
  accept(p: TaskProposal): Promise<void>;
  /** Drop a proposal without persisting. */
  skip(p: TaskProposal): void;
  /** Persist every proposal in one batch. Useful for "looks all
   *  good, ship it". */
  acceptAll(): Promise<void>;
}

export interface GoalDetailAITasksDeps {
  getGoal: () => Goal | null;
  getOpenTaskCount: () => number;
  getDoneTaskCount: () => number;
  /** Reload the goal's linked tasks after a successful accept. */
  reloadGoalTasks: () => Promise<void>;
  /** Reload the parent's goal view after a successful accept. */
  onUpdated: () => void | Promise<void>;
}

export function createGoalDetailAITasks(
  deps: GoalDetailAITasksDeps
): GoalDetailAITasksController {
  let busy = $state(false);
  let proposals = $state<TaskProposal[]>([]);
  let error = $state('');
  let abort: AbortController | null = null;

  function todayPlusDays(n: number): string {
    const d = new Date();
    d.setDate(d.getDate() + n);
    return fmtDateISO(d);
  }

  async function suggest() {
    const goal = deps.getGoal();
    if (!goal || busy) return;
    busy = true;
    error = '';
    proposals = [];
    abort = new AbortController();

    const ms = goal.milestones ?? [];
    const openMs = ms.filter((m) => !m.done).slice(0, 8).map((m) => m.text);
    const doneMs = ms.filter((m) => m.done).slice(-4).map((m) => m.text);
    const today = todayPlusDays(0);
    const friday = (() => {
      // Coming Friday — falls back to today+7 if it's already weekend.
      const d = new Date();
      const dow = d.getDay(); // 0 Sun … 5 Fri
      const delta = dow <= 5 ? 5 - dow : 7 - dow + 5;
      d.setDate(d.getDate() + (delta === 0 ? 5 : delta));
      return fmtDateISO(d);
    })();

    const ctx = [
      `Goal: ${goal.title}`,
      goal.description ? `Description: ${goal.description}` : '',
      goal.target_date ? `Target date: ${goal.target_date}` : '',
      goal.venture ? `Venture: ${goal.venture}` : '',
      goal.project ? `Project: ${goal.project}` : '',
      openMs.length > 0 ? `Open milestones:\n${openMs.map((m) => `- ${m}`).join('\n')}` : '',
      doneMs.length > 0 ? `Recent done milestones:\n${doneMs.map((m) => `- ${m}`).join('\n')}` : '',
      `Linked tasks: ${deps.getOpenTaskCount()} open, ${deps.getDoneTaskCount()} done`
    ].filter(Boolean).join('\n\n');

    const userMessage =
      'You are a founder-coach. The user has a goal and wants 3-5 concrete TASKS they could DO THIS WEEK to advance it.\n\n' +
      'Rules for each task:\n' +
      '- Action-oriented, starts with a verb (Email, Draft, Call, Outline, Ship, Interview, Sketch, …).\n' +
      '- Doable in one sitting (≤ 2h). NOT a milestone, NOT a vague intention.\n' +
      "- Specific enough that the user knows when it's done.\n" +
      '- Distinct from open milestones above — these are the WORK that closes a milestone, not a restatement of one.\n' +
      '- One line, ≤ 14 words. No quotes, no period, no bullet.\n' +
      '- Set due_date to a specific weekday this week (today is ' + today + ', this Friday is ' + friday + "). Distribute due dates so they don't all land on the same day. Format YYYY-MM-DD.\n\n" +
      'Return STRICT JSON ONLY (no markdown fences, no preamble), shape:\n' +
      '[{"text": "...", "due_date": "YYYY-MM-DD"}, ...]\n\n' +
      'Goal context:\n\n' + ctx;

    let acc = '';
    try {
      await api.chatStream(
        [{ role: 'user', content: userMessage }],
        undefined,
        {
          onChunk: (c) => { acc += c; },
          onError: (err) => { error = err.message; }
        },
        abort.signal
      );
      let cleaned = acc.trim();
      if (cleaned.startsWith('```')) {
        cleaned = cleaned.replace(/^```(?:json)?\s*/, '').replace(/```\s*$/, '').trim();
      }
      const parsed = JSON.parse(cleaned);
      if (!Array.isArray(parsed)) throw new Error('expected array');
      proposals = parsed
        .filter((p: unknown) => p && typeof p === 'object' && typeof (p as { text?: unknown }).text === 'string')
        .map((p) => {
          const obj = p as { text: string; due_date?: unknown };
          return {
            text: obj.text.trim(),
            dueDate: typeof obj.due_date === 'string' && obj.due_date ? obj.due_date : undefined,
            edit: false
          } satisfies TaskProposal;
        })
        .slice(0, 5);
      if (proposals.length === 0 && !error) {
        error = 'AI returned no task proposals.';
      }
    } catch (err) {
      if (!error) {
        error = `Couldn't parse tasks: ${errorMessage(err)}`;
      }
    } finally {
      busy = false;
      abort = null;
    }
  }

  // Stop — abort the stream but keep the partial proposals + error
  // for retry. Flip busy + null abort synchronously so the "stop"
  // button swaps to "suggest" instantly.
  function cancel() {
    abort?.abort();
    abort = null;
    busy = false;
  }

  async function accept(p: TaskProposal) {
    const goal = deps.getGoal();
    if (!goal || !p.text.trim()) return;
    try {
      // Created in today's daily note, same convention /tasks
      // quick-add uses. The goalId link is what makes it show up in
      // this goal's roll-up + burn-up next time we load.
      const daily = await api.daily('today');
      await api.createTask({
        notePath: daily.path,
        text: p.text.trim(),
        dueDate: p.dueDate || undefined,
        goalId: goal.id
      });
      proposals = proposals.filter((x) => x !== p);
      await deps.reloadGoalTasks();
      await deps.onUpdated();
      toast.success('task added');
    } catch (e) {
      toast.error('add task failed: ' + errorMessage(e));
    }
  }

  function skip(p: TaskProposal) {
    proposals = proposals.filter((x) => x !== p);
  }

  async function acceptAll() {
    const list = [...proposals];
    for (const p of list) await accept(p);
  }

  return {
    get busy() { return busy; },
    get proposals() { return proposals; },
    get error() { return error; },
    suggest,
    cancel,
    accept,
    skip,
    acceptAll
  };
}
