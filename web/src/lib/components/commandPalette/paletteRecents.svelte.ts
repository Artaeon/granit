// Recents ranking for the command palette.
//
// Pulled out of CommandPalette.svelte so the persistence + decay logic
// lives next to the types it operates on. The palette's $derived items
// builder reads recencyBoost() for every CmdItem it constructs;
// invoke() calls bump() before navigating so the same-tab redirect
// can't lose the write.
//
// Storage shape: a string[] in localStorage under RECENT_KEY, ordered
// most-recent-first, capped at RECENT_CAP. The cap is aggressive on
// purpose — a recent that hasn't been used in months doesn't deserve
// to crowd out a freshly-touched destination.
//
// recencyBoost(): linear decay across the cap. Most-recent gets the
// full RECENT_BUMP; the oldest still in the list gets ~10% of it.
// Additive (not a floor): an exact-match on a fresh item still beats a
// stale recent, which keeps the "I typed three letters and got the
// right thing" muscle memory intact even when recents is large.

import { loadStored, saveStored } from '$lib/util/storage';

const RECENT_KEY = 'granit.quickswitcher.recent';
const RECENT_CAP = 24;
const RECENT_BUMP = 250;

export interface PaletteRecentsController {
  /** Read-only view of the recents list. Position 0 = most recent. */
  readonly ids: string[];
  /** Move `id` to the front of the recents list (or insert it).
   *  Caps + persists. */
  bump(id: string): void;
  /** Additive ranking bonus for a given id. 0 if not in the list. */
  recencyBoost(id: string): number;
  /** Membership test. Used by the items builder to decide whether an
   *  open task should show on an empty query. */
  includes(id: string): boolean;
}

export function createPaletteRecents(): PaletteRecentsController {
  let recents = $state<string[]>(loadStored<string[]>(RECENT_KEY, []));

  function bump(id: string) {
    const next = [id, ...recents.filter((r) => r !== id)].slice(0, RECENT_CAP);
    recents = next;
    saveStored(RECENT_KEY, next);
  }

  function recencyBoost(id: string): number {
    const idx = recents.indexOf(id);
    if (idx < 0) return 0;
    // Linear decay over the recents cap so the most-recent gets the
    // full bump and the oldest in the list gets ~10% of it.
    return RECENT_BUMP * (1 - (idx / RECENT_CAP) * 0.9);
  }

  function includes(id: string): boolean {
    return recents.includes(id);
  }

  return {
    get ids() {
      return recents;
    },
    bump,
    recencyBoost,
    includes
  };
}
