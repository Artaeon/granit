// Filter state for the deadlines surface.
//
// Fifth extraction step out of routes/deadlines/+page.svelte. Owns
// the two user-driven filter dimensions (importance chip, free-text
// search) plus the URL-driven scope filters (project / goal_id /
// venture), and exposes the two derivations everything downstream
// reads from:
//
//   scoped     — rows narrowed by the URL scope params only
//   filtered   — scoped + importance + free-text search
//
// Plus the importanceCounts chip-row roll-up (over scoped, excluding
// already-met/cancelled so the chip count never lies) and the
// setFilter toggle the chip clicks call.
//
// The controller takes the rows list and the three scope strings as
// callbacks (not values) so reactivity flows from the page's $state /
// $derived sources without the controller having to know about
// stores or URL params.

import type { Deadline, DeadlineImportance } from '$lib/api';

export interface ImportanceCounts {
  critical: number;
  high: number;
  normal: number;
}

export interface DeadlinesFilterStateDeps {
  /** Loaded deadlines list. Reactivity flows via the getter so a
   *  $state binding behind it stays live. */
  getDeadlines: () => Deadline[];
  /** URL-driven scope project filter. Empty string = no scope. */
  getScopeProject: () => string;
  /** URL-driven scope goal-id filter. Empty string = no scope. */
  getScopeGoalId: () => string;
  /** URL-driven scope venture filter. Empty string = no scope. */
  getScopeVenture: () => string;
}

export interface DeadlinesFilterStateController {
  // Bindable get/set pair so bind:value works at the call site.
  importanceFilter: DeadlineImportance | null;
  q: string;

  // Derived — readonly.
  /** Scope-filtered rows (URL params only). Used by stats / coming-up /
   *  importance counts — anything that should reflect the scoped subset
   *  but not the user's chip filter. */
  readonly scoped: Deadline[];
  /** Final filtered rows (scope + importance + search). Used by the
   *  list, timeline, and calendar views. */
  readonly filtered: Deadline[];
  /** Importance chip-row counts, over scoped + alive rows only. */
  readonly importanceCounts: ImportanceCounts;

  /** Toggle the importance chip — same value twice clears the filter. */
  setFilter(v: DeadlineImportance | null): void;
  /** Reset both filter dimensions. Wired to "clear all" + Esc. */
  clearAll(): void;
}

export function createDeadlinesFilterState(
  deps: DeadlinesFilterStateDeps
): DeadlinesFilterStateController {
  let importanceFilter = $state<DeadlineImportance | null>(null);
  let q = $state('');

  // Scope-filter (URL params) is applied BEFORE the importance filter
  // so counts in the chip row reflect the scoped subset, not the
  // entire vault — matches the user's mental model when they land
  // here via "deadlines for this project".
  let scoped = $derived.by<Deadline[]>(() => {
    let out = deps.getDeadlines();
    const sp = deps.getScopeProject();
    const sg = deps.getScopeGoalId();
    const sv = deps.getScopeVenture();
    if (sp) out = out.filter((d) => d.project === sp);
    if (sg) out = out.filter((d) => d.goal_id === sg);
    if (sv) out = out.filter((d) => d.venture === sv);
    return out;
  });

  let filtered = $derived.by<Deadline[]>(() => {
    let out = scoped;
    if (importanceFilter) out = out.filter((d) => d.importance === importanceFilter);
    const term = q.trim().toLowerCase();
    if (term) {
      out = out.filter(
        (d) =>
          d.title.toLowerCase().includes(term) ||
          (d.description ?? '').toLowerCase().includes(term) ||
          (d.project ?? '').toLowerCase().includes(term) ||
          (d.venture ?? '').toLowerCase().includes(term)
      );
    }
    return out;
  });

  // Counts roll over the FULL scoped list (so the chip row shows the
  // global distribution); the list itself is filtered to whatever the
  // user picked. Hide already-met from the active-importance counts —
  // those belong to the "Met" tail and shouldn't inflate "you have X
  // critical things to worry about".
  let importanceCounts = $derived.by<ImportanceCounts>(() => {
    let critical = 0,
      high = 0,
      normal = 0;
    for (const d of scoped) {
      if (d.status === 'met' || d.status === 'cancelled') continue;
      if (d.importance === 'critical') critical++;
      else if (d.importance === 'high') high++;
      else normal++;
    }
    return { critical, high, normal };
  });

  function setFilter(v: DeadlineImportance | null) {
    importanceFilter = importanceFilter === v ? null : v;
  }

  function clearAll() {
    importanceFilter = null;
    q = '';
  }

  return {
    get importanceFilter() {
      return importanceFilter;
    },
    set importanceFilter(v) {
      importanceFilter = v;
    },
    get q() {
      return q;
    },
    set q(v) {
      q = v;
    },
    get scoped() {
      return scoped;
    },
    get filtered() {
      return filtered;
    },
    get importanceCounts() {
      return importanceCounts;
    },
    setFilter,
    clearAll
  };
}
