// Filter state for the calendar surface.
//
// Second extraction step out of routes/calendar/+page.svelte. Owns
// every dimension the user can filter the rendered feed by — the
// event-type chip set (`hidden`), the project-scoped board mode
// (`projectFilter`), the kind chip set (`kindFilter`), the colour-
// by-project overlay flag, and the slide-out filter drawer's
// open-state. All persistence lives here.
//
// Also owns the four core derivations that downstream views read
// from: the filtered-and-coloured `events` array, the per-type
// `typeCounts`, the visible chip strip (`visibleFilterChips` —
// hides the Content chip when no content events exist), and the
// month / agenda sub-derivations that the MonthView and AgendaView
// iterate.
//
// External state the controller can't own (the loaded feed,
// allProjects sidecar, cursor for the month-window slice) is
// reached via the deps bundle. sourceColors is imported directly
// because it's a writable store with its own persistence —
// applying source-colour overrides is part of the filter pipeline,
// not a separate concern.

import { applySourceColor, sourceColors } from './sourceColors';
import { fmtDateISO, addDays, startOfMonth } from './utils';
import { loadStored, loadStoredString, saveStored, saveStoredString } from '$lib/util/storage';
import type { CalendarEvent, Project } from '$lib/api';

const FILTER_KEY = 'granit.calendar.filters';
const PROJECT_FILTER_KEY = 'granit.calendar.project';
const KIND_FILTER_KEY = 'granit.calendar.kindFilter';
const COLOR_BY_PROJECT_KEY = 'granit.calendar.colorByProject';

export type EventFilterKey =
  | 'daily'
  | 'task_due'
  | 'task_scheduled'
  | 'event'
  | 'ics_event'
  | 'deadline'
  | 'goal_target'
  | 'meal_slot'
  | 'content_event';

export type ChipDef = { key: EventFilterKey; label: string; tone: string };

// Stable filter chip catalog — hoisted so each render iterates the
// same array (an inline literal in the template would tear keyed
// children down on every re-render). Drives both the always-visible
// pill strip above the grid and the sidebar Filters section.
export const FILTER_CHIPS: ReadonlyArray<ChipDef> = [
  { key: 'event', label: 'Events', tone: 'info' },
  { key: 'ics_event', label: 'ICS', tone: 'info' },
  { key: 'task_scheduled', label: 'Scheduled', tone: 'primary' },
  { key: 'task_due', label: 'Due', tone: 'warning' },
  { key: 'deadline', label: 'Deadlines', tone: 'error' },
  { key: 'goal_target', label: 'Goals', tone: 'mauve' },
  { key: 'meal_slot', label: 'Meals', tone: 'subtext' },
  { key: 'daily', label: 'Daily', tone: 'secondary' },
  // Content events get their own chip — same tone as the 'content'
  // event-type catalog colour so the chip + the on-grid accent read
  // as one feature. Hidden from the visible row when count = 0 (see
  // visibleFilterChips) so non-content users see no extra clutter.
  { key: 'content_event', label: 'Content', tone: 'lavender' }
];

function loadKindFilterFromStorage(): Set<string> {
  const raw = loadStoredString(KIND_FILTER_KEY, '');
  if (!raw) return new Set();
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (Array.isArray(parsed))
      return new Set(parsed.filter((x): x is string => typeof x === 'string'));
  } catch {
    // Malformed — fall through to empty set.
  }
  return new Set();
}

export interface CalendarFilterStateDeps {
  /** Raw event list from the loaded feed. The controller filters +
   *  colours it; iterating it directly bypasses every chip. */
  getAllEvents: () => CalendarEvent[];
  /** Projects sidecar — used to build the per-project colour map
   *  for the colourByProject overlay. */
  getAllProjects: () => Project[];
  /** Cursor from the view controller. Drives the monthEvents +
   *  agendaEvents window slices. */
  getCursor: () => Date;
}

