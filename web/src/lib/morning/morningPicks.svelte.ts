// Picked-tasks / habits / intentions state for the morning ritual.
//
// Second extraction step out of routes/morning/+page.svelte. Owns the
// three Set<string> selections the page tracks (task ids, habit names,
// prayer-intention ids), the toggle handlers, the show-all-tasks
// chrome flag, the "add a custom habit" form state, and the
// "add a new prayer" inline-create form (API call included).
//
// Reads its data dependencies via the deps bundle so the controller
// doesn't have to import the data loader directly: openTasks +
// knownHabits + activeIntentions live in the morningData controller
// next door, and the API client is injected for ergonomic testing.
//
// The page still owns the rendering and reads picked sizes via
// derived getters. Restore from the per-day snapshot lands through
// the `restore()` method which mirrors the persist() shape.

import { type HabitInfo, type PrayerIntention } from '$lib/api';
import { toast } from '$lib/components/toast';
import { errorMessage } from '$lib/util/errorMessage';

export interface MorningPicksDeps {
  /** Push a freshly-typed habit name into the known-habits list so
   *  the chip renders immediately. Owned by the data controller. */
  appendKnownHabit: (h: HabitInfo) => void;
  /** Prepend a newly-created prayer onto the active list so the
   *  picker can render it as already-checked. */
  prependActiveIntention: (p: PrayerIntention) => void;
  /** API call to create a prayer intention server-side. Injected so
   *  unit tests can stub it. */
  createPrayer: (args: {
    text: string;
    status: 'praying';
  }) => Promise<PrayerIntention>;
}

export interface MorningPicksController {
  pickedTasks: Set<string>;
  pickedHabits: Set<string>;
  pickedIntentions: Set<string>;
  showAllTasks: boolean;
  prayerPickerOpen: boolean;
  /** Free-text input for the "add a habit" form. */
  newHabit: string;
  /** Free-text input for the inline "add a prayer" form. */
  newPrayerText: string;
  /** True while the addNewPrayer() round-trip is pending. */
  readonly addingPrayer: boolean;

  toggleTask(id: string): void;
  toggleHabit(name: string): void;
  toggleIntention(id: string): void;
  /** Submit handler for the add-habit form. Adds the typed name to
   *  both the picked set and the known-habits list (via deps). */
  addCustomHabit(e: Event): void;
  /** Submit handler for the add-prayer form. POSTs to the API, then
   *  prepends the created intention and auto-picks it. */
  addNewPrayer(e: Event): Promise<void>;

  /** Pre-tick "warm" habits — anything with >=50% last-7d adherence.
   *  Only safe to call on a fresh page (no snapshot restored), else
   *  the user's deliberate choices get clobbered. */
  pretickWarmHabits(known: HabitInfo[]): void;

  /** Restore from snapshot — replaces all three sets. */
  restore(snap: {
    pickedTasks?: string[];
    pickedHabits?: string[];
    pickedIntentions?: string[];
    newHabit?: string;
  }): void;
}

export function createMorningPicks(deps: MorningPicksDeps): MorningPicksController {
  let pickedTasks = $state<Set<string>>(new Set());
  let pickedHabits = $state<Set<string>>(new Set());
  let pickedIntentions = $state<Set<string>>(new Set());
  let showAllTasks = $state(false);
  let prayerPickerOpen = $state(false);
  let newHabit = $state('');
  let newPrayerText = $state('');
  let addingPrayer = $state(false);

  function toggleTask(id: string) {
    if (pickedTasks.has(id)) pickedTasks.delete(id);
    else pickedTasks.add(id);
    pickedTasks = new Set(pickedTasks);
  }
  function toggleHabit(name: string) {
    if (pickedHabits.has(name)) pickedHabits.delete(name);
    else pickedHabits.add(name);
    pickedHabits = new Set(pickedHabits);
  }
  function toggleIntention(id: string) {
    if (pickedIntentions.has(id)) pickedIntentions.delete(id);
    else pickedIntentions.add(id);
    pickedIntentions = new Set(pickedIntentions);
  }
  function addCustomHabit(e: Event) {
    e.preventDefault();
    const n = newHabit.trim();
    if (!n) return;
    pickedHabits.add(n);
    pickedHabits = new Set(pickedHabits);
    deps.appendKnownHabit({
      name: n,
      days: [],
      currentStreak: 0,
      longestStreak: 0,
      last7Pct: 0,
      last30Pct: 0,
      doneToday: false
    });
    newHabit = '';
  }
  async function addNewPrayer(e: Event) {
    e.preventDefault();
    const text = newPrayerText.trim();
    if (!text || addingPrayer) return;
    addingPrayer = true;
    try {
      const created = await deps.createPrayer({ text, status: 'praying' });
      deps.prependActiveIntention(created);
      pickedIntentions.add(created.id);
      pickedIntentions = new Set(pickedIntentions);
      newPrayerText = '';
    } catch (err) {
      toast.error(`failed: ${errorMessage(err)}`);
    } finally {
      addingPrayer = false;
    }
  }

  function pretickWarmHabits(known: HabitInfo[]) {
    for (const k of known) {
      if (k.last7Pct >= 50) pickedHabits.add(k.name);
    }
    pickedHabits = new Set(pickedHabits);
  }

  function restore(snap: {
    pickedTasks?: string[];
    pickedHabits?: string[];
    pickedIntentions?: string[];
    newHabit?: string;
  }) {
    pickedTasks = new Set(snap.pickedTasks ?? []);
    pickedHabits = new Set(snap.pickedHabits ?? []);
    pickedIntentions = new Set(snap.pickedIntentions ?? []);
    newHabit = snap.newHabit ?? '';
  }

  return {
    get pickedTasks() {
      return pickedTasks;
    },
    set pickedTasks(v) {
      pickedTasks = v;
    },
    get pickedHabits() {
      return pickedHabits;
    },
    set pickedHabits(v) {
      pickedHabits = v;
    },
    get pickedIntentions() {
      return pickedIntentions;
    },
    set pickedIntentions(v) {
      pickedIntentions = v;
    },
    get showAllTasks() {
      return showAllTasks;
    },
    set showAllTasks(v) {
      showAllTasks = v;
    },
    get prayerPickerOpen() {
      return prayerPickerOpen;
    },
    set prayerPickerOpen(v) {
      prayerPickerOpen = v;
    },
    get newHabit() {
      return newHabit;
    },
    set newHabit(v) {
      newHabit = v;
    },
    get newPrayerText() {
      return newPrayerText;
    },
    set newPrayerText(v) {
      newPrayerText = v;
    },
    get addingPrayer() {
      return addingPrayer;
    },
    toggleTask,
    toggleHabit,
    toggleIntention,
    addCustomHabit,
    addNewPrayer,
    pretickWarmHabits,
    restore
  };
}
