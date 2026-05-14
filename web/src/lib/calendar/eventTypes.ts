// Event type catalog. Drives the create / edit picker, the chip
// glyph prefix on the calendar grid, and the default colour band
// when the user hasn't picked a specific colour.
//
// Glyphs are single ASCII characters so the chip stays readable in
// tight grid cells (no emoji per the app-wide no-emoji rule). Each
// type has a default colour token from the Catppuccin palette
// (matches granit's --color-* CSS variables); the user's explicit
// `color` field still wins when set.
//
// Adding a new type:
//   1. Append an entry to EVENT_TYPES below.
//   2. Backend doesn't need a code change — it stores Kind as a free
//      string and only the frontend interprets it. Unknown values
//      pass through and render as generic.

export interface EventTypeDef {
  /** Canonical lowercase id stored in events.json + X-GRANIT-KIND. */
  id: string;
  /** Short display label used in the picker chips + EventDetail. */
  label: string;
  /** Single-char glyph used as a chip prefix on the calendar grid. */
  glyph: string;
  /** Granit color token (matches the `--color-${name}` CSS variable
   *  + the calendar's per-color tint). The chip tint defaults to
   *  this when the event has no explicit `color` set. */
  color: string;
  /** Hover-help describing what this type is for. */
  description: string;
  /** Default duration in minutes for new events of this type. The
   *  CreateEvent form pre-fills the end-time using this when the
   *  user picks a type before entering a time. */
  defaultDurationMin?: number;
}

export const EVENT_TYPES: readonly EventTypeDef[] = [
  {
    id: 'meeting',
    label: 'Meeting',
    glyph: 'M',
    color: 'blue',
    description: 'Calls, 1:1s, syncs — anything with other people on the line.',
    defaultDurationMin: 30
  },
  {
    id: 'focus',
    label: 'Focus',
    glyph: 'F',
    color: 'mauve',
    description: 'Deep-work block. Don\'t schedule meetings on top.',
    defaultDurationMin: 90
  },
  {
    id: 'personal',
    label: 'Personal',
    glyph: 'P',
    color: 'pink',
    description: 'Workouts, meals, family — anything outside work.',
    defaultDurationMin: 60
  },
  {
    id: 'travel',
    label: 'Travel',
    glyph: 'T',
    color: 'teal',
    description: 'Commute, trips, transit. Block the time so you don\'t double-book.',
    defaultDurationMin: 60
  },
  {
    id: 'break',
    label: 'Break',
    glyph: 'B',
    color: 'green',
    description: 'Lunch, coffee, rest. Intentional downtime is on the calendar too.',
    defaultDurationMin: 30
  },
  {
    id: 'blocker',
    label: 'Blocker',
    glyph: 'X',
    color: 'red',
    description: 'Hard busy. Visible to scheduling AI as "do not place anything here".',
    defaultDurationMin: 60
  }
] as const;

const BY_ID = new Map(EVENT_TYPES.map((t) => [t.id, t]));

/** Look up a type by id. Returns null for empty / unknown ids so
 *  callers can render generic styling. The frontend never blocks an
 *  unknown kind — it just shows it as un-typed. Defensive trim +
 *  lowercase so a hand-edited events.json with `"kind": " Meeting "`
 *  still resolves; backend write-path already canonicalises but
 *  we don't trust the read path to never see drift. */
export function findEventType(id: string | undefined | null): EventTypeDef | null {
  if (!id) return null;
  const norm = id.trim().toLowerCase();
  if (!norm) return null;
  return BY_ID.get(norm) ?? null;
}

/** Glyph for a kind id, '' if unknown / empty. Used by chip
 *  renderers that want a "prefix or nothing" string. */
export function glyphForKind(id: string | undefined | null): string {
  return findEventType(id)?.glyph ?? '';
}

/** Default color for a kind. Returns '' when no type matches —
 *  callers can fall back to their own default. */
export function colorForKind(id: string | undefined | null): string {
  return findEventType(id)?.color ?? '';
}
