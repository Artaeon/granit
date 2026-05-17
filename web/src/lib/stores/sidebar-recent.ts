// Recently-visited nav routes for the sidebar's "Recent" rail.
// Per-device localStorage — recency is a personal navigation
// signal, not state worth a backend round-trip.
//
// Storage shape: most-recent-first array of route hrefs (e.g.
// "/notes", "/calendar"). Bounded at MAX_RECENT so the list
// never grows unbounded; older entries fall off as new routes
// are visited. Each route appears at most once — re-visiting a
// route moves it to the front rather than duplicating.
//
// Pinned routes are intentionally NOT filtered here: the
// sidebar renders Pinned and Recent as separate rails, and
// downstream filtering (see layout) drops a Recent entry that's
// already pinned so the user doesn't see the same href twice.

import { persistedWritable } from '$lib/util/persistedWritable';

const KEY = 'granit.sidebar.recent';

// Cap the list short — a long recents rail recreates the
// scanning problem we're trying to avoid. 5 is enough to cover
// the user's last working session without cluttering the nav.
export const MAX_RECENT = 5;

export const sidebarRecent = persistedWritable<string[]>(KEY, [], {
  validate: (raw) => {
    if (!Array.isArray(raw)) return [];
    return raw.filter((s): s is string => typeof s === 'string').slice(0, MAX_RECENT);
  }
});

// Hrefs that should never enter the recent list. Today is always
// rendered above all groups already; auth/setup screens are not
// navigation targets; settings is the footer rail. Empty href
// guards against pages we haven't tagged.
const IGNORED = new Set<string>(['', '/', '/settings']);

// Record a route visit. Bumps the route to the front of the
// list, drops any prior occurrence, and trims to MAX_RECENT.
// Called from the layout's page-store subscription on every
// navigation.
export function recordVisit(href: string) {
  if (!href || IGNORED.has(href)) return;
  sidebarRecent.update((cur) => {
    const without = cur.filter((h) => h !== href);
    const next = [href, ...without];
    return next.slice(0, MAX_RECENT);
  });
}
