// Project Manager context builder — pulls every relevant fact
// about a single project into one structured blob, ready to be
// dropped into the AI chat's system prelude.
//
// What "every relevant fact" means here: project metadata,
// linked goals, open + recently-done tasks, notes living under
// the project's folder. The goal is one well-shaped context
// dump so the AI never asks "what project, what's the goal,
// what's already done?" — the prelude answers all three before
// the user types.
//
// The HTTP-touching half (loadProjectContext) lives at the bottom
// and is intentionally thin. The bulk of this file is the pure
// formatter (renderProjectContext) that turns the bundle into a
// markdown blob — easy to unit-test, easy to reason about token
// cost.

import type { Goal, Note, Project, Task } from '$lib/api';

/** ProjectContextBundle — the structured shape the formatter
 *  receives. Built by loadProjectContext from real API calls,
 *  or assembled inline in tests. Every list is pre-truncated by
 *  the loader — the formatter trusts caller-supplied bounds. */
export interface ProjectContextBundle {
	project: Project;
	goals: Goal[];
	openTasks: Task[];
	doneTasks: Task[];
	notes: Note[];
	/** Total counts before truncation — used in the formatter to
	 *  surface "showing first 20 of 87 open tasks" when the loader
	 *  capped the list. */
	totals?: {
		openTasks?: number;
		doneTasks?: number;
		notes?: number;
	};
}

/** renderProjectContext — turns the bundle into a single markdown
 *  string ready for a system message. Pure function. The order
 *  matters: identity (name + venture) first, then goals (what
 *  we're trying to achieve), then tasks (what's open), then
 *  notes (what we've written about this). Most important context
 *  earliest so a truncated read still grasps the project. */
export function renderProjectContext(b: ProjectContextBundle): string {
	const lines: string[] = [];
	const p = b.project;

	lines.push(`# Project: ${p.name}`);
	const tags: string[] = [];
	if (p.status) tags.push(`status: ${p.status}`);
	if (p.kind) tags.push(`kind: ${p.kind}`);
	if (p.venture) tags.push(`venture: ${p.venture}`);
	if (p.priority) tags.push(`priority: P${p.priority}`);
	if (p.due_date) tags.push(`due: ${p.due_date}`);
	if (tags.length > 0) lines.push(tags.join(' · '));

	if (p.description && p.description.trim()) {
		lines.push('', '## Description', p.description.trim());
	}
	if (p.next_action && p.next_action.trim()) {
		lines.push('', `## Stated next action`, p.next_action.trim());
	}

	if (b.goals.length > 0) {
		lines.push('', '## Linked goals');
		for (const g of b.goals) {
			const bits: string[] = [];
			if (g.status && g.status !== 'active') bits.push(g.status);
			if (g.target_date) bits.push(`by ${g.target_date}`);
			const suffix = bits.length > 0 ? ` (${bits.join(' · ')})` : '';
			lines.push(`- **${g.title}**${suffix}`);
			if (g.description && g.description.trim()) {
				lines.push(`  ${truncate(g.description.trim().replace(/\n/g, ' '), 140)}`);
			}
		}
	}

	if (b.openTasks.length > 0) {
		const total = b.totals?.openTasks ?? b.openTasks.length;
		const moreNote =
			total > b.openTasks.length
				? ` (showing ${b.openTasks.length} of ${total})`
				: '';
		lines.push('', `## Open tasks${moreNote}`);
		for (const t of b.openTasks) {
			const meta: string[] = [];
			if (t.priority) meta.push(`P${t.priority}`);
			if (t.dueDate) meta.push(`due ${t.dueDate}`);
			if (t.scheduledStart) meta.push(`scheduled ${t.scheduledStart.slice(0, 10)}`);
			const suffix = meta.length > 0 ? ` _(${meta.join(' · ')})_` : '';
			lines.push(`- ${t.text}${suffix}`);
		}
	}

	if (b.doneTasks.length > 0) {
		const total = b.totals?.doneTasks ?? b.doneTasks.length;
		const moreNote =
			total > b.doneTasks.length ? ` (showing ${b.doneTasks.length} of ${total})` : '';
		lines.push('', `## Recently done${moreNote}`);
		for (const t of b.doneTasks) {
			lines.push(`- ${t.text}`);
		}
	}

	if (b.notes.length > 0) {
		const total = b.totals?.notes ?? b.notes.length;
		const moreNote =
			total > b.notes.length ? ` (showing ${b.notes.length} of ${total})` : '';
		lines.push('', `## Linked notes${moreNote}`);
		for (const n of b.notes) {
			lines.push(`- \`${n.path}\` — ${truncate(n.title || n.path, 60)}`);
		}
	}

	return lines.join('\n');
}

/** PROJECT_TASK_CAP / etc — exported so callers (and tests) can
 *  reason about loader bounds without duplicating literals. The
 *  caps protect the token budget; ratchet them up only after
 *  measuring real prompt sizes in production. */
export const PROJECT_TASK_CAP = 20;
export const PROJECT_DONE_TASK_CAP = 8;
export const PROJECT_NOTE_CAP = 15;

/** API-touching loader. Returns a bundle ready to be passed to
 *  renderProjectContext. Tolerant of partial failures: any list
 *  that fails to load comes back empty so the formatter still
 *  produces a useful (if smaller) blob. The caller is
 *  responsible for handling getProject failure — without the
 *  project itself there's no context to build. */
export async function loadProjectContext(
	name: string,
	deps: {
		getProject: (n: string) => Promise<Project>;
		listTasksForProject: (n: string, status: 'open' | 'done') => Promise<Task[]>;
		listGoalsForProject: (n: string) => Promise<Goal[]>;
		listNotesInFolder: (folder: string) => Promise<Note[]>;
	}
): Promise<ProjectContextBundle> {
	const project = await deps.getProject(name);

	const [openTasksAll, doneTasksAll, goalsAll, notesAll] = await Promise.all([
		deps.listTasksForProject(name, 'open').catch(() => [] as Task[]),
		deps.listTasksForProject(name, 'done').catch(() => [] as Task[]),
		deps.listGoalsForProject(name).catch(() => [] as Goal[]),
		project.folder
			? deps.listNotesInFolder(project.folder).catch(() => [] as Note[])
			: Promise.resolve([] as Note[])
	]);

	// Most-recent-first for done tasks — completedAt usually present
	// on done tasks; fallback to updatedAt so a malformed task
	// doesn't sink to the back. Sort is stable so original order
	// breaks ties.
	const sortedDone = [...doneTasksAll].sort((a, b) => {
		const ka = a.completedAt ?? a.updatedAt ?? '';
		const kb = b.completedAt ?? b.updatedAt ?? '';
		return kb.localeCompare(ka);
	});

	return {
		project,
		goals: goalsAll,
		openTasks: openTasksAll.slice(0, PROJECT_TASK_CAP),
		doneTasks: sortedDone.slice(0, PROJECT_DONE_TASK_CAP),
		notes: notesAll.slice(0, PROJECT_NOTE_CAP),
		totals: {
			openTasks: openTasksAll.length,
			doneTasks: doneTasksAll.length,
			notes: notesAll.length
		}
	};
}

function truncate(s: string, n: number): string {
	return s.length <= n ? s : s.slice(0, n - 1).trimEnd() + '…';
}
