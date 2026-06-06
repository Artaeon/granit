// Linked-tasks loader + 8-week burnup for GoalDetail.
//
// Tasks carry a free goalId reference; we fetch all and filter
// client-side. Same pattern ProjectDetail uses for project tasks.
// Burnup is bucketed by ISO week so a "W19" tally on the goal lines
// up with the dashboard TaskVelocityWidget and the project pages.

import { api, type Goal, type Task } from '$lib/api';
import { isoWeekString, startOfIsoWeek } from '$lib/util/isoWeek';

export type BurnupWeek = {
  label: string;
  count: number;
  isThisWeek: boolean;
};

export interface GoalDetailTasksBurnupController {
  readonly goalTasks: Task[];
  readonly burnup: BurnupWeek[];
  readonly burnupMax: number;
  readonly burnupTotal: number;
  readonly openTaskCount: number;
  readonly doneTaskCount: number;
  /** Refetch from /api/v1/tasks and filter against goal.id. */
  loadGoalTasks(): Promise<void>;
}

export interface GoalDetailTasksBurnupDeps {
  getGoal: () => Goal | null;
}

const BURNUP_WEEKS = 8;

export function createGoalDetailTasksBurnup(
  deps: GoalDetailTasksBurnupDeps
): GoalDetailTasksBurnupController {
  let goalTasks = $state<Task[]>([]);

  // Gen counter — when the parent swaps goal mid-fetch, the OLD
  // goal's task list must NOT overwrite the NEW goal's slice.
  let fetchGen = 0;

  async function loadGoalTasks() {
    const goal = deps.getGoal();
    if (!goal) return;
    const my = ++fetchGen;
    try {
      const r = await api.listTasks({});
      if (my !== fetchGen) return;
      goalTasks = r.tasks.filter((t) => t.goalId === goal.id);
    } catch {
      if (my !== fetchGen) return;
      goalTasks = [];
    }
  }

  const burnup = $derived.by<BurnupWeek[]>(() => {
    const now = new Date();
    const weekStart = startOfIsoWeek(now);
    const thisKey = isoWeekString(now);
    const order: string[] = [];
    const labels = new Map<string, string>();
    for (let i = BURNUP_WEEKS - 1; i >= 0; i--) {
      const d = new Date(weekStart);
      d.setDate(d.getDate() - i * 7);
      const k = isoWeekString(d);
      order.push(k);
      labels.set(k, k === thisKey ? 'Now' : k.split('W')[1]);
    }
    const counts = new Map<string, number>();
    for (const t of goalTasks) {
      if (!t.done || !t.completedAt) continue;
      const d = new Date(t.completedAt);
      if (Number.isNaN(d.getTime())) continue;
      const k = isoWeekString(d);
      if (!order.includes(k)) continue;
      counts.set(k, (counts.get(k) ?? 0) + 1);
    }
    return order.map((k) => ({
      label: labels.get(k) ?? k,
      count: counts.get(k) ?? 0,
      isThisWeek: k === thisKey
    }));
  });

  const burnupMax = $derived(burnup.reduce((m, b) => Math.max(m, b.count), 0));
  const burnupTotal = $derived(burnup.reduce((s, b) => s + b.count, 0));
  const openTaskCount = $derived(goalTasks.filter((t) => !t.done).length);
  const doneTaskCount = $derived(goalTasks.filter((t) => t.done).length);

  return {
    get goalTasks() { return goalTasks; },
    get burnup() { return burnup; },
    get burnupMax() { return burnupMax; },
    get burnupTotal() { return burnupTotal; },
    get openTaskCount() { return openTaskCount; },
    get doneTaskCount() { return doneTaskCount; },
    loadGoalTasks
  };
}
