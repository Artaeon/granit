// Multi-tab Phase 2 — Obsidian-style tabs in the main pane.
//
// Phase 1 (right pane companion sidebar) gave the user a stable
// "second surface" to keep an auxiliary view alongside the current
// route. Phase 2 layers tabs INSIDE the main pane so multiple routes
// can stay open simultaneously and the user can flip between them
// like browser tabs without leaving the SPA. Phase 3 will add
// drag-to-split + workspace presets.
//
// Tab model is intentionally thin: just the URL the tab is showing,
// a human-readable title (derived from the nav config / page title),
// and the last known scroll position so flipping back doesn't reset
// the view to the top. State that's already in the URL (filters,
// search, view mode) preserves itself; non-URL state (e.g. open
// drawer, picker selection) is deferred to Phase 3 — preserving it
// would require teaching every page to serialise its local state,
// which isn't viable for a single phase.
//
// Persistence is via localStorage under a single key. A corrupt
// payload falls back to an empty tabs array rather than throwing
// — the user just sees a fresh shell on next load instead of a
// broken layout. The strip is hidden until the user explicitly
// opens a second tab (or runs Mod+T) so existing single-route
// flows pay no visual cost.

import { writable, get } from 'svelte/store';

export interface Tab {
  /** crypto.randomUUID() — stable per-tab id, distinct from URL so
   *  duplicating a URL into multiple tabs stays unambiguous (Phase
   *  3 use-case, but the id is allocated now). */
  id: string;
  /** Pathname + search the tab is rendering, e.g. '/tasks?view=today'. */
  url: string;
  /** Display label. Derived from the nav config on creation; the
   *  layout updates it on navigation when the active-nav label
   *  changes. */
  title: string;
  /** Main-pane scrollTop the tab had when it was last active. The
   *  layout captures this on tab-switch (LEAVING) and restores it
   *  after the next route render (ENTERING). */
  scrollTop: number;
}

export interface TabsState {
  tabs: Tab[];
  activeTabId: string | null;
}

const KEY = 'granit.tabs';

function loadInitial(): TabsState {
  if (typeof localStorage === 'undefined') return { tabs: [], activeTabId: null };
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return { tabs: [], activeTabId: null };
    const parsed = JSON.parse(raw);
    if (!parsed || !Array.isArray(parsed.tabs)) return { tabs: [], activeTabId: null };
    // Sanitize each tab — drop entries missing required fields rather
    // than rendering an empty tab pill.
    const tabs: Tab[] = parsed.tabs.filter(
      (t: unknown): t is Tab =>
        !!t &&
        typeof (t as Tab).id === 'string' &&
        typeof (t as Tab).url === 'string' &&
        typeof (t as Tab).title === 'string'
    );
    const activeTabId: string | null =
      typeof parsed.activeTabId === 'string' &&
      tabs.some((t) => t.id === parsed.activeTabId)
        ? parsed.activeTabId
        : tabs[0]?.id ?? null;
    return { tabs, activeTabId };
  } catch {
    return { tabs: [], activeTabId: null };
  }
}

function persist(state: TabsState): void {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(KEY, JSON.stringify(state));
  } catch {
    // localStorage may be full / disabled — fail silent, tabs
    // already work in-memory for the session.
  }
}

export const tabsStore = writable<TabsState>(loadInitial());

tabsStore.subscribe(persist);

// Tiny UUID fallback — crypto.randomUUID is unavailable on a few
// older / non-secure-context browsers; falls back to Math.random
// which is fine for an opaque local id (no collision sensitivity
// across users).
function makeId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return 't-' + Math.random().toString(36).slice(2, 10) + Date.now().toString(36);
}

// ----- helpers ---------------------------------------------------------

/** Find a tab for this URL; if none, create one and activate it.
 *  Returns the tab id. */
export function ensureTab(url: string, title: string): string {
  let result = '';
  tabsStore.update((s) => {
    const existing = s.tabs.find((t) => t.url === url);
    if (existing) {
      result = existing.id;
      return s.activeTabId === existing.id ? s : { ...s, activeTabId: existing.id };
    }
    const id = makeId();
    const tab: Tab = { id, url, title, scrollTop: 0 };
    result = id;
    return { tabs: [...s.tabs, tab], activeTabId: id };
  });
  return result;
}

/** Update the active tab's URL + title in place. Bootstraps a tab
 *  if none exists yet — first-navigation case. Treat this like the
 *  browser's address bar: typing a new URL in the active tab
 *  replaces what it was showing without creating a duplicate. */