export interface CalendarFilterStateController {
  // Bindable filter state.
  hidden: Set<EventFilterKey>;
  projectFilter: string;
  kindFilter: Set<string>;
  filterDrawerOpen: boolean;
  colorByProject: boolean;

  // Derived (readonly).
  /** Filtered + coloured event list. The single source of truth
   *  every downstream view (HourGrid / MonthView / AgendaView /
   *  YearView) reads from. */
  readonly events: CalendarEvent[];
  /** Per-event-type count across the raw (pre-filter) feed —
   *  drives the chip badge numbers and the content-chip visibility
   *  gate. */
  readonly typeCounts: Record<string, number>;
  /** Visible chip strip — strips the Content chip out for users
   *  who aren't running a content pipeline. */
  readonly visibleFilterChips: ChipDef[];
  /** Project name → project colour map, built once from
   *  allProjects. Used by the colourByProject overlay. */
  readonly projectColorMap: Map<string, string>;
  /** Month-window slice of events for the MonthView grid. */
  readonly monthEvents: CalendarEvent[];
  /** Rolling 30-day forward slice for the AgendaView. */
  readonly agendaEvents: CalendarEvent[];

  /** Toggle the visibility of an event-type chip. */
  toggleType(t: EventFilterKey): void;
  /** Toggle a kind in the kindFilter set. */
  toggleKindFilter(id: string): void;
  /** Clear every kind from kindFilter. */
  clearKindFilter(): void;
}

