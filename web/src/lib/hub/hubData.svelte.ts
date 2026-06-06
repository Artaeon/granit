// Data state for the /hub launcher.
//
// First extraction step out of routes/hub/+page.svelte. Owns the
// items array, the loading flag, the load() function with a stale-
// response guard, and the install helper that wires the WS event
// subscription for live refresh when .granit/hub.json changes
// underneath us (CLI add, another tab edit, file-watcher reload).
//
// The hub stores everything in a single sidecar so one fetch covers
// links + tools + creds — there's no per-card detail call, the list
// payload IS the detail. A monotonic gen counter still guards against
// late responses winning over a fresher load() because reorder /
// patch / delete callers fire load() back-to-back and a slow first
// response could otherwise stomp on a newer one.
//
// Auth gate stays at the controller level — load() bails when the
// user isn't signed in yet so the first onMount call before the auth
// store hydrates is a safe no-op.
//
// Sticking to the controller-factory + install-helper split keeps the
// page free of the data-plumbing noise while leaving load() reachable
// from every caller that needs it (save, remove, toggleFavorite,
// reorder drop, import-dialog onImported).

import { get } from 'svelte/store';
import { onWsEvent } from '$lib/ws';
import { api, type HubItem } from '$lib/api';
import { auth } from '$lib/stores/auth';
import { toast } from '$lib/components/toast';

export interface HubDataController {
  /** Loaded hub items sidecar. Setter exposed so callers (drag
   *  reorder) can apply an optimistic update before the load()
   *  round-trip lands. */
  items: HubItem[];
  /** True while load() is in flight. Drives the "loading…" placeholder
   *  in the empty-state slot. */
  readonly loading: boolean;
  /** Fetch the hub sidecar. Safe to call concurrently — a stale-
   *  response guard drops out-of-order results. */
  load(): Promise<void>;
}

export function createHubData(): HubDataController {
  let items = $state<HubItem[]>([]);
  let loading = $state(false);
  // Monotonic gen counter — every load() bumps it; after each await
  // the closure compares its captured generation against the current
  // one and bails when a newer load() has already started. Without
  // this, a slow first load() can overwrite a fresh second one with
  // older data.
  let gen = 0;

  async function load() {
    // Auth gate — first onMount fire can land before the auth store
    // hydrates from cookies; without this guard the request would 401
    // and the toast would flash before the user even sees the page.
    if (!get(auth)) return;
    const my = ++gen;
    loading = true;
    try {
      const r = await api.listHubItems();
      if (my !== gen) return;
      items = r.items;
    } catch (e) {
      if (my !== gen) return;
      toast.error('failed to load hub: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      if (my === gen) loading = false;
    }
  }

  return {
    get items() {
      return items;
    },
    set items(v) {
      items = v;
    },
    get loading() {
      return loading;
    },
    load
  };
}

export interface HubDataLiveDeps {
  /** Triggered on every relevant WS event. The page wraps load() in
   *  the closure so this helper stays free of API knowledge. */
  reload: () => void;
}

/**
 * Install the live-refresh listener — WS state.changed on the hub
 * sidecar path. Returns the unsubscribe handle so the caller can wire
 * it directly into onMount's teardown.
 */
export function installHubDataLive(deps: HubDataLiveDeps): () => void {
  return onWsEvent((ev) => {
    if (ev.type === 'state.changed' && ev.path === '.granit/hub.json') deps.reload();
  });
}
