// Loaded data + loaders for the ProjectDetail surface.
//
// Three independent fetches, all keyed off the project record passed
// in via the deps getter:
//
//   • projectTasks       — every task whose `projectId` matches the
//                          project name OR whose `notePath` starts
//                          with the project's folder. Mirrors the
//                          server's projectView decoration so the
//                          drawer's task counts match the project
//                          list page.
//   • linkedGoals        — top-level goals (.granit/goals.json) whose
//                          `project` field matches. Read-only here;
//                          edits happen on /goals.
//   • projectVision      — per-project vision doc from the central
//                          /vision catalogue, keyed
//                          `project:<slug>` (stable across renames).
//                          404 = no vision yet, render the "anlegen"
//                          CTA — don't toast.
//
// Each fetch handles its own error path so a single endpoint
// failure (goals 500, vision down) doesn't break the rest of the
// drawer.

import { api, type Goal, type Project, type Task, type VisionDoc } from '$lib/api';
import { goto } from '$app/navigation';
import { errorMessage } from '$lib/util/errorMessage';
import { toast } from '$lib/components/toast';
import { slugifyTitle } from '$lib/util/slug';

export interface ProjectDetailDataController {
  // Loaded slices
  readonly projectTasks: Task[];
  readonly linkedGoals: Goal[];
  readonly projectVision: VisionDoc | null;

  // Loading flags surfaced to the template
  readonly loadingTasks: boolean;
  readonly projectVisionLoading: boolean;
  readonly projectVisionCreating: boolean;

  /** Stable vision key for the active project — `project:<slug>` so
   *  the central catalogue groups it next to Hauptvision/Mission. */
  readonly projectVisionKey: string;

  /** Fire all three loads in parallel. Called from the prop-watch
   *  $effect in the parent. */
  loadAll(): Promise<void>;
  /** Reload just the tasks (e.g. after a TaskRow patch). */
  loadTasks(): Promise<void>;
  /** Reload just the linked goals. */
  loadLinkedGoals(): Promise<void>;
  /** Reload just the vision doc (e.g. on WS state.changed for
   *  .granit/visions.json). */
  loadProjectVision(): Promise<void>;
  /** Create the project's vision doc + jump to /vision opened at
   *  that tab so the user can start writing. */
  createProjectVision(): Promise<void>;
}

export interface ProjectDetailDataDeps {
  getProject: () => Project;
}

export function createProjectDetailData(deps: ProjectDetailDataDeps): ProjectDetailDataController {
  let projectTasks = $state<Task[]>([]);
  let loadingTasks = $state(false);
  let linkedGoals = $state<Goal[]>([]);
  let projectVision = $state<VisionDoc | null>(null);
  let projectVisionLoading = $state(false);
  let projectVisionCreating = $state(false);

  const projectVisionKey = $derived(`project:${slugifyTitle(deps.getProject().name)}`);

  // Gen counters guard each loader against stale-response: when the
  // parent swaps project mid-fetch (master/detail list-click), the
  // OLD project's fetch must NOT overwrite the NEW project's slice.
  // Each loader stamps its in-flight fetch with the current gen;
  // only the latest gen is allowed to write back.
  let tasksGen = 0;
  let goalsGen = 0;
  let visionGen = 0;

  async function loadTasks() {
    const project = deps.getProject();
    const my = ++tasksGen;
    loadingTasks = true;
    try {
      const r = await api.listTasks({});
      if (my !== tasksGen) return;
      const folder = (project.folder ?? '').replace(/\/$/, '');
      projectTasks = r.tasks.filter((t) => {
        if (t.projectId === project.name) return true;
        if (folder && t.notePath.startsWith(folder + '/')) return true;
        return false;
      });
    } catch (e) {
      // eslint-disable-next-line no-console
      console.error(e);
    } finally {
      if (my === tasksGen) loadingTasks = false;
    }
  }

  async function loadLinkedGoals() {
    const project = deps.getProject();
    const my = ++goalsGen;
    try {
      const r = await api.listGoals();
      if (my !== goalsGen) return;
      linkedGoals = r.goals.filter((g) => g.project === project.name);
    } catch (e) {
      // Non-fatal — goals endpoint failure shouldn't break the
      // project page; just leave the section empty.
      // eslint-disable-next-line no-console
      console.error('listGoals', e);
    }
  }

  async function loadProjectVision() {
    const my = ++visionGen;
    projectVisionLoading = true;
    try {
      const doc = await api.getVisionDoc(projectVisionKey);
      if (my !== visionGen) return;
      projectVision = doc;
    } catch {
      if (my !== visionGen) return;
      projectVision = null;
    } finally {
      if (my === visionGen) projectVisionLoading = false;
    }
  }

  async function createProjectVision() {
    const project = deps.getProject();
    projectVisionCreating = true;
    try {
      await api.createVisionDoc({
        key: projectVisionKey,
        label: project.name
      });
      goto(`/vision?tab=${encodeURIComponent(projectVisionKey)}`);
    } catch (e) {
      toast.error('failed: ' + errorMessage(e));
    } finally {
      projectVisionCreating = false;
    }
  }

  async function loadAll() {
    await Promise.all([loadTasks(), loadLinkedGoals(), loadProjectVision()]);
  }

  return {
    get projectTasks() { return projectTasks; },
    get linkedGoals() { return linkedGoals; },
    get projectVision() { return projectVision; },
    get loadingTasks() { return loadingTasks; },
    get projectVisionLoading() { return projectVisionLoading; },
    get projectVisionCreating() { return projectVisionCreating; },
    get projectVisionKey() { return projectVisionKey; },
    loadAll,
    loadTasks,
    loadLinkedGoals,
    loadProjectVision,
    createProjectVision
  };
}
