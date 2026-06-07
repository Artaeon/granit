// Page-level URL intents the calendar honours on first mount.
//
// Four params, all consumed once:
//
//   ?plan=1            — Flip on plan mode + force day view. Used by
//                        the project detail's "schedule next action"
//                        button to hand off into the right state.
//
//   ?project=NAME      — Scope the calendar to a single project. Pairs
//                        with the per-event project picker; an event
//                        linked to project X only renders when this
//                        is empty or set to X.
//
//   ?agent=1           — Launch the Calendar Agent. Used by the chat
//                        sidebar. The param is stripped from the URL
//                        on mount so a refresh doesn't re-pop the
//                        agent dialog.
//
//   ?routineAi=1       — Launch the Daily Routine AI proposal drawer.
//                        Same strip-on-mount posture as ?agent.

import { goto } from '$app/navigation';
import type { CalendarViewStateController } from './calendarViewState.svelte';
import type { CalendarFilterStateController } from './calendarFilterState.svelte';

export interface CalendarUrlIntentsOptions {
  viewCtl: CalendarViewStateController;
  filterCtl: CalendarFilterStateController;
  /** Open the calendar agent (?agent=1). Same callback shape the
   *  keyboard module uses for Shift+A. */
  openAgent: () => void;
  /** Open the Daily Routine AI drawer (?routineAi=1). Optional so
   *  surfaces that don't wire the drawer can omit the handler. */
  openRoutineAI?: () => void;
}

export function applyCalendarUrlIntents(opts: CalendarUrlIntentsOptions): void {
  if (typeof window === 'undefined') return;
  const { viewCtl, filterCtl } = opts;
  const url = new URL(window.location.href);

  if (url.searchParams.get('plan') === '1' && !viewCtl.planMode) {
    viewCtl.planMode = true;
    viewCtl.view = 'day';
  }

  const proj = url.searchParams.get('project');
  if (proj) filterCtl.projectFilter = proj;

  // Track which one-shot params we consumed so we can strip them all
  // in a single goto() — chaining replaceStates would race the router.
  const consumed: string[] = [];

  if (url.searchParams.get('agent') === '1') {
    opts.openAgent();
    consumed.push('agent');
  }

  if (url.searchParams.get('routineAi') === '1' && opts.openRoutineAI) {
    opts.openRoutineAI();
    consumed.push('routineAi');
  }

  if (consumed.length > 0) {
    const params = new URLSearchParams(url.searchParams);
    for (const p of consumed) params.delete(p);
    void goto(`/calendar${params.toString() ? '?' + params : ''}`, {
      replaceState: true,
      keepFocus: true
    });
  }
}
