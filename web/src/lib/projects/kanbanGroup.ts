// Pure grouping helper for the projects Kanban view. Splitting it
// out keeps ProjectKanban.svelte focused on DOM + drag UX, and lets
// vitest pin the bucketing rules without rendering anything.
//
// Why a fixed list of buckets (not "whatever statuses appear"):
// the Kanban is the place to NUDGE projects between the canonical
// four states. Empty columns are intentional — they're a drop
// target. So we ALWAYS render all four, regardless of what the
// input contains.

import type { Project } from '$lib/api';

/** The canonical project lifecycle. Listed in the order the columns
 *  appear left-to-right on the board — flow direction matters for
 *  the user's mental model (idea → working → paused → wrapped). */
export const KANBAN_STATUSES = ['active', 'paused', 'completed', 'archived'] as const;
export type KanbanStatus = (typeof KANBAN_STATUSES)[number];

export interface KanbanBucket {
	status: KanbanStatus;
	/** Projects whose status matches, in the order they arrived
	 *  (caller controls sort — this function never re-sorts). */
	projects: Project[];
}

/** groupByStatus — splits a project list into the four canonical
 *  status buckets, preserving incoming order within each bucket.
 *  Projects with a missing/empty status are treated as 'active'
 *  (matches the rest of the page). Unknown statuses fall through
 *  into 'archived' so they're still visible somewhere and the user
 *  can re-classify them — silently dropping a project would be
 *  worse than parking it under an obvious column. */
export function groupByStatus(projects: Project[]): KanbanBucket[] {
	const buckets: Record<KanbanStatus, Project[]> = {
		active: [],
		paused: [],
		completed: [],
		archived: []
	};
	for (const p of projects) {
		const raw = (p.status ?? '').trim() || 'active';
		const key: KanbanStatus = (KANBAN_STATUSES as readonly string[]).includes(raw)
			? (raw as KanbanStatus)
			: 'archived';
		buckets[key].push(p);
	}
	return KANBAN_STATUSES.map((status) => ({ status, projects: buckets[status] }));
}

/** Human-readable column label. Centralised so the legend, the
 *  column header, and any aria-label all agree. */
export function statusLabel(s: KanbanStatus): string {
	switch (s) {
		case 'active':
			return 'Active';
		case 'paused':
			return 'Paused';
		case 'completed':
			return 'Completed';
		case 'archived':
			return 'Archived';
	}
}
