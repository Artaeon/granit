import { describe, expect, it, vi } from 'vitest';

// The sabbath module fires a window-side-effects block on import
// (refetchSchedule + visibilitychange listener). Mock the api so
// the fetch is a noop in tests.
vi.mock('$lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/api')>();
  return {
    ...actual,
    api: {
      ...actual.api,
      getSabbath: vi.fn().mockResolvedValue({ schedule: {} }),
      putSabbath: vi.fn().mockResolvedValue(undefined)
    }
  };
});

import { scheduleWindow, scheduleSaysNow, type SabbathSchedule } from './sabbath';

const SAT_24H: SabbathSchedule = {
  enabled: true,
  dayOfWeek: 6,         // Saturday
  startHour: 0,
  startMinute: 0,
  durationMinutes: 1440 // midnight → midnight
};

const FRI_18_TO_SAT_18: SabbathSchedule = {
  enabled: true,
  dayOfWeek: 5,         // Friday
  startHour: 18,
  startMinute: 0,
  durationMinutes: 1440 // sundown to sundown
};

describe('scheduleWindow', () => {
  it('returns null when the schedule is disabled', () => {
    const s = { ...SAT_24H, enabled: false };
    expect(scheduleWindow(s, new Date(2024, 4, 18, 12))).toBeNull();
  });

  it('returns the Saturday window for any moment inside it', () => {
    // Saturday May 18 2024, 12:00.
    const at = new Date(2024, 4, 18, 12);
    const w = scheduleWindow(SAT_24H, at);
    expect(w).not.toBeNull();
    expect(w!.start.getDate()).toBe(18);
    expect(w!.start.getHours()).toBe(0);
    expect(w!.end.getDate()).toBe(19);
    expect(w!.end.getHours()).toBe(0);
  });

  it('rolls back to the prior Saturday on a Sunday after the window closes', () => {
    // Sunday May 19 2024, 10:00 — the Saturday window has already ended.
    // The walk-back finds last week's Friday/Saturday match. For SAT_24H
    // that's the same prior Saturday.
    const at = new Date(2024, 4, 19, 10);
    const w = scheduleWindow(SAT_24H, at);
    expect(w).not.toBeNull();
    expect(w!.start.getDate()).toBe(18);
  });
});

describe('scheduleSaysNow — auto-rollover at the window edge', () => {
  it('is active one minute before midnight on Saturday', () => {
    const at = new Date(2024, 4, 18, 23, 59);
    expect(scheduleSaysNow(SAT_24H, at)).toBe(true);
  });

  it('flips off the instant Sunday begins (midnight rollover)', () => {
    const at = new Date(2024, 4, 19, 0, 0, 0, 0);
    // [start, end) — end is exclusive, so midnight Sunday is OUTSIDE the window.
    expect(scheduleSaysNow(SAT_24H, at)).toBe(false);
  });

  it('is active during the Friday-evening half of a sundown-to-sundown window', () => {
    // Friday May 17 2024, 20:00 — two hours into the window.
    const at = new Date(2024, 4, 17, 20, 0);
    expect(scheduleSaysNow(FRI_18_TO_SAT_18, at)).toBe(true);
  });

  it('is still active early Saturday afternoon (window spans midnight)', () => {
    // Saturday May 18 2024, 14:00 — still inside Friday 18:00 + 24h.
    const at = new Date(2024, 4, 18, 14, 0);
    expect(scheduleSaysNow(FRI_18_TO_SAT_18, at)).toBe(true);
  });

  it('closes at the sundown boundary on Saturday at 18:00', () => {
    const at = new Date(2024, 4, 18, 18, 0);
    expect(scheduleSaysNow(FRI_18_TO_SAT_18, at)).toBe(false);
  });
});
