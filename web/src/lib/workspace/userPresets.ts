// User-saved workspace presets — the "customizable" half of the
// workspace UX. Built-in presets in workspacePresets.ts give users a
// jumping-off point; this layer lets them save their OWN layouts as
// named presets they can re-apply later from the New menu.
//
// Persistence: localStorage. Same pattern as the rest of the
// workspace surface (workspaces themselves persist to
// granit.workspaces). Per-user only; no server round-trip.
//
// Cloning: when a preset is loaded back into a fresh workspace, the
// tree IDs are regenerated. Stable IDs only matter within a single
// workspace's lifetime; cloning prevents accidental collision when
// re-applying the same preset multiple times in a row.

import { loadStored, saveStored } from '$lib/util/storage';
import { isTree, makeLeaf, makeSplit, type TreeNode } from './splitTree';

export interface UserPreset {
  id: string;
  name: string;
  layout: TreeNode;
}

const KEY = 'granit.workspacePresets';

function newId(): string {
  let s = '';
  for (let i = 0; i < 8; i++) s += Math.floor(Math.random() * 36).toString(36);
  return s;
}

/** Deep-clone a tree with fresh node IDs at every level. The pane
 *  kinds + split direction + ratio carry over verbatim. */
export function cloneWithNewIds(node: TreeNode): TreeNode {
  if (node.kind === 'leaf') return makeLeaf(node.pane);
  return makeSplit(node.direction, cloneWithNewIds(node.first), cloneWithNewIds(node.second), node.ratio);
}

export function loadUserPresets(): UserPreset[] {
  const raw = loadStored<unknown>(KEY, []);
  if (!Array.isArray(raw)) return [];
  return raw.filter((p): p is UserPreset => {
    if (!p || typeof p !== 'object') return false;
    const o = p as Record<string, unknown>;
    return typeof o.id === 'string' && typeof o.name === 'string' && isTree(o.layout);
  });
}

/** Append a preset with the given name + a CLONED layout (so the
 *  saved copy doesn't share IDs with the live workspace it came
 *  from). Returns the saved entry. */
export function saveUserPreset(name: string, layout: TreeNode): UserPreset {
  const trimmed = name.trim() || 'Untitled';
  const existing = loadUserPresets();
  const fresh: UserPreset = {
    id: newId(),
    name: trimmed,
    layout: cloneWithNewIds(layout)
  };
  saveStored(KEY, [...existing, fresh]);
  return fresh;
}

export function removeUserPreset(id: string): void {
  saveStored(KEY, loadUserPresets().filter((p) => p.id !== id));
}
