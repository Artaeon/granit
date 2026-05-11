import { describe, expect, it } from 'vitest';
import type { Task } from '$lib/api';
import {
	buildAgentPrompt,
	parseAgentResponse,
	validateActions,
	summariseAction,
	type TaskAction
} from './agent';

function mk(id: string, text: string, extra: Partial<Task> = {}): Task {
	return {
		id,
		text,
		done: false,
		notePath: 'inbox.md',
		createdAt: '2026-01-01T00:00:00Z',
		updatedAt: '2026-01-01T00:00:00Z',
		...extra
	} as unknown as Task;
}

describe('buildAgentPrompt', () => {
	const today = '2026-05-11';

	it('embeds the user intent and the task digest', () => {
		const { system, user } = buildAgentPrompt(
			[mk('a', 'write report', { priority: 2, dueDate: '2026-05-15' })],
			'lower priority on the report',
			today
		);
		expect(system).toMatch(/STRICT JSON/);
		expect(system).toMatch(/set_priority/);
		expect(system).toMatch(/archive/);
		expect(user).toContain('lower priority on the report');
		expect(user).toContain('Today is 2026-05-11');
		expect(user).toContain('id:a — write report');
		expect(user).toContain('p2');
		expect(user).toContain('due 2026-05-15');
	});

	it('includes known project ids when supplied', () => {
		const { user } = buildAgentPrompt([mk('a', 'x')], 'do something', today, ['Granite', 'Site']);
		expect(user).toContain('Known project ids: Granite, Site');
	});

	it('omits the projects line when none supplied', () => {
		const { user } = buildAgentPrompt([mk('a', 'x')], 'do something', today, []);
		expect(user).not.toContain('Known project ids');
	});

	it('marks done tasks in the digest so the model knows to skip them', () => {
		const { user } = buildAgentPrompt(
			[mk('a', 'shipped feature', { done: true }), mk('b', 'open task')],
			'cleanup',
			today
		);
		expect(user).toContain('id:a — shipped feature · done');
		expect(user).not.toMatch(/id:b — open task · done/);
	});

	it('produces deterministic output for identical inputs', () => {
		const tasks = [mk('a', 'x', { priority: 1 })];
		const a = buildAgentPrompt(tasks, 'hi', today);
		const b = buildAgentPrompt(tasks, 'hi', today);
		expect(a).toEqual(b);
	});
});

describe('parseAgentResponse', () => {
	const ok = JSON.stringify({
		actions: [
			{ taskId: 'a', kind: 'set_priority', priority: 1, rationale: 'p1 because urgent' },
			{ taskId: 'b', kind: 'archive', rationale: 'dead weight from 2024 brainstorm' }
		]
	});

	it('parses a clean response', () => {
		const got = parseAgentResponse(ok);
		expect(got).toHaveLength(2);
		expect(got[0].kind).toBe('set_priority');
		expect(got[0].priority).toBe(1);
	});

	it('strips ```json fences', () => {
		const got = parseAgentResponse('```json\n' + ok + '\n```');
		expect(got).toHaveLength(2);
	});

	it('slices JSON out of trailing prose', () => {
		const got = parseAgentResponse('Here you go:\n' + ok + '\n\nLet me know.');
		expect(got).toHaveLength(2);
	});

	it('returns [] on garbage', () => {
		expect(parseAgentResponse('not json')).toEqual([]);
		expect(parseAgentResponse('')).toEqual([]);
		expect(parseAgentResponse('{not closed')).toEqual([]);
	});

	it('returns [] when actions is missing or not an array', () => {
		expect(parseAgentResponse('{"foo":1}')).toEqual([]);
		expect(parseAgentResponse('{"actions":"nope"}')).toEqual([]);
	});

	it('drops entries with missing taskId / kind / rationale', () => {
		const messy = JSON.stringify({
			actions: [
				{ taskId: 'a', kind: 'archive', rationale: 'real' },
				{ kind: 'archive', rationale: 'no id' },
				{ taskId: 'b', rationale: 'no kind' },
				{ taskId: 'c', kind: 'archive' }, // no rationale
				{ taskId: 'd', kind: 'nonsense_kind', rationale: 'bad enum' },
				{ taskId: '   ', kind: 'archive', rationale: 'blank id' }
			]
		});
		const got = parseAgentResponse(messy);
		expect(got).toHaveLength(1);
		expect(got[0].taskId).toBe('a');
	});

	it('keeps the argument fields per kind', () => {
		const r = parseAgentResponse(
			JSON.stringify({
				actions: [
					{ taskId: 'a', kind: 'set_due', dueDate: '2026-06-01', rationale: 'before review' },
					{
						taskId: 'b',
						kind: 'schedule',
						scheduledStart: '2026-05-12T09:00',
						durationMinutes: 45,
						rationale: 'fresh morning'
					},
					{ taskId: 'c', kind: 'snooze', snoozedUntil: '2026-08-01', rationale: 'seasonal' },
					{ taskId: 'd', kind: 'set_project', projectId: 'Granite', rationale: 'belongs there' },
					{ taskId: 'e', kind: 'change_text', text: 'cleaner title', rationale: 'clearer intent' }
				]
			})
		);
		expect(r[0].dueDate).toBe('2026-06-01');
		expect(r[1].scheduledStart).toBe('2026-05-12T09:00');
		expect(r[1].durationMinutes).toBe(45);
		expect(r[2].snoozedUntil).toBe('2026-08-01');
		expect(r[3].projectId).toBe('Granite');
		expect(r[4].text).toBe('cleaner title');
	});
});

