// View-time derivations for ProjectDetail.
//
// Pure computations off the loaded task list:
//
//   • openTasks / doneTasks  — split for everything downstream.
//   • progressPct            — milestone-style progress already on the
//                               project record, just rounded to int.
//   • weekSchedule           — 7-cell Mon-Sun strip of how many of
//                               this project's tasks are scheduled
//                               each day of the current week. Each
//                               cell carries the date, label, count,
//                               and isToday flag for the template.
//   • weekScheduleMax /
//     weekScheduleTotal      — for the density bar height + chip count
//                               above the strip.
//   • tasksByGoal            — Map<goalId, {open, done}>; surfaces task
//                               velocity per linked goal in the goals
//                               section.
//   • burnup                 — 8-week ISO-week completion bucket; same
//                               weekKey scheme as TaskVelocityWidget so
//                               "W19" matches what the dashboard shows.
//   • burnupMax /
//     burnupTotal            — for the bar height + chip count above
//                               the chart.

import type { Project, Task } from '$lib/api';
import { isoWeekString, startOfIsoWeek } from '$lib/util/isoWeek';
import { fmtDateISO as ymd } from '$lib/util/date';

export type WeekScheduleDay = {
  date: string;
  label: string;
  count: number;
  isToday: boolean;
};

export type BurnupWeek = {
  label: string;
  count: number;
  isThisWeek: boolean;
};

export interface ProjectStatsController {
  readonly openTasks: Task[];
  readonly doneTasks: Task[];
  readonly progressPct: number;
  readonly weekSchedule: WeekScheduleDay[];
  readonly weekScheduleMax: number;
  readonly weekScheduleTotal: number;
  readonly tasksByGoal: Map<string, { open: number; done: number }>;
  readonly burnup: BurnupWeek[];
  readonly burnupMax: number;
  readonly burnupTotal: number;
}

export interface ProjectStatsDeps {
  getProject: () => Project;
  getProjectTasks: () => Task[];
}

const BURNUP_WEEKS = 8;

export function createProjectStats(deps: ProjectStatsDeps): ProjectStatsController {
  const openTasks = $derived(deps.getProjectTasks().filter((t) => !t.done));
  const doneTasks = $derived(deps.getProjectTasks().filter((t) => t.done));
  const progressPct = $derived(Math.round((deps.getProject().progress ?? 0) * 100));

  const weekSchedule = $derived.by<WeekScheduleDay[]>(() => {
    const start = startOfIsoWeek(new Date());
    const today = ymd(new Date());
    const days: WeekScheduleDay[] = [];
    const labels = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
    for (let i = 0; i < 7; i++) {
      const d = new Date(start);
      d.setDate(d.getDate() + i);
      days.push({
        date: ymd(d),
        label: labels[i],
        count: 0,
        isToday: ymd(d) === today
      });
    }
    for (const t of deps.getProjectTasks()) {
      if (t.done || !t.scheduledStart) continue;
      const day = t.scheduledStart.slice(0, 10);
      const cell = days.find((x) => x.date === day);
      if (cell) cell.count++;
    }
    return days;
  });

  const weekScheduleMax = $derived(weekSchedule.reduce((m, d) => Math.max(m, d.count), 0));
  const weekScheduleTotal = $derived(weekSchedule.reduce((s, d) => s + d.count, 0));

  const tasksByGoal = $derived.by<Map<string, { open: number; done: number }>>(() => {
    const m = new Map<string, { open: number; done: number }>();
    for (const t of deps.getProjectTasks()) {
      if (!t.goalId) continue;
      const b = m.get(t.goalId) ?? { open: 0, done: 0 };
      if (t.done) b.done++;
      else b.open++;
      m.set(t.goalId, b);
    }
    return m;
  });

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
    for (const t of doneTasks) {
      if (!t.completedAt) continue;
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

  return {
    get openTasks() { return openTasks; },
    get doneTasks() { return doneTasks; },
    get progressPct() { return progressPct; },
    get weekSchedule() { return weekSchedule; },
    get weekScheduleMax() { return weekScheduleMax; },
    get weekScheduleTotal() { return weekScheduleTotal; },
    get tasksByGoal() { return tasksByGoal; },
    get burnup() { return burnup; },
    get burnupMax() { return burnupMax; },
    get burnupTotal() { return burnupTotal; }
  };
}
