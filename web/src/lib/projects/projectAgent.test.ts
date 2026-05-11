import { describe, expect, it } from 'vitest';
import type { Project } from '$lib/api';
import {
	buildProjectAgentPrompt,
	parseProjectAgentResponse,
	validateProjectActions,
	summariseProjectAction,
	computeProjectRevertPatch,
	mergeProjectProposals,
	type ProjectAction,
	type ProjectProposalState
} from './projectAgent';

function mk(name: string, extra: Partial<Project> = {}): Project {
	return {
		name,
		description: '',
		folder: '',
		tags: [],
		status: 'active',
		color: '',
		created_at: '',
		...extra
	} as unknown as Project;
}

describe('buildProjectAgentPrompt', () => {
	const today = '2026-05-11';

	it('embeds the intent and a project digest', () => {
		const { system, user } = buildProjectAgentPrompt(
			[mk('Granite', { status: 'active', priority: 2, venture: 'Artaeon' })],
			'pause low-priority ones',
			today
		);
		expect(system).toMatch(/STRICT JSON/);
		expect(system).toMatch(/set_status/);
		expect(system).toMatch(/archive/);
		expect(user).toContain('pause low-priority ones');
		expect(user).toContain('Today is 2026-05-11');
		expect(user).toContain('name:Granite');
		expect(user).toContain('p2');
		expect(user).toContain('venture:Artaeon');
	});

	it('lists known ventures when supplied', () => {
		const { user } = buildProjectAgentPrompt([mk('A')], 'x', today, ['Artaeon', 'Stoicera']);
		expect(user).toContain('Known ventures: Artaeon, Stoicera');
	});

	it('truncates long descriptions + next_action to keep the prompt slim', () => {
		const long = 'x'.repeat(500);
		const { user } = buildProjectAgentPrompt([mk('A', { description: long, next_action: long })], 'x', today);
		// Description capped at 80, next_action capped at 60.
		expect(user.length).toBeLessThan(2000);
		expect(user).toMatch(/desc:"x{80}"/);
		expect(user).toMatch(/next:"x{60}"/);
	});

	it('is deterministic for identical inputs', () => {
		const a = buildProjectAgentPrompt([mk('A')], 'x', today);
		const b = buildProjectAgentPrompt([mk('A')], 'x', today);
		expect(a).toEqual(b);
	});
});

describe('parseProjectAgentResponse', () => {
	const ok = JSON.stringify({
		actions: [
			{ projectName: 'A', kind: 'archive', rationale: 'dead' },
			{ projectName: 'B', kind: 'set_priority', priority: 1, rationale: 'low leverage' }
		]
	});

	it('parses a clean response', () => {
		const got = parseProjectAgentResponse(ok);
		expect(got).toHaveLength(2);
		expect(got[0].kind).toBe('archive');
		expect(got[1].priority).toBe(1);
	});

	it('strips fences and slices prose', () => {
		expect(parseProjectAgentResponse('```json\n' + ok + '\n```')).toHaveLength(2);
		expect(parseProjectAgentResponse('Here:\n' + ok + '\nDone.')).toHaveLength(2);
	});

	it('drops entries with missing required fields or invalid kind', () => {
		const messy = JSON.stringify({
			actions: [
				{ projectName: 'A', kind: 'archive', rationale: 'real' },
				{ kind: 'archive', rationale: 'no name' },
				{ projectName: 'B', rationale: 'no kind' },
				{ projectName: 'C', kind: 'archive' }, // no rationale
				{ projectName: 'D', kind: 'destroy', rationale: 'bad enum' }
			]
		});
		const got = parseProjectAgentResponse(messy);
		expect(got).toHaveLength(1);
		expect(got[0].projectName).toBe('A');
	});

	it('captures kind-specific args (status / priority / dates / venture / description)', () => {
		const r = parseProjectAgentResponse(
			JSON.stringify({
				actions: [
					{ projectName: 'A', kind: 'set_status', status: 'paused', rationale: 'on hold' },
					{ projectName: 'B', kind: 'set_due_date', due_date: '2026-06-01', rationale: 'sprint' },
					{
						projectName: 'C',
						kind: 'set_next_action',
						next_action: 'reach out to vendor',
						rationale: 'unblock'
					},
					{ projectName: 'D', kind: 'set_venture', venture: 'Artaeon', rationale: 'belongs' },
					{
						projectName: 'E',
						kind: 'change_description',
						description: 'new brief',
						rationale: 'clearer'
					}
				]
			})
		);
		expect(r[0].status).toBe('paused');
		expect(r[1].due_date).toBe('2026-06-01');
		expect(r[2].next_action).toBe('reach out to vendor');
		expect(r[3].venture).toBe('Artaeon');
		expect(r[4].description).toBe('new brief');
	});

	it('drops a set_status whose status is not in the canonical set', () => {
		const r = parseProjectAgentResponse(
			JSON.stringify({
				actions: [
					{ projectName: 'A', kind: 'set_status', status: 'on-hold', rationale: 'bad' }
				]
			})
		);
		expect(r[0].status).toBeUndefined();
	});

	it('returns [] on garbage', () => {
		expect(parseProjectAgentResponse('')).toEqual([]);
		expect(parseProjectAgentResponse('not json')).toEqual([]);
	});
});

