// View / layout state for the deadlines surface.
//
// Fourth extraction step out of routes/deadlines/+page.svelte. Owns
// the three persisted layout preferences:
//
//   viewMode           — 'list' | 'timeline' | 'calendar'
//   groupBy            — 'urgency' | 'status' | 'month'
//   collapsedSections  — Record<bucketKey, boolean> (three-state, see below)
//
// All three keys round-trip through localStorage so the user's last
// layout reopens exactly as they left it. Validation against the
// union types tolerates a stale key from an older version that no
// longer maps to a real view, falling back to a safe default rather
// than crashing.
//
// Pure UI state — no API calls, no derived data over rows. Splitting
// this out of the page frees the body from the persistence plumbing
// + makes the same controller usable from any other surface that
// embeds the deadlines list (pane shell, dashboard widget).

import { loadStored, saveStored, loadStoredString, saveStoredString } from '$lib/util/storage';
import type { GroupBy } from './deadlinesBuckets';

export type ViewMode = 'list' | 'timeline' | 'calendar';

const VIEW_KEY = 'granit.deadlines.view';
const GROUP_KEY = 'granit.deadlines.groupby';
const COLLAPSE_KEY = 'granit.deadlines.collapsedSections';

const VIEW_VALUES = ['list', 'timeline', 'calendar'] as const;
const GROUP_VALUES = ['urgency', 'status', 'month'] as const;

function loadView(): ViewMode {
  const v = loadStoredString(VIEW_KEY, 'list');
  return (VIEW_VALUES as readonly string[]).includes(v) ? (v as ViewMode) : 'list';
}
function loadGroup(): GroupBy {
  const v = loadStoredString(GROUP_KEY, 'urgency');
  return (GROUP_VALUES as readonly string[]).includes(v) ? (v as GroupBy) : 'urgency';
}

export interface DeadlinesViewStateController {
  viewMode: ViewMode;
  groupBy: GroupBy;
  collapsedSections: Record<string, boolean>;

  /** Three-state section toggle:
   *    undefined → true (explicitly collapsed)
   *    true      → false (explicitly expanded)
   *    false     → undefined (back to bucket-tone default)
   *  The undefined state means "use the default for this bucket
   *  tone" so the user can return to defaults by clicking twice. */
  toggleSection(key: string): void;

  /** Cycle viewMode through the canonical order (list → timeline →
   *  calendar → list). Wired to the `v` global hotkey. */
  cycleView(): void;
  /** Cycle groupBy through the canonical order (urgency → status →
   *  month → urgency). Wired to the `g` global hotkey. */
  cycleGroup(): void;
}

export function createDeadlinesViewState(): DeadlinesViewStateController {
  let viewMode = $state<ViewMode>(loadView());
  let groupBy = $state<GroupBy>(loadGroup());
  let collapsedSections = $state<Record<string, boolean>>(
    loadStored<Record<string, boolean>>(COLLAPSE_KEY, {})
  );

  $effect(() => saveStoredString(VIEW_KEY, viewMode));
  $effect(() => saveStoredString(GROUP_KEY, groupBy));
  $effect(() => saveStored(COLLAPSE_KEY, collapsedSections));

  function toggleSection(key: string) {
    const cur = collapsedSections[key];
    if (cur === undefined) collapsedSections = { ...collapsedSections, [key]: true };
    else if (cur === true) collapsedSections = { ...collapsedSections, [key]: false };
    else {
      const { [key]: _drop, ...rest } = collapsedSections;
      collapsedSections = rest;
    }
  }

  function cycleView() {
    const order: ViewMode[] = ['list', 'timeline', 'calendar'];
    viewMode = order[(order.indexOf(viewMode) + 1) % order.length];
  }
  function cycleGroup() {
    const order: GroupBy[] = ['urgency', 'status', 'month'];
    groupBy = order[(order.indexOf(groupBy) + 1) % order.length];
  }

  return {
    get viewMode() {
      return viewMode;
    },
    set viewMode(v) {
      viewMode = v;
    },
    get groupBy() {
      return groupBy;
    },
    set groupBy(v) {
      groupBy = v;
    },
    get collapsedSections() {
      return collapsedSections;
    },
    set collapsedSections(v) {
      collapsedSections = v;
    },
    toggleSection,
    cycleView,
    cycleGroup
  };
}
