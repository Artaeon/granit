// Inline category-edit controller for the habits surface.
//
// Each habit card shows its current category as a chip; clicking
// opens a small picker bound to this controller. The picker lists
// known categories (loaded lazily via api.listHabitCategories) and
// accepts a free-text value so the user can introduce a new bucket
// without leaving the page.
//
// The controller is a factory — mirroring tasksGroupAdd — so the
// route owns one instance and asks it which habit's editor is open.
// Commit happens via the onPatch dep so the route can refresh /
// optimistic-update its loaded data; this controller stays UI-only.
//
// Cancel-on-Esc + commit-on-Enter live in the consuming component;
// this file only owns open/close state, the draft string, and the
// memoised category list.

import { api } from '$lib/api';

export interface CategoryEditController {
  /** Name of the habit whose editor is open. null when nothing open. */
  openFor: string | null;
  /** Draft text the user is typing / picking. */
  draft: string;
  /** True while a patch is in flight — disables double-submit. */
  readonly busy: boolean;
  /** Memoised known categories. Populated on first loadCategories(). */
  readonly categories: string[];

  /** Open the editor for `habitName`, pre-filling the draft with
   *  the habit's current category (so the picker shows the current
   *  selection without surprise). Triggers a category-list refresh
   *  in the background. */
  open(habitName: string, current: string | undefined): void;
  /** Close without committing. */
  cancel(): void;
  /** Lazy-load the list of known categories. Memoised — repeated
   *  calls within a session reuse the cached value. Pass force=true
   *  after a successful patch to pick up newly-created categories. */
  loadCategories(force?: boolean): Promise<void>;
  /** Commit the draft for `habitName`. Empty draft clears the
   *  category (server: setting "" removes the sidecar entry).
   *  Calls onPatch on success so the caller can reload + close. */
  commit(habitName: string): Promise<void>;
}

export type CategoryEditDeps = {
  /** Called after a successful patch. The route uses this to reload
   *  its habit list (or optimistically merge the server response). */
  onPatch: (habitName: string, category: string) => void | Promise<void>;
};

export function createCategoryEditCtl(
  deps: CategoryEditDeps
): CategoryEditController {
  let openFor = $state<string | null>(null);
  let draft = $state('');
  let busy = $state(false);
  let categories = $state<string[]>([]);
  let loaded = false;
  let loading: Promise<void> | null = null;

  function open(habitName: string, current: string | undefined) {
    openFor = habitName;
    draft = current ?? '';
    // Fire-and-forget — the picker renders fine with an empty list
    // while the first request lands.
    void loadCategories();
  }

  function cancel() {
    openFor = null;
    draft = '';
  }

  async function loadCategories(force = false): Promise<void> {
    if (loaded && !force) return;
    if (loading && !force) return loading;
    loading = (async () => {
      try {
        const r = await api.listHabitCategories();
        categories = (r.categories ?? []).slice().sort((a, b) => a.localeCompare(b));
        loaded = true;
      } catch {
        // Soft-fail — the picker still works as a free-text input.
        categories = [];
      } finally {
        loading = null;
      }
    })();
    return loading;
  }

  async function commit(habitName: string): Promise<void> {
    if (busy) return;
    const next = draft.trim();
    busy = true;
    try {
      await deps.onPatch(habitName, next);
      openFor = null;
      draft = '';
      // Refresh categories so a freshly-introduced bucket appears
      // in subsequent pickers without a hard reload.
      if (next) void loadCategories(true);
    } finally {
      busy = false;
    }
  }

  return {
    get openFor() { return openFor; },
    set openFor(v) { openFor = v; },
    get draft() { return draft; },
    set draft(v) { draft = v; },
    get busy() { return busy; },
    get categories() { return categories; },
    open,
    cancel,
    loadCategories,
    commit
  };
}
