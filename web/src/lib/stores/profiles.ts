// Profiles store — caches the server's /api/v1/profiles response and
// re-fetches on the profile.changed WebSocket event. Mirrors the
// modules store pattern (same readable + ensureLoaded + activate
// interface) so consumers don't need to learn two shapes.
//
// One thing this store does NOT do: load eagerly on import. The nav
// only mounts the switcher when this store reports more than one
// profile, so the first read is from inside that conditional —
// `ensureLoaded()` covers it.

import { writable, get, type Readable } from 'svelte/store';
import { api, type ProfileEntry, type ProfilesResponse } from '$lib/api';
import { onWsEvent } from '$lib/ws';

interface State {
  loaded: boolean;
  profiles: ProfileEntry[];
  activeId: string;
}

const initial: State = { loaded: false, profiles: [], activeId: '' };
const { subscribe, set } = writable<State>(initial);

let loadingPromise: Promise<void> | null = null;
let wsBound = false;

async function refresh(): Promise<void> {
  try {
    const resp = await api.listProfiles();
    set({ loaded: true, profiles: resp.profiles, activeId: resp.activeId });
  } catch {
    // Silent fail — the switcher hides itself when profiles.length < 2,
    // and the settings page surfaces a "couldn't load" message via the
    // page-level try/catch around its own refresh call.
    set({ ...initial, loaded: true });
  }
}

function ensureLoaded(): Promise<void> {
  if (loadingPromise) return loadingPromise;
  loadingPromise = refresh();
  if (!wsBound && typeof window !== 'undefined') {
    wsBound = true;
    onWsEvent((ev) => {
      if (ev.type === 'profile.changed') refresh();
    });
  }
  return loadingPromise;
}

export const profilesStore: Readable<State> & {
  refresh(): Promise<void>;
  ensureLoaded(): Promise<void>;
  activate(id: string): Promise<void>;
  active(): ProfileEntry | null;
} = {
  subscribe,
  refresh,
  ensureLoaded,
  // activate issues the POST and writes the echoed response into the
  // store. No optimistic update — the switcher is rarely tapped and a
  // failed activation should keep the previous active visible.
  async activate(id: string): Promise<void> {
    const resp: ProfilesResponse = await api.activateProfile(id);
    set({ loaded: true, profiles: resp.profiles, activeId: resp.activeId });
  },
  active(): ProfileEntry | null {
    const st = get({ subscribe });
    if (!st.loaded || !st.activeId) return null;
    return st.profiles.find((p) => p.id === st.activeId) ?? null;
  }
};
