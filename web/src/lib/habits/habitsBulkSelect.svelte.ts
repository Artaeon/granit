// Bulk-select controller — drives the "Select" mode on the habits
// page. While `active`, each habit card renders a checkbox and the
// footer action bar shows aggregate operations (Archive N, Set
// category, Add tag). Parallel patch fan-out via Promise.all so 10
// archives don't queue 10 sequential round-trips; failures are
// collected and surfaced as a single coalesced toast — the rest of
// the batch still lands.
//
// The controller doesn't own the habits list (the page does). It
// just keeps a Set<string> of selected names and reads habits via
// the deps bundle when it needs to derive (e.g. "do all selected
// already have this tag"). Standard small-interface pattern.

import { api, type HabitInfo } from '$lib/api';

export interface BulkSelectControllerDeps {
  /** Current habits list — read fresh on every derivation so
   *  the controller sees post-reload changes without plumbing
   *  reactivity through props. */
  getHabits: () => HabitInfo[];
  /** Refresh after a bulk patch lands. */
  reload: () => Promise<void>;
  /** Optional per-patch hook (e.g. for telemetry). Called once per
   *  successful patch, not once per batch. */
  onPatch?: (name: string) => void;
}

export interface BulkSelectController {
  /** Whether bulk-select mode is on. Toggling off clears the
   *  selection so re-entering starts clean. */
  active: boolean;
  /** Names currently selected. Set semantics — order doesn't matter,
   *  click toggles membership. */
  readonly selected: Set<string>;
  /** Read-only derived count for the action-bar label ("Archive 3"). */
  readonly count: number;
  /** Whether a bulk operation is currently in flight. The action
   *  bar disables its buttons while this is true. */
  readonly busy: boolean;

  /** Enter / leave bulk-select mode. */
  toggleActive(): void;
  /** Force-leave bulk mode (Cancel button on the action bar). */
  cancel(): void;
  /** Toggle a single habit's membership in the selection. */
  toggle(name: string): void;
  /** True when the named habit is in the current selection.
   *  Used by the card checkbox to render its state. */
  isSelected(name: string): boolean;
  /** Select every habit in `names`. The page uses this for
   *  "select all visible" if it ever surfaces that. */
  selectAll(names: string[]): void;
  /** Clear the selection without leaving bulk mode. */
  clear(): void;

  /** Archive every selected habit (patchHabit archived=true) in
   *  parallel. Exits bulk-mode on success. */
  archiveSelected(): Promise<void>;
  /** Patch `category` on every selected habit. */
  setCategoryForSelected(category: string): Promise<void>;
  /** Append `tag` to the `tags` array of every selected habit.
   *  Skips habits that already carry the tag. */
  addTagToSelected(tag: string): Promise<void>;
}

export function createBulkSelectCtl(
  deps: BulkSelectControllerDeps
): BulkSelectController {
  let active = $state(false);
  let selected = $state(new Set<string>());
  let busy = $state(false);

  const count = $derived(selected.size);

  function toggleActive() {
    active = !active;
    if (!active) selected = new Set();
  }
  function cancel() {
    active = false;
    selected = new Set();
  }
  function toggle(name: string) {
    const next = new Set(selected);
    if (next.has(name)) next.delete(name);
    else next.add(name);
    selected = next;
  }
  function isSelected(name: string) {
    return selected.has(name);
  }
  function selectAll(names: string[]) {
    selected = new Set(names);
  }
  function clear() {
    selected = new Set();
  }

  // Generic bulk-patch runner. Takes a per-habit patch builder so
  // archive / set-category / add-tag all share the same fan-out +
  // coalesced-toast + busy flag wiring.
  async function runBulkPatch(
    verb: string,
    buildPatch: (h: HabitInfo) => Parameters<typeof api.patchHabit>[1] | null
  ) {
    if (busy || selected.size === 0) return;
    busy = true;
    const habits = deps.getHabits();
    const targets = habits.filter((h) => selected.has(h.name));
    const failed: string[] = [];
    let touched = 0;
    await Promise.all(
      targets.map(async (h) => {
        const patch = buildPatch(h);
        if (!patch) return; // skip habits that don't need this op
        try {
          await api.patchHabit(h.name, patch);
          deps.onPatch?.(h.name);
          touched++;
        } catch {
          failed.push(h.name);
        }
      })
    );
    busy = false;
    const { toast } = await import('$lib/components/toast');
    if (failed.length === 0 && touched > 0) {
      toast.success(`${verb} ${touched} habit${touched === 1 ? '' : 's'}`);
    } else if (touched > 0) {
      toast.warning(`${verb} ${touched}, failed: ${failed.join(', ')}`);
    } else if (failed.length > 0) {
      toast.error(`bulk ${verb} failed`);
    }
    await deps.reload();
    if (failed.length === 0) cancel();
  }

  async function archiveSelected() {
    await runBulkPatch('archived', () => ({ archived: true }));
  }
  async function setCategoryForSelected(category: string) {
    const cleaned = category.trim();
    if (!cleaned) return;
    await runBulkPatch('categorised', (h) =>
      h.category === cleaned ? null : { category: cleaned }
    );
  }
  async function addTagToSelected(tag: string) {
    const cleaned = tag.trim().replace(/^#/, '');
    if (!cleaned) return;
    await runBulkPatch('tagged', (h) => {
      const cur = h.tags ?? [];
      if (cur.includes(cleaned)) return null;
      return { tags: [...cur, cleaned] };
    });
  }

  return {
    get active() {
      return active;
    },
    set active(v) {
      active = v;
      if (!v) selected = new Set();
    },
    get selected() {
      return selected;
    },
    get count() {
      return count;
    },
    get busy() {
      return busy;
    },
    toggleActive,
    cancel,
    toggle,
    isSelected,
    selectAll,
    clear,
    archiveSelected,
    setCategoryForSelected,
    addTagToSelected
  };
}
