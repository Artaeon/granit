// Workspace commands surfaced through the command palette so every
// pane-tree operation (split, close, swap pane kind) is reachable via
// ⌘K / ⌘P without leaving the keyboard. VSCode's command-palette
// model: keep chords for the few hot ones, everything else is a
// palette entry.
//
// Pure module — accepts the singleton store, returns a flat array of
// {label, detail, icon, run}. Index for fuzzy matching happens at the
// palette layer, so this file knows nothing about scoring or recents.

import { goto } from '$app/navigation';
import { PANES, findPane, type PaneKind } from './paneRegistry';
import { workspaceStoreSingleton } from './workspaceStore.svelte';
import { leaves } from './splitTree';
import { WORKSPACE_PRESETS } from './workspacePresets';
import { toast } from '$lib/components/toast';

export type WorkspaceCmd = {
  /** Stable id, used by the palette for recency boosts. */
  id: string;
  label: string;
  /** Greyed-out hint shown next to the label. */
  detail: string;
  /** NavIcon name — keep to icons already in the catalog. */
  icon: string;
  /** Side-effect. The palette closes itself before invoking. */
  run: () => void;
};

const SPLIT_DEFAULT_PANE: PaneKind = 'notes';

// Pick the pane the workspace commands should default to when the
// caller wants a "new" pane — the first one that isn't currently in
// the focused leaf, falling back to the registry default. Mirrors
// PaneSlot's nextPaneCandidate so split-from-palette feels the same
// as split-from-header.
function differentPane(current: PaneKind): PaneKind {
  return PANES.find((p) => p.id !== current)?.id ?? SPLIT_DEFAULT_PANE;
}

export function workspaceCommands(): WorkspaceCmd[] {
  const store = workspaceStoreSingleton();
  const focusedId = store.focusedLeafId;
  const activeLeaves = leaves(store.active.layout);
  const focusedLeaf = activeLeaves.find((l) => l.id === focusedId) ?? activeLeaves[0];
  const focusedPaneKind = focusedLeaf?.pane;
  const focusedLabel = focusedPaneKind ? findPane(focusedPaneKind)?.label ?? focusedPaneKind : '';
  const canClose = activeLeaves.length > 1;

  const out: WorkspaceCmd[] = [
    {
      id: 'workspace:open',
      label: 'Open workspace',
      detail: store.active?.name ?? '/workspace',
      icon: 'workspace',
      run: () => goto('/workspace')
    }
  ];

  // Preset constructors — "New workspace: Daily" etc. Each one builds
  // a fresh tree via the preset and jumps to /workspace so the user
  // sees what they just made.
  for (const p of WORKSPACE_PRESETS) {
    out.push({
      id: 'workspace:new:' + p.id,
      label: `New workspace: ${p.name}`,
      detail: p.detail,
      icon: 'workspace',
      run: () => {
        store.createWithLayout(p.name, p.buildLayout());
        void goto('/workspace');
      }
    });
  }

  // Backup / portability. Clipboard pair — the user copies JSON out,
  // pastes it in. Pairs nicely with the CLI `granit backup` for
  // file-on-disk safety while staying KISS in the browser.
  out.push({
    id: 'workspace:export',
    label: 'Export active workspace to clipboard',
    detail: store.active?.name ?? '—',
    icon: 'workspace',
    run: async () => {
      try {
        await navigator.clipboard.writeText(store.exportActiveAsJSON());
        toast.success(`Copied "${store.active.name}" as JSON`);
      } catch {
        toast.error('Clipboard write failed — check browser permissions');
      }
    }
  });
  out.push({
    id: 'workspace:import',
    label: 'Import workspace from clipboard',
    detail: 'Paste JSON exported from another granit',
    icon: 'workspace',
    run: async () => {
      let json = '';
      try {
        json = await navigator.clipboard.readText();
      } catch {
        toast.error('Clipboard read failed — check browser permissions');
        return;
      }
      if (!json.trim()) {
        toast.warning('Clipboard is empty');
        return;
      }
      const err = store.importFromJSON(json);
      if (err) toast.error('Import failed: ' + err);
      else {
        toast.success(`Imported workspace "${store.active.name}"`);
        void goto('/workspace');
      }
    }
  });

  if (focusedLeaf && focusedPaneKind) {
    const target = differentPane(focusedPaneKind);
    out.push({
      id: 'workspace:split-h',
      label: 'Split focused leaf horizontally',
      detail: `${focusedLabel} → split right`,
      icon: 'workspace',
      run: () => {
        store.split(focusedLeaf.id, 'h', target);
        void goto('/workspace');
      }
    });
    out.push({
      id: 'workspace:split-v',
      label: 'Split focused leaf vertically',
      detail: `${focusedLabel} → split below`,
      icon: 'workspace',
      run: () => {
        store.split(focusedLeaf.id, 'v', target);
        void goto('/workspace');
      }
    });
    if (canClose) {
      out.push({
        id: 'workspace:close',
        label: 'Close focused leaf',
        detail: focusedLabel,
        icon: 'workspace',
        run: () => store.close(focusedLeaf.id)
      });
    }
    for (const p of PANES) {
      if (p.id === focusedPaneKind) continue;
      out.push({
        id: 'workspace:set-pane:' + p.id,
        label: `Set focused pane → ${p.label}`,
        detail: `${focusedLabel} → ${p.label}`,
        icon: 'workspace',
        run: () => {
          store.setPane(focusedLeaf.id, p.id);
          void goto('/workspace');
        }
      });
    }
  }

  return out;
}
