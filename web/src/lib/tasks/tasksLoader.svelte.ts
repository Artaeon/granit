// Tasks data loader.
//
// One job: fetch tasks + linkable-entity sidecars (projects / goals /
// deadlines) from the API into dataCtl, gated on every server-side
// filter the UI exposes. Re-fires whenever $auth or any server-pushed
// filter changes via an internal $effect.
//
// Why a controller (vs. a plain async in the parent):
//
//   • The filter-driven $effect has a tight dep list (only the filters
//     the server actually paginates on, not the full filter pipeline).
//     Inlined in TasksPane this was easy to drift; here it's the
//     factory's contract and nothing else touches it.
//
//   • The "skip the sidecar refetch when we already have them"
//     branches read dataCtl.X.length, then reassign dataCtl.X arrays.
//     Those reads have to stay OUTSIDE the effect's dep set (otherwise
//     Svelte 5 re-fires on $state array reassignment even when
//     contents are equal — tight-loops the page; symptom is a
//     saturating /api/v1/listTasks). The untrack() wrap inside the
//     effect is the load-bearing line; the explicit `void` list above
//     it is the SOURCE-OF-TRUTH for what should retrigger load.
//
// Errors swallow silently — 401 (stale auth) and network failures
// both land here. Leaving dataCtl.tasks/etc empty makes the
// empty-state copy render; a later WS reconnect or filter change
// retries naturally. No toast, no console noise.

import { untrack } from 'svelte';
import { api, type Project, type Goal, type Deadline } from '$lib/api';
import type { TasksFilterStateController } from './tasksFilterState.svelte';
import type { TasksDataController } from './tasksData.svelte';

export interface TasksLoaderDeps {
  /** Auth gate. The loader is a no-op while this returns false so the
   *  initial render doesn't fire a 401-bound fetch. */
  getAuth: () => unknown;
  filterCtl: TasksFilterStateController;
  dataCtl: TasksDataController;
}

export interface TasksLoaderController {
  /** Fetch tasks + sidecars into dataCtl. Idempotent in flight (a
   *  second call just stacks; the trailing reload coalesce in
   *  tasksLifecycle dedupes bursty triggers). */
  load(): Promise<void>;
}

export function createTasksLoader(deps: TasksLoaderDeps): TasksLoaderController {
  const { filterCtl, dataCtl } = deps;

  async function load() {
    if (!deps.getAuth()) return;
    dataCtl.loading = true;
    try {
      // Honor every server-side filter we expose. The client-side
      // filterCtl.filtered derivation still re-applies these (so the
      // view-specific logic like inbox/stale stays consistent), but
      // pushing them to the server first means we don't ship the
      // entire task graph over the wire when the user wants P1 only.
      const params: Parameters<typeof api.listTasks>[0] = {};
      if (filterCtl.status !== 'all') params.status = filterCtl.status;
      // The backend endpoint accepts a single tag; for multi-tag
      // filters we pass the first to narrow the server response and
      // AND-narrow the rest client-side in the filterCtl.filtered
      // derivation.
      if (filterCtl.tagFilters.length > 0) params.tag = filterCtl.tagFilters[0];
      if (filterCtl.priorityFilter !== '') params.priority = filterCtl.priorityFilter;
      if (filterCtl.projectFilter) params.project = filterCtl.projectFilter;
      if (filterCtl.goalFilter) params.goal = filterCtl.goalFilter;
      if (filterCtl.deadlineFilter) params.deadline = filterCtl.deadlineFilter;
      if (filterCtl.archivedMode === 'show') params.includeArchived = true;
      if (filterCtl.archivedMode === 'only') params.archived = true;
      const [list, p, gg, dd] = await Promise.all([
        api.listTasks(params),
        dataCtl.projects.length === 0
          ? api.listProjects().catch(() => ({ projects: [] as Project[] }))
          : Promise.resolve({ projects: dataCtl.projects }),
        dataCtl.goals.length === 0
          ? api.listGoals().catch(() => ({ goals: [] as Goal[] }))
          : Promise.resolve({ goals: dataCtl.goals }),
        dataCtl.deadlines.length === 0
          ? api.listDeadlines().catch(() => ({ deadlines: [] as Deadline[] }))
          : Promise.resolve({ deadlines: dataCtl.deadlines })
      ]);
      dataCtl.tasks = list.tasks;
      dataCtl.projects = p.projects;
      dataCtl.goals = gg.goals;
      dataCtl.deadlines = dd.deadlines;
    } catch {
      // 401 (stale auth) and network failures both end up here.
      // Silently leave dataCtl.tasks/dataCtl.projects empty so the
      // empty-state copy renders instead of the indefinite
      // dataCtl.loading spinner. A later WS reconnect or filter change
      // will retry naturally — no toast, no console noise.
    } finally {
      dataCtl.loading = false;
    }
  }

  // Single load driver. When auth resolves (or changes) it fires;
  // when status/tagFilters/etc. change it fires. We don't pair it
  // with onMount(load) — that would cause a double-fetch on initial
  // paint and (more importantly) was the source of the
  // "stays loading" bug when an early call set loading=true before
  // $auth was ready.
  //
  // load() is wrapped in untrack() because the function reads
  // dataCtl.projects.length / goals.length / deadlines.length to
  // decide whether to refetch the linkable-entity sidecars, and it
  // reassigns those arrays when fresh data lands. Without untrack,
  // those reads would become deps of THIS effect, and Svelte 5 fires
  // reactivity on $state array reassignment even when contents are
  // equal — turning a single initial fetch into a tight loop (most
  // visible when /api/v1/deadlines returns []: deadlines.length stays
  // 0, so every load() refires load(), saturating the page). The
  // explicit `void` list below is the source-of-truth for what should
  // retrigger load.
  $effect(() => {
    void deps.getAuth();
    void filterCtl.status;
    void filterCtl.tagFilters;
    void filterCtl.priorityFilter;
    void filterCtl.projectFilter;
    void filterCtl.goalFilter;
    void filterCtl.deadlineFilter;
    void filterCtl.archivedMode;
    untrack(() => load());
  });

  return { load };
}
