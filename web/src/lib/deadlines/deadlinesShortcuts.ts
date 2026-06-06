// Keyboard shortcut handler for the deadlines page.
//
// Seventh extraction step out of routes/deadlines/+page.svelte. Owns
// the global single-letter binds + the search-box Escape intercept.
// The handler is a pure builder — it returns two functions the page
// wires to <svelte:window on:keydown> and the search input — so the
// 60-LOC switch tree stops crowding the page script.
//
//   n        new deadline (delegates to deps.openCreate)
//   /        focus the title-search box
//   1/2/3    toggle Critical / High / Normal chip
//   v        cycle view mode (list → timeline → calendar → list)
//   g        cycle group-by (urgency → status → month → urgency)
//   ?        toggle the shortcuts-help popover
//   esc      clear filter, or close popover, or blur search
//
// We bail when the drawer is open or focus is inside any input,
// textarea or select — global single-letter binds otherwise eat
// typing. The search box has its own key handler to intercept Esc
// before it reaches us.

import type { DeadlineImportance } from '$lib/api';

export interface DeadlinesShortcutsDeps {
  /** True while the create/edit drawer is mounted — suppress all
   *  global binds in that case so typing in drawer inputs flows
   *  normally. */
  isDrawerOpen: () => boolean;
  /** True iff either of the user-driven filter dimensions is active.
   *  Drives the Esc-clears-filters branch. */
  hasActiveFilter: () => boolean;
  /** Shortcuts-help popover open state — paired getter/setter so the
   *  ? toggle can flip it and the Esc handler can close it. */
  getShortcutsOpen: () => boolean;
  setShortcutsOpen: (v: boolean) => void;
  /** Focusable input ref — / focuses it, search-Esc blurs via
   *  e.target. Page owns the <input> bind. */
  getSearchEl: () => HTMLInputElement | null;
  /** Drawer-open + chip-toggle + cycle delegates. Each one is a
   *  single line at the call site, but routing them through deps
   *  lets the page own state without the handler having to import
   *  three controllers. */
  openCreate: () => void;
  setFilter: (v: DeadlineImportance | null) => void;
  cycleView: () => void;
  cycleGroup: () => void;
  clearFilters: () => void;
  /** Search-Esc setter — blurs + clears in one go. */
  clearSearch: () => void;
}

export interface DeadlinesShortcutsController {
  onPageKey(e: KeyboardEvent): void;
  onSearchKey(e: KeyboardEvent): void;
}

export function createDeadlinesShortcuts(
  deps: DeadlinesShortcutsDeps
): DeadlinesShortcutsController {
  function onPageKey(e: KeyboardEvent) {
    if (deps.isDrawerOpen()) return;
    const t = e.target as HTMLElement | null;
    if (
      t &&
      (t.tagName === 'INPUT' ||
        t.tagName === 'TEXTAREA' ||
        t.tagName === 'SELECT' ||
        t.isContentEditable)
    ) {
      return;
    }
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    switch (e.key) {
      case '?':
        e.preventDefault();
        deps.setShortcutsOpen(!deps.getShortcutsOpen());
        break;
      case 'n':
        e.preventDefault();
        deps.openCreate();
        break;
      case '/':
        e.preventDefault();
        deps.getSearchEl()?.focus();
        deps.getSearchEl()?.select();
        break;
      case '1':
        e.preventDefault();
        deps.setFilter('critical');
        break;
      case '2':
        e.preventDefault();
        deps.setFilter('high');
        break;
      case '3':
        e.preventDefault();
        deps.setFilter('normal');
        break;
      case 'v':
        e.preventDefault();
        deps.cycleView();
        break;
      case 'g':
        e.preventDefault();
        deps.cycleGroup();
        break;
      case 'Escape':
        if (deps.getShortcutsOpen()) {
          e.preventDefault();
          deps.setShortcutsOpen(false);
        } else if (deps.hasActiveFilter()) {
          e.preventDefault();
          deps.clearFilters();
        }
        break;
    }
  }

  function onSearchKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      (e.target as HTMLInputElement).blur();
      deps.clearSearch();
    }
  }

  return { onPageKey, onSearchKey };
}
