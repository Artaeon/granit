// Tag-filter state for the habits surface.
//
// Sibling of habitsCategoryFilter, same factory shape. Tag matching
// uses OR semantics: a habit with tags [focus, am] matches when
// either "focus" or "am" is selected — pragmatic default for a
// glance-and-narrow chip row, matching the tasks-page tag chip
// behaviour.
//
// Empty selection = no filter. There is no "untagged" sentinel
// here (most habits will start untagged; offering a chip per-habit
// adder is the actionable surface, not a filter for it).

import type { HabitInfo } from '$lib/api';

export interface TagFilterController {
  /** Selected tag set. Empty = no filter. */
  selected: Set<string>;
  /** Tags present in the loaded habit list, deduped + sorted. */
  readonly knownTags: string[];
  readonly isActive: boolean;

  toggle(tag: string): void;
  clear(): void;
  matches(h: HabitInfo): boolean;
}

export type TagFilterDeps = {
  getHabits: () => HabitInfo[];
};

export function createTagFilterCtl(deps: TagFilterDeps): TagFilterController {
  let selected = $state<Set<string>>(new Set());

  let knownTags = $derived.by<string[]>(() => {
    const s = new Set<string>();
    for (const h of deps.getHabits()) {
      for (const t of h.tags ?? []) s.add(t);
    }
    return [...s].sort((a, b) => a.localeCompare(b));
  });

  let isActive = $derived(selected.size > 0);

  function toggle(tag: string) {
    const next = new Set(selected);
    if (next.has(tag)) next.delete(tag);
    else next.add(tag);
    selected = next;
  }

  function clear() {
    if (selected.size === 0) return;
    selected = new Set();
  }

  function matches(h: HabitInfo): boolean {
    if (selected.size === 0) return true;
    const ts = h.tags;
    if (!ts || ts.length === 0) return false;
    for (const t of ts) {
      if (selected.has(t)) return true;
    }
    return false;
  }

  return {
    get selected() { return selected; },
    set selected(v) { selected = v; },
    get knownTags() { return knownTags; },
    get isActive() { return isActive; },
    toggle,
    clear,
    matches
  };
}