export function setActiveTabUrl(url: string, title: string): void {
  tabsStore.update((s) => {
    if (s.activeTabId) {
      const tabs = s.tabs.map((t) =>
        t.id === s.activeTabId ? { ...t, url, title, scrollTop: 0 } : t
      );
      return { ...s, tabs };
    }
    const id = makeId();
    return { tabs: [{ id, url, title, scrollTop: 0 }], activeTabId: id };
  });
}

/** Open a brand-new tab after the active one and activate it.
 *  Returns the new tab's id. */
export function newTab(url: string, title: string): string {
  let result = '';
  tabsStore.update((s) => {
    const id = makeId();
    const tab: Tab = { id, url, title, scrollTop: 0 };
    const tabs = [...s.tabs];
    const activeIdx = tabs.findIndex((t) => t.id === s.activeTabId);
    if (activeIdx >= 0) tabs.splice(activeIdx + 1, 0, tab);
    else tabs.push(tab);
    result = id;
    return { tabs, activeTabId: id };
  });
  return result;
}

/** Remove a tab. If it was active, picks a neighbour (prefers right)
 *  and returns the URL the layout should navigate to. Returns
 *  `nextUrl: null` when the closed tab wasn't active (no nav needed)
 *  and `nextUrl: '/'` when there are no tabs left (go home). */
export function closeTab(id: string): { nextUrl: string | null } {
  let nextUrl: string | null = null;
  let activeClosed = false;
  tabsStore.update((s) => {
    const idx = s.tabs.findIndex((t) => t.id === id);
    if (idx < 0) return s;
    const tabs = s.tabs.filter((t) => t.id !== id);
    let activeTabId = s.activeTabId;
    if (activeTabId === id) {
      activeClosed = true;
      const neighbor = s.tabs[idx + 1] || s.tabs[idx - 1] || null;
      activeTabId = neighbor?.id ?? null;
      // No tabs left → go to the workspace home (NOT '/', which redirects
      // there anyway and would trip the nav effect into bootstrapping a
      // fresh tab — the "can't close the last tab" respawn).
      nextUrl = neighbor?.url ?? '/workspace';
    }
    return { tabs, activeTabId };
  });
  // Caller only needs nextUrl when the active tab was the one closed.
  return { nextUrl: activeClosed ? nextUrl : null };
}

/** Activate a tab by id. Returns its URL (so the layout can
 *  navigate) or null if the id is unknown. */
export function activateTab(id: string): string | null {
  let url: string | null = null;
  tabsStore.update((s) => {
    const tab = s.tabs.find((t) => t.id === id);
    if (!tab) return s;
    url = tab.url;
    return s.activeTabId === id ? s : { ...s, activeTabId: id };
  });
  return url;
}

/** Cycle to the next/prev tab. direction=1 forward, -1 backward.
 *  Returns the new active tab's URL, or null if there are no tabs. */
export function cycleTab(direction: 1 | -1): string | null {
  const s = get(tabsStore);
  if (s.tabs.length === 0) return null;
  const idx = s.tabs.findIndex((t) => t.id === s.activeTabId);
  const startIdx = idx < 0 ? 0 : idx;
  const nextIdx = (startIdx + direction + s.tabs.length) % s.tabs.length;
  return activateTab(s.tabs[nextIdx].id);
}

/** Activate the Nth tab (1-indexed, for Mod+1..9). Returns the
 *  URL the layout should navigate to, or null if N is out of range. */
export function activateNth(n: number): string | null {
  const s = get(tabsStore);
  const tab = s.tabs[n - 1];
  if (!tab) return null;
  return activateTab(tab.id);
}

/** Capture the current scroll position into the active tab so a
 *  later switch can restore it. No-op when there's no active tab. */
export function setActiveScroll(scrollTop: number): void {
  tabsStore.update((s) => {
    if (!s.activeTabId) return s;
    let changed = false;
    const tabs = s.tabs.map((t) => {
      if (t.id !== s.activeTabId) return t;
      if (t.scrollTop === scrollTop) return t;
      changed = true;
      return { ...t, scrollTop };
    });
    return changed ? { ...s, tabs } : s;
  });
}

/** Replace every tab + active id. Used by Mod+W on the last tab to
 *  reset state so the strip hides again on the next load. */
export function clearTabs(): void {
  tabsStore.set({ tabs: [], activeTabId: null });
}
