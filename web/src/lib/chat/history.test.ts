import { describe, it, expect, beforeEach } from 'vitest';
import {
  ACTIVE_THREAD_KEY,
  loadActiveThreadId,
  persistActiveThreadId,
  deriveLibraryLabel
} from './history';

// sessionStorage is jsdom-provided in vitest. Wipe between tests
// so a previous test's persisted id doesn't leak.
beforeEach(() => {
  sessionStorage.clear();
});

describe('persistActiveThreadId / loadActiveThreadId', () => {
  it('round-trips a non-empty id', () => {
    persistActiveThreadId('abc123');
    expect(loadActiveThreadId()).toBe('abc123');
  });

  it('returns empty string when nothing stored', () => {
    expect(loadActiveThreadId()).toBe('');
  });

  it('persisting empty string deletes the key (not literal ""))', () => {
    persistActiveThreadId('abc');
    expect(sessionStorage.getItem(ACTIVE_THREAD_KEY)).toBe('abc');
    persistActiveThreadId('');
    expect(sessionStorage.getItem(ACTIVE_THREAD_KEY)).toBeNull();
  });

  it('load returns empty for a key explicitly set to empty string', () => {
    // The persistActiveThreadId contract treats "" as delete, but
    // if something else wrote an empty string directly (legacy data,
    // hand-edit) the reader should still return "" cleanly.
    sessionStorage.setItem(ACTIVE_THREAD_KEY, '');
    expect(loadActiveThreadId()).toBe('');
  });
});

describe('deriveLibraryLabel', () => {
  it('returns empty for empty / whitespace input', () => {
    expect(deriveLibraryLabel('')).toBe('');
    expect(deriveLibraryLabel('   ')).toBe('');
    expect(deriveLibraryLabel('\n\t  \n')).toBe('');
  });

  it('keeps up to 4 words', () => {
    expect(deriveLibraryLabel('one two three four five six')).toBe('one two three four');
  });

  it('passes through under-4-word input unchanged', () => {
    expect(deriveLibraryLabel('hello world')).toBe('hello world');
    expect(deriveLibraryLabel('single')).toBe('single');
  });

  it('caps at 32 chars (even if the 4 words exceed it)', () => {
    // 4 words but very long — should cap mid-word.
    const out = deriveLibraryLabel('abcdefghij klmnopqrst uvwxyzabcd efghijkl');
    expect(out.length).toBeLessThanOrEqual(32);
  });

  it('collapses extra whitespace between words', () => {
    expect(deriveLibraryLabel('  hello   world   ')).toBe('hello world');
  });

  it('handles multibyte content without splitting characters', () => {
    // No JS string slicing surprise here — slice() is codepoint-aware
    // on BMP chars; we don't go through byte boundaries.
    expect(deriveLibraryLabel('Was bedeutet ἀγάπη wirklich heute')).toBe('Was bedeutet ἀγάπη wirklich');
  });

  it('drops empty tokens from extra whitespace splits', () => {
    // The .filter(Boolean) guard pins this: "  a    b  " mustn't
    // produce ["", "a", "", "", "b"] which would slice 4 → ['', 'a', '', '']
    // and collapse to "a".
    expect(deriveLibraryLabel('  a    b  ')).toBe('a b');
  });
});
