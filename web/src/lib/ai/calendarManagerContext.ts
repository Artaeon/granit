// Calendar Manager context builder — sidebar chat counterpart to
// the structured-action Calendar Agent dialog. Where the agent
// dialog proposes typed mutations to accept/reject one by one,
// this surface is for CONVERSATIONAL work: "what's my week
// looking like", "find me two hours of deep work tomorrow",
// "what should I move to make space for the launch prep".
//
// Same shape as projectManagerContext + goalManagerContext:
// pure render + dep-injected loader. The calendar's natural
// scope isn't one entity but a date window, so the bundle
// carries a list of upcoming events plus today/this-week task
// pressure (due, overdue, scheduled). Caps protect the prompt
// budget; totals carry pre-truncation counts so the prelude can
// honestly say "showing 30 of 87".

import type { CalendarEventEntry, Task } from '$lib/api';

/** CalendarContextBundle — the structured shape the formatter
 *  receives. The loader builds this from API calls; tests build
 *  it inline. Lists are pre-truncated by the loader.
 *
 *  windowStart/End are inclusive YYYY-MM-DD strings — the
 *  prelude surfaces them so the AI knows exactly what range
 *  it's reasoning over and can refuse questions about events
 *  outside that scope. */
export interface CalendarContextBundle {
	windowStart: string; // YYYY-MM-DD
	windowEnd: string; // YYYY-MM-DD
	todayISO: string;
	upcomingEvents: CalendarEventEntry[];
	dueToday: Task[];
	overdue: Task[];
	scheduledThisWeek: Task[];
	totals?: {
		upcomingEvents?: number;
		dueToday?: number;
		overdue?: number;
		scheduledThisWeek?: number;
	};
}

/** renderCalendarContext — markdown blob for the system
 *  prelude. Section order: window header (so the AI sees the
 *  scope first) → upcoming events → overdue (most urgent
 *  pressure) → due today → scheduled this week. Empty sections
 *  are omitted entirely so the model isn't tempted to comment
 *  on absent rows. */
export function renderCalendarContext(b: CalendarContextBundle): string {
	const lines: string[] = [];
	lines.push(`# Calendar — ${b.windowStart} → ${b.windowEnd}`);
	lines.push(`Today is ${b.todayISO}.`);

	if (b.upcomingEvents.length > 0) {
		const total = b.totals?.upcomingEvents ?? b.upcomingEvents.length;
		const more = total > b.upcomingEvents.length ? ` (showing ${b.upcomingEvents.length} of ${total})` : '';
		lines.push('', `## Upcoming events${more}`);
		for (const e of b.upcomingEvents) {
			const bits: string[] = [`${e.date}`];
			if (e.start_time) bits.push(`${e.start_time}${e.end_time ? '–' + e.end_time : ''}`);
			else bits.push('all-day');
			if (e.location) bits.push(`@ ${truncate(e.location, 30)}`);
			if (e.project_id) bits.push(`[${e.project_id}]`);
			if (e.rrule) bits.push('↻');
			lines.push(`- **${e.title}** · ${bits.join(' · ')}`);
		}
	}

	if (b.overdue.length > 0) {
		const total = b.totals?.overdue ?? b.overdue.length;
		const more = total > b.overdue.length ? ` (showing ${b.overdue.length} of ${total})` : '';
		lines.push('', `## Overdue${more}`);
		for (const t of b.overdue) {
			const meta: string[] = [];
			if (t.priority) meta.push(`P${t.priority}`);
			if (t.dueDate) meta.push(`was due ${t.dueDate}`);
			const suffix = meta.length > 0 ? ` _(${meta.join(' · ')})_` : '';
			lines.push(`- ${t.text}${suffix}`);
		}
	}

	if (b.dueToday.length > 0) {
		const total = b.totals?.dueToday ?? b.dueToday.length;
		const more = total > b.dueToday.length ? ` (showing ${b.dueToday.length} of ${total})` : '';
		lines.push('', `## Due today${more}`);
		for (const t of b.dueToday) {
			const meta: string[] = [];
			if (t.priority) meta.push(`P${t.priority}`);
			if (t.scheduledStart) meta.push(`scheduled ${t.scheduledStart.slice(11, 16)}`);
			const suffix = meta.length > 0 ? ` _(${meta.join(' · ')})_` : '';
			lines.push(`- ${t.text}${suffix}`);
		}
	}

	if (b.scheduledThisWeek.length > 0) {
		const total = b.totals?.scheduledThisWeek ?? b.scheduledThisWeek.length;
		const more =
			total > b.scheduledThisWeek.length
				? ` (showing ${b.scheduledThisWeek.length} of ${total})`
				: '';
		lines.push('', `## Scheduled this week${more}`);
		for (const t of b.scheduledThisWeek) {
			const day = t.scheduledStart ? t.scheduledStart.slice(0, 10) : '';
			const time = t.scheduledStart ? t.scheduledStart.slice(11, 16) : '';
			lines.push(`- ${day}${time ? ' ' + time : ''} — ${t.text}`);
		}
	}

	return lines.join('\n');
}

