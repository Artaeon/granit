// Bridge between DayState (typed in-memory) and the Daily Note
// frontmatter (loose YAML JSON the server reads/writes).
//
// Why a dedicated helper instead of just casting:
//   1. The note exists for the user's prose, not for us. We MUST
//      preserve any fields we don't recognise — a user who hand-
//      writes "weather: sunny" into the YAML shouldn't lose it
//      next time the app touches the note.
//   2. The pillar shape is nested; loose JSON.parse round-trips
//      drop the typing. Doing the merge here keeps the page code
//      from having to defend against malformed pillars on every
//      render.
//   3. Today's date format is a plain YYYY-MM-DD string. Centralised
//      here so the folder convention (Daily/<date>.md) has one
//      source of truth.
//
// What the frontmatter looks like:
//
//   rhythmus_mode: normal | chaotic | emergency
//   rhythmus_fatigue: 1..5
//   rhythmus_eaten: true | false
//   rhythmus_mit: string
//   rhythmus_pillars:
//     spirit:  { done: bool, note?: string }
//     food:    { done: bool, note?: string }
//     work:    { done: bool, note?: string }
//     body:    { done: bool, note?: string }
//     evening: { done: bool, note?: string }
//
// The "rhythmus_" prefix is intentional: the daily note is shared
// with examen / win-sentence / habits and we don't want flat keys
// to collide (e.g. `mode` would be ambiguous).

import { fmtDateISO } from '$lib/util/date';
import { PILLAR_ORDER, type PillarKey } from './pillars';
import {
  emptyDayState,
  emptyPillars,
  emptyShutdown,
  type DayMode,
  type DayState,
  type PillarState,
  type ShutdownState
} from './dayState';

/** Vault-relative path for the daily note of `at`. Mirrors what
 *  QuickCaptureFab + AIOverlay already write to so a single note
 *  per day stays the convention. */
export function dailyNotePath(at: Date = new Date()): string {
  return `Daily/${fmtDateISO(at)}.md`;
}

function isMode(v: unknown): v is DayMode {
  return v === 'normal' || v === 'chaotic' || v === 'emergency';
}

function parsePillar(v: unknown): PillarState {
  if (!v || typeof v !== 'object') return { done: false };
  const o = v as Record<string, unknown>;
  return {
    done: !!o.done,
    note: typeof o.note === 'string' ? o.note : undefined
  };
}

function parsePillars(v: unknown): Record<PillarKey, PillarState> {
  const out = emptyPillars();
  if (!v || typeof v !== 'object') return out;
  const src = v as Record<string, unknown>;
  for (const key of PILLAR_ORDER) {
    if (key in src) out[key] = parsePillar(src[key]);
  }
  return out;
}

function parseShutdown(v: unknown): ShutdownState {
  const out = emptyShutdown();
  if (!v || typeof v !== 'object') return out;
  const o = v as Record<string, unknown>;
  return {
    achieved: typeof o.achieved === 'string' ? o.achieved : '',
    tomorrow: typeof o.tomorrow === 'string' ? o.tomorrow : '',
    letGo:    typeof o.letGo    === 'string' ? o.letGo    : '',
    phoneAway: !!o.phoneAway
  };
}

/** Pull a DayState out of whatever frontmatter the note happens to
 *  have. Missing fields fall back to the empty-day defaults, so a
 *  fresh note (or one that never touched the rhythmus shape) reads
 *  as "no check-in yet today". */
export function parseDayFrontmatter(
  fm: Record<string, unknown> | undefined,
  date: string
): DayState {
  const base = emptyDayState(date);
  if (!fm) return base;
  const mode = isMode(fm.rhythmus_mode) ? fm.rhythmus_mode : null;
  const fatigue =
    typeof fm.rhythmus_fatigue === 'number' && fm.rhythmus_fatigue >= 1 && fm.rhythmus_fatigue <= 5
      ? Math.round(fm.rhythmus_fatigue)
      : base.fatigue;
  const eaten = !!fm.rhythmus_eaten;
  const mit = typeof fm.rhythmus_mit === 'string' ? fm.rhythmus_mit : '';
  const pillars = parsePillars(fm.rhythmus_pillars);
  const shutdown = parseShutdown(fm.rhythmus_shutdown);
  return { date, mode, fatigue, eaten, mit, pillars, shutdown };
}

/** Merge a DayState back into the note's existing frontmatter so
 *  unrelated fields (examen answers, win sentence, custom keys the
 *  user hand-wrote) survive intact. The returned object is what the
 *  caller passes to api.putNote / api.createNote. */
export function serializeDayFrontmatter(
  state: DayState,
  existing: Record<string, unknown> = {}
): Record<string, unknown> {
  const pillars: Record<string, { done: boolean; note?: string }> = {};
  for (const key of PILLAR_ORDER) {
    const p = state.pillars[key];
    pillars[key] = p.note ? { done: p.done, note: p.note } : { done: p.done };
  }
  const out: Record<string, unknown> = { ...existing };
  // Only persist `mode` once the user has actually picked one. Writing
  // null would replace a meaningful field with a placeholder.
  if (state.mode) out.rhythmus_mode = state.mode;
  else delete out.rhythmus_mode;
  out.rhythmus_fatigue = state.fatigue;
  out.rhythmus_eaten = state.eaten;
  if (state.mit.trim()) out.rhythmus_mit = state.mit.trim();
  else delete out.rhythmus_mit;
  out.rhythmus_pillars = pillars;
  const sd = state.shutdown;
  if (sd.achieved || sd.tomorrow || sd.letGo || sd.phoneAway) {
    out.rhythmus_shutdown = {
      achieved: sd.achieved,
      tomorrow: sd.tomorrow,
      letGo: sd.letGo,
      phoneAway: sd.phoneAway
    };
  } else {
    delete out.rhythmus_shutdown;
  }
  return out;
}
