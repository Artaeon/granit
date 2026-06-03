// Keyboard shortcut installer for the AI overlay.
//
// Three global shortcuts:
//
//   Mod+J         → toggle the overlay (from anywhere, including
//                   inside editors — "ask AI about what I'm writing"
//                   is the killer use case).
//   Escape        → dismiss the topmost sub-surface (picker → history
//                   slide-over → overlay itself). Layered so a stray
//                   Esc never wipes the user's whole panel state.
//   Mod+1..9      → switch to AGENT_MODES[idx-1]. Only while the
//                   overlay is open AND the user isn't typing into a
//                   textarea/input (numbers there should land as
//                   numbers, not mode jumps).
//
// Lives in $lib/chat as a pure module so the overlay's god-file
// doesn't carry two `function onKey` declarations that shadowed
// each other (one at the top level, one inside an $effect) — a
// foot-gun for future edits. The layered Esc dismissal stays in the
// host's scope via the `onEscape` callback, because the picker /
// history state is the host's to own.

import { AGENT_MODES } from '$lib/ai/agents';

export interface OverlayShortcutOptions {
  /** True while the overlay is open. Esc only fires when something
   *  to dismiss; Mod+1..9 only fires when the overlay is showing. */
  isOpen: () => boolean;
  /** Mod+J — open/close toggle. */
  toggle: () => void;
  /** Esc — the host does the layered dismissal (picker → history →
   *  overlay). preventDefault is already called by the time this
   *  fires; the host just decides what to close. */
  onEscape: () => void;
  /** Mod+1..9 — switch to mode at the given AGENT_MODES index. The
   *  installer does the parse + textarea-guard; the host commits. */
  selectMode: (modeId: string) => void;
}

/** Attaches keydown listeners on `window` and returns a teardown
 *  for the host's onMount. Safe to call inside an onMount return:
 *
 *    onMount(() => installOverlayShortcuts({ ... }));
 */
export function installOverlayShortcuts(opts: OverlayShortcutOptions): () => void {
  if (typeof window === 'undefined') return () => {};

  function onKey(e: KeyboardEvent) {
    if (opts.isOpen() && e.key === 'Escape') {
      e.preventDefault();
      opts.onEscape();
      return;
    }
    if ((e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 'j') {
      e.preventDefault();
      opts.toggle();
    }
  }

  // Mod+1..9 mode jumps — separate handler so the textarea/input
  // guard stays scoped and doesn't accidentally suppress the other
  // shortcuts when typing.
  function onModeKey(e: KeyboardEvent) {
    if (!opts.isOpen()) return;
    const mod = e.metaKey || e.ctrlKey;
    if (!mod || e.shiftKey || e.altKey) return;
    const target = e.target as HTMLElement | null;
    if (target instanceof HTMLTextAreaElement || target instanceof HTMLInputElement) return;
    const idx = parseInt(e.key, 10);
    if (Number.isNaN(idx) || idx < 1 || idx > Math.min(9, AGENT_MODES.length)) return;
    e.preventDefault();
    opts.selectMode(AGENT_MODES[idx - 1].id);
  }

  window.addEventListener('keydown', onKey);
  window.addEventListener('keydown', onModeKey);
  return () => {
    window.removeEventListener('keydown', onKey);
    window.removeEventListener('keydown', onModeKey);
  };
}
