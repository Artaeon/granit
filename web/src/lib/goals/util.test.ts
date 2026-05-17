import { describe, it, expect, beforeAll, afterAll, vi } from 'vitest';
import {
  daysUntilTarget,
  targetUrgencyTone,
  targetBorderColor,
  targetChip,
  statusColor,
  fmtTargetDate,
  goalTargetTone
} from './util';

// Freeze "today" so the tests don't drift across midnight. 2026-05-17
// happens to be a Sunday; the project's currentDate marker matches.
const FAKE_NOW = new Date('2026-05-17T12:00:00');

beforeAll(() => {
  vi.useFakeTimers();
  vi.setSystemTime(FAKE_NOW);
});

afterAll(() => {
  vi.useRealTimers();
});

describe('daysUntilTarget', () => {
  it('returns null for null/undefined/empty', () => {
    expect(daysUntilTarget(null)).toBeNull();
    expect(daysUntilTarget(undefined)).toBeNull();
    expect(daysUntilTarget('')).toBeNull();
  });
  it('returns null for unparseable free-text targets', () => {
    expect(daysUntilTarget('Q4 2026')).toBeNull();
    expect(daysUntilTarget('fall sometime')).toBeNull();
  });
  it('returns 0 for today', () => {
    expect(daysUntilTarget('2026-05-17')).toBe(0);
  });
  it('returns positive for future dates', () => {
    expect(daysUntilTarget('2026-05-24')).toBe(7);
  });
  it('returns negative for past dates', () => {
    expect(daysUntilTarget('2026-05-10')).toBe(-7);
  });
});

describe('targetUrgencyTone', () => {
  it('returns null for null', () => {
    expect(targetUrgencyTone(null)).toBeNull();
  });
  it('returns error for past-target (negative days)', () => {
    expect(targetUrgencyTone(-1)).toBe('error');
    expect(targetUrgencyTone(-100)).toBe('error');
  });
  it('returns warning for the next 30 days', () => {
    expect(targetUrgencyTone(0)).toBe('warning');
    expect(targetUrgencyTone(1)).toBe('warning');
    expect(targetUrgencyTone(30)).toBe('warning');
  });
  it('returns info for 31..90 days', () => {
    expect(targetUrgencyTone(31)).toBe('info');
    expect(targetUrgencyTone(90)).toBe('info');
  });
  it('returns null past 90 days (no urgency cue)', () => {
    expect(targetUrgencyTone(91)).toBeNull();
    expect(targetUrgencyTone(365)).toBeNull();
  });
});

describe('targetBorderColor', () => {
  // The border palette has a tighter "alarm" band than the tone:
  // <=7 days flips to error (not just <0). Test the breakpoints.
  it('returns surface2 for null', () => {
    expect(targetBorderColor(null)).toBe('var(--color-surface2)');
  });
  it('returns error for <=7 days (including past)', () => {
    expect(targetBorderColor(-5)).toBe('var(--color-error)');
    expect(targetBorderColor(0)).toBe('var(--color-error)');
    expect(targetBorderColor(7)).toBe('var(--color-error)');
  });
  it('returns warning for 8..30 days', () => {
    expect(targetBorderColor(8)).toBe('var(--color-warning)');
    expect(targetBorderColor(30)).toBe('var(--color-warning)');
  });
  it('returns info for 31..90 days', () => {
    expect(targetBorderColor(31)).toBe('var(--color-info)');
    expect(targetBorderColor(90)).toBe('var(--color-info)');
  });
  it('returns surface2 past 90 days', () => {
    expect(targetBorderColor(91)).toBe('var(--color-surface2)');
  });
});

