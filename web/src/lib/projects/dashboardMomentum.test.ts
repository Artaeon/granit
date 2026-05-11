import { describe, expect, it } from 'vitest';
import type { Task } from '$lib/api';
import { computeMomentum, MOMENTUM_WEEKS } from './dashboardMomentum';

function mkDone(completedAt: string): Task {
	return {
		id: 't_' + completedAt,
		notePath: 'x.md',
		lineNum: 1,
		text: 'done',
		done: true,
		priority: 0,
		completedAt
	} as Task;
}

describe('computeMomentum', () => {
	// Fixed Wednesday so the math is easy to reason about: the ISO
	// week boundary lands cleanly to either side.
	const NOW = new Date('2026-05-13T12:00:00Z');

	it('returns exactly MOMENTUM_WEEKS bars in chronological order', () => {
		const bars = computeMomentum([], NOW);
		expect(bars).toHaveLength(MOMENTUM_WEEKS);
		expect(bars[bars.length - 1].isThisWeek).toBe(true);
		// Only the last bar should be 'this week'.
		expect(bars.slice(0, -1).every((b) => !b.isThisWeek)).toBe(true);
	});

	it('counts completions in the current week', () => {
		const bars = computeMomentum(
			[mkDone('2026-05-12T09:00:00Z'), mkDone('2026-05-13T10:00:00Z')],
			NOW
		);
		const cur = bars[bars.length - 1];
		expect(cur.count).toBe(2);
		expect(cur.isThisWeek).toBe(true);
	});

	it('drops completions that fall outside the 4-week window', () => {
		// 60 days ago — well before the 4-week window
		const old = computeMomentum([mkDone('2026-03-01T10:00:00Z')], NOW);
		expect(old.reduce((s, b) => s + b.count, 0)).toBe(0);
	});

	it('silently skips tasks with no/invalid completedAt', () => {
		const bars = computeMomentum(
			[
				{ id: 'a', notePath: '', lineNum: 1, text: '', done: true, priority: 0 } as Task,
				{ ...mkDone('not-a-date') }
			],
			NOW
		);
		expect(bars.reduce((s, b) => s + b.count, 0)).toBe(0);
	});

	it('labels the current week as "Now" and past weeks with the ISO week number', () => {
		const bars = computeMomentum([], NOW);
		expect(bars[bars.length - 1].label).toBe('Now');
		// Past labels should be 2-digit week numbers, not 'Now'
		for (const b of bars.slice(0, -1)) {
			expect(b.label).not.toBe('Now');
			expect(b.label).toMatch(/^\d{2}$/);
		}
	});
});
