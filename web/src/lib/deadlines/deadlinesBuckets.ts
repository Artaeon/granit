// Bucket / group-by helpers for the deadlines list view. Second
// extraction step out of routes/deadlines/+page.svelte. All three
// group-by modes (urgency / status / month) live here so the page
// stops carrying ~120 LOC of taxonomy + label + tone tables.
//
// Pure functions over Deadline rows + a GroupBy mode. Stateless so
// the same helpers drive the page list, the dashboard widget, and
// any future surface that needs the same buckets.

import type { Deadline } from '$lib/api';
import { daysUntil } from './util';

export type GroupBy = 'urgency' | 'status' | 'month';
export type Bucket = string;

export const urgencyOrder: Bucket[] = [
  'overdue',
  'this_week',
  'this_month',
  'later',
  'met',
  'cancelled'
];
export const urgencyLabel: Record<string, string> = {
  overdue: 'Overdue',
  this_week: 'This week',
  this_month: 'This month',
  later: 'Later',
  met: 'Met',
  cancelled: 'Cancelled'
};
export const statusOrder: Bucket[] = ['active', 'missed', 'met', 'cancelled'];
export const statusLabel: Record<string, string> = {
  active: 'Active',
  missed: 'Missed',
  met: 'Met',
  cancelled: 'Cancelled'
};

export function urgencyBucket(d: Deadline): Bucket {
  if (d.status === 'met') return 'met';
  if (d.status === 'cancelled') return 'cancelled';
  const days = daysUntil(d.date);
  if (days < 0) return 'overdue';
  if (days <= 7) return 'this_week';
  if (days <= 31) return 'this_month';
  return 'later';
}

export function statusBucket(d: Deadline): Bucket {
  return d.status ?? 'active';
}

/** 'YYYY-MM' bucket key — sorted lexically gives chronological order. */
export function monthBucket(d: Deadline): Bucket {
  return d.date.slice(0, 7);
}

/** Localised month-name label for a 'YYYY-MM' bucket key. */
export function monthLabel(key: string): string {
  const [y, m] = key.split('-').map(Number);
  if (!y || !m) return key;
  const dt = new Date(y, m - 1, 1);
  return dt.toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
}

/** Display title for a bucket given the active group-by mode. */
export function bucketTitle(b: Bucket, groupBy: GroupBy): string {
  if (groupBy === 'urgency') return urgencyLabel[b] ?? b;
  if (groupBy === 'status') return statusLabel[b] ?? b;
  return monthLabel(b);
}

/** Bucket header tint — drives the section heading colour so the eye
 *  lands on Overdue / This week first under urgency, on Active under
 *  status, and on the urgency of the bucket's first row under month. */
export function bucketTone(b: Bucket, groupBy: GroupBy): string {
  if (groupBy === 'urgency') {
    switch (b) {
      case 'overdue':
        return 'error';
      case 'this_week':
        return 'warning';
      case 'this_month':
        return 'info';
      case 'met':
        return 'success';
      default:
        return 'dim';
    }
  }
  if (groupBy === 'status') {
    switch (b) {
      case 'active':
        return 'info';
      case 'missed':
        return 'error';
      case 'met':
        return 'success';
      default:
        return 'dim';
    }
  }
  // month — tint by how close the bucket is to today.
  const [y, m] = b.split('-').map(Number);
  if (!y || !m) return 'dim';
  const now = new Date();
  const monthsAhead = (y - now.getFullYear()) * 12 + (m - 1 - now.getMonth());
  if (monthsAhead < 0) return 'dim';
  if (monthsAhead === 0) return 'warning';
  if (monthsAhead <= 1) return 'info';
  return 'secondary';
}

/** Build the grouped-list map. Each bucket is a Map of [key, rows]
 *  preserving insertion order so the section render order matches the
 *  intended display order (urgency: most-urgent first, status: open
 *  first, month: chronological). */
export function buildGrouped(
  rows: Deadline[],
  groupBy: GroupBy
): Map<Bucket, Deadline[]> {
  const out = new Map<Bucket, Deadline[]>();
  if (groupBy === 'urgency') {
    for (const k of urgencyOrder) out.set(k, []);
    for (const d of rows) out.get(urgencyBucket(d))!.push(d);
  } else if (groupBy === 'status') {
    for (const k of statusOrder) out.set(k, []);
    for (const d of rows) {
      const b = statusBucket(d);
      if (!out.has(b)) out.set(b, []);
      out.get(b)!.push(d);
    }
  } else {
    // month — keys are the YYYY-MM in chronological order. We
    // collect first then sort, which is cheap (deadlines.json is
    // small enough that O(n log n) once isn't worth optimising).
    const tmp = new Map<string, Deadline[]>();
    for (const d of rows) {
      const k = monthBucket(d);
      if (!tmp.has(k)) tmp.set(k, []);
      tmp.get(k)!.push(d);
    }
    const keys = Array.from(tmp.keys()).sort();
    for (const k of keys) out.set(k, tmp.get(k)!);
  }
  return out;
}
