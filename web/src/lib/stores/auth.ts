import { writable } from 'svelte/store';
import { getToken, setToken as save, clearToken as clear } from '$lib/api';
import { clearAllDrafts } from '$lib/notes/drafts';

function createAuthStore() {
  const initial = typeof localStorage !== 'undefined' ? getToken() : null;
  const { subscribe, set } = writable<string | null>(initial);

  return {
    subscribe,
    setToken: (tok: string) => {
      save(tok);
      set(tok);
    },
    // Clearing auth wipes the token AND any cached draft note bodies.
    // A logout on a shared device shouldn't leak the previous user's
    // unsaved work to whoever logs in next. The drafts module is
    // append-only otherwise, so this is the only sensible eviction
    // hook.
    clear: () => {
      clear();
      clearAllDrafts();
      set(null);
    }
  };
}

export const auth = createAuthStore();
