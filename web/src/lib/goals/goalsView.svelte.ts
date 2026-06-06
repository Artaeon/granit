// View-time derivations for the goals surface.
//
// Eight derives that the page renders read-only (no mutation), all
// computed from dataCtl + filterCtl + a tiny piece of local state
// (stalledFilterOn). Bundling them here keeps the parent <script>
// focused on event handlers + AI controllers; the derives only
// change when the underlying data does.
//
//   • visibleGoals       — filterCtl.filtered, narrowed to stalled
//                          when the stalled-only chip is on
//   • kanbanGroups       — status-bucketed slices, sorted by
//                          target-date imminence within each column
//   • goalHero           — single most-imminent active/paused goal
//                          for the hero card above the list
//   • targetStats        — distribution across pastTarget /
//                          thisMonth / thisQuarter / later
//   • categories         — distinct category chips, frequency desc
//   • tags               — distinct tag chips, frequency desc
//   • ventures           — distinct venture chips, frequency desc
//   • activeGoalsCount   — reused by the audit panel header
//
// stalledFilterOn lives here because it only affects visibleGoals;
// the parent toggles it through the controller's getter/setter pair.

import type { Goal } from '$lib/api';
import { daysUntilTarget } from './util';
import type { GoalsDataController } from './goalsData.svelte';
import type { GoalsFilterStateController } from './goalsFilterState.svelte';

export type KanbanCol = 'active' | 'paused' | 'completed' | 'archived';
export const KANBAN_COLUMNS: KanbanCol[] = ['active', 'paused', 'completed', 'archived'];

export type TargetStats = {
  pastTarget: number;
  thisMonth: number;
  thisQuarter: number;
  later: number;
};

export interface GoalsViewController {
  stalledFilterOn: boolean;
  readonly visibleGoals: Goal[];
  readonly kanbanGroups: Record<KanbanCol, Goal[]>;
  readonly goalHero: { goal: Goal; days: number } | null;
  readonly targetStats: TargetStats;
  readonly categories: string[];
  readonly tags: string[];
  readonly ventures: string[];
  readonly activeGoalsCount: number;
}

export interface GoalsViewDeps {
  dataCtl: GoalsDataController;
  filterCtl: GoalsFilterStateController;
}

export function createGoalsView(deps: GoalsViewDeps): GoalsViewController {
  const { dataCtl, filterCtl } = deps;

  let stalledFilterOn = $state(false);

  // Re-derive over filterCtl.filtered rather than mutating filters so
  // the existing search/category/tag pickers aren't disturbed when
  // the user clicks the stalled-banner action.
  const visibleGoals = $derived.by(() => {
    if (!stalledFilterOn) return filterCtl.filtered;
    const stalledIds = new Set(dataCtl.stalledGoals.map((g) => g.id));
    return filterCtl.filtered.filter((g) => stalledIds.has(g.id));
  });

  // Kanban grouping — same status order as the tabs so the column
  // order matches the user's mental model. Sort within each column:
  // imminent target_date first, then by title for stability.
  const kanbanGroups = $derived.by<Record<KanbanCol, Goal[]>>(() => {
    const out: Record<KanbanCol, Goal[]> = {
      active: [], paused: [], completed: [], archived: []
    };
    for (const g of filterCtl.filtered) {
      const s = (g.status ?? 'active') as KanbanCol;
      if (out[s]) out[s].push(g);
    }
    const sortKey = (g: Goal): number => {
      const d = daysUntilTarget(g.target_date);
      // Goals with no parseable date sink to the bottom; among the
      // dated, smaller (closer / overdue) days come first.
      return d === null ? Number.POSITIVE_INFINITY : d;
    };
    for (const col of KANBAN_COLUMNS) {
      out[col].sort((a, b) => {
        const sa = sortKey(a), sb = sortKey(b);
        if (sa !== sb) return sa - sb;
        return a.title.localeCompare(b.title);
      });
    }
    return out;
  });

  // Hero — single most-imminent active or paused goal with a parseable
  // target_date. Falls back to null when the user has no dated goals
  // (the hero card simply doesn't render).
  const goalHero = $derived.by<{ goal: Goal; days: number } | null>(() => {
    let best: Goal | null = null;
    let bestDays = Infinity;
    for (const g of dataCtl.goals) {
      const status = g.status ?? 'active';
      if (status !== 'active' && status !== 'paused') continue;
      const days = daysUntilTarget(g.target_date);
      if (days === null) continue;
      // Earliest target wins; overdue goals (negative days) sort
      // ahead of upcoming ones because they need attention more.
      if (days < bestDays) {
        bestDays = days;
        best = g;
      }
    }
    return best ? { goal: best, days: bestDays } : null;
  });

  // Distribution of dated active+paused goals across urgency buckets,
  // surfaced as a one-line summary below the status tabs. Free-text
  // target_dates and undated goals are excluded.
  const targetStats = $derived.by<TargetStats>(() => {
    let pastTarget = 0, thisMonth = 0, thisQuarter = 0, later = 0;
    for (const g of dataCtl.goals) {
      const status = g.status ?? 'active';
      if (status !== 'active' && status !== 'paused') continue;
      const days = daysUntilTarget(g.target_date);
      if (days === null) continue;
      if (days < 0) pastTarget++;
      else if (days <= 30) thisMonth++;
      else if (days <= 90) thisQuarter++;
      else later++;
    }
    return { pastTarget, thisMonth, thisQuarter, later };
  });

  const categories = $derived.by(() => {
    const m = new Map<string, number>();
    for (const g of dataCtl.goals) {
      const c = (g.category ?? '').trim();
      if (!c) continue;
      m.set(c, (m.get(c) ?? 0) + 1);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([c]) => c);
  });

  const tags = $derived.by(() => {
    const m = new Map<string, number>();
    for (const g of dataCtl.goals) {
      for (const t of g.tags ?? []) m.set(t, (m.get(t) ?? 0) + 1);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([t]) => t);
  });

  const ventures = $derived.by(() => {
    const m = new Map<string, number>();
    for (const g of dataCtl.goals) {
      const v = (g.venture ?? '').trim();
      if (!v) continue;
      m.set(v, (m.get(v) ?? 0) + 1);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([v]) => v);
  });

  const activeGoalsCount = $derived(
    dataCtl.goals.filter((g) => (g.status ?? 'active') === 'active').length
  );

  return {
    get stalledFilterOn() {
      return stalledFilterOn;
    },
    set stalledFilterOn(v) {
      stalledFilterOn = v;
    },
    get visibleGoals() {
      return visibleGoals;
    },
    get kanbanGroups() {
      return kanbanGroups;
    },
    get goalHero() {
      return goalHero;
    },
    get targetStats() {
      return targetStats;
    },
    get categories() {
      return categories;
    },
    get tags() {
      return tags;
    },
    get ventures() {
      return ventures;
    },
    get activeGoalsCount() {
      return activeGoalsCount;
    }
  };
}
