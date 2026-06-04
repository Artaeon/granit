// Saved filter presets for the tasks surface.
//
// Second extraction step out of routes/tasks/+page.svelte. The page
// previously held both the presets array and the 13 reactive variables
// each preset reads from / writes to inline; this controller owns the
// presets list + persistence + CRUD, and reaches the page's filter +
// view state through a small snapshot bridge.
//
// The snapshot indirection (getSnapshot / applySnapshot in the deps
// bundle) keeps the controller decoupled from the eventual
// tasksFilterState + tasksViewState controllers — same controller
// works whether the underlying state lives inline in the page (today)
// or behind two purpose-built controllers (after the next two
// commits). The future workspace shell will hand a different snapshot
// bridge into the same store when tasks lives as an embedded pane.

import { loadStored, saveStored } from '$lib/util/storage';
import { toast } from '$lib/components/toast';
import type { View, Group, SortBy, SmartFilter } from './tasksHelpers';

// FilterSnapshot captures every preset-managed dimension in one
// object. Field names mirror the page-local variables they came from
// so the bridge functions stay a 1:1 mapping.
export interface FilterSnapshot {
  status: 'open' | 'done' | 'all';
  q: string;
  tagFilters: string[];
  projectFilter: string;
  priorityFilter: number | '';
  goalFilter: string;
  deadlineFilter: string;
  view: View;
  groupBy: Group;
  sortBy: SortBy;
  sourceFilter: 'task-notes' | 'all';
  smartFilter: SmartFilter;
  archivedMode: 'hide' | 'show' | 'only';
}

// FilterPreset is the on-disk shape. Predates the snapshot abstraction;
// the legacy `tag: string` single-tag field is kept so older saved
// presets keep round-tripping, but `tags?: string[]` is the source of
// truth for newer captures.
export type FilterPreset = {
  name: string;
  status: 'open' | 'done' | 'all';
  q: string;
  // Legacy string `tag` was a single tag; newer presets persist the
  // multi-tag array directly. captureCurrentAsPreset writes both
  // fields so older code paths reading `tag` still work, and
  // applyPreset prefers the array when present.
  tag: string;
  tags?: string[];
  project: string;
  priority: number | '';
  goal: string;
  deadline: string;
  view: View;
  groupBy: Group;
  // Newer fields — old presets without them load with falsy defaults
  // via the `?? ''` reads in applyPreset.
  sortBy?: SortBy;
  sourceFilter?: 'all' | 'task-notes';
  smartFilter?: SmartFilter;
  archivedMode?: 'hide' | 'show' | 'only';
};

const PRESETS_KEY = 'granit.tasks.presets';

// Built-in starter presets. Surface a few well-named common filter
// combos so the presets row isn't empty for first-time users. Only
// shown when the user has zero saved presets; once they save their
// own, the starter set hides. Clicking applies the combo; from there
// the user can tweak and "save current" to make it their own.
export const STARTER_PRESETS: FilterPreset[] = [
  { name: 'P1 this week', status: 'open', q: '', tag: '', project: '', priority: 1, goal: '', deadline: '', view: 'list', groupBy: 'due', smartFilter: 'thisWeek' },
  { name: 'Inbox', status: 'open', q: '', tag: '', project: '', priority: '', goal: '', deadline: '', view: 'inbox', groupBy: 'priority' },
  { name: 'Overdue', status: 'open', q: '', tag: '', project: '', priority: '', goal: '', deadline: '', view: 'list', groupBy: 'priority', smartFilter: 'overdue' },
  { name: 'Quick wins', status: 'open', q: '', tag: '', project: '', priority: '', goal: '', deadline: '', view: 'quickwins', groupBy: 'priority' },
  { name: 'Recently done', status: 'done', q: '', tag: '', project: '', priority: '', goal: '', deadline: '', view: 'review', groupBy: 'due' }
];

function snapshotToPreset(name: string, s: FilterSnapshot): FilterPreset {
  return {
    name,
    status: s.status,
    q: s.q,
    tag: s.tagFilters[0] ?? '',
    tags: [...s.tagFilters],
    project: s.projectFilter,
    priority: s.priorityFilter,
    goal: s.goalFilter,
    deadline: s.deadlineFilter,
    view: s.view,
    groupBy: s.groupBy,
    sortBy: s.sortBy,
    sourceFilter: s.sourceFilter,
    smartFilter: s.smartFilter,
    archivedMode: s.archivedMode
  };
}

