// Per-habit weekly-target edit popover state.
//
// Eighth extraction step out of routes/habits/+page.svelte. Owns the
// "which row's target popover is currently open" tracker + the
// bump / clear / targetState helpers that the template binds to.
//
// Targets live in localStorage via $lib/habits/targets (a global
// store) so the popover doesn't round-trip the server — cross-device
// drift is fine, and the chip is purely a display preference. This
// controller is the *editing UX* on top of that store: single-row
// popover-open state + bump-by-delta + clear handlers.
//
// targetState() is reactive on $habitTargets (via the deps getter)
// so a clearTarget call repaints the chip without a full reload.

import type { HabitInfo } from '$lib/api';
import { last7Done } from '$lib/habits/habitsDerives';
import { setHabitTarget } from '$lib/habits/targets';

export interface HabitsTargetsEditDeps {
  /** Reactive read of the habit-targets map. The page passes
   *  () => $habitTargets so the chip recomputes on store change. */
  getTargets: () => Record<string, number>;
}

export interface HabitsTargetsEditController {
  editingTarget: string | null;
  /** Snapshot for the chip row: target / week, done-this-week,
   *  pct (clamped to 1). null when the habit has no target set. */
  targetState(h: HabitInfo): { target: number; done: number; pct: number } | null;
  /** Bump within [1, 7]. delta is signed. */
  bumpTarget(name: string, delta: number): void;
  /** Drop the target + close the popover. */
  clearTarget(name: string): void;
}

export function createHabitsTargetsEdit(
  deps: HabitsTargetsEditDeps
): HabitsTargetsEditController {
  let editingTarget = $state<string | null>(null);

  function targetState(
    h: HabitInfo
  ): { target: number; done: number; pct: number } | null {
    const target = deps.getTargets()[h.name];
    if (!target) return null;
    const done = last7Done(h);
    return { target, done, pct: Math.min(1, done / target) };
  }

  function bumpTarget(name: string, delta: number) {
    const cur = deps.getTargets()[name] ?? 7;
    const next = Math.max(1, Math.min(7, cur + delta));
    setHabitTarget(name, next);
  }

  function clearTarget(name: string) {
    setHabitTarget(name, null);
    editingTarget = null;
  }

  return {
    get editingTarget() { return editingTarget; },
    set editingTarget(v) { editingTarget = v; },
    targetState,
    bumpTarget,
    clearTarget
  };
}
