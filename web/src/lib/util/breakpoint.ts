// Breakpoint store — single source of truth for "are we on a phone
// right now?". Three components were each rolling their own
// `window.matchMedia('(max-width: 767px)').matches` call which
// (a) duplicates the magic-number breakpoint string and (b) only
// reads the value at one point in time, leaving stale state when
// the user crosses the boundary (iPad rotation, browser-window
// resize on a touch laptop). This module:
//
//   - exposes `isMobile`, a Svelte readable that emits true/false
//     on every match-status change; components subscribe with `$isMobile`
//     and reactivity follows naturally;
//   - exposes `isMobileNow()` for non-reactive event-handler reads;
//   - keeps a SINGLE MediaQueryList behind the scenes so subscriber
//     count scales free of platform listener cost.
//
// SSR-safe: the readable seeds with `false` and the `start` function
// short-circuits when `window` is undefined; the first browser-side
// subscriber writes the real value.

import { readable } from 'svelte/store';

// 767px matches Tailwind's `md` lower bound. Coupled to that boundary
// on purpose — every responsive utility class in the codebase uses
// md:* / lg:* and consumers expect "mobile" to mean "below md".
const QUERY = '(max-width: 767px)';

export const isMobile = readable<boolean>(false, (set) => {
  if (typeof window === 'undefined') return;
  const mq = window.matchMedia(QUERY);
  set(mq.matches);
  const onChange = (e: MediaQueryListEvent) => set(e.matches);
  // addEventListener('change') is the modern path; the legacy
  // addListener fallback is irrelevant for our supported browser
  // matrix (Safari 14+, Chrome 90+, Firefox 78+).
  mq.addEventListener('change', onChange);
  return () => mq.removeEventListener('change', onChange);
});

/** Synchronous read of the mobile breakpoint. Use this inside event
 *  handlers where subscribing would be overkill (the handler only
 *  reads the value once per fire). Returns false during SSR. */
export function isMobileNow(): boolean {
  if (typeof window === 'undefined') return false;
  return window.matchMedia(QUERY).matches;
}
