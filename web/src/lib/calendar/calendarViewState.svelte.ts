// View / display state for the calendar surface.
//
// First extraction step out of routes/calendar/+page.svelte (1743 LOC).
// Owns the visual shape of the page: which view (day / workweek /
// week / month / year / agenda), the date the cursor is anchored on,
// plan-mode (single-day backlog + grid split), month grid density,
// hour-grid density (+ derived hourPx), the pipeline overlay flag,
// plus the navigation primitives (prev / next / gotoToday) and the
// per-view day-column derivation (viewDays).
//
// All persisted preferences live here with their localStorage keys so
// the page no longer carries any granit.calendar.* string literals.
// Same shape the tasksViewState controller introduced — getter/setter
// pairs for bindable state, getter-only for derivations, methods for
// imperative actions.

import { addDays, startOfWeek } from './utils';
import { loadStored, loadStoredString, saveStored, saveStoredString } from '$lib/util/storage';

export type View = 'day' | 'workweek' | 'week' | 'month' | 'year' | 'agenda';

export type HourDensity = 'compact' | 'normal' | 'spacious';

const VIEW_KEY = 'granit.calendar.view';
const PLAN_KEY = 'granit.calendar.planmode';
const MONTH_DENSITY_KEY = 'granit.calendar.monthDensity';
const HOUR_DENSITY_KEY = 'granit.calendar.hourDensity';

function defaultView(): View {
  // Persisted last-used view wins on every device. On a fresh visit
  // (no saved preference) we default to 'day' on small screens
  // because the 7-column week grid is unreadable below ~640px —
  // Google Calendar does the same.
  if (typeof window === 'undefined') return 'week';
  const saved = loadStoredString(VIEW_KEY, '');
  if (saved) return saved as View;
  return window.innerWidth < 640 ? 'day' : 'week';
}

function loadHourDensity(): HourDensity {
  const v = loadStoredString(HOUR_DENSITY_KEY, 'normal');
  return v === 'compact' || v === 'spacious' ? v : 'normal';
}

export interface CalendarViewStateController {
  // Bindable view config.
  view: View;
  cursor: Date;
  planMode: boolean;
  monthDensity: 'comfy' | 'compact';
  hourDensity: HourDensity;
  pipelineMode: boolean;

  // Derived (readonly).
  /** Per-hour pixel height for the time grid. Compact = 32px (~14h
   *  on a typical laptop without scroll), normal = 48px (historical
   *  default), spacious = 72px (meeting-heavy days). */
  readonly hourPx: number;
  /** Day columns rendered by the day/week/workweek views. Empty for
   *  month/year/agenda — those views render their own shape. */
  readonly viewDays: Date[];

  // Methods.
  /** Walk one step backwards in the active view's grain — one day in
   *  day view, one week in week/workweek, one month in month, one
   *  year in year, one week in agenda. */
  prev(): void;
  /** Walk one step forward — symmetric counterpart to prev(). */
  next(): void;
  /** Reset cursor to today. */
  gotoToday(): void;
  /** Flip plan mode. When turning ON, force view='day' so the layout
   *  stays sensible (a 7-column week grid + side rail is too tight
   *  on most screens). */
  togglePlanMode(): void;
}

export function createCalendarViewState(): CalendarViewStateController {
  let view = $state<View>(defaultView());
  let cursor = $state(new Date());
  let planMode = $state<boolean>(loadStoredString(PLAN_KEY, '0') === '1');
  let monthDensity = $state<'comfy' | 'compact'>(
    loadStoredString(MONTH_DENSITY_KEY, 'comfy') === 'compact' ? 'compact' : 'comfy'
  );
  let hourDensity = $state<HourDensity>(loadHourDensity());
  let pipelineMode = $state(false);

  // Persistence — same shape the page had inline.
  $effect(() => saveStoredString(VIEW_KEY, view));
  $effect(() => saveStoredString(PLAN_KEY, planMode ? '1' : '0'));
  $effect(() => saveStoredString(MONTH_DENSITY_KEY, monthDensity));
  $effect(() => saveStoredString(HOUR_DENSITY_KEY, hourDensity));

  let hourPx = $derived(
    hourDensity === 'compact' ? 32 : hourDensity === 'spacious' ? 72 : 48
  );

  let viewDays = $derived.by<Date[]>(() => {
    if (view === 'day') return [cursor];
    if (view === 'week') {
      const s = startOfWeek(cursor);
      return Array.from({ length: 7 }, (_, i) => addDays(s, i));
    }
    if (view === 'workweek') {
      // Mon–Fri anchored on the week containing cursor. startOfWeek
      // resolves to Sunday (locale-agnostic in this codebase), so we
      // step one day forward and emit five days. Saturday/Sunday are
      // dropped — the time-grid columns scale to fill width.
      const s = startOfWeek(cursor);
      const mon = addDays(s, 1);
      return Array.from({ length: 5 }, (_, i) => addDays(mon, i));
    }
    return [];
  });

  function prev() {
    if (view === 'day') cursor = addDays(cursor, -1);
    else if (view === 'week' || view === 'workweek') cursor = addDays(cursor, -7);
    else if (view === 'month')
      cursor = new Date(cursor.getFullYear(), cursor.getMonth() - 1, 1);
    else if (view === 'year')
      cursor = new Date(cursor.getFullYear() - 1, cursor.getMonth(), 1);
    else if (view === 'agenda') cursor = addDays(cursor, -7);
  }

  function next() {
    if (view === 'day') cursor = addDays(cursor, 1);
    else if (view === 'week' || view === 'workweek') cursor = addDays(cursor, 7);
    else if (view === 'month')
      cursor = new Date(cursor.getFullYear(), cursor.getMonth() + 1, 1);
    else if (view === 'year')
      cursor = new Date(cursor.getFullYear() + 1, cursor.getMonth(), 1);
    else if (view === 'agenda') cursor = addDays(cursor, 7);
  }

  function gotoToday() {
    cursor = new Date();
  }

  function togglePlanMode() {
    planMode = !planMode;
    if (planMode) view = 'day';
  }

  return {
    get view() {
      return view;
    },
    set view(v) {
      view = v;
    },
    get cursor() {
      return cursor;
    },
    set cursor(v) {
      cursor = v;
    },
    get planMode() {
      return planMode;
    },
    set planMode(v) {
      planMode = v;
    },
    get monthDensity() {
      return monthDensity;
    },
    set monthDensity(v) {
      monthDensity = v;
    },
    get hourDensity() {
      return hourDensity;
    },
    set hourDensity(v) {
      hourDensity = v;
    },
    get pipelineMode() {
      return pipelineMode;
    },
    set pipelineMode(v) {
      pipelineMode = v;
    },
    get hourPx() {
      return hourPx;
    },
    get viewDays() {
      return viewDays;
    },
    prev,
    next,
    gotoToday,
    togglePlanMode
  };
}
