// AI-proposed-milestones controller for GoalDetail.
//
// Asks the model to break the goal down into 3-5 milestones in
// strict JSON, then renders each proposal as an accept/skip chip
// inline in the Milestones section. Distributes due dates evenly
// between today and target_date when one is given; otherwise omits
// the due_date.
//
// Goes through chatStream so each suggestion is logged with token
// counts in settings -> AI features.

import { api, type Goal } from '$lib/api';
import { errorMessage } from '$lib/util/errorMessage';
import { toast } from '$lib/components/toast';

export interface MilestoneProposal {
  text: string;
  due_date?: string;
}

export interface GoalDetailAIMilestonesController {
  readonly busy: boolean;
  readonly proposals: MilestoneProposal[];
  readonly error: string;
  /** Fire a fresh suggestion stream against the live goal. */
  suggest(): Promise<void>;
  /** Abort the in-flight stream (Stop CTA — keeps the previous
   *  proposals visible). */
  cancel(): void;
  /** Persist a proposal as a real milestone on the goal, then drop
   *  it from the proposal list. */
  accept(p: MilestoneProposal): Promise<void>;
  /** Drop a proposal without persisting. */
  skip(p: MilestoneProposal): void;
}

export interface GoalDetailAIMilestonesDeps {
  getGoal: () => Goal | null;
  /** Reload the parent's goal view after a successful accept. */
  onUpdated: () => void | Promise<void>;
}

export function createGoalDetailAIMilestones(
  deps: GoalDetailAIMilestonesDeps
): GoalDetailAIMilestonesController {
  let busy = $state(false);
  let proposals = $state<MilestoneProposal[]>([]);
  let error = $state('');
  let abort: AbortController | null = null;

  async function suggest() {
    const goal = deps.getGoal();
    if (!goal || busy) return;
    busy = true;
    error = '';
    proposals = [];
    abort = new AbortController();
    const existing = (goal.milestones ?? [])
      .map((m) => `- ${m.text}${m.due_date ? ` (due ${m.due_date})` : ''}`)
      .join('\n');
    const ctx = [
      `Goal: ${goal.title}`,
      goal.description ? `Description: ${goal.description}` : '',
      goal.target_date ? `Target date: ${goal.target_date}` : '',
      existing ? `Existing milestones:\n${existing}` : 'No milestones yet.'
    ].filter(Boolean).join('\n\n');
    const userMessage =
      'Suggest 3-5 concrete milestones to break this goal down into trackable, finishable steps. ' +
      "Each milestone should be specific enough that the user knows when it's done. " +
      'Distribute due dates evenly between today and the target date when one is given; otherwise omit due_date.\n\n' +
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
        .filter((p: unknown) => p && typeof p === 'object' && typeof (p as MilestoneProposal).text === 'string')
        .map((p) => ({
          text: (p as MilestoneProposal).text.trim(),
          due_date: typeof (p as MilestoneProposal).due_date === 'string' ? (p as MilestoneProposal).due_date : undefined
        }))
        .slice(0, 5);
    } catch (err) {
      if (!error) {
        error = `Couldn't parse suggestions: ${errorMessage(err)}`;
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

  async function accept(p: MilestoneProposal) {
    const goal = deps.getGoal();
    if (!goal) return;
    try {
      await api.addGoalMilestone(goal.id, { text: p.text, due_date: p.due_date });
      proposals = proposals.filter((x) => x !== p);
      await deps.onUpdated();
    } catch (e) {
      toast.error('add failed: ' + errorMessage(e));
    }
  }

  function skip(p: MilestoneProposal) {
    proposals = proposals.filter((x) => x !== p);
  }

  return {
    get busy() { return busy; },
    get proposals() { return proposals; },
    get error() { return error; },
    suggest,
    cancel,
    accept,
    skip
  };
}
