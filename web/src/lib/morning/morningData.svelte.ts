// Data feeds for the morning ritual page.
//
// Third extraction step out of routes/morning/+page.svelte. Owns the
// six arrays / maps the page reads against: open tasks, known habits,
// active goals, the title-by-id lookup, upcoming deadlines, active
// prayer intentions, and today's calendar events. Plus the `load()`
// orchestrator that fans the API calls out in parallel, applies the
// graceful-degrade fallbacks for goals/prayer/calendar, and stages
// the responses back through a stale-response gen counter (so an
// older in-flight load can't clobber a newer one if the user
// re-triggers).
//
// The page injects `appendKnownHabit` / `prependActiveIntention` into
// the picks controller pointed at the same fields here — the setters
// on this controller are the single write site so both flows stay in
// sync.
//
// Auxiliary load-failure toasts route through `toast.warning` with a
// label; tasks + habits + deadlines stay load-bearing and surface
// through the loader's error string.

import {
  type Task,
  type HabitInfo,
  type Goal,
  type Deadline,
  type PrayerIntention,
  type CalendarEvent
} from '$lib/api';
import { toast } from '$lib/components/toast';
import { errorMessage } from '$lib/util/errorMessage';

export interface MorningDataDeps {
  /** Today as an ISO date string (yyyy-mm-dd). Captured once at page
   *  init — passed in so the load() result can self-document the day
   *  it pulled (used by the calendar fetch + the warm-habit pre-tick
   *  gate). */
  todayISO: string;

  // API surface — injected so tests can stub. Mirrors the api
  // singleton shape the page used to call directly.
  listTasks: (opts: { status: 'open' }) => Promise<{ tasks: Task[] }>;
  listHabits: () => Promise<{ habits: HabitInfo[] }>;
  listGoals: () => Promise<{ goals: Goal[]; total: number }>;
  tryListDeadlines: () => Promise<Deadline[] | null>;
  listPrayer: () => Promise<{ intentions: PrayerIntention[]; total: number }>;
  calendar: (from: string, to: string) => Promise<{ events: CalendarEvent[] }>;
}

export interface MorningDataController {
  readonly openTasks: Task[];
  readonly knownHabits: HabitInfo[];
  readonly activeGoals: Goal[];
  readonly allGoalsById: Record<string, string>;
  readonly upcomingDeadlines: Deadline[] | null;
  readonly activeIntentions: PrayerIntention[];
  readonly todayEvents: CalendarEvent[];

  /** Hard error from the load-bearing feeds (tasks / habits /
   *  deadlines). Surfaced as a red banner. */
  readonly error: string;

  /** Side-effect setters used by the picks controller's add forms
   *  so the new habit / new intention shows up in the list
   *  immediately, without a reload round-trip. */
  appendKnownHabit(h: HabitInfo): void;
  prependActiveIntention(p: PrayerIntention): void;

  /** Run the full load. Tolerates concurrent re-runs via a gen
   *  counter — a stale completion won't overwrite fresher state. */
  load(): Promise<void>;
}

export function createMorningData(deps: MorningDataDeps): MorningDataController {
  let openTasks = $state<Task[]>([]);
  let knownHabits = $state<HabitInfo[]>([]);
  let activeGoals = $state<Goal[]>([]);
  let allGoalsById = $state<Record<string, string>>({});
  let upcomingDeadlines = $state<Deadline[] | null>(null);
  let activeIntentions = $state<PrayerIntention[]>([]);
  let todayEvents = $state<CalendarEvent[]>([]);
  let error = $state('');

  // Stale-response guard. Bumped on every load(); any await that
  // resumes after a newer load() started simply returns without
  // mutating state. Without this, a slow first request could
  // clobber a fresh second request's data on completion.
  let gen = 0;

  async function load() {
    const my = ++gen;
    error = '';
    try {
      // Auxiliary feeds (goals, prayer, calendar) are graceful-degrade:
      // morning page must keep working when one of them fails, but a
      // silent fallback used to leave the user wondering why their
      // calendar/goals went missing. We log + toast.warning the
      // specific failure so the user can act, then continue with
      // empty shapes. Tasks + habits + deadlines are load-bearing and
      // still bubble up via the outer try/catch.
      const warn = (label: string) => (e: unknown) => {
        console.warn(`[morning] ${label} load failed:`, e);
        toast.warning(`${label} unavailable — ${errorMessage(e)}`);
      };
      const [t, h, g, d, p, cal] = await Promise.all([
        deps.listTasks({ status: 'open' }),
        deps.listHabits(),
        deps.listGoals().catch((e): { goals: Goal[]; total: number } => {
          warn('goals')(e);
          return { goals: [], total: 0 };
        }),
        deps.tryListDeadlines(),
        deps.listPrayer().catch((e) => {
          warn('prayer')(e);
          return { intentions: [] as PrayerIntention[], total: 0 };
        }),
        // Today's events power the AI briefing's "shape of the day"
        // paragraph + the stat-row event count. Tolerate failure —
        // morning page is still useful without the calendar feed.
        deps.calendar(deps.todayISO, deps.todayISO).catch((e) => {
          warn('calendar')(e);
          return { events: [] };
        })
      ]);
      if (my !== gen) return;
      openTasks = t.tasks;
      knownHabits = h.habits;
      activeIntentions = p.intentions.filter((x) => x.status === 'praying');
      activeGoals = g.goals.filter((x) => (x.status ?? 'active') === 'active').slice(0, 3);
      const map: Record<string, string> = {};
      for (const ge of g.goals) map[ge.id] = ge.title;
      allGoalsById = map;
      upcomingDeadlines = d;
      // Filter the feed to events+ics_events only (skip tasks +
      // deadlines, which we summarise from their own data). Sort
      // by start time so the brief reads in chronological order.
      todayEvents = (cal.events ?? [])
        .filter((e) => e.type === 'event' || e.type === 'ics_event')
        .sort((a, b) => (a.start ?? a.date ?? '').localeCompare(b.start ?? b.date ?? ''));
    } catch (e) {
      if (my !== gen) return;
      error = errorMessage(e);
    }
  }

  function appendKnownHabit(h: HabitInfo) {
    knownHabits = [...knownHabits, h];
  }
  function prependActiveIntention(p: PrayerIntention) {
    activeIntentions = [p, ...activeIntentions];
  }

  return {
    get openTasks() {
      return openTasks;
    },
    get knownHabits() {
      return knownHabits;
    },
    get activeGoals() {
      return activeGoals;
    },
    get allGoalsById() {
      return allGoalsById;
    },
    get upcomingDeadlines() {
      return upcomingDeadlines;
    },
    get activeIntentions() {
      return activeIntentions;
    },
    get todayEvents() {
      return todayEvents;
    },
    get error() {
      return error;
    },
    appendKnownHabit,
    prependActiveIntention,
    load
  };
}
