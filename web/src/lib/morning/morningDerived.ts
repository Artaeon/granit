// Pure derivation helpers for the morning ritual page.
//
// Eighth extraction step out of routes/morning/+page.svelte. These
// functions don't touch any reactive state — they take the loaded
// data + a today ISO string and return a fresh shape. Sit in a plain
// .ts (not .svelte.ts) so they're trivial to unit-test.
//
// What's covered:
//   - sortMorningTasks    — urgency-bucketed open task ordering
//   - sortMorningIntentions — prayer-intention grouping (tied / persons / general)
//   - pickUpcomingNear    — deadlines within the next 7 days, sorted
//   - morningStats        — at-a-glance number counts for the stat strip
//
// Behaviour-preserving: same bucket numbers, same rank algorithm,
// same 60/8/3 caps, same stats fields. A direct lift of the page's
// inline $derived.by bodies into named pure functions.

import type { Task, PrayerIntention, Deadline, CalendarEvent } from '$lib/api';

/** Urgency-bucketed open task ordering used by the "Pick your tasks"
 *  section. Bucket 0 = overdue+important, 1 = due today, 2 = quick
 *  wins, 9 = everything else. Within a bucket, rank is overdue depth
 *  (0) or priority-then-due-date (else). Capped at 60. */
export function sortMorningTasks(openTasks: Task[], todayISO: string): Task[] {
  type Bucketed = { task: Task; bucket: number; rank: number };
  const isOverdueImportant = (t: Task) =>
    !!t.dueDate && t.dueDate < todayISO && (t.priority === 1 || t.priority === 2);
  const isDueToday = (t: Task) => t.dueDate === todayISO;
  const isQuickWin = (t: Task) =>
    !t.scheduledStart &&
    (!t.dueDate || t.dueDate >= todayISO) &&
    (((t.estimatedMinutes ?? 0) > 0 && (t.estimatedMinutes ?? 0) <= 30) ||
      (t.text.length <= 60 && (t.priority === 0 || t.priority >= 3)));
  const bucketed: Bucketed[] = openTasks.map((t) => {
    let bucket = 9;
    if (isOverdueImportant(t)) bucket = 0;
    else if (isDueToday(t)) bucket = 1;
    else if (isQuickWin(t)) bucket = 2;
    const rank =
      bucket === 0
        ? -(todayISO.localeCompare(t.dueDate ?? '~') || 0)
        : (t.priority || 99) * 100 +
          (t.dueDate ? Number(t.dueDate.replace(/-/g, '').slice(2)) : 999_999);
    return { task: t, bucket, rank };
  });
  return bucketed
    .sort((a, b) => (a.bucket !== b.bucket ? a.bucket - b.bucket : a.rank - b.rank))
    .slice(0, 60)
    .map((x) => x.task);
}

/** Prayer intentions grouped by tie: venture / project / goal first,
 *  then person-only, then the rest. Within each group the original
 *  order is preserved (filter is stable). */
export function sortMorningIntentions(
  activeIntentions: PrayerIntention[]
): PrayerIntention[] {
  const tied = activeIntentions.filter((p) => p.venture || p.project || p.goal);
  const persons = activeIntentions.filter(
    (p) => p.person && !(p.venture || p.project || p.goal)
  );
  const general = activeIntentions.filter(
    (p) => !p.person && !(p.venture || p.project || p.goal)
  );
  return [...tied, ...persons, ...general];
}

/** Calendar-day delta from today to the given YYYY-MM-DD. Negative
 *  for past dates. Exposed for callers that want the same rounding
 *  logic the morning page uses. */
export function daysUntil(iso: string): number {
  const [y, m, d] = iso.split('-').map(Number);
  const due = new Date(y, m - 1, d);
  const t = new Date();
  t.setHours(0, 0, 0, 0);
  return Math.round((due.getTime() - t.getTime()) / 86_400_000);
}

/** Deadlines within 7 calendar days, oldest first, capped at 3.
 *  Cancelled + met statuses are dropped. */
export function pickUpcomingNear(
  upcomingDeadlines: Deadline[] | null
): { d: Deadline; days: number }[] {
  if (!upcomingDeadlines) return [];
  return upcomingDeadlines
    .filter((d) => d.status !== 'cancelled' && d.status !== 'met')
    .map((d) => ({ d, days: daysUntil(d.date) }))
    .filter((x) => x.days <= 7)
    .sort((a, b) => a.days - b.days)
    .slice(0, 3);
}

export interface MorningStats {
  openTasks: number;
  overdue: number;
  dueToday: number;
  events: number;
  deadlinesThisWeek: number;
}

/** At-a-glance counts for the morning stat strip. Reads against the
 *  loaded data arrays the rest of the page renders, so the numbers
 *  stay consistent with the lists below. */
export function morningStats(args: {
  openTasks: Task[];
  todayEvents: CalendarEvent[];
  upcomingNear: { d: Deadline; days: number }[];
  todayISO: string;
}): MorningStats {
  let overdue = 0;
  let dueToday = 0;
  for (const t of args.openTasks) {
    if (!t.dueDate) continue;
    if (t.dueDate < args.todayISO) overdue++;
    else if (t.dueDate === args.todayISO) dueToday++;
  }
  return {
    openTasks: args.openTasks.length,
    overdue,
    dueToday,
    events: args.todayEvents.length,
    deadlinesThisWeek: args.upcomingNear.length
  };
}
