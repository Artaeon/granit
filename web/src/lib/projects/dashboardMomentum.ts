// Pure helper: compute a 4-week tasks-completed-per-week bar series
// from the done-tasks slice of a ProjectContextBundle. Lives in its
// own file so the dashboard panel can stay markup-only and the
// bucketing logic can be tested without spinning up Svelte.
//
// ISO-week-Monday buckets keyed off the user's local clock —
// matches the bucketing the ProjectDetail burn-up and the projects
// list sparkline already use, so a "W19" cell on one surface lines
// up with the same week on another. The output is in chronological
// order (oldest week first, current week last) so the dashboard can
// render bars left-to-right "past → now" without re-reversing.
//
// `now` is injectable for tests; defaults to new Date(). Anything
// with a numeric completedAt that doesn't parse is silently
// skipped — a malformed task shouldn't sink the whole chart.
import type { Task } from '$lib/api';

export const MOMENTUM_WEEKS = 4;

export interface MomentumBar {
	/** Short label for the X-axis — week number for past weeks, 'Now' for the current week. */
	label: string;
	/** Tasks completed in this ISO-week bucket. */
	count: number;
	/** True for the week containing `now`, used to colour the bar. */
	isThisWeek: boolean;
}

function isoWeekKey(d: Date): string {
	const t = new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()));
	const day = (t.getUTCDay() + 6) % 7;
	t.setUTCDate(t.getUTCDate() - day + 3);
	const firstThu = new Date(Date.UTC(t.getUTCFullYear(), 0, 4));
	const week = 1 + Math.round((t.getTime() - firstThu.getTime()) / (7 * 24 * 60 * 60 * 1000));
	return `${t.getUTCFullYear()}-W${String(week).padStart(2, '0')}`;
}

function startOfIsoWeek(d: Date): Date {
	const t = new Date(d);
	const day = (t.getDay() + 6) % 7;
	t.setDate(t.getDate() - day);
	t.setHours(0, 0, 0, 0);
	return t;
}

/** computeMomentum — turns a list of done tasks into a fixed-length
 *  array of MOMENTUM_WEEKS bars, oldest first. The label for past
 *  weeks is the two-digit ISO week number ('W19' → '19'); the most
 *  recent week is labelled 'Now' so the user instantly spots where
 *  the cursor is. */
export function computeMomentum(doneTasks: Task[], now: Date = new Date()): MomentumBar[] {
	const weekStart = startOfIsoWeek(now);
	const thisKey = isoWeekKey(now);
	const order: string[] = [];
	const labels = new Map<string, string>();
	for (let i = MOMENTUM_WEEKS - 1; i >= 0; i--) {
		const d = new Date(weekStart);
		d.setDate(d.getDate() - i * 7);
		const k = isoWeekKey(d);
		order.push(k);
		labels.set(k, k === thisKey ? 'Now' : k.split('W')[1]);
	}
	const counts = new Map<string, number>();
	for (const t of doneTasks) {
		if (!t.completedAt) continue;
		const d = new Date(t.completedAt);
		if (Number.isNaN(d.getTime())) continue;
		const k = isoWeekKey(d);
		if (!order.includes(k)) continue;
		counts.set(k, (counts.get(k) ?? 0) + 1);
	}
	return order.map((k) => ({
		label: labels.get(k) ?? k,
		count: counts.get(k) ?? 0,
		isThisWeek: k === thisKey
	}));
}
