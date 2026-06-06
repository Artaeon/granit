// Add-habit-from-web controller.
//
// Sixth extraction step out of routes/habits/+page.svelte. Owns the
// add-form open/closed state, the in-progress name buffer, the
// submit-busy flag, and the addHabit() handler that lands the new
// habit on today's daily note.
//
// The existing toggleHabit endpoint already auto-creates the
// `- [ ] habit` line when the supplied name doesn't match anything in
// today's `## Habits` section (and creates the section + minimal
// frontmatter when the daily note doesn't exist yet). So "add a
// habit" is just a toggle call with done=false on a fresh name —
// keeps the API surface tight.
//
// The page wires aiCtl's adopt() callback through here so a
// suggested-from-goals habit gets dropped into the same pipeline:
// adopt(name) -> setName(name) -> addHabit() -> reload + dismiss.

import { api, todayISO } from '$lib/api';

export interface HabitsAddDeps {
  /** Reactive getter for the loaded HabitsResponse — used to pick up
   *  the server's today-date so day rollover doesn't desync. Falls
   *  back to client todayISO() before the first load. */
  getToday: () => string | undefined;
  /** Reload after a successful add so the new habit appears in the
   *  list. */
  reload: () => Promise<void>;
  /** Toast hook for the failure branch. */
  onError: (message: string) => void;
}

export interface HabitsAddController {
  addOpen: boolean;
  addName: string;
  readonly addBusy: boolean;
  /** Submit handler — also reachable from aiCtl.adopt(name) which
   *  pre-fills the name buffer before calling addHabit(). */
  addHabit(e?: Event): Promise<void>;
}

export function createHabitsAdd(deps: HabitsAddDeps): HabitsAddController {
  let addOpen = $state(false);
  let addName = $state('');
  let addBusy = $state(false);

  async function addHabit(e?: Event) {
    e?.preventDefault();
    const name = addName.trim();
    if (!name || addBusy) return;
    addBusy = true;
    try {
      // done=false for a fresh "track this" intent (the user hasn't
      // done it today yet, just wants the habit in the list).
      await api.toggleHabit(name, deps.getToday() ?? todayISO(), false);
      addName = '';
      addOpen = false;
      await deps.reload();
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      deps.onError(`couldn't add habit: ${msg}`);
    } finally {
      addBusy = false;
    }
  }

  return {
    get addOpen() { return addOpen; },
    set addOpen(v) { addOpen = v; },
    get addName() { return addName; },
    set addName(v) { addName = v; },
    get addBusy() { return addBusy; },
    addHabit
  };
}
