// Detail-drawer + context-menu state for the tasks surface.
//
// Two cousin concerns bundled together:
//
//   • Detail drawer — TaskDetail.svelte mounts when the user clicks a
//     card. Opening publishes to the workspace context bus so an AI
//     pane in the adjacent workspace slot can surface this task as
//     context (best-effort; no-op outside the workspace shell).
//
//   • Context menu — TaskContextMenu.svelte mounts at a click
//     position, anchored by (x, y). Closes by setting ctxTask = null
//     via the close handler.
//
// Lives as a .svelte.ts factory because the state uses runes ($state).
// All consumers stay outside this file — the parent just reads
// detCtl.X and calls detCtl.openDetail / openContext / closeContext.

import { workspaceContext } from '$lib/workspace/workspaceContext.svelte';
import type { Task } from '$lib/api';

export interface TasksDetailController {
  /** Currently-open task in the detail drawer (null when closed). */
  detailTask: Task | null;
  detailOpen: boolean;
  /** Open the detail drawer for a task. Also publishes the task to
   *  the workspace context bus so an adjacent AI pane can surface it
   *  as context. */
  openDetail(t: Task): void;

  /** Currently-open task in the context menu (null when closed). */
  readonly ctxTask: Task | null;
  readonly ctxX: number;
  readonly ctxY: number;
  /** Open the context menu at (x, y) anchored to a task. */
  openContext(t: Task, x: number, y: number): void;
  /** Close the context menu. */
  closeContext(): void;
}

export function createTasksDetail(): TasksDetailController {
  let detailTask = $state<Task | null>(null);
  let detailOpen = $state(false);
  let ctxTask = $state<Task | null>(null);
  let ctxX = $state(0);
  let ctxY = $state(0);

  function openDetail(t: Task) {
    detailTask = t;
    detailOpen = true;
    // Publish to the workspace context bus so an AI pane in the
    // adjacent slot can surface this task as context. Best-effort —
    // if the user isn't running TasksPane inside the workspace shell,
    // nothing reads the bus and the publish is a no-op.
    workspaceContext.publish({
      paneKind: 'tasks',
      itemId: t.id,
      label: t.text,
      excerpt: t.notePath
    });
  }

  function openContext(t: Task, x: number, y: number) {
    ctxTask = t;
    ctxX = x;
    ctxY = y;
  }

  function closeContext() {
    ctxTask = null;
  }

  return {
    get detailTask() {
      return detailTask;
    },
    set detailTask(v) {
      detailTask = v;
    },
    get detailOpen() {
      return detailOpen;
    },
    set detailOpen(v) {
      detailOpen = v;
    },
    openDetail,
    get ctxTask() {
      return ctxTask;
    },
    get ctxX() {
      return ctxX;
    },
    get ctxY() {
      return ctxY;
    },
    openContext,
    closeContext
  };
}
