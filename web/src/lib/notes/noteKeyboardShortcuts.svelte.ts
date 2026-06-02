// Global window-level keyboard shortcuts for the notes route.
//
// Combined under one keydown listener (was two separate $effects
// on the page, each doing its own active-element sniffing). Pure
// event wiring — every binding calls back into the page's
// existing setters/methods/controllers, none mutate page state
// directly.
//
// Shortcuts handled here:
//
//   ?                     — open shortcuts cheat sheet (skipped
//                           when typing into an input / contenteditable)
//   Mod-/                 — cycle view mode (edit → split → preview)
//   Mod-Shift-Z           — toggle focus mode
//   Mod-Shift-R           — toggle reading mode (preview + focus + serif)
//   Mod-Shift-P           — open slideshow / presentation overlay
//   Mod-Shift-←/→         — jump to previous / next daily note
//                           (only when current note is a daily)
//
// The page wraps install() in a single $effect so the listener
// teardown follows component lifecycle naturally.

import type { ViewModeController } from './viewModes.svelte';

export interface NoteShortcutsOpts {
  viewModes: ViewModeController;
  /** Returns the editor's DOM root, or undefined when no editor
   *  is mounted (preview-only view). Used by the daily-nav shortcut
   *  to distinguish "typing in the editor" from "typing in some
   *  other input on the page". */
  getEditorDOM: () => HTMLElement | undefined;
  getIsDaily: () => boolean;
  getDailyDate: () => string | null;
  hasNote: () => boolean;
  shiftDate: (iso: string, days: number) => string;
  gotoDaily: (date: string) => Promise<void> | void;
  openHelp: () => void;
  openPresentation: () => void;
}

function isModifier(e: KeyboardEvent): boolean {
  // Mac uses Cmd, every other platform uses Ctrl. The navigator UA
  // sniff is intentional — the modern userAgentData.platform isn't
  // available everywhere yet.
  const isMac = /Mac|iPhone|iPad/i.test(navigator.platform || navigator.userAgent);
  return isMac ? e.metaKey : e.ctrlKey;
}

interface ActiveContext {
  inInput: boolean;
  isEditable: boolean;
  el: HTMLElement | null;
}

function readActiveContext(): ActiveContext {
  const el = document.activeElement as HTMLElement | null;
  const tag = el?.tagName?.toLowerCase();
  const inInput = tag === 'input' || tag === 'textarea';
  const isEditable = !!el?.isContentEditable;
  return { inInput, isEditable, el };
}

export function installNoteShortcuts(opts: NoteShortcutsOpts): () => void {
  function onKey(e: KeyboardEvent) {
    const ctx = readActiveContext();

    // "?" — open shortcuts help. Skip when typing anywhere editable
    // so the user can still type a literal question mark in their
    // notes / inputs. CodeMirror's editable surface is a
    // contenteditable div, so `isEditable` already covers it.
    if (e.key === '?' && e.shiftKey && !ctx.inInput && !ctx.isEditable) {
      e.preventDefault();
      opts.openHelp();
      return;
    }

    const mod = isModifier(e);
    if (!mod) return;

    // Mod-/ — cycle view mode (edit → split → preview).
    if (e.key === '/' && !e.shiftKey && !e.altKey) {
      if (ctx.inInput) return;
      e.preventDefault();
      opts.viewModes.cycleViewMode();
      return;
    }

    // Mod-Shift-Z — toggle focus mode. Always live (even with the
    // editor focused) since it's a visibility toggle and doesn't
    // collide with any default editor binding.
    if (e.shiftKey && e.key.toLowerCase() === 'z') {
      e.preventDefault();
      opts.viewModes.toggleFocusMode();
      return;
    }

    // Mod-Shift-R — toggle reading mode (preview + focus + serif).
    if (e.shiftKey && e.key.toLowerCase() === 'r') {
      e.preventDefault();
      opts.viewModes.toggleReadingMode();
      return;
    }

    // Mod-Shift-P — open slideshow / presentation mode. Skipped on
    // an empty page (no note loaded).
    if (e.shiftKey && e.key.toLowerCase() === 'p') {
      e.preventDefault();
      if (opts.hasNote()) opts.openPresentation();
      return;
    }

    // Mod-Shift-←/→ — jump to previous / next daily note. Only on
    // daily notes (otherwise the chord has no obvious target). Skip
    // when typing into a non-editor input.
    if (e.shiftKey && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) {
      if (!opts.getIsDaily()) return;
      const dailyDate = opts.getDailyDate();
      if (!dailyDate) return;
      // Allow chord while focus is in the editor; suppress only for
      // unrelated inputs (search box, dialog field).
      if (ctx.inInput && ctx.el !== opts.getEditorDOM()) return;
      e.preventDefault();
      const delta = e.key === 'ArrowLeft' ? -1 : 1;
      void opts.gotoDaily(opts.shiftDate(dailyDate, delta));
      return;
    }
  }

  window.addEventListener('keydown', onKey);
  return () => window.removeEventListener('keydown', onKey);
}
