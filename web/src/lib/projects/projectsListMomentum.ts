// Per-project momentum derivations for the projects LIST page.
//
// Third extraction step out of routes/projects/+page.svelte. Pure
// stateless functions — no $state, no Svelte. The page wraps them
// in $derived so reactivity stays at the call site, but the math
// itself is plain JS / TS so it round-trips through unit tests.
//
// The list page surfaces a tiny 4-week sparkline + a "this week"
// count on every card so the user can spot a stalled project at a
// glance, without having to open the per-project detail panel. Same
// ISO-week bucketing the detail panel's burn-up uses, so the card
// and the panel agree exactly.
//
// SPARK_WEEKS controls both the look-back window and the bucket
// count — 4 keeps the sparkline narrow enough to fit in the card
// header alongside the progress bar and avoids the "sparkline that
// scrolls" failure mode.

import { type Project, type Task } from '$lib/api';
import { isoWeekString, startOfIsoWeek } from '$lib/util/isoWeek';
import { fmtDateISO as ymd } from '$lib/util/date';

export const SPARK_WEEKS = 4;

export interface ProjectMomentum {
  /** Completion counts, oldest to newest, length === SPARK_WEEKS. */
  spark: number[];
  /** Open tasks scheduled inside the current ISO week. */
  scheduledThisWeek: number;
}

/**
 * The ISO-week-keys for the sparkline, oldest first, length
 * SPARK_WEEKS. Pre-compute once per render so each card doesn't
 * redo the work.
 */
export function buildSparkWeekOrder(now: Date = new Date()): string[] {
  const start = startOfIsoWeek(now);
  const order: string[] = [];
  for (let i = SPARK_WEEKS - 1; i >= 0; i--) {
    const d = new Date(start);
    d.setDate(d.getDate() - i * 7);
    order.push(isoWeekString(d));
  }
  return order;
}

/**
 * Compute completion sparkline + scheduled-this-week count for every
 * project. Project membership matches the ProjectDetail logic so the
 * per-card view and the per-project view agree: explicit projectId
 * wins, otherwise notePath under the project's folder counts.
 *
 * Returns a map keyed by project name. Projects with no matched tasks
 * still appear in the map with zeroed buckets so the caller can call
 * `get(p.name)` without a null check.
 */
export function computeMomentumByProject(
  projects: Project[],
  tasks: Task[],
  sparkWeekOrder: string[],
  now: Date = new Date()
): Map<string, ProjectMomentum> {
  const out = new Map<string, ProjectMomentum>();
  const today = ymd(now);
  const monStart = ymd(startOfIsoWeek(now));
  for (const p of projects) {
    out.set(p.name, { spark: new Array(SPARK_WEEKS).fill(0), scheduledThisWeek: 0 });
  }
  for (const t of tasks) {
    // Project membership: explicit projectId OR notePath under a
    // project's folder. Mirrors the matching ProjectDetail uses
    // so the sparkline + the panel's burn-up agree exactly.
    const matched: Project[] = [];
    for (const p of projects) {
      if (t.projectId === p.name) {
        matched.push(p);
        continue;
      }
      const folder = (p.folder ?? '').replace(/\/$/, '');
      if (folder && t.notePath.startsWith(folder + '/')) matched.push(p);
    }
    if (matched.length === 0) continue;
    // Completion → bump the matching week bucket.
    if (t.done && t.completedAt) {
      const k = isoWeekString(new Date(t.completedAt));
      const idx = sparkWeekOrder.indexOf(k);
      if (idx >= 0) {
        for (const p of matched) {
          const m = out.get(p.name);
          if (m) m.spark[idx]++;
        }
      }
    }
    // Scheduled in current week → bump the count.
    if (!t.done && t.scheduledStart) {
      const day = t.scheduledStart.slice(0, 10);
      if (day >= monStart && day <= today) {
        for (const p of matched) {
          const m = out.get(p.name);
          if (m) m.scheduledThisWeek++;
        }
      }
    }
  }
  return out;
}
