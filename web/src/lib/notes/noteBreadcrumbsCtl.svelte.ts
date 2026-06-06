// Folder-breadcrumb state cluster for the notes editor header.
//
// Wraps the breadcrumbExpanded toggle + the three breadcrumbs
// derivations (allCrumbs / visibleCrumbs / crumbsCollapsed) in one
// controller. The pure derivation + collapse rule lives in
// $lib/notes/noteBreadcrumbs; this surface just threads them through
// the page's reactive graph and exposes a `reset()` the route's
// load() hook calls on real navigation.

import {
  noteCrumbs,
  visibleCrumbs as visibleCrumbsFn,
  crumbsCollapsed as crumbsCollapsedFn,
  type Crumb
} from '$lib/notes/noteBreadcrumbs';
import type { Note } from '$lib/api';

export interface NoteBreadcrumbsController {
  readonly allCrumbs: Crumb[];
  readonly visibleCrumbs: Crumb[];
  readonly crumbsCollapsed: boolean;
  readonly expanded: boolean;
  expand: () => void;
  reset: () => void;
}

export interface NoteBreadcrumbsOpts {
  getNote: () => Note | null;
}

export function createNoteBreadcrumbsCtl(
  opts: NoteBreadcrumbsOpts
): NoteBreadcrumbsController {
  let expanded = $state(false);
  const allCrumbs = $derived(noteCrumbs(opts.getNote()?.path));
  const visibleCrumbs = $derived(visibleCrumbsFn(allCrumbs, expanded));
  const crumbsCollapsed = $derived(crumbsCollapsedFn(allCrumbs, expanded));

  return {
    get allCrumbs() { return allCrumbs; },
    get visibleCrumbs() { return visibleCrumbs; },
    get crumbsCollapsed() { return crumbsCollapsed; },
    get expanded() { return expanded; },
    expand: () => { expanded = true; },
    reset: () => { expanded = false; }
  };
}
