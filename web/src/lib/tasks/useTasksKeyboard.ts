// TasksPane page-scoped keyboard handler.
//
// Owns the single window-level keydown listener for the SectionList /
// agenda / week / focus views. Kanban / TriageBoard / EisenhowerView
// install their own column-aware handlers — this listener
// short-circuits when one of those views is active so j/k/x/d/e/p
// don't double-fire.
//
// Pattern follows useKanbanKeyboard: plain TS, $state owned by the
// caller, refs object exposes parent state + actions as small
// callbacks. Lifecycle is one register/cleanup pair; call it from
// onMount and return the cleanup.

import { isTypingTarget } from '$lib/util/isTypingTarget';
import { VIEW_DIGIT_MAP, type View } from './tasksHelpers';
import type { Task } from '$lib/api';

export type TasksKeyboardRefs = {
  // ── Read parent state ──────────────────────────────────────────
  getView: () => View;
  getFiltered: () => Task[];
  getCursorIdx: () => number;
  getSelectionSize: () => number;

  // viewCtl getters/setters — bind through to the controller so
  // the handler doesn't need to know the controller's shape.
  isHelpOpen: () => boolean;
  setHelpOpen: (v: boolean) => void;
  isFilterPanelOpen: () => boolean;
  setFilterPanelOpen: (v: boolean) => void;
  cycleView: (dir: 1 | -1) => void;
  setView: (v: View) => void;

  // ── Trigger parent actions ─────────────────────────────────────
  setAgentOpen: (v: boolean) => void;
  focusCursor: (idx: number) => void;
  selectAllOrClear: () => void;
  toggleSelectedFor: (taskId: string) => void;
  toggleDoneFor: (t: Task) => void;
  openDetailFor: (t: Task) => void;
  cyclePriorityFor: (t: Task) => void;
  openSnoozeForCursor: () => void;
  clearSelection: () => void;
};

/** Install the keydown listener. Call from onMount, return the
 *  result so the cleanup runs on unmount. */
export function installTasksKeyboard(refs: TasksKeyboardRefs): () => void {
  function onKey(e: KeyboardEvent) {
    if (isTypingTarget(e.target)) return;
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    const k = e.key;

    // Help overlay — works on every view (including kanban-family,
    // which otherwise short-circuits below).
    if (k === '?') {
      refs.setHelpOpen(!refs.isHelpOpen());
      e.preventDefault();
      return;
    }
    if (refs.isHelpOpen() && k === 'Escape') {
      refs.setHelpOpen(false);
      return;
    }

    // `/` opens the slide-out filter panel so the global page-search
    // handler in +layout.svelte finds the embedded search input
    // visible. We DON'T preventDefault — the global handler runs next
    // and focuses the input.
    if (k === '/' && !refs.isFilterPanelOpen()) {
      refs.setFilterPanelOpen(true);
    }
    // Esc closes the filter panel before falling through to the
    // selection-clear branch lower down.
    if (k === 'Escape' && refs.isFilterPanelOpen()) {
      refs.setFilterPanelOpen(false);
      e.preventDefault();
      return;
    }

    // View cycling + direct-jump work on EVERY view (so the user can
    // bounce out of kanban → list with `]`, then back with `[`).
    // Must run before the kanban / triage / eisenhower early-return.
    if (k === '[') {
      refs.cycleView(-1);
      e.preventDefault();
      return;
    }
    if (k === ']') {
      refs.cycleView(1);
      e.preventDefault();
      return;
    }
    if (k in VIEW_DIGIT_MAP) {
      refs.setView(VIEW_DIGIT_MAP[k]);
      e.preventDefault();
      return;
    }

    // Kanban / TriageBoard / EisenhowerView own their own listener.
    const v = refs.getView();
    if (v === 'kanban' || v === 'triage' || v === 'eisenhower') return;

    const cursorIdx = refs.getCursorIdx();

    if (k === 'j') {
      refs.focusCursor(cursorIdx < 0 ? 0 : cursorIdx + 1);
      e.preventDefault();
      return;
    }
    if (k === 'k') {
      refs.focusCursor(cursorIdx < 0 ? 0 : cursorIdx - 1);
      e.preventDefault();
      return;
    }

    // 'a' opens the Task Agent. Distinct from per-task shortcuts —
    // no cursor task required, the agent reads the filtered list (or
    // bulk-selection if one is active).
    if (k === 'a') {
      refs.setAgentOpen(true);
      e.preventDefault();
      return;
    }
    // Shift+A — bulk select-all / clear toggle. event.key on Shift+A
    // reports "A" uppercase; e.shiftKey disambiguates the unlikely
    // caps-lock case.
    if (k === 'A' && e.shiftKey) {
      refs.selectAllOrClear();
      e.preventDefault();
      return;
    }

    const filtered = refs.getFiltered();
    const t = cursorIdx >= 0 ? filtered[cursorIdx] : null;
    if (!t) return;

    if (k === 'x') {
      refs.toggleSelectedFor(t.id);
      e.preventDefault();
    } else if (k === 'd') {
      refs.toggleDoneFor(t);
      e.preventDefault();
    } else if (k === 'e') {
      refs.openDetailFor(t);
      e.preventDefault();
    } else if (k === 'p') {
      refs.cyclePriorityFor(t);
      e.preventDefault();
    } else if (k === 's') {
      // Open the snooze popover on the cursor task. The popover owns
      // the date picker; this triggers the in-card button so
      // positioning + outside-click dismiss behave like a mouse click.
      refs.openSnoozeForCursor();
      e.preventDefault();
    } else if (k === 'Escape') {
      if (refs.getSelectionSize() > 0) {
        refs.clearSelection();
        e.preventDefault();
      }
    }
  }
  window.addEventListener('keydown', onKey);
  return () => window.removeEventListener('keydown', onKey);
}

