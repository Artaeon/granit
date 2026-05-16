// findTime — pure helpers behind the "Find time" dialog. Walks the
// feed's CalendarEvent list, projects each timed event into its
// occupied wall-clock minute span, then surfaces the first N gaps of
// at least `minDurationMin` length inside a configurable working
// window across the chosen date range.
//
// Inputs are the same CalendarEvent[] the calendar surface renders,
// so the function composes with the user's active filters — running
// Find time while filtered to project X scopes the conflict check to
// that project's events. That's deliberate: the user is usually
// asking "where can I fit a one-off into THIS view" not "where can I
// fit it into the whole calendar".
//
// All-day events block the entire working window. Tasks (any type
// starting with `task_`) are NOT counted as conflicts — a scheduled
// task can be moved; we don't want the picker hiding slots just
// because the user's parked an open-ended TODO there.

import type { CalendarEvent } from '$lib/api';

export interface FreeGap {
	/** YYYY-MM-DD — the day this gap lives on. */
	date: string;
	/** ISO weekday-month-day label for header rendering. */
	dayLabel: string;
	/** "HH:MM" wall-clock. */
	startLabel: string;
	endLabel: string;
	/** Native Date for the start — used when the caller wants to
	 *  pre-fill a CreateEvent modal from the picked slot. */
	startDate: Date;
	/** Length of the gap in minutes (≥ requested duration). */
	durationMinutes: number;
}

export interface FindTimeOptions {
	/** Inclusive ISO start day. */
	fromISO: string;
	/** Inclusive ISO end day. */
	toISO: string;
	/** Minimum slot length, minutes. */
	minDurationMin: number;
	/** Working-window start hour (24-h int). Defaults 9. */
	workStartHour?: number;
	/** Working-window end hour (24-h int, exclusive). Defaults 18. */
	workEndHour?: number;
	/** Skip Sat/Sun when true. Defaults true — matches the dashboard's
	 *  "deep work" window convention. */
	weekdaysOnly?: boolean;
	/** Max gaps to return. Defaults 10. */
	limit?: number;
}

interface Span {
	start: number; // minutes from midnight, clamped to working window
	end: number;
}

function fmtMin(m: number): string {
	const h = Math.floor(m / 60);
	const mi = m % 60;
	return `${String(h).padStart(2, '0')}:${String(mi).padStart(2, '0')}`;
}

function isoToDate(iso: string): Date {
	const [y, m, d] = iso.split('-').map(Number);
	return new Date(y, m - 1, d);
}

function dateToISO(d: Date): string {
	const y = d.getFullYear();
	const m = String(d.getMonth() + 1).padStart(2, '0');
	const dd = String(d.getDate()).padStart(2, '0');
	return `${y}-${m}-${dd}`;
}

/** Project a single CalendarEvent onto a list of (date → minute-span)
 *  blocks. Events without a recognisable start are ignored; tasks are
 *  treated as non-blocking (see file header). */
function projectEvent(ev: CalendarEvent, winStart: number, winEnd: number): Map<string, Span[]> {
	const out = new Map<string, Span[]>();
	if (ev.type === 'task_scheduled' || ev.type === 'task_due' || ev.type === 'daily') return out;
	// All-day events: emit a single full-window block on the event's date.
	if (!ev.start) {
		const key = ev.date;
		if (!key) return out;
		out.set(key, [{ start: winStart, end: winEnd }]);
		return out;
	}
	const s = new Date(ev.start);
	const e = ev.end ? new Date(ev.end) : new Date(s.getTime() + (ev.durationMinutes ?? 60) * 60_000);
	// Multi-day timed events: split per local-day. Bail above a sane
	// cap to keep the projection bounded on bad data (a malformed
	// all-day-as-timed entry running 1000 years).
	let cursor = new Date(s.getFullYear(), s.getMonth(), s.getDate());
	const endDay = new Date(e.getFullYear(), e.getMonth(), e.getDate());
	let safety = 0;
	while (cursor.getTime() <= endDay.getTime() && safety < 31) {
		safety++;
		// Slice the segment by checking whether the cursor day is the
		// event's start day / end day / a middle day. Middle days are
		// fully occupied within the working window.
		const isStartDay =
			cursor.getFullYear() === s.getFullYear() &&
			cursor.getMonth() === s.getMonth() &&
			cursor.getDate() === s.getDate();
		const isEndDay =
			cursor.getFullYear() === e.getFullYear() &&
			cursor.getMonth() === e.getMonth() &&
			cursor.getDate() === e.getDate();
		const segStartMin = isStartDay ? s.getHours() * 60 + s.getMinutes() : 0;
		const segEndMin = isEndDay ? e.getHours() * 60 + e.getMinutes() : 24 * 60;
		const clipS = Math.max(winStart, segStartMin);
		const clipE = Math.min(winEnd, segEndMin);
		if (clipE > clipS) {
			const key = dateToISO(cursor);
			const list = out.get(key) ?? [];
			list.push({ start: clipS, end: clipE });
			out.set(key, list);
		}
		cursor = new Date(cursor.getFullYear(), cursor.getMonth(), cursor.getDate() + 1);
	}
	return out;
}

