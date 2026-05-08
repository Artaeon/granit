// Global open/close state for the AIOverlay component. Lives here
// instead of inside the component so any UI surface (sidebar
// button, command-palette entry, mobile bottom-nav) can flip the
// overlay without prop-drilling or refs.
//
// AIOverlay subscribes to this store to drive its `open` flag, and
// also writes to it on Mod+J / Esc / close-button so the rest of
// the app stays in sync with what the user sees.
//
// `aiOverlaySeed` is a write-once handoff for "open the overlay
// pre-filled with this prompt, and possibly switch to this mode" —
// quick-action chips in the sidebar push to it, the overlay
// consumes-and-clears it on each open via takeAIOverlaySeed().

import { writable } from 'svelte/store';

export const aiOverlayOpen = writable(false);

export interface AIOverlaySeed {
  /** Mode id to switch to before sending. Optional. */
  modeId?: string;
  /** Pre-fill the composer with this text. */
  text: string;
  /** When true, fire the message as soon as the overlay opens. When
   *  false, just pre-fill and let the user edit before submitting. */
  send?: boolean;
}

export const aiOverlaySeed = writable<AIOverlaySeed | null>(null);

export function openAIOverlay(seed?: AIOverlaySeed): void {
  if (seed) aiOverlaySeed.set(seed);
  aiOverlayOpen.set(true);
}

export function closeAIOverlay(): void {
  aiOverlayOpen.set(false);
}

export function toggleAIOverlay(): void {
  aiOverlayOpen.update((v) => !v);
}

/** Consume + clear the pending seed. Returns null if none was
 *  pending. The overlay calls this from its open-effect so the
 *  seed doesn't fire twice on a subsequent open. */
export function takeAIOverlaySeed(): AIOverlaySeed | null {
  let cur: AIOverlaySeed | null = null;
  aiOverlaySeed.update((s) => {
    cur = s;
    return null;
  });
  return cur;
}
