// Tasks Agent — conversational, structured-action AI for the
// /tasks page. The user types a free-text intent ("clean up
// anything older than two weeks", "schedule the deep work for
// Friday morning", "everything with 'admin' goes to low
// priority"), the model proposes a list of typed actions, the
// user accepts/rejects each.
//
// Why structured actions (not a chat that calls tools mid-stream):
// the audit-gated chatStream pipeline is already wired for plain
// streaming. Letting the AI return STRICT JSON of typed actions
// lets us reuse that pipeline AND keep the user in control —
// every change is a card the user must approve. No silent
// mutations.
//
// Why a focused action enum (not "edit any field"): forced
// constraint pushes the model to make BIG SHIFT decisions ("this
// is a P3 not a P1") instead of fiddling with prose ("update the
// notes to clarify..."). The list mirrors what patchTask
// supports, so applying an action is a single PATCH.

import type { Task } from '$lib/api';
import {
	mergeProposals as coreMerge,
	extractActions,
	type ProposalFlags
} from '$lib/agents/core';

/** The action enum the AI is allowed to propose. Keep this in
 *  sync with the apply() helper below — adding a new action means
 *  (a) listing it here, (b) teaching the prompt about it,
 *  (c) handling it in applyAgentAction. */
export type TaskActionKind =
	| 'set_priority'
	| 'set_due'
	| 'clear_due'
	| 'schedule'
	| 'clear_schedule'
	| 'mark_done'
	| 'archive'
	| 'unarchive'
	| 'snooze'
	| 'set_project'
	| 'change_text';

export interface TaskAction {
	taskId: string;
	kind: TaskActionKind;
	/** Set-shaped action argument. One field per action kind; only
	 *  the relevant one is read on apply(). The model returns these
	 *  inline so a proposal JSON line is fully self-contained. */
	priority?: number; // set_priority
	dueDate?: string; // set_due, "YYYY-MM-DD"
	scheduledStart?: string; // schedule, ISO 8601
	durationMinutes?: number; // schedule
	snoozedUntil?: string; // snooze, "YYYY-MM-DD"
	projectId?: string; // set_project ("" to clear)
	text?: string; // change_text
	/** One-sentence justification surfaced under the action card.
	 *  Required — proposals without a rationale are dropped. */
	rationale: string;
}

const VALID_KINDS = new Set<TaskActionKind>([
	'set_priority',
	'set_due',
	'clear_due',
	'schedule',
	'clear_schedule',
	'mark_done',
	'archive',
	'unarchive',
	'snooze',
	'set_project',
	'change_text'
]);

/** buildAgentPrompt — assembles the system + user pair sent to
 *  chatStream. Includes the user's free-text intent + a compact
 *  digest of every task in scope (id + text + priority + due +
 *  schedule). Models without big context handle ~30-50 tasks
 *  cleanly; the caller is responsible for narrowing before
 *  passing in (use the page's `filtered` list, not the full
 *  vault). */
export function buildAgentPrompt(
	tasks: Task[],
	userText: string,
	todayISO: string,
	availableProjects: string[] = []
): { system: string; user: string } {
	const lines = tasks
		.map((t) => {
			const bits: string[] = [`id:${t.id} — ${t.text}`];
			if (t.done) bits.push('done');
			if (t.priority) bits.push(`p${t.priority}`);
			if (t.dueDate) bits.push(`due ${t.dueDate}`);
			if (t.scheduledStart) bits.push(`scheduled ${t.scheduledStart.slice(0, 10)}`);
			if (t.projectId) bits.push(`project ${t.projectId}`);
			if (t.snoozedUntil) bits.push(`snoozed ${t.snoozedUntil}`);
			return bits.join(' · ');
		})
		.join('\n');

	const system =
		'You are a focused task-management agent. The user types ONE intent; you respond with a list of typed actions to apply to their tasks. ' +
		'Hard rules: ' +
		'(1) Return STRICT JSON ONLY, no fences, no preamble. Schema: ' +
		'{"actions":[{"taskId":"<exact id>","kind":"<kind>","rationale":"<one short sentence>","<arg-fields>":...}]}. ' +
		'(2) Use EXACT taskId values from the list. Never invent IDs. ' +
		'(3) Allowed kinds: set_priority (priority 1-3), set_due (dueDate "YYYY-MM-DD"), clear_due (no args), ' +
		'schedule (scheduledStart ISO 8601 + optional durationMinutes), clear_schedule (no args), ' +
		'mark_done (no args), archive (no args — drops from active list), unarchive (no args), ' +
		'snooze (snoozedUntil "YYYY-MM-DD"), set_project (projectId — use "" to clear), change_text (text). ' +
		'(4) Each action MUST include a "rationale" — ONE sentence under 16 words explaining why this specific change. No generic praise. ' +
		'(5) Be selective. If the user asks a broad question, propose 3-12 actions, not 40. Empty {"actions":[]} is a valid answer when the intent yields nothing. ' +
		'(6) If the user asks something you cannot encode as actions (e.g. "explain", "summarise"), return {"actions":[]}. ' +
		'(7) Never set priority outside 1-3. Never set a past due date. Never schedule tasks already marked done.';

	const projectsLine =
		availableProjects.length > 0
			? `\nKnown project ids: ${availableProjects.join(', ')}.`
			: '';

	const user =
		`Today is ${todayISO}.${projectsLine}\n\n` +
		`User intent: ${userText.trim()}\n\n` +
		`Tasks in scope (${tasks.length}):\n${lines}`;

	return { system, user };
}

