// Coalesce-reload helper. Three places (NotesTree, /notes index,
// RecentNotesWidget dashboard tile) re-implemented the same trailing-
// edge throttle for WS-driven reloads, with subtle differences:
// NotesTree + notes index used the correct "first-event-fires-trailing-
// edge-with-pending" pattern; RecentNotesWidget used a naive debounce
// that would never fire if events kept arriving.
//
// This module gives them a single canonical implementation. Pattern:
// the first call to `trigger()` arms a timer; subsequent calls within
// the window mark `pending` so the timer's expiration definitely fires
// the work. A pure debounce would defer indefinitely under sustained
// traffic, which is the wrong shape when the user expects "eventually
// fresh" lists during long editor sessions.
//
// Usage:
//   const reload = createCoalescedReload(load, 600);
//   onMount(() => onWsEvent((ev) => { if (...) reload.trigger(); }));
//   onDestroy(reload.cancel);
//
// The work function returns void or Promise<void>; we don't await it
// here — the next trigger after work resolves arms a fresh window
// without piling up a queue.

export interface CoalescedReload {
  /** Arm the timer if not already armed; mark pending if already
   *  armed so the trailing call will run with the latest data. */
  trigger(): void;
  /** Clear pending state + cancel the timer. Call from onDestroy
   *  so a navigation doesn't leave a setTimeout holding a dead
   *  closure. */
  cancel(): void;
  /** Run the work right now, bypassing the coalesce window. Useful
   *  for visibility-change handlers where we want freshness on tab
   *  return without waiting for the timer. */
  flush(): void;
}

export function createCoalescedReload(
  work: () => unknown | Promise<unknown>,
  windowMs = 600
): CoalescedReload {
  let timer: ReturnType<typeof setTimeout> | null = null;
  let pending = false;

  function trigger(): void {
    pending = true;
    if (timer !== null) return;
    timer = setTimeout(() => {
      timer = null;
      if (!pending) return;
      pending = false;
      void work();
    }, windowMs);
  }

  function cancel(): void {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
    pending = false;
  }

  function flush(): void {
    cancel();
    void work();
  }

  return { trigger, cancel, flush };
}
