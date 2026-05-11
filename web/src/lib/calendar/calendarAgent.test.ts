import { describe, expect, it } from 'vitest';
import type { CalendarEventEntry } from '$lib/api';
import {
	buildCalendarAgentPrompt,
	parseCalendarAgentResponse,
	validateCalendarActions,
	summariseCalendarAction,
	computeCalendarRevertPatch,
	mergeCalendarProposals,
	type CalendarAction
} from './calendarAgent';

function mk(id: string, title: string, extra: Partial<CalendarEventEntry> = {}): CalendarEventEntry {
	return {
		id,
		title,
		date: '2026-05-20',
		...extra
	} as CalendarEventEntry;
}

describe('buildCalendarAgentPrompt', () => {
	const today = '2026-05-11';

	it('embeds intent + event digest', () => {
		const { system, user } = buildCalendarAgentPrompt(
			[mk('E1', 'Standup', { date: '2026-05-12', start_time: '09:00', end_time: '09:30' })],
			'move standup to 10am',
			today
		);
		expect(system).toMatch(/STRICT JSON/);
		expect(system).toMatch(/set_event_time/);
		expect(user).toContain('Today is 2026-05-11');
		expect(user).toContain('move standup to 10am');
		expect(user).toContain('id:E1');
		expect(user).toContain('"Standup"');
		expect(user).toContain('09:00–09:30');
	});

	it('flags recurring events so the model knows changes apply to the series', () => {
		const { user } = buildCalendarAgentPrompt(
			[mk('E1', 'Weekly review', { rrule: 'FREQ=WEEKLY' })],
			'x',
			today
		);
		expect(user).toContain('recurring');
	});

	it('marks all-day events distinctly so set_event_time isn\'t mis-applied', () => {
		const { user } = buildCalendarAgentPrompt([mk('E1', 'Holiday')], 'x', today);
		expect(user).toContain('all-day');
	});

	it('lists known projects when supplied', () => {
		const { user } = buildCalendarAgentPrompt([mk('E', 't')], 'x', today, ['Granite', 'Site']);
		expect(user).toContain('Known projects: Granite, Site');
	});

	it('sorts the event digest chronologically — date asc, then start_time asc, untimed last per day', () => {
		const { user } = buildCalendarAgentPrompt(
			[
				mk('LATE', 'Tomorrow late', { date: '2026-05-13', start_time: '15:00', end_time: '16:00' }),
				mk('EARLY', 'Today early', { date: '2026-05-12', start_time: '08:00', end_time: '09:00' }),
				mk('ALLDAY', 'Today all-day', { date: '2026-05-12' }),
				mk('AFTER', 'Today after early', { date: '2026-05-12', start_time: '14:00', end_time: '15:00' })
			],
			'x',
			today
		);
		const earlyAt = user.indexOf('id:EARLY');
		const afterAt = user.indexOf('id:AFTER');
		const alldayAt = user.indexOf('id:ALLDAY');
		const lateAt = user.indexOf('id:LATE');
		// Day 12 timed entries come first in time order, all-day last
		// for that day, then day 13.
		expect(earlyAt).toBeLessThan(afterAt);
		expect(afterAt).toBeLessThan(alldayAt);
		expect(alldayAt).toBeLessThan(lateAt);
	});

	it('does not mutate the caller\'s events array', () => {
		const input = [
			mk('B', 'b', { date: '2026-05-15' }),
			mk('A', 'a', { date: '2026-05-12' })
		];
		buildCalendarAgentPrompt(input, 'x', today);
		// Caller still sees the original order.
		expect(input.map((e) => e.id)).toEqual(['B', 'A']);
	});
});

