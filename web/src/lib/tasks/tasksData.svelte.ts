// Data state for the tasks surface.
//
// Fifth extraction step out of routes/tasks/+page.svelte. Owns the
// four arrays the page works against — tasks, projects, goals,
// deadlines — plus the loading flag, the subtask-tree derivations
// (parentMap + childCount), the at-a-glance stats summary, and the
// per-vault metadata derivations (allTags, countOpen, countDone).
//
// Also owns the per-user subtree collapse state (collapsedIds) and
// the two helpers that consume it (isHiddenByCollapse, toggleCollapsed)
// because both depend on parentMap — co-locating them keeps the
// hierarchy-aware logic in one file.
//
// The page still owns load() and the WS subscription orchestration:
// load() touches the `$auth` store + builds API params from filterCtl,
// and the WS / visibility listeners must install from component
// context. Both write into this controller's tasks/projects/goals/
// deadlines via the setter pairs.

import { type Task, type Project, type Goal, type Deadline } from '$lib/api';
import { loadStored, saveStored } from '$lib/util/storage';
import { isSnoozed } from './tasksHelpers';

const COLLAPSE_KEY = 'granit.tasks.collapsed';

export interface TaskStats {
  open: number;
  overdue: number;
  todayCount: number;
  doneToday: number;
  doneWeek: number;
  snoozed: number;
  sumEstMin: number;
  noEstCount: number;
  avgPriority: number;
}

export interface TasksDataController {
  // Loaded sidecars — bindable so load() can write the API response
  // back into the controller.
  tasks: Task[];
  projects: Project[];
  goals: Goal[];
  deadlines: Deadline[];

  /** True while load() is in flight. Drives the toolbar spinner. */
  loading: boolean;

  // Derived — readonly.
  /** task-id → parent-id map. Built once per tasks update; used by
   *  the collapse logic and by the hasSubtasks smart filter. */
  readonly parentMap: Map<string, string>;
  /** parent-id → number of direct children. Used to show the
   *  collapse chevron and the (n) child count next to it. */
  readonly childCount: Map<string, number>;
  /** Every tag mentioned across the loaded tasks, sorted. Used by
   *  the tag-filter chip cluster. */
  readonly allTags: string[];
  /** Count of !done tasks across the unfiltered list. */
  readonly countOpen: number;
  /** Count of done tasks across the unfiltered list. */
  readonly countDone: number;
  /** At-a-glance summary chips (open / overdue / today / done /
   *  snoozed / minute-budget / average priority). */
  readonly stats: TaskStats;

  // Bindable per-user subtree collapse state. Persisted to localStorage
  // under granit.tasks.collapsed.
  collapsedIds: Set<string>;

  /** Walk ancestry; true if any ancestor (transitive) is collapsed. */
  isHiddenByCollapse(taskId: string, collapsed: Set<string>): boolean;
  /** Toggle the collapse state of a single task. */
  toggleCollapsed(taskId: string): void;
}

