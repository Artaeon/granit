// Per-note browsing history: visited-headings tracking + per-note
// scroll-position memory. Both extracted from notes/[...path]/+page
// because they're orthogonal to the page lifecycle — they're pure
// localStorage map helpers keyed by note path, with an LRU-style
// cap so the storage payload doesn't grow unbounded as the user
// roams a big vault.
//
// The two subsystems share the "Record<path, T> with cap" shape
// but differ in the value type (number[] vs number) and the
// per-entry retention rule (last-N visited headings vs latest
// scroll-top), so they live as a small twin-API rather than a
// single generic.

import { loadStored, saveStored } from '$lib/util/storage';

const VISITED_KEY = 'granit.note.visited';
const SCROLL_KEY = 'granit.note.scroll';

const MAX_VISITED_HEADINGS_PER_NOTE = 200;
const MAX_NOTES_TRACKED_VISITED = 100;
const VISITED_TRIM_TARGET = 80;
const MAX_NOTES_TRACKED_SCROLL = 200;
const SCROLL_TRIM_TARGET = 150;

// ─── Visited headings ─────────────────────────────────────────────

/** Read the full visited-line map; SSR-safe, returns {} on parse fail. */
export function loadVisitedMap(): Record<string, number[]> {
  return loadStored<Record<string, number[]>>(VISITED_KEY, {});
}

/** Persist the full visited-line map. SSR-safe, quota-safe. */
export function saveVisitedMap(m: Record<string, number[]>): void {
  saveStored(VISITED_KEY, m);
}

/**
 * Record `line` as visited for `path` and persist. Caps the per-
 * note list at 200 entries (oldest-first eviction, models "recently
 * read" reasonably for revisits) and the global note count at 100
 * (drop oldest 20 when exceeded).
 *
 * Returns the post-write set so the caller can refresh local state
 * with one round-trip rather than re-loading.
 */
export function recordVisitedLine(path: string, line: number): Set<number> {
  const m = loadVisitedMap();
  const arr = (m[path] ?? []).filter((x) => x !== line);
  arr.push(line);
  if (arr.length > MAX_VISITED_HEADINGS_PER_NOTE) {
    arr.splice(0, arr.length - MAX_VISITED_HEADINGS_PER_NOTE);
  }
  m[path] = arr;
  const keys = Object.keys(m);
  if (keys.length > MAX_NOTES_TRACKED_VISITED) {
    for (const k of keys.slice(0, keys.length - VISITED_TRIM_TARGET)) delete m[k];
  }
  saveVisitedMap(m);
  return new Set(arr);
}

/** Clear visited headings for one note (Outline reset button). */
export function clearVisitedFor(path: string): void {
  const m = loadVisitedMap();
  delete m[path];
  saveVisitedMap(m);
}

// ─── Scroll position ──────────────────────────────────────────────

/** Read the path → scroll-top map. */
export function loadScrollMap(): Record<string, number> {
  return loadStored<Record<string, number>>(SCROLL_KEY, {});
}

/**
 * Remember the last scroll position for a note. Skips zero so the
 * map doesn't fill up with notes the user opened and immediately
 * closed without scrolling. Caps the map at 200 paths (drop the
 * earliest-added 50 when exceeded — cheap heuristic, the user's
 * recently-viewed notes still land safely).
 */
export function rememberScroll(path: string, top: number): void {
  if (top <= 0) return;
  const m = loadScrollMap();
  m[path] = top;
  const keys = Object.keys(m);
  if (keys.length > MAX_NOTES_TRACKED_SCROLL) {
    for (const k of keys.slice(0, keys.length - SCROLL_TRIM_TARGET)) delete m[k];
  }
  saveStored(SCROLL_KEY, m);
}

/** Recall the saved scroll-top for a note (0 = top / never seen). */
export function recallScroll(path: string): number {
  return loadScrollMap()[path] ?? 0;
}
