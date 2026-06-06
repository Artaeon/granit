// Data caches + WS refresh + debounced full-text search for the
// command palette.
//
// Owns the seven indexed slices (notes, projects, goals, tasks,
// habits, deadlines, events) plus the full-text searchHits and the
// dataLoaded flag the empty-state UI reads. Loading is lazy: the
// palette calls load() on the first open(), Promise.allSettled across
// the parallel fetches, then sets dataLoaded so subsequent opens get
// the cached lists immediately. Per-source errors are swallowed so a
// /goals 500 doesn't kill the whole switcher — the user can still
// jump to pages + notes.
//
// installPaletteDataRefresh() wires the WS subscription: each event
// type only refreshes the relevant slice so a task change doesn't
// drag a goals + projects + notes round-trip with it. Caller is
// responsible for calling it from onMount and returning the cleanup.
//
// runSearch() is debounced 180ms by the caller (via a Svelte $effect
// on the query string) and gated to ≥3 chars — short queries get an
// empty list. searchToken protects against out-of-order responses so
// a slow query never overwrites a faster newer one.

import {
  api,
  fmtDateISO,
  todayISO,
  type Project,
  type Goal,
  type SearchHit,
  type Task,
  type HabitInfo,
  type Deadline,
  type CalendarEvent
} from '$lib/api';
import { onWsEvent } from '$lib/ws';

export interface PaletteNote {
  path: string;
  title: string;
}

export interface PaletteDataController {
  readonly notes: PaletteNote[];
  readonly projects: Project[];
  readonly goals: Goal[];
  readonly tasks: Task[];
  readonly habits: HabitInfo[];
  readonly deadlines: Deadline[];
  readonly events: CalendarEvent[];
  readonly searchHits: SearchHit[];
  readonly dataLoaded: boolean;

  /** Fire all sources in parallel; sets dataLoaded once the slowest
   *  resolves. Idempotent — caller decides whether to skip on a
   *  warm cache (the palette gates on !dataLoaded). */
  load(): Promise<void>;
  /** Run the full-text search for `query`. No-op (and clears hits)
   *  when query.trim().length < 3. Stale responses are dropped via
   *  an internal token. */
  runSearch(query: string): Promise<void>;
  /** Subscribe to WS refresh events so every cache stays warm.
   *  Call from onMount and return the result so the cleanup runs
   *  on unmount. */
  installRefresh(): () => void;
}

