// Save-status presentation controller for the notes editor.
//
// Combines four tightly-coupled surfaces into one controller so the
// page doesn't carry the four separate plumbings:
//
//   1. nowTick           — single 1s setInterval that drives every
//                          relative-time label on the surface. Two
//                          older surfaces each had their own (1s +
//                          5s) plus a manual `void nowTick` keep-
//                          alive; both now reactive purely through
//                          $derived deps on this tick.
//   2. saveStatus        — derived string ("Saving…", "Saved 4s ago",
//                          "Edited", "Save failed") for the header.
//   3. lastSavedDisplay  — relative label ("Saved 4s ago") for the
//                          status bar.
//   4. saveFlash         — brief 1.2s outline pulse after every
//                          successful autosave. Without this, saves
//                          are invisible; the status text updates
//                          silently and the user has no positive
//                          confirmation that their work made it to
//                          disk. Doesn't fire on explicit Mod-S
//                          saves (those already get a toast.success).
//
// Same reactive contract the inline $state had — the page reads each
// metric via a $derived alias and binds NoteHeader's `saveFlash` to
// `statusCtl.saveFlash`. `install()` owns the setInterval + the flash
// timer cleanup; the page wraps it in $effect for lifecycle binding.

import { saveStatus as saveStatusFn, lastSavedDisplay as lastSavedDisplayFn } from '$lib/notes/noteSaveStatus';

export interface NoteSaveStatusOpts {
  getSaving: () => boolean;
  getDirty: () => boolean;
  getSaveFailed: () => boolean;
  getLastSavedAt: () => number | null;
}

export interface NoteSaveStatusController {
  readonly nowTick: number;
  readonly saveStatus: string;
  readonly lastSavedDisplay: string;
  readonly saveFlash: boolean;
  /** Install the 1s tick + the saveFlash watcher. Returns the
   *  cleanup the caller must return from its $effect. */
  install: () => () => void;
}

export function createNoteSaveStatusCtl(
  opts: NoteSaveStatusOpts
): NoteSaveStatusController {
  let nowTick = $state(Date.now());
  let saveFlash = $state(false);

  const saveStatus = $derived(
    saveStatusFn({
      saving: opts.getSaving(),
      saveFailed: opts.getSaveFailed(),
      dirty: opts.getDirty(),
      lastSavedAt: opts.getLastSavedAt(),
      nowTick
    })
  );
  const lastSavedDisplay = $derived(
    lastSavedDisplayFn(opts.getLastSavedAt(), nowTick)
  );

  // Watch lastSavedAt — every successful save bumps it, which is our
  // signal to flash the header button. We don't gate on the saving
  // flag because the flash is meant to surface the transition into
  // "saved" specifically. This $effect binds to the calling
  // component's lifecycle (factory called from component-init scope).
  let flashTimer: ReturnType<typeof setTimeout> | null = null;
  $effect(() => {
    const last = opts.getLastSavedAt();
    if (!last) return;
    saveFlash = true;
    if (flashTimer) clearTimeout(flashTimer);
    flashTimer = setTimeout(() => {
      saveFlash = false;
      flashTimer = null;
    }, 1200);
    return () => {
      if (flashTimer) { clearTimeout(flashTimer); flashTimer = null; }
    };
  });

  function install(): () => void {
    const tick = setInterval(() => (nowTick = Date.now()), 1000);
    return () => clearInterval(tick);
  }

  return {
    get nowTick() { return nowTick; },
    get saveStatus() { return saveStatus; },
    get lastSavedDisplay() { return lastSavedDisplay; },
    get saveFlash() { return saveFlash; },
    install
  };
}
