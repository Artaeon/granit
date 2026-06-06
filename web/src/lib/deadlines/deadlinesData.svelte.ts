// Data state for the deadlines surface.
//
// Third extraction step out of routes/deadlines/+page.svelte. Owns
// the loaded sidecars (deadlines + the three linked context arrays:
// goals, projects, ventures) plus the lazy open-tasks pool used by
// the drawer's "link to tasks" multi-select, the loading + busy
// flags, and the load() / ensureTasksLoaded() functions.
//
// Mirrors $lib/goals/goalsData and $lib/tasks/tasksData: the page
// still owns the onMount install ordering, the WebSocket
// subscription, and visibility-aware refresh — those touch
// component-scoped lifecycle that can't move into a controller. The
// controller exposes the sidecars as get/set pairs so optimistic
// updates from quick-actions and the drawer remain possible.

import { api, type Deadline, type Goal, type Project, type Task, type Venture } from '$lib/api';
import { toast } from '$lib/components/toast';
import { errorMessage } from '$lib/util/errorMessage';

export interface DeadlinesDataDeps {
  /** Boolean snapshot of the auth store — used as a guard before
   *  load(). The page passes () => !!$auth so the read stays
   *  reactive in the calling context. */
  isAuthed: () => boolean;
}

export interface DeadlinesDataController {
  deadlines: Deadline[];
  goals: Goal[];
  projects: Project[];
  ventures: Venture[];

  /** Lazy pool of open tasks — populated by ensureTasksLoaded() the
   *  first time the drawer opens, then reused across subsequent
   *  drawer sessions. Keeps the page paint fast on big vaults. */
  openTasks: Task[];
  tasksLoaded: boolean;

  /** True while load() is in flight. Drives the skeleton + spinner. */
  loading: boolean;
  /** True while a write (save / remove / quick-action) is in flight.
   *  Drives the drawer Save-button disabled state. */
  busy: boolean;

  /** Fetch deadlines + linked context (goals, projects, ventures) in
   *  parallel. Failures of the secondary calls don't block the
   *  deadlines list itself. */
  load(): Promise<void>;
  /** Lazy-populate openTasks the first time we need it. Idempotent —
   *  subsequent calls are no-ops once tasksLoaded flips true. */
  ensureTasksLoaded(): Promise<void>;
}

export function createDeadlinesData(deps: DeadlinesDataDeps): DeadlinesDataController {
  let deadlines = $state<Deadline[]>([]);
  let goals = $state<Goal[]>([]);
  let projects = $state<Project[]>([]);
  let ventures = $state<Venture[]>([]);
  let openTasks = $state<Task[]>([]);
  let tasksLoaded = $state(false);
  let loading = $state(false);
  let busy = $state(false);

  async function load() {
    if (!deps.isAuthed()) return;
    loading = true;
    try {
      const [dl, gl, pl, vl] = await Promise.all([
        api.listDeadlines(),
        api.listGoals().catch(() => ({ goals: [] as Goal[], total: 0 })),
        api.listProjects().catch(() => ({ projects: [] as Project[], total: 0 })),
        api.listVentures().catch(() => ({ ventures: [] as Venture[], total: 0 }))
      ]);
      deadlines = dl.deadlines;
      goals = gl.goals;
      projects = pl.projects;
      ventures = vl.ventures;
    } catch (e) {
      toast.error('load failed: ' + errorMessage(e));
    } finally {
      loading = false;
    }
  }

  async function ensureTasksLoaded() {
    if (tasksLoaded) return;
    try {
      const r = await api.listTasks({ status: 'open' });
      openTasks = r.tasks;
    } catch {
      openTasks = [];
    } finally {
      tasksLoaded = true;
    }
  }

  return {
    get deadlines() {
      return deadlines;
    },
    set deadlines(v) {
      deadlines = v;
    },
    get goals() {
      return goals;
    },
    set goals(v) {
      goals = v;
    },
    get projects() {
      return projects;
    },
    set projects(v) {
      projects = v;
    },
    get ventures() {
      return ventures;
    },
    set ventures(v) {
      ventures = v;
    },
    get openTasks() {
      return openTasks;
    },
    set openTasks(v) {
      openTasks = v;
    },
    get tasksLoaded() {
      return tasksLoaded;
    },
    set tasksLoaded(v) {
      tasksLoaded = v;
    },
    get loading() {
      return loading;
    },
    set loading(v) {
      loading = v;
    },
    get busy() {
      return busy;
    },
    set busy(v) {
      busy = v;
    },
    load,
    ensureTasksLoaded
  };
}
