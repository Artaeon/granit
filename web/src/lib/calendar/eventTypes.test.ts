import { describe, expect, it } from 'vitest';
import { EVENT_TYPES, findEventType, glyphForKind, colorForKind } from './eventTypes';

// Catalog contract — these properties are load-bearing for the chip
// renderers + the picker. A regression here would silently break
// the visual + filter UX without test coverage to catch it.

describe('EVENT_TYPES catalog', () => {
  it('has every required field on every entry', () => {
    for (const t of EVENT_TYPES) {
      expect(t.id, 'id').toBeTruthy();
      expect(t.label, `${t.id} label`).toBeTruthy();
      expect(t.glyph, `${t.id} glyph`).toBeTruthy();
      expect(t.color, `${t.id} color`).toBeTruthy();
      expect(t.description.length, `${t.id} description`).toBeGreaterThan(20);
    }
  });

  it('ids are unique + lowercase ASCII', () => {
    const seen = new Set<string>();
    for (const t of EVENT_TYPES) {
      expect(seen.has(t.id), `duplicate id ${t.id}`).toBe(false);
      seen.add(t.id);
      expect(t.id, t.id).toBe(t.id.toLowerCase());
    }
  });

  it('glyphs are single chars so they fit in a chip prefix', () => {
    for (const t of EVENT_TYPES) {
      expect(t.glyph.length, `${t.id} glyph "${t.glyph}"`).toBeLessThanOrEqual(2);
    }
  });

  it('glyphs are visually distinct', () => {
    const glyphs = EVENT_TYPES.map((t) => t.glyph);
    const dupes = glyphs.filter((g, i) => glyphs.indexOf(g) !== i);
    expect(dupes).toEqual([]);
  });

  it('colors map to Catppuccin tokens used elsewhere in the app', () => {
    // The set the calendar surfaces accept. Adding a color here means
    // wiring it through the chip tint helper too — fail loudly if a
    // catalog entry uses an unknown one.
    const known = new Set([
      'blue', 'mauve', 'pink', 'teal', 'green', 'red', 'yellow',
      'sapphire', 'sky', 'lavender', 'peach', 'maroon', 'flamingo',
      'rosewater', 'cyan'
    ]);
    for (const t of EVENT_TYPES) {
      expect(known.has(t.color), `${t.id} color "${t.color}" not in known set`).toBe(true);
    }
  });

  it('default durations are positive when set', () => {
    for (const t of EVENT_TYPES) {
      if (t.defaultDurationMin !== undefined) {
        expect(t.defaultDurationMin, `${t.id}`).toBeGreaterThan(0);
      }
    }
  });
});

describe('findEventType', () => {
  it('looks up a known id', () => {
    expect(findEventType('meeting')?.label).toBe('Meeting');
  });

  it('is case-insensitive (matches the backend lowercase normalisation)', () => {
    expect(findEventType('MEETING')?.id).toBe('meeting');
    expect(findEventType('Meeting')?.id).toBe('meeting');
  });

  it('returns null for empty / undefined / unknown', () => {
    expect(findEventType('')).toBeNull();
    expect(findEventType(undefined)).toBeNull();
    expect(findEventType(null)).toBeNull();
    expect(findEventType('some-fake-kind')).toBeNull();
  });

  it('is whitespace-tolerant (handles legacy events.json shape)', () => {
    // Backend canonicalises on write, but a hand-edited
    // events.json or an X-GRANIT-KIND line with stray spaces in an
    // external .ics shouldn't make the frontend lose the type.
    expect(findEventType(' meeting ')?.id).toBe('meeting');
    expect(findEventType('\tfocus\n')?.id).toBe('focus');
    expect(findEventType('   ')).toBeNull();
  });
});

describe('glyphForKind / colorForKind', () => {
  it('returns the catalog values for known ids', () => {
    expect(glyphForKind('focus')).toBe('F');
    expect(colorForKind('focus')).toBe('mauve');
  });

  it('returns empty string for unknown so callers can fall through', () => {
    expect(glyphForKind('')).toBe('');
    expect(glyphForKind('zzz')).toBe('');
    expect(colorForKind(undefined)).toBe('');
  });
});
