// Workspace state — the "named layouts" part of the granit vision.
//
// Owns the array of saved workspaces plus the active-workspace id.
// Each workspace is a small persisted struct: a stable id, a
// user-named label, and a split-tree layout (see splitTree.ts).
//
// Persistence: localStorage key `granit.workspaces`. Two legacy
// migrations:
//   v0 → v2: `granit.workspace.layout` single-layout flat shape
//            from the earliest prototype is folded into a "Default"
//            workspace.
//   v1 → v2: per-workspace `layout: { left, right, ratio }` flat
//            shape from the named-workspace prototype is rebuilt
//            into a horizontal split tree.
// Both run on load so existing users never see a blank shell.

import { loadStored, saveStored } from '$lib/util/storage';
import type { PaneKind } from './paneRegistry';
import {
  fromFlat,
  isTree,
  leaves,
  makeLeaf,
  splitLeaf as splitLeafTree,
  closeLeaf as closeLeafTree,
  updateRatio as updateRatioTree,
  updatePane as updatePaneTree,
  type Direction,
  type TreeNode
} from './splitTree';

const STORE_KEY = 'granit.workspaces';
const LEGACY_LAYOUT_KEY = 'granit.workspace.layout';

export type Workspace = {
  id: string;
  name: string;
  /** Layout is a split-tree. Migrations rebuild older flat shapes
   *  into a horizontal split on first read. */
  layout: TreeNode;
};

function defaultLayout(): TreeNode {
  return fromFlat('tasks', 'calendar', 0.5);
}

function newId(): string {
  let id = '';
  for (let i = 0; i < 8; i++) id += Math.floor(Math.random() * 36).toString(36);
  return id;
}

// ── Persistence shape + migrations ────────────────────────────────

type PersistedV2 = { workspaces: Workspace[]; activeId: string };
type FlatLayout = { left: PaneKind; right: PaneKind; ratio: number };

function isFlatLayout(x: unknown): x is FlatLayout {
  if (!x || typeof x !== 'object') return false;
  const o = x as Record<string, unknown>;
  return typeof o.left === 'string' && typeof o.right === 'string' && typeof o.ratio === 'number';
}

function migrateLegacyLayout(): Workspace | null {
  const raw = loadStored<unknown>(LEGACY_LAYOUT_KEY, null);
  if (!isFlatLayout(raw)) return null;
  return {
    id: newId(),
    name: 'Default',
    layout: fromFlat(raw.left, raw.right, raw.ratio)
  };
}

function normalizeWorkspace(w: unknown): Workspace | null {
  if (!w || typeof w !== 'object') return null;
  const o = w as Record<string, unknown>;
  const name = typeof o.name === 'string' ? o.name : 'Workspace';
  const id = typeof o.id === 'string' ? o.id : newId();
  // v2 already-a-tree path.
  if (isTree(o.layout)) {
    return { id, name, layout: o.layout };
  }
  // v1 flat-layout path — rebuild into a tree.
  if (isFlatLayout(o.layout)) {
    return { id, name, layout: fromFlat(o.layout.left, o.layout.right, o.layout.ratio) };
  }
  return null;
}

function loadInitial(): PersistedV2 {
  const stored = loadStored<unknown>(STORE_KEY, null);
  if (stored && typeof stored === 'object') {
    const o = stored as Record<string, unknown>;
    if (Array.isArray(o.workspaces) && o.workspaces.length > 0) {
      const migrated = o.workspaces
        .map(normalizeWorkspace)
        .filter((w): w is Workspace => w !== null);
      if (migrated.length > 0) {
        const activeId =
          typeof o.activeId === 'string' && migrated.find((w) => w.id === o.activeId)
            ? o.activeId
            : migrated[0].id;
        return { workspaces: migrated, activeId };
      }
    }
  }
  // First-run or migration-from-v0 path.
  const legacy = migrateLegacyLayout();
  const seed: Workspace = legacy ?? {
    id: newId(),
    name: 'Default',
    layout: defaultLayout()
  };
  return { workspaces: [seed], activeId: seed.id };
}

// ── Controller ────────────────────────────────────────────────────

export interface WorkspaceStoreController {
  readonly workspaces: Workspace[];
  /** Currently-active workspace. Always defined — the store keeps
   *  workspaces.length >= 1 and the activeId pointing at a real
   *  entry. */
  readonly active: Workspace;
  activeId: string;
  /** The leaf the user most recently interacted with in the active
   *  workspace. Workspace-scoped: switching workspaces resets it to
   *  the new active's first leaf. Drives where `g w` lands and what
   *  PaneSlot tints as focused. Always points at a real leaf. */
  readonly focusedLeafId: string;
  /** Set the focused leaf in the active workspace. Silent no-op when
   *  the id doesn't belong to the active layout. */
  focus(leafId: string): void;

