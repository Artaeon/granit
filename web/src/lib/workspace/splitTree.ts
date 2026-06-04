// Split-tree primitives for the workspace shell.
//
// The workspace layout is a tree: leaves hold pane kinds, splits
// have a direction (h = side-by-side, v = stacked) and a ratio for
// the first child's share of the parent's size. Every node carries
// a stable id so the persistence layer + UI can refer to them
// across re-renders.
//
// All operations are pure — given a tree and an id, they return a
// new tree. The workspace store wraps these in setters; nothing
// outside this file mutates the tree shape directly.

import type { PaneKind } from './paneRegistry';

export type Direction = 'h' | 'v';

export type LeafNode = {
  kind: 'leaf';
  id: string;
  pane: PaneKind;
};

export type SplitNode = {
  kind: 'split';
  id: string;
  direction: Direction;
  /** First child's share of the parent's size, 0.1 .. 0.9. */
  ratio: number;
  first: TreeNode;
  second: TreeNode;
};

export type TreeNode = LeafNode | SplitNode;

// Local id helper. Mirrors the workspaceStore version so split-tree
// internals don't reach back into the consumer for IDs.
function nid(): string {
  let s = '';
  for (let i = 0; i < 8; i++) s += Math.floor(Math.random() * 36).toString(36);
  return s;
}

export function makeLeaf(pane: PaneKind): LeafNode {
  return { kind: 'leaf', id: nid(), pane };
}

export function makeSplit(
  direction: Direction,
  first: TreeNode,
  second: TreeNode,
  ratio = 0.5
): SplitNode {
  return { kind: 'split', id: nid(), direction, ratio, first, second };
}

// ── Tree validation + reconstruction ──────────────────────────────

/** Type guard for runtime-parsed trees (persisted JSON). */
export function isTree(x: unknown): x is TreeNode {
  if (!x || typeof x !== 'object') return false;
  const o = x as Record<string, unknown>;
  if (o.kind === 'leaf') {
    return typeof o.id === 'string' && typeof o.pane === 'string';
  }
  if (o.kind === 'split') {
    return (
      typeof o.id === 'string' &&
      (o.direction === 'h' || o.direction === 'v') &&
      typeof o.ratio === 'number' &&
      isTree(o.first) &&
      isTree(o.second)
    );
  }
  return false;
}

// ── Pure tree operations ──────────────────────────────────────────

/**
 * Split a leaf in place. The leaf becomes the `first` child of a new
 * split node; `second` is a fresh leaf with `newPane`. Returns the
 * new tree with the same root identity as the input where possible.
 *
 * No-op when the leaf isn't found (the tree is returned unchanged).
 */
export function splitLeaf(
  tree: TreeNode,
  leafId: string,
  direction: Direction,
  newPane: PaneKind
): TreeNode {
  if (tree.kind === 'leaf') {
    if (tree.id !== leafId) return tree;
    return makeSplit(direction, tree, makeLeaf(newPane));
  }
  const first = splitLeaf(tree.first, leafId, direction, newPane);
  const second = splitLeaf(tree.second, leafId, direction, newPane);
  if (first === tree.first && second === tree.second) return tree;
  return { ...tree, first, second };
}

/**
 * Close a leaf. The parent split is replaced by the sibling subtree;
 * leaves that aren't found and the root leaf (no parent) are
 * returned unchanged — the workspace must always have at least one
 * leaf so the shell can't render empty.
 */
export function closeLeaf(tree: TreeNode, leafId: string): TreeNode {
  if (tree.kind === 'leaf') return tree;
  // Check for direct children — if one of them is the target leaf,
  // collapse this split into the other.
  if (tree.first.kind === 'leaf' && tree.first.id === leafId) return tree.second;
  if (tree.second.kind === 'leaf' && tree.second.id === leafId) return tree.first;
  // Otherwise recurse.
  const first = closeLeaf(tree.first, leafId);
  const second = closeLeaf(tree.second, leafId);
  if (first === tree.first && second === tree.second) return tree;
  return { ...tree, first, second };
}

/** Update the ratio on a specific split node. Clamped 0.1 .. 0.9. */
export function updateRatio(tree: TreeNode, splitId: string, ratio: number): TreeNode {
  const next = Math.min(0.9, Math.max(0.1, ratio));
  if (tree.kind === 'leaf') return tree;
  if (tree.id === splitId) {
    if (tree.ratio === next) return tree;
    return { ...tree, ratio: next };
  }
  const first = updateRatio(tree.first, splitId, ratio);
  const second = updateRatio(tree.second, splitId, ratio);
  if (first === tree.first && second === tree.second) return tree;
  return { ...tree, first, second };
}

/** Change the pane kind in a specific leaf. */
export function updatePane(tree: TreeNode, leafId: string, pane: PaneKind): TreeNode {
  if (tree.kind === 'leaf') {
    if (tree.id !== leafId || tree.pane === pane) return tree;
    return { ...tree, pane };
  }
  const first = updatePane(tree.first, leafId, pane);
  const second = updatePane(tree.second, leafId, pane);
  if (first === tree.first && second === tree.second) return tree;
  return { ...tree, first, second };
}

/** Flatten to a list of leaves (left-to-right depth-first order).
 *  Used by the mobile single-leaf view + the leaf-count check. */
export function leaves(tree: TreeNode): LeafNode[] {
  if (tree.kind === 'leaf') return [tree];
  return [...leaves(tree.first), ...leaves(tree.second)];
}

/** Migrate the v1 flat layout shape into a tree. */
export function fromFlat(left: PaneKind, right: PaneKind, ratio: number): TreeNode {
  return makeSplit('h', makeLeaf(left), makeLeaf(right), ratio);
}
