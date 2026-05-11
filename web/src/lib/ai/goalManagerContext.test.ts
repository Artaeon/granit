import { describe, expect, it } from 'vitest';
import type { Goal, Milestone, Note, Task } from '$lib/api';
import {
	renderGoalContext,
	loadGoalContext,
	GOAL_TASK_CAP,
	GOAL_DONE_TASK_CAP,
	GOAL_NOTE_CAP
} from './goalManagerContext';

function mkGoal(over: Partial<Goal> = {}): Goal {
	return {
		id: 'G_test',
		title: 'Ship Granit v1',
		status: 'active',
		...over
	} as Goal;
}
function mkTask(text: string, over: Partial<Task> = {}): Task {
	return {
		id: 't_' + text,
		notePath: 'Goals/ship.md',
		lineNum: 1,
		text,
		done: false,
		priority: 0,
		goalId: 'G_test',
		...over
	} as Task;
}
function mkMilestone(text: string, over: Partial<Milestone> = {}): Milestone {
	return { text, done: false, ...over } as Milestone;
}
function mkNote(path: string, title: string): Note {
	return { path, title, modTime: '', size: 0 } as Note;
}

describe('renderGoalContext', () => {
	it('puts identity (title + tags) first', () => {
		const out = renderGoalContext({
			goal: mkGoal({
				title: 'Ship Granit v1',
				status: 'active',
				target_date: '2026-09-01',
				review_frequency: 'weekly',
				venture: 'Artaeon',
				category: 'product'
			}),
			openTasks: [],
			doneTasks: [],
			notes: []
		});
		const titleAt = out.indexOf('# Goal: Ship Granit v1');
		expect(titleAt).toBe(0);
		expect(out).toContain('status: active');
		expect(out).toContain('target: 2026-09-01');
		expect(out).toContain('review: weekly');
		expect(out).toContain('venture: Artaeon');
		expect(out).toContain('category: product');
	});

	it('orders sections: identity → description → milestones → open → done → notes', () => {
		const out = renderGoalContext({
			goal: mkGoal({
				description: 'A v1 release of the knowledge manager.',
				milestones: [mkMilestone('design schema', { done: true }), mkMilestone('write docs')]
			}),
			openTasks: [mkTask('write the readme')],
			doneTasks: [mkTask('sketch the homepage', { done: true })],
			notes: [mkNote('Goals/ship.md', 'Ship notes')]
		});
		const desc = out.indexOf('## Description');
		const mil = out.indexOf('## Milestones');
		const open = out.indexOf('## Open tasks');
		const done = out.indexOf('## Recently done');
		const notes = out.indexOf('## Linked notes');
		expect(desc).toBeGreaterThan(-1);
		expect(desc).toBeLessThan(mil);
		expect(mil).toBeLessThan(open);
		expect(open).toBeLessThan(done);
		expect(done).toBeLessThan(notes);
	});

	it('renders the linked project as its own line when goal.project is set', () => {
		const out = renderGoalContext({
			goal: mkGoal({ project: 'Granite' }),
			openTasks: [],
			doneTasks: [],
			notes: []
		});
		expect(out).toContain('project: Granite');
	});

	it('omits empty sections cleanly (no description, no milestones, no tasks, no notes)', () => {
		const out = renderGoalContext({
			goal: mkGoal(),
			openTasks: [],
			doneTasks: [],
			notes: []
		});
		expect(out).not.toContain('## Description');
		expect(out).not.toContain('## Milestones');
		expect(out).not.toContain('## Open tasks');
		expect(out).not.toContain('## Recently done');
		expect(out).not.toContain('## Linked notes');
		expect(out).not.toContain('project:');
	});

	it('renders milestones with done / pending checkboxes and inline due dates', () => {
		const out = renderGoalContext({
			goal: mkGoal({
				milestones: [
					mkMilestone('design schema', { done: true, completed_at: '2026-02-15' }),
					mkMilestone('write docs', { due_date: '2026-06-01' })
				]
			}),
			openTasks: [],
			doneTasks: [],
			notes: []
		});
		expect(out).toMatch(/- \[x\] design schema.*done 2026-02-15/);
		expect(out).toMatch(/- \[ \] write docs.*due 2026-06-01/);
	});

	it('shows truncation note when totals exceed list length for open + done tasks', () => {
		const out = renderGoalContext({
			goal: mkGoal(),
			openTasks: Array.from({ length: 20 }, (_, i) => mkTask(`task ${i}`)),
			doneTasks: Array.from({ length: 8 }, (_, i) => mkTask(`done ${i}`, { done: true })),
			notes: [],
			totals: { openTasks: 24, doneTasks: 30 }
		});
		expect(out).toContain('Open tasks (showing 20 of 24)');
		expect(out).toContain('Recently done (showing 8 of 30)');
	});

	it('shows task priority + due alongside the task text', () => {
		const out = renderGoalContext({
			goal: mkGoal(),
			openTasks: [mkTask('write report', { priority: 3, dueDate: '2026-06-01' })],
			doneTasks: [],
			notes: []
		});
		expect(out).toMatch(/write report.*P3.*due 2026-06-01/);
	});

	it('handles null milestones (goal.milestones === null) without crashing', () => {
		const out = renderGoalContext({
			goal: mkGoal({ milestones: null }),
			openTasks: [],
			doneTasks: [],
			notes: []
		});
		expect(out).not.toContain('## Milestones');
	});
});

