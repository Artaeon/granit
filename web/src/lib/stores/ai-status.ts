// Shared AI status store. Fetches /api/v1/ai/status once at first
// subscription and caches the result; any UI surface (sidebar,
// AIOverlay header, settings page) can read the same value without
// re-fetching. Refetches on `state.changed` events targeting the
// AI config sidecar so a user flipping providers in /settings sees
// the new model surface instantly in the sidebar.
//
// Pattern: a writable store with a get() helper that lazily fires
// the fetch. Consumers just `$aiStatus` and read fields when present.
// First read returns null while the fetch is in flight — the sidebar
// pill renders a 3-character placeholder skeleton instead of jumping
// in layout once the value arrives.

import { writable, type Readable } from 'svelte/store';
import { api, type AIStatus } from '$lib/api';
import { onWsEvent } from '$lib/ws';
import { auth } from '$lib/stores/auth';
import { get } from 'svelte/store';

const internal = writable<AIStatus | null>(null);
let fetching = false;
let bootstrapped = false;

async function refresh(): Promise<void> {
  if (fetching) return;
  fetching = true;
  try {
    const s = await api.getAIStatus();
    internal.set(s);
  } catch {
    // Silent on failure — the sidebar just hides the pill.
    // Settings page shows a richer error if needed.
  } finally {
    fetching = false;
  }
}

// Lazy bootstrap: the first read fires the fetch. Subsequent reads
// reuse the cache. WS hookup wires the live refetch on AI config
// changes.
function bootstrap(): void {
  if (bootstrapped) return;
  if (typeof window === 'undefined') return;
  bootstrapped = true;
  // Skip the fetch until auth is hydrated — otherwise the call 401s
  // and we waste a request on every cold start.
  const authVal = get(auth);
  if (authVal) {
    void refresh();
  } else {
    // Wait for auth, then fire once.
    const unsubAuth = auth.subscribe((tok) => {
      if (tok) {
        void refresh();
        unsubAuth();
      }
    });
  }
  // AI config sidecar lives at .granit/ai-prefs.json. When the
  // settings page writes it, the server broadcasts state.changed
  // and we refetch so the sidebar pill reflects the new provider
  // immediately.
  onWsEvent((ev) => {
    if (ev.type === 'state.changed' && ev.path?.startsWith('.granit/ai-')) {
      void refresh();
    }
  });
}

export const aiStatus: Readable<AIStatus | null> = {
  subscribe: (run, invalidate) => {
    bootstrap();
    return internal.subscribe(run, invalidate);
  }
};

/** Manual refresh trigger — for the settings page's "test connection"
 *  button or after a manual provider switch that bypasses the WS path. */
export function refreshAIStatus(): Promise<void> {
  return refresh();
}
