// Shared formatters for a task's due-date field. Three near-identical
// copies of these helpers lived inline in TaskCard.svelte plus
// ad-hoc inline derivations elsewhere — consolidating here keeps the
// "today / tomorrow / in 3d / 2w ago" semantics consistent across every
// surface that renders a due date.
//
// All three operate on the YYYY-MM-DD shape the backend stores for
// dueDate. Local-time semantics intentional: every caller is rendering
// for the user's wall clock, not UTC.

import { todayISO } from './date';

// Returns a CSS color-tone class for a due date string. 'text-dim' when
// no date is set, 'text-error' if overdue, 'text-warning' if due today,
// 'text-dim' otherwise (future).
export function dueClass(due?: string): string {
  if (!due) return 'text-dim';
  const today = todayISO();
  if (due < today) return 'text-error';
  if (due === today) return 'text-warning';
  return 'text-dim';
}

// Relative-aware label. Reads naturally — "today", "tomorrow",
// "yesterday", "in 3d", "+2w", or a localised date for far-out dates.
// Matches what users expect from Things / Reminders / Todoist.
export function dueLabel(due: string): string {
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const [y, m, d] = due.split('-').map(Number);
  const date = new Date(y, m - 1, d);
  const days = Math.round((date.getTime() - today.getTime()) / 86_400_000);
  if (days === 0) return 'today';
  if (days === 1) return 'tomorrow';
  if (days === -1) return 'yesterday';
  if (days < 0 && days >= -6) return `${-days}d ago`;
  if (days > 0 && days <= 6) return `in ${days}d`;
  if (days < 0 && days >= -28) return `${Math.round(-days / 7)}w ago`;
  if (days > 0 && days <= 28) return `in ${Math.round(days / 7)}w`;
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

// Icon char per due-state — nothing fancy, just enough to let the eye
// distinguish "overdue" (warn triangle) from "today" (clock) from
// "future" (calendar) at a glance.
export function dueIcon(due: string): string {
  const today = todayISO();
  if (due < today) return '⚠';
  if (due === today) return '⏰';
  return '📅';
}
