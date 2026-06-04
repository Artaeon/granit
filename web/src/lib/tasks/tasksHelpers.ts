// Pure helpers and constants for the tasks surface.
//
// First extraction step out of routes/tasks/+page.svelte (2964 LOC).
// Everything in this file is stateless: enum-like unions, view-cycle
// orderings, predicates that take a Task and return a value, and the
// minute → human-budget formatter. No reactivity, no $state, no
// $derived, no $effect — those live in the .svelte.ts controllers
// added in the follow-up commits.
//
// Splitting these out first means the controllers can import a stable
// API instead of redefining the same predicates inline, and other
// components (TaskCard, AIStaleVerdicts, EisenhowerView, Kanban) can
// pull from one source — previously isSnoozed was redefined in
// TaskCard alongside the page's copy, which drifted over time.
//
// Anything in here is a candidate for sharing with the future
// workspace shell (Phase 2 of the granit vision): a TasksPane embedded
// in a workspace will derive over the same predicates the standalone
// route does.

import { todayISO, fmtDateISO, type Task } from '$lib/api';

// View / Group / SortBy / SmartFilter unions are the page's vocabulary.
// Both the controller and the template need them; keep them in one
// place so future-you doesn't add a new view in two files.
export type View =
  | 'list'
  | 'kanban'
  | 'today'
  | 'week'
  | 'triage'
  | 'inbox'
  | 'stale'
  | 'duplicates'
  | 'quickwins'
  | 'review'
  | 'eisenhower';

export type Group = 'due' | 'priority' | 'note' | 'project' | 'tag' | 'goal' | 'deadline';

export type SortBy = 'auto' | 'priority' | 'due' | 'age' | 'alpha' | 'estimate';

export type SmartFilter =
  | ''
  | 'overdue'
  | 'today'
  | 'tomorrow'
  | 'thisWeek'
  | 'noDue'
  | 'noPriority'
  | 'highPriority'
  | 'hasSubtasks'
  | 'hasEstimate'
  | 'noEstimate';

// View-mode cycle for `[` / `]` keyboard navigation. Order mirrors the
// visible-tab order in the segmented pills (primary cluster, then
// overflow) so the chord walks the user through the same tabs their
// eye sees, left-to-right. Includes every view shape so the cycle is
// exhaustive — narrow viewports that hide some buttons still reach
// them via the chord, which is the whole point of having one.
export const VIEW_CYCLE: View[] = [
  'today',
  'list',
  'week',
  'kanban',
  'eisenhower',
  'inbox',
  'quickwins',
  'stale',
  'duplicates',
  'review',
  'triage'
];

// Numeric direct-jump map for `1`-`5` shortcuts. Maps to the PRIMARY
// tab cluster (Today / List / Kanban / Matrix / Week) in the same
// left-to-right order the segmented pill renders. Overflow views stay
// reachable via the More-views dropdown and the `[` / `]` cycle — no
// digit binding so power users learn to thumb the primary cluster by
// number.
export const VIEW_DIGIT_MAP: Record<string, View> = {
  '1': 'today',
  '2': 'list',
  '3': 'kanban',
  '4': 'eisenhower',
  '5': 'week'
};

// Overflow dropdown vocabulary. Drives both the dropdown items AND
// the "More: <label>" button text so the user can see which overflow
// view is currently active without opening the menu.
export const OVERFLOW_VIEWS: { key: View; label: string; title: string }[] = [
  { key: 'triage', label: 'Triage', title: 'AI-driven inbox triage proposals' },
  { key: 'inbox', label: 'Inbox', title: 'untriaged tasks awaiting categorisation' },
  { key: 'stale', label: 'Stale', title: 'not touched in 7+ days — needs a decision' },
  {
    key: 'duplicates',
    label: 'Duplicates',
    title: 'near-duplicate task pairs by text similarity — deterministic scan, no AI'
  },
  {
    key: 'quickwins',
    label: 'Quick wins',
    title: 'high priority + ≤30 min — tackle a few before lunch'
  },
  { key: 'review', label: 'Review', title: 'completed in the last 7 days — celebrate the wins' }
];

// Folder prefixes + filename pattern that count a notePath as a
// dedicated task surface (daily / Tasks/ / Projects/ / Daily/). Used by
// isTaskLikePath; exposed so other surfaces can apply the same shape
// without re-encoding the rules.
const TASK_FOLDER_PREFIXES = ['daily/', 'tasks/', 'projects/'];
const RE_DAILY_NAME = /(?:^|\/)\d{4}-\d{2}-\d{2}\.md$/;

