// Subtask list + manual add controller for the TaskDetail drawer.
//
// Two surfaces share this controller:
//
//   1. The "Subtasks" section, which lists every direct child of the
//      current task — fetched by pulling every task in the same
//      notePath and filtering to those whose parentLine matches OUR
//      lineNum, then exposed as a flat sorted array. The list is
//      refreshed after any add / toggle / delete so the user sees
//      the result immediately.
//
//   2. The one-line manual add input, which writes through createTask
//      with parentLine set so the new line ends up INDENTED in the
//      markdown — a real subtask, not a flat sibling. Mirrors the
//      AI-Decompose flow's parentLine wiring on purpose; both should
//      produce the same shape of child line.
//
// subtasksGen is a generation counter — every loadSubtasks call bumps
// it, and the in-flight response only commits if its captured
// generation still matches. Without this, closing or re-targeting
// the drawer mid-fetch lets a stale response land and overwrite the
// new task's subtask list (or call onChanged on an unmounted drawer).
//
// reset() is called by the parent's per-task $effect when the drawer
// re-targets — wipes the visible list + bumps the generation counter
// so a still-pending fetch can't paint stale child rows.

import { api, type Task } from '$lib/api';
import { toast } from '$lib/components/toast';
import { cleanTaskText } from '$lib/util/taskParse';

export interface TaskDetailSubtasksController {
  readonly subtasks: Task[];
  readonly loaded: boolean;
  /** Manual add input buffer — bound directly from the template. */
  manualBuf: string;
  readonly manualBusy: boolean;

  /** Reload children from the API. Generation-guarded — late
   *  responses are dropped if a newer call has started. */
  load(): Promise<void>;
  /** Reset visible state for a fresh task target. Use from the
   *  parent's per-task init effect. */
  reset(): void;

  /** Add the manual-buf text as a new subtask. */
  addManual(): Promise<void>;
  toggleDone(s: Task): Promise<void>;
  remove(s: Task): Promise<void>;
}

export type TaskDetailSubtasksDeps = {
  getTask: () => Task | null;
  onChanged: () => void | Promise<void>;
};

export function createTaskDetailSubtasks(deps: TaskDetailSubtasksDeps): TaskDetailSubtasksController {
  let subtasks = $state<Task[]>([]);
  let loaded = $state(false);
  let manualBuf = $state('');
  let manualBusy = $state(false);
  let gen = 0;

  async function load() {
    const myGen = ++gen;
    const task = deps.getTask();
    if (!task) {
      subtasks = [];
      loaded = true;
      return;
    }
    try {
      const r = await api.listTasks({ note: task.notePath });
      if (myGen !== gen) return; // superseded — drop the response
      // Match by parentLine — that's what the parser computes for
      // every indented task it parses. Also exclude the parent itself.
      subtasks = r.tasks
        .filter((t) => t.id !== task.id && t.parentLine === task.lineNum)
        .sort((a, b) => a.lineNum - b.lineNum);
      loaded = true;
    } catch {
      if (myGen !== gen) return;
      subtasks = [];
      loaded = true;
    }
  }

  function reset() {
    gen++;
    subtasks = [];
    loaded = false;
    manualBuf = '';
  }

  async function addManual() {
    const task = deps.getTask();
    if (!task) return;
    const text = manualBuf.trim();
    if (!text || manualBusy) return;
    manualBusy = true;
    try {
      const created = await api.createTask({
        notePath: task.notePath,
        text,
        goalId: task.goalId,
        deadlineId: task.deadlineId,
        parentLine: task.lineNum
      });
      // projectId isn't in CreateOpts — patch it on after creation so
      // the new line inherits the parent's project sidecar metadata.
      if (task.projectId && created?.id) {
        try {
          await api.patchTask(created.id, { projectId: task.projectId });
        } catch {}
      }
      manualBuf = '';
      await deps.onChanged();
      await load();
      toast.success('Subtask added');
    } catch (e) {
      toast.error('Add failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      manualBusy = false;
    }
  }

  async function toggleDone(s: Task) {
    try {
      await api.patchTask(s.id, { done: !s.done });
      await deps.onChanged();
      await load();
    } catch (e) {
      toast.error('Save failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function remove(s: Task) {
    if (!confirm(`Delete subtask "${cleanTaskText(s.text)}"?`)) return;
    try {
      await api.deleteTask(s.id);
      await deps.onChanged();
      await load();
    } catch (e) {
      toast.error('Delete failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  return {
    get subtasks() { return subtasks; },
    get loaded() { return loaded; },
    get manualBuf() { return manualBuf; },
    set manualBuf(v) { manualBuf = v; },
    get manualBusy() { return manualBusy; },
    load,
    reset,
    addManual,
    toggleDone,
    remove
  };
}
