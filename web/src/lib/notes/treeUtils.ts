import type { Note } from '$lib/api';

export interface TreeNode {
  name: string;
  path: string; // vault-relative; '' for root
  isFolder: boolean;
  children?: TreeNode[];
  note?: Note;
  /** total notes including descendants (folder only) */
  count?: number;
}

/** Tree sort orders. 'alpha' is the classic file-manager rhythm —
 *  folders first, then names A→Z. 'recent' surfaces the user's
 *  active work by sorting notes most-recently-modified first; a
 *  folder's position is driven by its newest descendant, so the
 *  folder containing today's daily-note floats to the top of its
 *  parent without the user having to expand it. */
export type TreeSort = 'alpha' | 'recent';

export function buildTree(notes: Note[], sort: TreeSort = 'alpha'): TreeNode {
  const root: TreeNode = { name: '', path: '', isFolder: true, children: [] };
  for (const n of notes) {
    const parts = n.path.split('/').filter(Boolean);
    let cur = root;
    for (let i = 0; i < parts.length - 1; i++) {
      const folderName = parts[i];
      let next = cur.children!.find((c) => c.isFolder && c.name === folderName);
      if (!next) {
        next = { name: folderName, path: parts.slice(0, i + 1).join('/'), isFolder: true, children: [] };
        cur.children!.push(next);
      }
      cur = next;
    }
    cur.children!.push({
      name: parts[parts.length - 1],
      path: n.path,
      isFolder: false,
      note: n
    });
  }
  countAndSort(root, sort);
  return root;
}

function nodeModMs(n: TreeNode): number {
  const t = n.note?.modTime;
  if (!t) return -Infinity;
  const ms = Date.parse(t);
  return Number.isFinite(ms) ? ms : -Infinity;
}

// Recursive count + sort. Returns { count, newestMs } so a folder's
// sort key under 'recent' can use its deepest descendant's modTime.
// Tracking newest at every level (rather than re-walking on each
// compare) keeps the sort O(N) overall.
function countAndSort(node: TreeNode, sort: TreeSort): { count: number; newestMs: number } {
  if (!node.children) {
    return { count: node.isFolder ? 0 : 1, newestMs: nodeModMs(node) };
  }
  let total = 0;
  let newest = -Infinity;
  const stats = new Map<TreeNode, { count: number; newestMs: number }>();
  for (const c of node.children) {
    const s = countAndSort(c, sort);
    stats.set(c, s);
    total += s.count;
    if (s.newestMs > newest) newest = s.newestMs;
  }
  node.count = total;
  node.children.sort((a, b) => {
    // Folders always sort above files within the same level so
    // the "directories on top" mental model holds across both
    // sort keys.
    if (a.isFolder !== b.isFolder) return a.isFolder ? -1 : 1;
    if (sort === 'recent') {
      const am = stats.get(a)!.newestMs;
      const bm = stats.get(b)!.newestMs;
      // Newer first; -Infinity sinks (no parseable modTime).
      // Tie-break alphabetically so equal-mtime rows don't shuffle.
      if (am !== bm) return bm - am;
    }
    return a.name.toLowerCase().localeCompare(b.name.toLowerCase());
  });
  return { count: total, newestMs: newest };
}

/** Filter the tree by query — keep folders that contain matching notes. */
export function filterTree(node: TreeNode, q: string): TreeNode | null {
  if (!q) return node;
  const ql = q.toLowerCase();
  if (!node.isFolder) {
    return node.name.toLowerCase().includes(ql) || node.path.toLowerCase().includes(ql) ? node : null;
  }
  const kept: TreeNode[] = [];
  for (const c of node.children ?? []) {
    const f = filterTree(c, q);
    if (f) kept.push(f);
  }
  if (kept.length === 0) return null;
  return { ...node, children: kept, count: kept.reduce((s, c) => s + (c.count ?? 1), 0) };
}

/** Returns the set of folder paths that contain the given file path. */
export function ancestorFolders(filePath: string): Set<string> {
  const out = new Set<string>();
  const parts = filePath.split('/');
  for (let i = 1; i < parts.length; i++) out.add(parts.slice(0, i).join('/'));
  return out;
}
