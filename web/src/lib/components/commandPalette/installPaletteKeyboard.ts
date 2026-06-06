// Global keyboard install for the command palette.
//
// Owns the single window-level keydown listener that drives the
// palette from anywhere in the app. Pulled out of CommandPalette.svelte
// so the component shell isn't 80+ lines of arrow-key plumbing on top
// of its $state declarations.
//
// Three responsibilities, in priority order:
//
//  1. Open / close — Mod-K and Mod-P both toggle the palette. The two
//     keybinds converged when the old "actions only" Mod-K stopped
//     earning its weight; we collapsed them rather than maintain two
//     surfaces. Mod-P preempts the browser print dialog globally;
//     PrintPreview's own Mod-P handler runs in the capture phase with
//     stopImmediatePropagation so the print overlay still wins when
//     it's the focused surface.
//
//  2. Mod-Shift-F escape hatch — jumps to /search for full-text deep
//     dives. The palette's Content section is the inline preview but
//     caps at 12 hits; for more, /search. Skipped when typing into an
//     input so the user can still type 'F' or use the browser's own
//     Cmd-Shift-F binding.
//
//  3. In-palette navigation — only fires when open is true. Arrow keys
//     move the cursor, Enter invokes, Esc closes, Tab / Shift-Tab jump
//     to the first item of the next / previous group (power gesture
//     for skipping a long Pages run into Tasks or Content). Tab has
//     no modifiers on purpose so it never collides with browser
//     tab-navigation (the palette swallows focus while open).
//
// Pattern follows installTasksKeyboard: refs object exposes parent
// state + actions as small callbacks, returns a cleanup. The component
// calls it from onMount and returns the result.

import { goto } from '$app/navigation';
import type { CmdItem, Group } from './paletteTypes';

export interface PaletteKeyboardRefs {
  // ── State reads ────────────────────────────────────────────────
  isOpen: () => boolean;
  /** Current cursor index in the flat items list. */
  getSelected: () => number;
  /** Mutator for the cursor index — accepts the new value. */
  setSelected: (n: number) => void;
  getItems: () => CmdItem[];
  getGrouped: () => { group: Group; items: CmdItem[] }[];

  // ── Actions ────────────────────────────────────────────────────
  open: () => void;
  close: () => void;
  invoke: (item: CmdItem | undefined) => void;
  /** Scroll the currently-selected row into view. The component
   *  reads the DOM via the data-cmd-idx attribute — we keep the
   *  query selector on the component side so this helper can stay
   *  DOM-agnostic. */
  scrollSelectedIntoView: () => void;
}

/** Install the window keydown listener. Call from onMount, return
 *  the result so the cleanup runs on unmount. */
export function installPaletteKeyboard(refs: PaletteKeyboardRefs): () => void {
  function onKey(e: KeyboardEvent) {
    const meta = e.metaKey || e.ctrlKey;
    if (meta && !e.shiftKey && (e.key === 'k' || e.key === 'K')) {
      e.preventDefault();
      if (refs.isOpen()) refs.close();
      else refs.open();
      return;
    }
    // Mod-P → same surface as Mod-K. Preempts the browser print
    // dialog globally; PrintPreview's own Mod-P handler runs in the
    // capture phase + stopImmediatePropagation so the print overlay
    // still wins when it's the focused surface.
    if (meta && !e.shiftKey && (e.key === 'p' || e.key === 'P')) {
      e.preventDefault();
      if (refs.isOpen()) refs.close();
      else refs.open();
      return;
    }
    // Mod-Shift-F → full-text search. Skip when typing into an input
    // so the user can still type 'F' or use the browser's
    // Cmd-Shift-F if they want it.
    if (meta && e.shiftKey && (e.key === 'f' || e.key === 'F')) {
      const el = document.activeElement as HTMLElement | null;
      const tag = el?.tagName?.toLowerCase();
      if (tag === 'input' || tag === 'textarea' || el?.isContentEditable) return;
      e.preventDefault();
      void goto('/search');
      return;
    }
    if (!refs.isOpen()) return;
    if (e.key === 'Escape') {
      e.preventDefault();
      refs.close();
      return;
    }
    const items = refs.getItems();
    const selected = refs.getSelected();
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      refs.setSelected(Math.min(items.length - 1, selected + 1));
      refs.scrollSelectedIntoView();
      return;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      refs.setSelected(Math.max(0, selected - 1));
      refs.scrollSelectedIntoView();
      return;
    }
    if (e.key === 'Enter') {
      e.preventDefault();
      refs.invoke(items[selected]);
      return;
    }
    // Tab / Shift-Tab — jump to the first item of the next / previous
    // group. Power gesture for hopping past a long Pages list into
    // Tasks or Content without arrow-spamming. Without modifiers so
    // it never collides with browser tab-navigation (the palette
    // swallows focus while open).
    if (e.key === 'Tab') {
      e.preventDefault();
      const grouped = refs.getGrouped();
      if (grouped.length === 0) return;
      // Find the current group index from `selected`.
      let acc = 0;
      let curGroup = 0;
      for (let i = 0; i < grouped.length; i++) {
        const end = acc + grouped[i].items.length;
        if (selected < end) { curGroup = i; break; }
        acc = end;
      }
      const dir = e.shiftKey ? -1 : 1;
      const nextGroup = (curGroup + dir + grouped.length) % grouped.length;
      // Flat index of the first item in `nextGroup`.
      let offset = 0;
      for (let i = 0; i < nextGroup; i++) offset += grouped[i].items.length;
      refs.setSelected(offset);
      refs.scrollSelectedIntoView();
      return;
    }
  }
  window.addEventListener('keydown', onKey);
  return () => window.removeEventListener('keydown', onKey);
}
