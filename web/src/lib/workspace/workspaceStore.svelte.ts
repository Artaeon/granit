// Workspace state — the "named layouts" part of the granit vision.
//
// Owns the array of saved workspaces plus the active-workspace id.
// Each workspace is a small persisted struct: a stable id, a
// user-named label, and the two-pane layout (left pane kind, right
// pane kind, gutter ratio).
//
// Persistence: localStorage key `granit.workspaces`. A v0 migration
// reads the older `granit.workspace.layout` single-layout shape and
// folds it into a single "Default" workspace on first run, so users
// upgrading from the prototype shell keep their layout.
//
// Lives in lib/workspace because the workspace route + slot
// components both read this store. The future Phase 3 "Phase 3 of
// the vision" (workspace persistence into `.granit/workspaces.json`)
// will swap loadStored/saveStored for an API call without changing
// any consumer.

import { loadStored, saveStored } from '$lib/util/storage';
import type { PaneKind } from './paneRegistry';

const STORE_KEY = 'granit.workspaces';
const LEGACY_LAYOUT_KEY = 'granit.workspace.layout';

export type WorkspaceLayout = {
  left: PaneKind;
  right: PaneKind;
  /** 0.1 .. 0.9 — left-pane width fraction. */
  ratio: number;
};

export type Workspace = {
  id: string;
  name: string;
  layout: WorkspaceLayout;
};

const DEFAULT_LAYOUT: WorkspaceLayout = {
  left: 'tasks',
  right: 'calendar',
  ratio: 0.5
};

// Small id helper — no crypto.randomUUID() so we don't depend on the
// secure context being available. Eight base36 digits is plenty for
// the handful of workspaces a user will ever have.
function newId(): string {
  let id = '';
  for (let i = 0; i < 8; i++) id += Math.floor(Math.random() * 36).toString(36);
  return id;
}

function isLayout(x: unknown): x is WorkspaceLayout {
  if (!x || typeof x !== 'object') return false;
  const o = x as Record<string, unknown>;
  return typeof o.left === 'string' && typeof o.right === 'string' && typeof o.ratio === 'number';
}

type PersistedState = { workspaces: Workspace[]; activeId: string };

function migrateLegacyLayout(): Workspace | null {
  const raw = loadStored<unknown>(LEGACY_LAYOUT_KEY, null);
  if (!isLayout(raw)) return null;
  return {
    id: newId(),
    name: 'Default',
    layout: { ...raw, ratio: Math.min(0.9, Math.max(0.1, raw.ratio)) }
  };
}

function loadInitial(): PersistedState {
  const stored = loadStored<PersistedState | null>(STORE_KEY, null);
  if (
    stored &&
    Array.isArray(stored.workspaces) &&
    stored.workspaces.length > 0 &&
    typeof stored.activeId === 'string'
  ) {
    return stored;
  }
  // First-run or migration path. Fold the legacy single-layout
  // store (if any) into a Default workspace so existing users
  // don't lose their setup.
  const legacy = migrateLegacyLayout();
  const seed: Workspace = legacy ?? {
    id: newId(),
    name: 'Default',
    layout: { ...DEFAULT_LAYOUT }
  };
  return { workspaces: [seed], activeId: seed.id };
}

export interface WorkspaceStoreController {
  /** All saved workspaces, in user-ordered display order. */
  readonly workspaces: Workspace[];
  /** The currently-active workspace. Always defined because the
   *  store guarantees workspaces.length >= 1. */
  readonly active: Workspace;
  /** Active workspace's id. Bindable so the tray can drive it
   *  directly via click handlers. */
  activeId: string;
  /** Patch the active workspace's layout. Used by the gutter-drag
   *  + slot pickers in the workspace route. */
  patchActiveLayout(patch: Partial<WorkspaceLayout>): void;
  /** Append a new workspace and switch to it. */
  create(name?: string): void;
  /** Rename a workspace in place. */
  rename(id: string, name: string): void;
  /** Drop a workspace. No-op when only one remains — the store
   *  guarantees at least one workspace at all times so the user
   *  can't end up with an empty tray. */
  remove(id: string): void;
}

export function createWorkspaceStore(): WorkspaceStoreController {
  const initial = loadInitial();
  let workspaces = $state<Workspace[]>(initial.workspaces);
  let activeId = $state<string>(initial.activeId);

  // Guarantee the active id always points at a real workspace.
  // Without this, deleting the active workspace would orphan the
  // pointer and the tray would render an empty pane.
  $effect(() => {
    if (!workspaces.find((w) => w.id === activeId)) {
      activeId = workspaces[0]?.id ?? '';
    }
  });

  $effect(() => saveStored(STORE_KEY, { workspaces, activeId }));

  let active = $derived<Workspace>(
    workspaces.find((w) => w.id === activeId) ?? workspaces[0]
  );

  function patchActiveLayout(patch: Partial<WorkspaceLayout>) {
    workspaces = workspaces.map((w) =>
      w.id === activeId ? { ...w, layout: { ...w.layout, ...patch } } : w
    );
  }

  function create(name?: string) {
    const used = new Set(workspaces.map((w) => w.name));
    let nextName = name?.trim() || 'Workspace';
    if (!name) {
      // Auto-name when no name is provided. "Workspace", then
      // "Workspace 2", "Workspace 3", etc.
      let n = 1;
      while (used.has(n === 1 ? nextName : `${nextName} ${n}`)) n++;
      if (n > 1) nextName = `${nextName} ${n}`;
    }
    const fresh: Workspace = {
      id: newId(),
      name: nextName,
      layout: { ...DEFAULT_LAYOUT }
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
    if (workspaces.length <= 1) return; // always keep at least one
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
    patchActiveLayout,
    create,
    rename,
    remove
  };
}
