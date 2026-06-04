// Data state for the calendar surface.
//
// Third extraction step out of routes/calendar/+page.svelte. Owns the
// five loaded arrays (feed, nativeEvents, habits, allProjects,
// calSources), the loading + savingSources flags, and the prefetch
// window (fetchFrom / fetchTo) the calendar API call uses.
//
// Also owns every load function — load (feed), loadNativeEvents,
// loadHabits, loadAllProjects, loadSources — plus toggleSource which
// patches the disabled-calendars list with optimistic UI. The page
// still owns the onMount hooks and the WS subscription orchestration:
// install order is route-specific and a controller that ran the
// subscription internally would have to expose the WS unsubscribe
// shape anyway.

import { api, type CalendarFeed, type CalendarEventEntry, type CalendarSource, type HabitInfo, type Project } from '$lib/api';
import { addDays, fmtDateISO } from './utils';
import { toast } from '$lib/components/toast';
import { errorMessage } from '$lib/util/errorMessage';

export interface CalendarDataDeps {
  /** Active cursor from the view controller. load() may widen the
   *  prefetch window when cursor walks outside the current span. */
  getCursor: () => Date;
  /** Boolean snapshot of the auth store — used as a guard before
   *  every API call. The page passes () => !!$auth so the read
   *  stays reactive in the calling context. */
  isAuthed: () => boolean;
}

export interface CalendarDataController {
  // Loaded data — bindable so optimistic updates from outside (drag
  // handlers, the unified-create surface, etc.) can still write back
  // through the controller before the next load().
  feed: CalendarFeed | null;
  nativeEvents: CalendarEventEntry[];
  habits: HabitInfo[];
  allProjects: Project[];
  calSources: CalendarSource[];

  // Loading flags.
  loading: boolean;
  savingSources: boolean;

  // Prefetch window (bindable so the keyboard shortcut + WS handlers
  // can read it; updated inside load() when cursor walks outside it).
  fetchFrom: Date;
  fetchTo: Date;

  /** Refetch the calendar feed for the current prefetch window. The
   *  cursor (read via deps.getCursor) drives the window — if cursor
   *  falls outside fetchFrom..fetchTo, the window is widened first. */
  load(): Promise<void>;
  /** Refetch the editable native event entries (separate from the
   *  feed because the Calendar Agent operates only on these). */
  loadNativeEvents(): Promise<void>;
  /** Refetch habit definitions. Independent of the event feed so a
   *  habit tick doesn't refetch the calendar (and vice versa). */
  loadHabits(): Promise<void>;
  /** Refetch the project sidecar used by the filter dropdown and the
   *  colour-by-project overlay. */
  loadAllProjects(): Promise<void>;
  /** Refetch the calendar source list (per-ICS toggles). */
  loadSources(): Promise<void>;
  /** Optimistically flip a calendar source's enabled flag, push the
   *  new disabled list to the server, and refresh the feed. Rolls
   *  back on failure with a toast. */
  toggleSource(src: CalendarSource): Promise<void>;
}

export function createCalendarData(deps: CalendarDataDeps): CalendarDataController {
  let feed = $state<CalendarFeed | null>(null);
  let nativeEvents = $state<CalendarEventEntry[]>([]);
  let habits = $state<HabitInfo[]>([]);
  let allProjects = $state<Project[]>([]);
  let calSources = $state<CalendarSource[]>([]);
  let loading = $state(false);
  let savingSources = $state(false);
  let fetchFrom = $state(addDays(new Date(), -7));
  let fetchTo = $state(addDays(new Date(), 60));

  async function load() {
    if (!deps.isAuthed()) return;
    loading = true;
    try {
      const cursor = deps.getCursor();
      if (cursor < fetchFrom || cursor > fetchTo) {
        fetchFrom = addDays(cursor, -14);
        fetchTo = addDays(cursor, 60);
      }
      feed = await api.calendar(fmtDateISO(fetchFrom), fmtDateISO(fetchTo));
    } finally {
      loading = false;
    }
  }

  async function loadNativeEvents() {
    if (!deps.isAuthed()) return;
    try {
      const r = await api.listEvents();
      nativeEvents = r.events;
    } catch {
      nativeEvents = [];
    }
  }

  async function loadHabits() {
    if (!deps.isAuthed()) return;
    try {
      const r = await api.listHabits();
      habits = r.habits;
    } catch {
      habits = [];
    }
  }

  async function loadAllProjects() {
    try {
      const r = await api.listProjects();
      allProjects = r.projects ?? [];
    } catch {
      allProjects = [];
    }
  }

  async function loadSources() {
    try {
      const r = await api.listCalendarSources();
      calSources = r.sources;
    } catch {
      // No vault access yet, or no .ics files — silently skip.
      calSources = [];
    }
  }

  async function toggleSource(src: CalendarSource) {
    // Snapshot the desired post-toggle state up front so subsequent
    // logic doesn't depend on `src.enabled` mutating mid-flight (the
    // `src` we received is a proxy backed by `calSources`; if anything
    // races with this handler the read could observe the wrong value).
    const targetId = src.id;
    const wasEnabled = src.enabled;
    const willBeEnabled = !wasEnabled;
    savingSources = true;
    // Optimistic flip — write a NEW array (not an in-place mutation
    // of the existing proxy), so Svelte's keyed each block re-runs
    // its bindings with the right `enabled` value before the network
    // round-trip completes. Without this the user saw the toggle stay
    // on its old visual state until the second click triggered a
    // re-render via the response replace, hence "click twice".
    calSources = calSources.map((s) =>
      s.id === targetId ? { ...s, enabled: willBeEnabled } : s
    );
    // Compute the new `disabled` list from the OPTIMISTIC state so
    // we send the canonical set the user just expressed intent for.
    const newDisabled = calSources.filter((s) => !s.enabled).map((s) => s.source);
    try {
      const r = await api.patchCalendarSources(newDisabled);
      // Replace with the server's authoritative shape (always a fresh
      // array reference — guards against any future shape drift).
      calSources = [...r.sources];
      await load(); // refresh feed with new disabled list
    } catch (e) {
      // Roll the optimistic flip back so the UI matches what the
      // server still has on disk.
      calSources = calSources.map((s) =>
        s.id === targetId ? { ...s, enabled: wasEnabled } : s
      );
      toast.error('save failed: ' + errorMessage(e));
    } finally {
      savingSources = false;
    }
  }

  return {
    get feed() {
      return feed;
    },
    set feed(v) {
      feed = v;
    },
    get nativeEvents() {
      return nativeEvents;
    },
    set nativeEvents(v) {
      nativeEvents = v;
    },
    get habits() {
      return habits;
    },
    set habits(v) {
      habits = v;
    },
    get allProjects() {
      return allProjects;
    },
    set allProjects(v) {
      allProjects = v;
    },
    get calSources() {
      return calSources;
    },
    set calSources(v) {
      calSources = v;
    },
    get loading() {
      return loading;
    },
    set loading(v) {
      loading = v;
    },
    get savingSources() {
      return savingSources;
    },
    set savingSources(v) {
      savingSources = v;
    },
    get fetchFrom() {
      return fetchFrom;
    },
    set fetchFrom(v) {
      fetchFrom = v;
    },
    get fetchTo() {
      return fetchTo;
    },
    set fetchTo(v) {
      fetchTo = v;
    },
    load,
    loadNativeEvents,
    loadHabits,
    loadAllProjects,
    loadSources,
    toggleSource
  };
}
