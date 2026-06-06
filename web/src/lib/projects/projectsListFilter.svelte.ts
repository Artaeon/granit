// Filter / search / sort / group state for the projects LIST page.
//
// Second extraction step out of routes/projects/+page.svelte. Owns
// the two locally-held filter dimensions (`q` — text search, and
// `statusFilter` — the active/paused/completed/archived/all status
// pill) plus every derivation that branches off of them: `ventures`
// (sidebar group headers + create-form autocomplete), `filtered`
// (the canonical project list once status + venture + search have
// applied), `kanbanFeed` (every status column always renders, so
// status filtering is skipped there), and `grouped` (filtered list
// broken into venture buckets).
//
// `ventureFilter` is URL-derived state held by the page — passed in
// via the deps bundle rather than owned here, so this controller
// stays unaware of $app/stores and is testable in isolation. Same
// for `projects`: read through a getter so reactivity propagates
// from the data controller without crossing module boundaries.

import { type Project } from '$lib/api';

export type StatusFilter = 'all' | 'active' | 'paused' | 'completed' | 'archived';

export interface ProjectsListFilterDeps {
  /** Loaded projects from projectsListData. Read via getter so the
   *  $state reactivity behind it propagates. */
  getProjects: () => Project[];
  /** URL-derived venture scope, or '' when unscoped. The page owns
   *  the URL plumbing so this controller stays free of $page. */
  getVentureFilter: () => string;
}

export interface ProjectsListFilterController {
  /** Free-text search across name / description / kind / venture /
   *  tags. Bindable. */
  q: string;
  /** Status pill selection. Bindable. */
  statusFilter: StatusFilter;

  /** Distinct venture names sorted alphabetically — used by the
   *  sidebar group headers and by ProjectCreate's autocomplete. */
  readonly ventures: string[];
  /** Status + venture + search applied; sorted by status tier →
   *  priority desc → name. The canonical "what to show" list. */
  readonly filtered: Project[];
  /** Kanban variant — skips statusFilter (every status column must
   *  render) but keeps venture + search. Sort matches `filtered`
   *  within each column so card positions are deterministic. */
  readonly kanbanFeed: Project[];
  /** filtered, bucketed by venture. When ventureFilter is active the
   *  page already conveys scope via a URL chip, so a single group
   *  is returned and the sidebar suppresses the headers. Projects
   *  without a venture land in a trailing 'Unassigned' bucket
   *  marked with '—' rather than scattering. */
  readonly grouped: { venture: string; projects: Project[] }[];
}

export function createProjectsListFilter(
  deps: ProjectsListFilterDeps
): ProjectsListFilterController {
  let q = $state('');
  let statusFilter = $state<StatusFilter>('active');

  const ventures = $derived.by<string[]>(() => {
    const projects = deps.getProjects();
    const set = new Set<string>();
    for (const p of projects) {
      const v = (p.venture ?? '').trim();
      if (v) set.add(v);
    }
    return [...set].sort((a, b) => a.localeCompare(b));
  });

  // Kanban feed: all four status columns must always render, so we
  // skip the statusFilter here — venture + search still apply.
  // The sort matches `filtered` so a project's column position is
  // deterministic across views.
  const kanbanFeed = $derived.by<Project[]>(() => {
    const projects = deps.getProjects();
    const ventureFilter = deps.getVentureFilter();
    let list = projects;
    if (ventureFilter) list = list.filter((p) => (p.venture ?? '') === ventureFilter);
    const term = q.trim().toLowerCase();
    if (term) {
      list = list.filter((p) =>
        p.name.toLowerCase().includes(term) ||
        (p.description ?? '').toLowerCase().includes(term) ||
        (p.tags ?? []).some((t) => t.toLowerCase().includes(term)) ||
        (p.kind ?? '').toLowerCase().includes(term) ||
        (p.venture ?? '').toLowerCase().includes(term)
      );
    }
    // Within a column: priority desc, then name. Status is encoded
    // by the column itself so no status tier needed here.
    return [...list].sort((a, b) => {
      const pa = a.priority ?? 0;
      const pb = b.priority ?? 0;
      if (pa !== pb) return pb - pa;
      return a.name.localeCompare(b.name);
    });
  });

  const filtered = $derived.by<Project[]>(() => {
    const projects = deps.getProjects();
    const ventureFilter = deps.getVentureFilter();
    let list = projects;
    if (statusFilter !== 'all') list = list.filter((p) => (p.status ?? 'active') === statusFilter);
    if (ventureFilter) list = list.filter((p) => (p.venture ?? '') === ventureFilter);
    const term = q.trim().toLowerCase();
    if (term) {
      list = list.filter((p) =>
        p.name.toLowerCase().includes(term) ||
        (p.description ?? '').toLowerCase().includes(term) ||
        (p.tags ?? []).some((t) => t.toLowerCase().includes(term)) ||
        (p.kind ?? '').toLowerCase().includes(term) ||
        (p.venture ?? '').toLowerCase().includes(term)
      );
    }
    // Sort: active first → priority desc → name
    return [...list].sort((a, b) => {
      const sa = a.status ?? 'active';
      const sb = b.status ?? 'active';
      if (sa !== sb) {
        const order = { active: 0, paused: 1, completed: 2, archived: 3 } as Record<string, number>;
        return (order[sa] ?? 9) - (order[sb] ?? 9);
      }
      const pa = a.priority ?? 0;
      const pb = b.priority ?? 0;
      if (pa !== pb) return pb - pa;
      return a.name.localeCompare(b.name);
    });
  });

  // Group filtered projects by venture, preserving the sort order above.
  // Projects without a venture land in a single 'Unassigned' group at the
  // end — having one named bucket is less noisy than scattering them.
  // When the user has explicitly filtered to a venture we skip the group
  // headers entirely (the URL chip already conveys the scope).
  const grouped = $derived.by<{ venture: string; projects: Project[] }[]>(() => {
    const ventureFilter = deps.getVentureFilter();
    if (ventureFilter) return [{ venture: ventureFilter, projects: filtered }];
    const map = new Map<string, Project[]>();
    for (const p of filtered) {
      const v = (p.venture ?? '').trim() || '—';
      const arr = map.get(v) ?? [];
      arr.push(p);
      map.set(v, arr);
    }
    const named: { venture: string; projects: Project[] }[] = [];
    let unassigned: { venture: string; projects: Project[] } | null = null;
    for (const [venture, list] of map) {
      const g = { venture, projects: list };
      if (venture === '—') unassigned = g;
      else named.push(g);
    }
    named.sort((a, b) => a.venture.localeCompare(b.venture));
    return unassigned ? [...named, unassigned] : named;
  });

  return {
    get q() {
      return q;
    },
    set q(v) {
      q = v;
    },
    get statusFilter() {
      return statusFilter;
    },
    set statusFilter(v) {
      statusFilter = v;
    },
    get ventures() {
      return ventures;
    },
    get filtered() {
      return filtered;
    },
    get kanbanFeed() {
      return kanbanFeed;
    },
    get grouped() {
      return grouped;
    }
  };
}
