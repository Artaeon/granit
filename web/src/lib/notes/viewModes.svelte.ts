// View-mode controller for the notes route page.
//
// Owns three coupled presentational flags:
//
//   viewMode:    'edit' | 'preview' | 'split' — main pane layout.
//                Cycled by Mod-/ via cycleViewMode().
//   focusMode:   distraction-free chrome (sidebar + info-rail hidden).
//                Toggled by Mod-Shift-Z; remembered across reloads.
//   readingMode: combo of viewMode='preview' + focusMode=true + a
//                CSS class on the preview pane (serif typography,
//                narrower max-width). Toggled by Mod-Shift-R; when
//                turned on it snapshots the prior view + focus so
//                turning it off restores the user's normal setup
//                rather than clobbering them.
//
// All three persist to localStorage and rehydrate on construction.
// The page consumes the controller's getters in its template — same
// reactive contract as the inline $state had, just without the
// scattered persistence effects and the prior-state snapshot
// machinery living next to unrelated route concerns.

import { loadStoredString, saveStoredString } from '$lib/util/storage';

export type ViewMode = 'edit' | 'preview' | 'split';

const VIEW_KEY = 'granit.note.viewMode';
const FOCUS_KEY = 'granit.note.focus';
const READING_KEY = 'granit.note.reading';

const VIEW_ORDER: ViewMode[] = ['edit', 'split', 'preview'];

function loadInitialViewMode(): ViewMode {
  const v = loadStoredString(VIEW_KEY, '');
  if (v === 'edit' || v === 'preview' || v === 'split') return v;
  return 'edit';
}

function loadBool(key: string): boolean {
  if (typeof localStorage === 'undefined') return false;
  try {
    return localStorage.getItem(key) === '1';
  } catch {
    return false;
  }
}

function saveBool(key: string, on: boolean): void {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(key, on ? '1' : '0');
  } catch {
    // Quota / private-mode failures — silent; the toggle still
    // works for the current session.
  }
}

export interface ViewModeController {
  readonly viewMode: ViewMode;
  readonly focusMode: boolean;
  readonly readingMode: boolean;
  setViewMode(m: ViewMode): void;
  cycleViewMode(): void;
  setFocusMode(on: boolean): void;
  toggleFocusMode(): void;
  setReadingMode(on: boolean): void;
  toggleReadingMode(): void;
}

export function createViewModeController(): ViewModeController {
  let viewMode = $state<ViewMode>(loadInitialViewMode());
  let focusMode = $state<boolean>(loadBool(FOCUS_KEY));
  let readingMode = $state<boolean>(loadBool(READING_KEY));

  // Snapshots taken when reading-mode is turned on, restored when
  // it's turned off. Kept as plain `let` (no reactivity needed —
  // they're internal bookkeeping, not consumed by the template).
  let priorView: ViewMode | null = null;
  let priorFocus: boolean | null = null;

  function setViewMode(m: ViewMode) {
    viewMode = m;
    saveStoredString(VIEW_KEY, m);
  }

  function cycleViewMode() {
    const idx = VIEW_ORDER.indexOf(viewMode);
    setViewMode(VIEW_ORDER[(idx + 1) % VIEW_ORDER.length]);
  }

  function setFocusMode(on: boolean) {
    focusMode = on;
    saveBool(FOCUS_KEY, on);
  }

  function toggleFocusMode() {
    setFocusMode(!focusMode);
  }

  function setReadingMode(on: boolean) {
    if (on === readingMode) return;
    if (on) {
      // Snapshot the user's current state so we can restore it.
      priorView = viewMode;
      priorFocus = focusMode;
      setViewMode('preview');
      setFocusMode(true);
    } else if (priorView !== null) {
      setViewMode(priorView);
      setFocusMode(priorFocus ?? false);
      priorView = null;
      priorFocus = null;
    }
    readingMode = on;
    saveBool(READING_KEY, on);
  }

  function toggleReadingMode() {
    setReadingMode(!readingMode);
  }

  return {
    get viewMode() {
      return viewMode;
    },
    get focusMode() {
      return focusMode;
    },
    get readingMode() {
      return readingMode;
    },
    setViewMode,
    cycleViewMode,
    setFocusMode,
    toggleFocusMode,
    setReadingMode,
    toggleReadingMode
  };
}
