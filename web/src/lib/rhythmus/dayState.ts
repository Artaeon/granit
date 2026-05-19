// The shape of a single day's Rhythmus state. Persisted as the
// frontmatter of Daily/YYYY-MM-DD.md so the TUI + future agents
// see the same data without a separate sidecar — and so the body
// of the daily note stays free prose.
//
// Why three day modes + a null:
//   - null: the user hasn't checked in yet today. The Heute-Karte
//     shows the morning check-in instead of the pillar list.
//   - normal: all five pillars expected.
//   - chaotic: only the load-bearing ones (food / work / body /
//     evening) — spirit collapses to a one-breath form.
//   - emergency: bare survival list. Most pillars hide. The
//     point is permission to do less, not productivity.

import type { PillarKey } from './pillars';

export type DayMode = 'normal' | 'chaotic' | 'emergency';

export type PillarState = {
  done: boolean;
  /** Per-day override of the minimum text — empty/undefined means
   *  the user accepts whatever the rhythm settings say for the
   *  current mode. Lets the user write "Psalm 23" once on a
   *  Wednesday without changing the global default. */
  note?: string;
};

/** Four-line shutdown bookmark. Persisted alongside the rest of
 *  the day state so the next morning's check-in (and the weekly
 *  review) can see what the user committed to last night. */
export type ShutdownState = {
  achieved: string;
  tomorrow: string;
  letGo: string;
  phoneAway: boolean;
};

export type DayState = {
  /** YYYY-MM-DD (local). The folder convention is Daily/<date>.md. */
  date: string;
  /** null when the user hasn't run the morning check-in yet. */
  mode: DayMode | null;
  /** 1 (rested) … 5 (wrecked). Defaults to 3 if not yet rated. */
  fatigue: number;
  /** First-meal-of-the-day flag — drives the "eat first" branch of
   *  the nextAction engine before any pillar takes priority. */
  eaten: boolean;
  /** Most Important Task: a single free-text sentence. Not a task
   *  id — the user names what matters today; if they want to wire
   *  it to a real task, the work-pillar handler does that. */
  mit: string;
  pillars: Record<PillarKey, PillarState>;
  shutdown: ShutdownState;
};

export function emptyShutdown(): ShutdownState {
  return { achieved: '', tomorrow: '', letGo: '', phoneAway: false };
}

export function emptyPillars(): Record<PillarKey, PillarState> {
  return {
    spirit:  { done: false },
    food:    { done: false },
    work:    { done: false },
    body:    { done: false },
    evening: { done: false }
  };
}

export function emptyDayState(date: string): DayState {
  return {
    date,
    mode: null,
    fatigue: 3,
    eaten: false,
    mit: '',
    pillars: emptyPillars(),
    shutdown: emptyShutdown()
  };
}