/** parseAgentResponse — extracts the actions array from the
 *  model reply. Shape-level extraction is shared via
 *  $lib/agents/core.extractActions; this function adds the
 *  task-specific schema check (taskId + kind enum + rationale). */
export function parseAgentResponse(raw: string): TaskAction[] {
	const arr = extractActions(raw);
	const out: TaskAction[] = [];
	for (const entry of arr) {
		if (!entry || typeof entry !== 'object') continue;
		const e = entry as Record<string, unknown>;
		if (typeof e.taskId !== 'string' || !e.taskId.trim()) continue;
		if (typeof e.kind !== 'string' || !VALID_KINDS.has(e.kind as TaskActionKind)) continue;
		if (typeof e.rationale !== 'string' || !e.rationale.trim()) continue;
		const action: TaskAction = {
			taskId: e.taskId.trim(),
			kind: e.kind as TaskActionKind,
			rationale: e.rationale.trim()
		};
		if (typeof e.priority === 'number') action.priority = e.priority;
		if (typeof e.dueDate === 'string') action.dueDate = e.dueDate;
		if (typeof e.scheduledStart === 'string') action.scheduledStart = e.scheduledStart;
		if (typeof e.durationMinutes === 'number') action.durationMinutes = e.durationMinutes;
		if (typeof e.snoozedUntil === 'string') action.snoozedUntil = e.snoozedUntil;
		if (typeof e.projectId === 'string') action.projectId = e.projectId;
		if (typeof e.text === 'string') action.text = e.text;
		out.push(action);
	}
	return out;
}

/** validateActions — second pass after parse. Drops actions whose
 *  taskId no longer exists on the live list (model hallucination
 *  or task deleted mid-stream), clamps priority to 1-3, and drops
 *  actions whose required arg is missing for their kind. The page
 *  passes the output straight into the proposal UI — anything
 *  that survives here is safe to apply. */
export function validateActions(actions: TaskAction[], liveTasks: Task[]): TaskAction[] {
	const liveIds = new Set(liveTasks.map((t) => t.id));
	const out: TaskAction[] = [];
	for (const a of actions) {
		if (!liveIds.has(a.taskId)) continue;

		switch (a.kind) {
			case 'set_priority':
				if (typeof a.priority !== 'number') continue;
				out.push({ ...a, priority: clamp(Math.round(a.priority), 1, 3) });
				break;
			case 'set_due':
				if (!a.dueDate || !isYMD(a.dueDate)) continue;
				out.push(a);
				break;
			case 'schedule':
				if (!a.scheduledStart || !isIsoLike(a.scheduledStart)) continue;
				out.push(a);
				break;
			case 'snooze':
				if (!a.snoozedUntil || !isYMD(a.snoozedUntil)) continue;
				out.push(a);
				break;
			case 'set_project':
				if (typeof a.projectId !== 'string') continue;
				out.push(a);
				break;
			case 'change_text':
				if (!a.text || !a.text.trim()) continue;
				out.push({ ...a, text: a.text.trim() });
				break;
			case 'clear_due':
			case 'clear_schedule':
			case 'mark_done':
			case 'archive':
			case 'unarchive':
				out.push(a);
				break;
		}
	}
	return out;
}

/** ProposalState — the row state the TaskAgent dialog tracks per
 *  action: the action itself plus dialog-side flags. Shape lives
 *  here because the dialog imports it; merge semantics live in
 *  $lib/agents/core. */
export type ProposalState = TaskAction & ProposalFlags;

/** mergeProposals — thin wrapper that supplies the task-specific
 *  identity key (`taskId::kind`) to the generic merger. All the
 *  re-stream semantics (freeze applied args, preserve engaged
 *  rows when the new parse drops them) live in $lib/agents/core
 *  and are tested there. */
