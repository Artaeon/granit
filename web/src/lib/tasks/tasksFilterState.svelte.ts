// Filter state for the tasks surface.
//
// Third extraction step out of routes/tasks/+page.svelte. Owns every
// dimension the user can filter the task list by — text search, tag
// chips, project / priority / goal / deadline drop-downs, the
// archived-mode tristate, the smart-filter chip cluster, and the
// task-notes-only source toggle — plus the localStorage persistence
// for the one filter that needs to outlive a refresh (sourceFilter).
//
// Also owns the `filtered` derivation. This is the single source of
// truth that the rest of the page reads from: groupings (listGroups),
// the week view, the kanban, the cursor model, ask-tasks, agent runs,
// the active-chip count, and the "clear all" button. Putting it next
// to the inputs that drive it keeps the dependency chain in one file
// and makes the eventual workspace pane drop-in trivial — the pane
// embeds the controller and reads .filtered, no plumbing.
//
// External state the controller can't own (tasks list, view mode,
// child-count map for hasSubtasks) is reached via the deps bundle.
// Same pattern the noteAutosave controller established: small interface,
// no globals, replaceable for testing or for the workspace shell.

import { type Task, type Project, type Goal, type Deadline } from '$lib/api';
import { loadStoredString, saveStoredString } from '$lib/util/storage';
import {
  type SmartFilter,
  type View,
  isSnoozed,
  isTaskLikePath,
  smartPredicate
} from './tasksHelpers';

const SOURCE_KEY = 'granit.tasks.source.v2';

export interface FilterChip {
  key: string;
  label: string;
  clear: () => void;
  tone?: string;
}

export interface TasksFilterStateDeps {
  /** Loaded tasks. Every filter dimension reads this; reactivity is
   *  picked up via the $state binding behind the getter. */
  getTasks: () => Task[];
  /** Projects sidecar. Used by the project filter to expand the
   *  match into folder + tag membership. */
  getProjects: () => Project[];
  /** Goals sidecar — only for activeFilterChips label lookup
   *  ("goal: <title>" instead of a bare ID). */
  getGoals: () => Goal[];
  /** Deadlines sidecar — same role as goals for the deadline chip. */
  getDeadlines: () => Deadline[];
  /** Active view mode. The filter pipeline branches on
   *  today/inbox/stale/quickwins/review to honor each view's specific
   *  shape; everything else falls through to the generic filter. */
  getView: () => View;
  /** Parent → child-count map from the data controller's parentMap
   *  derivation. Needed by smartPredicate's hasSubtasks branch. */
  getChildCount: () => Map<string, number>;
}

export interface TasksFilterStateController {
  // Bindable state — get + set pair so `bind:value={ctl.q}` works.
  status: 'open' | 'done' | 'all';
  q: string;
  tagFilters: string[];
  projectFilter: string;
  priorityFilter: number | '';
  goalFilter: string;
  deadlineFilter: string;
  archivedMode: 'hide' | 'show' | 'only';
  smartFilter: SmartFilter;
  sourceFilter: 'task-notes' | 'all';

  // Derived — readonly.
  readonly filtered: Task[];
  readonly smartCounts: Record<string, number>;
  readonly activeFilterCount: number;
  readonly activeFilterChips: FilterChip[];

  /** Reset every filter dimension to its default. Wired to the
   *  "clear all" chip in the active-filter row. */
  clearAll(): void;
}

