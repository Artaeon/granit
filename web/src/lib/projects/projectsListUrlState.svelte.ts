// URL-driven view state for the projects LIST page.
//
// Fifth extraction step out of routes/projects/+page.svelte. The
// projects list pushes every persisted user choice through the URL
// — which project is selected (?p=), which view mode is active
// (?view=list|kanban|timeline|heatmap), whether the dashboard
// overlay is open (?dashboard=1), and which venture filter is
// scoping the list (?venture=). The page is bookmarkable: a shared
// link round-trips the same view.
//
// This controller centralizes the read side (derives from a
// search-params getter) and the write side (URLSearchParams mutation
// + goto). The page injects $page + goto so the controller stays
// free of $app/* imports and can be tested with a stub navigate
// function.
//
// Helpers preserve the original switching semantics: selectProject
// also clears ?dashboard so a different project doesn't keep the
// previous project's dashboard mounted; openDashboard no-ops without
// a selected project; setViewMode strips ?view when going back to
// the default 'list' mode so canonical URLs stay clean.

import { type Project } from '$lib/api';

export type ViewMode = 'list' | 'kanban' | 'timeline' | 'heatmap';

export interface ProjectsListUrlStateDeps {
  /** Latest URLSearchParams from $page.url. The page wraps the
   *  $page store read here so the controller stays oblivious to
   *  $app/stores. */
  getSearchParams: () => URLSearchParams;
  /** Loaded projects — used to resolve selectedName back to a full
   *  Project record for the detail pane. */
  getProjects: () => Project[];
  /** SvelteKit navigation, wrapped to keep the controller free of
   *  $app/navigation. The page passes goto directly. */
  navigate: (url: string, opts?: { replaceState?: boolean; keepFocus?: boolean }) => void;
}

export interface ProjectsListUrlStateController {
  /** Currently selected project name from ?p=. Empty when nothing
   *  is selected. */
  readonly selectedName: string;
  /** Project record matching selectedName, or null when the name
   *  doesn't resolve (e.g. stale URL after a delete). */
  readonly selected: Project | null;
  /** Active view mode. Defaults to 'list' for any missing/unknown
   *  ?view value so a broken link still renders something usable. */
  readonly viewMode: ViewMode;
  /** Active venture scope from ?venture=. Empty when unscoped. */
  readonly ventureFilter: string;
  /** True when ?dashboard=1 is present. The dashboard overlay
   *  sits above whichever view is active without unmounting it. */
  readonly dashboardOpen: boolean;

  /** Set ?p= and (always) clear ?dashboard. Pass '' to deselect. */
  selectProject(name: string): void;
  /** Set ?view=. Strips the param for the default 'list' mode so
   *  canonical URLs stay clean. */
  setViewMode(v: ViewMode): void;
  /** Add ?dashboard=1. No-op when no project is selected — the
   *  dashboard needs a target. */
  openDashboard(): void;
  /** Drop ?dashboard. Leaves the rest of the URL alone. */
  closeDashboard(): void;
  /** Drop ?venture. Used by both the sidebar chip and the toolbar
   *  chip in chart views. */
  clearVentureFilter(): void;
}

export function createProjectsListUrlState(
  deps: ProjectsListUrlStateDeps
): ProjectsListUrlStateController {
  const selectedName = $derived(deps.getSearchParams().get('p') ?? '');
  const selected = $derived(
    deps.getProjects().find((p) => p.name === selectedName) ?? null
  );
  const viewMode = $derived<ViewMode>(
    (() => {
      const v = deps.getSearchParams().get('view');
      if (v === 'kanban' || v === 'timeline' || v === 'heatmap') return v;
      return 'list';
    })()
  );
  const ventureFilter = $derived(deps.getSearchParams().get('venture') ?? '');
  const dashboardOpen = $derived(deps.getSearchParams().get('dashboard') === '1');

  function selectProject(name: string) {
    const params = new URLSearchParams(deps.getSearchParams());
    if (name) params.set('p', name);
    else params.delete('p');
    // Closing or switching the project also closes the dashboard
    // overlay — a different project shouldn't keep the prior
    // project's dashboard mounted in the background.
    params.delete('dashboard');
    deps.navigate(`/projects?${params.toString()}`, { replaceState: true, keepFocus: true });
  }

  function setViewMode(v: ViewMode) {
    const params = new URLSearchParams(deps.getSearchParams());
    if (v === 'list') params.delete('view');
    else params.set('view', v);
    deps.navigate(`/projects?${params.toString()}`, { replaceState: true, keepFocus: true });
  }

  function openDashboard() {
    if (!selectedName) return;
    const params = new URLSearchParams(deps.getSearchParams());
    params.set('dashboard', '1');
    deps.navigate(`/projects?${params.toString()}`, { replaceState: true, keepFocus: true });
  }

  function closeDashboard() {
    const params = new URLSearchParams(deps.getSearchParams());
    params.delete('dashboard');
    deps.navigate(`/projects?${params.toString()}`, { replaceState: true, keepFocus: true });
  }

  function clearVentureFilter() {
    const params = new URLSearchParams(deps.getSearchParams());
    params.delete('venture');
    deps.navigate(`/projects?${params.toString()}`, { replaceState: true, keepFocus: true });
  }

  return {
    get selectedName() {
      return selectedName;
    },
    get selected() {
      return selected;
    },
    get viewMode() {
      return viewMode;
    },
    get ventureFilter() {
      return ventureFilter;
    },
    get dashboardOpen() {
      return dashboardOpen;
    },
    selectProject,
    setViewMode,
    openDashboard,
    closeDashboard,
    clearVentureFilter
  };
}