export function mergeProposals(prev: ProposalState[], next: TaskAction[]): ProposalState[] {
	return coreMerge(prev, next, (a) => `${a.taskId}::${a.kind}`);
}

/** Human-readable summary of an action — for the proposal card UI.
 *  Centralised so labels stay consistent and we can reuse the
 *  formatter in tests. */
export function summariseAction(a: TaskAction, task: Task | undefined): string {
	const t = task?.text ? `"${truncate(task.text, 36)}"` : a.taskId;
	switch (a.kind) {
		case 'set_priority':
			return `Set ${t} to P${a.priority}`;
		case 'set_due':
			return `Due ${t} on ${a.dueDate}`;
		case 'clear_due':
			return `Clear due date on ${t}`;
		case 'schedule':
			return `Schedule ${t} for ${a.scheduledStart}${
				a.durationMinutes ? ` (${a.durationMinutes}m)` : ''
			}`;
		case 'clear_schedule':
			return `Unschedule ${t}`;
		case 'mark_done':
			return `Mark ${t} done`;
		case 'archive':
			return `Archive ${t}`;
		case 'unarchive':
			return `Restore ${t} to inbox`;
		case 'snooze':
			return `Snooze ${t} until ${a.snoozedUntil}`;
		case 'set_project':
			return a.projectId
				? `Move ${t} to project "${a.projectId}"`
				: `Remove ${t} from its project`;
		case 'change_text':
			return `Rename ${t} → "${truncate(a.text ?? '', 36)}"`;
	}
}

/** Patch shape accepted by api.patchTask — duplicated narrowly
 *  here so undo can be computed in a pure module without a runtime
 *  dependency on $lib/api. Keep in sync with api.ts patchTask
 *  signature. */
export type TaskRevertPatch = {
	done?: boolean;
	priority?: number;
	dueDate?: string;
	text?: string;
	scheduledStart?: string;
	durationMinutes?: number;
	projectId?: string;
	snoozedUntil?: string;
	triage?: 'inbox' | 'triaged' | 'scheduled' | 'done' | 'dropped' | 'snoozed';
	clearSchedule?: boolean;
};

/** computeRevertPatch — given an action that's ABOUT to apply and
 *  the task's pre-state, build the patch that would undo it. Used
 *  by the dialog's "Undo run" button: each applied action stashes
 *  its revert patch, undo plays them back via patchTask.
 *
 *  Returns null when there's nothing to revert (e.g. clear_schedule
 *  on a task that already had no schedule). Pure — no API calls,
 *  so vitest can pin every case. */
export function computeRevertPatch(action: TaskAction, preTask: Task): TaskRevertPatch | null {
	switch (action.kind) {
		case 'set_priority':
			return { priority: preTask.priority ?? 0 };
		case 'set_due':
		case 'clear_due':
			return { dueDate: preTask.dueDate ?? '' };
		case 'schedule':
			if (preTask.scheduledStart) {
				const r: TaskRevertPatch = { scheduledStart: preTask.scheduledStart };
				if (preTask.durationMinutes) r.durationMinutes = preTask.durationMinutes;
				return r;
			}
			return { clearSchedule: true };
		case 'clear_schedule':
			if (preTask.scheduledStart) {
				const r: TaskRevertPatch = { scheduledStart: preTask.scheduledStart };
				if (preTask.durationMinutes) r.durationMinutes = preTask.durationMinutes;
				return r;
			}
			return null;
		case 'mark_done':
			return { done: preTask.done ?? false };
		case 'archive':
			return { done: preTask.done ?? false, triage: preTask.triage ?? 'inbox' };
		case 'unarchive':
			return { done: preTask.done ?? false, triage: preTask.triage ?? 'inbox' };
		case 'snooze':
			return { snoozedUntil: preTask.snoozedUntil ?? '' };
		case 'set_project':
			return { projectId: preTask.projectId ?? '' };
		case 'change_text':
			return { text: preTask.text ?? '' };
	}
}

function clamp(n: number, lo: number, hi: number): number {
	return Math.max(lo, Math.min(hi, n));
}
function isYMD(s: string): boolean {
	return /^\d{4}-\d{2}-\d{2}$/.test(s);
}
function isIsoLike(s: string): boolean {
	// Accept "YYYY-MM-DDTHH:MM" and "YYYY-MM-DDTHH:MM:SS[Z]" — the
	// model often returns the first; the server normalises either.
	return /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}/.test(s);
}
function truncate(s: string, n: number): string {
	return s.length <= n ? s : s.slice(0, n - 1).trimEnd() + '…';
}
