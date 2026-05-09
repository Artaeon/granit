// AI prompts + types for the tasks-page agents. Lifted out of the
// tasks/+page.svelte god-file so the prompt strings — which are
// the actual product behaviour, not UI plumbing — live in a single
// reviewable surface. Each builder returns the {system, user}
// pair fed to api.chatStream; the page handles the streaming +
// state mutation.
//
// Why prompt extraction instead of full agent extraction: the
// agents themselves bind tight to page state (tasks list, focus
// hours, plan results), and a clean extraction would need a full
// shared-state contract. Prompts are the most-changed and most-
// reviewed piece — pulling them out gives 80% of the readability
// benefit at 0% of the refactor risk.

import type { Task } from '$lib/api';

export type PlanItem = {
  taskId: string;
  order: number;
  estimateMinutes: number;
  rationale: string;
};

export type StaleVerdict = {
  taskId: string;
  verdict: 'keep' | 'defer' | 'archive';
  rationale: string;
};

/**
 * Stale-task accountability prompt. Reviews tasks the user hasn't
 * touched in 7+ days and returns one verdict per row — keep / defer
 * / archive — with a one-sentence rationale. Pushes the model to
 * archive aggressively rather than be polite (a vague "keep" on
 * abandoned ideas defeats the point of the review).
 */
export function buildStaleVerdictPrompt(
  candidates: Task[],
  todayISO: string
): { system: string; user: string } {
  const lines = candidates
    .map((t) => {
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
    })
    .join('\n');
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
    `Today is ${todayISO}. Review these stale tasks. Use the EXACT taskId values; do not invent IDs.\n\n` +
    `Stale tasks (${candidates.length}):\n${lines}`;
  return { system, user };
}

/**
 * Validate a streamed verdicts array against the live task list +
 * the allowed verdict enum. Drops malformed entries silently — the
 * UI shouldn't render verdicts that no longer correspond to real
 * tasks or carry an unknown decision.
 */
export function validateStaleVerdicts(items: StaleVerdict[], liveTasks: Task[]): StaleVerdict[] {
  return items.filter(
    (v) =>
      v &&
      typeof v.taskId === 'string' &&
      (v.verdict === 'keep' || v.verdict === 'defer' || v.verdict === 'archive') &&
      liveTasks.some((t) => t.id === v.taskId)
  );
}

/**
 * Plan-my-day. Picks 3-7 tasks for today bounded by the user's
 * declared focus minutes, in run order. Returns strict JSON so the
 * UI can render an accept-each-row panel without parsing prose.
 *
 * The prompt is deliberately sharp about refusing the "everything
 * is important" trap — a vague plan is worse than no plan, so we
 * push the model to drop tasks rather than shrink estimates.
 */
export function buildPlanDayPrompt(
  tasks: Task[],
  todayISO: string,
  focusHours: number
): { system: string; user: string } {
  const focusMinutes = Math.max(30, Math.round(focusHours * 60));
  const open = tasks.filter((t) => !t.done).slice(0, 30);
  const lines = open
    .map((t) => {
      const bits: string[] = [`id:${t.id} — ${t.text}`];
      if (t.priority) bits.push(`p${t.priority}`);
      if (t.dueDate) bits.push(`due ${t.dueDate}`);
      if (t.scheduledStart) bits.push(`scheduled ${t.scheduledStart.slice(0, 10)}`);
      if (t.estimatedMinutes) bits.push(`est ${t.estimatedMinutes}m`);
      return bits.join(' · ');
    })
    .join('\n');
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
  const user =
    `Today is ${todayISO}. The user has roughly ${focusHours} hour${focusHours === 1 ? '' : 's'} (~${focusMinutes} minutes) of focus time today. ` +
    'Build a plan from their open tasks below. Use the EXACT taskId values in the JSON; do not invent new ones.\n\n' +
    'Open tasks:\n\n' +
    lines;
  return { system, user };
}

/**
 * Validate + sort a streamed plan against the live task list.
 * Drops items whose taskId doesn't exist (model hallucinations) and
 * orders by `order` so the UI can render confidently.
 */
export function validatePlanItems(items: PlanItem[], liveTasks: Task[]): PlanItem[] {
  return items
    .filter((p) => p && typeof p.taskId === 'string' && liveTasks.some((t) => t.id === p.taskId))
    .sort((a, b) => (a.order ?? 99) - (b.order ?? 99));
}

/**
 * Round a Date up to the next 15-minute boundary (mutates).
 * Used by acceptPlanItem so pinned tasks land on clean wall-clock
 * marks (09:15 / 09:30) rather than awkward 09:07 stamps.
 */
export function roundUpTo15Min(d: Date): Date {
  const m = d.getMinutes();
  const remainder = m % 15;
  if (remainder !== 0) d.setMinutes(m + (15 - remainder), 0, 0);
  else d.setSeconds(0, 0);
  return d;
}
