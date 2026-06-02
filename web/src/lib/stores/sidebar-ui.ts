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
// Storage key is versioned: v2 carries the new default. v1 is
// migrated on first read so users whose only state is the old
// "collapse everything" default get the new layout without losing
// any deliberate toggles. Users with a custom set keep what they
// had — migration is "default-equivalence detection", not blind
// overwrite.

import { writable } from 'svelte/store';
import { loadStored, loadStoredString, saveStored, saveStoredString } from '$lib/util/storage';

const COLLAPSED_KEY_V1 = 'granit.sidebar.collapsed';
const COLLAPSED_KEY = 'granit.sidebar.collapsed.v2';
const COMPACT_KEY = 'granit.sidebar.compact';
const HIDDEN_KEY = 'granit.sidebar.hidden';

const DEFAULT_COLLAPSED: Record<string, boolean> = {
  spiritual: true,
  knowledge: true,
  ai: true
};

// Old default before the v2 reorg. Used purely for migration —
// if the user's v1 state matches this exactly, they never
// customized; replace with the v2 default. Includes 'daily' even
// though the section is gone, since v1 users may have toggled it.
const OLD_DEFAULT_COLLAPSED: Record<string, boolean> = {
  plan: true,
  spiritual: true,
  life: true,
  knowledge: true,
  ai: true
};

function sameMap(a: Record<string, boolean>, b: Record<string, boolean>): boolean {
  const ak = Object.keys(a);
  const bk = Object.keys(b);
  if (ak.length !== bk.length) return false;
  for (const k of ak) if (a[k] !== b[k]) return false;
  return true;
}

function dropV1Key(): void {
  try {
    window.localStorage.removeItem(COLLAPSED_KEY_V1);
  } catch {
    // Quota / private-mode failures — harmless; v1 just lingers.
  }
}

function loadCollapsed(): Record<string, boolean> {
  if (typeof window !== 'undefined' && window.localStorage?.getItem(COLLAPSED_KEY) === null) {
    const v1 = window.localStorage.getItem(COLLAPSED_KEY_V1);
    if (v1) {
      try {
        const parsed = JSON.parse(v1) as Record<string, boolean>;
        // User customized — keep their map, but strip the dropped
        // 'daily' key since the section no longer exists.
        if (!sameMap(parsed, OLD_DEFAULT_COLLAPSED)) {
          delete parsed.daily;
          saveStored(COLLAPSED_KEY, parsed); // freeze v2 so we never re-migrate
          dropV1Key();
          return parsed;
        }
        // User had the exact old default — fall through to new default.
      } catch {
        // Bad JSON — fall through to new default.
      }
    }
    // Either no v1 key, v1 matched the old default, or v1 was unparseable.
    // Either way, write the new default to v2 and clean up v1 so this
    // migration block becomes dead code on the next boot.
    const fresh = { ...DEFAULT_COLLAPSED };
    saveStored(COLLAPSED_KEY, fresh);
    if (v1) dropV1Key();
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
