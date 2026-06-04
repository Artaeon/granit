// Pane registry — the catalog of pane types the workspace shell can
// drop into any slot. First Phase 2 building block of the granit
// vision (VSCode-for-life): a pane is any self-contained surface
// that owns its own state and template, instantiated like a Svelte
// component.
//
// The registry deliberately stays tiny — three entries, no metadata
// beyond what the slot UI needs (a stable id, a human label, a
// component reference). Adding a new pane type is one line.

import type { Component } from 'svelte';
import TasksPane from '$lib/tasks/TasksPane.svelte';
import CalendarPane from '$lib/calendar/CalendarPane.svelte';
import GoalsPane from '$lib/goals/GoalsPane.svelte';
import NotesListPane from '$lib/notes/NotesListPane.svelte';
import FinancePane from '$lib/finance/FinancePane.svelte';

export type PaneKind = 'tasks' | 'calendar' | 'goals' | 'notes' | 'finance';

export interface PaneEntry {
  /** Stable on-disk id. Persisted in workspace layout state. */
  id: PaneKind;
  /** Human-readable label shown in the slot picker. */
  label: string;
  /** Svelte component constructor — what the slot renders. */
  component: Component;
}

export const PANES: ReadonlyArray<PaneEntry> = [
  { id: 'tasks', label: 'Tasks', component: TasksPane },
  { id: 'calendar', label: 'Calendar', component: CalendarPane },
  { id: 'goals', label: 'Goals', component: GoalsPane },
  { id: 'notes', label: 'Notes', component: NotesListPane },
  { id: 'finance', label: 'Finance', component: FinancePane }
];

export function findPane(id: PaneKind): PaneEntry | undefined {
  return PANES.find((p) => p.id === id);
}