describe('parseCalendarAgentResponse', () => {
	const ok = JSON.stringify({
		actions: [
			{ eventId: 'E1', kind: 'rename_event', title: 'Daily standup', rationale: 'clearer' },
			{
				eventId: 'E2',
				kind: 'set_event_time',
				start_time: '10:00',
				end_time: '10:30',
				rationale: 'after coffee'
			}
		]
	});

	it('parses a clean response', () => {
		const r = parseCalendarAgentResponse(ok);
		expect(r).toHaveLength(2);
		expect(r[0].kind).toBe('rename_event');
		expect(r[1].start_time).toBe('10:00');
	});

	it('drops entries missing required fields or with unknown kind', () => {
		const r = parseCalendarAgentResponse(
			JSON.stringify({
				actions: [
					{ eventId: 'E1', kind: 'rename_event', title: 't', rationale: 'r' },
					{ kind: 'rename_event', title: 't', rationale: 'no id' },
					{ eventId: 'E2', kind: 'delete_event', rationale: 'bad kind' },
					{ eventId: 'E3', kind: 'rename_event' /* no rationale */ }
				]
			})
		);
		expect(r).toHaveLength(1);
		expect(r[0].eventId).toBe('E1');
	});
});

describe('validateCalendarActions', () => {
	const today = '2026-05-11';
	const live: CalendarEventEntry[] = [
		mk('E1', 'Standup', { date: '2026-05-12', start_time: '09:00', end_time: '09:30' }),
		mk('E2', 'Lunch', { date: '2026-05-12' }) // all-day
	];

	it('drops hallucinated eventId', () => {
		const out = validateCalendarActions(
			[
				{ eventId: 'E1', kind: 'rename_event', title: 't', rationale: 'r' },
				{ eventId: 'ghost', kind: 'rename_event', title: 't', rationale: 'r' }
			],
			live,
			today
		);
		expect(out.map((a) => a.eventId)).toEqual(['E1']);
	});

	it('drops move_event_to_date with malformed date', () => {
		const out = validateCalendarActions(
			[
				{ eventId: 'E1', kind: 'move_event_to_date', date: 'tomorrow', rationale: 'r' },
				{ eventId: 'E1', kind: 'move_event_to_date', date: '2026-06-01', rationale: 'r' }
			],
			live,
			today
		);
		expect(out.map((a) => a.date)).toEqual(['2026-06-01']);
	});

	it('drops move_event_to_date set to a PAST date', () => {
		const out = validateCalendarActions(
			[{ eventId: 'E1', kind: 'move_event_to_date', date: '2026-01-01', rationale: 'r' }],
			live,
			today
		);
		expect(out).toEqual([]);
	});

	it('accepts move_event_to_date set to TODAY', () => {
		const out = validateCalendarActions(
			[{ eventId: 'E1', kind: 'move_event_to_date', date: today, rationale: 'r' }],
			live,
			today
		);
		expect(out).toHaveLength(1);
	});

	it('drops set_event_time with malformed times', () => {
		const out = validateCalendarActions(
			[
				{
					eventId: 'E1',
					kind: 'set_event_time',
					start_time: '9:00',
					end_time: '10:00',
					rationale: 'r'
				}, // missing leading zero
				{
					eventId: 'E1',
					kind: 'set_event_time',
					start_time: '25:00',
					end_time: '26:00',
					rationale: 'r'
				}, // > 23h
				{
					eventId: 'E1',
					kind: 'set_event_time',
					start_time: '10:00',
					end_time: '10:30',
					rationale: 'r'
				} // valid
			],
			live,
			today
		);
		expect(out).toHaveLength(1);
		expect(out[0].start_time).toBe('10:00');
	});

	it('drops set_event_time where end_time <= start_time', () => {
		const out = validateCalendarActions(
			[
				{
					eventId: 'E1',
					kind: 'set_event_time',
					start_time: '14:00',
					end_time: '13:00',
					rationale: 'r'
				},
				{
					eventId: 'E1',
					kind: 'set_event_time',
					start_time: '14:00',
					end_time: '14:00',
					rationale: 'r'
				}
			],
			live,
			today
		);
		expect(out).toEqual([]);
	});

	it('drops empty location/project/title rewrites', () => {
		const out = validateCalendarActions(
			[
				{ eventId: 'E1', kind: 'rename_event', title: '  ', rationale: 'r' },
				{ eventId: 'E1', kind: 'set_event_location', location: '', rationale: 'r' },
				{ eventId: 'E1', kind: 'set_event_project', project: '\n', rationale: 'r' }
			],
			live,
			today
		);
		expect(out).toEqual([]);
	});

	it('passes through no-arg clear actions', () => {
		const out = validateCalendarActions(
			[
				{ eventId: 'E1', kind: 'clear_event_location', rationale: 'r' },
				{ eventId: 'E2', kind: 'clear_event_project', rationale: 'r' }
			],
			live,
			today
		);
		expect(out).toHaveLength(2);
	});

	it('normalises color to lowercase', () => {
		const out = validateCalendarActions(
			[{ eventId: 'E1', kind: 'set_event_color', color: 'BLUE', rationale: 'r' }],
			live,
			today
		);
		expect(out[0].color).toBe('blue');
	});
});

