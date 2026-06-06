// Rename + delete handlers for the habits surface.
//
// Fourth extraction step out of routes/habits/+page.svelte. Owns the
// inline-edit buffer for renames (editingName + renameDraft + start /
// cancel / submit) and the confirm-then-call delete flow.
//
// Habits have no record file — both handlers rewrite the underlying
// `## Habits` checkbox lines across every daily note. The backend
// handles the cross-file scan; this controller just collects the new
// name (or a destructive-action confirmation) and triggers it, then
// nudges the data controller to reload.
//
// Persisted weekly targets follow the rename: a habit's target lives
// in localStorage keyed by name, so renaming Foo -> Bar copies the
// Foo entry to Bar and drops the original. Delete drops the orphan.
//
// Toast + busy flag both go through deps so this controller stays
// free of the data controller import — same shape goalsData /
// financeData use for cross-controller wiring.

import { api, type HabitInfo } from '$lib/api';
import { habitTargets, setHabitTarget } from '$lib/habits/targets';
import { get } from 'svelte/store';

export interface HabitsRenameDeps {
  /** Single-cell busy key set on the data controller. The page passes
   *  the dataCtl busy setter so a rename / delete in flight disables
   *  the row's other actions. */
  setBusy: (key: string | null) => void;
  /** Reload after a successful rename / delete. */
  reload: () => Promise<void>;
  /** Toast hooks — injected so this controller doesn't have to
   *  import the toast singleton. */
  onSuccess: (message: string) => void;
  onError: (message: string) => void;
}

export interface HabitsRenameController {
  editingName: string | null;
  renameDraft: string;
  startRename(h: HabitInfo): void;
  cancelRename(): void;
  submitRename(oldName: string): Promise<void>;
  deleteHabit(name: string): Promise<void>;
}

export function createHabitsRename(deps: HabitsRenameDeps): HabitsRenameController {
  let editingName = $state<string | null>(null);
  let renameDraft = $state('');

  function startRename(h: HabitInfo) {
    editingName = h.name;
    renameDraft = h.name;
  }
  function cancelRename() {
    editingName = null;
    renameDraft = '';
  }

  async function submitRename(oldName: string) {
    const next = renameDraft.trim();
    if (!next || next === oldName) {
      cancelRename();
      return;
    }
    deps.setBusy(oldName);
    try {
      const res = await api.renameHabit(oldName, next);
      cancelRename();
      // Migrate any persisted weekly target so the new name keeps its goal.
      const tgt = get(habitTargets)[oldName];
      if (tgt != null) {
        setHabitTarget(next, tgt);
        setHabitTarget(oldName, null);
      }
      await deps.reload();
      deps.onSuccess(
        `renamed · ${res.filesTouched} ${res.filesTouched === 1 ? 'daily' : 'dailies'} updated`
      );
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      deps.onError(`rename failed: ${msg}`);
    } finally {
      deps.setBusy(null);
    }
  }

  async function deleteHabit(name: string) {
    if (!confirm(
      `Delete habit "${name}"?\n\nThis strips every checkbox line under ## Habits across every daily note in your vault — past streak data for this habit is gone. The daily notes themselves stay; only the matching lines are removed.`
    )) return;
    deps.setBusy(name);
    try {
      const res = await api.deleteHabit(name);
      // Drop any persisted target — it's now orphaned.
      setHabitTarget(name, null);
      await deps.reload();
      deps.onSuccess(
        `deleted · ${res.filesTouched} ${res.filesTouched === 1 ? 'daily' : 'dailies'} cleaned`
      );
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      deps.onError(`delete failed: ${msg}`);
    } finally {
      deps.setBusy(null);
    }
  }

  return {
    get editingName() { return editingName; },
    set editingName(v) { editingName = v; },
    get renameDraft() { return renameDraft; },
    set renameDraft(v) { renameDraft = v; },
    startRename,
    cancelRename,
    submitRename,
    deleteHabit
  };
}
