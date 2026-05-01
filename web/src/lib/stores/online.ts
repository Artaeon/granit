import { writable, derived, type Readable } from 'svelte/store';
import { wsConnected } from '$lib/ws';

// Tracks the navigator's network state plus whether our auth'd WebSocket
// is alive. We treat "offline" as "navigator says offline OR ws hasn't been
// up for >5s after a recent disconnect". Pure navigator.onLine is unreliable
// (can lie on captive portals) — combining the two gives a more accurate
// indicator while not flapping on transient connection blips.

const navOnline = writable(typeof navigator !== 'undefined' ? navigator.onLine : true);

if (typeof window !== 'undefined') {
  window.addEventListener('online', () => navOnline.set(true));
  window.addEventListener('offline', () => navOnline.set(false));
}

export const isOnline: Readable<boolean> = derived(
  [navOnline, wsConnected],
  ([$nav, $ws]) => $nav && ($ws || $nav)
);

// `wasOffline` flips true once we observe an offline → online transition,
// so the UI can announce "back online" briefly and then reset.
export const wasOffline = writable(false);

if (typeof window !== 'undefined') {
  let prev = navigator.onLine;
  window.addEventListener('online', () => {
    if (!prev) wasOffline.set(true);
    prev = true;
    setTimeout(() => wasOffline.set(false), 3000);
  });
  window.addEventListener('offline', () => {
    prev = false;
  });
}
