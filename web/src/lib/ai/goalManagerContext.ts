// Goal Manager context builder — pulls every relevant fact about
// a single goal into one structured blob, ready to be dropped into
// the AI chat's system prelude.
//
// What "every relevant fact" means here: goal metadata (title,
// status, target_date, review cadence, venture, category), the
// goal's own description, its nested milestones, plus the open +
// recently-done tasks tagged against this goal. The aim is one
// well-shaped context dump so the AI never asks "which goal, what
// progress, what's already done?" — the prelude answers all three
// before the user types.
//
// Mirrors web/src/lib/ai/projectManagerContext.ts in shape and
// conventions: a pure formatter (renderGoalContext) plus a thin
// HTTP-touching loader (loadGoalContext) that takes
// dependency-injected fetchers so the tests don't need to mock
// fetch. The pure half is what gets unit-tested.
//
// Notes/folder lookup is intentionally NOT wired in: the canonical
// Goal schema (internal/goals.Goal, mirrored at web/src/lib/api.ts)
// has no folder field — `notes` on Goal is free-text sidecar prose,
// not a vault directory. Rather than invent a folder convention
// here we keep the notes slice on the bundle (so callers can pass
// linked notes if a future folder convention emerges) and the
// formatter renders the section when non-empty. The default loader
// returns an empty notes list.

import type { Goal, Milestone, Note, Task } from '$lib/api';

/** GoalContextBundle — the structured shape the formatter receives.
 *  Built by loadGoalContext from real API calls, or assembled inline
 *  in tests. Every list is pre-truncated by the loader — the
 *  formatter trusts caller-supplied bounds and only surfaces the
 *  "showing X of Y" hint when the totals diverge. */
export interface GoalContextBundle {
	goal: Goal;
	openTasks: Task[];
	doneTasks: Task[];
	/** Notes "linked" to the goal. The default loader returns []
	 *  because Goal has no folder field; the slot exists so callers
	 *  with a custom folder convention can populate it. */
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

/** renderGoalContext — turns the bundle into a single markdown
 *  string ready for a system message. Pure function. The order
 *  matters: identity (title + status + target + cadence + venture)
 *  first, then description (the "why"), then milestones (the
 *  shape of progress baked into the goal itself), then linked
 *  tasks (what's open + what's been done), then notes (if any
 *  caller plumbed them in). Most important context earliest so a
 *  truncated read still grasps the goal. */
export function renderGoalContext(b: GoalContextBundle): string {
	const lines: string[] = [];
	const g = b.goal;

	lines.push(`# Goal: ${g.title}`);
	const tags: string[] = [];
	if (g.status) tags.push(`status: ${g.status}`);
	if (g.target_date) tags.push(`target: ${g.target_date}`);
	if (g.review_frequency) tags.push(`review: ${g.review_frequency}`);
	if (g.venture) tags.push(`venture: ${g.venture}`);
	if (g.category) tags.push(`category: ${g.category}`);
	if (tags.length > 0) lines.push(tags.join(' · '));

	if (g.project && g.project.trim()) {
		lines.push(`project: ${g.project.trim()}`);
	}

	if (g.description && g.description.trim()) {
		lines.push('', '## Description', g.description.trim());
	}

	const milestones: Milestone[] = Array.isArray(g.milestones) ? g.milestones : [];
	if (milestones.length > 0) {
		lines.push('', '## Milestones');
		for (const m of milestones) {
			const box = m.done ? '[x]' : '[ ]';
			const bits: string[] = [];
			if (m.due_date) bits.push(`due ${m.due_date}`);
			if (m.done && m.completed_at) bits.push(`done ${m.completed_at}`);
			const suffix = bits.length > 0 ? ` _(${bits.join(' · ')})_` : '';
			lines.push(`- ${box} ${m.text}${suffix}`);
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
			total > b.doneTasks.length
				? ` (showing ${b.doneTasks.length} of ${total})`
				: '';
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

/** GOAL_TASK_CAP / etc — exported so callers (and tests) can reason
 *  about loader bounds without duplicating literals. The caps
 *  protect the token budget; ratchet them up only after measuring
 *  real prompt sizes in production. Slightly tighter than the
 *  project caps because a goal is a narrower scope — twenty open
 *  tasks for one goal is already a lot. */
export const GOAL_TASK_CAP = 20;
export const GOAL_DONE_TASK_CAP = 8;
export const GOAL_NOTE_CAP = 15;

/** API-touching loader. Returns a bundle ready to be passed to
 *  renderGoalContext. Tolerant of partial failures: any list that
 *  fails to load comes back empty so the formatter still produces a
 *  useful (if smaller) blob. The caller is responsible for handling
 *  getGoal failure — without the goal itself there's no context to
 *  build.
 *
 *  listNotesForGoal is optional: Goal has no folder field today so
 *  the default integration omits it; callers with a folder hint can
 *  supply it and the loader will pipe the result through the same
 *  cap + total bookkeeping as tasks. */
export async function loadGoalContext(
	id: string,
	deps: {
		getGoal: (id: string) => Promise<Goal>;
		listTasksForGoal: (id: string, status: 'open' | 'done') => Promise<Task[]>;
		listNotesForGoal?: (goal: Goal) => Promise<Note[]>;
	}
): Promise<GoalContextBundle> {
	const goal = await deps.getGoal(id);

	const [openTasksAll, doneTasksAll, notesAll] = await Promise.all([
		deps.listTasksForGoal(id, 'open').catch(() => [] as Task[]),
		deps.listTasksForGoal(id, 'done').catch(() => [] as Task[]),
		deps.listNotesForGoal
			? deps.listNotesForGoal(goal).catch(() => [] as Note[])
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
		goal,
		openTasks: openTasksAll.slice(0, GOAL_TASK_CAP),
		doneTasks: sortedDone.slice(0, GOAL_DONE_TASK_CAP),
		notes: notesAll.slice(0, GOAL_NOTE_CAP),
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