export function createPaletteData(): PaletteDataController {
  let notes = $state<PaletteNote[]>([]);
  let projects = $state<Project[]>([]);
  let goals = $state<Goal[]>([]);
  let tasks = $state<Task[]>([]);
  let habits = $state<HabitInfo[]>([]);
  let deadlines = $state<Deadline[]>([]);
  let events = $state<CalendarEvent[]>([]);
  let searchHits = $state<SearchHit[]>([]);
  let dataLoaded = $state(false);
  let searchToken = 0;

  // Single window: today → today + 14 days. Shared by the initial
  // load() call and the WS-driven event.changed refetch so the date
  // math + cutoff stay in lockstep.
  function fetchEventsWindow(): Promise<void> {
    const today = todayISO();
    const cutoffDate = new Date();
    cutoffDate.setDate(cutoffDate.getDate() + 14);
    return api.calendar(today, fmtDateISO(cutoffDate)).then(
      (r) => { events = r.events; },
      () => {}
    );
  }

  async function load(): Promise<void> {
    const np = api.listNotes({ limit: 30 }).then(
      (r) => { notes = r.notes.map((n) => ({ path: n.path, title: n.title })); },
      () => {}
    );
    const pp = api.listProjects().then(
      (r) => { projects = r.projects; },
      () => {}
    );
    const gp = api.listGoals().then(
      (r) => { goals = r.goals; },
      () => {}
    );
    // Open tasks — fuzzy filter searches the text + project name. We
    // pull only open ones (closed tasks aren't actionable navigation
    // targets); the /tasks page is the canonical home if the user
    // wants archived/done. Cap defensively at 200 in case a heavy
    // user has hundreds open — the palette ranks by score so a
    // needle still finds the right row, but we don't ship a list of
    // 500 to fuzzyScore on every keystroke.
    const tp = api.listTasks({ status: 'open' }).then(
      (r) => { tasks = r.tasks.slice(0, 200); },
      () => {}
    );
    // All habits — usually <20, no cap needed.
    const hp = api.listHabits().then(
      (r) => { habits = r.habits; },
      () => {}
    );
    // Active deadlines only (met/cancelled clutter the list).
    const dp = api.tryListDeadlines().then(
      (r) => {
        const all = r ?? [];
        deadlines = all.filter((d) => d.status === 'active');
      },
      () => {}
    );
    // Calendar events — next 14 days. Covers "go to my meeting"
    // jumps. Past events aren't useful as nav targets; further-out
    // events go through the calendar page directly.
    const ep = fetchEventsWindow();
    await Promise.allSettled([np, pp, gp, tp, hp, dp, ep]);
    dataLoaded = true;
  }

  async function runSearch(query: string): Promise<void> {
    const token = ++searchToken;
    if (query.trim().length < 3) {
      searchHits = [];
      return;
    }
    try {
      const r = await api.search(query, 12);
      if (token !== searchToken) return; // stale response, ignore
      searchHits = r.results;
    } catch {
      if (token === searchToken) searchHits = [];
    }
  }

  // Live-refresh every indexed slice on WS events. Each event type
  // only refreshes the relevant slice so a task change doesn't drag
  // a goals + projects + notes round-trip with it.
  function installRefresh(): () => void {
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' || ev.type === 'note.removed') {
        api.listNotes({ limit: 30 }).then(
          (r) => { notes = r.notes.map((n) => ({ path: n.path, title: n.title })); },
          () => {}
        );
      }
      if (ev.type === 'project.changed' || ev.type === 'project.removed') {
        api.listProjects().then(
          (r) => { projects = r.projects; },
          () => {}
        );
      }
      // Goals live in .granit/goals.json — broadcast as state.changed.
      // We only refetch when the goals file specifically changes so a
      // habits sidecar write doesn't trigger a goals API roundtrip.
      if (ev.type === 'state.changed' && ev.path.endsWith('goals.json')) {
        api.listGoals().then(
          (r) => { goals = r.goals; },
          () => {}
        );
      }
      // Tasks — any task mutation invalidates the open-tasks list.
      if (ev.type === 'task.changed') {
        api.listTasks({ status: 'open' }).then(
          (r) => { tasks = r.tasks.slice(0, 200); },
          () => {}
        );
      }
      // Habits sidecars live under .granit/habits/. A check-off or
      // habit-add fires state.changed with that prefix.
      if (ev.type === 'state.changed' && ev.path?.startsWith('.granit/habits/')) {
        api.listHabits().then(
          (r) => { habits = r.habits; },
          () => {}
        );
      }
      // Deadlines live in .granit/deadlines.json.
      if (ev.type === 'state.changed' && ev.path?.endsWith('deadlines.json')) {
        api.tryListDeadlines().then(
          (r) => {
            const all = r ?? [];
            deadlines = all.filter((d) => d.status === 'active');
          },
          () => {}
        );
      }
      // Calendar events — refetch the 14-day window on event
      // mutations. Skipping note.changed here on purpose: note writes
      // fire on every editor autosave keystroke, and piling a
      // calendar refetch onto every one would burn bandwidth for the
      // rare case where a note edit changes a scheduled-task event.
      if (ev.type === 'event.changed' || ev.type === 'event.removed') {
        void fetchEventsWindow();
      }
    });
  }

  return {
    get notes() { return notes; },
    get projects() { return projects; },
    get goals() { return goals; },
    get tasks() { return tasks; },
    get habits() { return habits; },
    get deadlines() { return deadlines; },
    get events() { return events; },
    get searchHits() { return searchHits; },
    get dataLoaded() { return dataLoaded; },
    load,
    runSearch,
    installRefresh
  };
}
