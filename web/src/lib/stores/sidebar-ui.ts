// Per-device sidebar layout preferences. Three pieces of state:
//
//   compact          — icon-only rail vs expanded rail (md+ only;
//                      mobile drawer always renders full mode)
//   collapsedSections — record of section.id → true so the wire
//                      format stays tiny (only collapsed sections
//                      are stored)
//   hiddenSections    — record of section.id → true. Distinct from
//                      collapsed: a hidden section disappears from
//                      the sidebar entirely (no header, no items),
//                      not just folded shut. Used by the Settings
//                      "Sidebar Views" panel for the user who wants
//                      to drop a whole pillar (e.g. "I don't want
//                      AI in my nav at all"). Empty record = nothing
//                      hidden, all sections visible.
//
// All three used to live as $state inside +layout.svelte. Pulled
// into a shared store so the aside (which sets the width class)
// and the nav sidebar component (which renders the rows) consume
// the same source instead of passing state down and toggles back
// up.
//
// Default-expand the work pillars (Plan + Life) and collapse the
// reference pillars (Spiritual + Knowledge + AI). The earlier
// default collapsed everything which hid Finance behind a header
// — users on Today never saw it unless they remembered to click.
// Surfacing Plan + Life trades a slightly longer rail for
// discoverability of the user's primary work surfaces.
//
// Storage key is versioned. v3 carries the current default.
// v1 + v2 are best-effort cleaned up on first read so the
// localStorage view stays tidy. We DO NOT attempt to migrate
// old values into the new key — the previous attempt (v2's
// "preserve customizations from v1") left users on intermediate
// layouts stuck on whatever was written when an earlier commit
// set defaults that no longer apply. Defaults change rarely;
// bumping the version is the simplest correct mechanism.
//
// If a user wants to force the current default at any time:
//   localStorage.removeItem('granit.sidebar.collapsed.v3')
//   location.reload()

import { writable } from 'svelte/store';
import { loadStored, loadStoredString, saveStored, saveStoredString } from '$lib/util/storage';

const COLLAPSED_KEY = 'granit.sidebar.collapsed.v3';
const LEGACY_COLLAPSED_KEYS = [
  'granit.sidebar.collapsed', // v1
  'granit.sidebar.collapsed.v2'
];
const COMPACT_KEY = 'granit.sidebar.compact';
const HIDDEN_KEY = 'granit.sidebar.hidden';

const DEFAULT_COLLAPSED: Record<string, boolean> = {
  spiritual: true,
  knowledge: true,
  ai: true
};

function dropLegacyKeys(): void {
  if (typeof window === 'undefined') return;
  for (const k of LEGACY_COLLAPSED_KEYS) {
    try {
      window.localStorage.removeItem(k);
    } catch {
      // Quota / private-mode failures — harmless; the keys just
      // linger as unused JSON.
    }
  }
}

function loadCollapsed(): Record<string, boolean> {
  if (typeof window !== 'undefined' && window.localStorage?.getItem(COLLAPSED_KEY) === null) {
    // Fresh install (or version bump). Write the current default
    // and clear out any stale legacy keys so we don't carry forward
    // intermediate-commit state.
    const fresh = { ...DEFAULT_COLLAPSED };
    saveStored(COLLAPSED_KEY, fresh);
    dropLegacyKeys();
    return fresh;
  }
  return loadStored<Record<string, boolean>>(COLLAPSED_KEY, { ...DEFAULT_COLLAPSED });
}

export const collapsedSections = writable<Record<string, boolean>>(loadCollapsed());

export function toggleSection(id: string): void {
  collapsedSections.update((cur) => {
    const next = { ...cur };
    if (next[id]) delete next[id];
    else next[id] = true;
    saveStored(COLLAPSED_KEY, next);
    return next;
  });
}

// Force-expand a section without persisting the change, so closing
// it again — and going elsewhere — restores the user's preference.
// Used by the auto-expand-active-section effect in the layout.
export function expandSectionTransient(id: string): void {
  collapsedSections.update((cur) => {
    if (!cur[id]) return cur;
    const next = { ...cur };
    next[id] = false;
    return next;
  });
}

export const sidebarCompact = writable<boolean>(loadStoredString(COMPACT_KEY, '0') === '1');

export function toggleSidebarCompact(): void {
  sidebarCompact.update((cur) => {
    const next = !cur;
    saveStoredString(COMPACT_KEY, next ? '1' : '0');
    return next;
  });
}

// Hidden sections — set by the Settings "Sidebar Views" panel. Empty
// record by default (everything visible). Same shape as collapsed
// but a different semantic — hidden = "don't render at all in the
// rail", whereas collapsed = "render the header, just fold items".
function loadHidden(): Record<string, boolean> {
  return loadStored<Record<string, boolean>>(HIDDEN_KEY, {});
}

export const hiddenSections = writable<Record<string, boolean>>(loadHidden());

export function setSectionHidden(id: string, hidden: boolean): void {
  hiddenSections.update((cur) => {
    const next = { ...cur };
    if (hidden) next[id] = true;
    else delete next[id];
    saveStored(HIDDEN_KEY, next);
    return next;
  });
}