function presetToSnapshot(p: FilterPreset): FilterSnapshot {
  return {
    status: p.status,
    q: p.q,
    tagFilters: Array.isArray(p.tags) ? [...p.tags] : (p.tag ? [p.tag] : []),
    projectFilter: p.project,
    priorityFilter: p.priority,
    goalFilter: p.goal,
    deadlineFilter: p.deadline,
    view: p.view,
    groupBy: p.groupBy,
    sortBy: p.sortBy ?? 'auto',
    sourceFilter: p.sourceFilter ?? 'all',
    smartFilter: p.smartFilter ?? '',
    archivedMode: p.archivedMode ?? 'hide'
  };
}

export interface PresetsControllerDeps {
  /** Read every preset-managed dimension as one object. Called when
   *  the user saves the current view as a preset OR when comparing
   *  the current state to an existing preset for the highlight ring. */
  getSnapshot: () => FilterSnapshot;
  /** Write every preset-managed dimension back into the host state.
   *  Called when the user clicks an existing preset chip. */
  applySnapshot: (s: FilterSnapshot) => void;
}

export interface PresetsController {
  /** User-saved presets if any, otherwise the starter set. */
  readonly visiblePresets: FilterPreset[];
  /** True when zero user presets are saved — drives the "Starters"
   *  header label so the user knows the chips they see are the
   *  built-in suggestions, not their own saves. */
  readonly isShowingStarters: boolean;
  /** Prompt for a name and save the current snapshot as a preset.
   *  Names are unique — same-name save overwrites the prior entry. */
  capture(): void;
  /** Restore a preset's filter + view state into the host. */
  apply(p: FilterPreset): void;
  /** Drop a preset by name. */
  remove(name: string): void;
  /** True when every dimension of the preset matches the current
   *  snapshot. Used to highlight the "active" chip. */
  matches(p: FilterPreset): boolean;
}

export function createPresetsController(deps: PresetsControllerDeps): PresetsController {
  let presets = $state<FilterPreset[]>(loadStored<FilterPreset[]>(PRESETS_KEY, []));

  function persist() {
    saveStored(PRESETS_KEY, presets);
  }

  function capture() {
    const name = prompt('Name this filter preset:', '');
    if (!name || !name.trim()) return;
    const trimmed = name.trim();
    const next = presets.filter((p) => p.name !== trimmed);
    next.unshift(snapshotToPreset(trimmed, deps.getSnapshot()));
    presets = next;
    persist();
    toast.success(`Saved preset "${trimmed}"`);
  }

  function apply(p: FilterPreset) {
    deps.applySnapshot(presetToSnapshot(p));
  }

  function remove(name: string) {
    presets = presets.filter((p) => p.name !== name);
    persist();
  }

  function matches(p: FilterPreset): boolean {
    const s = deps.getSnapshot();
    const presetTags = Array.isArray(p.tags) ? p.tags : (p.tag ? [p.tag] : []);
    if (presetTags.length !== s.tagFilters.length) return false;
    if (presetTags.some((t, i) => t !== s.tagFilters[i])) return false;
    return (
      p.status === s.status &&
      p.q === s.q &&
      p.project === s.projectFilter &&
      p.priority === s.priorityFilter &&
      p.goal === s.goalFilter &&
      p.deadline === s.deadlineFilter &&
      p.view === s.view &&
      p.groupBy === s.groupBy &&
      (p.sortBy ?? 'auto') === s.sortBy &&
      (p.sourceFilter ?? 'all') === s.sourceFilter &&
      (p.smartFilter ?? '') === s.smartFilter &&
      (p.archivedMode ?? 'hide') === s.archivedMode
    );
  }

  return {
    get visiblePresets() {
      return presets.length > 0 ? presets : STARTER_PRESETS;
    },
    get isShowingStarters() {
      return presets.length === 0;
    },
    capture,
    apply,
    remove,
    matches
  };
}
