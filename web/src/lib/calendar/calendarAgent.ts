// Calendar Agent — conversational mutation engine for /calendar.
// Mirrors $lib/tasks/agent, $lib/projects/projectAgent,
// $lib/goals/goalAgent. Shared re-stream + extraction lives in
// $lib/agents/core.
//
// SCOPE — deliberately narrow:
//   - NATIVE events only (CalendarEventEntry). ICS-sourced events
//     are read-only on the server; proposing edits there would
//     just 403. The validator drops them.
//   - SERIES-level edits only. Per-occurrence overrides for
//     recurring events have their own (complex) UI and need an
//     anchor key the agent can't reliably emit.
//   - NO deletes. Cancellation is irreversible — no usable
//     "undo" patch beyond recreating the event from snapshotted
//     fields, which is a different shape. The user can delete
//     directly from the existing event-detail modal.
//   - NO rrule edits. The grammar is RFC 5545 and the model
//     can produce subtly-wrong rules ("FREQ=WEEKLY;BYDAY=MO,TH"
//     when the user meant Tuesday) with no way to validate
//     without expanding into occurrences. Stays in the picker UI.
//
// Allowed kinds patch one field at a time so undo restores cleanly:
// rename_event, move_event_to_date, set_event_time,
// set_event_color, set_event_location / clear_event_location,
// set_event_project / clear_event_project.

import type { CalendarEventEntry } from '$lib/api';
import {
	extractActions,
	mergeProposals as coreMerge,
	type ProposalFlags
} from '$lib/agents/core';

export type CalendarActionKind =
	| 'rename_event'
	| 'move_event_to_date'
	| 'set_event_time'
	| 'set_event_color'
	| 'set_event_location'
	| 'clear_event_location'
	| 'set_event_project'
	| 'clear_event_project';

export interface CalendarAction {
	eventId: string;
	kind: CalendarActionKind;
	title?: string; // rename_event
	date?: string; // move_event_to_date — YYYY-MM-DD
	start_time?: string; // set_event_time — HH:MM
	end_time?: string; // set_event_time — HH:MM
	color?: string; // set_event_color
	location?: string; // set_event_location
	project?: string; // set_event_project
	rationale: string;
}

export type CalendarProposalState = CalendarAction & ProposalFlags;

const VALID_KINDS = new Set<CalendarActionKind>([
	'rename_event',
	'move_event_to_date',
	'set_event_time',
	'set_event_color',
	'set_event_location',
	'clear_event_location',
	'set_event_project',
	'clear_event_project'
]);

/** buildCalendarAgentPrompt — system + user pair. Events are
 *  listed with their id + title + date + time range + color +
 *  location so the agent has the leverage points without leaking
 *  notePath or reminder noise. Recurring events are flagged
 *  inline so the model knows the change applies to the series. */
