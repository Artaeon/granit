// Tasks page lifecycle plumbing — the bits that have to run once on
// mount and tear down on unmount.
//
//   • Subscribe to the WS task.changed / note.changed / note.removed
//     stream and coalesce reloads (bulk operations fire dozens in a
//     row; a trailing-edge window collapses them into one fetch).
//   • Force a fresh load when the tab becomes visible / focused,
//     bypassing the coalesce — a tab in the background never got the
//     WS events so the user sees a stale list until the next fetch
//     window otherwise.
//
// Returns the unsub function the parent's onMount expects so the
// install line stays a one-liner:
//
//   onMount(() => installTasksLifecycle({ load }));
//
// Pure orchestration — no Svelte runes, no DOM beyond document /
// window event listeners. Lives as a .ts (not .svelte.ts) so other
// surfaces that want the same reload coherence can reuse it without
// dragging in $effect plumbing.

import { onWsEvent } from '$lib/ws';
import { createCoalescedReload } from '$lib/util/coalesce';

export interface TasksLifecycleOptions {
  /** What to call when fresh data is needed. The lifecycle wraps this
   *  in createCoalescedReload so bursts of WS events collapse into
   *  one fetch. */
  load: () => Promise<void> | void;
  /** Trailing-edge debounce window (ms) for the coalesced reload.
   *  600ms matches what the calendar + inbox widgets use — slow
   *  enough that a 50-item bulk triage doesn't fire 50 fetches, fast
   *  enough that single-event changes feel immediate. */
  windowMs?: number;
}

export function installTasksLifecycle(opts: TasksLifecycleOptions): () => void {
  const reload = createCoalescedReload(() => opts.load(), opts.windowMs ?? 600);

  const unsub = onWsEvent((ev) => {
    // task.changed fires after every patchTask (including drag-drops
    // from the kanban — without that, moves would only show up on a
    // manual refresh). Match the same event set the calendar / inbox
    // widgets honor so behaviour stays consistent across surfaces.
    if (ev.type === 'note.changed' || ev.type === 'note.removed' || ev.type === 'task.changed') {
      reload.trigger();
    }
  });

  // Visibility / focus: bypass the coalesce so the user gets fresh
  // data immediately on tab return. A backgrounded tab never got the
  // WS events; without this, a task ticked off on the phone while
  // desktop was hidden would stay open here until the next event
  // fired.
  const onVisible = () => {
    if (document.visibilityState === 'visible') reload.flush();
  };
  document.addEventListener('visibilitychange', onVisible);
  window.addEventListener('focus', onVisible);

  return () => {
    unsub();
    reload.cancel();
    document.removeEventListener('visibilitychange', onVisible);
    window.removeEventListener('focus', onVisible);
  };
}
