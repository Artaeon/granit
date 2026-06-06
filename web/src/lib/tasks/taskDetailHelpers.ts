// Pure stateless helpers for the TaskDetail drawer. Pulled out of
// TaskDetail.svelte to keep the component's <script> body focused on
// what's actually reactive — these constants, formatters, and prompt
// builders don't read or write any $state, so they belong in a plain
// .ts module.

import type { Task } from '$lib/api';
import { cleanTaskText } from '$lib/util/taskParse';

/** Recurrence pill options shown in the drawer's "Recurrence" row.
 *  Empty value = "no recurrence" (clears the marker). */
export const recurrenceOptions: { value: string; label: string }[] = [
  { value: '', label: 'none' },
  { value: 'daily', label: 'daily' },
  { value: 'weekly', label: 'weekly' },
  { value: 'monthly', label: 'monthly' },
  { value: '3x-week', label: '3× / week' }
];

/** Triage states surfaced as pill buttons in the drawer. */
export const triageStates: NonNullable<Task['triage']>[] = [
  'inbox',
  'triaged',
  'scheduled',
  'done',
  'dropped',
  'snoozed'
];

/** Locale-aware datetime formatter for the read-only metadata rows.
 *  Returns an em-dash for missing values so the layout doesn't shift. */
export function fmtDate(s?: string): string {
  if (!s) return '—';
  const d = new Date(s);
  return d.toLocaleString();
}

/** Build the YYYY-MM-DDTHH:MM (local wall-clock) snooze target for
 *  `days` from now at `hour`. Local-time semantics on purpose — the
 *  backend stores snoozedUntil as wall-clock so "in 2 weeks at 9am"
 *  matches the user's intuition regardless of TZ shifts. */
export function snoozeOffset(days: number, hour = 9): string {
  const d = new Date();
  d.setDate(d.getDate() + days);
  d.setHours(hour, 0, 0, 0);
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  const hh = String(d.getHours()).padStart(2, '0');
  const mi = String(d.getMinutes()).padStart(2, '0');
  return `${y}-${m}-${dd}T${hh}:${mi}`;
}

/** Build the seed prompt the "Ask AI" header button feeds into the
 *  AIOverlay. Pre-fills the composer with this task's title +
 *  scheduling + notes as context so the model has enough to answer
 *  "help me break this down" / "draft a plan" without the user
 *  having to re-state the task. */
export function buildAskAIPrompt(task: Task): string {
  const lines = [`I'm working on this task:`, '', `- ${cleanTaskText(task.text)}`];
  if (task.dueDate) lines.push(`- due ${task.dueDate}`);
  if (task.priority) lines.push(`- priority P${task.priority}`);
  if (task.scheduledStart) lines.push(`- scheduled ${task.scheduledStart}`);
  if (task.estimatedMinutes) lines.push(`- estimate ${task.estimatedMinutes}m`);
  if (task.tags && task.tags.length > 0) lines.push(`- tags: ${task.tags.join(', ')}`);
  if (task.notes && task.notes.trim() !== '') {
    lines.push('', `My notes on it:`, task.notes.trim());
  }
  lines.push('', `What would help me move it forward?`);
  return lines.join('\n');
}
