// View + sort state for the habits surface.
//
// First extraction step out of routes/habits/+page.svelte. Owns the
// two persisted preferences — view mode (today / week / list / heatmap)
// and sort key (streak / completion / behind / alpha) — and the
// view-time sorted-habits derivation that consumes the sort key.
//
// Persisted per-device via localStorage so a user who lives in Week
// view doesn't have to toggle every time. List remains the default for
// first-time users — the 90-day grid is the most informative entry
// point.
//
// The page reads view + sortBy via getters and binds the <select> to
// the controller; the sorted list is a $derived so toggling sort
// repaints without a reload.

import type { HabitInfo, HabitsResponse } from '$lib/api';
import { loadStoredString, saveStoredString } from '$lib/util/storage';

// 'category' is the grouped-by-category lens added alongside the
// original four. It re-uses the list-view card via a passed snippet;
// the controller doesn't render anything itself.
export type HabitsView = 'today' | 'week' | 'list' | 'heatmap' | 'category';
export type HabitsSort = 'streak' | 'completion' | 'alpha' | 'behind' | 'reminder';

const VIEW_KEY = 'granit.habits.view';
const SORT_KEY = 'granit.habits.sort';

export interface HabitsViewStateDeps {
  /** Reactive getter for the loaded habits payload — the sorted
   *  derivation reads from here so a reload picks up new habits
   *  without re-binding. */
  getData: () => HabitsResponse | null;
}

export interface HabitsViewStateController {
  view: HabitsView;
  sortBy: HabitsSort;
  /** Sort applied to data.habits. Returns [] when data is null so the
   *  page can `{#each sortedHabits}` unconditionally. */
  readonly sortedHabits: HabitInfo[];
}

export function createHabitsViewState(deps: HabitsViewStateDeps): HabitsViewStateController {
  let view = $state<HabitsView>(loadStoredString(VIEW_KEY, 'list') as HabitsView);
  let sortBy = $state<HabitsSort>(loadStoredString(SORT_KEY, 'streak') as HabitsSort);

  $effect(() => saveStoredString(VIEW_KEY, view));
  $effect(() => saveStoredString(SORT_KEY, sortBy));

  // Each sort key maps to a comparator. "behind" surfaces struggling
  // habits at the top so a Sunday review naturally shows what needs
  // attention without scrolling.
  let sortedHabits = $derived.by<HabitInfo[]>(() => {
    const data = deps.getData();
    if (!data) return [];
    const list = [...data.habits];
    switch (sortBy) {
      case 'streak':
        return list.sort((a, b) => b.currentStreak - a.currentStreak);
      case 'completion':
        return list.sort((a, b) => b.last30Pct - a.last30Pct);
      case 'behind':
        return list.sort((a, b) => a.last7Pct - b.last7Pct);
      case 'alpha':
        return list.sort((a, b) => a.name.localeCompare(b.name));
      case 'reminder':
        // HH:MM ascending; habits without a reminder go last so a
        // morning scan reads top-down by wake time.
        return list.sort((a, b) => {
          const ar = a.reminderTime ?? '';
          const br = b.reminderTime ?? '';
          if (!ar && !br) return a.name.localeCompare(b.name);
          if (!ar) return 1;
          if (!br) return -1;
          return ar.localeCompare(br);
        });
    }
  });

  return {
    get view() { return view; },
    set view(v) { view = v; },
    get sortBy() { return sortBy; },
    set sortBy(v) { sortBy = v; },
    get sortedHabits() { return sortedHabits; }
  };
}