describe('validateProjectActions', () => {
	const live = [mk('A'), mk('B'), mk('C')];

	it('drops hallucinated projectName', () => {
		const acts: ProjectAction[] = [
			{ projectName: 'A', kind: 'archive', rationale: 'r' },
			{ projectName: 'ghost', kind: 'archive', rationale: 'fake' }
		];
		expect(validateProjectActions(acts, live).map((a) => a.projectName)).toEqual(['A']);
	});

	it('clamps priority to 1..3 and rounds', () => {
		const out = validateProjectActions(
			[
				{ projectName: 'A', kind: 'set_priority', priority: 9, rationale: 'r' },
				{ projectName: 'B', kind: 'set_priority', priority: 0, rationale: 'r' },
				{ projectName: 'C', kind: 'set_priority', priority: 2.6, rationale: 'r' }
			],
			live
		);
		expect(out.map((a) => a.priority)).toEqual([3, 1, 3]);
	});

	it('drops set_due_date with non YYYY-MM-DD', () => {
		const out = validateProjectActions(
			[
				{ projectName: 'A', kind: 'set_due_date', due_date: 'tomorrow', rationale: 'r' },
				{ projectName: 'B', kind: 'set_due_date', due_date: '2026-06-01', rationale: 'r' }
			],
			live
		);
		expect(out.map((a) => a.projectName)).toEqual(['B']);
	});

	it('drops empty next_action / venture / description rewrites', () => {
		const out = validateProjectActions(
			[
				{ projectName: 'A', kind: 'set_next_action', next_action: '  ', rationale: 'r' },
				{ projectName: 'B', kind: 'set_venture', venture: '', rationale: 'r' },
				{
					projectName: 'C',
					kind: 'change_description',
					description: '\n',
					rationale: 'r'
				}
			],
			live
		);
		expect(out).toEqual([]);
	});

	it('passes through no-arg actions', () => {
		const acts: ProjectAction[] = [
			{ projectName: 'A', kind: 'archive', rationale: 'r' },
			{ projectName: 'B', kind: 'unarchive', rationale: 'r' },
			{ projectName: 'C', kind: 'clear_venture', rationale: 'r' }
		];
		expect(validateProjectActions(acts, live)).toHaveLength(3);
	});

	it('drops set_status with unknown status', () => {
		const out = validateProjectActions(
			[{ projectName: 'A', kind: 'set_status', rationale: 'r' }],
			live
		);
		expect(out).toEqual([]);
	});
});

describe('computeProjectRevertPatch', () => {
	it('reverts set_status to pre-state status', () => {
		expect(
			computeProjectRevertPatch(
				{ projectName: 'A', kind: 'set_status', status: 'paused', rationale: '' },
				mk('A', { status: 'active' })
			)
		).toEqual({ status: 'active' });
	});

	it('reverts archive / unarchive to pre-state status', () => {
		expect(
			computeProjectRevertPatch(
				{ projectName: 'A', kind: 'archive', rationale: '' },
				mk('A', { status: 'active' })
			)
		).toEqual({ status: 'active' });
		expect(
			computeProjectRevertPatch(
				{ projectName: 'A', kind: 'unarchive', rationale: '' },
				mk('A', { status: 'archived' })
			)
		).toEqual({ status: 'archived' });
	});

	it('reverts set/clear pairs to the prior value (empty if none)', () => {
		expect(
			computeProjectRevertPatch(
				{ projectName: 'A', kind: 'set_due_date', due_date: '2026-09-01', rationale: '' },
				mk('A', { due_date: '2026-05-15' })
			)
		).toEqual({ due_date: '2026-05-15' });
		expect(
			computeProjectRevertPatch(
				{ projectName: 'A', kind: 'clear_due_date', rationale: '' },
				mk('A')
			)
		).toEqual({ due_date: '' });
	});

	it('reverts set_priority — defaults to 0 when no prior priority', () => {
		expect(
			computeProjectRevertPatch(
				{ projectName: 'A', kind: 'set_priority', priority: 3, rationale: '' },
				mk('A')
			)
		).toEqual({ priority: 0 });
	});

	it('reverts change_description to the prior text', () => {
		expect(
			computeProjectRevertPatch(
				{ projectName: 'A', kind: 'change_description', description: 'new', rationale: '' },
				mk('A', { description: 'original' })
			)
		).toEqual({ description: 'original' });
	});
});

describe('summariseProjectAction', () => {
	const p = mk('Granite Knowledge Manager');
	it('formats common kinds with the project name', () => {
		expect(
			summariseProjectAction(
				{ projectName: 'Granite Knowledge Manager', kind: 'archive', rationale: '' },
				p
			)
		).toMatch(/Archive/);
		expect(
			summariseProjectAction(
				{ projectName: 'A', kind: 'set_priority', priority: 1, rationale: '' },
				mk('A')
			)
		).toMatch(/P1/);
	});

	it('falls back to projectName when project is undefined', () => {
		expect(
			summariseProjectAction({ projectName: 'ghost', kind: 'archive', rationale: '' }, undefined)
		).toMatch(/Archive/);
	});
});

describe('mergeProjectProposals', () => {
	it('preserves applied state across re-parse', () => {
		const prev: ProjectProposalState[] = [
			{ projectName: 'A', kind: 'archive', rationale: 'r', applied: true }
		];
		const out = mergeProjectProposals(prev, [
			{ projectName: 'A', kind: 'archive', rationale: 'r' }
		]);
		expect(out).toHaveLength(1);
		expect(out[0].applied).toBe(true);
	});

	it('keeps an applied row even when the new parse drops it', () => {
		const prev: ProjectProposalState[] = [
			{ projectName: 'A', kind: 'archive', rationale: 'r', applied: true }
		];
		expect(mergeProjectProposals(prev, [])).toHaveLength(1);
	});
});
