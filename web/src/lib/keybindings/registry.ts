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
  // Route(s) on which this shortcut is active. Used by the
  // ShortcutsOverlay to surface a "Current page" section at the
  // top of the cheat sheet so the user sees the page-relevant
  // chords without scrolling through global bindings. String =
  // prefix match on pathname; RegExp = test against pathname;
  // array = any-match across the entries. Bindings without a
  // route apply everywhere their scope covers.
  route?: string | RegExp | Array<string | RegExp>;
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
  },
  // ── Right pane (companion sidebar) ──────────────────────────────
  // Phase 1 of the multi-pane workspace. Mod+\ toggles the pane;
  // Mod+Shift+1..5 jump straight to a content option (calendar /
  // notes / AI / vision / widgets). Listener lives in +layout.svelte
  // so the shortcuts fire from anywhere — they're app-shell moves,
  // not text edits.
  {
    id: 'right-pane-toggle',
    label: 'Toggle right pane',
    keys: 'Mod+\\',
    scope: 'global',
    description: 'Show or hide the companion right pane. Width and last content choice persist across reloads.'
  },
  {
    id: 'right-pane-calendar',
    label: 'Right pane: Calendar',
    keys: 'Mod+Shift+1',
    scope: 'global',
    description: 'Switch the right pane to the Calendar view (today + tomorrow).'
  },
  {
    id: 'right-pane-notes',
    label: 'Right pane: Notes',
    keys: 'Mod+Shift+2',
    scope: 'global',
    description: 'Switch the right pane to the Notes view (15 most-recent).'
  },
  {
    id: 'right-pane-ai',
    label: 'Right pane: AI',
    keys: 'Mod+Shift+3',
    scope: 'global',
    description: 'Switch the right pane to the AI launcher card.'
  },
  {
    id: 'right-pane-vision',
    label: 'Right pane: Vision',
    keys: 'Mod+Shift+4',
    scope: 'global',
    description: 'Switch the right pane to the pinned Vision doc.'
  },
  {
    id: 'right-pane-widgets',
    label: 'Right pane: Widgets',
    keys: 'Mod+Shift+5',
    scope: 'global',
    description: 'Switch the right pane to a slim vertical strip of dashboard widgets.'
  },
  {
    id: 'right-pane-tasks',
    label: 'Right pane: Tasks',
    keys: 'Mod+Shift+6',
    scope: 'global',
    description: "Switch the right pane to today's task list (overdue + due today + scheduled + P1)."
  },
  {
    id: 'right-pane-today',
    label: 'Right pane: Today',
    keys: 'Mod+Shift+7',
    scope: 'global',
    description: "Switch the right pane to the Today combo (daily note preview + today's tasks)."
  },
  {
    id: 'right-pane-goals',
    label: 'Right pane: Goals',
    keys: 'Mod+Shift+8',
    scope: 'global',
    description: 'Switch the right pane to the active goals list with progress bars.'
  },
  {
    id: 'right-pane-habits',
    label: 'Right pane: Habits',
    keys: 'Mod+Shift+9',
    scope: 'global',
    description: "Switch the right pane to today's habit check-ins."
  },
  {
    id: 'right-pane-dashboard',
    label: 'Right pane: Dashboard',
    keys: 'Mod+Shift+0',
    scope: 'global',
    description: 'Switch the right pane to the expanded dashboard widget column.'
  },
  // ── Tabs (Phase 2 — main-pane Obsidian-style tabs) ──────────────
  // Mod+T opens a fresh tab on the current route, Mod+W closes the
  // active tab, Mod+Tab / Mod+Shift+Tab cycle between them, Mod+1..9
  // jumps to a specific tab. Mod+1..9 overlaps with the AIOverlay's
  // mode quick-switch — the overlay listener gates itself behind
  // `open`, so the tab handler only fires when the overlay isn't
  // intercepting first.
  {
    id: 'tab-new',
    label: 'New tab',
    keys: 'Mod+T',
    scope: 'global',
    description: 'Open a new tab on the current route — like a browser duplicate-tab.'
  },
  {
    id: 'tab-close',
    label: 'Close tab',
    keys: 'Mod+W',
    scope: 'global',
    description: 'Close the active tab. On the last tab, also navigates home and clears the strip.'
  },
  {
    id: 'tab-cycle-next',
    label: 'Next tab',
    keys: 'Mod+Tab',
    scope: 'global',
    description: 'Cycle forward through open tabs.'
  },
  {
    id: 'tab-cycle-prev',
    label: 'Previous tab',
    keys: 'Mod+Shift+Tab',
    scope: 'global',
    description: 'Cycle backward through open tabs.'
  },
  {
    id: 'tab-activate-n',
    label: 'Activate tab N',
    keys: 'Mod+1..9',
    scope: 'global',
    description: 'Jump to the Nth open tab (1-indexed). Inactive while the AI overlay is open — there Mod+1..9 picks the AI mode.'
  },
  // ── /tasks page-scoped bindings ──────────────────────────────────
  // These ship as part of Stream F (power-user efficiency). The
  // handler still lives in /tasks/+page.svelte's onKey listener —
  // registering here is what makes the ?-overlay's "Current page"
  // section surface them. `scope: 'tasks'` keeps them out of the
  // generic Global/App groups so the overlay's render path can
  // bucket them into the per-page section instead.
  {
    id: 'tasks-nav-down',
    label: 'Cursor down',
    keys: 'j',
    scope: 'tasks',
    route: '/tasks',
    description: 'Move the keyboard cursor to the next task in the filtered list.'
  },
  {
    id: 'tasks-nav-up',
    label: 'Cursor up',
    keys: 'k',
    scope: 'tasks',
    route: '/tasks',
    description: 'Move the keyboard cursor to the previous task in the filtered list.'
  },
  {
    id: 'tasks-toggle-select',
    label: 'Toggle bulk-select on cursor task',
    keys: 'x',
    scope: 'tasks',
    route: '/tasks',
    description: 'Add or remove the cursor task from the bulk selection.'
  },
  {
    id: 'tasks-toggle-done',
    label: 'Toggle done on cursor task',
    keys: 'd',
    scope: 'tasks',
    route: '/tasks',
    description: 'Mark the cursor task done — or undo it if it was already done.'
  },
  {
    id: 'tasks-open-detail',
    label: 'Open task detail',
    keys: 'e',
    scope: 'tasks',
    route: '/tasks',
    description: 'Open the right-hand TaskDetail drawer for the cursor task.'
  },
  {
    id: 'tasks-cycle-priority',
    label: 'Cycle priority',
    keys: 'p',
    scope: 'tasks',
    route: '/tasks',
    description: 'Cycle the cursor task through P0 → P1 → P2 → P3 → P0.'
  },
  {
    id: 'tasks-agent',
    label: 'Open Task Agent',
    keys: 'a',
    scope: 'tasks',
    route: '/tasks',
    description: 'Open the conversational AI agent scoped to the filtered list (or current bulk-selection).'
  },
  {
    id: 'tasks-snooze',
    label: 'Snooze cursor task',
    keys: 's',
    scope: 'tasks',
    route: '/tasks',
    description: 'Open the snooze picker for the cursor task — pick a wake date and the task hides until then.'
  },
  {
    id: 'tasks-select-all',
    label: 'Select / clear all filtered',
    keys: 'Shift+A',
    scope: 'tasks',
    route: '/tasks',
    description: 'Bulk-select every task in the current filtered list. Press again with everything selected to clear.'
  },
  {
    id: 'tasks-view-prev',
    label: 'Previous view mode',
    keys: '[',
    scope: 'tasks',
    route: '/tasks',
    description: 'Cycle backward through the visible view-mode tabs (Today / List / Week / Kanban / Matrix …).'
  },
  {
    id: 'tasks-view-next',
    label: 'Next view mode',
    keys: ']',
    scope: 'tasks',
    route: '/tasks',
    description: 'Cycle forward through the visible view-mode tabs.'
  },
  {
    id: 'tasks-view-1',
    label: 'Jump to Today view',
    keys: '1',
    scope: 'tasks',
    route: '/tasks',
    description: 'Jump directly to the Today view (overdue + due today + scheduled today).'
  },
  {
    id: 'tasks-view-2',
    label: 'Jump to List view',
    keys: '2',
    scope: 'tasks',
    route: '/tasks',
    description: 'Jump directly to the grouped List view.'
  },
  {
    id: 'tasks-view-3',
    label: 'Jump to Kanban view',
    keys: '3',
    scope: 'tasks',
    route: '/tasks',
    description: 'Jump directly to the Kanban view.'
  },
  {
    id: 'tasks-view-4',
    label: 'Jump to Matrix view',
    keys: '4',
    scope: 'tasks',
    route: '/tasks',
    description: 'Jump directly to the Eisenhower matrix view.'
  }
];

// Test whether a binding's route filter matches the given pathname.
// Bindings without a route always match (i.e. apply everywhere).
export function bindingMatchesRoute(binding: KeyBinding, pathname: string): boolean {
  if (!binding.route) return true;
  const entries = Array.isArray(binding.route) ? binding.route : [binding.route];
  for (const r of entries) {
    if (typeof r === 'string') {
      if (pathname === r || pathname.startsWith(r + '/') || pathname.startsWith(r + '?')) return true;
      // Bare prefix match too (e.g. route='/tasks' matches '/tasks/123').
      if (r.endsWith('/')) {
        if (pathname.startsWith(r)) return true;
      } else if (pathname === r || pathname.startsWith(r + '/')) {
        return true;
      }
    } else if (r instanceof RegExp) {
      if (r.test(pathname)) return true;
    }
  }
  return false;
}

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
