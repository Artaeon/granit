import { describe, expect, it } from 'vitest';
import type { Goal } from '$lib/api';
import {
	buildGoalAgentPrompt,
	parseGoalAgentResponse,
	validateGoalActions,
	summariseGoalAction,
	computeGoalRevertPatch,
	type GoalAction
} from './goalAgent';

function mk(id: string, title: string, extra: Partial<Goal> = {}): Goal {
	return {
		id,
		title,
		status: 'active',
		...extra
	} as unknown as Goal;
}

describe('buildGoalAgentPrompt', () => {
	const today = '2026-05-11';

	it('embeds intent + digest', () => {
		const { system, user } = buildGoalAgentPrompt(
			[mk('G1', 'Ship Granit v1', { target_date: '2026-06-01', review_frequency: 'monthly' })],
			'push out targets',
			today
		);
		expect(system).toMatch(/STRICT JSON/);
		expect(system).toMatch(/set_target_date/);
		expect(user).toContain('Today is 2026-05-11');
		expect(user).toContain('push out targets');
		expect(user).toContain('id:G1');
		expect(user).toContain('"Ship Granit v1"');
		expect(user).toContain('target 2026-06-01');
		expect(user).toContain('review:monthly');
	});

	it('lists known ventures', () => {
		const { user } = buildGoalAgentPrompt([mk('G', 'x')], 'i', today, ['Artaeon']);
		expect(user).toContain('Known ventures: Artaeon');
	});
});

describe('parseGoalAgentResponse', () => {
	it('parses clean responses + drops malformed entries', () => {
		const raw = JSON.stringify({
			actions: [
				{ goalId: 'G1', kind: 'archive', rationale: 'dead' },
				{ kind: 'archive', rationale: 'no id' },
				{ goalId: 'G2', rationale: 'no kind' },
				{ goalId: 'G3', kind: 'archive' }, // no rationale
				{ goalId: 'G4', kind: 'destroy', rationale: 'bad enum' }
			]
		});
		const got = parseGoalAgentResponse(raw);
		expect(got).toHaveLength(1);
		expect(got[0].goalId).toBe('G1');
	});

	it('drops set_status with status outside the canonical set', () => {
		const r = parseGoalAgentResponse(
			JSON.stringify({
				actions: [{ goalId: 'G1', kind: 'set_status', status: 'pending', rationale: 'r' }]
			})
		);
		expect(r[0].status).toBeUndefined();
	});

	it('drops set_review_frequency with a non-canonical frequency', () => {
		const r = parseGoalAgentResponse(
			JSON.stringify({
				actions: [
					{ goalId: 'G1', kind: 'set_review_frequency', review_frequency: 'daily', rationale: 'r' }
				]
			})
		);
		expect(r[0].review_frequency).toBeUndefined();
	});

	it('captures per-kind args', () => {
		const r = parseGoalAgentResponse(
			JSON.stringify({
				actions: [
					{ goalId: 'G1', kind: 'set_status', status: 'paused', rationale: 'r' },
					{ goalId: 'G2', kind: 'set_target_date', target_date: '2026-09-01', rationale: 'r' },
					{
						goalId: 'G3',
						kind: 'set_review_frequency',
						review_frequency: 'quarterly',
						rationale: 'r'
					},
					{ goalId: 'G4', kind: 'change_title', title: 'new title', rationale: 'r' }
				]
			})
		);
		expect(r[0].status).toBe('paused');
		expect(r[1].target_date).toBe('2026-09-01');
		expect(r[2].review_frequency).toBe('quarterly');
		expect(r[3].title).toBe('new title');
	});
});

describe('validateGoalActions', () => {
	const live = [mk('G1', 'A'), mk('G2', 'B')];

	it('drops hallucinated goalId', () => {
		const out = validateGoalActions(
			[
				{ goalId: 'G1', kind: 'archive', rationale: 'r' },
				{ goalId: 'ghost', kind: 'archive', rationale: 'r' }
			],
			live
		);
		expect(out.map((a) => a.goalId)).toEqual(['G1']);
	});

	it('drops set_target_date with non YYYY-MM-DD', () => {
		const out = validateGoalActions(
			[
				{ goalId: 'G1', kind: 'set_target_date', target_date: 'tomorrow', rationale: 'r' },
				{ goalId: 'G2', kind: 'set_target_date', target_date: '2026-06-01', rationale: 'r' }
			],
			live
		);
		expect(out.map((a) => a.goalId)).toEqual(['G2']);
	});

	it('drops empty change_title / change_description', () => {
		const out = validateGoalActions(
			[
				{ goalId: 'G1', kind: 'change_title', title: '  ', rationale: 'r' },
				{ goalId: 'G2', kind: 'change_description', description: '', rationale: 'r' }
			],
			live
		);
		expect(out).toEqual([]);
	});
});

describe('computeGoalRevertPatch', () => {
	it('reverts set_status / archive / unarchive to pre-state', () => {
		expect(
			computeGoalRevertPatch(
				{ goalId: 'G', kind: 'archive', rationale: '' },
				mk('G', 'x', { status: 'active' })
			)
		).toEqual({ status: 'active' });
		expect(
			computeGoalRevertPatch(
				{ goalId: 'G', kind: 'unarchive', rationale: '' },
				mk('G', 'x', { status: 'archived' })
			)
		).toEqual({ status: 'archived' });
	});

	it('reverts target/review/venture set+clear pairs', () => {
		expect(
			computeGoalRevertPatch(
				{ goalId: 'G', kind: 'clear_target_date', rationale: '' },
				mk('G', 'x', { target_date: '2026-09-01' })
			)
		).toEqual({ target_date: '2026-09-01' });
		expect(
			computeGoalRevertPatch(
				{ goalId: 'G', kind: 'clear_review_frequency', rationale: '' },
				mk('G', 'x', { review_frequency: 'weekly' })
			)
		).toEqual({ review_frequency: 'weekly' });
		expect(
			computeGoalRevertPatch(
				{ goalId: 'G', kind: 'clear_venture', rationale: '' },
				mk('G', 'x', { venture: 'Artaeon' })
			)
		).toEqual({ venture: 'Artaeon' });
	});

	it('reverts change_title / change_description to prior values', () => {
		expect(
			computeGoalRevertPatch(
				{ goalId: 'G', kind: 'change_title', title: 'new', rationale: '' },
				mk('G', 'old title')
			)
		).toEqual({ title: 'old title' });
		expect(
			computeGoalRevertPatch(
				{ goalId: 'G', kind: 'change_description', description: 'new', rationale: '' },
				mk('G', 't', { description: 'old desc' })
			)
		).toEqual({ description: 'old desc' });
	});
});

describe('summariseGoalAction', () => {
	const g = mk('G1', 'Q3 launch readiness program');
	it('formats per-kind with the goal title', () => {
		expect(summariseGoalAction({ goalId: 'G1', kind: 'archive', rationale: '' }, g)).toMatch(
			/Archive/
		);
		expect(
			summariseGoalAction(
				{ goalId: 'G1', kind: 'set_target_date', target_date: '2026-09-01', rationale: '' },
				g
			)
		).toMatch(/2026-09-01/);
	});

	it('falls back to goalId when goal is undefined', () => {
		expect(summariseGoalAction({ goalId: 'ghost', kind: 'archive', rationale: '' }, undefined)).toMatch(
			/Archive/
		);
	});
});