describe('targetChip', () => {
  it('returns null when daysUntil is null', () => {
    expect(targetChip(null)).toBeNull();
    expect(targetChip('Q4 2026')).toBeNull();
  });
  it('past-target → "Nd past target"', () => {
    expect(targetChip('2026-05-10')).toEqual({ label: '7d past target', tone: 'error' });
  });
  it('today → "today"', () => {
    expect(targetChip('2026-05-17')).toEqual({ label: 'today', tone: 'error' });
  });
  it('tomorrow → "tomorrow"', () => {
    expect(targetChip('2026-05-18')).toEqual({ label: 'tomorrow', tone: 'warning' });
  });
  it('day-count below 14 → "in Nd"', () => {
    expect(targetChip('2026-05-23')).toEqual({ label: 'in 6d', tone: 'warning' });
  });
  it('within 14..59 days → "in Nw"', () => {
    // 30 days out → in 4w, tone warning at boundary
    expect(targetChip('2026-06-16')).toEqual({ label: 'in 4w', tone: 'warning' });
  });
  it('60+ days → "in Nmo"', () => {
    // 120 days out → in 4mo, tone subtext
    expect(targetChip('2026-09-14')).toEqual({ label: 'in 4mo', tone: 'subtext' });
  });
});

describe('statusColor', () => {
  it('returns the canonical bg/text for each status', () => {
    expect(statusColor('active')).toEqual({ bg: 'bg-primary/15', text: 'text-primary' });
    expect(statusColor('paused')).toEqual({ bg: 'bg-surface1', text: 'text-subtext' });
    expect(statusColor('completed')).toEqual({ bg: 'bg-surface0', text: 'text-success' });
    expect(statusColor('archived')).toEqual({ bg: 'bg-surface1', text: 'text-dim' });
  });
  it('falls back to subtext for unknown status (including undefined)', () => {
    expect(statusColor(undefined)).toEqual({ bg: 'bg-surface1', text: 'text-subtext' });
    expect(statusColor('what')).toEqual({ bg: 'bg-surface1', text: 'text-subtext' });
  });
});

describe('fmtTargetDate', () => {
  it('returns empty for null/undefined/empty', () => {
    expect(fmtTargetDate(null)).toBe('');
    expect(fmtTargetDate(undefined)).toBe('');
    expect(fmtTargetDate('')).toBe('');
  });
  it('formats a parseable ISO date', () => {
    // Locale-dependent; toLocaleDateString with the options always
    // includes "Jan" / "2026". Don't pin the exact comma format.
    const out = fmtTargetDate('2026-01-12');
    expect(out).toMatch(/Jan/);
    expect(out).toMatch(/2026/);
    expect(out).toMatch(/12/);
  });
  it('passes through free-text targets the user typed', () => {
    expect(fmtTargetDate('Q4 2026')).toBe('Q4 2026');
    expect(fmtTargetDate('fall sometime')).toBe('fall sometime');
  });
});

describe('goalTargetTone', () => {
  it('returns null for completed status regardless of date', () => {
    expect(goalTargetTone('completed', '2026-05-10')).toBeNull();
    expect(goalTargetTone('completed', '2030-01-01')).toBeNull();
  });
  it('returns null for archived status regardless of date', () => {
    expect(goalTargetTone('archived', '2026-05-10')).toBeNull();
  });
  it('returns null for goals with no target_date', () => {
    expect(goalTargetTone('active', null)).toBeNull();
    expect(goalTargetTone('active', 'Q4 2026')).toBeNull();
  });
  it('returns error for active past-target', () => {
    expect(goalTargetTone('active', '2026-05-10')).toBe('error');
  });
  it('returns warning for active within 30 days', () => {
    expect(goalTargetTone('active', '2026-05-24')).toBe('warning');
  });
  it('returns info for active 31..90 days', () => {
    expect(goalTargetTone('active', '2026-07-15')).toBe('info');
  });
  it('paused status gets the same urgency treatment as active', () => {
    expect(goalTargetTone('paused', '2026-05-10')).toBe('error');
    expect(goalTargetTone('paused', '2026-05-24')).toBe('warning');
  });
});
