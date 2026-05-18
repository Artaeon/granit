import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { findBinding, KEYBINDINGS, matchesKey } from './registry';

// matchesKey resolves the chord string against an event. The
// non-trivial bit is 'Mod' — on Mac it matches metaKey, elsewhere
// ctrlKey. We mock navigator.platform to flip platforms.

let realNavigator: Navigator;

beforeEach(() => {
  realNavigator = globalThis.navigator;
});

afterEach(() => {
  vi.unstubAllGlobals();
  Object.defineProperty(globalThis, 'navigator', { value: realNavigator, configurable: true });
});

function stubPlatform(value: string): void {
  // jsdom navigator is read-only on some setups; redefine the prop
  // wholesale so the matchesKey lookup sees our value.
  Object.defineProperty(globalThis, 'navigator', {
    value: { platform: value, userAgent: value },
    configurable: true
  });
}

function ev(opts: Partial<KeyboardEventInit & { key: string }>): KeyboardEvent {
  return new KeyboardEvent('keydown', { key: 'a', ...opts });
}

describe('matchesKey', () => {
  it('matches a single letter case-insensitively without modifiers', () => {
    stubPlatform('Linux');
    expect(matchesKey(ev({ key: 'O' }), 'O')).toBe(true);
    expect(matchesKey(ev({ key: 'o' }), 'O')).toBe(true);
  });

  it('requires Shift when the chord includes Shift', () => {
    stubPlatform('Linux');
    expect(matchesKey(ev({ key: 'O', shiftKey: true, ctrlKey: true }), 'Mod+Shift+O')).toBe(true);
    expect(matchesKey(ev({ key: 'O', shiftKey: false, ctrlKey: true }), 'Mod+Shift+O')).toBe(false);
  });

  it('resolves Mod to ctrlKey on non-mac platforms', () => {
    stubPlatform('Linux');
    expect(matchesKey(ev({ key: 'k', ctrlKey: true }), 'Mod+K')).toBe(true);
    expect(matchesKey(ev({ key: 'k', metaKey: true }), 'Mod+K')).toBe(false);
  });

  it('resolves Mod to metaKey on mac platforms', () => {
    stubPlatform('MacIntel');
    expect(matchesKey(ev({ key: 'k', metaKey: true }), 'Mod+K')).toBe(true);
    expect(matchesKey(ev({ key: 'k', ctrlKey: true }), 'Mod+K')).toBe(false);
  });

  it('does not match a plain key when Ctrl/Meta is pressed', () => {
    stubPlatform('Linux');
    // 'O' on its own shouldn't fire while Ctrl is held — that's
    // a different chord (Ctrl+O) belonging to a different action.
    expect(matchesKey(ev({ key: 'O', ctrlKey: true }), 'O')).toBe(false);
  });

  it('matches non-letter keys by exact event.key (Escape, ?)', () => {
    stubPlatform('Linux');
    expect(matchesKey(ev({ key: 'Escape' }), 'Escape')).toBe(true);
    expect(matchesKey(ev({ key: '?', shiftKey: true }), '?')).toBe(false); // shift state checked
    expect(matchesKey(ev({ key: '?' }), '?')).toBe(true);
  });

  it('rejects Alt when not part of the chord', () => {
    stubPlatform('Linux');
    expect(matchesKey(ev({ key: 'k', ctrlKey: true, altKey: true }), 'Mod+K')).toBe(false);
  });
});

describe('KEYBINDINGS', () => {
  it('keeps every id unique', () => {
    const ids = KEYBINDINGS.map((b) => b.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('findBinding returns the matching record', () => {
    const tray = findBinding('tray-jump');
    expect(tray?.keys).toBe('Mod+Shift+O');
  });

  it('findBinding returns undefined for unknown ids', () => {
    expect(findBinding('does-not-exist')).toBeUndefined();
  });
});