describe('validateActions', () => {
	const live = [mk('a', 'one'), mk('b', 'two'), mk('c', 'three')];

	it('drops actions whose taskId is not live (hallucinated id)', () => {
		const acts: TaskAction[] = [
			{ taskId: 'a', kind: 'archive', rationale: 'real' },
			{ taskId: 'ghost', kind: 'archive', rationale: 'hallucinated' }
		];
		expect(validateActions(acts, live).map((a) => a.taskId)).toEqual(['a']);
	});

	it('clamps priority to 1..3 and rounds floats', () => {
		const out = validateActions(
			[
				{ taskId: 'a', kind: 'set_priority', priority: 7, rationale: 'urgent' },
				{ taskId: 'b', kind: 'set_priority', priority: 0, rationale: 'low' },
				{ taskId: 'c', kind: 'set_priority', priority: 2.4, rationale: 'mid' }
			],
			live
		);
		expect(out.map((a) => a.priority)).toEqual([3, 1, 2]);
	});

	it('drops set_priority without a numeric priority', () => {
		const out = validateActions(
			[{ taskId: 'a', kind: 'set_priority', rationale: 'no priority arg' }],
			live
		);
		expect(out).toEqual([]);
	});

	it('drops set_due / snooze with non YYYY-MM-DD dates', () => {
		const out = validateActions(
			[
				{ taskId: 'a', kind: 'set_due', dueDate: 'tomorrow', rationale: 'words' },
				{ taskId: 'b', kind: 'set_due', dueDate: '2026-06-01', rationale: 'real' },
				{ taskId: 'c', kind: 'snooze', snoozedUntil: 'next week', rationale: 'no' }
			],
			live
		);
		expect(out.map((a) => a.taskId)).toEqual(['b']);
	});

	it('keeps schedule with ISO 8601 start, drops free-text starts', () => {
		const out = validateActions(
			[
				{
					taskId: 'a',
					kind: 'schedule',
					scheduledStart: '2026-05-12T09:00',
					rationale: 'morning'
				},
				{
					taskId: 'b',
					kind: 'schedule',
					scheduledStart: 'tomorrow at nine',
					rationale: 'free text'
				}
			],
			live
		);
		expect(out.map((a) => a.taskId)).toEqual(['a']);
	});

	it('drops change_text with empty/whitespace text', () => {
		const out = validateActions(
			[
				{ taskId: 'a', kind: 'change_text', text: 'a real title', rationale: 'better' },
				{ taskId: 'b', kind: 'change_text', text: '   ', rationale: 'blank' }
			],
			live
		);
		expect(out.map((a) => a.taskId)).toEqual(['a']);
		expect(out[0].text).toBe('a real title');
	});

	it('passes through no-arg actions', () => {
		const acts: TaskAction[] = [
			{ taskId: 'a', kind: 'mark_done', rationale: 'shipped' },
			{ taskId: 'b', kind: 'archive', rationale: 'drop' },
			{ taskId: 'c', kind: 'clear_due', rationale: 'reset' }
		];
		expect(validateActions(acts, live)).toHaveLength(3);
	});

	it('allows set_project with empty string (= clear project)', () => {
		const out = validateActions(
			[{ taskId: 'a', kind: 'set_project', projectId: '', rationale: 'unassign' }],
			live
		);
		expect(out).toHaveLength(1);
		expect(out[0].projectId).toBe('');
	});
});

describe('summariseAction', () => {
	const t = mk('a', 'write the weekly report for the team');

	it('formats common actions with the task title', () => {
		expect(summariseAction({ taskId: 'a', kind: 'set_priority', priority: 1, rationale: '' }, t))
			.toMatch(/Set "write the weekly report.*" to P1/);
		expect(summariseAction({ taskId: 'a', kind: 'archive', rationale: '' }, t)).toMatch(/Archive/);
		expect(summariseAction({ taskId: 'a', kind: 'set_due', dueDate: '2026-06-01', rationale: '' }, t))
			.toMatch(/Due .* on 2026-06-01/);
	});

	it('handles set_project with and without a project id', () => {
		expect(
			summariseAction({ taskId: 'a', kind: 'set_project', projectId: 'Granite', rationale: '' }, t)
		).toMatch(/Move .* to project "Granite"/);
		expect(
			summariseAction({ taskId: 'a', kind: 'set_project', projectId: '', rationale: '' }, t)
		).toMatch(/Remove .* from its project/);
	});

	it('falls back to taskId when the task lookup is undefined', () => {
		expect(summariseAction({ taskId: 'ghost', kind: 'archive', rationale: '' }, undefined)).toMatch(
			/Archive ghost/
		);
	});
});
