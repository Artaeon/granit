// Data state for the habits surface.
//
// Third extraction step out of routes/habits/+page.svelte. Owns the
// loaded HabitsResponse, the loading + busy + bulkBusy flags, the
// load() function, the three toggle handlers (single-day, today, bulk
// tick-all), and every derivation that operates over the data alone:
//
//   - anchorsFor: reverse-lookup index mapping a habit name to the
//     OTHER habits that anchor to it. The UI surfaces chains in both
//     directions ("after X" + "triggers Y, Z") so the user reads the
//     full chain without scrolling forwards-only.
//   - todayDone / todayTotal: header progress chip ("3 / 7 done today").
//   - undoneToday: list filtered for the "Tick all" button so the
//     header knows whether to show it.
//
// Toggle handlers all use the optimistic-flip + reconciling-load()
// pattern: flip the local data immediately for snappy feel, then
// round-trip the server, then reload to pull authoritative truth.
// Toast errors are funneled through a deps callback so this file
// stays free of the toast singleton — easier to unit-test.
//
// The page still owns onMount install ordering and the WS subscription
// (different lifecycle), and the AI / target / stack / rename surfaces
// (different concerns).

import { api, type HabitInfo, type HabitsResponse, todayISO } from '$lib/api';

export interface HabitsDataDeps {
  /** Boolean snapshot of the auth store — used as a guard before
   *  load(). The page passes () => !!$auth so the read stays
   *  reactive in the calling context. */
  isAuthed: () => boolean;
  /** Toast hook for the catch branches. Injected so the controller
   *  doesn't have to import the toast singleton — keeps it
   *  pure-data, easier to unit-test. */
  onError: (message: string) => void;
}

export interface HabitsDataController {
  readonly data: HabitsResponse | null;
  readonly loading: boolean;
  /** Single-cell busy key (`${name}|${date}` for a dot toggle,
   *  bare name for rename / delete / stack edits). Page surfaces
   *  read this to disable individual buttons. */
  busy: string | null;
  readonly bulkBusy: boolean;

  // Derived.
  /** Reverse-lookup index: habit name -> habits anchored to it. */
  readonly anchorsFor: Record<string, string[]>;
  readonly todayDone: number;
  readonly todayTotal: number;
  readonly undoneToday: HabitInfo[];

  load(): Promise<void>;
  /** Convenience wrapper around toggleOnDate for today's date. */
  toggleToday(h: HabitInfo): Promise<void>;
  /** Click-on-dot retro-toggle. Works for any date, including future
   *  ones (so the user can plan-log). Optimistic flip then reload. */
  toggleOnDate(h: HabitInfo, date: string, want: boolean): Promise<void>;
  /** Power-user shortcut: tick every habit not yet done today. Skips
   *  habits without a taskIdToday (the daily note hasn't materialised
   *  the `## Habits` section yet). */
  tickAllToday(): Promise<void>;
}

export function createHabitsData(deps: HabitsDataDeps): HabitsDataController {
  let data = $state<HabitsResponse | null>(null);
  let loading = $state(false);
  let busy = $state<string | null>(null);
  let bulkBusy = $state(false);

  async function load() {
    if (!deps.isAuthed()) return;
    loading = true;
    try {
      data = await api.listHabits();
    } finally {
      loading = false;
    }
  }

  async function toggleToday(h: HabitInfo) {
    await toggleOnDate(h, data?.today ?? todayISO(), !h.doneToday);
  }

  async function toggleOnDate(h: HabitInfo, date: string, want: boolean) {
    busy = `${h.name}|${date}`;
    if (data) {
      const habit = data.habits.find((x) => x.name === h.name);
      const day = habit?.days.find((d) => d.date === date);
      if (day) day.done = want;
      if (habit && date === data.today) habit.doneToday = want;
      data = { ...data };
    }
    try {
      await api.toggleHabit(h.name, date, want);
      await load();
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      deps.onError(`couldn't toggle: ${msg}`);
      await load(); // restore truth
    } finally {
      busy = null;
    }
  }

  // Power-user shortcut for the morning rhythm: a single click ticks
  // every habit not yet done today. Only enabled when at least one
  // habit can be toggled (some require the daily note's `## Habits`
  // section to exist first — those are skipped). Optimistic flip on
  // each, then a single load() reconciles. Errors are toasted but
  // we keep going for the rest so a single bad row doesn't block the
  // bulk action.
  async function tickAllToday() {
    if (!data || bulkBusy) return;
    const targets = data.habits.filter((h) => !h.doneToday && h.taskIdToday);
    if (targets.length === 0) return;
    bulkBusy = true;
    const today = data.today;
    // Optimistic: flip everything in one pass before the network round-trips
    for (const h of targets) {
      const habit = data.habits.find((x) => x.name === h.name);
      const day = habit?.days.find((d) => d.date === today);
      if (day) day.done = true;
      if (habit) habit.doneToday = true;
    }
    data = { ...data };
    const failed: string[] = [];
    await Promise.all(
      targets.map(async (h) => {
        try {
          await api.toggleHabit(h.name, today, true);
        } catch {
          failed.push(h.name);
        }
      })
    );
    bulkBusy = false;
    await load();
    if (failed.length > 0) {
      deps.onError(`couldn't tick: ${failed.join(', ')}`);
    }
  }

  // Reverse-lookup index: habitName -> list of OTHER habits that
  // anchor to it. Lets the UI surface chains in both directions —
  // when a habit IS an anchor for others, the page shows "triggers:
  // Y, Z" alongside the existing "after X" badge so the user sees
  // the full chain without scrolling through every row to find
  // forward references.
  //
  // Empty entries are intentionally absent (rather than `: []`) so
  // template `{#if anchorsFor[name]?.length}` reads cleanly.
  let anchorsFor = $derived.by<Record<string, string[]>>(() => {
    const out: Record<string, string[]> = {};
    if (!data) return out;
    for (const h of data.habits) {
      const anchor = h.stackAfter;
      if (!anchor) continue;
      if (!out[anchor]) out[anchor] = [];
      out[anchor].push(h.name);
    }
    // Stable ordering — alphabetical, so a 2-tab user doesn't see
    // the list reshuffle when something unrelated changes.
    for (const k of Object.keys(out)) {
      out[k].sort();
    }
    return out;
  });

  let todayDone = $derived(data ? data.habits.filter((h) => h.doneToday).length : 0);
  let todayTotal = $derived(data ? data.habits.length : 0);
  let undoneToday = $derived(
    data ? data.habits.filter((h) => !h.doneToday && h.taskIdToday) : []
  );

  return {
    get data() { return data; },
    get loading() { return loading; },
    // busy stays bidirectional — rename / stack / add controllers
    // write through deps.setBusy which is wired to `(v) => { busy = v; }`
    // from the parent. Drop the others — no caller writes them.
    get busy() { return busy; },
    set busy(v) { busy = v; },
    get bulkBusy() { return bulkBusy; },

    get anchorsFor() { return anchorsFor; },
    get todayDone() { return todayDone; },
    get todayTotal() { return todayTotal; },
    get undoneToday() { return undoneToday; },

    load,
    toggleToday,
    toggleOnDate,
    tickAllToday
  };
}
