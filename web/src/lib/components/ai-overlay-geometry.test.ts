import { describe, it, expect, beforeEach } from 'vitest';
import {
  PANEL_WIDTH_DEFAULT,
  PANEL_WIDTH_MIN,
  PANEL_WIDTH_MAX,
  PANEL_WIDTH_KEY,
  SHEET_SNAP_KEY,
  clampPanelWidth,
  loadPanelWidth,
  persistPanelWidth,
  nextPanelWidthForKey,
  loadSheetSnap,
  snapHeightPx,
  clampSheetHeight,
  nearestSnap
} from './ai-overlay-geometry';

// localStorage is jsdom-provided in vitest; clear between tests so a
// previous test's persisted width doesn't leak.
beforeEach(() => {
  localStorage.clear();
});

describe('clampPanelWidth', () => {
  it('returns value unchanged inside the range', () => {
    expect(clampPanelWidth(500)).toBe(500);
  });
  it('clamps below min', () => {
    expect(clampPanelWidth(50)).toBe(PANEL_WIDTH_MIN);
  });
  it('clamps above max', () => {
    expect(clampPanelWidth(10000)).toBe(PANEL_WIDTH_MAX);
  });
  it('exact min and max pass through', () => {
    expect(clampPanelWidth(PANEL_WIDTH_MIN)).toBe(PANEL_WIDTH_MIN);
    expect(clampPanelWidth(PANEL_WIDTH_MAX)).toBe(PANEL_WIDTH_MAX);
  });
});

describe('loadPanelWidth', () => {
  it('returns default when nothing stored', () => {
    expect(loadPanelWidth()).toBe(PANEL_WIDTH_DEFAULT);
  });
  it('returns default for non-numeric stored value', () => {
    localStorage.setItem(PANEL_WIDTH_KEY, 'not-a-number');
    expect(loadPanelWidth()).toBe(PANEL_WIDTH_DEFAULT);
  });
  it('returns clamped value when out of range', () => {
    localStorage.setItem(PANEL_WIDTH_KEY, '99999');
    expect(loadPanelWidth()).toBe(PANEL_WIDTH_MAX);
  });
  it('round-trips through persist', () => {
    persistPanelWidth(500);
    expect(loadPanelWidth()).toBe(500);
  });
});

describe('nextPanelWidthForKey', () => {
  // Inverted axis is the easy-to-flip-the-sign bit; pin both directions.
  it('ArrowLeft widens (panel is right-anchored)', () => {
    expect(nextPanelWidthForKey(400, 'ArrowLeft')).toBe(416);
  });
  it('ArrowRight narrows', () => {
    expect(nextPanelWidthForKey(400, 'ArrowRight')).toBe(384);
  });
  it('Home jumps to max', () => {
    expect(nextPanelWidthForKey(400, 'Home')).toBe(PANEL_WIDTH_MAX);
  });
  it('End jumps to min', () => {
    expect(nextPanelWidthForKey(400, 'End')).toBe(PANEL_WIDTH_MIN);
  });
  it('returns null for non-resize keys (caller can fall through)', () => {
    expect(nextPanelWidthForKey(400, 'Escape')).toBeNull();
    expect(nextPanelWidthForKey(400, 'a')).toBeNull();
  });
  it('clamps when widening past max', () => {
    expect(nextPanelWidthForKey(PANEL_WIDTH_MAX, 'ArrowLeft')).toBe(PANEL_WIDTH_MAX);
  });
  it('clamps when narrowing past min', () => {
    expect(nextPanelWidthForKey(PANEL_WIDTH_MIN, 'ArrowRight')).toBe(PANEL_WIDTH_MIN);
  });
});

describe('loadSheetSnap', () => {
  it('returns mid when nothing stored', () => {
    expect(loadSheetSnap()).toBe('mid');
  });
  it('returns stored peek / mid / full', () => {
    localStorage.setItem(SHEET_SNAP_KEY, 'peek');
    expect(loadSheetSnap()).toBe('peek');
    localStorage.setItem(SHEET_SNAP_KEY, 'full');
    expect(loadSheetSnap()).toBe('full');
  });
  it('falls back to mid for any other string', () => {
    localStorage.setItem(SHEET_SNAP_KEY, 'half-open');
    expect(loadSheetSnap()).toBe('mid');
  });
});

describe('snapHeightPx', () => {
  it('peek is 35% of viewport', () => {
    expect(snapHeightPx('peek', 1000)).toBe(350);
  });
  it('mid is 65% of viewport', () => {
    expect(snapHeightPx('mid', 1000)).toBe(650);
  });
  it('full is 92% of viewport', () => {
    expect(snapHeightPx('full', 1000)).toBe(920);
  });
  it('rounds to integer pixels', () => {
    // 35% of 333 = 116.55 → 117
    expect(snapHeightPx('peek', 333)).toBe(117);
  });
});

describe('clampSheetHeight', () => {
  it('passes through values inside range', () => {
    // 1000 * 0.18 = 180 min, 1000 * 0.95 = 950 max
    expect(clampSheetHeight(500, 1000)).toBe(500);
  });
  it('clamps below min', () => {
    expect(clampSheetHeight(50, 1000)).toBe(180);
  });
  it('clamps above max', () => {
    expect(clampSheetHeight(2000, 1000)).toBe(950);
  });
});

describe('nearestSnap', () => {
  // 400/1000 = 40% — closer to peek (35) than mid (65). Diff 5 vs 25.
  it('picks peek when ratio is 40%', () => {
    expect(nearestSnap(400, 1000)).toBe('peek');
  });
  // 600/1000 = 60% — closer to mid (65) than peek (35). Diff 5 vs 25.
  it('picks mid when ratio is 60%', () => {
    expect(nearestSnap(600, 1000)).toBe('mid');
  });
  // 900/1000 = 90% — closer to full (92) than mid (65). Diff 2 vs 25.
  it('picks full when ratio is 90%', () => {
    expect(nearestSnap(900, 1000)).toBe('full');
  });
  it('zero viewport defaults to mid (degenerate guard)', () => {
    expect(nearestSnap(500, 0)).toBe('mid');
  });
  // Tie at 50% (peek-mid midpoint). Order in the for-loop is peek, mid,
  // full — first hit wins. The iteration uses `< bestDist` so an
  // equal distance does NOT replace the incumbent. 50% lands 15 from
  // peek (35) and 15 from mid (65) — peek comes first, peek wins.
  it('breaks ties toward the earlier-iterated snap (peek over mid)', () => {
    expect(nearestSnap(500, 1000)).toBe('peek');
  });
});
