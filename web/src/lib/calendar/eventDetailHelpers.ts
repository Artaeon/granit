// Pure helpers extracted from EventDetail.svelte — no runes, no state.
// Lives here so the modal stays focused on view + controller wiring and
// the math is unit-testable in isolation.
//
// Three concerns:
//   1. utcRFC3339FromLocalParts — wall-clock YYYY-MM-DD + HH:MM → UTC
//      RFC3339 (Z-suffixed). Used by the ICS write path only; events.json
//      stores date + HH:MM verbatim and skips the conversion.
//   2. exDateKey — surface the canonical anchor for EXDATE / override
//      lookups. Prefers event.override_key (already-overridden
//      occurrences), then falls back to event.start (timed) or
//      event.date (all-day).
//   3. colorOptions — the picker palette. Same shape used elsewhere
//      in the app; centralised here so the modal doesn't carry it inline.

import type { CalendarEvent } from '$lib/api';

/** Build a UTC RFC3339 (Z-suffixed) string from YYYY-MM-DD + HH:MM
 *  interpreted as the user's LOCAL wall clock. The Date constructor
 *  (with separate y/mo/d/h/mi args) treats the inputs as local time,
 *  and `.toISOString()` then renders the resulting instant in UTC —
 *  so a EU user typing 14:30 sends "12:30:00Z" in summer. The
 *  backend's parseClientTime accepts RFC3339, and the icswriter
 *  emits UTC Z, so the round-trip preserves wall-clock intent.
 *  Used only for the ICS write path; events.json takes separate
 *  date + HH:MM fields and stores them verbatim (see the patchEvent
 *  branch). Name is utc* (not local*) because the OUTPUT is
 *  a UTC instant — the misleading earlier name made it look like a
 *  floating-time helper. */
export function utcRFC3339FromLocalParts(date: string, time: string): string {
  const [y, mo, d] = date.split('-').map(Number);
  const [h, mi] = time.split(':').map(Number);
  return new Date(y, mo - 1, d, h, mi, 0, 0).toISOString();
}

/** Skip just THIS occurrence of a recurring event — the user's
 *  "team meeting cancelled this week, but keep the series" move.
 *  Adds an EXDATE entry to the source event; the expander filters
 *  it out the next time the calendar renders. The expander
 *  (internal/serveapi/ics.go isExcluded) compares against:
 *    - YYYY-MM-DD for all-day events
 *    - YYYY-MM-DDTHH:MM:SS in UTC for timed events
 *  We must send the UTC format so the next render's exclusion
 *  check actually matches. Only native recurring events get this;
 *  ICS skip would need to round-trip the source .ics file which
 *  isn't on the patch path today. */
export function exDateKey(event: CalendarEvent | null): string {
  if (!event) return '';
  // For an already-overridden occurrence, the canonical anchor is
  // surfaced as event.override_key by the calendar feed. Prefer it
  // — re-deriving from event.start would point at the OVERRIDDEN
  // time, not the series anchor, and the EXDATE/Override map keys
  // by anchor.
  if (event.override_key) return event.override_key;
  if (event.start) {
    // event.start is the floating-ISO emit from handleCalendar
    // ("2026-05-09T08:00:00", no Z, no offset). The server keys
    // overrides + EXDATEs by the same wall-clock digits — slicing
    // the leading 19 chars matches that shape directly.
    // Round-tripping through new Date(...).toISOString() would
    // re-anchor the wall-clock to the client zone and then re-emit
    // in UTC, shifting the key by the client offset (e.g. on UTC+2,
    // 08:00 floating → 06:00 UTC). The skip / reset endpoints would
    // then store at the wrong anchor and the EXDATE would no longer
    // match the expander's emitted occurrence.
    return event.start.slice(0, 19);
  }
  return event.date ?? '';
}

/** Color picker palette — Apple's named-color set. Shared between
 *  the edit form here and create flows elsewhere. */
export const colorOptions: { name: string; hex: string }[] = [
  { name: 'red', hex: '#ff3b30' },
  { name: 'orange', hex: '#ff9500' },
  { name: 'yellow', hex: '#ffcc00' },
  { name: 'green', hex: '#34c759' },
  { name: 'mint', hex: '#00c7be' },
  { name: 'teal', hex: '#5ac8fa' },
  { name: 'blue', hex: '#007aff' },
  { name: 'indigo', hex: '#5856d6' },
  { name: 'purple', hex: '#af52de' },
  { name: 'pink', hex: '#ff2d55' },
  { name: 'brown', hex: '#a2845e' },
  { name: 'gray', hex: '#8e8e93' }
];
