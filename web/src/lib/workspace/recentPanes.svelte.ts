// Recent-panes MRU. Tracks the last N paneKinds the user navigated
// into (via workspace pane-set or palette pick). Persists per-device
// to localStorage so the list survives reloads. Not synced across
// devices — a follow-up feature, intentionally out of scope here.
//
// IMPORTANT: this used to declare `let recent = $state(loadInitial())`
// at module top-level. Module-level $state in production bundles can
// hit a TDZ ReferenceError ("Cannot access 'G' before initialization")
// when concatenated module init runs before the Svelte 5 runtime
// finishes binding. Same failure mode as the $isMobile bug.
//
// The replacement is a plain TypeScript variable + getter. Consumers
// (workspaceCommands.ts) only read the list synchronously when the
// palette is opened, so no reactivity is actually needed — every
// palette open already runs after every push().
//
// The file stays a .svelte.ts so the import path `./recentPanes.svelte`
// resolves without churn at every call site.

import { loadStored, saveStored } from '$lib/util/storage';
import type { PaneKind } from './paneRegistry';

const STORE_KEY = 'granit.workspace.recentPanes';
const MAX_RECENT = 10;

function loadInitial(): PaneKind[] {
  const raw = loadStored<unknown>(STORE_KEY, []);
  if (!Array.isArray(raw)) return [];
  return raw.filter((x): x is string => typeof x === 'string') as PaneKind[];
}

let recent: PaneKind[] = loadInitial();

export const recentPanes = {
  /** MRU-ordered: index 0 is the most recently opened pane. */
  get list(): readonly PaneKind[] {
    return recent;
  },
  /** Promote `kind` to the head of the MRU list. De-dupes any
   *  prior occurrence so the same kind never appears twice. */
  push(kind: PaneKind): void {
    recent = [kind, ...recent.filter((k) => k !== kind)].slice(0, MAX_RECENT);
    saveStored(STORE_KEY, recent);
  },
  /** Wipe the MRU. Reserved for tests + a possible future
   *  "clear recents" command. */
  clear(): void {
    recent = [];
    saveStored(STORE_KEY, recent);
  }
};
