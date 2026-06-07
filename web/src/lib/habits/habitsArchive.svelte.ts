// Archive controller — manages the "Show archived" toggle and the
// per-habit restore action. Lives next to the habits page so the
// controller can be embedded in the future workspace pane the same
// way tasksFilterState is.
//
// Pattern matches the other .svelte.ts controllers (tasksFilterState,
// goalsData): small interface, getter/setter pairs for bindable state,
// deps bundle for external concerns (reload + post-patch hook).
// No global side-effects beyond the toggle persisting to localStorage
// so a power user who lives with "show archived" on doesn't have to
// flip it every page load.

import { api } from '$lib/api';
import { loadStoredString, saveStoredString } from '$lib/util/storage';

const SHOW_ARCHIVED_KEY = 'granit.habits.showArchived';

export interface ArchiveControllerDeps {
  /** Refresh the habits list after a patch lands. Same `load()` the
   *  page already exposes — the controller doesn't own the data,
   *  just triggers a re-read so streak / archived flags reconcile. */
  reload: () => Promise<void>;
  /** Optional hook called after every successful patch. Used by the
   *  bulk-select controller to coalesce a single toast across N
   *  restores; the page itself can leave it undefined. */
  onPatch?: (name: string) => void;
}

export interface ArchiveController {
  /** When false (default), archived habits are filtered out of every
   *  view. When true, the archive section renders at the bottom of
   *  the page. Persisted across reloads so a user reviewing their
   *  archive doesn't lose context on refresh. */
  showArchived: boolean;
  /** Per-habit restore busy marker — components disable the Restore
   *  button while the patch is in flight. */
  readonly busy: string | null;
  /** Patch archived=false on the named habit then reload. Errors
   *  surface as a toast (imported lazily so the controller doesn't
   *  pull in components on every page load). */
  restore(name: string): Promise<void>;
  /** Patch archived=true on the named habit. Symmetric to restore;
   *  used by the bulk-action bar's Archive button. */
  archive(name: string): Promise<void>;
}

export function createArchiveCtl(deps: ArchiveControllerDeps): ArchiveController {
  let showArchived = $state(loadStoredString(SHOW_ARCHIVED_KEY, '0') === '1');
  let busy = $state<string | null>(null);

  $effect(() => saveStoredString(SHOW_ARCHIVED_KEY, showArchived ? '1' : '0'));

  async function patch(name: string, archived: boolean) {
    busy = name;
    try {
      await api.patchHabit(name, { archived });
      deps.onPatch?.(name);
      await deps.reload();
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      const { toast } = await import('$lib/components/toast');
      toast.error(`couldn't ${archived ? 'archive' : 'restore'} ${name}: ${msg}`);
    } finally {
      busy = null;
    }
  }

  return {
    get showArchived() {
      return showArchived;
    },
    set showArchived(v) {
      showArchived = v;
    },
    get busy() {
      return busy;
    },
    restore: (name) => patch(name, false),
    archive: (name) => patch(name, true)
  };
}
