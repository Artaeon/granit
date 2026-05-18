// ISO 8601 week-number computation, mirroring Go's time.ISOWeek so
// the frontend and backend agree on which week a given date belongs
// to. Mon..Sun weeks; week 1 is the week containing the first
// Thursday of the year.
//
// JS doesn't ship an ISO-week helper, and Intl.DateTimeFormat with
// `weekOfYear` is patchy in production browsers. Multiple callers
// (/plans/week, /review, /projects sparkline, WeeklyPlanWidget) need
// the same machinery — keeping the formula in one place stops them
// drifting.

/** ISO week parts ({ year, week }) for `at`. The year is the year
 *  of the Thursday of `at`'s week — same convention Go's ISOWeek
 *  uses so a Jan-1-on-Friday case correctly reports the prior
 *  year's W52/W53. */
export function isoWeekParts(at: Date = new Date()): { year: number; week: number } {
  const target = new Date(at.valueOf());
  const dayNr = (at.getDay() + 6) % 7; // Mon=0..Sun=6
  target.setDate(target.getDate() - dayNr + 3);
  const firstThursday = target.valueOf();
  target.setMonth(0, 1);
  if (target.getDay() !== 4) {
    target.setMonth(0, 1 + ((4 - target.getDay()) + 7) % 7);
  }
  const week = 1 + Math.ceil((firstThursday - target.valueOf()) / 604_800_000);
  const thursday = new Date(at.valueOf());
  thursday.setDate(thursday.getDate() - dayNr + 3);
  return { year: thursday.getFullYear(), week };
}

/** ISO week for `at` formatted "YYYY-Www" (e.g. "2026-W19"). */
export function isoWeekString(at: Date = new Date()): string {
  const { year, week } = isoWeekParts(at);
  return `${year}-W${String(week).padStart(2, '0')}`;
}

/** Monday 00:00 (local time) of `at`'s ISO week. Used by sparklines
 *  and "this week vs last week" comparisons where a stable bucket
 *  boundary matters more than the formatted label. */
export function startOfIsoWeek(at: Date): Date {
  const t = new Date(at);
  const day = (t.getDay() + 6) % 7;
  t.setDate(t.getDate() - day);
  t.setHours(0, 0, 0, 0);
  return t;
}

/** Vault-relative path of the plan note for `at`'s ISO week. Single
 *  source of truth so renames are one-edit changes. */
export function planNotePath(at: Date = new Date()): string {
  return `Plans/${isoWeekString(at)}.md`;
}
