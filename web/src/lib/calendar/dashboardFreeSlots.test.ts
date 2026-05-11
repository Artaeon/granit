import { describe, expect, it } from 'vitest';
import type { CalendarEventEntry } from '$lib/api';
import {
	computeFreeSlots,
	countDeepMorningBlocks,
	isWeekday,
	MIN_FREE_SLOT_MINUTES,
	WORK_END_HOUR,
	WORK_START_HOUR
} from './dashboardFreeSlots';

function mkEvent(
	date: string,
	start?: string,
	end?: string,
	over: Partial<CalendarEventEntry> = {}
): CalendarEventEntry {
	return {
		id: 'e_' + date + '_' + (start ?? 'all'),
		title: 'evt',
		date,
		start_time: start,
		end_time: end,
		...over
	} as CalendarEventEntry;
}

describe('isWeekday', () => {
	it('Mon-Fri are weekdays, Sat-Sun are not', () => {
		// 2026-05-11 = Mon, 12 Tue, 13 Wed, 14 Thu, 15 Fri, 16 Sat, 17 Sun
		expect(isWeekday(new Date(2026, 4, 11))).toBe(true);
		expect(isWeekday(new Date(2026, 4, 15))).toBe(true);
		expect(isWeekday(new Date(2026, 4, 16))).toBe(false);
		expect(isWeekday(new Date(2026, 4, 17))).toBe(false);
	});
});

describe('computeFreeSlots', () => {
	// Monday 2026-05-11. Local date so the weekday math is stable.
	const MON = new Date(2026, 4, 11);

	it('returns exactly N weekday days, skipping the weekend', () => {
		const out = computeFreeSlots([], MON, 5);
		expect(out).toHaveLength(5);
		expect(out.map((d) => d.weekday)).toEqual(['Mon', 'Tue', 'Wed', 'Thu', 'Fri']);
	});

	it('returns the full working window when no events block the day', () => {
		const out = computeFreeSlots([], MON, 1);
		expect(out[0].slots).toHaveLength(1);
		expect(out[0].slots[0].durationMinutes).toBe((WORK_END_HOUR - WORK_START_HOUR) * 60);
		expect(out[0].slots[0].startLabel).toBe('09:00');
		expect(out[0].slots[0].endLabel).toBe('18:00');
		expect(out[0].hasDeepMorning).toBe(true);
	});

	it('splits the day around a midday event', () => {
		const out = computeFreeSlots(
			[mkEvent('2026-05-11', '12:00', '13:00')],
			MON,
			1
		);
		// 09:00-12:00 (180min) and 13:00-18:00 (300min) — both > min
		expect(out[0].slots).toHaveLength(2);
		expect(out[0].slots[0].startLabel).toBe('09:00');
		expect(out[0].slots[0].endLabel).toBe('12:00');
		expect(out[0].slots[1].startLabel).toBe('13:00');
		expect(out[0].slots[1].endLabel).toBe('18:00');
	});

	it('drops free gaps shorter than the minimum slot', () => {
		// 09:00-09:30 stub gap is too short, then a long span.
		const out = computeFreeSlots(
			[mkEvent('2026-05-11', '09:30', '10:00')],
			MON,
			1
		);
		// Only 10:00-18:00 should make it through.
		expect(out[0].slots).toHaveLength(1);
		expect(out[0].slots[0].startLabel).toBe('10:00');
		expect(out[0].slots[0].durationMinutes).toBe(8 * 60);
		expect(out[0].slots[0].durationMinutes).toBeGreaterThanOrEqual(MIN_FREE_SLOT_MINUTES);
	});

	it('all-day events block the whole working window', () => {
		const out = computeFreeSlots([mkEvent('2026-05-11')], MON, 1);
		expect(out[0].slots).toHaveLength(0);
		expect(out[0].hasDeepMorning).toBe(false);
	});

	it('merges overlapping events before slot inversion', () => {
		const out = computeFreeSlots(
			[
				mkEvent('2026-05-11', '10:00', '11:30'),
				mkEvent('2026-05-11', '11:00', '12:00')
			],
			MON,
			1
		);
		// Merged block is 10:00-12:00; free slots: 09:00-10:00 (60),
		// 12:00-18:00 (360). Both ≥ min.
		expect(out[0].slots).toHaveLength(2);
		expect(out[0].slots[0].endLabel).toBe('10:00');
		expect(out[0].slots[1].startLabel).toBe('12:00');
	});

	it('flips hasDeepMorning false when any event overlaps 09:00-12:00', () => {
		const out = computeFreeSlots(
			[mkEvent('2026-05-11', '10:00', '10:30')],
			MON,
			1
		);
		expect(out[0].hasDeepMorning).toBe(false);
	});

	it('countDeepMorningBlocks sums hasDeepMorning across the days', () => {
		const days = computeFreeSlots(
			[
				mkEvent('2026-05-11', '10:00', '11:00'), // Mon: no deep
				mkEvent('2026-05-13', '14:00', '15:00') // Wed: still deep
			],
			MON,
			5
		);
		expect(countDeepMorningBlocks(days)).toBe(4);
	});
});
