// Daily-note helpers — pure date math + body-marker parsing used
// by the notes route page. Moved out so the page's reactive
// derivations stay one-liners and these utilities become
// individually testable.
//
// A note is "daily" when its basename is YYYY-MM-DD.md OR its
// frontmatter has type=daily (with a date field). The page
// uses parseDailyDate to detect that; downstream UI keys
// off the result (prev/next-day jumps, daily-context panel,
// day-activity feed).

import type { Note } from '$lib/api';

/** Matches a daily-note basename: e.g. `2026-06-02.md` or
 *  `journal/2026-06-02.md`. Captures the date string. */
export const DAILY_DATE_RE = /(\d{4}-\d{2}-\d{2})\.md$/;

/** The marker the daily-note template seeds under "## Day overview".
 *  Stays in the underlying markdown so external editors round-trip
 *  it unchanged; the renderer substitutes a live aggregated feed at
 *  preview time. */
export const DAY_ACTIVITY_MARKER = '<!-- granit:day-activity -->';

/** Today's date in local timezone as YYYY-MM-DD. We don't use
 *  toISOString() because it converts to UTC and would flip the
 *  date at midnight in any non-UTC zone. */
export function todayLocalISO(): string {
  const t = new Date();
  return `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`;
}

/** Returns YYYY-MM-DD shifted by `days`. Works with negative shifts
 *  and handles month/year rollovers via Date arithmetic. */
export function shiftDate(iso: string, days: number): string {
  const [y, m, d] = iso.split('-').map(Number);
  const dt = new Date(y, m - 1, d);
  dt.setDate(dt.getDate() + days);
  const yy = dt.getFullYear();
  const mm = String(dt.getMonth() + 1).padStart(2, '0');
  const dd = String(dt.getDate()).padStart(2, '0');
  return `${yy}-${mm}-${dd}`;
}

/** Detect a daily note. Path-pattern wins; frontmatter type=daily
 *  is the fallback for notes that live at non-daily paths but were
 *  authored from the daily template. Returns null when neither
 *  applies. */
export function parseDailyDate(note: Note | null): string | null {
  if (!note) return null;
  const m = note.path.match(DAILY_DATE_RE);
  if (m) return m[1];
  const fm = note.frontmatter as Record<string, unknown> | undefined;
  if (fm && fm.type === 'daily' && typeof fm.date === 'string') {
    return fm.date.slice(0, 10);
  }
  return null;
}

/** Split a body around the day-activity marker. Returns null when
 *  the marker is absent so callers can short-circuit. */
export function splitDayActivity(src: string): { before: string; after: string } | null {
  const idx = src.indexOf(DAY_ACTIVITY_MARKER);
  if (idx < 0) return null;
  return {
    before: src.slice(0, idx),
    after: src.slice(idx + DAY_ACTIVITY_MARKER.length)
  };
}

/** Human-friendly relative label for a daily date — "today",
 *  "yesterday", "tomorrow", or the weekday name. Caller passes
 *  `todayIso` so the helper stays pure (no clock read inside). */
export function formatRelativeDailyLabel(dailyDate: string, todayIso: string): string {
  if (dailyDate === todayIso) return 'today';
  if (dailyDate === shiftDate(todayIso, -1)) return 'yesterday';
  if (dailyDate === shiftDate(todayIso, 1)) return 'tomorrow';
  const [y, m, d] = dailyDate.split('-').map(Number);
  const dt = new Date(y, m - 1, d);
  return dt.toLocaleDateString(undefined, { weekday: 'long' });
}