export function buildCalendarAgentPrompt(
	events: CalendarEventEntry[],
	userText: string,
	todayISO: string,
	knownProjects: string[] = []
): { system: string; user: string } {
	// Sort chronologically so the model reads events in flow order
	// (yesterday → today → tomorrow → …). Untimed events on the
	// same day sort to the end after timed ones. Pure local sort
	// — caller's array isn't mutated.
	const sorted = [...events].sort((a, b) => {
		if (a.date !== b.date) return a.date.localeCompare(b.date);
		const sa = a.start_time ?? '99:99';
		const sb = b.start_time ?? '99:99';
		return sa.localeCompare(sb);
	});
	const lines = sorted
		.map((e) => {
			const bits: string[] = [`id:${e.id} — "${e.title}"`];
			bits.push(e.date);
			if (e.start_time) bits.push(`${e.start_time}–${e.end_time ?? '?'}`);
			else bits.push('all-day');
			if (e.color) bits.push(`color:${e.color}`);
			if (e.location) bits.push(`loc:"${e.location.slice(0, 40)}"`);
			if (e.project_id) bits.push(`project:${e.project_id}`);
			if (e.rrule) bits.push('recurring');
			return bits.join(' · ');
		})
		.join('\n');

	const system =
		'You are a focused calendar-management agent. The user types ONE intent; you respond with a list of typed actions to apply to their native calendar events. ' +
		'Hard rules: ' +
		'(1) Return STRICT JSON ONLY, no fences, no preamble. Schema: ' +
		'{"actions":[{"eventId":"<exact id>","kind":"<kind>","rationale":"<one sentence>","<arg-fields>":...}]}. ' +
		'(2) Use EXACT eventId values. Never invent IDs. ' +
		'(3) Allowed kinds: ' +
		'rename_event (title: replacement string), ' +
		'move_event_to_date (date: "YYYY-MM-DD" — moves the WHOLE event/series to that day; time of day preserved), ' +
		'set_event_time (start_time: "HH:MM" 24h, end_time: "HH:MM" 24h — must satisfy end_time > start_time), ' +
		'set_event_color (color: a single palette word — blue, red, green, yellow, orange, purple, pink, teal, cyan, mauve, peach, sapphire, lavender, flamingo), ' +
		'set_event_location (location: string), clear_event_location (no args), ' +
		'set_event_project (project: project name), clear_event_project (no args). ' +
		'(4) Each action MUST include a "rationale" — ONE sentence under 16 words explaining the change. ' +
		'(5) Recurring events: changes apply to the WHOLE SERIES. If the user wants to edit a single occurrence, return {"actions":[]} and explain in the rationale that per-occurrence overrides go through the event detail UI. Never invent override semantics. ' +
		'(6) Never set a past date. Never set start_time >= end_time. ' +
		'(7) Be selective. Propose 3-12 actions max for a broad intent. Empty {"actions":[]} is valid when nothing concrete applies.';

	const projectsLine =
		knownProjects.length > 0 ? `\nKnown projects: ${knownProjects.join(', ')}.` : '';

	const user =
		`Today is ${todayISO}.${projectsLine}\n\n` +
		`User intent: ${userText.trim()}\n\n` +
		`Events in scope (${events.length}, native events only):\n${lines}`;

	return { system, user };
}

/** parseCalendarAgentResponse — strict-schema check over the
 *  generic extractActions output. */
export function parseCalendarAgentResponse(raw: string): CalendarAction[] {
	const arr = extractActions(raw);
	const out: CalendarAction[] = [];
	for (const entry of arr) {
		if (!entry || typeof entry !== 'object') continue;
		const e = entry as Record<string, unknown>;
		if (typeof e.eventId !== 'string' || !e.eventId.trim()) continue;
		if (typeof e.kind !== 'string' || !VALID_KINDS.has(e.kind as CalendarActionKind)) continue;
		if (typeof e.rationale !== 'string' || !e.rationale.trim()) continue;
		const action: CalendarAction = {
			eventId: e.eventId.trim(),
			kind: e.kind as CalendarActionKind,
			rationale: e.rationale.trim()
		};
		if (typeof e.title === 'string') action.title = e.title;
		if (typeof e.date === 'string') action.date = e.date;
		if (typeof e.start_time === 'string') action.start_time = e.start_time;
		if (typeof e.end_time === 'string') action.end_time = e.end_time;
		if (typeof e.color === 'string') action.color = e.color;
		if (typeof e.location === 'string') action.location = e.location;
		if (typeof e.project === 'string') action.project = e.project;
		out.push(action);
	}
	return out;
}

/** validateCalendarActions — drops actions whose eventId isn't
 *  on the live list, drops malformed dates/times, drops anything
 *  with end_time <= start_time. Treats today as the past
 *  boundary (today is fine, yesterday isn't). */
