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
import { recentPanes } from './recentPanes.svelte';
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
  /** NavIcon name — gives each workspace its own glanceable
   *  identity in the StatusBar pills + future shell chrome. The
   *  default 'workspace' (a 2x2 grid) matches the BottomNav glyph
   *  so a never-customised workspace still reads as "workspace". */
  icon?: string;
  /** Layout is a split-tree. Migrations rebuild older flat shapes
   *  into a horizontal split on first read. */
  layout: TreeNode;
};

function defaultLayout(): TreeNode {
  // A fresh workspace opens as the dashboard canvas — the Today widgets
  // — so the workspace reads as a "desktop" out of the box. Users split
  // / swap in other panes (tasks, calendar, …) from there.
  return makeLeaf('dashboard');
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
  // Optional icon field; pre-W7 entries don't carry it. Skip silently
  // when missing; the consumers default to 'workspace' on read.
  const icon = typeof o.icon === 'string' && o.icon ? o.icon : undefined;
  // v2 already-a-tree path.
  if (isTree(o.layout)) {
    return { id, name, icon, layout: o.layout };
  }
  // v1 flat-layout path — rebuild into a tree.
  if (isFlatLayout(o.layout)) {
    return { id, name, icon, layout: fromFlat(o.layout.left, o.layout.right, o.layout.ratio) };
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
  /** When set, the /workspace shell renders ONLY this leaf — splits
   *  are hidden but their tree is preserved. Transient (cleared on
   *  active-workspace switch or when the leaf disappears). null
   *  means the normal multi-leaf SplitView is in charge. */
  readonly maximizedLeafId: string | null;
  /** Toggle the maximize state for the given leaf. Maximizing a
   *  different leaf swaps the target; calling on the already-
   *  maximized leaf unmaximizes. */
  toggleMaximize(leafId: string): void;

  // Workspace CRUD.
  create(name?: string): void;
  /** Create a new workspace with a custom starting layout. Used by
   *  the preset commands ("New workspace: Daily" etc.). The layout
   *  must be a fresh tree (new ids); the caller owns construction.
   *  Names follow the same dedupe rule as create(): if `name` is
   *  taken, suffixes ` 2`, ` 3`, … until unique. */
  createWithLayout(name: string, layout: TreeNode): void;
  rename(id: string, name: string): void;
  /** Set the NavIcon name for a workspace. Empty/whitespace falls
   *  back to the default 'workspace' glyph. */
  setIcon(id: string, icon: string): void;
  remove(id: string): void;

  // Active-workspace layout mutations. Each one is a tree-shape
  // operation against the active workspace's split-tree.

  /** Replace the pane kind in a leaf. */
  setPane(leafId: string, pane: PaneKind): void;
  /** Swap the pane kinds in two leaves. Both must exist in the active
   *  layout; no-op on equal ids, unknown ids, or identical panes.
   *  Used by the header drag-and-drop in PaneSlot. */
  swap(leafIdA: string, leafIdB: string): void;
  /** Update the gutter ratio on a split. */
  setRatio(splitId: string, ratio: number): void;
  /** Split a leaf into two — the existing leaf becomes the first
   *  child, a fresh leaf with `newPane` becomes the second. */
  split(leafId: string, direction: Direction, newPane: PaneKind): void;
  /** Close a leaf. The parent split collapses into the sibling
   *  subtree. The store never closes the LAST leaf so the shell
   *  can't render empty — those calls are no-ops. */
  close(leafId: string): void;

  // Backup / portability. Layouts are persisted to localStorage, so
  // moving between devices or recovering from a wipe needs a portable
  // representation. JSON round-trips through the same normalize +
  // isTree path the legacy migrations use.

  /** Serialise the active workspace to JSON. Includes its name and
   *  layout (with original ids). */
  exportActiveAsJSON(): string;
  /** Parse a JSON workspace and append it as a new entry. Returns
   *  null on success, or an error string. The new workspace becomes
   *  active so the user immediately sees what they imported. Names
   *  go through the same uniqueName dedupe as create(). */
  importFromJSON(json: string): string | null;
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
  // Maximized leaf — transient too. When non-null, the workspace shell
  // renders only this leaf. The same self-healing rule applies: if the
  // leaf disappears (workspace switch, close, split-collapse), reset
  // to null so the user lands back in the normal SplitView.
  let maximizedLeafId = $state<string | null>(null);
  $effect(() => {
    const ids = leaves(active.layout).map((l) => l.id);
    if (ids.length === 0) {
      focusedLeafId = '';
      maximizedLeafId = null;
      return;
    }
    if (!ids.includes(focusedLeafId)) {
      focusedLeafId = ids[0];
    }
    if (maximizedLeafId !== null && !ids.includes(maximizedLeafId)) {
      maximizedLeafId = null;
    }
  });

  function focus(leafId: string) {
    const ids = leaves(active.layout).map((l) => l.id);
    if (ids.includes(leafId)) focusedLeafId = leafId;
  }

  function toggleMaximize(leafId: string) {
    const ids = leaves(active.layout).map((l) => l.id);
    if (!ids.includes(leafId)) return;
    // Maximizing a leaf that's already maximized → unmaximize.
    // Maximizing a different leaf while another is maximized →
    // switch to the new one (matches VSCode's behaviour).
    maximizedLeafId = maximizedLeafId === leafId ? null : leafId;
  }

  function patchActiveLayout(next: TreeNode) {
    workspaces = workspaces.map((w) =>
      w.id === activeId ? { ...w, layout: next } : w
    );
  }

  function setPane(leafId: string, pane: PaneKind) {
    patchActiveLayout(updatePaneTree(active.layout, leafId, pane));
    // Record into the MRU so palette "Recent:" entries reflect every
    // pane swap regardless of where it was triggered (header picker,
    // palette command, programmatic route → pane promotion).
    recentPanes.push(pane);
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

  function swap(leafIdA: string, leafIdB: string) {
    if (leafIdA === leafIdB) return;
    const allLeaves = leaves(active.layout);
    const a = allLeaves.find((l) => l.id === leafIdA);
    const b = allLeaves.find((l) => l.id === leafIdB);
    if (!a || !b || a.pane === b.pane) return;
    // Two sequential setPane calls — the underlying patches collapse
    // into the same effect-tick from the consumer's perspective.
    const aPane = a.pane;
    setPane(leafIdA, b.pane);
    setPane(leafIdB, aPane);
  }

  // Pick a name that doesn't collide with an existing workspace. If
  // `desired` is taken, suffix with ' 2', ' 3', …. Empty / missing
  // falls back to "Workspace".
  function uniqueName(desired?: string): string {
    const used = new Set(workspaces.map((w) => w.name));
    const base = desired?.trim() || 'Workspace';
    if (!used.has(base)) return base;
    let n = 2;
    while (used.has(`${base} ${n}`)) n++;
    return `${base} ${n}`;
  }

  function create(name?: string) {
    const fresh: Workspace = {
      id: newId(),
      name: uniqueName(name),
      layout: makeLeaf('tasks')
    };
    workspaces = [...workspaces, fresh];
    activeId = fresh.id;
  }

  function createWithLayout(name: string, layout: TreeNode) {
    const fresh: Workspace = {
      id: newId(),
      name: uniqueName(name),
      layout
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

  function setIcon(id: string, icon: string) {
    const trimmed = icon.trim();
    workspaces = workspaces.map((w) =>
      w.id === id ? { ...w, icon: trimmed || undefined } : w
    );
  }

  function remove(id: string) {
    if (workspaces.length <= 1) return;
    workspaces = workspaces.filter((w) => w.id !== id);
  }

  function exportActiveAsJSON(): string {
    // Strip the id so an import doesn't collide on re-import-on-same-
    // device. The store assigns a fresh id at the import boundary.
    return JSON.stringify({ name: active.name, layout: active.layout }, null, 2);
  }

  function importFromJSON(json: string): string | null {
    let parsed: unknown;
    try {
      parsed = JSON.parse(json);
    } catch (e) {
      return 'Invalid JSON: ' + (e instanceof Error ? e.message : String(e));
    }
    if (!parsed || typeof parsed !== 'object') return 'Expected a workspace object';
    const o = parsed as Record<string, unknown>;
    const name = typeof o.name === 'string' && o.name.trim() ? o.name : 'Imported';
    if (!isTree(o.layout)) return 'Invalid layout shape';
    const fresh: Workspace = {
      id: newId(),
      name: uniqueName(name),
      layout: o.layout
    };
    workspaces = [...workspaces, fresh];
    activeId = fresh.id;
    return null;
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
    get maximizedLeafId() {
      return maximizedLeafId;
    },
    focus,
    toggleMaximize,
    create,
    createWithLayout,
    rename,
    setIcon,
    remove,
    setPane,
    swap,
    setRatio,
    split,
    close,
    exportActiveAsJSON,
    importFromJSON
  };
}
