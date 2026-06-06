// Habit stack-anchor edit handler ("after I do X, I do this").
//
// Fifth extraction step out of routes/habits/+page.svelte. Owns the
// inline-edit buffer (editingStack + stackDraft) + the start / cancel
// / submit triple that PUTs the new anchor to the server.
//
// Behavioural-science staple: anchoring a new habit to an existing
// completed action makes consistency easier than willpower. Persisted
// server-side in .granit/habits-stacks.json via PUT
// /api/v1/habits/{name}/stack — same sidecar the TUI reads.
//
// Busy + reload + toast all funnel through deps so this stays a
// pure-UI buffer with no cross-controller imports — same shape as
// renameCtl.

import { api, type HabitInfo } from '$lib/api';

export interface HabitsStackEditDeps {
  /** Single-cell busy key set on the data controller. The page passes
   *  the dataCtl busy setter so a stack edit in flight disables the
   *  row's other actions. */
  setBusy: (key: string | null) => void;
  /** Reload after a successful stack update. */
  reload: () => Promise<void>;
  /** Toast hook — injected so this controller doesn't have to
   *  import the toast singleton. */
  onError: (message: string) => void;
}

export interface HabitsStackEditController {
  editingStack: string | null;
  stackDraft: string;
  startStackEdit(h: HabitInfo): void;
  cancelStackEdit(): void;
  submitStackEdit(name: string): Promise<void>;
}

export function createHabitsStackEdit(deps: HabitsStackEditDeps): HabitsStackEditController {
  let editingStack = $state<string | null>(null);
  let stackDraft = $state('');

  function startStackEdit(h: HabitInfo) {
    editingStack = h.name;
    stackDraft = h.stackAfter ?? '';
  }
  function cancelStackEdit() {
    editingStack = null;
    stackDraft = '';
  }
  async function submitStackEdit(name: string) {
    const next = stackDraft.trim();
    deps.setBusy(name);
    try {
      await api.setHabitStack(name, next);
      editingStack = null;
      stackDraft = '';
      await deps.reload();
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      deps.onError(`stack update failed: ${msg}`);
    } finally {
      deps.setBusy(null);
    }
  }

  return {
    get editingStack() { return editingStack; },
    set editingStack(v) { editingStack = v; },
    get stackDraft() { return stackDraft; },
    set stackDraft(v) { stackDraft = v; },
    startStackEdit,
    cancelStackEdit,
    submitStackEdit
  };
}
