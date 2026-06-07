// Recent-panes MRU. Tracks the last N paneKinds the user navigated
// into (via workspace pane-set, route navigation, or palette pick).
// Persists per-device to localStorage so the list survives reloads.
// Not synced across devices — a follow-up feature, intentionally out
// of scope here.
//
// Setter-side persistence: we write on each mutation rather than
// wiring a $effect, because $effect requires a component / root
// setup context that's not guaranteed when this module is first
// imported by a non-Svelte caller. Mirrors the safe-pattern used in
// notePinAction.svelte.ts — plain object with getters reading a
// $state variable, no auto-store-subscription tricks.

import { loadStored, saveStored } from '$lib/util/storage';
import type { PaneKind } from './paneRegistry';

const STORE_KEY = 'granit.workspace.recentPanes';
const MAX_RECENT = 10;

function loadInitial(): PaneKind[] {
  const raw = loadStored<unknown>(STORE_KEY, []);
  if (!Array.isArray(raw)) return [];
  return raw.filter((x): x is string => typeof x === 'string') as PaneKind[];
}

let recent = $state<PaneKind[]>(loadInitial());

export const recentPanes = {
  /** MRU-ordered: index 0 is the most recently opened pane. */
  get list(): readonly PaneKind[] {
    return recent;
  },
  /** Promote `kind` to the head of the MRU list. De-dupes any
   *  prior occurrence so the same kind never appears twice. */
  push(kind: PaneKind): void {
    recent = [kind, ...recent.filter((k) => k !== kind)].slice(0, MAX_RECENT);
    saveStored(STORE_KEY, recent);
  },
  /** Wipe the MRU. Used by tests + a possible future "clear recents"
   *  command. */
  clear(): void {
    recent = [];
    saveStored(STORE_KEY, recent);
  }
};
