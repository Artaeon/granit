import { describe, expect, it } from 'vitest';
import {
  priorityTone,
  priorityTextClass,
  priorityBadgeClass,
  priorityBorderClass
} from './priority';

describe('priorityTone', () => {
  it('maps 1 → error, 2 → warning, 3 → info', () => {
    expect(priorityTone(1)).toBe('error');
    expect(priorityTone(2)).toBe('warning');
    expect(priorityTone(3)).toBe('info');
  });

  it('falls back to dim for any other number', () => {
    expect(priorityTone(0)).toBe('dim');
    expect(priorityTone(4)).toBe('dim');
    expect(priorityTone(99)).toBe('dim');
    expect(priorityTone(-1)).toBe('dim');
  });

  it('handles null / undefined as dim', () => {
    expect(priorityTone(null)).toBe('dim');
    expect(priorityTone(undefined)).toBe('dim');
  });
});

describe('priorityTextClass', () => {
  it('returns the matching text-color class for each tone', () => {
    expect(priorityTextClass(1)).toBe('text-error');
    expect(priorityTextClass(2)).toBe('text-warning');
    expect(priorityTextClass(3)).toBe('text-info');
    expect(priorityTextClass(0)).toBe('text-dim');
  });
});

describe('priorityBadgeClass', () => {
  it('composes bg + text + border at consistent weights', () => {
    expect(priorityBadgeClass(1)).toBe('bg-error/20 text-error border-error/30');
    expect(priorityBadgeClass(2)).toBe('bg-warning/20 text-warning border-warning/30');
    expect(priorityBadgeClass(3)).toBe('bg-info/20 text-info border-info/30');
  });

  it('returns empty string for dim tone — chip stays undecorated', () => {
    expect(priorityBadgeClass(0)).toBe('');
    expect(priorityBadgeClass(undefined)).toBe('');
  });
});

describe('priorityBorderClass', () => {
  it('returns matching border colors for each tone', () => {
    expect(priorityBorderClass(1)).toBe('border-error');
    expect(priorityBorderClass(2)).toBe('border-warning');
    expect(priorityBorderClass(3)).toBe('border-info');
  });

  it('falls back to surface1 (subtle) for dim', () => {
    expect(priorityBorderClass(0)).toBe('border-surface1');
    expect(priorityBorderClass(null)).toBe('border-surface1');
  });
});
