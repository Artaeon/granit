// Workspace presets — named starting layouts. Each preset returns a
// fresh TreeNode (with fresh ids), so calling buildLayout() twice
// builds two distinct trees that won't clash inside the persistence
// layer.
//
// Kept deliberately small. Adding a new preset is one entry. The
// command palette surfaces every preset as "New workspace: <name>";
// the StatusBar's `+` button stays the muscle-memory blank shortcut.

import { makeLeaf, makeSplit, type TreeNode } from './splitTree';

export type WorkspacePreset = {
  /** Stable id, used by the palette for recency boosts + i18n keys
   *  if we ever ship them. */
  id: string;
  /** What the user sees in the "New workspace: …" command label. */
  name: string;
  /** One-line hint shown next to the label in the palette. */
  detail: string;
  /** Constructor — returns a fresh tree with new ids on every call. */
  buildLayout(): TreeNode;
};

export const WORKSPACE_PRESETS: ReadonlyArray<WorkspacePreset> = [
  {
    id: 'blank',
    name: 'Blank',
    detail: 'Single Tasks pane',
    buildLayout: () => makeLeaf('tasks')
  },
  {
    id: 'daily',
    name: 'Daily',
    detail: 'Tasks · Calendar',
    buildLayout: () => makeSplit('h', makeLeaf('tasks'), makeLeaf('calendar'), 0.5)
  },
  {
    id: 'plan',
    name: 'Plan',
    detail: 'Goals · Notes',
    buildLayout: () => makeSplit('h', makeLeaf('goals'), makeLeaf('notes'), 0.5)
  },
  {
    id: 'write',
    name: 'Write',
    detail: 'Notes · AI',
    buildLayout: () => makeSplit('h', makeLeaf('notes'), makeLeaf('chat'), 0.62)
  },
  {
    id: 'money',
    name: 'Money',
    detail: 'Finance · Calendar',
    buildLayout: () => makeSplit('h', makeLeaf('finance'), makeLeaf('calendar'), 0.62)
  }
];

export function findPreset(id: string): WorkspacePreset | undefined {
  return WORKSPACE_PRESETS.find((p) => p.id === id);
}
