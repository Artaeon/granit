// Per-device pinned-notes set, backed by localStorage. The tree
// surfaces these at the top above the regular folder list so notes
// the user revisits often (today's daily, the running master plan,
// the morning ritual checklist) are one click away regardless of
// where they live in the folder hierarchy.
//
// Single source of truth across the notes tree + any future
// pin-aware surfaces (dashboard widget, command palette). Reactive
// via a writable store so any component subscribing to it sees
// pin/unpin updates without re-reading localStorage.

import { writable } from 'svelte/store';

const KEY = 'granit.notes.pinned';

function load(): Set<string> {
  if (typeof localStorage === 'undefined') return new Set();
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return new Set();
    const arr = JSON.parse(raw) as unknown;
    if (!Array.isArray(arr)) return new Set();
    return new Set(arr.filter((x): x is string => typeof x === 'string'));
  } catch {
    return new Set();
  }
}

function persist(set: Set<string>) {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(KEY, JSON.stringify([...set]));
  } catch {}
}

export const pinnedNotes = writable<Set<string>>(load());

export function isPinned(path: string): boolean {
  let pinned = false;
  pinnedNotes.subscribe((s) => { pinned = s.has(path); })();
  return pinned;
}

export function togglePin(path: string) {
  pinnedNotes.update((s) => {
    const next = new Set(s);
    if (next.has(path)) next.delete(path);
    else next.add(path);
    persist(next);
    return next;
  });
}

export function unpinPath(path: string) {
  pinnedNotes.update((s) => {
    if (!s.has(path)) return s;
    const next = new Set(s);
    next.delete(path);
    persist(next);
    return next;
  });
}
