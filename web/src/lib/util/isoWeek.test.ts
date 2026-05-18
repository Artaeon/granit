import { describe, expect, it } from 'vitest';
import { isoWeekParts, isoWeekString, planNotePath, startOfIsoWeek } from './isoWeek';

describe('isoWeekParts', () => {
  it('returns week 1 for an early-January Thursday', () => {
    // Thursday Jan 4 2024 — definitely in W01.
    expect(isoWeekParts(new Date(2024, 0, 4))).toEqual({ year: 2024, week: 1 });
  });

  it('returns the prior year for a Jan-1 that falls before that year\'s first Thursday', () => {
    // Jan 1 2023 was a Sunday — ISO 8601 puts it in W52 of 2022.
    expect(isoWeekParts(new Date(2023, 0, 1))).toEqual({ year: 2022, week: 52 });
  });

  it('rolls forward to the next year when Dec-31 belongs to W01 of the next year', () => {
    // Dec 31 2018 was a Monday — ISO puts it in 2019-W01.
    expect(isoWeekParts(new Date(2018, 11, 31))).toEqual({ year: 2019, week: 1 });
  });

  it('produces the same week for Monday and Sunday of the same ISO week', () => {
    // Mon May 13 2024 and Sun May 19 2024 are both W20.
    const mon = isoWeekParts(new Date(2024, 4, 13));
    const sun = isoWeekParts(new Date(2024, 4, 19));
    expect(mon).toEqual(sun);
    expect(mon.week).toBe(20);
  });
});

describe('isoWeekString', () => {
  it('formats as YYYY-Www with zero-padded week', () => {
    expect(isoWeekString(new Date(2024, 0, 4))).toBe('2024-W01');
    expect(isoWeekString(new Date(2024, 4, 13))).toBe('2024-W20');
  });
});

describe('startOfIsoWeek', () => {
  it('rolls a Thursday back to that ISO week\'s Monday at 00:00 local', () => {
    // Thursday May 16 2024 → Monday May 13 2024 00:00.
    const start = startOfIsoWeek(new Date(2024, 4, 16, 14, 30, 12));
    expect(start.getFullYear()).toBe(2024);
    expect(start.getMonth()).toBe(4); // May
    expect(start.getDate()).toBe(13);
    expect(start.getHours()).toBe(0);
    expect(start.getMinutes()).toBe(0);
    expect(start.getSeconds()).toBe(0);
  });

  it('returns a Monday input unchanged in date, with hours zeroed', () => {
    const start = startOfIsoWeek(new Date(2024, 4, 13, 23, 59, 59));
    expect(start.getDate()).toBe(13);
    expect(start.getHours()).toBe(0);
  });

  it('crosses a month boundary when the week starts in the prior month', () => {
    // Sunday Jun 2 2024 → Monday May 27 2024.
    const start = startOfIsoWeek(new Date(2024, 5, 2));
    expect(start.getMonth()).toBe(4); // May
    expect(start.getDate()).toBe(27);
  });
});

describe('planNotePath', () => {
  it('uses the same week string under Plans/', () => {
    expect(planNotePath(new Date(2024, 4, 13))).toBe('Plans/2024-W20.md');
  });
});