// isTaskLikePath: heuristic for "this notePath came from a note the
// user clearly meant as a task surface, not a reading list that just
// happens to use `- [ ]` for visual bullets". Pure path-based so we
// don't have to fetch frontmatter for every task.
//
// Match rules (any one is enough to count as task-like):
//   - filename is YYYY-MM-DD.md anywhere → daily note
//   - path begins with Daily/, Tasks/, or Projects/ at any depth
//   - notePath empty → tasks created via the API without a host note;
//     we keep them visible because they were explicit
//
// The folder list intentionally does NOT include arbitrary user
// folders; the user can still see those by flipping the source filter
// to 'all' in the UI. Folder names are case-insensitive on the prefix
// to be friendly to mac/windows-originated vaults.
export function isTaskLikePath(p: string): boolean {
  if (!p) return true;
  if (RE_DAILY_NAME.test(p)) return true;
  const lower = p.toLowerCase();
  for (const prefix of TASK_FOLDER_PREFIXES) {
    if (lower.startsWith(prefix)) return true;
  }
  return false;
}

// Active-snooze predicate. A task is "snoozed" when snoozedUntil is set
// AND parses to a future timestamp. Past timestamps are no-ops (the
// snooze elapsed naturally); unset is the default. Used by the page
// filter pipeline AND by TaskCard's per-row chip; sharing this avoids
// the drift the duplicate definitions used to suffer.
export function isSnoozed(t: Task): boolean {
  if (!t.snoozedUntil) return false;
  const sn = new Date(t.snoozedUntil);
  if (isNaN(sn.getTime())) return false;
  return sn.getTime() > Date.now();
}

// Stale predicate. "Not touched in 7+ days" — done tasks never count
// (they're complete, not abandoned). Used by the Stale view AND the
// AI stale-verdict surface; both want the same threshold so the
// candidate list matches what the user sees.
export function isStale(t: Task): boolean {
  if (t.done) return false;
  const ref = t.updatedAt ?? t.createdAt;
  if (!ref) return false;
  const d = new Date(ref);
  if (isNaN(d.getTime())) return false;
  const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
  return d.getTime() < sevenDaysAgo;
}

// Smart-filter predicate. Each smart-filter chip narrows the visible
// task list to those matching one of a small fixed set of common
// queries. Computed against today's date re-derived per call so a
// long-lived session that crosses midnight rolls over without a
// reload.
//
// childCountByTaskId is the parent → child-count map computed by the
// view controller's parentMap derivation. We accept it as a parameter
// rather than wiring a global so this predicate stays pure and the
// controller owns the dependency.
export function smartPredicate(
  sf: SmartFilter,
  t: Task,
  childCountByTaskId?: Map<string, number>
): boolean {
  if (!sf) return true;
  const today = todayISO();
  const tomorrow = (() => {
    const d = new Date(today + 'T00:00:00');
    d.setDate(d.getDate() + 1);
    return fmtDateISO(d);
  })();
  const weekEnd = (() => {
    const d = new Date(today + 'T00:00:00');
    d.setDate(d.getDate() + 7);
    return fmtDateISO(d);
  })();
  const due = t.dueDate ?? '';
  const sched = t.scheduledStart ? t.scheduledStart.slice(0, 10) : '';
  const dateSignal = due || sched;
  switch (sf) {
    case 'overdue':
      return !t.done && !!due && due < today;
    case 'today':
      return !t.done && (due === today || sched === today);
    case 'tomorrow':
      return !t.done && (due === tomorrow || sched === tomorrow);
    case 'thisWeek':
      return !t.done && !!dateSignal && dateSignal >= today && dateSignal <= weekEnd;
    case 'noDue':
      return !t.done && !due && !sched;
    case 'noPriority':
      return !t.done && (t.priority === 0 || t.priority === 4);
    case 'highPriority':
      return !t.done && t.priority === 1;
    case 'hasSubtasks':
      return !t.done && (childCountByTaskId?.get(t.id) ?? 0) > 0;
    case 'hasEstimate':
      return !t.done && !!t.estimatedMinutes && t.estimatedMinutes > 0;
    case 'noEstimate':
      return !t.done && (!t.estimatedMinutes || t.estimatedMinutes === 0);
    default:
      return true;
  }
}

// Format minutes as a compact human-readable budget — "45m",
// "3h 20m", "1d 4h". 8h is one "day-block" by convention; the chip
// stays scannable even on overflowing backlogs.
export function fmtEstBudget(mins: number): string {
  if (mins < 60) return `${mins}m`;
  if (mins < 8 * 60) {
    const h = Math.floor(mins / 60);
    const m = mins - h * 60;
    return m === 0 ? `${h}h` : `${h}h ${m}m`;
  }
  const d = Math.floor(mins / (8 * 60));
  const remH = Math.floor((mins - d * 8 * 60) / 60);
  return remH === 0 ? `${d}d` : `${d}d ${remH}h`;
}
