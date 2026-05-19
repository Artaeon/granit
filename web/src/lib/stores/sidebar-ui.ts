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
// Default-collapse everything except the Primary section (Heute /
// Projekte / Kalender / Rhythmus / Review — the Rhythmus-OS surface
// from 2026-05-19). The four secondary sections start collapsed so
// the sidebar reads as five primary tabs first, with the rest of
// the 27 routes available behind a tap. Existing users keep their
// stored preference because the persisted record overrides this
// default on read.

import { writable } from 'svelte/store';
import { loadStored, loadStoredString, saveStored, saveStoredString } from '$lib/util/storage';

const COLLAPSED_KEY = 'granit.sidebar.collapsed';
const COMPACT_KEY = 'granit.sidebar.compact';

const DEFAULT_COLLAPSED: Record<string, boolean> = {
  daily: true,
  plan: true,
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