export function createTasksData(): TasksDataController {
  let tasks = $state<Task[]>([]);
  let projects = $state<Project[]>([]);
  let goals = $state<Goal[]>([]);
  let deadlines = $state<Deadline[]>([]);
  let loading = $state(false);
  let collapsedIds = $state<Set<string>>(
    new Set(loadStored<string[]>(COLLAPSE_KEY, []))
  );

  $effect(() => saveStored(COLLAPSE_KEY, Array.from(collapsedIds)));

  // Parent map: for every task with indent > 0, finds its parent in
  // the same notePath (the nearest preceding task with smaller
  // indent). Built once per task-list update so the collapse logic
  // doesn't recompute O(N²) on every render.
  let parentMap = $derived.by<Map<string, string>>(() => {
    const m = new Map<string, string>();
    // Group by notePath then walk in line order so the parent search
    // is bounded to within a note.
    const byNote: Record<string, Task[]> = {};
    for (const t of tasks) (byNote[t.notePath] ??= []).push(t);
    for (const list of Object.values(byNote)) {
      list.sort((a, b) => a.lineNum - b.lineNum);
      const stack: Task[] = [];
      for (const t of list) {
        const ind = t.indent ?? 0;
        while (stack.length > 0 && (stack[stack.length - 1].indent ?? 0) >= ind) {
          stack.pop();
        }
        if (stack.length > 0) m.set(t.id, stack[stack.length - 1].id);
        stack.push(t);
      }
    }
    return m;
  });

  // Inverse: parentId → child count. Used to know whether a task even
  // HAS children (show the chevron) and to count them in the toggle
  // label.
  let childCount = $derived.by<Map<string, number>>(() => {
    const c = new Map<string, number>();
    for (const childId of parentMap.keys()) {
      const parent = parentMap.get(childId)!;
      c.set(parent, (c.get(parent) ?? 0) + 1);
    }
    return c;
  });

  let allTags = $derived.by<string[]>(() => {
    const s = new Set<string>();
    for (const t of tasks) for (const tag of t.tags ?? []) s.add(tag);
    return Array.from(s).sort();
  });

  let countOpen = $derived(tasks.filter((t) => !t.done).length);
  let countDone = $derived(tasks.filter((t) => t.done).length);

  // At-a-glance stats over the unfiltered open task list. Surfaced
  // as small chips above the list so the user always knows the
  // overall load — even when a filter is hiding most of it.
  let stats = $derived.by<TaskStats>(() => {
    const today = new Date().toISOString().slice(0, 10);
    // Week boundary: completedAt within the last 7 calendar days
    // (Sunday-relative would surprise users mid-week, so we keep it
    // rolling-7d). Used by the "Done · 7d" chip.
    const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
    let open = 0,
      overdue = 0,
      todayCount = 0,
      doneToday = 0,
      doneWeek = 0,
      snoozed = 0,
      // sumEstMin accumulates estimatedMinutes across OPEN,
      // non-snoozed tasks. Power users budget their day in minutes —
      // surfacing "1280m queued" tells them at a glance whether the
      // filtered list is doable today (a typical focus day is
      // ~5h = 300m). Tasks with no estimate contribute 0 — those get
      // a separate "untouched" counter the user can act on.
      sumEstMin = 0,
      // Count of open non-snoozed tasks with no estimate — power-UI
      // nudge to add estimates so the time budget chip becomes
      // accurate. Shown only when > 0.
      noEstCount = 0,
      // priority accumulator for the average. Skips P0 (unset)
      // because mixing "no priority" with P1/P2/P3 would skew the
      // mean toward 0 and read as falsely-low urgency.
      prioritySum = 0,
      priorityCount = 0;
    for (const t of tasks) {
      const sn = isSnoozed(t);
      if (!t.done) {
        open++;
        if (sn) snoozed++;
        else {
          const d = t.dueDate ?? (t.scheduledStart ? t.scheduledStart.slice(0, 10) : '');
          if (d && d < today) overdue++;
          else if (d === today) todayCount++;
        }
        if (t.priority >= 1 && t.priority <= 3) {
          prioritySum += t.priority;
          priorityCount++;
        }
        if (t.estimatedMinutes && t.estimatedMinutes > 0) {
          sumEstMin += t.estimatedMinutes;
        } else {
          noEstCount++;
        }
      } else if (t.completedAt) {
        const day = t.completedAt.slice(0, 10);
        if (day === today) doneToday++;
        if (new Date(t.completedAt).getTime() > sevenDaysAgo) doneWeek++;
      }
    }
    const avgPriority = priorityCount > 0 ? prioritySum / priorityCount : 0;
    return {
      open,
      overdue,
      todayCount,
      doneToday,
      doneWeek,
      snoozed,
      sumEstMin,
      noEstCount,
      avgPriority
    };
  });

  function isHiddenByCollapse(taskId: string, collapsed: Set<string>): boolean {
    let cur: string | undefined = parentMap.get(taskId);
    while (cur) {
      if (collapsed.has(cur)) return true;
      cur = parentMap.get(cur);
    }
    return false;
  }

  function toggleCollapsed(taskId: string) {
    const next = new Set(collapsedIds);
    if (next.has(taskId)) next.delete(taskId);
    else next.add(taskId);
    collapsedIds = next;
  }

  return {
    get tasks() {
      return tasks;
    },
    set tasks(v) {
      tasks = v;
    },
    get projects() {
      return projects;
    },
    set projects(v) {
      projects = v;
    },
    get goals() {
      return goals;
    },
    set goals(v) {
      goals = v;
    },
    get deadlines() {
      return deadlines;
    },
    set deadlines(v) {
      deadlines = v;
    },
    get loading() {
      return loading;
    },
    set loading(v) {
      loading = v;
    },
    get parentMap() {
      return parentMap;
    },
    get childCount() {
      return childCount;
    },
    get allTags() {
      return allTags;
    },
    get countOpen() {
      return countOpen;
    },
    get countDone() {
      return countDone;
    },
    get stats() {
      return stats;
    },
    get collapsedIds() {
      return collapsedIds;
    },
    set collapsedIds(v) {
      collapsedIds = v;
    },
    isHiddenByCollapse,
    toggleCollapsed
  };
}
