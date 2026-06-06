// Per-habit weekly targets, stored per-device in localStorage.
//
// A target is "do this habit N days per week" — e.g. running 3x/wk
// is a success, 5x/wk is a stretch. We don't gate anything on the
// target; we just surface a chip so the user reads "this week 3/5"
// at a glance. No backend round-trip — habits themselves are
// derived from daily-note checkboxes, and the target is purely a
// display preference. Cross-device drift is fine.

import { get } from 'svelte/store';
import { persistedWritable } from '$lib/util/persistedWritable';

const KEY = 'granit.habits.targets';

export const habitTargets = persistedWritable<Record<string, number>>(KEY, {}, {
  validate: (raw) => {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return {};
    const out: Record<string, number> = {};
    for (const [k, v] of Object.entries(raw)) {
      const n = Number(v);
      if (Number.isFinite(n) && n >= 1 && n <= 7) out[k] = Math.round(n);
    }
    return out;
  }
});

export function setHabitTarget(name: string, target: number | null) {
  habitTargets.update((m) => {
    const next = { ...m };
    if (target === null || target === 0) delete next[name];
    else next[name] = Math.max(1, Math.min(7, Math.round(target)));
    return next;
  });
}

