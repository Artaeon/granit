// Pinned-notes store — backed by the server's
// .granit/sidebar-pinned.json (api.listPinned / api.setPinned) so the
// notes tree, the existing dashboard PinnedWidget, the granit TUI,
// and any future pin-aware surface share a single source of truth.
//
// Initially this lived in localStorage; we discovered the server-side
// system already existed and unified rather than diverge. Sync model:
//  - Initial load fetches the server list.
//  - togglePin writes to the server, optimistic-updates the store.
//  - WS state.changed events for the pinned file refresh the store.
//
// Calling code uses the store like any writable Set<string> — the
// network I/O is hidden inside togglePin / unpinPath.

import { writable, get } from 'svelte/store';
import { api } from '$lib/api';
import { onWsEvent } from '$lib/ws';

export const pinnedNotes = writable<Set<string>>(new Set());
let initialized = false;
let wsUnsub: (() => void) | null = null;

async function reload() {
  try {
    const r = await api.listPinned();
    pinnedNotes.set(new Set(r.pinned.map((p) => p.path)));
  } catch {
    // Best-effort — a 401 / network glitch leaves the store with
    // its current value rather than wiping pins to empty.
  }
}

/** Lazy-init: first caller (typically the notes tree or the
 *  dashboard widget) triggers a fetch + subscribes to WS events for
 *  cross-tab / cross-device sync. Subsequent callers re-use the
 *  same store with no additional load. */
export function ensurePinnedLoaded() {
  if (initialized) return;
  initialized = true;
  void reload();
  wsUnsub = onWsEvent((ev) => {
    if (ev.type === 'state.changed' && ev.path === '.granit/sidebar-pinned.json') {
      void reload();
    }
  });
}

export function isPinned(path: string): boolean {
  return get(pinnedNotes).has(path);
}

export async function togglePin(path: string) {
  const current = get(pinnedNotes);
  const willPin = !current.has(path);
  // Optimistic update so the UI flips instantly; the server response
  // is authoritative and we replace on success.
  const next = new Set(current);
  if (willPin) next.add(path);
  else next.delete(path);
  pinnedNotes.set(next);
  try {
    const r = await api.setPinned(path, willPin);
    pinnedNotes.set(new Set(r.pinned.map((p) => p.path)));
  } catch {
    // Roll back on failure so the user sees the actual state.
    pinnedNotes.set(current);
  }
}

export async function unpinPath(path: string) {
  if (!get(pinnedNotes).has(path)) return;
  await togglePin(path);
}

// Test-only / future-cleanup helper: unsubscribe + reset. Not
// exported via the public surface in normal use.
export function _teardownForTests() {
  wsUnsub?.();
  wsUnsub = null;
  initialized = false;
  pinnedNotes.set(new Set());
}