/** Caps protect token budget. Picked from rough measurement of
 *  a busy week of events + tasks; tune up after measuring real
 *  prompt sizes in production. */
export const CAL_EVENTS_CAP = 30;
export const CAL_OVERDUE_CAP = 15;
export const CAL_DUE_TODAY_CAP = 15;
export const CAL_SCHEDULED_CAP = 25;

/** loadCalendarContext — fetches a calendar bundle for the
 *  next `daysAhead` days. Tolerant of partial failure: any list
 *  fetch failure yields [] for that slice. */
export async function loadCalendarContext(
	deps: {
		listEvents: () => Promise<CalendarEventEntry[]>;
		listTasks: () => Promise<Task[]>;
	},
	options: { todayISO: string; daysAhead?: number } = { todayISO: '' }
): Promise<CalendarContextBundle> {
	const todayISO = options.todayISO || todayISOFallback();
	const daysAhead = options.daysAhead ?? 14;
	const windowEnd = isoPlusDays(todayISO, daysAhead);

	const [allEvents, allTasks] = await Promise.all([
		deps.listEvents().catch(() => [] as CalendarEventEntry[]),
		deps.listTasks().catch(() => [] as Task[])
	]);

	// Events: in window, sorted chronologically.
	const upcomingAll = allEvents
		.filter((e) => e.date && e.date >= todayISO && e.date <= windowEnd)
		.sort((a, b) => {
			if (a.date !== b.date) return a.date.localeCompare(b.date);
			const sa = a.start_time ?? '99:99';
			const sb = b.start_time ?? '99:99';
			return sa.localeCompare(sb);
		});

	// Open tasks only — done tasks don't shape upcoming work.
	const open = allTasks.filter((t) => !t.done);
	const overdueAll = open
		.filter((t) => t.dueDate && t.dueDate < todayISO)
		.sort((a, b) => (a.dueDate ?? '').localeCompare(b.dueDate ?? ''));
	const dueTodayAll = open.filter((t) => t.dueDate === todayISO);
	const scheduledAll = open
		.filter(
			(t) => t.scheduledStart && t.scheduledStart.slice(0, 10) >= todayISO && t.scheduledStart.slice(0, 10) <= windowEnd
		)
		.sort((a, b) => (a.scheduledStart ?? '').localeCompare(b.scheduledStart ?? ''));

	return {
		windowStart: todayISO,
		windowEnd,
		todayISO,
		upcomingEvents: upcomingAll.slice(0, CAL_EVENTS_CAP),
		overdue: overdueAll.slice(0, CAL_OVERDUE_CAP),
		dueToday: dueTodayAll.slice(0, CAL_DUE_TODAY_CAP),
		scheduledThisWeek: scheduledAll.slice(0, CAL_SCHEDULED_CAP),
		totals: {
			upcomingEvents: upcomingAll.length,
			overdue: overdueAll.length,
			dueToday: dueTodayAll.length,
			scheduledThisWeek: scheduledAll.length
		}
	};
}

function isoPlusDays(iso: string, days: number): string {
	const [y, m, d] = iso.split('-').map(Number);
	const dt = new Date(Date.UTC(y, m - 1, d));
	dt.setUTCDate(dt.getUTCDate() + days);
	const yy = dt.getUTCFullYear();
	const mm = String(dt.getUTCMonth() + 1).padStart(2, '0');
	const dd = String(dt.getUTCDate()).padStart(2, '0');
	return `${yy}-${mm}-${dd}`;
}
function todayISOFallback(): string {
	const d = new Date();
	return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}
function truncate(s: string, n: number): string {
	return s.length <= n ? s : s.slice(0, n - 1).trimEnd() + '…';
}
