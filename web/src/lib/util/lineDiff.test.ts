import { describe, expect, it } from 'vitest';
import { lineDiff, diffStats } from './lineDiff';

describe('lineDiff', () => {
  it('returns all eq for identical inputs', () => {
    const got = lineDiff('a\nb\nc', 'a\nb\nc');
    expect(got.every((l) => l.type === 'eq')).toBe(true);
    expect(got.map((l) => l.text)).toEqual(['a', 'b', 'c']);
  });

  it('marks an inserted line as add', () => {
    const got = lineDiff('a\nc', 'a\nb\nc');
    expect(got).toEqual([
      { type: 'eq', text: 'a' },
      { type: 'add', text: 'b' },
      { type: 'eq', text: 'c' }
    ]);
  });

  it('marks a deleted line as del', () => {
    const got = lineDiff('a\nb\nc', 'a\nc');
    expect(got).toEqual([
      { type: 'eq', text: 'a' },
      { type: 'del', text: 'b' },
      { type: 'eq', text: 'c' }
    ]);
  });

  it('handles a replaced line as del + add', () => {
    const got = lineDiff('a\nb\nc', 'a\nB\nc');
    expect(got.filter((l) => l.type === 'del')).toEqual([{ type: 'del', text: 'b' }]);
    expect(got.filter((l) => l.type === 'add')).toEqual([{ type: 'add', text: 'B' }]);
  });

  it('handles fully different inputs', () => {
    const got = lineDiff('a\nb', 'x\ny');
    // Every line is del-or-add; no eq survives.
    expect(got.every((l) => l.type !== 'eq')).toBe(true);
    expect(got.filter((l) => l.type === 'del').map((l) => l.text)).toEqual(['a', 'b']);
    expect(got.filter((l) => l.type === 'add').map((l) => l.text)).toEqual(['x', 'y']);
  });

  it('handles trailing-only addition', () => {
    const got = lineDiff('a\nb', 'a\nb\nc\nd');
    expect(got.filter((l) => l.type === 'add').map((l) => l.text)).toEqual(['c', 'd']);
  });

  it('handles trailing-only deletion', () => {
    const got = lineDiff('a\nb\nc\nd', 'a\nb');
    expect(got.filter((l) => l.type === 'del').map((l) => l.text)).toEqual(['c', 'd']);
  });

  it('handles empty old text (entire new is added)', () => {
    // String.split('\n') on '' returns [''], so the diff sees one
    // empty "line" deleted from a and both new lines added from b
    // — no equals (the empty string isn't shared with 'a' or 'b').
    // Documenting the actual shape here so a future change to that
    // boundary breaks loudly.
    const got = lineDiff('', 'a\nb');
    expect(got.filter((l) => l.type === 'add').map((l) => l.text)).toEqual(['a', 'b']);
    expect(got.filter((l) => l.type === 'del').map((l) => l.text)).toEqual(['']);
    expect(got.filter((l) => l.type === 'eq')).toEqual([]);
  });
});

describe('diffStats', () => {
  it('counts added + removed lines', () => {
    const diff = lineDiff('a\nb', 'a\nB\nc');
    const stats = diffStats(diff);
    expect(stats.added).toBe(2); // B + c
    expect(stats.removed).toBe(1); // b
  });

  it('returns zero for identical inputs', () => {
    expect(diffStats(lineDiff('x\ny', 'x\ny'))).toEqual({ added: 0, removed: 0 });
  });

  it('returns zero for empty diff', () => {
    expect(diffStats([])).toEqual({ added: 0, removed: 0 });
  });
});
