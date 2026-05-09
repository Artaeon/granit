import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest';
import { relativeTime } from './relativeTime';

// Anchor every test at a fixed instant so the assertions stay
// stable across CI runs. The shape we need to test is *what
// `relativeTime` says given a delta from "now"*, not real wall-
// clock time.
const NOW = new Date('2026-05-09T12:00:00Z').getTime();

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
});

afterEach(() => {
  vi.useRealTimers();
});

describe('relativeTime', () => {
  it('returns empty string for null / undefined / NaN', () => {
    expect(relativeTime(null)).toBe('');
    expect(relativeTime(undefined)).toBe('');
    expect(relativeTime('not a date')).toBe('');
  });

  it('uses "just now" inside the 5-second window', () => {
    expect(relativeTime(NOW - 1000)).toBe('just now');
    expect(relativeTime(NOW - 4500)).toBe('just now');
  });

  it('formats minute-grain age with "ago" suffix', () => {
    expect(relativeTime(NOW - 60_000)).toBe('1m ago');
    expect(relativeTime(NOW - 5 * 60_000)).toBe('5m ago');
  });

  it('formats hour-grain age', () => {
    expect(relativeTime(NOW - 3 * 3600_000)).toBe('3h ago');
  });

  it('formats day-grain age', () => {
    expect(relativeTime(NOW - 2 * 86_400_000)).toBe('2d ago');
  });

  it('formats week-grain age (7-29 days)', () => {
    expect(relativeTime(NOW - 14 * 86_400_000)).toBe('2w ago');
  });

  it('formats month-grain age (>30 days)', () => {
    expect(relativeTime(NOW - 60 * 86_400_000)).toBe('2mo ago');
  });

  it('returns empty for future input by default', () => {
    expect(relativeTime(NOW + 60_000)).toBe('');
  });

  it('formats future when opt-in', () => {
    // "in X" prefix is used for future tense; the past-tense
    // "ago" suffix is past-only.
    expect(relativeTime(NOW + 5 * 60_000, { future: true })).toBe('in 5m');
  });

  it('compact mode drops the "ago" suffix', () => {
    expect(relativeTime(NOW - 5 * 60_000, { compact: true })).toBe('5m');
  });

  it('falls back to calendar date past dateThresholdDays', () => {
    const got = relativeTime(NOW - 10 * 86_400_000, {
      dateThresholdDays: 7,
      dateFormatter: (d) => d.toISOString().slice(0, 10)
    });
    expect(got).toBe('2026-04-29');
  });

  it('accepts a Date instance', () => {
    expect(relativeTime(new Date(NOW - 60_000))).toBe('1m ago');
  });

  it('accepts an ISO string', () => {
    expect(relativeTime('2026-05-09T11:55:00Z')).toBe('5m ago');
  });
});
