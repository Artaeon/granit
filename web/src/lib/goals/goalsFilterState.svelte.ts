// View + filter state for the goals surface.
//
// First extraction step out of routes/goals/+page.svelte (1486 LOC).
// Combines the view-mode selector (cards/list/kanban) with the five
// filter dimensions (statusFilter, categoryFilter, tagFilter,
// ventureFilter, free-text q) because they all narrow the same goals
// list — and combining them avoids a third controller for what is a
// small surface compared to /tasks.
//
// Owns the `filtered` derivation that every view reads from, and the
// `counts` summary the status-chip strip displays. External state the
// controller can't own (the loaded goals list) is reached via the
// deps bundle.

import type { Goal } from '$lib/api';
import { loadStoredString, saveStoredString } from '$lib/util/storage';

const VIEW_KEY = 'granit.goals.view';

export type GoalsViewMode = 'cards' | 'list' | 'kanban';
export type GoalsStatusFilter = 'all' | 'active' | 'paused' | 'completed' | 'archived';

export interface GoalsCounts {
  all: number;
  active: number;
  paused: number;
  completed: number;
  archived: number;
}

export interface GoalsFilterStateDeps {
  /** Loaded goals list — filtered + counted. */
  getGoals: () => Goal[];
}

export interface GoalsFilterStateController {
  // Bindable state.
  viewMode: GoalsViewMode;
  statusFilter: GoalsStatusFilter;
  categoryFilter: string;
  tagFilter: string;
  ventureFilter: string;
  q: string;

  // Derived (readonly).
  /** Goals matching every active filter, in original order. */
  readonly filtered: Goal[];
  /** Per-status counts used by the chip strip badges. */
  readonly counts: GoalsCounts;
}

export function createGoalsFilterState(
  deps: GoalsFilterStateDeps
): GoalsFilterStateController {
  let viewMode = $state<GoalsViewMode>(
    loadStoredString(VIEW_KEY, 'cards') as GoalsViewMode
  );
  let statusFilter = $state<GoalsStatusFilter>('all');
  let categoryFilter = $state<string>('');
  let tagFilter = $state<string>('');
  let ventureFilter = $state<string>('');
  let q = $state<string>('');

  $effect(() => saveStoredString(VIEW_KEY, viewMode));

  let filtered = $derived.by<Goal[]>(() => {
    let list = deps.getGoals();
    if (statusFilter !== 'all')
      list = list.filter((g) => (g.status ?? 'active') === statusFilter);
    if (categoryFilter)
      list = list.filter((g) => g.category === categoryFilter);
    if (tagFilter) list = list.filter((g) => (g.tags ?? []).includes(tagFilter));
    if (ventureFilter)
      list = list.filter((g) => (g.venture ?? '') === ventureFilter);
    const term = q.trim().toLowerCase();
    if (term) {
      list = list.filter(
        (g) =>
          g.title.toLowerCase().includes(term) ||
          (g.description ?? '').toLowerCase().includes(term) ||
          (g.notes ?? '').toLowerCase().includes(term) ||
          (g.venture ?? '').toLowerCase().includes(term)
      );
    }
    return list;
  });

  let counts = $derived.by<GoalsCounts>(() => {
    const goals = deps.getGoals();
    return {
      all: goals.length,
      active: goals.filter((g) => (g.status ?? 'active') === 'active').length,
      paused: goals.filter((g) => g.status === 'paused').length,
      completed: goals.filter((g) => g.status === 'completed').length,
      archived: goals.filter((g) => g.status === 'archived').length
    };
  });

  return {
    get viewMode() {
      return viewMode;
    },
    set viewMode(v) {
      viewMode = v;
    },
    get statusFilter() {
      return statusFilter;
    },
    set statusFilter(v) {
      statusFilter = v;
    },
    get categoryFilter() {
      return categoryFilter;
    },
    set categoryFilter(v) {
      categoryFilter = v;
    },
    get tagFilter() {
      return tagFilter;
    },
    set tagFilter(v) {
      tagFilter = v;
    },
    get ventureFilter() {
      return ventureFilter;
    },
    set ventureFilter(v) {
      ventureFilter = v;
    },
    get q() {
      return q;
    },
    set q(v) {
      q = v;
    },
    get filtered() {
      return filtered;
    },
    get counts() {
      return counts;
    }
  };
}
