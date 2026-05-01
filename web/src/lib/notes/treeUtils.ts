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

export function buildTree(notes: Note[]): TreeNode {
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
  countAndSort(root);
  return root;
}

function countAndSort(node: TreeNode): number {
  if (!node.children) return node.isFolder ? 0 : 1;
  let total = 0;
  for (const c of node.children) total += countAndSort(c);
  node.count = total;
  node.children.sort((a, b) => {
    if (a.isFolder !== b.isFolder) return a.isFolder ? -1 : 1;
    return a.name.toLowerCase().localeCompare(b.name.toLowerCase());
  });
  return total;
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
