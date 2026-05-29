// Shared title resolver for the multi-tab Phase 2 strip + the
// layout's navigation-interception path. Centralised so the pill
// label and the active-tab title stay derived from the same nav
// config (no drift between "what the strip shows" and "what the
// layout writes into the store").
//
// Longest-href wins so /notes/graph resolves to "Graph", not the
// parent "Notes" entry. Mirrors the logic in active.ts but takes
// a raw url string instead of subscribing to the page store —
// pure function, cheap to call from anywhere.

import { nav } from './config';

export function titleForUrl(url: string): string {
  const pathname = url.split('?')[0].split('#')[0] || '/';
  let best: { href: string; label: string } | undefined;
  for (const n of nav) {
    if (n.href === pathname || (n.href !== '/' && pathname.startsWith(n.href + '/'))) {
      if (!best || n.href.length > best.href.length) best = n;
    }
  }
  if (best) return best.label;
  // Fallback: last decoded segment of the path. /notes/Daily/2026-05-29.md
  // becomes "2026-05-29.md" which is meaningful for note tabs.
  const segs = pathname.split('/').filter(Boolean);
  if (segs.length === 0) return 'Today';
  try {
    return decodeURIComponent(segs[segs.length - 1]);
  } catch {
    return segs[segs.length - 1];
  }
}
