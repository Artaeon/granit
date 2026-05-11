// Goal Agent — conversational mutation engine for /goals.
// Mirrors $lib/projects/projectAgent.ts; shared re-stream and
// extraction semantics live in $lib/agents/core.
//
// Scope: top-level goal fields (status, target date, review
// frequency, venture, title, description, archive). Milestone-
// level edits (add / mark done / reorder) require a different
// patch surface (nested array) and stay with the existing
// milestone-suggestion AI in GoalDetail.svelte.

import type { Goal } from '$lib/api';
import {
	extractActions,
	mergeProposals as coreMerge,
	type ProposalFlags
} from '$lib/agents/core';

export const GOAL_STATUSES = ['active', 'paused', 'completed', 'archived'] as const;
export type GoalStatus = (typeof GOAL_STATUSES)[number];

export const GOAL_REVIEW_FREQUENCIES = ['weekly', 'monthly', 'quarterly'] as const;
export type GoalReviewFrequency = (typeof GOAL_REVIEW_FREQUENCIES)[number];

export type GoalActionKind =
	| 'set_status'
	| 'set_target_date'
	| 'clear_target_date'
	| 'set_review_frequency'
	| 'clear_review_frequency'
	| 'set_venture'
	| 'clear_venture'
	| 'change_title'
	| 'change_description'
	| 'archive'
	| 'unarchive';

export interface GoalAction {
	goalId: string;
	kind: GoalActionKind;
	status?: GoalStatus;
	target_date?: string;
	review_frequency?: GoalReviewFrequency;
	venture?: string;
	title?: string;
	description?: string;
	rationale: string;
}

export type GoalProposalState = GoalAction & ProposalFlags;

const VALID_KINDS = new Set<GoalActionKind>([
	'set_status',
	'set_target_date',
	'clear_target_date',
	'set_review_frequency',
	'clear_review_frequency',
	'set_venture',
	'clear_venture',
	'change_title',
	'change_description',
	'archive',
	'unarchive'
]);
const VALID_STATUSES = new Set<GoalStatus>(GOAL_STATUSES);
const VALID_FREQUENCIES = new Set<GoalReviewFrequency>(GOAL_REVIEW_FREQUENCIES);

export function buildGoalAgentPrompt(
	goals: Goal[],
	userText: string,
	todayISO: string,
	knownVentures: string[] = []
): { system: string; user: string } {
	const lines = goals
		.map((g) => {
			const bits: string[] = [`id:${g.id} — "${g.title}"`];
			const status = g.status ?? 'active';
			bits.push(status);
			if (g.target_date) bits.push(`target ${g.target_date}`);
			if (g.review_frequency) bits.push(`review:${g.review_frequency}`);
			if (g.venture) bits.push(`venture:${g.venture}`);
			if (g.description)
				bits.push(`desc:"${g.description.slice(0, 80).replace(/\n/g, ' ')}"`);
			return bits.join(' · ');
		})
		.join('\n');

	const system =
		'You are a focused goal-management agent. The user types ONE intent; you respond with a list of typed actions to apply to their goals. ' +
		'Hard rules: ' +
		'(1) Return STRICT JSON ONLY, no fences, no preamble. Schema: ' +
		'{"actions":[{"goalId":"<exact id>","kind":"<kind>","rationale":"<one sentence>","<arg-fields>":...}]}. ' +
		'(2) Use EXACT goalId values. Never invent IDs. ' +
		'(3) Allowed kinds: ' +
		'set_status (status: active|paused|completed|archived), ' +
		'set_target_date (target_date "YYYY-MM-DD"), clear_target_date (no args), ' +
		'set_review_frequency (review_frequency: weekly|monthly|quarterly), clear_review_frequency (no args), ' +
		'set_venture (venture: string), clear_venture (no args), ' +
		'change_title (title: replacement, under 60 chars), ' +
		'change_description (description: replacement, 1-3 sentences), ' +
		'archive (shorthand for set_status:archived), unarchive (shorthand for set_status:active). ' +
		'(4) Each action MUST include a "rationale" — ONE sentence under 16 words. No generic praise. ' +
		'(5) Be selective. Propose 3-10 actions max for a broad intent. Empty {"actions":[]} is valid. ' +
		'(6) Never set a past target_date. Never change_title to something the user obviously didn\'t want.';

	const venturesLine =
		knownVentures.length > 0 ? `\nKnown ventures: ${knownVentures.join(', ')}.` : '';

	const user =
		`Today is ${todayISO}.${venturesLine}\n\n` +
		`User intent: ${userText.trim()}\n\n` +
		`Goals in scope (${goals.length}):\n${lines}`;

	return { system, user };
}

