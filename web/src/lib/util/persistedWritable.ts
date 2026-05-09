// Helper for the localStorage-backed Svelte writable pattern. Three
// stores (sidebarPins, sourceColors, habitTargets) re-implemented
// the same shape: read JSON on init → validate → writable → subscribe
// to write back. Centralising it gives every per-device preference
// store the same SSR-safe / quota-safe / parse-fail-safe behaviour.
//
// Why not just use writable + a manual subscribe at each call site:
//   - SSR: localStorage is undefined during prerender; the helper
//     short-circuits to the default in that case.
//   - Quota / privacy mode: writes throw silently; helper swallows.
//   - Parse failure: the stored JSON might be from an older app
//     version with a different shape; helper validates via the
//     caller's predicate and falls back to the default rather than
//     poisoning the store with garbage.
//
// The store-level subscriber leaks by design — these stores live
// for the app session, so the unsubscribe is never called. Same as
// the original hand-rolled versions.

import { writable, type Writable } from 'svelte/store';

export interface PersistedWritableOptions<T> {
  /** Validate the raw parsed value. Return the validated T or throw
   *  to fall back to defaultValue. Default: identity (assumes the
   *  parsed JSON has the right shape — fine for primitive arrays,
   *  risky for nested types). */
  validate?: (raw: unknown) => T;
}

export function persistedWritable<T>(
  key: string,
  defaultValue: T,
  options: PersistedWritableOptions<T> = {}
): Writable<T> {
  const initial = (() => {
    if (typeof localStorage === 'undefined') return defaultValue;
    try {
      const raw = localStorage.getItem(key);
      if (raw === null) return defaultValue;
      const parsed = JSON.parse(raw) as unknown;
      return options.validate ? options.validate(parsed) : (parsed as T);
    } catch {
      return defaultValue;
    }
  })();

  const store = writable<T>(initial);

  store.subscribe((v) => {
    if (typeof localStorage === 'undefined') return;
    try {
      localStorage.setItem(key, JSON.stringify(v));
    } catch {
      // quota / privacy mode — silently drop. The user's session
      // still has the in-memory value; only persistence degrades.
    }
  });

  return store;
}