  // Workspace CRUD.
  create(name?: string): void;
  rename(id: string, name: string): void;
  remove(id: string): void;

  // Active-workspace layout mutations. Each one is a tree-shape
  // operation against the active workspace's split-tree.

  /** Replace the pane kind in a leaf. */
  setPane(leafId: string, pane: PaneKind): void;
  /** Update the gutter ratio on a split. */
  setRatio(splitId: string, ratio: number): void;
  /** Split a leaf into two — the existing leaf becomes the first
   *  child, a fresh leaf with `newPane` becomes the second. */
  split(leafId: string, direction: Direction, newPane: PaneKind): void;
  /** Close a leaf. The parent split collapses into the sibling
   *  subtree. The store never closes the LAST leaf so the shell
   *  can't render empty — those calls are no-ops. */
  close(leafId: string): void;
}

// Module-level singleton. The StatusBar (workspace pills) and the
// /workspace route share this instance so a switch / rename / create
// in one surface shows up instantly in the other. Tests can still
// call createWorkspaceStore() to build isolated instances.
let _singleton: WorkspaceStoreController | null = null;
export function workspaceStoreSingleton(): WorkspaceStoreController {
  if (_singleton) return _singleton;
  _singleton = createWorkspaceStore();
  return _singleton;
}

export function createWorkspaceStore(): WorkspaceStoreController {
  const initial = loadInitial();
  let workspaces = $state<Workspace[]>(initial.workspaces);
  let activeId = $state<string>(initial.activeId);

  // Guarantee the active id always points at a real workspace.
  $effect(() => {
    if (!workspaces.find((w) => w.id === activeId)) {
      activeId = workspaces[0]?.id ?? '';
    }
  });

  $effect(() => saveStored(STORE_KEY, { workspaces, activeId }));

  let active = $derived<Workspace>(
    workspaces.find((w) => w.id === activeId) ?? workspaces[0]
  );

  // Focused leaf — workspace-scoped, transient (no persistence). Keep
  // it in sync with the layout: when the active workspace changes or
  // when the layout no longer contains the focused id (a split, close,
  // or pane swap that removed it), snap to the first leaf so the
  // store's external contract — "always a real leaf" — holds.
  let focusedLeafId = $state<string>('');
  $effect(() => {
    const ids = leaves(active.layout).map((l) => l.id);
    if (ids.length === 0) {
      focusedLeafId = '';
      return;
    }
    if (!ids.includes(focusedLeafId)) {
      focusedLeafId = ids[0];
    }
  });

  function focus(leafId: string) {
    const ids = leaves(active.layout).map((l) => l.id);
    if (ids.includes(leafId)) focusedLeafId = leafId;
  }

  function patchActiveLayout(next: TreeNode) {
    workspaces = workspaces.map((w) =>
      w.id === activeId ? { ...w, layout: next } : w
    );
  }

  function setPane(leafId: string, pane: PaneKind) {
    patchActiveLayout(updatePaneTree(active.layout, leafId, pane));
  }

  function setRatio(splitId: string, ratio: number) {
    patchActiveLayout(updateRatioTree(active.layout, splitId, ratio));
  }

  function split(leafId: string, direction: Direction, newPane: PaneKind) {
    patchActiveLayout(splitLeafTree(active.layout, leafId, direction, newPane));
  }

  function close(leafId: string) {
    // Refuse to close the very last leaf — the shell needs at least
    // one to render.
    if (leaves(active.layout).length <= 1) return;
    patchActiveLayout(closeLeafTree(active.layout, leafId));
  }

  function create(name?: string) {
    const used = new Set(workspaces.map((w) => w.name));
    let nextName = name?.trim() || 'Workspace';
    if (!name) {
      let n = 1;
      while (used.has(n === 1 ? nextName : `${nextName} ${n}`)) n++;
      if (n > 1) nextName = `${nextName} ${n}`;
    }
    const fresh: Workspace = {
      id: newId(),
      name: nextName,
      layout: makeLeaf('tasks')
    };
    workspaces = [...workspaces, fresh];
    activeId = fresh.id;
  }

  function rename(id: string, name: string) {
    const trimmed = name.trim();
    if (!trimmed) return;
    workspaces = workspaces.map((w) =>
      w.id === id ? { ...w, name: trimmed } : w
    );
  }

  function remove(id: string) {
    if (workspaces.length <= 1) return;
    workspaces = workspaces.filter((w) => w.id !== id);
  }

  return {
    get workspaces() {
      return workspaces;
    },
    get active() {
      return active;
    },
    get activeId() {
      return activeId;
    },
    set activeId(v) {
      activeId = v;
    },
    get focusedLeafId() {
      return focusedLeafId;
    },
    focus,
    create,
    rename,
    remove,
    setPane,
    setRatio,
    split,
    close
  };
}