// Convenience wrapper: most callers already have a viewCtl / filterCtl
// / selCtl trio matching the established controller pattern, so the
// 20-line refs object collapses to one of these calls. The underlying
// refs shape stays as-is so callers with non-standard bindings can
// still use installTasksKeyboard directly.
import type { TasksFilterStateController } from './tasksFilterState.svelte';
import type { TasksViewStateController } from './tasksViewState.svelte';
import type { TasksSelectionController } from './tasksSelection.svelte';

export function installTasksKeyboardForCtls(args: {
  filterCtl: TasksFilterStateController;
  viewCtl: TasksViewStateController;
  selCtl: TasksSelectionController;
  setAgentOpen: (v: boolean) => void;
  toggleDoneFor: (t: Task) => void;
  openDetailFor: (t: Task) => void;
  cyclePriorityFor: (t: Task) => void;
}): () => void {
  const { filterCtl, viewCtl, selCtl } = args;
  return installTasksKeyboard({
    getView: () => viewCtl.view,
    getFiltered: () => filterCtl.filtered,
    getCursorIdx: () => selCtl.cursorIdx,
    getSelectionSize: () => selCtl.selectedIds.size,
    isHelpOpen: () => viewCtl.helpOpen,
    setHelpOpen: (v) => (viewCtl.helpOpen = v),
    isFilterPanelOpen: () => viewCtl.filterPanelOpen,
    setFilterPanelOpen: (v) => (viewCtl.filterPanelOpen = v),
    cycleView: (dir) => viewCtl.cycleView(dir),
    setView: (v) => (viewCtl.view = v),
    setAgentOpen: args.setAgentOpen,
    focusCursor: selCtl.focusCursor,
    selectAllOrClear: selCtl.selectAllOrClear,
    toggleSelectedFor: selCtl.toggleSelected,
    toggleDoneFor: args.toggleDoneFor,
    openDetailFor: args.openDetailFor,
    cyclePriorityFor: args.cyclePriorityFor,
    openSnoozeForCursor: selCtl.openSnoozePickerForCursor,
    clearSelection: selCtl.clear
  });
}
