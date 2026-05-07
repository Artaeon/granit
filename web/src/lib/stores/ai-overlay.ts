// Global open/close state for the AIOverlay component. Lives here
// instead of inside the component so any UI surface (sidebar
// button, command-palette entry, mobile bottom-nav) can flip the
// overlay without prop-drilling or refs.
//
// AIOverlay subscribes to this store to drive its `open` flag, and
// also writes to it on Mod+J / Esc / close-button so the rest of
// the app stays in sync with what the user sees.

import { writable } from 'svelte/store';

export const aiOverlayOpen = writable(false);

export function openAIOverlay(): void {
  aiOverlayOpen.set(true);
}

export function closeAIOverlay(): void {
  aiOverlayOpen.set(false);
}

export function toggleAIOverlay(): void {
  aiOverlayOpen.update((v) => !v);
}
