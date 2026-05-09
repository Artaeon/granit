// Pinned nav items in the sidebar. Per-device localStorage —
// pinning is a personal layout preference, not state worth a
// backend round-trip. Pins are stored as the route href so the
// resolver can match them against the existing NavItem list and
// skip any that have been removed (module disabled, route gone).
//
// Reorder: pins keep insertion order, so the most-recent pin is
// at the bottom. We don't expose an explicit reorder gesture yet
// — the pin list usually stays small enough that order doesn't
// matter, and a drag handle would clutter the rail.

import { get } from 'svelte/store';
import { persistedWritable } from '$lib/util/persistedWritable';

const KEY = 'granit.sidebar.pinned';

export const sidebarPins = persistedWritable<string[]>(KEY, [], {
  validate: (raw) => {
    if (!Array.isArray(raw)) return [];
    return raw.filter((s): s is string => typeof s === 'string');
  }
});

export function togglePin(href: string) {
  sidebarPins.update((cur) => {
    if (cur.includes(href)) return cur.filter((h) => h !== href);
    return [...cur, href];
  });
}

export function isPinned(href: string): boolean {
  return get(sidebarPins).includes(href);
}