function mergeSpans(spans: Span[]): Span[] {
	if (spans.length === 0) return spans;
	const sorted = [...spans].sort((a, b) => a.start - b.start);
	const out: Span[] = [sorted[0]];
	for (let i = 1; i < sorted.length; i++) {
		const last = out[out.length - 1];
		const cur = sorted[i];
		if (cur.start <= last.end) last.end = Math.max(last.end, cur.end);
		else out.push({ ...cur });
	}
	return out;
}

/** findFreeGaps — main entry point. Returns up to `limit` gaps that
 *  satisfy `minDurationMin`, oldest-first. */
export function findFreeGaps(events: CalendarEvent[], opts: FindTimeOptions): FreeGap[] {
	const winStart = (opts.workStartHour ?? 9) * 60;
	const winEnd = (opts.workEndHour ?? 18) * 60;
	const minDur = Math.max(15, opts.minDurationMin);
	const limit = Math.max(1, opts.limit ?? 10);
	const weekdaysOnly = opts.weekdaysOnly ?? true;

	// Project every event into a per-day span list.
	const byDay = new Map<string, Span[]>();
	for (const ev of events) {
		const proj = projectEvent(ev, winStart, winEnd);
		for (const [k, spans] of proj) {
			const list = byDay.get(k) ?? [];
			list.push(...spans);
			byDay.set(k, list);
		}
	}

	// Walk the date range day-by-day, invert blocked spans to gaps,
	// keep those ≥ minDur, stop when limit hit.
	const out: FreeGap[] = [];
	const from = isoToDate(opts.fromISO);
	const to = isoToDate(opts.toISO);
	let cursor = new Date(from);
	let safety = 0;
	while (cursor.getTime() <= to.getTime() && out.length < limit && safety < 366) {
		safety++;
		const dow = cursor.getDay();
		if (weekdaysOnly && (dow === 0 || dow === 6)) {
			cursor = new Date(cursor.getFullYear(), cursor.getMonth(), cursor.getDate() + 1);
			continue;
		}
		const iso = dateToISO(cursor);
		const blocked = mergeSpans(byDay.get(iso) ?? []);
		let walk = winStart;
		const dayLabel = cursor.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' });
		const pushGap = (s: number, e: number) => {
			if (e - s < minDur) return;
			const startDate = new Date(cursor.getFullYear(), cursor.getMonth(), cursor.getDate(), Math.floor(s / 60), s % 60);
			out.push({
				date: iso,
				dayLabel,
				startLabel: fmtMin(s),
				endLabel: fmtMin(e),
				startDate,
				durationMinutes: e - s
			});
		};
		for (const b of blocked) {
			if (b.start > walk) pushGap(walk, b.start);
			if (out.length >= limit) break;
			walk = Math.max(walk, b.end);
		}
		if (out.length < limit && winEnd > walk) pushGap(walk, winEnd);
		cursor = new Date(cursor.getFullYear(), cursor.getMonth(), cursor.getDate() + 1);
	}
	return out;
}