export function createTasksFilterState(
  deps: TasksFilterStateDeps
): TasksFilterStateController {
  let status = $state<'open' | 'done' | 'all'>('open');
  let q = $state('');
  // tagFilters — multi-tag filter with AND semantics. Clicking a tag
  // chip toggles its membership; the visible list shrinks to tasks
  // that carry EVERY active tag. URL serialization is comma-separated
  // (?tag=foo,bar) so shared links round-trip; the backend listTasks
  // call passes only the first tag (the endpoint supports a single
  // tag param) and the rest are AND-narrowed client-side.
  let tagFilters = $state<string[]>([]);
  let projectFilter = $state('');
  let priorityFilter = $state<number | ''>('');
  let goalFilter = $state('');
  let deadlineFilter = $state('');
  // Archived view modes:
  //   'hide'  — default. archived tasks are hidden from every list (server-side filter).
  //   'show'  — show archived tasks alongside active so the user can see the full picture.
  //   'only'  — show ONLY archived (the "archive drawer" view).
  let archivedMode = $state<'hide' | 'show' | 'only'>('hide');
  let smartFilter = $state<SmartFilter>('');
  // Source filter — separates "tasks the user actually wrote as
  // tasks" from "stray `- [ ]` bullets in reading notes / brainstorm
  // pages". Default 'all' matches the README's promise and Amplenote-
  // style capture from arbitrary notes; flipping to 'task-notes'
  // narrows to dedicated task surfaces.
  //
  // Storage key bumped from .source to .source.v2 so existing users
  // who had been silently defaulted to the old 'task-notes' get the
  // new behaviour once (and can re-pick strict mode if they want).
  let sourceFilter = $state<'task-notes' | 'all'>(
    loadStoredString(SOURCE_KEY, 'all') === 'task-notes' ? 'task-notes' : 'all'
  );
  $effect(() => saveStoredString(SOURCE_KEY, sourceFilter));

  let filtered = $derived.by<Task[]>(() => {
    const tasks = deps.getTasks();
    const projects = deps.getProjects();
    const view = deps.getView();
    const childCount = deps.getChildCount();
    let out = tasks;
    if (sourceFilter === 'task-notes') {
      out = out.filter((t) => isTaskLikePath(t.notePath));
    }
    if (q.trim()) {
      const ql = q.toLowerCase();
      out = out.filter(
        (t) => t.text.toLowerCase().includes(ql) || t.notePath.toLowerCase().includes(ql)
      );
    }
    if (priorityFilter !== '') out = out.filter((t) => t.priority === priorityFilter);
    // Multi-tag AND filter — the backend already narrowed by the
    // first tag (if any), so we only need to re-check the rest here.
    // Doing it client-side keeps the filter UI snappy: clicking a
    // second tag chip doesn't force a refetch + re-render of the
    // whole list.
    if (tagFilters.length > 1) {
      out = out.filter((t) => {
        const tags = t.tags ?? [];
        for (let i = 1; i < tagFilters.length; i++) {
          if (!tags.includes(tagFilters[i])) return false;
        }
        return true;
      });
    }
    if (goalFilter) out = out.filter((t) => t.goalId === goalFilter);
    if (deadlineFilter) out = out.filter((t) => t.deadlineId === deadlineFilter);
    if (projectFilter) {
      const proj = projects.find((p) => p.name === projectFilter);
      if (proj) {
        out = out.filter((t) => {
          if (t.projectId === proj.name) return true;
          if (proj.folder && t.notePath.startsWith(proj.folder + '/')) return true;
          if (proj.tags && proj.tags.some((tag) => t.tags?.includes(tag))) return true;
          return false;
        });
      }
    }
    // View-specific filtering
    if (view === 'today') {
      // Today view = open tasks that have a date signal pointing at
      // today: due_date today, scheduled_start today, OR overdue
      // (anything past-due needs to be addressed today by default).
      // Snoozed tasks excluded — if you snoozed a task to tomorrow,
      // it shouldn't crowd today's list.
      const today = new Date().toISOString().slice(0, 10);
      out = out.filter((t) => {
        if (t.done || isSnoozed(t)) return false;
        const due = t.dueDate ?? '';
        const sched = t.scheduledStart ? t.scheduledStart.slice(0, 10) : '';
        return due === today || sched === today || (!!due && due < today);
      });
    } else if (view === 'inbox') {
      out = out.filter((t) => !t.done && (t.triage || 'inbox') === 'inbox');
    } else if (view === 'stale') {
      // isStale lives in helpers; importing it just for this branch is
      // a tighter dep surface than reaching across modules.
      const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
      out = out.filter((t) => {
        if (t.done) return false;
        const ref = t.updatedAt ?? t.createdAt;
        if (!ref) return false;
        const d = new Date(ref);
        if (isNaN(d.getTime())) return false;
        return d.getTime() < sevenDaysAgo;
      });
    } else if (view === 'quickwins') {
      out = out.filter(
        (t) =>
          !t.done &&
          t.priority >= 1 &&
          t.priority <= 2 &&
          t.estimatedMinutes &&
          t.estimatedMinutes <= 30
      );
    } else if (view === 'review') {
      const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
      out = out.filter(
        (t) => t.done && t.completedAt && new Date(t.completedAt).getTime() > sevenDaysAgo
      );
    } else {
      // For all non-special views, hide currently-snoozed tasks
      // unless explicitly viewing all/done.
      if (status === 'open') out = out.filter((t) => !isSnoozed(t));
    }
    // Smart filter chip — applied last so it always operates on the
    // result of every other dimension.
    if (smartFilter) {
      out = out.filter((t) => smartPredicate(smartFilter, t, childCount));
    }
    return out;
  });

  // Smart-filter live counts — how many tasks would each chip show
  // given the OTHER active filters. We count against the loaded
  // tasks (no filters applied) — simple and gives the user "across
  // the whole vault, how many overdue exist?". Other active filters
  // are still applied to the visible list.
  let smartCounts = $derived.by(() => {
    const tasks = deps.getTasks();
    const childCount = deps.getChildCount();
    const counts: Record<string, number> = {};
    const filters: SmartFilter[] = [
      'overdue',
      'today',
      'tomorrow',
      'thisWeek',
      'noDue',
      'noPriority',
      'highPriority',
      'hasSubtasks',
      'hasEstimate',
      'noEstimate'
    ];
    for (const f of filters) counts[f] = 0;
    for (const t of tasks) {
      if (t.archived && archivedMode === 'hide') continue;
      if (isSnoozed(t) && status === 'open') continue;
      for (const f of filters) {
        if (smartPredicate(f, t, childCount)) counts[f]++;
      }
    }
    return counts;
  });

  let activeFilterCount = $derived(
    (priorityFilter !== '' ? 1 : 0) +
      (projectFilter ? 1 : 0) +
      tagFilters.length +
      (goalFilter ? 1 : 0) +
      (deadlineFilter ? 1 : 0) +
      (q ? 1 : 0) +
      (status !== 'open' ? 1 : 0) +
      (sourceFilter !== 'all' ? 1 : 0)
  );

  // Active-filter chip row. Each filter that's not at its default
  // surfaces as a removable chip. Lets power users see + clear
  // filters at a glance without opening the filter drawer (mobile)
  // or scrolling the sidebar (desktop).
  let activeFilterChips = $derived.by<FilterChip[]>(() => {
    const goals = deps.getGoals();
    const deadlines = deps.getDeadlines();
    const out: FilterChip[] = [];
    if (q) {
      out.push({
        key: 'q',
        label: `search: "${q.length > 18 ? q.slice(0, 17) + '…' : q}"`,
        clear: () => (q = '')
      });
    }
    if (status !== 'open') {
      out.push({
        key: 'status',
        label: `status: ${status}`,
        clear: () => (status = 'open')
      });
    }
    if (priorityFilter !== '') {
      const tone =
        priorityFilter === 1
          ? 'text-error'
          : priorityFilter === 2
            ? 'text-warning'
            : 'text-info';
      out.push({
        key: 'priority',
        label: `P${priorityFilter}`,
        clear: () => (priorityFilter = ''),
        tone
      });
    }
    if (projectFilter) {
      out.push({
        key: 'project',
        label: `project: ${projectFilter.length > 16 ? projectFilter.slice(0, 15) + '…' : projectFilter}`,
        clear: () => (projectFilter = '')
      });
    }
    // One filter chip per active tag — clicking × removes that
    // single tag, not the whole multi-tag filter set.
    for (const t of tagFilters) {
      out.push({
        key: `tag:${t}`,
        label: `#${t.replace(/^#/, '')}`,
        clear: () => (tagFilters = tagFilters.filter((x) => x !== t))
      });
    }
    if (goalFilter) {
      const g = goals.find((x) => x.id === goalFilter);
      out.push({
        key: 'goal',
        label: `goal: ${g?.title ?? goalFilter}`,
        clear: () => (goalFilter = '')
      });
    }
    if (deadlineFilter) {
      const d = deadlines.find((x) => x.id === deadlineFilter);
      out.push({
        key: 'deadline',
        label: `deadline: ${d?.title ?? deadlineFilter}`,
        clear: () => (deadlineFilter = '')
      });
    }
    if (sourceFilter !== 'all') {
      out.push({
        key: 'source',
        label: 'task notes only',
        clear: () => (sourceFilter = 'all')
      });
    }
    if (smartFilter) {
      const labels: Record<string, string> = {
        overdue: 'overdue',
        today: 'today',
        tomorrow: 'tomorrow',
        thisWeek: 'this week',
        noDue: 'no due date',
        noPriority: 'no priority',
        highPriority: 'high priority',
        hasSubtasks: 'has subtasks',
        hasEstimate: 'has estimate',
        noEstimate: 'no estimate'
      };
      out.push({
        key: 'smart',
        label: labels[smartFilter] ?? smartFilter,
        clear: () => (smartFilter = '')
      });
    }
    return out;
  });

  function clearAll() {
    q = '';
    status = 'open';
    priorityFilter = '';
    projectFilter = '';
    tagFilters = [];
    goalFilter = '';
    deadlineFilter = '';
    sourceFilter = 'all';
    smartFilter = '';
  }

  return {
    get status() {
      return status;
    },
    set status(v) {
      status = v;
    },
    get q() {
      return q;
    },
    set q(v) {
      q = v;
    },
    get tagFilters() {
      return tagFilters;
    },
    set tagFilters(v) {
      tagFilters = v;
    },
    get projectFilter() {
      return projectFilter;
    },
    set projectFilter(v) {
      projectFilter = v;
    },
    get priorityFilter() {
      return priorityFilter;
    },
    set priorityFilter(v) {
      priorityFilter = v;
    },
    get goalFilter() {
      return goalFilter;
    },
    set goalFilter(v) {
      goalFilter = v;
    },
    get deadlineFilter() {
      return deadlineFilter;
    },
    set deadlineFilter(v) {
      deadlineFilter = v;
    },
    get archivedMode() {
      return archivedMode;
    },
    set archivedMode(v) {
      archivedMode = v;
    },
    get smartFilter() {
      return smartFilter;
    },
    set smartFilter(v) {
      smartFilter = v;
    },
    get sourceFilter() {
      return sourceFilter;
    },
    set sourceFilter(v) {
      sourceFilter = v;
    },
    get filtered() {
      return filtered;
    },
    get smartCounts() {
      return smartCounts;
    },
    get activeFilterCount() {
      return activeFilterCount;
    },
    get activeFilterChips() {
      return activeFilterChips;
    },
    clearAll
  };
}
