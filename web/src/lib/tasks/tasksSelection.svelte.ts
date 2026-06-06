// Tasks-page cursor + bulk-selection state.
//
//   • cursorIdx     — the j/k keyboard cursor's row index inside the
//                     current filtered list. Reset effect snaps the
//                     cursor back into range whenever the filtered
//                     list changes (a filter that shrinks below the
//                     cursor would otherwise leave it pointing at a
//                     stale row).
//   • selectedIds   — the bulk-selected task ids. A Set so add / drop
//                     stays O(1); the parent reads .size for the
//                     BulkBar visibility gate.
//   • focusCursor   — clamp idx, set cursor, scroll the row into view
//                     via the data-task-id attr.
//   • selectAllOrClear — chord-toggle bulk select. Reselects when
//                        anything's unselected; clears when everything
//                        already is.
//   • openSnoozePickerForCursor — keyboard trigger for the in-card
//                                 snooze picker; cleanest cross-
//                                 component invocation is .click()
//                                 the snooze button via the row's
//                                 data-task-id wrapper.
//
// Controller factory pattern: the caller passes a `getFiltered`
// reader so the controller never owns the source of truth for what's
// on screen; that stays in filterCtl. Selection mutation goes
// through `selectedIds = new Set(...)` reassignment to retrigger
// reactivity.

import { toast } from '$lib/components/toast';
import type { Task } from '$lib/api';

export interface TasksSelectionDeps {
  /** filterCtl.filtered — the list j/k navigates and select-all
   *  operates against. The controller calls it on every read so
   *  cursor reset, focus, and snooze-trigger all see the LIVE
   *  filtered set, not a frozen snapshot. */
  getFiltered: () => Task[];
}

export interface TasksSelectionController {
  cursorIdx: number;
  selectedIds: Set<string>;
  /** Clamp + set the cursor, then scroll the target row into view.
   *  Idempotent when called with the same idx. */
  focusCursor(idx: number): void;
  /** Add / drop a single id from the selection. Reassigns the Set
   *  so Svelte 5 reactivity fires. */
  toggleSelected(id: string): void;
  /** Chord-toggle: if every filtered row is selected, clear; else
   *  select all. Toasts the new state for keyboard users. */
  selectAllOrClear(): void;
  /** Open the snooze picker on the cursor row via the in-card snooze
   *  button. Silent no-op when the row hasn't rendered yet. */
  openSnoozePickerForCursor(): void;
  /** Drop the selection — used after BulkBar actions complete. */
  clear(): void;
}

export function createTasksSelection(deps: TasksSelectionDeps): TasksSelectionController {
  let cursorIdx = $state<number>(-1);
  let selectedIds = $state<Set<string>>(new Set());

  // Reset cursor when the filtered list shrinks past it. We read the
  // whole `filtered` array (not just .length) so any change to the
  // filter pipeline retriggers — a swap that keeps length identical
  // but rearranges items could otherwise leave the cursor pointing
  // at a stale row. The Math.max(0, …) keeps cursorIdx valid (>= 0)
  // even when filtered is empty; cursor-read sites also `?.` against
  // out-of-bounds so a flicker between the effect firing and the
  // render path resolves gracefully.
  $effect(() => {
    const filtered = deps.getFiltered();
    void filtered;
    if (cursorIdx >= filtered.length) {
      cursorIdx = Math.max(0, filtered.length - 1);
    }
  });

  function focusCursor(idx: number) {
    const filtered = deps.getFiltered();
    cursorIdx = Math.max(0, Math.min(filtered.length - 1, idx));
    const t = filtered[cursorIdx];
    if (!t) return;
    queueMicrotask(() => {
      const el = document.querySelector(`[data-task-id="${t.id}"]`) as HTMLElement | null;
      if (el) el.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    });
  }

  function toggleSelected(id: string) {
    const next = new Set(selectedIds);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    selectedIds = next;
  }

  function selectAllOrClear() {
    const filtered = deps.getFiltered();
    if (filtered.length === 0) return;
    const allSelected = filtered.every((t) => selectedIds.has(t.id));
    if (allSelected) {
      selectedIds = new Set();
      toast.info('Selection cleared');
      return;
    }
    selectedIds = new Set(filtered.map((t) => t.id));
    toast.success(`Selected ${filtered.length} task${filtered.length === 1 ? '' : 's'}`);
  }

  function openSnoozePickerForCursor() {
    const filtered = deps.getFiltered();
    const t = cursorIdx >= 0 ? filtered[cursorIdx] : null;
    if (!t) return;
    const row = document.querySelector(`[data-task-id="${t.id}"]`);
    if (!row) return;
    const btn = row.querySelector('button[aria-label="snooze"]') as HTMLButtonElement | null;
    if (btn) btn.click();
  }

  function clear() {
    selectedIds = new Set();
  }

  return {
    get cursorIdx() {
      return cursorIdx;
    },
    set cursorIdx(v) {
      cursorIdx = v;
    },
    get selectedIds() {
      return selectedIds;
    },
    set selectedIds(v) {
      selectedIds = v;
    },
    focusCursor,
    toggleSelected,
    selectAllOrClear,
    openSnoozePickerForCursor,
    clear
  };
}
