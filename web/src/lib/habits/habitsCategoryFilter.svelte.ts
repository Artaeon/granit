// Category-filter state for the habits surface.
//
// Companion to habitsCategoryEdit: that one writes a habit's
// category, this one narrows the visible list by category. The
// filter is multi-select with inclusion semantics — selecting
// "Health" + "Mind" shows habits in either category. An explicit
// "uncategorized" pseudo-bucket selects habits with no category at
// all so the user can find + tag them.
//
// Empty selection = no filter (show all). Same convention as the
// tasks/tags multi-select chip cluster.
//
// The factory exposes a predicate the route can apply alongside its
// existing sort + tag filter, plus the derived list of known
// categories (drawn from the loaded habits so the chip row stays in
// sync with reality — no separate API call needed for the chips).

import type { HabitInfo } from '$lib/api';

/** Sentinel value used in the selected-set to represent "habits
 *  with no category". Picked so it cannot collide with a real
 *  category name (the server normalises away leading spaces). */
export const UNCATEGORIZED = '__uncategorized__';

export interface CategoryFilterController {
  /** Selected category keys. Empty = no filter. Includes the
   *  UNCATEGORIZED sentinel when the user wants uncategorised
   *  habits in the result. */
  selected: Set<string>;
  /** Categories present in the loaded habit list, deduped +
   *  alphabetically sorted. Drives the chip row. */
  readonly knownCategories: string[];
  /** Whether any uncategorised habit exists — controls visibility
   *  of the "Uncategorized" chip. */
  readonly hasUncategorized: boolean;
  /** True iff at least one category is selected. The header chip
   *  "All" reads from this to know whether to render as the
   *  active reset. */
  readonly isActive: boolean;

  /** Toggle membership of `key` (either a category name or the
   *  UNCATEGORIZED sentinel). */
  toggle(key: string): void;
  /** Clear the selection — equivalent to clicking the "All" chip. */
  clear(): void;
  /** Predicate the caller threads through its sort pipeline. */
  matches(h: HabitInfo): boolean;
}

export type CategoryFilterDeps = {
  /** Loaded habit list. Read each time the derivations recompute so
   *  the chip row reflects the current vault. */
  getHabits: () => HabitInfo[];
};

export function createCategoryFilterCtl(
  deps: CategoryFilterDeps
): CategoryFilterController {
  let selected = $state<Set<string>>(new Set());

  let knownCategories = $derived.by<string[]>(() => {
    const s = new Set<string>();
    for (const h of deps.getHabits()) {
      if (h.category) s.add(h.category);
    }
    return [...s].sort((a, b) => a.localeCompare(b));
  });

  let hasUncategorized = $derived(deps.getHabits().some((h) => !h.category));

  let isActive = $derived(selected.size > 0);

  function toggle(key: string) {
    // Reassign so Svelte's reactivity picks up the change — mutating
    // a Set in place doesn't trigger $derived recomputes.
    const next = new Set(selected);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    selected = next;
  }

  function clear() {
    if (selected.size === 0) return;
    selected = new Set();
  }

  function matches(h: HabitInfo): boolean {
    if (selected.size === 0) return true;
    if (!h.category) return selected.has(UNCATEGORIZED);
    return selected.has(h.category);
  }

  return {
    get selected() { return selected; },
    set selected(v) { selected = v; },
    get knownCategories() { return knownCategories; },
    get hasUncategorized() { return hasUncategorized; },
    get isActive() { return isActive; },
    toggle,
    clear,
    matches
  };
}
