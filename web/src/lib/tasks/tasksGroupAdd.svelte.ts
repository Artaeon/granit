// In-group quick-add controller.
//
// Each section in SectionList exposes a small "+ add" affordance.
// Clicking it opens an inline input scoped to that group. Submitting
// creates a task whose defaults (dueDate / priority / project / tag /
// goal / deadline / notePath) match the group key — a task added to
// the "this_week" bucket lands due in 3 days, one added to the
// project group lands tagged with that project, and so on. No
// scattering: the new task shows up where the user added it.
//
// This file owns the open key + draft text + busy flag, plus the
// pure groupAddDefaults mapping. submitGroupAdd hits the API; the
// parent passes a reload callback that fires on success.

import { api, todayISO, fmtDateISO, type Project } from '$lib/api';
import { toast } from '$lib/components/toast';
import type { Group } from './tasksHelpers';

export type GroupAddDefaults = {
  dueDate?: string;
  priority?: number;
  projectId?: string;
  tags?: string[];
  goalId?: string;
  deadlineId?: string;
  notePathHint?: string;
};

/** Map (group-by dimension, group key) → createTask defaults so a
 *  task added to a bucket lands in that bucket. Pure; testable in
 *  isolation. */
export function computeGroupAddDefaults(
  groupBy: Group,
  group: string,
  projects: Project[]
): GroupAddDefaults {
  const today = todayISO();
  if (groupBy === 'due') {
    switch (group) {
      case 'overdue':
      case 'today':
        return { dueDate: today };
      case 'tomorrow': {
        const d = new Date(today + 'T00:00:00');
        d.setDate(d.getDate() + 1);
        return { dueDate: fmtDateISO(d) };
      }
      case 'this_week': {
        const d = new Date(today + 'T00:00:00');
        d.setDate(d.getDate() + 3);
        return { dueDate: fmtDateISO(d) };
      }
      case 'later': {
        const d = new Date(today + 'T00:00:00');
        d.setDate(d.getDate() + 14);
        return { dueDate: fmtDateISO(d) };
      }
      case 'no_date':
      default:
        return {};
    }
  }
  if (groupBy === 'priority') {
    const p = Number(group);
    return p >= 1 && p <= 3 ? { priority: p } : {};
  }
  if (groupBy === 'tag') {
    return group === '(untagged)' ? {} : { tags: [group] };
  }
  if (groupBy === 'project') {
    const proj = projects.find((p) => p.name === group);
    if (!proj) return {};
    return { projectId: proj.name };
  }
  if (groupBy === 'goal') return group === '(no goal)' ? {} : { goalId: group };
  if (groupBy === 'deadline') return group === '(no deadline)' ? {} : { deadlineId: group };
  if (groupBy === 'note') return { notePathHint: group };
  return {};
}

export interface TasksGroupAddController {
  /** Which group's add row is open. null = nothing open. */
  key: string | null;
  /** Draft text in the open row. */
  text: string;
  /** Submit in flight — keeps the row busy + disables double-submit. */
  readonly busy: boolean;

  /** Open the row for `group` (set key + clear text). */
  open(group: string): void;
  /** Close the row without submitting. */
  cancel(): void;
  /** Create the task for `group` using current text + computed
   *  defaults. On success, clear text and call onAdded() to reload;
   *  leave the row open so the user can keep capturing. */
  submit(group: string): Promise<void>;
}

export type TasksGroupAddDeps = {
  getGroupBy: () => Group;
  getProjects: () => Project[];
  onAdded: () => void | Promise<void>;
};

export function createTasksGroupAdd(deps: TasksGroupAddDeps): TasksGroupAddController {
  let key = $state<string | null>(null);
  let text = $state('');
  let busy = $state(false);

  function open(group: string) {
    key = group;
    text = '';
  }

  function cancel() {
    key = null;
    text = '';
  }

  async function submit(group: string) {
    const t = text.trim();
    if (!t || busy) return;
    busy = true;
    try {
      const defaults = computeGroupAddDefaults(deps.getGroupBy(), group, deps.getProjects());
      // notePath fallback chain:
      //   1. note-grouped key IS the notePath
      //   2. otherwise today's daily — the safe capture target
      let notePath = defaults.notePathHint ?? '';
      if (!notePath) {
        try {
          const daily = await api.daily('today');
          notePath = daily.path;
        } catch {
          notePath = `${todayISO()}.md`;
        }
      }
      const body: Parameters<typeof api.createTask>[0] = { notePath, text: t };
      if (defaults.dueDate) body.dueDate = defaults.dueDate;
      if (defaults.priority !== undefined) body.priority = defaults.priority;
      if (defaults.tags && defaults.tags.length > 0) body.tags = defaults.tags;
      if (defaults.projectId) body.projectId = defaults.projectId;
      if (defaults.goalId) body.goalId = defaults.goalId;
      if (defaults.deadlineId) body.deadlineId = defaults.deadlineId;
      await api.createTask(body);
      text = '';
      await deps.onAdded();
      toast.success('task added');
      // Leave the row open so the user can keep capturing without
      // re-opening it. Esc / blur dismisses.
    } catch (e) {
      toast.error('add failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      busy = false;
    }
  }

  return {
    get key() { return key; },
    set key(v) { key = v; },
    get text() { return text; },
    set text(v) { text = v; },
    get busy() { return busy; },
    open,
    cancel,
    submit
  };
}
