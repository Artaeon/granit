import { describe, expect, it } from 'vitest';
import type { CalendarEventEntry, Task } from '$lib/api';
import {
	renderCalendarContext,
	loadCalendarContext,
	CAL_EVENTS_CAP,
	CAL_OVERDUE_CAP
} from './calendarManagerContext';

const TODAY = '2026-05-12';

function mkEvent(id: string, date: string, over: Partial<CalendarEventEntry> = {}): CalendarEventEntry {
	return { id, title: `Event ${id}`, date, ...over } as CalendarEventEntry;
}
function mkTask(text: string, over: Partial<Task> = {}): Task {
	return {
		id: 't_' + text,
		notePath: 'inbox.md',
		lineNum: 1,
		text,
		done: false,
		priority: 0,
		...over
	} as Task;
}

describe('renderCalendarContext', () => {
	it('opens with the date window so the AI knows scope', () => {
		const out = renderCalendarContext({
			windowStart: TODAY,
			windowEnd: '2026-05-26',
			todayISO: TODAY,
			upcomingEvents: [],
			dueToday: [],
			overdue: [],
			scheduledThisWeek: []
		});
		expect(out.startsWith(`# Calendar — ${TODAY} → 2026-05-26`)).toBe(true);
		expect(out).toContain(`Today is ${TODAY}`);
	});

	it('orders sections: upcoming → overdue → due today → scheduled', () => {
		const out = renderCalendarContext({
			windowStart: TODAY,
			windowEnd: '2026-05-26',
			todayISO: TODAY,
			upcomingEvents: [mkEvent('E', '2026-05-13')],
			dueToday: [mkTask('today task', { dueDate: TODAY })],
			overdue: [mkTask('overdue task', { dueDate: '2026-05-01' })],
			scheduledThisWeek: [mkTask('sched task', { scheduledStart: '2026-05-14T09:00' })]
		});
		const a = out.indexOf('## Upcoming events');
		const b = out.indexOf('## Overdue');
		const c = out.indexOf('## Due today');
		const d = out.indexOf('## Scheduled this week');
		expect(a).toBeLessThan(b);
		expect(b).toBeLessThan(c);
		expect(c).toBeLessThan(d);
	});

	it('omits empty sections', () => {
		const out = renderCalendarContext({
			windowStart: TODAY,
			windowEnd: '2026-05-26',
			todayISO: TODAY,
			upcomingEvents: [],
			dueToday: [],
			overdue: [],
			scheduledThisWeek: []
		});
		expect(out).not.toContain('## Upcoming');
		expect(out).not.toContain('## Overdue');
	});

	it('shows truncation note when totals exceed list length', () => {
		const events = Array.from({ length: 30 }, (_, i) => mkEvent(`E${i}`, `2026-05-${String(13 + (i % 10)).padStart(2, '0')}`));
		const out = renderCalendarContext({
			windowStart: TODAY,
			windowEnd: '2026-05-26',
			todayISO: TODAY,
			upcomingEvents: events,
			dueToday: [],
			overdue: [],
			scheduledThisWeek: [],
			totals: { upcomingEvents: 87 }
		});
		expect(out).toContain('showing 30 of 87');
	});

	it('marks recurring events with ↻ and all-day vs timed events distinctly', () => {
		const out = renderCalendarContext({
			windowStart: TODAY,
			windowEnd: '2026-05-26',
			todayISO: TODAY,
			upcomingEvents: [
				mkEvent('R', '2026-05-13', { title: 'Weekly review', rrule: 'FREQ=WEEKLY' }),
				mkEvent('A', '2026-05-14', { title: 'Holiday' }),
				mkEvent('T', '2026-05-15', { title: 'Sprint planning', start_time: '10:00', end_time: '11:00' })
			],
			dueToday: [],
			overdue: [],
			scheduledThisWeek: []
		});
		expect(out).toContain('↻');
		expect(out).toContain('all-day');
		expect(out).toContain('10:00–11:00');
	});
});

describe('loadCalendarContext', () => {
	it('filters events to the date window and sorts chronologically', async () => {
		const b = await loadCalendarContext(
			{
				listEvents: async () => [
					mkEvent('past', '2025-12-01'),
					mkEvent('future', '2027-01-01'),
					mkEvent('day1late', '2026-05-13', { start_time: '15:00' }),
					mkEvent('day1early', '2026-05-13', { start_time: '08:00' }),
					mkEvent('inrange', '2026-05-15')
				],
				listTasks: async () => []
			},
			{ todayISO: TODAY, daysAhead: 14 }
		);
		expect(b.upcomingEvents.map((e) => e.id)).toEqual(['day1early', 'day1late', 'inrange']);
		expect(b.totals?.upcomingEvents).toBe(3);
	});

	it('classifies tasks into overdue / due-today / scheduled-in-window', async () => {
		const b = await loadCalendarContext(
			{
				listEvents: async () => [],
				listTasks: async () => [
					mkTask('overdue', { dueDate: '2026-05-01' }),
					mkTask('today', { dueDate: TODAY }),
					mkTask('done old', { done: true, dueDate: '2026-05-01' }),
					mkTask('scheduled this week', { scheduledStart: '2026-05-14T09:00' }),
					mkTask('out of window', { scheduledStart: '2027-06-01T09:00' })
				]
			},
			{ todayISO: TODAY, daysAhead: 14 }
		);
		expect(b.overdue.map((t) => t.text)).toEqual(['overdue']);
		expect(b.dueToday.map((t) => t.text)).toEqual(['today']);
		expect(b.scheduledThisWeek.map((t) => t.text)).toEqual(['scheduled this week']);
	});

	it('caps each list and reports totals', async () => {
		const events = Array.from({ length: 50 }, (_, i) =>
			mkEvent(`E${i}`, '2026-05-13')
		);
		const overdue = Array.from({ length: 30 }, (_, i) =>
			mkTask(`overdue ${i}`, { dueDate: '2026-05-01' })
		);
		const b = await loadCalendarContext(
			{
				listEvents: async () => events,
				listTasks: async () => overdue
			},
			{ todayISO: TODAY, daysAhead: 14 }
		);
		expect(b.upcomingEvents.length).toBe(CAL_EVENTS_CAP);
		expect(b.overdue.length).toBe(CAL_OVERDUE_CAP);
		expect(b.totals?.upcomingEvents).toBe(50);
		expect(b.totals?.overdue).toBe(30);
	});

	it('tolerates partial failure (events fetch throws)', async () => {
		const b = await loadCalendarContext(
			{
				listEvents: async () => {
					throw new Error('boom');
				},
				listTasks: async () => [mkTask('still here', { dueDate: TODAY })]
			},
			{ todayISO: TODAY }
		);
		expect(b.upcomingEvents).toEqual([]);
		expect(b.dueToday.map((t) => t.text)).toEqual(['still here']);
	});

	it('uses daysAhead to size the window', async () => {
		const b = await loadCalendarContext(
			{ listEvents: async () => [], listTasks: async () => [] },
			{ todayISO: '2026-05-12', daysAhead: 7 }
		);
		expect(b.windowStart).toBe('2026-05-12');
		expect(b.windowEnd).toBe('2026-05-19');
	});
});
