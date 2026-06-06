// Side-data loaders for EventDetail.svelte — projects + calendar
// sources. Both are read-only lookups that drive small UI bits:
//   - projects feed the project-link <select> in the edit form.
//   - sources feed the icsWritable derivation (an ICS event is
//     editable only when its source file is writable).
//
// Both used to be inline onMount handlers in the modal, but they're
// orthogonal to the edit / delete / save flows that dominate the
// file, so they live here and the modal calls one installer.

import { api, type CalendarSource, type Project } from '$lib/api';

export interface EventDetailLoadersController {
  /** Project list — empty when load failed or no projects exist. */
  readonly projects: Project[];
  /** Calendar source list — empty when load failed. */
  readonly sources: CalendarSource[];
  /** Re-run both loads. The modal currently doesn't refresh after
   *  mount, but exposing this keeps the door open for "I just added
   *  a project — show it in the picker" without a page reload. */
  reload(): Promise<void>;
}

/** Spin up the controller and kick off both loads. Callers invoke
 *  this from $effect / onMount so failures degrade silently to empty
 *  arrays (the picker hides itself when projects is empty; the
 *  icsWritable derivation falls back to event.editable). */
export function createEventDetailLoaders(): EventDetailLoadersController {
  let projects = $state<Project[]>([]);
  let sources = $state<CalendarSource[]>([]);

  async function loadProjects() {
    try {
      const r = await api.listProjects();
      projects = r.projects ?? [];
    } catch {
      projects = [];
    }
  }

  async function loadSources() {
    try {
      const r = await api.listCalendarSources();
      sources = r.sources;
    } catch {
      sources = [];
    }
  }

  async function reload() {
    await Promise.all([loadProjects(), loadSources()]);
  }

  // Fire-and-forget initial load. Components call createX() during
  // <script> init and the resulting promise reaches the microtask
  // queue immediately — by the time the user clicks 'edit', both
  // arrays are populated in the common case.
  void reload();

  return {
    get projects() { return projects; },
    get sources() { return sources; },
    reload
  };
}
