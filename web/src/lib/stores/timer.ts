// Active-timer store. Single source of truth for "is a clock-in
// running right now?" — hydrated on auth from /timetracker, kept in
// sync via WS frames timer.started / timer.stopped, and updated by
// the TaskCard play button locally so the UI is responsive even if
// the WS round-trip lags.
//
// Holds the minutes-per-task rollup too so TaskCard can show a "23m
// tracked" pill without each card making its own request.

import { writable, derived, type Readable } from 'svelte/store';
import type { ActiveTimer } from '$lib/api';

export const activeTimer = writable<ActiveTimer | null>(null);
export const minutesByTaskId = writable<Record<string, number>>({});

// elapsedSec: a derived store that ticks every second so the pill
// shows live elapsed time. Returns 0 when no timer is running.
const tick = writable(Date.now());
if (typeof window !== 'undefined') {
  setInterval(() => tick.set(Date.now()), 1000);
}

export const elapsedSec: Readable<number> = derived([activeTimer, tick], ([$t, $now]) => {
  if (!$t) return 0;
  const start = new Date($t.startTime).getTime();
  if (isNaN(start)) return 0;
  return Math.max(0, Math.floor(($now - start) / 1000));
});

// fmtDuration: "1:23" for under an hour, "1h 23m" otherwise. Cheap
// formatter so callers don't reinvent.
export function fmtDuration(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  if (m < 60) return `${m}:${String(seconds % 60).padStart(2, '0')}`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}
