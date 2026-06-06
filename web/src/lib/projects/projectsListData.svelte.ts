// Data state for the projects LIST page.
//
// First extraction step out of routes/projects/+page.svelte. Owns the
// two arrays the list page works against — projects and tasks — plus
// the loading flag, the load() function with a stale-response guard,
// and the install helper that wires the WS event subscription and the
// visibility/focus refetch listeners.
//
// The list page pulls all tasks (not just per-project) so each card
// can render a 4-week sparkline and a per-status mini-bar without the
// detail panel having to be open. Two-call parallel load so the
// sparkline/this-week counts don't wait for projects to finish before
// tasks even start.
//
// A monotonic gen counter guards against late responses winning over
// a fresher load() — mirrors the projectDetailData pattern. Tab
// suspension on mobile (and desktop background tabs) drops WS events
// silently, so a visibilitychange/focus listener forces a refetch
// when the user comes back so we never present a stale list.
//
// Sticking to the controller-factory + install-helper split keeps the
// page free of the data-plumbing noise while leaving load() reachable
// from every caller that needs it (created/deleted callbacks, status
// changes, agent run-end, kanban drag-end).

import { onWsEvent } from '$lib/ws';
import { api, type Project, type Task } from '$lib/api';
import { toast } from '$lib/components/toast';

export interface ProjectsListDataController {
  /** Loaded projects sidecar. */
  projects: Project[];
  /** Loaded tasks sidecar — every task in the vault, so each list
   *  card can render its own momentum sparkline without a separate
   *  per-project fetch. */
  tasks: Task[];
  /** True while load() is in flight. Drives the "loading…" placeholder
   *  in the empty-state slot. */
  readonly loading: boolean;
  /** Fetch both sidecars in parallel. Safe to call concurrently —
   *  a stale-response guard drops out-of-order results. */
  load(): Promise<void>;
}

export function createProjectsListData(): ProjectsListDataController {
  let projects = $state<Project[]>([]);
  let tasks = $state<Task[]>([]);
  let loading = $state(false);
  // Monotonic gen counter — every load() bumps it; after each await
  // the closure compares its captured generation against the current
  // one and bails when a newer load() has already started. Without
  // this, a slow first load() can overwrite a fresh second one with
  // older data.
  let gen = 0;

  async function load() {
    const my = ++gen;
    loading = true;
    try {
      const [pr, tr] = await Promise.all([
        api.listProjects(),
        api.listTasks({}).catch(() => ({ tasks: [] as Task[], total: 0 }))
      ]);
      if (my !== gen) return;
      projects = pr.projects;
      tasks = tr.tasks ?? [];
    } catch (e) {
      if (my !== gen) return;
      toast.error('failed to load projects: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      if (my === gen) loading = false;
    }
  }

  return {
    get projects() {
      return projects;
    },
    set projects(v) {
      projects = v;
    },
    get tasks() {
      return tasks;
    },
    set tasks(v) {
      tasks = v;
    },
    get loading() {
      return loading;
    },
    load
  };
}

export interface ProjectsListLiveDeps {
  /** Triggered on every relevant WS event, on tab-visible, and on
   *  window focus. The page wraps load() in the closure so this
   *  helper stays free of API knowledge. */
  reload: () => void;
}

/**
 * Install the live-refresh listeners — WS project/note events plus the
 * tab-visible / window-focus refetch. Mobile browsers (and desktop
 * tabs in the background) suspend the WS so events fired while we
 * were away are simply lost; refetching when the tab becomes visible
 * again is cheap and means the user never wonders why a project they
 * created on another device isn't showing up.
 */
export function installProjectsListLive(deps: ProjectsListLiveDeps): () => void {
  const unsub = onWsEvent((ev) => {
    if (ev.type.startsWith('project.')) deps.reload();
    if (ev.type === 'note.changed' || ev.type === 'note.removed') deps.reload();
  });
  const onVisible = () => {
    if (document.visibilityState === 'visible') deps.reload();
  };
  document.addEventListener('visibilitychange', onVisible);
  window.addEventListener('focus', onVisible);
  return () => {
    unsub();
    document.removeEventListener('visibilitychange', onVisible);
    window.removeEventListener('focus', onVisible);
  };
}
