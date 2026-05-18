// Derived store for "which NavItem matches the current route".
// Longest-href wins so a sub-route (e.g. /notes/graph) doesn't
// resolve to its parent (/notes) just because the array order puts
// the parent first. Without this, the mobile header label and the
// active-pill highlight would both stick on the parent when the
// user is actually on the child route.

import { derived } from 'svelte/store';
import { page } from '$app/stores';
import { nav, type NavItem } from './config';

export const activeNav = derived(page, ($page) => {
  const pathname = $page.url.pathname;
  let best: NavItem | undefined;
  for (const n of nav) {
    if (n.href === pathname || (n.href !== '/' && pathname.startsWith(n.href + '/'))) {
      if (!best || n.href.length > best.href.length) best = n;
    }
  }
  return best;
});