export function validateCalendarActions(
	actions: CalendarAction[],
	live: CalendarEventEntry[],
	todayISO: string
): CalendarAction[] {
	const liveById = new Map<string, CalendarEventEntry>();
	for (const e of live) liveById.set(e.id, e);
	const out: CalendarAction[] = [];
	for (const a of actions) {
		if (!liveById.has(a.eventId)) continue;
		switch (a.kind) {
			case 'rename_event':
				if (!a.title || !a.title.trim()) continue;
				out.push({ ...a, title: a.title.trim() });
				break;
			case 'move_event_to_date':
				if (!a.date || !isYMD(a.date)) continue;
				if (a.date < todayISO) continue;
				out.push(a);
				break;
			case 'set_event_time':
				if (!a.start_time || !isHM(a.start_time)) continue;
				if (!a.end_time || !isHM(a.end_time)) continue;
				if (a.end_time <= a.start_time) continue;
				out.push(a);
				break;
			case 'set_event_color':
				if (!a.color || !a.color.trim()) continue;
				out.push({ ...a, color: a.color.trim().toLowerCase() });
				break;
			case 'set_event_location':
				if (!a.location || !a.location.trim()) continue;
				out.push({ ...a, location: a.location.trim() });
				break;
			case 'set_event_project':
				if (!a.project || !a.project.trim()) continue;
				out.push({ ...a, project: a.project.trim() });
				break;
			case 'clear_event_location':
			case 'clear_event_project':
				out.push(a);
				break;
		}
	}
	return out;
}

/** Patch shape mirroring api.patchEvent. Only the fields the
 *  agent's actions can touch. */
export type CalendarRevertPatch = {
	title?: string;
	date?: string;
	start_time?: string;
	end_time?: string;
	color?: string;
	location?: string;
	project_id?: string;
};

/** computeCalendarRevertPatch — pure undo. Each action reverts
 *  to the touched field's pre-state value. set_event_time
 *  reverts BOTH times together (you can't restore just the start
 *  cleanly if the event was all-day before — but the validator
 *  only let set_event_time through for events with existing
 *  times). For all-day events that the model accidentally fed a
 *  time to, the revert is start_time:'' + end_time:'' which the
 *  server treats as "all-day" via omitempty. */
export function computeCalendarRevertPatch(
	a: CalendarAction,
	pre: CalendarEventEntry
): CalendarRevertPatch | null {
	switch (a.kind) {
		case 'rename_event':
			return { title: pre.title };
		case 'move_event_to_date':
			return { date: pre.date };
		case 'set_event_time':
			return {
				start_time: pre.start_time ?? '',
				end_time: pre.end_time ?? ''
			};
		case 'set_event_color':
			return { color: pre.color ?? '' };
		case 'set_event_location':
		case 'clear_event_location':
			return { location: pre.location ?? '' };
		case 'set_event_project':
		case 'clear_event_project':
			return { project_id: pre.project_id ?? '' };
	}
}

export function summariseCalendarAction(
	a: CalendarAction,
	e: CalendarEventEntry | undefined
): string {
	const t = e?.title ?? a.eventId;
	switch (a.kind) {
		case 'rename_event':
			return `Rename "${truncate(t, 28)}" → "${truncate(a.title ?? '', 32)}"`;
		case 'move_event_to_date':
			return `Move "${truncate(t, 28)}" to ${a.date}`;
		case 'set_event_time':
			return `Reschedule "${truncate(t, 28)}" to ${a.start_time}–${a.end_time}`;
		case 'set_event_color':
			return `Tint "${truncate(t, 28)}" ${a.color}`;
		case 'set_event_location':
			return `Set location on "${truncate(t, 28)}" → "${truncate(a.location ?? '', 28)}"`;
		case 'clear_event_location':
			return `Clear location on "${truncate(t, 28)}"`;
		case 'set_event_project':
			return `Link "${truncate(t, 28)}" to project "${a.project}"`;
		case 'clear_event_project':
			return `Unlink "${truncate(t, 28)}" from its project`;
	}
}

export function mergeCalendarProposals(
	prev: CalendarProposalState[],
	next: CalendarAction[]
): CalendarProposalState[] {
	return coreMerge(prev, next, (a) => `${a.eventId}::${a.kind}`);
}

function isYMD(s: string): boolean {
	return /^\d{4}-\d{2}-\d{2}$/.test(s);
}
function isHM(s: string): boolean {
	// HH:MM 24-hour, 00:00 to 23:59. We don't accept seconds.
	return /^([01]\d|2[0-3]):[0-5]\d$/.test(s);
}
function truncate(s: string, n: number): string {
	return s.length <= n ? s : s.slice(0, n - 1).trimEnd() + '…';
}
