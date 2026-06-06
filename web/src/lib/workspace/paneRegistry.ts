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
import ChatPane from '$lib/chat/ChatPane.svelte';
import TodayPane from '$lib/today/TodayPane.svelte';
import HabitsPane from '$lib/habits/HabitsPane.svelte';
import DeadlinesPane from '$lib/deadlines/DeadlinesPane.svelte';
import ProjectsPane from '$lib/projects/ProjectsPane.svelte';

export type PaneKind =
  | 'today'
  | 'tasks'
  | 'calendar'
  | 'goals'
  | 'notes'
  | 'habits'
  | 'deadlines'
  | 'projects'
  | 'finance'
  | 'chat';

export interface PaneEntry {
  /** Stable on-disk id. Persisted in workspace layout state. */
  id: PaneKind;
  /** Human-readable label shown in the slot picker. */
  label: string;
  /** NavIcon name — same identity vocabulary as the BottomNav + sidebar
   *  so a Tasks pane reads the same icon everywhere it appears. */
  icon: string;
  /** Svelte component constructor — what the slot renders. */
  component: Component;
}

export const PANES: ReadonlyArray<PaneEntry> = [
  // Daily-glance pane first — most workspaces will want it anchored
  // somewhere visible. Built on the same data sources as the home
  // route + right-pane Today widget, just rendered in pane chrome.
  { id: 'today', label: 'Today', icon: 'today', component: TodayPane },
  { id: 'tasks', label: 'Tasks', icon: 'tasks', component: TasksPane },
  { id: 'calendar', label: 'Calendar', icon: 'calendar', component: CalendarPane },
  { id: 'goals', label: 'Goals', icon: 'goals', component: GoalsPane },
  { id: 'notes', label: 'Notes', icon: 'notes', component: NotesListPane },
  { id: 'habits', label: 'Habits', icon: 'habits', component: HabitsPane },
  { id: 'deadlines', label: 'Deadlines', icon: 'deadline', component: DeadlinesPane },
  { id: 'projects', label: 'Projects', icon: 'projects', component: ProjectsPane },
  { id: 'finance', label: 'Finance', icon: 'finance', component: FinancePane },
  // "AI as a pane type" — the innovative bit of the granit vision.
  // Park the chat next to any working surface (notes / tasks / etc.)
  // to use AI as a contextual companion.
  { id: 'chat', label: 'AI', icon: 'chat', component: ChatPane }
];

export function findPane(id: PaneKind): PaneEntry | undefined {
  return PANES.find((p) => p.id === id);
}

// Map an in-app route to the pane kind that owns the same surface.
// Used by the ⌥W "open current route in workspace" shortcut so the
// user can promote any page they're on into a workspace pane in one
// keystroke. Routes without a pane counterpart (settings, auth, etc.)
// return null and the shortcut becomes a no-op.
export function routeToPaneKind(pathname: string): PaneKind | null {
  if (pathname === '/') return 'today';
  if (pathname === '/tasks' || pathname.startsWith('/tasks/')) return 'tasks';
  if (pathname === '/calendar' || pathname.startsWith('/calendar/')) return 'calendar';
  if (pathname === '/goals' || pathname.startsWith('/goals/')) return 'goals';
  if (pathname === '/notes' || pathname.startsWith('/notes/')) return 'notes';
  if (pathname === '/habits' || pathname.startsWith('/habits/')) return 'habits';
  if (pathname === '/deadlines' || pathname.startsWith('/deadlines/')) return 'deadlines';
  if (pathname === '/projects' || pathname.startsWith('/projects/')) return 'projects';
  if (pathname === '/finance' || pathname.startsWith('/finance/')) return 'finance';
  if (pathname === '/chat' || pathname.startsWith('/chat/')) return 'chat';
  return null;
}
