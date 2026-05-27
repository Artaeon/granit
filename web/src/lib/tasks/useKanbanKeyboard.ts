// Shared keyboard-navigation helpers for the kanban-style views
// (Kanban, TriageBoard, EisenhowerView). The cursor is a flat index
// across a 2-D list-of-lists ("columns of task ids"); j/k step
// through the flattened sequence, h/l jump column boundaries.
//
// This is plain TS by design — `cursorIdx` is `$state` owned by the
// caller. Putting the state here would force a .svelte.ts file +
// transitive runes import; callers already have $state available
// in their <script>, so passing it in is the lighter touch.

import { isTypingTarget } from '$lib/util/isTypingTarget';
import type { Task } from '$lib/api';

/** One column in a kanban-style view. The `ids` array is the task
 *  order WITHIN this column — j/k walks it top-to-bottom. */
export type KanbanCol = {
  key: string;
  ids: string[];
};

/** Flatten columns → one ordered array of task ids. */
export function flattenIds(columns: KanbanCol[]): string[] {
  const out: string[] = [];
  for (const c of columns) for (const id of c.ids) out.push(id);
  return out;
}

/** Resolve the (column index, in-column index) for a given flat
 *  cursor index. Returns null if out of bounds. */
export function locate(columns: KanbanCol[], cursorIdx: number): { col: number; row: number } | null {
  if (cursorIdx < 0) return null;
  let n = cursorIdx;
  for (let c = 0; c < columns.length; c++) {
    if (n < columns[c].ids.length) return { col: c, row: n };
    n -= columns[c].ids.length;
  }
  return null;
}

/** Inverse of locate: flat cursor index for a (column, row). Clamps
 *  row to the column's last task. Returns -1 if the column is empty
 *  AND there's no neighbouring non-empty column. */
export function flatIndex(columns: KanbanCol[], col: number, row: number): number {
  if (columns.length === 0) return -1;
  const cc = Math.max(0, Math.min(columns.length - 1, col));
  if (columns[cc].ids.length === 0) {
    // Empty column — find nearest non-empty in either direction.
    for (let step = 1; step < columns.length; step++) {
      const left = cc - step;
      const right = cc + step;
      if (right < columns.length && columns[right].ids.length > 0) return flatIndex(columns, right, 0);
      if (left >= 0 && columns[left].ids.length > 0) return flatIndex(columns, left, columns[left].ids.length - 1);
    }
    return -1;
  }
  const rr = Math.max(0, Math.min(columns[cc].ids.length - 1, row));
  let base = 0;
  for (let i = 0; i < cc; i++) base += columns[i].ids.length;
  return base + rr;
}

/** Step cursor by `dir` (+1 / -1) through the flattened list. */
export function stepFlat(columns: KanbanCol[], cursorIdx: number, dir: 1 | -1): number {
  const flat = flattenIds(columns);
  if (flat.length === 0) return -1;
  if (cursorIdx < 0) return dir === 1 ? 0 : flat.length - 1;
  return Math.max(0, Math.min(flat.length - 1, cursorIdx + dir));
}

/** Step cursor across columns (h/l). Preserves row within the new
 *  column; clamps to that column's last task when shorter. */
export function stepColumn(columns: KanbanCol[], cursorIdx: number, dir: 1 | -1): number {
  const here = locate(columns, cursorIdx);
  if (!here) return flatIndex(columns, dir === 1 ? 0 : columns.length - 1, 0);
  const targetCol = Math.max(0, Math.min(columns.length - 1, here.col + dir));
  return flatIndex(columns, targetCol, here.row);
}

/** Scroll the DOM card for `taskId` into view. No-op when the
 *  element isn't mounted yet. 'instant' avoids the smooth-scroll
 *  jank when j-spamming through a long column. */
export function scrollCursorIntoView(taskId: string | null): void {
  if (!taskId || typeof document === 'undefined') return;
  queueMicrotask(() => {
    const el = document.querySelector(`[data-kanban-task-id="${taskId}"]`) as HTMLElement | null;
    if (el) el.scrollIntoView({ block: 'nearest', behavior: 'instant' as ScrollBehavior });
  });
}

/** Callbacks the view supplies — anything optional is silently
 *  skipped when missing. */
export type KanbanKeyHandlers = {
  /** Current task by id, or null. */
  taskById: (id: string) => Task | null | undefined;
  /** Read the current cursor index. */
  getCursorIdx: () => number;
  /** Write the cursor index. */
  setCursorIdx: (n: number) => void;
  /** Current column list (regenerated per call so derived state
   *  stays fresh). */
  getColumns: () => KanbanCol[];
  /** Selection set, when the view exposes one. */
  selectedIds?: () => Set<string>;
  setSelectedIds?: (next: Set<string>) => void;
  /** Detail-drawer opener. */
  onOpenDetail?: (t: Task) => void;
  /** Toggle done on a task. */
  onToggleDone?: (t: Task) => void;
  /** Cycle priority on a task. */
  onCyclePriority?: (t: Task) => void;
};

/** Build a window-level keydown handler for a kanban-style view.
 *  Honors the canonical "skip when typing" guard so j/k don't eat
 *  letters in inputs / contenteditable. */
export function makeKanbanKeyHandler(h: KanbanKeyHandlers): (e: KeyboardEvent) => void {
  return (e: KeyboardEvent) => {
    if (isTypingTarget(e.target)) return;
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    const k = e.key;
    const cols = h.getColumns();
    const flat = flattenIds(cols);

    // j/k — step through flattened list
    if (k === 'j' || k === 'k') {
      const next = stepFlat(cols, h.getCursorIdx(), k === 'j' ? 1 : -1);
      h.setCursorIdx(next);
      scrollCursorIntoView(flat[next] ?? null);
      e.preventDefault();
      return;
    }
    // h/l — step across columns
    if (k === 'h' || k === 'l') {
      const next = stepColumn(cols, h.getCursorIdx(), k === 'l' ? 1 : -1);
      h.setCursorIdx(next);
      scrollCursorIntoView(flattenIds(cols)[next] ?? null);
      e.preventDefault();
      return;
    }
    if (k === 'Escape') {
      h.setCursorIdx(-1);
      if (h.setSelectedIds && h.selectedIds && h.selectedIds().size > 0) {
        h.setSelectedIds(new Set());
      }
      e.preventDefault();
      return;
    }
    // Remaining bindings need a task under the cursor.
    const idx = h.getCursorIdx();
    if (idx < 0 || idx >= flat.length) return;
    const t = h.taskById(flat[idx]);
    if (!t) return;
    if (k === 'x') {
      if (h.selectedIds && h.setSelectedIds) {
        const cur = h.selectedIds();
        const next = new Set(cur);
        if (next.has(t.id)) next.delete(t.id);
        else next.add(t.id);
        h.setSelectedIds(next);
      }
      e.preventDefault();
    } else if (k === 'd') {
      h.onToggleDone?.(t);
      e.preventDefault();
    } else if (k === 'e') {
      h.onOpenDetail?.(t);
      e.preventDefault();
    } else if (k === 'p') {
      h.onCyclePriority?.(t);
      e.preventDefault();
    }
  };
}
