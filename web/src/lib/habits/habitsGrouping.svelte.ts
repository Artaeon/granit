// Group-by-category state for the habits surface.
//
// Sixth view mode of the habits page (the existing four are
// today/week/list/heatmap, this slice also adds "category"). Pure
// state controller — emits a stable category → habits[] map plus
// the ordered category-key list so the view component can iterate
// in a deterministic order.
//
// Ordering: known categories first, alphabetically; "Uncategorized"
// always last so the user sees their grouped habits before the
// "needs sorting" bucket. Habits inside a group retain the order
// the caller hands in (so the route's sort comparator still
// drives intra-group ordering — this controller does not re-sort).

import type { HabitInfo } from '$lib/api';

/** Stable key used for the "no category" bucket in the grouping
 *  map. Picked so it cannot collide with a real category name and
 *  matches the sentinel in habitsCategoryFilter. */
export const UNCATEGORIZED_KEY = '__uncategorized__';

export type GroupedHabits = {
  /** Ordered category keys, "uncategorised" sentinel last. */
  order: string[];
  /** Category key → list of habits. Empty buckets are absent so
   *  the view never renders an empty section. */
  byCategory: Map<string, HabitInfo[]>;
};

export interface GroupingController {
  /** Display label for `key`. Returns "Uncategorized" for the
   *  sentinel and the raw key otherwise; the view component reads
   *  this so it doesn't need to know about the sentinel. */
  labelOf(key: string): string;
  /** Whether `key` is the uncategorised bucket. */
  isUncategorized(key: string): boolean;
  /** The grouped view. Reactive — recomputes when habits change. */
  readonly grouped: GroupedHabits;
}

export type GroupingDeps = {
  /** Already-filtered + sorted habit list. The controller stays
   *  agnostic of which sort/filter pipeline produced it. */
  getHabits: () => HabitInfo[];
};

export function createGroupingCtl(deps: GroupingDeps): GroupingController {
  let grouped = $derived.by<GroupedHabits>(() => {
    const byCategory = new Map<string, HabitInfo[]>();
    for (const h of deps.getHabits()) {
      const key = h.category ?? UNCATEGORIZED_KEY;
      const bucket = byCategory.get(key);
      if (bucket) bucket.push(h);
      else byCategory.set(key, [h]);
    }
    const order: string[] = [];
    for (const k of byCategory.keys()) {
      if (k !== UNCATEGORIZED_KEY) order.push(k);
    }
    order.sort((a, b) => a.localeCompare(b));
    if (byCategory.has(UNCATEGORIZED_KEY)) order.push(UNCATEGORIZED_KEY);
    return { order, byCategory };
  });

  function labelOf(key: string): string {
    return key === UNCATEGORIZED_KEY ? 'Uncategorized' : key;
  }

  function isUncategorized(key: string): boolean {
    return key === UNCATEGORIZED_KEY;
  }

  return {
    get grouped() { return grouped; },
    labelOf,
    isUncategorized
  };
}
