// Pure helpers for the Calendar dashboard's "free slot map" and
// "deep-work blocks" cards. Kept out of CalendarDashboardPanel.svelte
// so the bucketing maths can be unit-tested without spinning up
// Svelte.
//
// The shared notion is the "working window": 09:00–18:00 on weekdays
// (Mon–Fri). Saturdays and Sundays are excluded — Granit users are
// often Sabbath-observant and weekend free time isn't actionable
// in the same way weekday free time is. Tune via WORK_START / WORK_END
// constants if a future config surface lets the user pick.
//
// Inputs are CalendarEventEntry rows (the same shape the chat
// prelude consumes). Tasks are *not* counted as blocking — a
// scheduled task can be moved; an event with another human in it
// usually can't. This matches how the user actually thinks about
// "free time".

import type { CalendarEventEntry } from '$lib/api';

/** Working hours bounds, in 24-h ints. */
export const WORK_START_HOUR = 9;
export const WORK_END_HOUR = 18;
/** Deep-work morning slot — used by the "deep work blocks" count
 *  on the hero card. A weekday morning counts as a deep-work block
 *  when no event overlaps 09:00–12:00 on that day. */
export const DEEP_MORNING_END_HOUR = 12;
/** Minimum free-slot length surfaced on the free-slot map card. */
export const MIN_FREE_SLOT_MINUTES = 60;

/** FreeSlot — an open span on a specific weekday inside the
 *  working window. Start/end are HH:MM strings so the dashboard
 *  can render them directly without re-formatting. */
export interface FreeSlot {
	/** YYYY-MM-DD — the day this slot lives on. */
	date: string;
	startMinutes: number; // minutes from midnight
	endMinutes: number;
	startLabel: string; // "HH:MM"
	endLabel: string;
	durationMinutes: number;
}

/** FreeSlotsDay — per-weekday bucket of free slots. Days are
 *  emitted in chronological order; `weekday` is the locale-agnostic
 *  short label ("Mon", "Tue", ...) for header rendering. */
export interface FreeSlotsDay {
	date: string;
	weekday: string;
	slots: FreeSlot[];
	/** True when no event blocks 09:00–12:00 on this day — surfaced
	 *  as the "deep-work block" count on the hero card. */
	hasDeepMorning: boolean;
}

const WEEKDAY_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

/** isWeekday — Mon–Fri only. Pure on the Date input. */
export function isWeekday(d: Date): boolean {
	const dow = d.getDay();
	return dow >= 1 && dow <= 5;
}

/** parseHHMM — '13:45' → 825. Returns NaN for unparseable input
 *  so callers can fall back to "treat as all-day". */
function parseHHMM(s: string | undefined): number {
	if (!s) return NaN;
	const m = /^(\d{1,2}):(\d{2})/.exec(s);
	if (!m) return NaN;
	const h = Number(m[1]);
	const mi = Number(m[2]);
	if (Number.isNaN(h) || Number.isNaN(mi)) return NaN;
	return h * 60 + mi;
}

