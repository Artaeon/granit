// Shared deadline-utility helpers. Lifted out of the per-page
// implementations so the dashboard widget, the /deadlines list, the
// morning routine, and the note-page strip all agree on the same
// arithmetic + sorting rules.

import type { Deadline, DeadlineImportance } from '$lib/api';

/** Days from today (local time, midnight) to an ISO YYYY-MM-DD date. Negative = past. */
export function daysUntil(iso: string): number {
  const [y, m, d] = iso.split('-').map(Number);
  if (!y || !m || !d) return 0;
  const target = new Date(y, m - 1, d);
  const t = new Date();
  t.setHours(0, 0, 0, 0);
  return Math.round((target.getTime() - t.getTime()) / 86_400_000);
}

const importanceRank: Record<DeadlineImportance, number> = {
  critical: 0,
  high: 1,
  normal: 2
};

/**
 * Pick the single most-urgent active deadline for the big countdown card.
 * Sort key: importance (critical → high → normal), then earliest date.
 * Returns null when nothing active is upcoming.
 */
export function pickHeroDeadline(list: Deadline[] | null | undefined): Deadline | null {
  if (!list) return null;
  const active = list.filter((d) => d.status !== 'met' && d.status !== 'cancelled');
  if (active.length === 0) return null;
  const sorted = [...active].sort((a, b) => {
    const ra = importanceRank[a.importance] ?? 2;
    const rb = importanceRank[b.importance] ?? 2;
    if (ra !== rb) return ra - rb;
    return a.date.localeCompare(b.date);
  });
  return sorted[0] ?? null;
}

/** "27 days until …" — for the hero countdown card. */
export function bigCountdownLabel(days: number): string {
  if (days < 0) return `${Math.abs(days)} ${Math.abs(days) === 1 ? 'day' : 'days'} overdue`;
  if (days === 0) return 'Today';
  if (days === 1) return 'Tomorrow';
  return `${days} days until`;
}