describe('computeCalendarRevertPatch', () => {
	it('reverts rename_event to the pre-state title', () => {
		expect(
			computeCalendarRevertPatch(
				{ eventId: 'E1', kind: 'rename_event', title: 'new', rationale: '' },
				mk('E1', 'original')
			)
		).toEqual({ title: 'original' });
	});

	it('reverts move_event_to_date to the prior date', () => {
		expect(
			computeCalendarRevertPatch(
				{ eventId: 'E1', kind: 'move_event_to_date', date: '2026-06-01', rationale: '' },
				mk('E1', 't', { date: '2026-05-20' })
			)
		).toEqual({ date: '2026-05-20' });
	});

	it('reverts set_event_time to both prior times (or empty strings if all-day)', () => {
		expect(
			computeCalendarRevertPatch(
				{ eventId: 'E1', kind: 'set_event_time', start_time: '10:00', end_time: '11:00', rationale: '' },
				mk('E1', 't', { start_time: '09:00', end_time: '09:30' })
			)
		).toEqual({ start_time: '09:00', end_time: '09:30' });

		// All-day before — revert clears the times so the event
		// returns to all-day (omitempty on the server).
		expect(
			computeCalendarRevertPatch(
				{ eventId: 'E1', kind: 'set_event_time', start_time: '10:00', end_time: '11:00', rationale: '' },
				mk('E1', 't')
			)
		).toEqual({ start_time: '', end_time: '' });
	});

	it('reverts color/location/project set+clear pairs', () => {
		const pre = mk('E1', 't', { color: 'red', location: 'office', project_id: 'Granite' });
		expect(
			computeCalendarRevertPatch(
				{ eventId: 'E1', kind: 'set_event_color', color: 'blue', rationale: '' },
				pre
			)
		).toEqual({ color: 'red' });
		expect(
			computeCalendarRevertPatch(
				{ eventId: 'E1', kind: 'clear_event_location', rationale: '' },
				pre
			)
		).toEqual({ location: 'office' });
		expect(
			computeCalendarRevertPatch(
				{ eventId: 'E1', kind: 'clear_event_project', rationale: '' },
				pre
			)
		).toEqual({ project_id: 'Granite' });
	});
});

describe('summariseCalendarAction', () => {
	const e = mk('E1', 'Daily team standup');
	it('formats per-kind with the event title', () => {
		expect(
			summariseCalendarAction(
				{ eventId: 'E1', kind: 'move_event_to_date', date: '2026-06-01', rationale: '' },
				e
			)
		).toMatch(/2026-06-01/);
		expect(
			summariseCalendarAction(
				{
					eventId: 'E1',
					kind: 'set_event_time',
					start_time: '10:00',
					end_time: '11:00',
					rationale: ''
				},
				e
			)
		).toMatch(/10:00–11:00/);
	});

	it('falls back to eventId when event is undefined', () => {
		expect(summariseCalendarAction({ eventId: 'g', kind: 'rename_event', title: 't', rationale: '' }, undefined)).toMatch(
			/Rename "g"/
		);
	});
});

describe('mergeCalendarProposals', () => {
	it('preserves applied state across re-parse', () => {
		const prev = [{ eventId: 'E', kind: 'rename_event' as const, rationale: 'r', applied: true }];
		const out = mergeCalendarProposals(prev, [{ eventId: 'E', kind: 'rename_event', rationale: 'r' }]);
		expect(out[0].applied).toBe(true);
	});
});
