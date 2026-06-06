// Two loaders the AI overlay panel needs at open-time, bundled
// together because they share the same stale-response guard pattern
// (a monotonic gen counter so a slow earlier call doesn't overwrite a
// fresh later one).
//
//   • Status — provider · model · sabbath flag for the header pill.
//     A 401 / network failure leaves statusInfo as null and logs a
//     console.warn; nothing toasts since the header just hides itself
//     gracefully.
//
//   • Snapshot — the Context Engine's snapshot (events / tasks /
//     recent notes / goals / deadlines). Used as the "what's going
//     on" context for non-note routes. Same failure model as status:
//     leave snapshotData null, log a warn; the UI renders an
//     "unavailable" chip with a retry button.
//
// Both are pure-data fetches. The controllers stay outside any
// component lifecycle — the parent calls .load() from its open
// $effect.

import { api } from '$lib/api';
import { errorMessage } from '$lib/util/errorMessage';

export interface AIStatusInfo {
  provider: string;
  model: string;
  sabbath: boolean;
}

export interface AIStatusController {
  readonly statusInfo: AIStatusInfo | null;
  load(): Promise<void>;
}

export function createAIStatusLoader(): AIStatusController {
  let statusInfo = $state<AIStatusInfo | null>(null);
  let gen = 0;

  async function load() {
    const myGen = ++gen;
    try {
      const s = await api.getAIStatus();
      if (myGen !== gen) return;
      statusInfo = {
        provider: s.global_provider,
        model: s.global_model,
        sabbath: !!s.sabbath_active
      };
    } catch (e) {
      if (myGen !== gen) return;
      statusInfo = null;
      // eslint-disable-next-line no-console
      console.warn('[ai-overlay] status load failed:', errorMessage(e));
    }
  }

  return {
    get statusInfo() {
      return statusInfo;
    },
    load
  };
}

export interface AISnapshotController {
  readonly snapshotLoading: boolean;
  /** Opaque snapshot shape — backend evolves it independently, so we
   *  pass it through to the prelude builder verbatim. */
  readonly snapshotData: unknown;
  load(): Promise<void>;
}

export function createAISnapshotLoader(): AISnapshotController {
  let snapshotLoading = $state(false);
  let snapshotData = $state<unknown>(null);
  let gen = 0;

  async function load() {
    if (snapshotLoading) return;
    const myGen = ++gen;
    snapshotLoading = true;
    try {
      const r = await api.getAISnapshot();
      if (myGen !== gen) return;
      snapshotData = r.snapshot ?? null;
    } catch (e) {
      if (myGen !== gen) return;
      snapshotData = null;
      // eslint-disable-next-line no-console
      console.warn('[ai-overlay] snapshot load failed:', errorMessage(e));
    } finally {
      if (myGen === gen) snapshotLoading = false;
    }
  }

  return {
    get snapshotLoading() {
      return snapshotLoading;
    },
    get snapshotData() {
      return snapshotData;
    },
    load
  };
}
