import { describe, expect, it } from 'vitest';
import { emptyDayState, type DayState, type DayMode } from './dayState';
import { nextAction } from './nextAction';

// A day where the user has checked in but done nothing yet.
function fresh(overrides: Partial<DayState> = {}): DayState {
  return { ...emptyDayState('2026-05-19'), mode: 'normal', ...overrides };
}

// HH:MM helper — same local-time math the engine itself uses.
function at(hhmm: string): Date {
  const [h, m] = hhmm.split(':').map((x) => parseInt(x, 10));
  const d = new Date(2026, 4, 19, h, m, 0, 0);
  return d;
}

describe('nextAction — eat-first rule', () => {
  it('returns "iss zuerst" once the eat-nag time has passed and food is open', () => {
    const out = nextAction(fresh(), { now: at('10:30') });
    expect(out.pillar).toBe('food');
    expect(out.label).toMatch(/iss/i);
  });

  it('does not nag before the configured eat-nag time', () => {
    const out = nextAction(fresh(), { now: at('07:30') });
    expect(out.pillar).not.toBe('food');
  });

  it('asks immediately in emergency mode regardless of time', () => {
    const out = nextAction(fresh({ mode: 'emergency' }), { now: at('06:00') });
    expect(out.pillar).toBe('food');
    expect(out.reason).toMatch(/notfall/i);
  });

  it('skips the eat rule once the food pillar is marked done', () => {
    const out = nextAction(
      fresh({ eaten: true, pillars: { ...emptyDayState('x').pillars, food: { done: true } } }),
      { now: at('11:00') }
    );
    expect(out.pillar).not.toBe('food');
  });
});

describe('nextAction — evening shutdown', () => {
  it('takes over past the configured evening time when evening pillar is open', () => {
    const out = nextAction(fresh({ eaten: true }), { now: at('21:00') });
    expect(out.pillar).toBe('evening');
    expect(out.label).toMatch(/shutdown/i);
  });

  it('does NOT trigger before the evening time, even if work is done', () => {
    const out = nextAction(
      fresh({
        eaten: true,
        pillars: { ...emptyDayState('x').pillars, work: { done: true }, body: { done: true }, spirit: { done: true } }
      }),
      { now: at('19:30') }
    );
    expect(out.pillar).not.toBe('evening');
  });

  it('honours a custom evening start time', () => {
    const out = nextAction(fresh({ eaten: true }), {
      now: at('19:00'),
      eveningStartsAt: '19:00'
    });
    expect(out.pillar).toBe('evening');
  });

  it('falls through to rest when evening is already done', () => {
    const out = nextAction(
      fresh({
        eaten: true,
        pillars: {
          spirit: { done: true },
          food: { done: true },
          work: { done: true },
          body: { done: true },
          evening: { done: true }
        }
      }),
      { now: at('21:00') }
    );
    expect(out.pillar).toBe('rest');
  });
});

describe('nextAction — MIT focus block', () => {
  it('proposes a 45-minute block on a normal day', () => {
    const out = nextAction(
      fresh({ eaten: true, mit: 'Kundenprojekt fertigstellen' }),
      { now: at('14:00') }
    );
    expect(out.pillar).toBe('work');
    expect(out.label).toMatch(/45/);
    expect(out.label).toMatch(/Kundenprojekt/);
  });

  it('drops the focus block to 25 minutes on a chaotic day', () => {
    const out = nextAction(
      fresh({ mode: 'chaotic', eaten: true, mit: 'Mail an Kunden' }),
      { now: at('14:00') }
    );
    expect(out.label).toMatch(/25/);
  });

  it('skips the MIT branch when work is already marked done', () => {
    const out = nextAction(
      fresh({
        eaten: true,
        mit: 'noch was',
        pillars: { ...emptyDayState('x').pillars, work: { done: true } }
      }),
      { now: at('14:00') }
    );
    expect(out.pillar).not.toBe('work');
  });

  it('falls through to body when MIT is blank', () => {
    const out = nextAction(fresh({ eaten: true, mit: '   ' }), { now: at('14:00') });
    expect(out.pillar).toBe('body');
  });

  it('skips the work focus block entirely on Sabbath', () => {
    const out = nextAction(
      fresh({ eaten: true, mit: 'Kundenprojekt' }),
      { now: at('14:00'), sabbath: true }
    );
    expect(out.pillar).not.toBe('work');
  });
});

describe('nextAction — body fatigue branch', () => {
  it('suggests 10 minutes movement when fatigue is low', () => {
    const out = nextAction(fresh({ eaten: true, fatigue: 2 }), { now: at('15:00') });
    expect(out.pillar).toBe('body');
    expect(out.label).toMatch(/10 Minuten/);
  });

  it('suggests a walk instead when fatigue is high', () => {
    const out = nextAction(fresh({ eaten: true, fatigue: 5 }), { now: at('15:00') });
    expect(out.pillar).toBe('body');
    expect(out.label).toMatch(/Spaziergang/);
  });

  it('skips body entirely in emergency mode', () => {
    const out = nextAction(fresh({ mode: 'emergency', eaten: true }), { now: at('15:00') });
    expect(out.pillar).not.toBe('body');
  });

  it('relaxes the bar after 18:00 — no gym pressure regardless of fatigue', () => {
    const out = nextAction(fresh({ eaten: true, fatigue: 2 }), { now: at('18:30') });
    expect(out.pillar).toBe('body');
    expect(out.reason).toMatch(/Sp[äa]t/);
    expect(out.label).toMatch(/10 Minuten reichen/);
  });
});

describe('nextAction — spirit + rest', () => {
  it('asks for a prayer when only spirit is open on a normal day', () => {
    const out = nextAction(
      fresh({
        eaten: true,
        pillars: {
          spirit: { done: false },
          food: { done: true },
          work: { done: true },
          body: { done: true },
          evening: { done: false }
        }
      }),
      { now: at('16:00') }
    );
    expect(out.pillar).toBe('spirit');
  });

  it('does not nag for spirit on chaotic or emergency days', () => {
    for (const mode of ['chaotic', 'emergency'] as DayMode[]) {
      const out = nextAction(
        fresh({
          mode,
          eaten: true,
          pillars: {
            spirit: { done: false },
            food: { done: true },
            work: { done: true },
            body: { done: true },
            evening: { done: false }
          }
        }),
        { now: at('16:00') }
      );
      expect(out.pillar).not.toBe('spirit');
    }
  });

  it('returns the rest answer when every pillar is done', () => {
    const out = nextAction(
      fresh({
        eaten: true,
        pillars: {
          spirit: { done: true },
          food: { done: true },
          work: { done: true },
          body: { done: true },
          evening: { done: true }
        }
      }),
      { now: at('16:00') }
    );
    expect(out.pillar).toBe('rest');
    expect(out.label).toMatch(/Frei|Lesen|Familie/);
  });
});
