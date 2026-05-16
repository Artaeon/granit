import { describe, expect, it } from 'vitest';
import type { CalendarEvent } from '$lib/api';
import { findFreeGaps } from './findTime';

function ev(over: Partial<CalendarEvent>): CalendarEvent {
	return {
		type: 'event',
		title: 'e',
		...over
	} as CalendarEvent;
}

// Use a known weekday so weekdaysOnly=true doesn't drop the days.
// 2026-05-18 Mon, 19 Tue, 20 Wed, 21 Thu, 22 Fri.
const MON = '2026-05-18';
const TUE = '2026-05-19';

describe('findFreeGaps', () => {
	it('returns the full working window when the day has no events', () => {
		const gaps = findFreeGaps([], {
			fromISO: MON,
			toISO: MON,
			minDurationMin: 60,
			workStartHour: 9,
			workEndHour: 17
		});
		expect(gaps).toHaveLength(1);
		expect(gaps[0].startLabel).toBe('09:00');
		expect(gaps[0].endLabel).toBe('17:00');
		expect(gaps[0].durationMinutes).toBe(480);
	});

	it('skips weekends when weekdaysOnly is true (default)', () => {
		// Sat 2026-05-16, Sun 17, Mon 18.
		const gaps = findFreeGaps([], {
			fromISO: '2026-05-16',
			toISO: '2026-05-18',
			minDurationMin: 60
		});
		// Only Monday should produce a gap.
		expect(gaps.every((g) => g.date === '2026-05-18')).toBe(true);
		expect(gaps).toHaveLength(1);
	});

	it('produces gaps around a midday meeting', () => {
		const gaps = findFreeGaps(
			[ev({ start: `${MON}T11:00:00`, end: `${MON}T12:00:00` })],
			{ fromISO: MON, toISO: MON, minDurationMin: 60, workStartHour: 9, workEndHour: 17 }
		);
		expect(gaps).toHaveLength(2);
		expect(gaps[0].startLabel).toBe('09:00');
		expect(gaps[0].endLabel).toBe('11:00');
		expect(gaps[1].startLabel).toBe('12:00');
		expect(gaps[1].endLabel).toBe('17:00');
	});

	it('drops gaps shorter than the requested duration', () => {
		const gaps = findFreeGaps(
			[
				ev({ start: `${MON}T09:30:00`, end: `${MON}T10:30:00` }),
				ev({ start: `${MON}T11:00:00`, end: `${MON}T16:30:00` })
			],
			{ fromISO: MON, toISO: MON, minDurationMin: 60, workStartHour: 9, workEndHour: 17 }
		);
		// Gap 09:00-09:30 (30min) is dropped; 10:30-11:00 (30min) is dropped;
		// 16:30-17:00 (30min) is dropped. Nothing surfaces.
		expect(gaps).toHaveLength(0);
	});

	it('treats all-day events as full-window blocks', () => {
		const gaps = findFreeGaps(
			[ev({ date: MON, title: 'PTO' })],
			{ fromISO: MON, toISO: TUE, minDurationMin: 60, workStartHour: 9, workEndHour: 17 }
		);
		// Mon fully blocked → only Tue surfaces.
		expect(gaps.every((g) => g.date === TUE)).toBe(true);
	});

	it('ignores tasks — they are not conflicts', () => {
		const gaps = findFreeGaps(
			[
				ev({ type: 'task_scheduled', start: `${MON}T10:00:00`, end: `${MON}T16:00:00` })
			],
			{ fromISO: MON, toISO: MON, minDurationMin: 60, workStartHour: 9, workEndHour: 17 }
		);
		// Task spans most of the day but doesn't block — full window
		// should be one gap.
		expect(gaps).toHaveLength(1);
		expect(gaps[0].durationMinutes).toBe(480);
	});

	it('limits the result set', () => {
		const gaps = findFreeGaps([], {
			fromISO: MON,
			toISO: '2026-05-29', // through Fri the week after
			minDurationMin: 60,
			workStartHour: 9,
			workEndHour: 17,
			limit: 3
		});
		expect(gaps).toHaveLength(3);
	});

	it('merges overlapping blocks before inverting', () => {
		const gaps = findFreeGaps(
			[
				ev({ start: `${MON}T10:00:00`, end: `${MON}T11:30:00` }),
				ev({ start: `${MON}T11:00:00`, end: `${MON}T12:00:00` })
			],
			{ fromISO: MON, toISO: MON, minDurationMin: 60, workStartHour: 9, workEndHour: 17 }
		);
		// Combined block is 10:00–12:00. Pre-block 09:00–10:00 is
		// exactly minDur → kept. Post-block 12:00–17:00 is kept.
		expect(gaps).toHaveLength(2);
		expect(gaps[0].endLabel).toBe('10:00');
		expect(gaps[1].startLabel).toBe('12:00');
	});
});
