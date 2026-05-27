// Central keybinding registry. Today's shortcuts live scattered
// across +layout.svelte, AIOverlay.svelte, CommandPalette.svelte,
// QuickCaptureFab.svelte, /deadlines, /jots etc. — each component
// owns its own keydown listener with hard-coded chord checks.
//
// That's worked but it has two costs:
//   1. There's no single place to answer "what shortcuts does the
//      app actually have?" — a Cheatsheet overlay or About page
//      currently has to be hand-curated (and ShortcutsHelpOverlay
//      drifts from the editor keymap for exactly this reason).
//   2. Remapping for a user is impossible — the chords are baked
//      into the listener closures.
//
// This module is the first half of fixing both: declarative
// bindings + a matchesKey() helper that consumers can call from
// their existing onkeydown handlers. Migrating every consumer is
// out of scope for the initial drop — the layout's Mod+Shift+O
// tray jump exercises the API and the rest stays as is until
// the next pass.
//
// Conventions:
//   - id        kebab-case, stable, used by future remap UI
//   - keys      canonical chord: 'Mod+Shift+O', 'Ctrl+Shift+N', '?'
//               'Mod' resolves to ⌘ on macOS, Ctrl elsewhere — match
//               CodeMirror's convention so the cheat-sheet renderer
//               can swap glyphs without each binding caring.
//   - scope     'global' fires from anywhere (including inside an
//               <input>); 'app' fires anywhere outside text-typing
//               targets; a route string ('deadlines', 'jots') means
//               that route hosts the listener itself.

export type BindingScope = 'global' | 'app' | string;

export interface KeyBinding {
  id: string;
  label: string;
  keys: string;
  scope: BindingScope;
  description?: string;
}

// Single source of truth for the global + app-wide shortcuts. Route-
// scoped bindings (Deadlines `n` etc.) can register themselves here
// over time; the first drop only enumerates what's currently in the
// layout.
//
// New entries should land here FIRST and the consuming component
// reads via findBinding(id). This is what closes the drift loop the
// ShortcutsHelpOverlay used to suffer from — adding a new chord in a
// component without telling the cheat sheet meant the cheat sheet
// went stale until somebody noticed.
export const KEYBINDINGS: KeyBinding[] = [
  {
    id: 'quick-jump',
    label: 'Quick jump · command palette',
    keys: 'Mod+K',
    scope: 'global',
    description: 'Open the command palette to navigate anywhere — pages, notes, projects, goals, agent commands.'
  },
  {
    id: 'ask-ai',
    label: 'Ask AI',
    keys: 'Mod+J',
    scope: 'global',
    description: 'Open the AI overlay from anywhere.'
  },
  {
    id: 'tray-jump',
    label: 'Jump to last note',
    keys: 'Mod+Shift+O',
    scope: 'global',
    description: "Reopen whatever the note tray remembers — fires even while typing, because it's an app-shell shortcut, not a text edit."
  },
  {
    id: 'quick-capture',
    label: 'Quick capture · new task',
    keys: 'Ctrl+Shift+N',
    scope: 'global',
    description: 'Open the task-capture modal from anywhere. Optional priority / due / project / recurrence.'
  },
  {
    id: 'voice-note',
    label: 'Voice note',
    keys: 'Ctrl+Shift+V',
    scope: 'global',
    description: 'Open the voice-note recorder — captures audio + transcribes to a fresh note.'
  },
  {
    id: 'shortcuts-help',
    label: 'Show keyboard shortcuts',
    keys: '?',
    scope: 'app',
    description: 'Pop the cheat sheet overlay listing every shortcut the app knows about.'
  },
  {
    id: 'focus-page-search',
    label: 'Focus page search',
    keys: '/',
    scope: 'app',
    description: 'Focus the current page\'s primary search/filter input. Pauses on pages without one.'
  },
  {
    id: 'go-to-today',
    label: 'Go to today\'s daily note',
    keys: 'g d',
    scope: 'app',
    description: 'Two-key chord — press g then d within 350ms to jump straight to today\'s Daily note.'
  },
  {
    id: 'print-preview',
    label: 'Print preview',
    keys: 'Mod+P',
    scope: 'app',
    description: 'Open the branded print/PDF preview for the current note (browser print stays available via the OS shortcut menu).'
  }
];

export function findBinding(id: string): KeyBinding | undefined {
  return KEYBINDINGS.find((b) => b.id === id);
}

// matchesKey resolves a chord string against a KeyboardEvent.
// 'Mod' matches metaKey on macOS, ctrlKey elsewhere. Single-character
// keys are case-insensitive ('o' matches 'O'). Special keys
// (Escape, Enter, ?, /, ...) match by event.key directly.
export function matchesKey(event: KeyboardEvent, keys: string): boolean {
  const parts = keys.split('+').map((p) => p.trim());
  const target = parts[parts.length - 1];
  const wantMod = parts.includes('Mod');
  const wantCtrl = parts.includes('Ctrl');
  const wantShift = parts.includes('Shift');
  const wantAlt = parts.includes('Alt');

  const isMac =
    typeof navigator !== 'undefined' &&
    /Mac|iPhone|iPad/i.test(navigator.platform || navigator.userAgent);
  const modPressed = isMac ? event.metaKey : event.ctrlKey;

  if (wantMod && !modPressed) return false;
  if (!wantMod && wantCtrl && !event.ctrlKey) return false;
  if (!wantMod && !wantCtrl && (event.metaKey || event.ctrlKey)) return false;
  if (wantShift !== event.shiftKey) return false;
  if (wantAlt !== event.altKey) return false;

  // Compare the leaf key. Single-letter keys: lowercase compare.
  // Anything else (Escape, ArrowDown, ?, /): exact match on event.key.
  if (target.length === 1) {
    return event.key.toLowerCase() === target.toLowerCase();
  }
  return event.key === target;
}
