// Keyboard handler factory for InlineAIMenu's prompt input.
//
// The menu's <input> binds onkeydown to the handler returned here.
// Lives in a plain .ts module (no Svelte runes) because the function
// itself is stateless — every piece of state it reads is supplied
// by getters/setters on the deps object, every action it triggers is
// a callback. Lifting it out of the .svelte component keeps that
// surface focused on layout and makes the chord-precedence rules
// (Esc > History recall > Arrow nav > Enter routing) reviewable in
// one place.
//
// Chord precedence (top-down, first match wins):
//
//   1. Esc                — close the menu.
//   2. ArrowUp/ArrowDown  — recall prompt history WHEN the input is
//                           empty OR already showing a recalled
//                           prompt. Mod-modified arrows skip this so
//                           power users get list navigation. Otherwise
//                           falls through to (3).
//   3. ArrowUp/ArrowDown  — cycle the preset highlight, wrapping at
//                           the ends.
//   4. Enter              — three sub-rules:
//                              a. typed query AND any preset visible
//                                 → pick the highlighted preset
//                                 (lets "type to filter, arrow, Enter"
//                                 work).
//                              b. typed query but no preset visible
//                                 → fire as a custom Ask.
//                              c. empty query → pick the highlighted
//                                 preset.
//                           The "filtering" branch and the "empty +
//                           highlighted" branch both end on runPreset,
//                           but stay distinct so future changes (a
//                           "no presets" zero-state with a different
//                           Enter behaviour) have a clean seam.

import type { Preset } from './inline-ai-presets';

export interface MenuKeysDeps {
  /** Current input value. Read for the empty-vs-filtered branching;
   *  written when ArrowUp/ArrowDown recalls a history entry. */
  getPromptInput: () => string;
  setPromptInput: (s: string) => void;

  /** Reactive history list and 1D cursor. -1 = live input; 0 = most
   *  recent. The cursor is mutated when ArrowUp/ArrowDown lands in
   *  history mode. */
  getHistory: () => string[];
  getHistoryIdx: () => number;
  setHistoryIdx: (n: number) => void;

  /** Reactive list of presets currently shown (mode + query filter
   *  applied) and the highlight cursor into it. */
  getVisiblePresets: () => Preset[];
  getHighlightedIdx: () => number;
  setHighlightedIdx: (n: number) => void;

  /** Run a preset. Called by Enter on a highlighted row. */
  runPreset: (p: Preset) => void;
  /** Fire the typed prompt as a custom Ask. Called by Enter when
   *  the query matched no presets. */
  runCustomPrompt: () => void;
  /** Close the menu — bound to Esc. */
  onClose: () => void;
}

export function createMenuKeyHandler(
  deps: MenuKeysDeps
): (e: KeyboardEvent) => void {
  return function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      deps.onClose();
      return;
    }
    // History recall — Up/Down with the cursor in the input cycles
    // through previous prompts before falling through to action-list
    // navigation. We only treat the input as a "history field" when
    // the cursor is at the start AND the input is either empty or
    // already showing a history entry; otherwise Up still navigates
    // the list (so power users who don't care about history get the
    // expected behaviour). Mod-modified arrows always go to the list.
    const history = deps.getHistory();
    const promptInput = deps.getPromptInput();
    if (
      (e.key === 'ArrowUp' || e.key === 'ArrowDown') &&
      history.length > 0 &&
      !e.metaKey &&
      !e.ctrlKey
    ) {
      const idx = deps.getHistoryIdx();
      const inHistoryMode = idx >= 0 || promptInput.length === 0;
      if (inHistoryMode) {
        e.preventDefault();
        if (e.key === 'ArrowUp') {
          const next = Math.min(history.length - 1, idx + 1);
          deps.setHistoryIdx(next);
          deps.setPromptInput(history[next] ?? '');
        } else {
          const next = Math.max(-1, idx - 1);
          deps.setHistoryIdx(next);
          deps.setPromptInput(next === -1 ? '' : (history[next] ?? ''));
        }
        return;
      }
    }
    const visiblePresets = deps.getVisiblePresets();
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      const hi = deps.getHighlightedIdx();
      deps.setHighlightedIdx((hi + 1) % Math.max(1, visiblePresets.length));
      return;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      const hi = deps.getHighlightedIdx();
      deps.setHighlightedIdx(
        (hi - 1 + visiblePresets.length) % Math.max(1, visiblePresets.length)
      );
      return;
    }
    if (e.key === 'Enter') {
      e.preventDefault();
      // If the prompt input has text AND there's still at least one
      // preset visible, the user is filtering — Enter picks the
      // highlighted preset. If the query cleared the list (matched
      // nothing), Enter runs the prompt as a custom Ask. This gives
      // both "type a thought, hit Enter" and "type to filter,
      // arrow, Enter" patterns. The earlier comparison against
      // `PRESETS.length` was always true in practice (mode-filter
      // narrows visiblePresets BELOW the unfiltered total before any
      // query runs), so it added complexity without doing work.
      const filtering = promptInput.trim().length > 0 && visiblePresets.length > 0;
      if (filtering) {
        const p = visiblePresets[deps.getHighlightedIdx()];
        if (p) deps.runPreset(p);
        return;
      }
      if (promptInput.trim().length > 0) {
        deps.runCustomPrompt();
        return;
      }
      const p = visiblePresets[deps.getHighlightedIdx()];
      if (p) deps.runPreset(p);
    }
  };
}
