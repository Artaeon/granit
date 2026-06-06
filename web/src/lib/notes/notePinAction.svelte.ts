// Pin / unpin a note from the editor's header toolbar.
//
// Reads the pinned set straight off the shared pinnedNotes store
// (same source of truth the notes tree, dashboard widget, and TUI
// subscribe to). ensurePinnedLoaded() runs in the page's onMount so
// the store is hot by the time the toolbar renders; the controller
// owns only the busy flag + the toggle wrapper.

import { pinnedNotes, togglePin as togglePinPath } from '$lib/notes/pinnedNotes';
import { get } from 'svelte/store';
import type { Note } from '$lib/api';

export interface NotePinAction {
  readonly pinned: string[];
  readonly pinBusy: boolean;
  togglePin: () => Promise<void>;
}

export interface NotePinActionOpts {
  getNote: () => Note | null;
}

export function createNotePinAction(opts: NotePinActionOpts): NotePinAction {
  let pinBusy = $state(false);
  let pinned = $state<string[]>(get(pinnedNotes));

  // Subscribe to the shared store via a $effect so the controller
  // stays a reactive citizen — the page no longer needs `$pinnedNotes`
  // in its own surface and the auto-store-subscription it requires.
  $effect(() => {
    const unsub = pinnedNotes.subscribe((v) => { pinned = v; });
    return unsub;
  });

  async function togglePin(): Promise<void> {
    const note = opts.getNote();
    if (!note) return;
    pinBusy = true;
    try {
      await togglePinPath(note.path);
    } finally {
      pinBusy = false;
    }
  }

  return {
    get pinned() { return pinned; },
    get pinBusy() { return pinBusy; },
    togglePin
  };
}
