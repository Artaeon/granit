// Project Agent — conversational mutation engine for /projects.
// Mirrors $lib/tasks/agent.ts; the shared re-stream / extract
// semantics live in $lib/agents/core so behaviour stays
// consistent across agents.
//
// Why mirror the shape: the dialog UX (free-text intent → typed
// actions → accept/skip cards → run-scoped undo) generalises
// across entities. Each agent picks the actions appropriate to
// its domain. Projects have a smaller, slower-moving state space
// than tasks — fewer status transitions, no scheduling — so the
// action set is correspondingly narrower.

import type { Project } from '$lib/api';
import { extractActions, type ProposalFlags } from '$lib/agents/core';
import { mergeProposals as coreMerge } from '$lib/agents/core';

/** The canonical project lifecycle, matches kanbanGroup.KANBAN_STATUSES. */
export const PROJECT_STATUSES = ['active', 'paused', 'completed', 'archived'] as const;
export type ProjectStatus = (typeof PROJECT_STATUSES)[number];

/** Allowed action verbs. Mirrors the patchProject surface — every
 *  action collapses to a single PATCH. Keep this list aligned
 *  with applyProjectAction in the component. */
export type ProjectActionKind =
	| 'set_status'
	| 'set_priority'
	| 'set_due_date'
	| 'clear_due_date'
	| 'set_next_action'
	| 'clear_next_action'
	| 'set_venture'
	| 'clear_venture'
	| 'change_description'
	| 'archive'
	| 'unarchive';

export interface ProjectAction {
	projectName: string;
	kind: ProjectActionKind;
	status?: ProjectStatus;
	priority?: number;
	due_date?: string;
	next_action?: string;
	venture?: string;
	description?: string;
	rationale: string;
}

export type ProjectProposalState = ProjectAction & ProposalFlags;

const VALID_KINDS = new Set<ProjectActionKind>([
	'set_status',
	'set_priority',
	'set_due_date',
	'clear_due_date',
	'set_next_action',
	'clear_next_action',
	'set_venture',
	'clear_venture',
	'change_description',
	'archive',
	'unarchive'
]);

const VALID_STATUSES = new Set<ProjectStatus>(PROJECT_STATUSES);

/** buildProjectAgentPrompt — system + user pair fed to chatStream.
 *  The digest lists name + status + priority + venture + due date
 *  + next action so the model has the leverage points without
 *  drowning in the project's full description (which can be
 *  paragraphs). The system prompt enumerates valid kinds + their
 *  args so the model returns clean JSON. */
export function buildProjectAgentPrompt(
	projects: Project[],
	userText: string,
	todayISO: string,
	knownVentures: string[] = []
): { system: string; user: string } {
	const lines = projects
		.map((p) => {
			const bits: string[] = [`name:${p.name}`];
			const status = p.status ?? 'active';
			bits.push(status);
			if (p.priority) bits.push(`p${p.priority}`);
			if (p.venture) bits.push(`venture:${p.venture}`);
			if (p.due_date) bits.push(`due ${p.due_date}`);
			if (p.next_action) bits.push(`next:"${p.next_action.slice(0, 60).replace(/\n/g, ' ')}"`);
			if (p.description)
				bits.push(`desc:"${p.description.slice(0, 80).replace(/\n/g, ' ')}"`);
			return bits.join(' · ');
		})
		.join('\n');

	const system =
		'You are a focused project-management agent. The user types ONE intent; you respond with a list of typed actions to apply to their projects. ' +
		'Hard rules: ' +
		'(1) Return STRICT JSON ONLY, no fences, no preamble. Schema: ' +
		'{"actions":[{"projectName":"<exact name>","kind":"<kind>","rationale":"<one short sentence>","<arg-fields>":...}]}. ' +
		'(2) Use EXACT project names from the list. Never invent names. ' +
		'(3) Allowed kinds: ' +
		'set_status (status: active|paused|completed|archived), ' +
		'set_priority (priority 1-3), ' +
		'set_due_date (due_date "YYYY-MM-DD"), clear_due_date (no args), ' +
		'set_next_action (next_action: one-line concrete next step), clear_next_action (no args), ' +
		'set_venture (venture: string from known ventures, or a NEW one — venture is free-text), clear_venture (no args), ' +
		'change_description (description: replacement prose, 1-3 sentences), ' +
		'archive (no args — shorthand for set_status:archived), unarchive (no args — shorthand for set_status:active). ' +
		'(4) Each action MUST include a "rationale" — ONE sentence under 16 words explaining why this specific change. No generic praise. ' +
		'(5) Be selective. If the user asks a broad question, propose 3-10 actions, not 30. Empty {"actions":[]} is a valid answer when the intent yields nothing concrete. ' +
		'(6) If the user asks something you cannot encode as actions (e.g. "summarise" or "explain"), return {"actions":[]}. ' +
		'(7) Never set priority outside 1-3. Never set a past due_date. Never propose change_description for a project whose existing description already encodes the same fact.';

	const venturesLine =
		knownVentures.length > 0 ? `\nKnown ventures: ${knownVentures.join(', ')}.` : '';

	const user =
		`Today is ${todayISO}.${venturesLine}\n\n` +
		`User intent: ${userText.trim()}\n\n` +
		`Projects in scope (${projects.length}):\n${lines}`;

	return { system, user };
}

/** parseProjectAgentResponse — strict-schema validator over the
 *  generic extractActions output. Drops malformed rows silently. */
