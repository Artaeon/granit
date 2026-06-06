// Pure view-time derivations for the habits surface.
//
// Seventh extraction step out of routes/habits/+page.svelte. Holds the
// stateless helpers the template consumes per-habit per-render:
//
//   - bestDay(h): best day-of-week from the 90-day window
//   - last7Done(h): count of done days in the last 7
//   - shortDow(date) / shortDate(date): week-grid header labels
//   - weekDays(h): last 7 days slice (chronological)
//
// All are pure — no $state, no reactivity — so this is a plain .ts
// file, not a .svelte.ts controller. The page imports the named
// functions and calls them inside {@const ...} blocks per row.

import type { HabitInfo } from '$lib/api';

const DOW_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

// Group the 90-day window by weekday and compute per-day completion
// percent. Returns the day-of-week with the highest pct, or null when
// no day has any logged occurrences (a fresh habit). Pure derivation
// — no extra API surface.
export function bestDay(h: HabitInfo): { label: string; pct: number } | null {
  const buckets = [0, 0, 0, 0, 0, 0, 0];
  const counts = [0, 0, 0, 0, 0, 0, 0];
  for (const d of h.days) {
    // YYYY-MM-DD parsed in local time. Using new Date(s) directly
    // would parse as UTC and shift the weekday by one for users east
    // of UTC; explicit local construction avoids that.
    const [y, m, day] = d.date.split('-').map(Number);
    const dow = new Date(y, m - 1, day).getDay();
    counts[dow]++;
    if (d.done) buckets[dow]++;
  }
  let bestDow = -1;
  let bestPct = 0;
  for (let i = 0; i < 7; i++) {
    if (counts[i] === 0) continue;
    const pct = buckets[i] / counts[i];
    if (pct > bestPct) {
      bestPct = pct;
      bestDow = i;
    }
  }
  if (bestDow === -1 || bestPct === 0) return null;
  return { label: DOW_LABELS[bestDow], pct: Math.round(bestPct * 100) };
}

// Count of done days in the last 7. Drives both the per-habit
// completion% calc and the targetState chip.
export function last7Done(h: HabitInfo): number {
  return h.days.slice(-7).filter((d) => d.done).length;
}

// The server returns 90 days oldest -> newest. We want the last 7 in
// chronological order so columns read left=oldest right=today.
export function weekDays(h: HabitInfo): HabitInfo['days'] {
  return h.days.slice(-7);
}

export function shortDow(date: string): string {
  const [y, m, d] = date.split('-').map(Number);
  return DOW_LABELS[new Date(y, m - 1, d).getDay()];
}

export function shortDate(date: string): string {
  const [, m, d] = date.split('-').map(Number);
  return `${m}/${d}`;
}
