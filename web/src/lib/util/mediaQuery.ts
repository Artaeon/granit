// Reactive media-query helper. Several files manually wired up
// matchMedia listeners with the same shape:
//   - read mql.matches synchronously for an initial value
//   - addEventListener('change') (preferred)
//   - addListener fallback for older browsers (deprecated but real)
//   - removeEventListener / removeListener on cleanup
//
// Centralising as a Svelte readable store: callers do
//   const isLg = mediaQuery('(min-width: 1024px)');
// and read `$isLg` in templates. The store auto-subscribes on first
// reader and auto-cleans-up when the last subscriber leaves, so no
// per-call onDestroy is needed.
//
// SSR-safe: returns a readable that yields false during prerender
// (no window) — matches the conservative "treat as not-matching"
// default the manual code used.

import { readable, type Readable } from 'svelte/store';

// Pre-2018 Safari + IE shape — `addListener`/`removeListener` are
// deprecated but still in some browsers (Linux WebKit forks, older
// Android). We can't extend `MediaQueryList` because the lib.dom
// types declare these as required (different signature than ours);
// a structural fallback type lets us probe at runtime without
// fighting the built-in declaration.
type LegacyMql = {
  addListener?: (fn: (e: MediaQueryListEvent) => void) => void;
  removeListener?: (fn: (e: MediaQueryListEvent) => void) => void;
};

export function mediaQuery(query: string): Readable<boolean> {
  return readable<boolean>(false, (set) => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return () => {};
    }
    const mql = window.matchMedia(query);
    const legacy = mql as unknown as LegacyMql;
    set(mql.matches);
    const handler = (e: MediaQueryListEvent) => set(e.matches);
    if (typeof mql.addEventListener === 'function') {
      mql.addEventListener('change', handler);
    } else {
      legacy.addListener?.(handler);
    }
    return () => {
      if (typeof mql.removeEventListener === 'function') {
        mql.removeEventListener('change', handler);
      } else {
        legacy.removeListener?.(handler);
      }
    };
  });
}
