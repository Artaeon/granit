// Data state for the goals surface.
//
// Second extraction step out of routes/goals/+page.svelte. Owns the
// four loaded arrays (goals + the three linked sidecars: openTasks,
// doneTasks, projects), the two loading flags (loading, firstLoaded),
// the load() function, and every derivation that operates over the
// data alone — the per-goal rollups (open/done task counts + linked
// project lookup), the stalled-goal detection, and the
// rollupFor/staleness/recentCompletionForGoal helpers.
//
// The page still owns the onMount install ordering, the WS
// subscription, and the visibility-aware refresh. Both call into
// dataCtl methods; the controller exposes loaded state via
// getter/setter pairs so the agent surface can still mutate the
// arrays optimistically before the next load().

import { api, type Goal, type Project, type Task } from '$lib/api';

export type GoalRollups = {
  byGoalOpen: Map<string, number>;
  byGoalDone: Map<string, number>;
  projByName: Map<string, Project>;
};

export type GoalRollup = {
  open: number;
  done: number;
  project: Project | null;
};

export interface GoalsDataDeps {
  /** Boolean snapshot of the auth store — used as a guard before
   *  load(). The page passes () => !!$auth so the read stays
   *  reactive in the calling context. */
  isAuthed: () => boolean;
}

export interface GoalsDataController {
  goals: Goal[];
  openTasks: Task[];
  doneTasks: Task[];
  projects: Project[];

  loading: boolean;
  firstLoaded: boolean;

  // Derived.
  readonly rollups: GoalRollups;
  readonly stalledGoals: Goal[];

  /** Fetch goals + linked context (tasks + projects) in parallel.
   *  Failures of secondary calls don't block the goals list itself. */
  load(): Promise<void>;
  /** Per-goal rollup chip data: open/done task counts + the linked
   *  project (matched case-insensitively against goal.project). */
  rollupFor(g: Goal): GoalRollup;
  /** Days since the ISO timestamp; +Inf for missing / invalid. */
  staleness(iso: string | undefined): number;
  /** True if any task linked to the goal completed in the last 14
   *  days. Used by the stalled-goal detection. */
  recentCompletionForGoal(goalId: string): boolean;
}

export function createGoalsData(deps: GoalsDataDeps): GoalsDataController {
  let goals = $state<Goal[]>([]);
  let openTasks = $state<Task[]>([]);
  let doneTasks = $state<Task[]>([]);
  let projects = $state<Project[]>([]);
  let loading = $state(false);
  let firstLoaded = $state(false);

  async function load() {
    if (!deps.isAuthed()) return;
    loading = true;
    try {
      // Fetch goals + linked context (tasks + projects) in parallel.
      // The roll-up is purely advisory — failures of the secondary
      // calls shouldn't block the goals list itself, so each is
      // wrapped in its own try and logged-but-ignored on error.
      const [list, openRes, doneRes, projRes] = await Promise.allSettled([
        api.listGoals(),
        api.listTasks({ status: 'open' }),
        api.listTasks({ status: 'done' }),
        api.listProjects()
      ]);
      if (list.status === 'fulfilled') goals = list.value.goals;
      openTasks = openRes.status === 'fulfilled' ? openRes.value.tasks : [];
      doneTasks = doneRes.status === 'fulfilled' ? doneRes.value.tasks : [];
      projects = projRes.status === 'fulfilled' ? projRes.value.projects : [];
    } finally {
      loading = false;
      firstLoaded = true;
    }
  }

  // Per-goal index of linked tasks (by goalId) + linked projects (by
  // matching goal.project against project.name, since the schema is
  // free-text not FK). Computed in a single pass over each list so
  // the per-goal lookups in the render loop stay O(1). Used by the
  // cards view to surface a "5 open · 2 done · 1 project" chip row.
  let rollups = $derived.by<GoalRollups>(() => {
    const byGoalOpen = new Map<string, number>();
    const byGoalDone = new Map<string, number>();
    for (const t of openTasks) {
      if (!t.goalId) continue;
      byGoalOpen.set(t.goalId, (byGoalOpen.get(t.goalId) ?? 0) + 1);
    }
    for (const t of doneTasks) {
      if (!t.goalId) continue;
      byGoalDone.set(t.goalId, (byGoalDone.get(t.goalId) ?? 0) + 1);
    }
    // Projects index by name (lowercased) so a goal.project field
    // matches case-insensitively. We only need a presence check +
    // the matched project's `progress` field for surface-level
    // context, so the value is the project itself.
    const projByName = new Map<string, Project>();
    for (const p of projects) projByName.set(p.name.toLowerCase(), p);
    return { byGoalOpen, byGoalDone, projByName };
  });

  function rollupFor(g: Goal): GoalRollup {
    const open = rollups.byGoalOpen.get(g.id) ?? 0;
    const done = rollups.byGoalDone.get(g.id) ?? 0;
    const project = g.project
      ? rollups.projByName.get(g.project.toLowerCase()) ?? null
      : null;
    return { open, done, project };
  }

  function staleness(iso: string | undefined): number {
    if (!iso) return Number.POSITIVE_INFINITY;
    const t = Date.parse(iso);
    if (Number.isNaN(t)) return Number.POSITIVE_INFINITY;
    return Math.floor((Date.now() - t) / 86_400_000);
  }

  function recentCompletionForGoal(goalId: string): boolean {
    const fortnight = Date.now() - 14 * 86_400_000;
    for (const t of doneTasks) {
      if (t.goalId !== goalId) continue;
      const ts = Date.parse(t.updatedAt ?? t.createdAt ?? '');
      if (!Number.isNaN(ts) && ts >= fortnight) return true;
    }
    return false;
  }

  // Stalled-goal detection: an active goal whose record itself
  // hasn't been touched in 30+ days AND no linked task has been
  // completed in the last 14 days. The banner exists to convert "I
  // forgot about this" into a visible signal without auto-archiving
  // — the user decides whether to update the goal, log a milestone,
  // or move it to paused/archived.
  let stalledGoals = $derived.by<Goal[]>(() =>
    goals.filter((g) => {
      const status = g.status ?? 'active';
      if (status !== 'active') return false;
      const days = staleness(g.updated_at);
      if (days < 30) return false;
      return !recentCompletionForGoal(g.id);
    })
  );

  return {
    get goals() {
      return goals;
    },
    set goals(v) {
      goals = v;
    },
    get openTasks() {
      return openTasks;
    },
    set openTasks(v) {
      openTasks = v;
    },
    get doneTasks() {
      return doneTasks;
    },
    set doneTasks(v) {
      doneTasks = v;
    },
    get projects() {
      return projects;
    },
    set projects(v) {
      projects = v;
    },
    get loading() {
      return loading;
    },
    set loading(v) {
      loading = v;
    },
    get firstLoaded() {
      return firstLoaded;
    },
    set firstLoaded(v) {
      firstLoaded = v;
    },
    get rollups() {
      return rollups;
    },
    get stalledGoals() {
      return stalledGoals;
    },
    load,
    rollupFor,
    staleness,
    recentCompletionForGoal
  };
}
