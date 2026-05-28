// Single source of truth for the 10 right-pane content options. The
// dropdown picker in RightPane reads from here; the global keyboard
// router in +layout reads bindingId per row to wire its chord. Adding
// or reordering an entry here automatically updates both surfaces.
//
// `iconPath` is a raw SVG snippet (paths/circles/etc.) drawn at 16x16
// inside an outline-styled <svg> the picker already owns — keeping it
// as a string sidesteps shipping ten one-off Svelte components for
// what amounts to a couple of <path> tags each.

import type { RightPaneContent } from '$lib/stores/rightPane';

export interface RightPaneContentOption {
  id: RightPaneContent;
  label: string;
  /** Tooltip surface — disambiguates "Calendar" vs "the /calendar route". */
  title: string;
  /** Keybinding registry id; resolved at render-time via findBinding. */
  bindingId: string;
  /** Raw SVG path/shape snippet drawn at 16x16 inside the picker's <svg>. */
  iconPath: string;
}

export const RIGHT_PANE_OPTIONS: RightPaneContentOption[] = [
  {
    id: 'calendar',
    label: 'Calendar',
    title: 'Today + tomorrow events',
    bindingId: 'right-pane-calendar',
    iconPath:
      '<rect x="3" y="4" width="18" height="18" rx="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/>'
  },
  {
    id: 'notes',
    label: 'Recent notes',
    title: '15 most-recent notes',
    bindingId: 'right-pane-notes',
    iconPath:
      '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="8" y1="13" x2="16" y2="13"/><line x1="8" y1="17" x2="14" y2="17"/>'
  },
  {
    id: 'ai',
    label: 'AI',
    title: 'AI launcher',
    bindingId: 'right-pane-ai',
    iconPath:
      '<path d="M12 3v3M12 18v3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M3 12h3M18 12h3M5.6 18.4l2.1-2.1M16.3 7.7l2.1-2.1"/><circle cx="12" cy="12" r="3.5"/>'
  },
  {
    id: 'vision',
    label: 'Vision',
    title: 'Pinned vision doc',
    bindingId: 'right-pane-vision',
    iconPath:
      '<circle cx="12" cy="12" r="3"/><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/>'
  },
  {
    id: 'widgets',
    label: 'Widgets',
    title: 'Slim widget strip (3)',
    bindingId: 'right-pane-widgets',
    iconPath:
      '<rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/>'
  },
  {
    id: 'tasks',
    label: 'Tasks',
    title: "Today's tasks",
    bindingId: 'right-pane-tasks',
    iconPath:
      '<polyline points="9 11 12 14 22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/>'
  },
  {
    id: 'today',
    label: 'Today',
    title: 'Daily note + tasks combo',
    bindingId: 'right-pane-today',
    iconPath: '<circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>'
  },
  {
    id: 'goals',
    label: 'Goals',
    title: 'Active goals + progress',
    bindingId: 'right-pane-goals',
    iconPath:
      '<circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="6"/><circle cx="12" cy="12" r="2"/>'
  },
  {
    id: 'habits',
    label: 'Habits',
    title: "Today's habit check-ins",
    bindingId: 'right-pane-habits',
    iconPath:
      '<path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"/>'
  },
  {
    id: 'dashboard',
    label: 'Dashboard',
    title: 'Expanded widget column (6)',
    bindingId: 'right-pane-dashboard',
    iconPath:
      '<rect x="3" y="3" width="18" height="18" rx="2"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="9" y1="21" x2="9" y2="9"/>'
  }
];

/**
 * Mod-glyph chord formatter. Mirrors the convention the ShortcutsOverlay
 * cheatsheet uses so the chord shown in the picker matches the chord
 * the user actually presses. `isMac` is passed in so callers control
 * the navigator check (component lifecycle); this helper is pure.
 */
export function displayChord(keys: string, isMac: boolean): string {
  if (!keys) return '';
  const modGlyph = isMac ? '⌘' : 'Ctrl';
  const shiftGlyph = isMac ? '⇧' : 'Shift';
  return keys
    .replace(/\bMod\b/g, modGlyph)
    .replace(/\bShift\b/g, shiftGlyph)
    .replace(/\+/g, isMac ? '' : '+');
}
