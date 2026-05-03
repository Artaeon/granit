// Modules store — caches the server's /api/v1/modules response and
// re-fetches on the modules.changed WebSocket event. Pages call
// isEnabled(id) to gate nav, route guards, and dashboard widgets.
//
// Default-on semantics: when the store hasn't loaded yet, or when an
// unknown ID is queried, isEnabled returns true. This matches the
// server-side modules.Registry.Enabled fallback so a transient API
// failure can't accidentally hide every module.

import { writable, get, type Readable } from 'svelte/store';
import { api, type ModulesResponse, type CoreModuleEntry, type ModuleEntry } from '$lib/api';
import { onWsEvent } from '$lib/ws';

interface State {
  loaded: boolean;
  modules: ModuleEntry[];
  coreIds: CoreModuleEntry[];
  // Built lookup map from id -> enabled. Built once per refresh so
  // isEnabled stays O(1).
  enabledById: Record<string, boolean>;
}

const initial: State = { loaded: false, modules: [], coreIds: [], enabledById: {} };
const { subscribe, set } = writable<State>(initial);

let loadingPromise: Promise<void> | null = null;
let wsBound = false;

function buildEnabledMap(resp: ModulesResponse): Record<string, boolean> {
  const out: Record<string, boolean> = {};
  for (const m of resp.modules) out[m.id] = m.enabled;
  // Core IDs are always-on by definition. Surface them in the map so
  // isEnabled('notes') returns true even though notes isn't a
  // toggleable module.
  for (const c of resp.coreIds) out[c.id] = true;
  return out;
}

async function refresh(): Promise<void> {
  try {
    const resp = await api.listModules();
    set({
      loaded: true,
      modules: resp.modules,
      coreIds: resp.coreIds,
      enabledById: buildEnabledMap(resp)
    });
  } catch {
    // Silent fail. Default-on isEnabled keeps the UI usable until the
    // next refresh succeeds.
    set({ ...initial, loaded: true });
  }
}

function ensureLoaded(): Promise<void> {
  if (loadingPromise) return loadingPromise;
  loadingPromise = refresh();
  if (!wsBound && typeof window !== 'undefined') {
    wsBound = true;
    onWsEvent((ev) => {
      if (ev.type === 'modules.changed') refresh();
    });
  }
  return loadingPromise;
}

export const modulesStore: Readable<State> & {
  refresh(): Promise<void>;
  isEnabled(id: string): boolean;
  ensureLoaded(): Promise<void>;
  set(patch: Record<string, boolean>): Promise<void>;
} = {
  subscribe,
  refresh,
  ensureLoaded,
  // isEnabled is the hot path consulted by the layout nav filter +
  // route guards on every navigation. Defaults to true when the store
  // hasn't loaded yet OR when the id is unknown — same migration-safe
  // fallback the server uses, so a fresh client doesn't render an
  // empty sidebar while the first fetch completes.
  isEnabled(id: string): boolean {
    const st = get({ subscribe });
    if (!st.loaded) return true;
    const v = st.enabledById[id];
    return v === undefined ? true : v;
  },
  // set issues a PUT and writes the echoed response into the store.
  // No optimistic update: the round-trip is fast and the user does
  // this rarely (settings page only).
  async set(patch: Record<string, boolean>): Promise<void> {
    const resp = await api.setModules(patch);
    set({
      loaded: true,
      modules: resp.modules,
      coreIds: resp.coreIds,
      enabledById: buildEnabledMap(resp)
    });
  }
};

// Best-effort eager load when a page imports the store. The call is
// idempotent — refresh() chains onto loadingPromise.
if (typeof window !== 'undefined') {
  void ensureLoaded();
}
