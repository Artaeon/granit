import { writable } from 'svelte/store';
import { getToken, setToken as save, clearToken as clear } from '$lib/api';

function createAuthStore() {
  const initial = typeof localStorage !== 'undefined' ? getToken() : null;
  const { subscribe, set } = writable<string | null>(initial);

  return {
    subscribe,
    setToken: (tok: string) => {
      save(tok);
      set(tok);
    },
    clear: () => {
      clear();
      set(null);
    }
  };
}

export const auth = createAuthStore();