describe('loadGoalContext', () => {
	it('caps lists per the published limits and reports the totals', async () => {
		const open = Array.from({ length: 50 }, (_, i) =>
			mkTask(`open-${i}`, { id: `o${i}` })
		);
		const done = Array.from({ length: 30 }, (_, i) =>
			mkTask(`done-${i}`, {
				id: `d${i}`,
				done: true,
				completedAt: `2026-05-${String(i + 1).padStart(2, '0')}`
			})
		);
		const notes = Array.from({ length: 40 }, (_, i) =>
			mkNote(`Goals/note-${i}.md`, `Note ${i}`)
		);
		const bundle = await loadGoalContext('G_test', {
			getGoal: async () => mkGoal(),
			listTasksForGoal: async (_, s) => (s === 'open' ? open : done),
			listNotesForGoal: async () => notes
		});
		expect(bundle.openTasks.length).toBe(GOAL_TASK_CAP);
		expect(bundle.doneTasks.length).toBe(GOAL_DONE_TASK_CAP);
		expect(bundle.notes.length).toBe(GOAL_NOTE_CAP);
		expect(bundle.totals?.openTasks).toBe(50);
		expect(bundle.totals?.doneTasks).toBe(30);
		expect(bundle.totals?.notes).toBe(40);
	});

	it('sorts done tasks newest-first by completedAt', async () => {
		const bundle = await loadGoalContext('G_test', {
			getGoal: async () => mkGoal(),
			listTasksForGoal: async (_, s) =>
				s === 'open'
					? []
					: [
							mkTask('oldest', { done: true, completedAt: '2026-01-01' }),
							mkTask('newest', { done: true, completedAt: '2026-05-01' }),
							mkTask('mid', { done: true, completedAt: '2026-03-01' })
						]
		});
		expect(bundle.doneTasks.map((t) => t.text)).toEqual(['newest', 'mid', 'oldest']);
	});

	it('survives partial failures — open task listing throws, others still load', async () => {
		const bundle = await loadGoalContext('G_test', {
			getGoal: async () => mkGoal({ title: 'Survivor' }),
			listTasksForGoal: async (_, s) => {
				if (s === 'open') throw new Error('boom');
				return [mkTask('done one', { done: true })];
			}
		});
		expect(bundle.openTasks).toEqual([]);
		expect(bundle.doneTasks.length).toBe(1);
		expect(bundle.goal.title).toBe('Survivor');
	});

	it('returns empty notes when no listNotesForGoal dep is provided', async () => {
		const bundle = await loadGoalContext('G_test', {
			getGoal: async () => mkGoal(),
			listTasksForGoal: async () => []
		});
		expect(bundle.notes).toEqual([]);
		expect(bundle.totals?.notes).toBe(0);
	});

	it('survives a failing notes loader without poisoning the rest of the bundle', async () => {
		const bundle = await loadGoalContext('G_test', {
			getGoal: async () => mkGoal(),
			listTasksForGoal: async () => [mkTask('still here')],
			listNotesForGoal: async () => {
				throw new Error('notes blew up');
			}
		});
		expect(bundle.notes).toEqual([]);
		expect(bundle.openTasks.length).toBe(1);
	});

	it('renders a complete bundle through the formatter end-to-end', async () => {
		const bundle = await loadGoalContext('G_test', {
			getGoal: async () =>
				mkGoal({
					description: 'A clear and measurable goal.',
					target_date: '2026-09-01',
					milestones: [mkMilestone('half done', { done: true })],
					project: 'Granite'
				}),
			listTasksForGoal: async (_, s) =>
				s === 'open' ? [mkTask('write the readme')] : []
		});
		const out = renderGoalContext(bundle);
		expect(out).toContain('# Goal: Ship Granit v1');
		expect(out).toContain('project: Granite');
		expect(out).toContain('## Description');
		expect(out).toContain('## Milestones');
		expect(out).toContain('write the readme');
	});
});
