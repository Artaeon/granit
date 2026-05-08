// Pomodoro timer store — persists across navigation + tab close
// via localStorage so a closed tab during a 25-minute session
// resumes correctly when reopened. State shape:
//
//   - mode: 'idle' | 'focus' | 'break'
//   - endsAt: epoch ms when the current session ends
//   - focusMin / breakMin: configurable durations
//   - lastTask: optional label of what the user is focusing on
//
// The countdown is derived from endsAt rather than incremented
// every second — clock skew + tab throttling don't drift the
// remaining time, and closing/reopening the tab continues exactly
// where it left off.

import { writable, derived, type Readable } from 'svelte/store';

const KEY = 'granit.pomodoro';

export type PomoMode = 'idle' | 'focus' | 'break';

export interface PomoState {
  mode: PomoMode;
  /** epoch ms when the running session ends — only meaningful when mode !== 'idle' */
  endsAt: number;
  /** minutes for next focus session (default 25) */
  focusMin: number;
  /** minutes for next break (default 5) */
  breakMin: number;
  /** free-text label of what the user is working on */
  lastTask: string;
  /** epoch ms when the last completed focus session ended — used to
   *  show a 'finished N min ago' message after the timer rings out */
  lastFinishedAt: number;
}

const DEFAULT: PomoState = {
  mode: 'idle',
  endsAt: 0,
  focusMin: 25,
  breakMin: 5,
  lastTask: '',
  lastFinishedAt: 0
};

function load(): PomoState {
  if (typeof localStorage === 'undefined') return { ...DEFAULT };
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return { ...DEFAULT };
    const s = { ...DEFAULT, ...(JSON.parse(raw) as Partial<PomoState>) };
    // If the stored session is past its endsAt, mark idle but
    // remember the lastFinishedAt so the UI can surface 'finished
    // N min ago' on the next open.
    if (s.mode !== 'idle' && s.endsAt > 0 && Date.now() > s.endsAt) {
      const wasMode = s.mode;
      s.mode = 'idle';
      // Only stamp lastFinishedAt if we caught a focus session
      // ending — break-overruns aren't a 'win' to celebrate.
      if (wasMode === 'focus') s.lastFinishedAt = s.endsAt;
      s.endsAt = 0;
    }
    return s;
  } catch {
    return { ...DEFAULT };
  }
}

function persist(s: PomoState) {
  if (typeof localStorage === 'undefined') return;
  try { localStorage.setItem(KEY, JSON.stringify(s)); } catch {}
}

export const pomodoro = writable<PomoState>(load());
pomodoro.subscribe((s) => persist(s));

// Cross-tab sync — if another tab starts/stops the timer, this
// tab picks up the change so the floating pill stays consistent.
if (typeof window !== 'undefined') {
  window.addEventListener('storage', (e) => {
    if (e.key !== KEY || !e.newValue) return;
    try {
      pomodoro.set({ ...DEFAULT, ...(JSON.parse(e.newValue) as Partial<PomoState>) });
    } catch {}
  });
}

// Derived: ms remaining (negative if past endsAt).
export const pomoRemaining: Readable<number> = derived(pomodoro, ($s, set) => {
  function tick() {
    set($s.mode === 'idle' ? 0 : $s.endsAt - Date.now());
  }
  tick();
  if ($s.mode === 'idle') return;
  const id = setInterval(tick, 250);
  return () => clearInterval(id);
});

export function startFocus(label: string = '') {
  pomodoro.update((s) => ({
    ...s,
    mode: 'focus',
    endsAt: Date.now() + s.focusMin * 60_000,
    lastTask: label || s.lastTask
  }));
}

export function startBreak() {
  pomodoro.update((s) => ({
    ...s,
    mode: 'break',
    endsAt: Date.now() + s.breakMin * 60_000
  }));
}

export function stopTimer() {
  pomodoro.update((s) => ({ ...s, mode: 'idle', endsAt: 0 }));
}

export function setDurations(focusMin: number, breakMin: number) {
  pomodoro.update((s) => ({
    ...s,
    focusMin: Math.max(5, Math.min(120, Math.round(focusMin))),
    breakMin: Math.max(1, Math.min(60, Math.round(breakMin)))
  }));
}

export function setLastTask(label: string) {
  pomodoro.update((s) => ({ ...s, lastTask: label }));
}

export function fmtMMSS(ms: number): string {
  if (ms < 0) ms = 0;
  const total = Math.ceil(ms / 1000);
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
}
