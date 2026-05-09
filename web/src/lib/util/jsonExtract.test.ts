import { describe, expect, it } from 'vitest';
import { extractJsonBlock } from './jsonExtract';

describe('extractJsonBlock', () => {
  it('returns null for empty input', () => {
    expect(extractJsonBlock('')).toBeNull();
  });

  it('returns null when no opening brace exists', () => {
    expect(extractJsonBlock('hello world')).toBeNull();
  });

  it('extracts a simple object', () => {
    expect(extractJsonBlock('{"a":1}')).toBe('{"a":1}');
  });

  it('peels a markdown code fence with json tag', () => {
    expect(extractJsonBlock('```json\n{"a":1}\n```')).toBe('{"a":1}');
  });

  it('peels a markdown code fence without language', () => {
    expect(extractJsonBlock('```\n{"a":1}\n```')).toBe('{"a":1}');
  });

  it('skips prose preamble before the JSON', () => {
    expect(extractJsonBlock('Here is the result:\n{"k":"v"}')).toBe('{"k":"v"}');
  });

  it('handles nested braces', () => {
    expect(extractJsonBlock('{"outer":{"inner":42}}')).toBe('{"outer":{"inner":42}}');
  });

  it('returns null for unbalanced braces (still streaming)', () => {
    // Models often emit JSON byte-by-byte; the first chunks will
    // open more `{` than they close. Returning null lets the
    // caller wait for more chunks before parsing.
    expect(extractJsonBlock('{"plan":[{"id":1')).toBeNull();
  });

  it('finds the first complete object even with trailing prose', () => {
    expect(extractJsonBlock('{"a":1} and that is the result')).toBe('{"a":1}');
  });
});
