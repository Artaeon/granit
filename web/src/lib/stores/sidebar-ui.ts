// Per-device sidebar layout preferences. Two pieces of state:
//
//   compact          — icon-only rail vs expanded rail (md+ only;
//                      mobile drawer always renders full mode)
//   collapsedSections — record of section.id → true so the wire
//                      format stays tiny (only collapsed sections
//                      are stored)
//
// Both used to live as $state inside +layout.svelte. Pulled into a
// shared store so the aside (which sets the width class) and the
// nav sidebar component (which renders the rows) consume the same
// source instead of passing state down and toggles back up.
//
// Default-collapse everything except Daily. The original default
// was "all expanded", which surfaced 25+ items at once and made
// the sidebar feel like a phonebook. Daily stays expanded because
// it's the morning/tasks/calendar cluster every user lives in. The
// others get expanded as needed and the choice persists. Existing
// users keep whatever they had.

import { writable } from 'svelte/store';
import { loadStored, loadStoredString, saveStored, saveStoredString } from '$lib/util/storage';

const COLLAPSED_KEY = 'granit.sidebar.collapsed';
const COMPACT_KEY = 'granit.sidebar.compact';

const DEFAULT_COLLAPSED: Record<string, boolean> = {
  plan: true,
  spiritual: true,
  life: true,
  knowledge: true,
  ai: true
};

function loadCollapsed(): Record<string, boolean> {
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
