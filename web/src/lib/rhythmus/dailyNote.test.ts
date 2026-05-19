import { describe, expect, it } from 'vitest';
import { dailyNotePath, parseDayFrontmatter, serializeDayFrontmatter } from './dailyNote';
import { emptyDayState, type DayState } from './dayState';

describe('dailyNotePath', () => {
  it('uses the Daily/<date>.md convention QuickCaptureFab + AIOverlay already follow', () => {
    expect(dailyNotePath(new Date(2026, 4, 19))).toBe('Daily/2026-05-19.md');
  });
});

describe('parseDayFrontmatter', () => {
  it('returns the empty day when the note has no frontmatter at all', () => {
    expect(parseDayFrontmatter(undefined, '2026-05-19')).toEqual(emptyDayState('2026-05-19'));
  });

  it('reads a well-formed rhythmus block', () => {
    const out = parseDayFrontmatter(
      {
        rhythmus_mode: 'chaotic',
        rhythmus_fatigue: 4,
        rhythmus_eaten: true,
        rhythmus_mit: 'Kundenprojekt fertig',
        rhythmus_pillars: {
          spirit:  { done: true },
          food:    { done: true },
          work:    { done: false, note: 'angefangen' },
          body:    { done: false },
          evening: { done: false }
        }
      },
      '2026-05-19'
    );
    expect(out.mode).toBe('chaotic');
    expect(out.fatigue).toBe(4);
    expect(out.eaten).toBe(true);
    expect(out.mit).toBe('Kundenprojekt fertig');
    expect(out.pillars.spirit.done).toBe(true);
    expect(out.pillars.work.note).toBe('angefangen');
  });

  it('clamps fatigue outside 1..5 back to the default', () => {
    const out = parseDayFrontmatter({ rhythmus_fatigue: 7 }, '2026-05-19');
    expect(out.fatigue).toBe(3);
  });

  it('treats an unknown mode string as "not checked in yet"', () => {
    const out = parseDayFrontmatter({ rhythmus_mode: 'turbo' }, '2026-05-19');
    expect(out.mode).toBeNull();
  });

  it('survives malformed pillar nodes without throwing', () => {
    const out = parseDayFrontmatter(
      { rhythmus_pillars: { spirit: 'oops', food: null, work: { done: 'yes' } } },
      '2026-05-19'
    );
    expect(out.pillars.spirit.done).toBe(false);
    expect(out.pillars.food.done).toBe(false);
    expect(out.pillars.work.done).toBe(true); // truthy string coerces to done
  });
});

describe('serializeDayFrontmatter', () => {
  it('writes the rhythmus_ block alongside whatever else was already there', () => {
    const state: DayState = {
      ...emptyDayState('2026-05-19'),
      mode: 'normal',
      fatigue: 2,
      eaten: true,
      mit: 'Hero copy schreiben',
      pillars: {
        spirit:  { done: true },
        food:    { done: true },
        work:    { done: false },
        body:    { done: false },
        evening: { done: false }
      }
    };
    const out = serializeDayFrontmatter(state, { weather: 'sunny', win: 'first coffee' });
    expect(out.weather).toBe('sunny'); // user's hand-written field survives
    expect(out.win).toBe('first coffee');
    expect(out.rhythmus_mode).toBe('normal');
    expect(out.rhythmus_fatigue).toBe(2);
    expect(out.rhythmus_eaten).toBe(true);
    expect(out.rhythmus_mit).toBe('Hero copy schreiben');
    expect(out.rhythmus_pillars).toEqual({
      spirit:  { done: true },
      food:    { done: true },
      work:    { done: false },
      body:    { done: false },
      evening: { done: false }
    });
  });

  it('omits a null mode + an empty MIT instead of writing placeholders', () => {
    const state: DayState = emptyDayState('2026-05-19');
    const out = serializeDayFrontmatter(state);
    expect(out.rhythmus_mode).toBeUndefined();
    expect(out.rhythmus_mit).toBeUndefined();
  });

  it('round-trips a state through parse -> serialize without drift', () => {
    const state: DayState = {
      date: '2026-05-19',
      mode: 'emergency',
      fatigue: 5,
      eaten: false,
      mit: '',
      pillars: {
        spirit:  { done: false, note: 'too tired' },
        food:    { done: false },
        work:    { done: false },
        body:    { done: false },
        evening: { done: false }
      },
      shutdown: { achieved: 'fed myself', tomorrow: 'breathe', letGo: 'inbox', phoneAway: true }
    };
    const fm = serializeDayFrontmatter(state);
    const back = parseDayFrontmatter(fm, '2026-05-19');
    expect(back).toEqual(state);
  });

  it('omits shutdown when every field is empty', () => {
    const out = serializeDayFrontmatter(emptyDayState('2026-05-19'));
    expect(out.rhythmus_shutdown).toBeUndefined();
  });

  it('reads back a shutdown block when present', () => {
    const out = parseDayFrontmatter(
      { rhythmus_shutdown: { achieved: 'hero copy', phoneAway: true } },
      '2026-05-19'
    );
    expect(out.shutdown.achieved).toBe('hero copy');
    expect(out.shutdown.phoneAway).toBe(true);
    expect(out.shutdown.tomorrow).toBe('');
  });
});
