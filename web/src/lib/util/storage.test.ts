import { describe, expect, it, beforeEach } from 'vitest';
import {
  loadStored,
  saveStored,
  loadStoredString,
  saveStoredString
} from './storage';

// jsdom 29 / vitest 4 ship a localStorage shim that doesn't always
// expose `.clear()`. Emulate clear by iterating keys; works against
// every Storage implementation.
function clearStorage() {
  const keys: string[] = [];
  for (let i = 0; i < localStorage.length; i++) {
    const k = localStorage.key(i);
    if (k) keys.push(k);
  }
  for (const k of keys) localStorage.removeItem(k);
}
beforeEach(clearStorage);

describe('loadStored / saveStored (JSON)', () => {
  it('returns the default when key is missing', () => {
    expect(loadStored('granit.test.missing', { ok: true })).toEqual({ ok: true });
  });

  it('round-trips a JSON object', () => {
    saveStored('granit.test.obj', { a: 1, b: 'two' });
    expect(loadStored('granit.test.obj', null)).toEqual({ a: 1, b: 'two' });
  });

  it('returns the default on malformed JSON', () => {
    localStorage.setItem('granit.test.broken', '{not json');
    expect(loadStored('granit.test.broken', { fallback: true })).toEqual({ fallback: true });
  });

  it('saveStored(undefined) removes the key', () => {
    saveStored('granit.test.removeme', { x: 1 });
    expect(localStorage.getItem('granit.test.removeme')).not.toBeNull();
    saveStored('granit.test.removeme', undefined);
    expect(localStorage.getItem('granit.test.removeme')).toBeNull();
  });

  it('runs the optional validator', () => {
    saveStored('granit.test.validate', { kind: 'unknown' });
    const got = loadStored<{ kind: string }>('granit.test.validate', { kind: 'fallback' }, (v) => {
      const parsed = v as { kind?: string };
      if (parsed.kind === 'unknown') throw new Error('reject');
      return parsed as { kind: string };
    });
    expect(got).toEqual({ kind: 'fallback' });
  });

  it('quota errors are absorbed silently on save', () => {
    // Force setItem to throw — emulates a quota-exceeded error path.
    const original = localStorage.setItem.bind(localStorage);
    localStorage.setItem = () => {
      throw new DOMException('quota', 'QuotaExceededError');
    };
    expect(() => saveStored('granit.test.quota', { big: 'x'.repeat(10) })).not.toThrow();
    localStorage.setItem = original;
  });
});

describe('loadStoredString / saveStoredString', () => {
  it('returns default for missing key', () => {
    expect(loadStoredString('granit.test.s.missing', 'fallback')).toBe('fallback');
  });

  it('round-trips a raw string (no JSON quoting)', () => {
    saveStoredString('granit.test.s.raw', 'hello world');
    // Round-trip
    expect(loadStoredString('granit.test.s.raw', '')).toBe('hello world');
    // Verifies no JSON.stringify happened — raw chars on disk.
    expect(localStorage.getItem('granit.test.s.raw')).toBe('hello world');
  });

  it('saveStoredString(undefined) removes the key', () => {
    saveStoredString('granit.test.s.removeme', 'x');
    saveStoredString('granit.test.s.removeme', undefined);
    expect(localStorage.getItem('granit.test.s.removeme')).toBeNull();
  });
});
