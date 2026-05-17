import { describe, it, expect } from 'vitest';
import type { ChatMessage } from '$lib/api';
import {
  findPrecedingUserIndex,
  buildBranchTitle,
  pruneNumKeyedRecord
} from './branch';

// Tiny chat-message factory so the test reads as a transcript.
function msgs(roles: ('user' | 'assistant' | 'system')[]): ChatMessage[] {
  return roles.map((role, i) => ({ role, content: `${role} ${i}` }));
}

describe('findPrecedingUserIndex', () => {
  it('returns the immediately-preceding user index', () => {
    // [user, assistant, user, assistant] — for the LAST assistant
    // the answer is index 2 (the second user message).
    expect(findPrecedingUserIndex(msgs(['user', 'assistant', 'user', 'assistant']), 3)).toBe(2);
  });

  it('skips system messages between assistant and user', () => {
    expect(findPrecedingUserIndex(msgs(['user', 'system', 'assistant']), 2)).toBe(0);
  });

  it('returns -1 when no user message exists before the assistant', () => {
    expect(findPrecedingUserIndex(msgs(['system', 'assistant']), 1)).toBe(-1);
  });

  it('returns -1 for out-of-range assistantIdx', () => {
    expect(findPrecedingUserIndex(msgs(['user', 'assistant']), 99)).toBe(-1);
    expect(findPrecedingUserIndex(msgs(['user', 'assistant']), -1)).toBe(-1);
  });

  it('returns -1 for assistantIdx 0 (no preceding messages at all)', () => {
    expect(findPrecedingUserIndex(msgs(['assistant']), 0)).toBe(-1);
  });
});

describe('buildBranchTitle', () => {
  it('appends " (branch)" to short titles unchanged', () => {
    expect(buildBranchTitle('hello world')).toBe('hello world (branch)');
  });

  it('caps source at 60 chars then appends', () => {
    const sixtyOne = 'x'.repeat(61);
    const out = buildBranchTitle(sixtyOne);
    // 60 chars from the source + " (branch)" tail
    expect(out).toBe('x'.repeat(60) + ' (branch)');
  });

  it('does not cap titles exactly at 60', () => {
    const sixty = 'x'.repeat(60);
    expect(buildBranchTitle(sixty)).toBe(sixty + ' (branch)');
  });

  it('handles an empty source title', () => {
    expect(buildBranchTitle('')).toBe(' (branch)');
  });
});

describe('pruneNumKeyedRecord', () => {
  it('drops entries whose key is >= cutoff', () => {
    const got = pruneNumKeyedRecord({ 0: 'a', 1: 'b', 2: 'c', 3: 'd' }, 2);
    expect(got).toEqual({ 0: 'a', 1: 'b' });
  });

  it('keeps all entries when cutoff is past the last key', () => {
    const got = pruneNumKeyedRecord({ 0: 'a', 1: 'b' }, 10);
    expect(got).toEqual({ 0: 'a', 1: 'b' });
  });

  it('drops all entries when cutoff is 0', () => {
    const got = pruneNumKeyedRecord({ 0: 'a', 1: 'b' }, 0);
    expect(got).toEqual({});
  });

  it('returns empty object for empty input', () => {
    expect(pruneNumKeyedRecord({}, 5)).toEqual({});
  });

  it('drops non-numeric keys defensively', () => {
    // Object.entries gives string keys; "foo" coerces to NaN which
    // is not finite — caller's state should never put non-numeric
    // keys here, but the helper is the guard.
    const got = pruneNumKeyedRecord({ 0: 'a', foo: 'b' } as unknown as Record<number, string>, 5);
    expect(got).toEqual({ 0: 'a' });
  });

  it('preserves value identity (not a deep copy)', () => {
    const v = { tag: 'shared' };
    const got = pruneNumKeyedRecord({ 0: v }, 5);
    expect(got[0]).toBe(v);
  });
});
