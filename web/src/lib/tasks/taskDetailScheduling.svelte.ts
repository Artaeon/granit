// Scheduling controller for the TaskDetail drawer.
//
// Owns three short controls that all commit-on-change:
//
//   1. Due date — YYYY-MM-DD or '' to clear. Backend accepts either.
//   2. Scheduled start — YYYY-MM-DDTHH:MM, local wall-clock (no zone).
//      The backend stores wall-clock so a 9 AM block stays 9 AM even
//      if the user crosses a TZ boundary mid-week (see commit 05183fc).
//   3. Snooze — quick-action chips (tomorrow / 2d / next week / etc.)
//      that flip snoozedUntil + triage='snoozed' together so the task
//      hides from default views until the timestamp passes.
//
// These commit on every change (no draft autosave) because the input
// surfaces are short controls — the loss-on-reload exposure is
// bounded to the keystroke just typed, not paragraphs of work.
//
// snoozeActive is a $derived view used by the header chip + the
// "until X" inline display.

import type { Task } from '$lib/api';
import { snoozeOffset } from './taskDetailHelpers';

type SchedulingPatch = {
  dueDate?: string;
  scheduledStart?: string;
  snoozedUntil?: string;
  triage?: NonNullable<Task['triage']>;
};

export interface TaskDetailSchedulingController {
  /** Due-date buffer — bound to the date input. */
  dueBuf: string;
  /** Scheduled-start date buffer (YYYY-MM-DD). */
  schedDateBuf: string;
  /** Scheduled-start time buffer (HH:MM). */
  schedTimeBuf: string;
  /** True while the task's snoozedUntil is in the future. */
  readonly snoozeActive: boolean;

  /** Reseed the three buffers from a fresh task target. */
  initFor(task: Task): void;

  /** Commit dueBuf if it differs from the server. */
  commitDue(): Promise<void>;
  /** Compose schedDateBuf + schedTimeBuf into the wall-clock string
   *  and commit if it differs from the server. */
  commitScheduled(): Promise<void>;
  /** Clear buffers + commit empty scheduledStart. */
  clearScheduled(): Promise<void>;
  /** Snooze for `days` days at the default 9 AM local. */
  snoozeUntil(days: number): Promise<void>;
  /** Drop the snooze and put the task back in triage='triaged'. */
  unsnooze(): Promise<void>;
}

export type TaskDetailSchedulingDeps = {
  getTask: () => Task | null;
  patch: (p: SchedulingPatch) => Promise<void>;
};

export function createTaskDetailScheduling(deps: TaskDetailSchedulingDeps): TaskDetailSchedulingController {
  let dueBuf = $state('');
  let schedDateBuf = $state('');
  let schedTimeBuf = $state('');

  const snoozeActive = $derived.by(() => {
    const task = deps.getTask();
    if (!task?.snoozedUntil) return false;
    const sn = new Date(task.snoozedUntil);
    return Number.isFinite(sn.getTime()) && sn.getTime() > Date.now();
  });

  function initFor(task: Task) {
    dueBuf = task.dueDate ?? '';
    schedDateBuf = task.scheduledStart ? task.scheduledStart.slice(0, 10) : '';
    schedTimeBuf = task.scheduledStart ? task.scheduledStart.slice(11, 16) : '';
  }

  async function commitDue() {
    const task = deps.getTask();
    if (!task) return;
    const next = dueBuf.trim();
    if (next === (task.dueDate ?? '')) return;
    await deps.patch({ dueDate: next });
  }

  async function commitScheduled() {
    const task = deps.getTask();
    if (!task) return;
    const ds = schedDateBuf.trim();
    const ts = schedTimeBuf.trim();
    let next = '';
    if (ds && ts) next = `${ds}T${ts}`;
    // Sensible default if the user picked a date but no time — most
    // people block scheduled work at 9 AM.
    else if (ds) next = `${ds}T09:00`;
    if (next === (task.scheduledStart ?? '')) return;
    await deps.patch({ scheduledStart: next });
  }

  async function clearScheduled() {
    if (!deps.getTask()) return;
    schedDateBuf = '';
    schedTimeBuf = '';
    await deps.patch({ scheduledStart: '' });
  }

  async function snoozeUntil(days: number) {
    await deps.patch({ snoozedUntil: snoozeOffset(days), triage: 'snoozed' });
  }

  async function unsnooze() {
    await deps.patch({ snoozedUntil: '', triage: 'triaged' });
  }

  return {
    get dueBuf() { return dueBuf; },
    set dueBuf(v) { dueBuf = v; },
    get schedDateBuf() { return schedDateBuf; },
    set schedDateBuf(v) { schedDateBuf = v; },
    get schedTimeBuf() { return schedTimeBuf; },
    set schedTimeBuf(v) { schedTimeBuf = v; },
    get snoozeActive() { return snoozeActive; },
    initFor,
    commitDue,
    commitScheduled,
    clearScheduled,
    snoozeUntil,
    unsnooze
  };
}
