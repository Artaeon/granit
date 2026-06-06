// Per-day localStorage round-trip for the morning ritual.
//
// Sixth extraction step out of routes/morning/+page.svelte. Wraps the
// Snapshot interface, the persist/restore/clearPersisted trio, and
// the $effect that fires persist() whenever a tracked field changes.
//
// The snapshot scope:
//   - scripture (rotation source + custom text + custom source)
//   - focus (winSentence + goal + linkedGoalId)
//   - picks (three Sets + the in-flight newHabit text)
//   - thoughts (free-text textarea on the page)
//
// The "Bring forward" thoughts textarea lives on the page itself
// (small enough that a dedicated controller would be overkill), so
// it's wired in via getter+setter rather than a controller binding.
// Same for the storage key — passed in so a future per-user override
// stays cheap.
//
// install() returns the persist + restore + clear handles plus the
// effect teardown (run() the effect to actually subscribe; the
// returned dispose lets a future remount uninstall cleanly).

import { loadStored, saveStored } from '$lib/util/storage';
import type { MorningScriptureController } from './morningScripture.svelte';
import type { MorningFocusController } from './morningFocus.svelte';
import type { MorningPicksController } from './morningPicks.svelte';

interface Snapshot {
  scriptureSource: string;
  customScripture: string;
  customSource: string;
  winSentence: string;
  goal: string;
  linkedGoalId: string;
  pickedTasks: string[];
  pickedHabits: string[];
  pickedIntentions: string[];
  thoughts: string;
  newHabit: string;
}

export interface MorningPersistenceDeps {
  /** Storage key. Page passes `granit.morning.${today}` so yesterday's
   *  half-finished progress doesn't bleed into today. */
  storageKey: string;
  scriptureCtl: MorningScriptureController;
  focusCtl: MorningFocusController;
  picksCtl: MorningPicksController;
  /** Read+write of the "Bring forward" thoughts textarea, kept on the
   *  page itself. */
  getThoughts: () => string;
  setThoughts: (v: string) => void;
}

export interface MorningPersistenceController {
  /** Returns true if a snapshot was found and applied; false on a
   *  fresh load (caller uses this to gate the warm-habit pre-tick). */
  restore(): boolean;
  /** Drop the snapshot — called after a successful lockIn save so
   *  the next page open starts clean. */
  clear(): void;
  /** True iff a snapshot key is present in localStorage. Used by the
   *  load() flow to decide whether to pre-tick warm habits. */
  hasSnapshot(): boolean;
}

/** Install the persistence round-trip. The $effect that auto-saves
 *  on field changes is wired up via the deps' controllers; call
 *  `restore()` once during onMount to hydrate, and `clear()` after a
 *  successful lockIn. */
export function installMorningPersistence(
  deps: MorningPersistenceDeps
): MorningPersistenceController {
  const {
    storageKey,
    scriptureCtl,
    focusCtl,
    picksCtl,
    getThoughts,
    setThoughts
  } = deps;

  function persist() {
    const s: Snapshot = {
      scriptureSource: scriptureCtl.scripture.source,
      customScripture: scriptureCtl.customScripture,
      customSource: scriptureCtl.customSource,
      winSentence: focusCtl.winSentence,
      goal: focusCtl.goal,
      linkedGoalId: focusCtl.linkedGoalId,
      pickedTasks: [...picksCtl.pickedTasks],
      pickedHabits: [...picksCtl.pickedHabits],
      pickedIntentions: [...picksCtl.pickedIntentions],
      thoughts: getThoughts(),
      newHabit: picksCtl.newHabit
    };
    saveStored<Snapshot>(storageKey, s);
  }

  function restore(): boolean {
    const s = loadStored<Snapshot | null>(storageKey, null);
    if (!s) return false;
    scriptureCtl.restore({
      scriptureSource: s.scriptureSource,
      customScripture: s.customScripture,
      customSource: s.customSource
    });
    focusCtl.restore({
      winSentence: s.winSentence,
      goal: s.goal,
      linkedGoalId: s.linkedGoalId
    });
    picksCtl.restore({
      pickedTasks: s.pickedTasks,
      pickedHabits: s.pickedHabits,
      pickedIntentions: s.pickedIntentions,
      newHabit: s.newHabit
    });
    setThoughts(s.thoughts ?? '');
    return true;
  }

  function clear() {
    saveStored<Snapshot>(storageKey, undefined);
  }

  function hasSnapshot(): boolean {
    return typeof localStorage !== 'undefined' && !!localStorage.getItem(storageKey);
  }

  // Auto-persist on any tracked field change. Effects must run inside
  // a component context, so the caller wires this up — we expose the
  // tracked-read + persist() pair as a single closure so the caller
  // just does `$effect(install.autosave)`.
  // Touching every field marks it as a dependency.
  $effect(() => {
    void scriptureCtl.scripture;
    void scriptureCtl.customScripture;
    void scriptureCtl.customSource;
    void focusCtl.winSentence;
    void focusCtl.goal;
    void focusCtl.linkedGoalId;
    void picksCtl.pickedTasks;
    void picksCtl.pickedHabits;
    void picksCtl.pickedIntentions;
    void getThoughts();
    void picksCtl.newHabit;
    persist();
  });

  return { restore, clear, hasSnapshot };
}
