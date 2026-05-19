import { describe, expect, it } from 'vitest';

// Import via a fresh module ID each test because the persistedWritable
// reads localStorage at module-init time. We test the pure helpers
// (label / minimum / visible-in) and the validate path that runs on
// whatever JSON the user has on disk.
import {
  DEFAULT_CONFIG,
  pillarLabel,
  pillarMinimumFor,
  pillarVisibleIn,
  type RhythmusConfig
} from './minima';
import { DEFAULT_PILLARS, type PillarKey } from './pillars';

describe('pillarLabel', () => {
  it('returns the user override when set', () => {
    const cfg: RhythmusConfig = { ...DEFAULT_CONFIG, labels: { body: 'Sport' } };
    expect(pillarLabel(cfg, 'body')).toBe('Sport');
  });

  it('falls back to the hardcoded default when the override is missing', () => {
    expect(pillarLabel(DEFAULT_CONFIG, 'body')).toBe(DEFAULT_PILLARS.body.label);
  });
});

describe('pillarMinimumFor', () => {
  it('returns the per-mode minimum text', () => {
    expect(pillarMinimumFor(DEFAULT_CONFIG, 'food', 'normal')).toBe('erste Mahlzeit');
    expect(pillarMinimumFor(DEFAULT_CONFIG, 'food', 'chaotic')).toBe('irgendwas essen');
    expect(pillarMinimumFor(DEFAULT_CONFIG, 'food', 'emergency')).toBe('Wasser + Brot');
  });
});

describe('pillarVisibleIn', () => {
  it('hides the work pillar in emergency mode by default', () => {
    expect(pillarVisibleIn(DEFAULT_CONFIG, 'work', 'emergency')).toBe(false);
  });

  it('shows the work pillar in normal + chaotic modes', () => {
    expect(pillarVisibleIn(DEFAULT_CONFIG, 'work', 'normal')).toBe(true);
    expect(pillarVisibleIn(DEFAULT_CONFIG, 'work', 'chaotic')).toBe(true);
  });

  it('shows every other pillar in every mode by default', () => {
    const keys: PillarKey[] = ['spirit', 'food', 'body', 'evening'];
    for (const k of keys) {
      expect(pillarVisibleIn(DEFAULT_CONFIG, k, 'normal')).toBe(true);
      expect(pillarVisibleIn(DEFAULT_CONFIG, k, 'chaotic')).toBe(true);
      expect(pillarVisibleIn(DEFAULT_CONFIG, k, 'emergency')).toBe(true);
    }
  });

  it('honours a custom hideInEmergency flag', () => {
    const cfg: RhythmusConfig = {
      ...DEFAULT_CONFIG,
      minima: {
        ...DEFAULT_CONFIG.minima,
        body: { ...DEFAULT_CONFIG.minima.body, hideInEmergency: true }
      }
    };
    expect(pillarVisibleIn(cfg, 'body', 'emergency')).toBe(false);
    expect(pillarVisibleIn(cfg, 'body', 'normal')).toBe(true);
  });
});