function fmtHHMM(minutes: number): string {
	const h = Math.floor(minutes / 60);
	const m = minutes % 60;
	return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`;
}

/** isoFromDate — YYYY-MM-DD in local time. Pulled out so tests can
 *  pass a fixed Date and get a deterministic key. */
function isoFromDate(d: Date): string {
	const y = d.getFullYear();
	const m = String(d.getMonth() + 1).padStart(2, '0');
	const day = String(d.getDate()).padStart(2, '0');
	return `${y}-${m}-${day}`;
}

/** addDays — pure local-time day arithmetic. The calendar surface
 *  speaks YYYY-MM-DD without TZ; we keep everything in local time
 *  so the user's "tomorrow morning" stays their tomorrow morning. */
function addDays(d: Date, n: number): Date {
	const t = new Date(d);
	t.setDate(t.getDate() + n);
	return t;
}

/** eventBlocks — per-day "what time spans are blocked" extraction.
 *  All-day events block the whole working window; timed events block
 *  their actual span clipped to the working window. Events that fall
 *  entirely outside the working window are ignored.
 *
 *  Returned spans are NOT merged — mergeAndSlice does that. */
function eventBlocks(events: CalendarEventEntry[]): Array<{ start: number; end: number }> {
	const blocks: Array<{ start: number; end: number }> = [];
	const winStart = WORK_START_HOUR * 60;
	const winEnd = WORK_END_HOUR * 60;
	for (const e of events) {
		const s = parseHHMM(e.start_time);
		const en = parseHHMM(e.end_time);
		if (Number.isNaN(s)) {
			// All-day — block the entire working window.
			blocks.push({ start: winStart, end: winEnd });
			continue;
		}
		const start = Math.max(winStart, s);
		const end = Math.min(winEnd, Number.isNaN(en) ? s + 60 : en);
		if (end > start) blocks.push({ start, end });
	}
	return blocks;
}

/** mergeBlocks — sort + merge overlapping/adjacent spans so the
 *  free-slot calculation only inverts a clean disjoint cover. */
function mergeBlocks(
	blocks: Array<{ start: number; end: number }>
): Array<{ start: number; end: number }> {
	if (blocks.length === 0) return [];
	const sorted = [...blocks].sort((a, b) => a.start - b.start);
	const merged: Array<{ start: number; end: number }> = [sorted[0]];
	for (let i = 1; i < sorted.length; i++) {
		const last = merged[merged.length - 1];
		const cur = sorted[i];
		if (cur.start <= last.end) {
			last.end = Math.max(last.end, cur.end);
		} else {
			merged.push({ ...cur });
		}
	}
	return merged;
}

/** invertToFreeSlots — given the merged blocked spans inside the
 *  working window, return the gaps that are at least
 *  MIN_FREE_SLOT_MINUTES long. */
function invertToFreeSlots(
	merged: Array<{ start: number; end: number }>,
	date: string
): FreeSlot[] {
	const winStart = WORK_START_HOUR * 60;
	const winEnd = WORK_END_HOUR * 60;
	const slots: FreeSlot[] = [];
	let cursor = winStart;
	for (const b of merged) {
		if (b.start > cursor && b.start - cursor >= MIN_FREE_SLOT_MINUTES) {
			slots.push({
				date,
				startMinutes: cursor,
				endMinutes: b.start,
				startLabel: fmtHHMM(cursor),
				endLabel: fmtHHMM(b.start),
				durationMinutes: b.start - cursor
			});
		}
		cursor = Math.max(cursor, b.end);
	}
	if (winEnd > cursor && winEnd - cursor >= MIN_FREE_SLOT_MINUTES) {
		slots.push({
			date,
			startMinutes: cursor,
			endMinutes: winEnd,
			startLabel: fmtHHMM(cursor),
			endLabel: fmtHHMM(winEnd),
			durationMinutes: winEnd - cursor
		});
	}
	return slots;
}

/** computeFreeSlots — walk forward from `from` for up to `weekdays`
 *  weekday days, build a per-day free-slot list, and surface the
 *  hasDeepMorning flag. `from` is treated as a local Date; the
 *  starting day is included if it's a weekday.
 *
 *  Events outside the listed days are silently ignored — callers
 *  can pass the full upcoming bundle without pre-filtering. */
export function computeFreeSlots(
	events: CalendarEventEntry[],
	from: Date,
	weekdays = 5
): FreeSlotsDay[] {
	const byDate = new Map<string, CalendarEventEntry[]>();
	for (const e of events) {
		if (!e.date) continue;
		const list = byDate.get(e.date) ?? [];
		list.push(e);
		byDate.set(e.date, list);
	}
	const out: FreeSlotsDay[] = [];
	let cursor = new Date(from);
	cursor.setHours(0, 0, 0, 0);
	let safety = 0;
	while (out.length < weekdays && safety < 60) {
		safety++;
		if (!isWeekday(cursor)) {
			cursor = addDays(cursor, 1);
			continue;
		}
		const iso = isoFromDate(cursor);
		const dayEvents = byDate.get(iso) ?? [];
		const merged = mergeBlocks(eventBlocks(dayEvents));
		const slots = invertToFreeSlots(merged, iso);
		const hasDeepMorning = !merged.some(
			(b) => b.start < DEEP_MORNING_END_HOUR * 60 && b.end > WORK_START_HOUR * 60
		);
		out.push({
			date: iso,
			weekday: WEEKDAY_LABELS[cursor.getDay()],
			slots,
			hasDeepMorning
		});
		cursor = addDays(cursor, 1);
	}
	return out;
}

/** countDeepMorningBlocks — convenience for the hero card "X deep
 *  blocks" count. Equal to the days[].hasDeepMorning sum so the
 *  hero card stays a one-liner. */
export function countDeepMorningBlocks(days: FreeSlotsDay[]): number {
	return days.reduce((n, d) => n + (d.hasDeepMorning ? 1 : 0), 0);
}
