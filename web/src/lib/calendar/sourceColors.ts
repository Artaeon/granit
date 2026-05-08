// Per-device color override for ICS calendar sources. The server
// emits 'cyan' for every ICS event by default — useful as a single
// "this came from a feed" marker, less useful when the user has
// multiple subscriptions and wants to tell faith / training / work
// apart at a glance. We store the user's per-source choice in
// localStorage so the calendar grid can paint each source in a
// distinct hue without any backend round-trip.
//
// Future: round-trip to a config sidecar for cross-device sync.
// For now per-device is enough — the same user usually has the
// same colour preferences across machines anyway.

import { writable, get } from 'svelte/store';

const KEY = 'granit.calendar.source-colors';

/** Tone names matching the calendar's eventTypeColor palette so
 *  the picker reuses the same swatches the rest of the UI does. */
export type CalendarTone =
  | ''
  | 'red'
  | 'yellow'
  | 'orange'
  | 'green'
  | 'blue'
  | 'purple'
  | 'cyan'
  | 'pink';

function load(): Record<string, CalendarTone> {
  if (typeof localStorage === 'undefined') return {};
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return {};
    return JSON.parse(raw) as Record<string, CalendarTone>;
  } catch {
    return {};
  }
}

function persist(map: Record<string, CalendarTone>) {
  if (typeof localStorage === 'undefined') return;
  try { localStorage.setItem(KEY, JSON.stringify(map)); } catch {}
}

export const sourceColors = writable<Record<string, CalendarTone>>(load());
sourceColors.subscribe((m) => persist(m));

export function setSourceColor(source: string, tone: CalendarTone) {
  sourceColors.update((m) => {
    const next = { ...m };
    if (tone === '') delete next[source]; // empty resets to default
    else next[source] = tone;
    return next;
  });
}

export function getSourceColor(source: string): CalendarTone {
  return get(sourceColors)[source] ?? '';
}

/** Re-color an event in-place based on the saved per-source map.
 *  Pass-through for non-ICS events (no source) so callers don't
 *  need to gate the call. */
export function applySourceColor<T extends { color?: string; source?: string }>(
  ev: T,
  map: Record<string, CalendarTone>
): T {
  if (!ev.source) return ev;
  const override = map[ev.source];
  if (!override) return ev;
  return { ...ev, color: override };
}
