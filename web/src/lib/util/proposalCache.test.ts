import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest';
import { saveProposals, loadProposals } from './proposalCache';

// jsdom 29 / vitest 4's Storage shim doesn't always expose .clear();
// purge by iterating keys instead.
function clearStorage() {
  const keys: string[] = [];
  for (let i = 0; i < localStorage.length; i++) {
    const k = localStorage.key(i);
    if (k) keys.push(k);
  }
  for (const k of keys) localStorage.removeItem(k);
}
beforeEach(clearStorage);

afterEach(() => {
  vi.useRealTimers();
});

describe('proposalCache', () => {
  it('round-trips items', () => {
    saveProposals('granit.test.cache', [{ id: 1 }, { id: 2 }]);
    expect(loadProposals('granit.test.cache')).toEqual([{ id: 1 }, { id: 2 }]);
  });

  it('returns [] for missing key', () => {
    expect(loadProposals('granit.test.cache.missing')).toEqual([]);
  });

  it('saving an empty array removes the key', () => {
    saveProposals('granit.test.cache.empty', [{ id: 1 }]);
    saveProposals('granit.test.cache.empty', []);
    expect(localStorage.getItem('granit.test.cache.empty')).toBeNull();
  });

  it('treats entries older than 24h as missing and clears them', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00Z'));
    saveProposals('granit.test.cache.ttl', [{ id: 1 }]);
    // Advance 25 hours — past the TTL.
    vi.setSystemTime(new Date('2026-01-02T01:00:00Z'));
    expect(loadProposals('granit.test.cache.ttl')).toEqual([]);
    // Stale entry should also have been cleaned during the read.
    expect(localStorage.getItem('granit.test.cache.ttl')).toBeNull();
  });

  it('keeps entries that are still inside the TTL', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00Z'));
    saveProposals('granit.test.cache.fresh', [{ id: 1 }]);
    vi.setSystemTime(new Date('2026-01-01T20:00:00Z')); // +20h
    expect(loadProposals('granit.test.cache.fresh')).toEqual([{ id: 1 }]);
  });

  it('returns [] on malformed cache shape', () => {
    localStorage.setItem('granit.test.cache.bad', JSON.stringify({ no: 'envelope' }));
    expect(loadProposals('granit.test.cache.bad')).toEqual([]);
  });
});
