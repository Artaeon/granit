// Linkable-entity lookup for the TaskDetail drawer.
//
// The Project / Goal / Deadline <select> dropdowns need the full lists
// so the user can pick from them. Loaded lazily on the first drawer
// open per session — the list pages don't pay the lookup cost on
// every card render, and once cached we reuse the same arrays for
// every subsequent task the user clicks. Reset semantics live in the
// parent's per-task $effect (which sets linksLoaded back to false on
// component mount); within a session the cache is sticky on purpose.
//
// Failures degrade silently to empty lists — the <select> just shows
// "(none)" rather than blocking the drawer behind a fetch error.

import { api, type Project, type Goal, type Deadline } from '$lib/api';

export interface TaskDetailLinksController {
  readonly projects: Project[];
  readonly goals: Goal[];
  readonly deadlines: Deadline[];
  /** Idempotent — no-ops after the first successful call. */
  load(): Promise<void>;
}

export function createTaskDetailLinks(): TaskDetailLinksController {
  let projects = $state<Project[]>([]);
  let goals = $state<Goal[]>([]);
  let deadlines = $state<Deadline[]>([]);
  let loaded = $state(false);
  // inflight gates concurrent load() calls (template + parent both
  // call); `loaded` only flips true once at least one slice
  // succeeded, so a cold-network total failure stays retryable
  // when the drawer next opens.
  let inflight = false;

  async function load() {
    if (loaded || inflight) return;
    inflight = true;
    try {
      // Three independent reads — settle in parallel and degrade
      // silently on per-list failure rather than blocking the drawer.
      const [pp, gg, dd] = await Promise.allSettled([
        api.listProjects(),
        api.listGoals(),
        api.listDeadlines()
      ]);
      if (pp.status === 'fulfilled') projects = pp.value.projects;
      if (gg.status === 'fulfilled') goals = gg.value.goals;
      if (dd.status === 'fulfilled') deadlines = dd.value.deadlines;
      // At least one slice landed → consider this load successful so
      // we don't refetch on every drawer open. If all three rejected
      // (likely the user is offline / backend down), leave loaded
      // false so the next open retries.
      if (pp.status === 'fulfilled' || gg.status === 'fulfilled' || dd.status === 'fulfilled') {
        loaded = true;
      }
    } finally {
      inflight = false;
    }
  }

  return {
    get projects() { return projects; },
    get goals() { return goals; },
    get deadlines() { return deadlines; },
    load
  };
}