export function parseProjectAgentResponse(raw: string): ProjectAction[] {
	const arr = extractActions(raw);
	const out: ProjectAction[] = [];
	for (const entry of arr) {
		if (!entry || typeof entry !== 'object') continue;
		const e = entry as Record<string, unknown>;
		if (typeof e.projectName !== 'string' || !e.projectName.trim()) continue;
		if (typeof e.kind !== 'string' || !VALID_KINDS.has(e.kind as ProjectActionKind)) continue;
		if (typeof e.rationale !== 'string' || !e.rationale.trim()) continue;
		const action: ProjectAction = {
			projectName: e.projectName.trim(),
			kind: e.kind as ProjectActionKind,
			rationale: e.rationale.trim()
		};
		if (typeof e.status === 'string' && VALID_STATUSES.has(e.status as ProjectStatus))
			action.status = e.status as ProjectStatus;
		if (typeof e.priority === 'number') action.priority = e.priority;
		if (typeof e.due_date === 'string') action.due_date = e.due_date;
		if (typeof e.next_action === 'string') action.next_action = e.next_action;
		if (typeof e.venture === 'string') action.venture = e.venture;
		if (typeof e.description === 'string') action.description = e.description;
		out.push(action);
	}
	return out;
}

/** validateProjectActions — second pass. Drops actions whose
 *  projectName isn't on the live list (model hallucination),
 *  clamps priority, requires YYYY-MM-DD dates, drops empty
 *  rewrites. */
export function validateProjectActions(actions: ProjectAction[], live: Project[]): ProjectAction[] {
	const liveNames = new Set(live.map((p) => p.name));
	const out: ProjectAction[] = [];
	for (const a of actions) {
		if (!liveNames.has(a.projectName)) continue;
		switch (a.kind) {
			case 'set_status':
				if (!a.status || !VALID_STATUSES.has(a.status)) continue;
				out.push(a);
				break;
			case 'set_priority':
				if (typeof a.priority !== 'number') continue;
				out.push({ ...a, priority: clamp(Math.round(a.priority), 1, 3) });
				break;
			case 'set_due_date':
				if (!a.due_date || !isYMD(a.due_date)) continue;
				out.push(a);
				break;
			case 'set_next_action':
				if (!a.next_action || !a.next_action.trim()) continue;
				out.push({ ...a, next_action: a.next_action.trim() });
				break;
			case 'set_venture':
				if (!a.venture || !a.venture.trim()) continue;
				out.push({ ...a, venture: a.venture.trim() });
				break;
			case 'change_description':
				if (!a.description || !a.description.trim()) continue;
				out.push({ ...a, description: a.description.trim() });
				break;
			case 'clear_due_date':
			case 'clear_next_action':
			case 'clear_venture':
			case 'archive':
			case 'unarchive':
				out.push(a);
				break;
		}
	}
	return out;
}

/** Patch shape mirroring patchProject(). All-optional. */
export type ProjectRevertPatch = {
	status?: ProjectStatus;
	priority?: number;
	due_date?: string;
	next_action?: string;
	venture?: string;
	description?: string;
};

/** computeProjectRevertPatch — pure undo computation. Each action
 *  reverts to the pre-state values of the fields it touched. */
export function computeProjectRevertPatch(
	action: ProjectAction,
	pre: Project
): ProjectRevertPatch | null {
	switch (action.kind) {
		case 'set_status':
			return { status: (pre.status ?? 'active') as ProjectStatus };
		case 'set_priority':
			return { priority: pre.priority ?? 0 };
		case 'set_due_date':
		case 'clear_due_date':
			return { due_date: pre.due_date ?? '' };
		case 'set_next_action':
		case 'clear_next_action':
			return { next_action: pre.next_action ?? '' };
		case 'set_venture':
		case 'clear_venture':
			return { venture: pre.venture ?? '' };
		case 'change_description':
			return { description: pre.description ?? '' };
		case 'archive':
		case 'unarchive':
			// Both are shorthand for set_status — revert to pre status.
			return { status: (pre.status ?? 'active') as ProjectStatus };
	}
}

/** Human-readable summary — used in proposal cards. */
export function summariseProjectAction(a: ProjectAction, p: Project | undefined): string {
	const n = p?.name ?? a.projectName;
	switch (a.kind) {
		case 'set_status':
			return `Move "${truncate(n, 24)}" → ${a.status}`;
		case 'set_priority':
			return `Set "${truncate(n, 24)}" priority to P${a.priority}`;
		case 'set_due_date':
			return `Due "${truncate(n, 24)}" on ${a.due_date}`;
		case 'clear_due_date':
			return `Clear due date on "${truncate(n, 24)}"`;
		case 'set_next_action':
			return `Next action on "${truncate(n, 24)}" → "${truncate(a.next_action ?? '', 40)}"`;
		case 'clear_next_action':
			return `Clear next action on "${truncate(n, 24)}"`;
		case 'set_venture':
			return `Move "${truncate(n, 24)}" into venture "${a.venture}"`;
		case 'clear_venture':
			return `Remove "${truncate(n, 24)}" from its venture`;
		case 'change_description':
			return `Rewrite description of "${truncate(n, 24)}"`;
		case 'archive':
			return `Archive "${truncate(n, 24)}"`;
		case 'unarchive':
			return `Restore "${truncate(n, 24)}" to active`;
	}
}

/** mergeProjectProposals — thin wrapper supplying the project-
 *  specific identity key to the shared merger. */
export function mergeProjectProposals(
	prev: ProjectProposalState[],
	next: ProjectAction[]
): ProjectProposalState[] {
	return coreMerge(prev, next, (a) => `${a.projectName}::${a.kind}`);
}

function clamp(n: number, lo: number, hi: number): number {
	return Math.max(lo, Math.min(hi, n));
}
function isYMD(s: string): boolean {
	return /^\d{4}-\d{2}-\d{2}$/.test(s);
}
function truncate(s: string, n: number): string {
	return s.length <= n ? s : s.slice(0, n - 1).trimEnd() + '…';
}