export function createCalendarFilterState(
  deps: CalendarFilterStateDeps
): CalendarFilterStateController {
  let hidden = $state<Set<EventFilterKey>>(
    new Set(loadStored<EventFilterKey[]>(FILTER_KEY, []))
  );
  let projectFilter = $state<string>(loadStoredString(PROJECT_FILTER_KEY, ''));
  let kindFilter = $state<Set<string>>(loadKindFilterFromStorage());
  let filterDrawerOpen = $state(false);
  let colorByProject = $state<boolean>(
    loadStoredString(COLOR_BY_PROJECT_KEY, '0') === '1'
  );

  $effect(() => saveStored(FILTER_KEY, Array.from(hidden)));
  $effect(() => saveStoredString(PROJECT_FILTER_KEY, projectFilter));
  $effect(() =>
    saveStoredString(KIND_FILTER_KEY, JSON.stringify([...kindFilter]))
  );
  $effect(() =>
    saveStoredString(COLOR_BY_PROJECT_KEY, colorByProject ? '1' : '0')
  );

  let projectColorMap = $derived.by<Map<string, string>>(() => {
    const m = new Map<string, string>();
    for (const p of deps.getAllProjects()) {
      if (p.name && p.color) m.set(p.name, p.color);
    }
    return m;
  });

  let events = $derived.by<CalendarEvent[]>(() => {
    const all = deps.getAllEvents();
    const tones = $sourceColors;
    return all
      .filter((e) => !hidden.has(e.type as EventFilterKey))
      .filter((e) => {
        // content_event is a derived filter: when the chip is
        // toggled off (key in hidden), we hide events that look
        // like content (type=event + kind=content). Non-content
        // events are unaffected.
        if (!hidden.has('content_event')) return true;
        return !(e.type === 'event' && e.kind === 'content');
      })
      .filter((e) => {
        if (!projectFilter) return true;
        return e.project_id === projectFilter;
      })
      .filter((e) => {
        // Empty kindFilter = show everything (no filter applied).
        // Non-empty set: only events whose type is calendar-related
        // (event / ics_event) participate in the type filter;
        // tasks + deadlines never carry a kind and always pass
        // through so a "show only Focus events" filter doesn't
        // hide today's overdue task. The '__untyped' sentinel id
        // lets the user also include kind-less events. Trim +
        // lowercase the stored kind defensively so a hand-edited
        // " Meeting " or "FOCUS" value still matches against the
        // lowercase catalog ids.
        if (kindFilter.size === 0) return true;
        if (e.type !== 'event' && e.type !== 'ics_event') return true;
        const k = (e.kind ?? '').trim().toLowerCase();
        return k ? kindFilter.has(k) : kindFilter.has('__untyped');
      })
      .map((e) => applySourceColor(e, tones))
      .map((e) => {
        if (!colorByProject || !e.project_id) return e;
        const c = projectColorMap.get(e.project_id);
        if (!c) return e;
        // Only override the user-event color path — task / deadline
        // rows have their own meaningful colour rules (priority,
        // importance) we shouldn't trample.
        if (
          e.type !== 'event' &&
          e.type !== 'task_scheduled' &&
          e.type !== 'task_due'
        )
          return e;
        return { ...e, color: c };
      });
  });

  let typeCounts = $derived.by<Record<string, number>>(() => {
    const c: Record<string, number> = {};
    for (const e of deps.getAllEvents()) {
      c[e.type] = (c[e.type] ?? 0) + 1;
      // content_event isn't a wire-shape type — it's a kind on
      // type='event'. Count separately so the chip can show its
      // own tally (and the visibility gate below has a value to
      // read).
      if (e.type === 'event' && e.kind === 'content') {
        c['content_event'] = (c['content_event'] ?? 0) + 1;
      }
    }
    return c;
  });

  let visibleFilterChips = $derived<ChipDef[]>(
    FILTER_CHIPS.filter(
      (c) =>
        c.key !== 'content_event' || (typeCounts['content_event'] ?? 0) > 0
    )
  );

  let monthEvents = $derived.by<CalendarEvent[]>(() => {
    const cursor = deps.getCursor();
    const ms = startOfMonth(cursor);
    const me = new Date(ms.getFullYear(), ms.getMonth() + 1, 0);
    return events.filter((ev) => {
      const key = ev.date ?? (ev.start ? ev.start.slice(0, 10) : '');
      if (!key) return false;
      return key >= fmtDateISO(ms) && key <= fmtDateISO(me);
    });
  });

  // Agenda view shows a rolling 30-day flat list anchored at
  // cursor. Past-dated events stay invisible — the agenda is a
  // "what's next" surface, not a historical log (the day/week
  // views and tasks dashboard cover the look-back use case).
  let agendaEvents = $derived.by<CalendarEvent[]>(() => {
    const cursor = deps.getCursor();
    const from = fmtDateISO(cursor);
    const to = fmtDateISO(addDays(cursor, 30));
    return events.filter((ev) => {
      const key = ev.date ?? (ev.start ? ev.start.slice(0, 10) : '');
      if (!key) return false;
      return key >= from && key <= to;
    });
  });

  function toggleType(t: EventFilterKey) {
    const next = new Set(hidden);
    if (next.has(t)) next.delete(t);
    else next.add(t);
    hidden = next;
  }

  function toggleKindFilter(id: string) {
    const next = new Set(kindFilter);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    kindFilter = next;
  }

  function clearKindFilter() {
    kindFilter = new Set();
  }

  return {
    get hidden() {
      return hidden;
    },
    set hidden(v) {
      hidden = v;
    },
    get projectFilter() {
      return projectFilter;
    },
    set projectFilter(v) {
      projectFilter = v;
    },
    get kindFilter() {
      return kindFilter;
    },
    set kindFilter(v) {
      kindFilter = v;
    },
    get filterDrawerOpen() {
      return filterDrawerOpen;
    },
    set filterDrawerOpen(v) {
      filterDrawerOpen = v;
    },
    get colorByProject() {
      return colorByProject;
    },
    set colorByProject(v) {
      colorByProject = v;
    },
    get events() {
      return events;
    },
    get typeCounts() {
      return typeCounts;
    },
    get visibleFilterChips() {
      return visibleFilterChips;
    },
    get projectColorMap() {
      return projectColorMap;
    },
    get monthEvents() {
      return monthEvents;
    },
    get agendaEvents() {
      return agendaEvents;
    },
    toggleType,
    toggleKindFilter,
    clearKindFilter
  };
}
