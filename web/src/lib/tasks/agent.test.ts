import { describe, expect, it } from 'vitest';
import type { Task } from '$lib/api';
import {
	buildAgentPrompt,
	parseAgentResponse,
	validateActions,
	summariseAction,
	computeRevertPatch,
	mergeProposals,
	type TaskAction,
	type ProposalState
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

describe('mergeProposals', () => {
	const a = (taskId: string, kind: TaskAction['kind'], extra: Partial<TaskAction> = {}): TaskAction => ({
		taskId,
		kind,
		rationale: 'r',
		...extra
	});

	it('preserves applied state across a re-parse with the same row', () => {
		const prev: ProposalState[] = [{ ...a('t1', 'archive'), applied: true }];
		const out = mergeProposals(prev, [a('t1', 'archive')]);
		expect(out).toHaveLength(1);
		expect(out[0].applied).toBe(true);
	});

	it('keeps a previously-applied row even when the new parse drops it', () => {
		// Common case: user accepted, parent reloaded, task left the
		// filtered scope, validateActions filtered the row out. The
		// row must stay visible so the user keeps the audit trail.
		const prev: ProposalState[] = [{ ...a('t1', 'archive'), applied: true }];
		const out = mergeProposals(prev, []);
		expect(out).toHaveLength(1);
		expect(out[0].taskId).toBe('t1');
		expect(out[0].applied).toBe(true);
	});

	it('keeps a previously-rejected row even when the new parse drops it', () => {
		const prev: ProposalState[] = [{ ...a('t1', 'archive'), rejected: true }];
		const out = mergeProposals(prev, []);
		expect(out).toHaveLength(1);
		expect(out[0].rejected).toBe(true);
	});

	it('drops PENDING rows that the new parse no longer mentions', () => {
		// Model retracted — fine, less churn for the user.
		const prev: ProposalState[] = [a('t1', 'archive') as ProposalState];
		const out = mergeProposals(prev, []);
		expect(out).toEqual([]);
	});

	it('uses the NEW action args for pending rows', () => {
		const prev: ProposalState[] = [a('t1', 'set_priority', { priority: 1 }) as ProposalState];
		const out = mergeProposals(prev, [a('t1', 'set_priority', { priority: 3 })]);
		expect(out).toHaveLength(1);
		expect(out[0].priority).toBe(3);
	});

	it('FREEZES the action args for applied rows (do not lie about what was applied)', () => {
		const prev: ProposalState[] = [
			{ ...a('t1', 'set_priority', { priority: 1 }), applied: true }
		];
		const out = mergeProposals(prev, [a('t1', 'set_priority', { priority: 3 })]);
		expect(out).toHaveLength(1);
		expect(out[0].priority).toBe(1); // not 3 — the user accepted priority=1
		expect(out[0].applied).toBe(true);
	});

	it('appends new rows the previous parse did not have', () => {
		const prev: ProposalState[] = [a('t1', 'archive') as ProposalState];
		const out = mergeProposals(prev, [a('t1', 'archive'), a('t2', 'snooze')]);
		expect(out.map((p) => p.taskId)).toEqual(['t1', 't2']);
	});

	it('orders applied-but-dropped rows after the new parse output', () => {
		// Visual sanity: live (newly-parsed) rows on top, frozen rows
		// from past chunks underneath so they don't move around.
		const prev: ProposalState[] = [
			{ ...a('t1', 'archive'), applied: true },
			a('t2', 'snooze') as ProposalState
		];
		const out = mergeProposals(prev, [a('t3', 'set_priority', { priority: 2 })]);
		expect(out.map((p) => p.taskId)).toEqual(['t3', 't1']); // t2 was pending, dropped
	});
});

describe('computeRevertPatch', () => {
	it('reverts set_priority to the pre-state priority', () => {
		const pre = mk('a', 'x', { priority: 1 });
		const rev = computeRevertPatch(
			{ taskId: 'a', kind: 'set_priority', priority: 3, rationale: '' },
			pre
		);
		expect(rev).toEqual({ priority: 1 });
	});

	it('reverts set_priority when there was no prior priority (→ 0)', () => {
		const pre = mk('a', 'x');
		const rev = computeRevertPatch(
			{ taskId: 'a', kind: 'set_priority', priority: 2, rationale: '' },
			pre
		);
		expect(rev).toEqual({ priority: 0 });
	});

	it('reverts set_due AND clear_due to the prior due date (empty if none)', () => {
		const pre1 = mk('a', 'x', { dueDate: '2026-05-15' });
		expect(
			computeRevertPatch({ taskId: 'a', kind: 'set_due', dueDate: '2026-06-01', rationale: '' }, pre1)
		).toEqual({ dueDate: '2026-05-15' });
		expect(
			computeRevertPatch({ taskId: 'a', kind: 'clear_due', rationale: '' }, pre1)
		).toEqual({ dueDate: '2026-05-15' });

		const pre2 = mk('a', 'x');
		expect(
			computeRevertPatch({ taskId: 'a', kind: 'set_due', dueDate: '2026-06-01', rationale: '' }, pre2)
		).toEqual({ dueDate: '' });
	});

	it('reverts schedule: restores prior schedule when present, else clearSchedule', () => {
		const had = mk('a', 'x', { scheduledStart: '2026-05-12T09:00', durationMinutes: 60 });
		expect(
			computeRevertPatch(
				{ taskId: 'a', kind: 'schedule', scheduledStart: '2026-05-13T14:00', rationale: '' },
				had
			)
		).toEqual({ scheduledStart: '2026-05-12T09:00', durationMinutes: 60 });

		const free = mk('a', 'x');
		expect(
			computeRevertPatch(
				{ taskId: 'a', kind: 'schedule', scheduledStart: '2026-05-13T14:00', rationale: '' },
				free
			)
		).toEqual({ clearSchedule: true });
	});

	it('reverts clear_schedule to the prior schedule, or null if there was none', () => {
		const had = mk('a', 'x', { scheduledStart: '2026-05-12T09:00' });
		expect(
			computeRevertPatch({ taskId: 'a', kind: 'clear_schedule', rationale: '' }, had)
		).toEqual({ scheduledStart: '2026-05-12T09:00' });

		const free = mk('a', 'x');
		expect(computeRevertPatch({ taskId: 'a', kind: 'clear_schedule', rationale: '' }, free)).toBeNull();
	});

	it('reverts mark_done to the prior done flag', () => {
		const open = mk('a', 'x', { done: false });
		expect(computeRevertPatch({ taskId: 'a', kind: 'mark_done', rationale: '' }, open)).toEqual({
			done: false
		});
	});

	it('reverts archive AND unarchive to the prior done + triage', () => {
		const pre = mk('a', 'x', { done: false, triage: 'inbox' });
		expect(computeRevertPatch({ taskId: 'a', kind: 'archive', rationale: '' }, pre)).toEqual({
			done: false,
			triage: 'inbox'
		});
		const archived = mk('a', 'x', { done: true, triage: 'dropped' });
		expect(computeRevertPatch({ taskId: 'a', kind: 'unarchive', rationale: '' }, archived)).toEqual({
			done: true,
			triage: 'dropped'
		});
	});

	it('reverts snooze / set_project / change_text to prior values', () => {
		const pre = mk('a', 'old text', {
			snoozedUntil: '2026-09-01',
			projectId: 'Granite'
		});
		expect(
			computeRevertPatch(
				{ taskId: 'a', kind: 'snooze', snoozedUntil: '2026-12-01', rationale: '' },
				pre
			)
		).toEqual({ snoozedUntil: '2026-09-01' });
		expect(
			computeRevertPatch(
				{ taskId: 'a', kind: 'set_project', projectId: 'Other', rationale: '' },
				pre
			)
		).toEqual({ projectId: 'Granite' });
		expect(
			computeRevertPatch(
				{ taskId: 'a', kind: 'change_text', text: 'new text', rationale: '' },
				pre
			)
		).toEqual({ text: 'old text' });
	});
});
