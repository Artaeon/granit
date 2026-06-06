// Small view-time derivations + a tiny mobile-forcing effect bundled
// together because none is big enough to warrant its own module:
//
//   • headline                  — the "Tue, Jan 5" / "Jan 5-11" /
//                                  "January 2026" string that sits
//                                  above the grid for every view.
//   • pipelineButtonAvailable   — true when the active view + active
//                                  filters expose a content pipeline
//                                  shape (kanban on month, swim lanes
//                                  on week/workweek) AND at least one
//                                  content event exists.
//   • pipelineButtonLabel       — "Pipeline" on month, "Channels"
//                                  elsewhere.
//   • forceDayOnFirstMobile     — a one-shot effect that pins the
//                                  view to 'day' the first time we
//                                  hit a mobile breakpoint, so a
//                                  desktop user resizing down doesn't
//                                  get a cramped week grid.
//   • mobileForceState          — visible mostly for tests; the
//                                  parent's $isMobile / mobileForced
//                                  pair used to live inline as a $effect.

import { mediaQuery } from '$lib/util/mediaQuery';
import {
  addDays,
  endOfWeek,
  startOfWeek
} from './utils';
import type { CalendarViewStateController } from './calendarViewState.svelte';
import type { CalendarFilterStateController } from './calendarFilterState.svelte';

export interface CalendarViewController {
  readonly headline: string;
  readonly pipelineButtonAvailable: boolean;
  readonly pipelineButtonLabel: string;
  /** Reactive $isMobile store value — exposed so the parent template
   *  can keep using it for breakpoint-conditional chrome. */
  readonly isMobile: boolean;
}

export interface CalendarViewDeps {
  viewCtl: CalendarViewStateController;
  filterCtl: CalendarFilterStateController;
}

export function createCalendarView(deps: CalendarViewDeps): CalendarViewController {
  const { viewCtl, filterCtl } = deps;
  const isMobile = mediaQuery('(max-width: 767px)');

  // One-shot "force day view on mobile" rule. Fires on the first
  // render where $isMobile flips true; a desktop user resizing down
  // crosses the breakpoint once and lands on the day view instead of a
  // cramped week. We don't re-fire on subsequent mobile-true flips —
  // the user might have manually picked week and we shouldn't undo
  // that choice.
  let mobileViewForced = $state(false);
  $effect(() => {
    if ($isMobile && !mobileViewForced) {
      viewCtl.view = 'day';
      mobileViewForced = true;
    }
  });

  const headline = $derived.by(() => {
    if (viewCtl.view === 'day') {
      return viewCtl.cursor.toLocaleDateString(undefined, {
        weekday: $isMobile ? 'short' : 'long',
        month: 'short',
        day: 'numeric'
      });
    }
    if (viewCtl.view === 'week') {
      const s = startOfWeek(viewCtl.cursor);
      const e = endOfWeek(viewCtl.cursor);
      if (s.getMonth() === e.getMonth()) {
        return `${s.toLocaleDateString(undefined, { month: 'short' })} ${s.getDate()}–${e.getDate()}`;
      }
      return `${s.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })} – ${e.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}`;
    }
    if (viewCtl.view === 'workweek') {
      const s = addDays(startOfWeek(viewCtl.cursor), 1); // Mon
      const e = addDays(s, 4); // Fri
      if (s.getMonth() === e.getMonth()) {
        return `${s.toLocaleDateString(undefined, { month: 'short' })} ${s.getDate()}–${e.getDate()} (Mon–Fri)`;
      }
      return `${s.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })} – ${e.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })} (Mon–Fri)`;
    }
    if (viewCtl.view === 'month') {
      return viewCtl.cursor.toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
    }
    if (viewCtl.view === 'year') return String(viewCtl.cursor.getFullYear());
    if (viewCtl.view === 'agenda') return 'Agenda · next 30 days';
    return '';
  });

  const pipelineButtonAvailable = $derived(
    (viewCtl.view === 'month' || viewCtl.view === 'week' || viewCtl.view === 'workweek') &&
      (filterCtl.typeCounts['content_event'] ?? 0) > 0
  );

  // Auto-close pipeline mode when the user navigates to a view that
  // doesn't support it. Without this the header button hides but
  // pipelineMode stays true, and a returning view re-renders the
  // overlay in a stale shape.
  $effect(() => {
    if (viewCtl.pipelineMode && !pipelineButtonAvailable) {
      viewCtl.pipelineMode = false;
    }
  });

  const pipelineButtonLabel = $derived(viewCtl.view === 'month' ? 'Pipeline' : 'Channels');

  return {
    get headline() {
      return headline;
    },
    get pipelineButtonAvailable() {
      return pipelineButtonAvailable;
    },
    get pipelineButtonLabel() {
      return pipelineButtonLabel;
    },
    get isMobile() {
      return $isMobile;
    }
  };
}
