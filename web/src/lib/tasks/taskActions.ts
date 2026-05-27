// Shared pure helpers + API wrappers for the two most common in-place
// task edits — cycle priority and toggle done. Five separate components
// (TaskCard, +page.svelte's keyboard handler, Kanban, TriageBoard,
// EisenhowerView) each had their own copy of `((p || 0) + 1) % 4` and
// `api.patchTask(id, { done: !t.done })`. Centralising the call
// signature keeps the contract authoritative without trying to also
// unify the per-caller optimistic-update + toast pattern (that was
// deliberately kept per-component because each caller's error UX
// differs slightly).

import { api, type Task } from '$lib/api';

// Pure: returns the next priority value in the 0 → 1 → 2 → 3 → 0
// cycle. Tolerates undefined / null / NaN by treating them as 0.
export function nextPriority(p: number): number {
  return ((p || 0) + 1) % 4;
}

// Calls api.patchTask with the next priority. Returns the updated task.
// Caller is responsible for optimistic update + error handling — this
// is just the patch-call shape.
export async function applyNextPriority(task: Task): Promise<Task> {
  return api.patchTask(task.id, { priority: nextPriority(task.priority) });
}

// Flip done state. Returns the updated task.
export async function toggleDoneOf(task: Task): Promise<Task> {
  return api.patchTask(task.id, { done: !task.done });
}
