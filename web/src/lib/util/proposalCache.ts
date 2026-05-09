// AI proposal caching. Several surfaces (tasks/triage, tasks/deadline-
// detect, settings/inbox-rewrite) generate batches of suggestions
// that the user reviews one-by-one. The user paid tokens for those —
// a refresh / SW update / accidental tab close shouldn't burn the
// work. We persist {at, items} under a feature key so the UI can
// rehydrate on next paint, but expire after 24 h since the underlying
// tasks may have moved on.
//
// Returns the items only; the wrapping {at, items} envelope is
// internal. Saving an empty array removes the key (cleaner than
// leaving a {items:[]} entry around).

import { loadStored, saveStored } from './storage';

const PROPOSAL_TTL_MS = 24 * 60 * 60 * 1000;

interface CachedProposals<T> {
  at: number;
  items: T[];
}

/**
 * Persist a list of AI proposals under `key` with a fresh timestamp.
 * Empty list deletes the key.
 */
export function saveProposals<T>(key: string, items: T[]): void {
  if (items.length === 0) {
    saveStored<CachedProposals<T>>(key, undefined);
    return;
  }
  saveStored<CachedProposals<T>>(key, { at: Date.now(), items });
}

/**
 * Read cached proposals; returns [] if missing, malformed, or
 * older than 24 h. Stale entries are removed during read so the
 * key doesn't accumulate dead weight.
 */
export function loadProposals<T>(key: string): T[] {
  const cached = loadStored<CachedProposals<T> | null>(key, null);
  if (!cached || !cached.at || !Array.isArray(cached.items)) return [];
  if (Date.now() - cached.at > PROPOSAL_TTL_MS) {
    saveStored<CachedProposals<T>>(key, undefined);
    return [];
  }
  return cached.items;
}