export function parseGoalAgentResponse(raw: string): GoalAction[] {
	const arr = extractActions(raw);
	const out: GoalAction[] = [];
	for (const entry of arr) {
		if (!entry || typeof entry !== 'object') continue;
		const e = entry as Record<string, unknown>;
		if (typeof e.goalId !== 'string' || !e.goalId.trim()) continue;
		if (typeof e.kind !== 'string' || !VALID_KINDS.has(e.kind as GoalActionKind)) continue;
		if (typeof e.rationale !== 'string' || !e.rationale.trim()) continue;
		const action: GoalAction = {
			goalId: e.goalId.trim(),
			kind: e.kind as GoalActionKind,
			rationale: e.rationale.trim()
		};
		if (typeof e.status === 'string' && VALID_STATUSES.has(e.status as GoalStatus))
			action.status = e.status as GoalStatus;
		if (typeof e.target_date === 'string') action.target_date = e.target_date;
		if (
			typeof e.review_frequency === 'string' &&
			VALID_FREQUENCIES.has(e.review_frequency as GoalReviewFrequency)
		)
			action.review_frequency = e.review_frequency as GoalReviewFrequency;
		if (typeof e.venture === 'string') action.venture = e.venture;
		if (typeof e.title === 'string') action.title = e.title;
		if (typeof e.description === 'string') action.description = e.description;
		out.push(action);
	}
	return out;
}

export function validateGoalActions(actions: GoalAction[], live: Goal[]): GoalAction[] {
	const liveIds = new Set(live.map((g) => g.id));
	const out: GoalAction[] = [];
	for (const a of actions) {
		if (!liveIds.has(a.goalId)) continue;
		switch (a.kind) {
			case 'set_status':
				if (!a.status || !VALID_STATUSES.has(a.status)) continue;
				out.push(a);
				break;
			case 'set_target_date':
				if (!a.target_date || !isYMD(a.target_date)) continue;
				out.push(a);
				break;
			case 'set_review_frequency':
				if (!a.review_frequency || !VALID_FREQUENCIES.has(a.review_frequency)) continue;
				out.push(a);
				break;
			case 'set_venture':
				if (!a.venture || !a.venture.trim()) continue;
				out.push({ ...a, venture: a.venture.trim() });
				break;
			case 'change_title':
				if (!a.title || !a.title.trim()) continue;
				out.push({ ...a, title: a.title.trim() });
				break;
			case 'change_description':
				if (!a.description || !a.description.trim()) continue;
				out.push({ ...a, description: a.description.trim() });
				break;
			case 'clear_target_date':
			case 'clear_review_frequency':
			case 'clear_venture':
			case 'archive':
			case 'unarchive':
				out.push(a);
				break;
		}
	}
	return out;
}

export type GoalRevertPatch = {
	status?: GoalStatus;
	target_date?: string;
	review_frequency?: GoalReviewFrequency | '';
	venture?: string;
	title?: string;
	description?: string;
};

export function computeGoalRevertPatch(a: GoalAction, pre: Goal): GoalRevertPatch | null {
	switch (a.kind) {
		case 'set_status':
		case 'archive':
		case 'unarchive':
			return { status: (pre.status ?? 'active') as GoalStatus };
		case 'set_target_date':
		case 'clear_target_date':
			return { target_date: pre.target_date ?? '' };
		case 'set_review_frequency':
		case 'clear_review_frequency':
			return { review_frequency: (pre.review_frequency ?? '') as GoalReviewFrequency | '' };
		case 'set_venture':
		case 'clear_venture':
			return { venture: pre.venture ?? '' };
		case 'change_title':
			return { title: pre.title ?? '' };
		case 'change_description':
			return { description: pre.description ?? '' };
	}
}

export function summariseGoalAction(a: GoalAction, g: Goal | undefined): string {
	const t = g?.title ?? a.goalId;
	switch (a.kind) {
		case 'set_status':
			return `Move "${truncate(t, 24)}" → ${a.status}`;
		case 'set_target_date':
			return `Target "${truncate(t, 24)}" by ${a.target_date}`;
		case 'clear_target_date':
			return `Clear target date on "${truncate(t, 24)}"`;
		case 'set_review_frequency':
			return `Review "${truncate(t, 24)}" ${a.review_frequency}`;
		case 'clear_review_frequency':
			return `Clear review cadence on "${truncate(t, 24)}"`;
		case 'set_venture':
			return `Move "${truncate(t, 24)}" into venture "${a.venture}"`;
		case 'clear_venture':
			return `Remove "${truncate(t, 24)}" from its venture`;
		case 'change_title':
			return `Rename "${truncate(t, 24)}" → "${truncate(a.title ?? '', 30)}"`;
		case 'change_description':
			return `Rewrite description of "${truncate(t, 24)}"`;
		case 'archive':
			return `Archive "${truncate(t, 24)}"`;
		case 'unarchive':
			return `Restore "${truncate(t, 24)}" to active`;
	}
}

export function mergeGoalProposals(
	prev: GoalProposalState[],
	next: GoalAction[]
): GoalProposalState[] {
	return coreMerge(prev, next, (a) => `${a.goalId}::${a.kind}`);
}

function isYMD(s: string): boolean {
	return /^\d{4}-\d{2}-\d{2}$/.test(s);
}
function truncate(s: string, n: number): string {
	return s.length <= n ? s : s.slice(0, n - 1).trimEnd() + '…';
}
