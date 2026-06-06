// Pure helpers extracted from routes/deadlines/+page.svelte. First
// extraction step of the deadlines god-file teardown. These are
// stateless functions the page uses across both rendering paths and
// quick-actions — pulling them out of the script block frees ~50 LOC
// from the page body and (more importantly) makes them reusable by
// the pane shell, the dashboard widget, and any future deadline
// surface without copying.
//
// daysUntil already lives in ./util — we don't duplicate it here.

import type { Deadline } from '$lib/api';
import { daysUntil } from './util';

/** Today as YYYY-MM-DD in the user's local timezone — matches
 *  daysUntil's local-midnight semantics so "today" labels line up. */
export function todayISO(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

/** Loose YYYY-MM-DD shape check. Doesn't verify calendar validity —
 *  the browser date input prevents most malformed values upstream. */
export function isValidDate(s: string): boolean {
  return /^\d{4}-\d{2}-\d{2}$/.test(s);
}

/** Add N days to an ISO YYYY-MM-DD string and return the same format.
 *  Used by snooze quick-actions. Local-time arithmetic (matches
 *  daysUntil's local-midnight semantics). */
export function addDaysISO(iso: string, n: number): string {
  const [y, m, d] = iso.split('-').map(Number);
  const dt = new Date(y, m - 1, d);
  dt.setDate(dt.getDate() + n);
  return `${dt.getFullYear()}-${String(dt.getMonth() + 1).padStart(2, '0')}-${String(dt.getDate()).padStart(2, '0')}`;
}

/** Human countdown label for a deadline row. "today" / "tomorrow" /
 *  "in 5d" / "3d ago" — honors met/cancelled status so the chip stays
 *  meaningful after a row gets resolved. */
export function countdown(d: Deadline): string {
  if (d.status === 'met') return 'met';
  if (d.status === 'cancelled') return 'cancelled';
  const n = daysUntil(d.date);
  if (n === 0) return 'today';
  if (n === 1) return 'tomorrow';
  if (n === -1) return 'yesterday';
  if (n > 1) return `in ${n}d`;
  return `${-n}d ago`;
}

// Cross-link helpers — the deadline's project / goal_id / venture
// chips on the row become real links to the corresponding
// entity-detail page so the user can pivot from a deadline to its
// parent context in one click.
export function projectHref(name: string): string {
  return `/projects?p=${encodeURIComponent(name)}`;
}
export function goalHref(id: string): string {
  return `/goals?focus=${encodeURIComponent(id)}`;
}
export function ventureHref(name: string): string {
  return `/ventures?v=${encodeURIComponent(name)}`;
}
