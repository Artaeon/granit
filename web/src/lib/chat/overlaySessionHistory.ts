// sessionStorage-backed in-flight history for the global AI overlay.
// Extracted out of AIOverlay.svelte because (a) the load/persist pair
// is pure plumbing — same shape as half a dozen other localStorage
// shims in the codebase — and (b) keeping the storage key + the cap
// rule in one named module means future surfaces that want to share
// the overlay's "this-tab transient draft" can do so without copying
// the magic string and silently desyncing.
//
// Long-term thread history (the LRU 30 saved threads visible in the
// history rail) lives in $lib/chat/history.ts. This module is only
// the per-tab pre-save buffer that keeps a chat alive across
// Esc → reopen / Mod+J toggles inside the same tab.

import type { ChatMessage } from '$lib/api';

// Keep this exported so consumers can reference the exact key when
// debugging in DevTools — but the loaders below are the only sane
// way to read/write because the cap rule + JSON shape live with them.
export const OVERLAY_HISTORY_KEY = 'granit.ai.overlay.messages';

// Max messages we round-trip to sessionStorage. The overlay is meant
// for quick questions; long-running threads live on the /chat page,
// and the LRU history rail covers "I want this back next week". A
// hard cap keeps the JSON blob small enough that the per-keystroke
// $effect-driven persist stays cheap.
const MAX_OVERLAY_MESSAGES = 30;

export function loadOverlayHistory(): ChatMessage[] {
  if (typeof sessionStorage === 'undefined') return [];
  try {
    const raw = sessionStorage.getItem(OVERLAY_HISTORY_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(
      (m): m is ChatMessage =>
        m && typeof m === 'object' && typeof m.role === 'string' && typeof m.content === 'string'
    );
  } catch {
    return [];
  }
}

export function persistOverlayHistory(list: ChatMessage[]): void {
  if (typeof sessionStorage === 'undefined') return;
  try {
    const trimmed = list.length > MAX_OVERLAY_MESSAGES ? list.slice(-MAX_OVERLAY_MESSAGES) : list;
    sessionStorage.setItem(OVERLAY_HISTORY_KEY, JSON.stringify(trimmed));
  } catch {
    // quota / privacy mode / storage disabled — silently drop. The
    // user just loses the cross-toggle persistence, not the running
    // conversation in memory.
  }
}
