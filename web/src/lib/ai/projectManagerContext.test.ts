import { describe, expect, it } from 'vitest';
import type { Goal, Note, Project, Task } from '$lib/api';
import {
	renderProjectContext,
	loadProjectContext,
	PROJECT_TASK_CAP,
	PROJECT_DONE_TASK_CAP,
	PROJECT_NOTE_CAP
} from './projectManagerContext';

function mkProject(over: Partial<Project> = {}): Project {
	return {
		name: 'Granite',
		description: '',
		folder: 'Projects/Granite',
		tags: [],
		status: 'active',
		color: '',
		created_at: '',
		...over
	} as Project;
}
function mkTask(text: string, over: Partial<Task> = {}): Task {
	return {
		id: 't_' + text,
		notePath: 'Projects/Granite/tasks.md',
		lineNum: 1,
		text,
		done: false,
		priority: 0,
		...over
	} as Task;
}
function mkGoal(title: string, over: Partial<Goal> = {}): Goal {
	return { id: 'G_' + title, title, ...over } as Goal;
}
function mkNote(path: string, title: string): Note {
	return { path, title, modTime: '', size: 0 } as Note;
}

describe('renderProjectContext', () => {
	it('puts identity (name + tags) first', () => {
		const out = renderProjectContext({
			project: mkProject({ name: 'Granite', status: 'active', kind: 'software', venture: 'Artaeon' }),
			goals: [],
			openTasks: [],
			doneTasks: [],
			notes: []
		});
		const nameAt = out.indexOf('# Project: Granite');
		expect(nameAt).toBe(0);
		expect(out).toContain('status: active');
		expect(out).toContain('kind: software');
		expect(out).toContain('venture: Artaeon');
	});

	it('orders sections: identity → description → next-action → goals → open → done → notes', () => {
		const out = renderProjectContext({
			project: mkProject({
				description: 'A knowledge manager',
				next_action: 'ship v1'
			}),
			goals: [mkGoal('Ship Granit v1')],
			openTasks: [mkTask('write the readme')],
			doneTasks: [mkTask('design schema', { done: true })],
			notes: [mkNote('Projects/Granite/notes.md', 'Granite notes')]
		});
		const a = out.indexOf('## Description');
		const b = out.indexOf('## Stated next action');
		const c = out.indexOf('## Linked goals');
		const d = out.indexOf('## Open tasks');
		const e = out.indexOf('## Recently done');
		const f = out.indexOf('## Linked notes');
		expect(a).toBeLessThan(b);
		expect(b).toBeLessThan(c);
		expect(c).toBeLessThan(d);
		expect(d).toBeLessThan(e);
		expect(e).toBeLessThan(f);
	});

	it('omits empty sections cleanly', () => {
		const out = renderProjectContext({
			project: mkProject({ name: 'X' }),
			goals: [],
			openTasks: [],
			doneTasks: [],
			notes: []
		});
		expect(out).not.toContain('## Description');
		expect(out).not.toContain('## Linked goals');
		expect(out).not.toContain('## Open tasks');
		expect(out).not.toContain('## Recently done');
		expect(out).not.toContain('## Linked notes');
	});

	it('shows truncation note when totals exceed list length', () => {
		const out = renderProjectContext({
			project: mkProject(),
			goals: [],
			openTasks: Array.from({ length: 20 }, (_, i) => mkTask(`task ${i}`)),
			doneTasks: [],
			notes: [],
			totals: { openTasks: 87, doneTasks: 0, notes: 0 }
		});
		expect(out).toContain('showing 20 of 87');
	});

	it('does NOT show truncation note when totals match the list', () => {
		const out = renderProjectContext({
			project: mkProject(),
			goals: [],
			openTasks: [mkTask('only one')],
			doneTasks: [],
			notes: [],
			totals: { openTasks: 1 }
		});
		expect(out).not.toContain('showing');
	});

	it('marks non-active goal statuses + target dates inline', () => {
		const out = renderProjectContext({
			project: mkProject(),
			goals: [
				mkGoal('Paused thing', { status: 'paused' }),
				mkGoal('On the books', { target_date: '2026-09-01' })
			],
			openTasks: [],
			doneTasks: [],
			notes: []
		});
		expect(out).toContain('Paused thing');
		expect(out).toContain('paused');
		expect(out).toContain('by 2026-09-01');
	});

	it('shows task priority + due alongside the task text', () => {
		const out = renderProjectContext({
			project: mkProject(),
			goals: [],
			openTasks: [mkTask('write report', { priority: 3, dueDate: '2026-06-01' })],
			doneTasks: [],
			notes: []
		});
		expect(out).toMatch(/write report.*P3.*due 2026-06-01/);
	});
});

describe('loadProjectContext', () => {
	it('caps lists per the published limits and reports the total', async () => {
		const tasks = Array.from({ length: 50 }, (_, i) =>
			mkTask(`open-${i}`, { id: `o${i}` })
		);
		const done = Array.from({ length: 30 }, (_, i) =>
			mkTask(`done-${i}`, { id: `d${i}`, done: true, completedAt: `2026-05-${String(i + 1).padStart(2, '0')}` })
		);
		const notes = Array.from({ length: 40 }, (_, i) =>
			mkNote(`Projects/Granite/${i}.md`, `Note ${i}`)
		);
		const bundle = await loadProjectContext('Granite', {
			getProject: async () => mkProject(),
			listTasksForProject: async (_, s) => (s === 'open' ? tasks : done),
			listGoalsForProject: async () => [],
			listNotesInFolder: async () => notes
		});
		expect(bundle.openTasks.length).toBe(PROJECT_TASK_CAP);
		expect(bundle.doneTasks.length).toBe(PROJECT_DONE_TASK_CAP);
		expect(bundle.notes.length).toBe(PROJECT_NOTE_CAP);
		expect(bundle.totals?.openTasks).toBe(50);
		expect(bundle.totals?.doneTasks).toBe(30);
		expect(bundle.totals?.notes).toBe(40);
	});

	it('sorts done tasks newest-first by completedAt', async () => {
		const bundle = await loadProjectContext('Granite', {
			getProject: async () => mkProject(),
			listTasksForProject: async (_, s) =>
				s === 'open'
					? []
					: [
							mkTask('oldest', { done: true, completedAt: '2026-01-01' }),
							mkTask('newest', { done: true, completedAt: '2026-05-01' }),
							mkTask('mid', { done: true, completedAt: '2026-03-01' })
						],
			listGoalsForProject: async () => [],
			listNotesInFolder: async () => []
		});
		expect(bundle.doneTasks.map((t) => t.text)).toEqual(['newest', 'mid', 'oldest']);
	});

	it('survives partial failures (goal listing fails) — returns empty for that slice', async () => {
		const bundle = await loadProjectContext('Granite', {
			getProject: async () => mkProject(),
			listTasksForProject: async () => [],
			listGoalsForProject: async () => {
				throw new Error('boom');
			},
			listNotesInFolder: async () => []
		});
		expect(bundle.goals).toEqual([]);
		expect(bundle.project.name).toBe('Granite');
	});

	it('skips note lookup when the project has no folder', async () => {
		let calls = 0;
		await loadProjectContext('Granite', {
			getProject: async () => mkProject({ folder: '' }),
			listTasksForProject: async () => [],
			listGoalsForProject: async () => [],
			listNotesInFolder: async () => {
				calls++;
				return [];
			}
		});
		expect(calls).toBe(0);
	});
});
