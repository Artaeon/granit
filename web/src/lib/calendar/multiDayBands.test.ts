import { describe, it, expect } from 'vitest';
import type { CalendarEvent } from '$lib/api';
import { computeMultiDayBands, consumedKey } from './utils';

// Day factory — local midnight on `YYYY-MM-DD`. The bands math
// doesn't care about timezones (it works in calendar-date space
// the same way fmtDateISO does), but tests pin local dates so a
// CI box in a different zone doesn't flip the day boundary.
function day(iso: string): Date {
  const [y, m, d] = iso.split('-').map(Number);
  return new Date(y, m - 1, d);
}

// All-day event factory — title + date + optional eventId.
function allDay(title: string, date: string, opts?: { eventId?: string; source?: string }): CalendarEvent {
  return {
    type: 'event',
    title,
    date,
    eventId: opts?.eventId,
    source: opts?.source
  } as CalendarEvent;
}

// Timed event factory — has start, NOT date. Should be ignored.
function timed(title: string, start: string): CalendarEvent {
  return {
    type: 'event',
    title,
    start
  } as CalendarEvent;
}

describe('computeMultiDayBands', () => {
  const monFri = [
    day('2026-05-18'), day('2026-05-19'), day('2026-05-20'),
    day('2026-05-21'), day('2026-05-22')
  ];

  it('returns empty bands when days.length < 2', () => {
    const r = computeMultiDayBands([day('2026-05-18')], [
      allDay('Vacation', '2026-05-18', { eventId: 'v1' })
    ]);
    expect(r.bands).toEqual([]);
    expect(r.consumed.size).toBe(0);
  });

  it('returns empty bands when no events span ≥2 days', () => {
    const r = computeMultiDayBands(monFri, [
      allDay('Mon meeting', '2026-05-18', { eventId: 'm1' }),
      allDay('Wed reminder', '2026-05-20', { eventId: 'r1' })
    ]);
    expect(r.bands).toEqual([]);
    expect(r.consumed.size).toBe(0);
  });

  it('groups consecutive days of the same eventId into one band', () => {
    const r = computeMultiDayBands(monFri, [
      allDay('Vacation', '2026-05-18', { eventId: 'v1' }),
      allDay('Vacation', '2026-05-19', { eventId: 'v1' }),
      allDay('Vacation', '2026-05-20', { eventId: 'v1' })
    ]);
    expect(r.bands).toHaveLength(1);
    expect(r.bands[0]).toMatchObject({ startCol: 0, endCol: 2, lane: 0 });
    expect(r.bands[0].event.title).toBe('Vacation');
  });

  it('splits a cluster on a gap into separate bands', () => {
    // Same event id, days 0+1, then 3+4 — gap at day 2 → two bands.
    const r = computeMultiDayBands(monFri, [
      allDay('Trip', '2026-05-18', { eventId: 't1' }),
      allDay('Trip', '2026-05-19', { eventId: 't1' }),
      allDay('Trip', '2026-05-21', { eventId: 't1' }),
      allDay('Trip', '2026-05-22', { eventId: 't1' })
    ]);
    expect(r.bands).toHaveLength(2);
    expect(r.bands[0]).toMatchObject({ startCol: 0, endCol: 1 });
    expect(r.bands[1]).toMatchObject({ startCol: 3, endCol: 4 });
  });

  it('clusters by title+source when eventId is missing', () => {
    const r = computeMultiDayBands(monFri, [
      allDay('Vacation', '2026-05-18', { source: 'personal.ics' }),
      allDay('Vacation', '2026-05-19', { source: 'personal.ics' })
    ]);
    expect(r.bands).toHaveLength(1);
    expect(r.bands[0].endCol).toBe(1);
  });

  it('does NOT merge events with same title but different source', () => {
    // Two ICS calendars both have a "Vacation" on consecutive days.
    // They should stay separate.
    const r = computeMultiDayBands(monFri, [
      allDay('Vacation', '2026-05-18', { source: 'work.ics' }),
      allDay('Vacation', '2026-05-19', { source: 'personal.ics' })
    ]);
    expect(r.bands).toEqual([]); // each cluster has only 1 day
  });

  it('ignores timed events (only all-day events form bands)', () => {
    const r = computeMultiDayBands(monFri, [
      timed('Meeting Mon', '2026-05-18T09:00:00'),
      timed('Meeting Tue', '2026-05-19T09:00:00'),
      timed('Meeting Wed', '2026-05-20T09:00:00')
    ]);
    expect(r.bands).toEqual([]);
  });

  it('lane-stacks overlapping bands', () => {
    // Vacation spans Mon-Fri (all 5 days). Conference spans Wed-Thu.
    // Conference must go in lane 1 because lane 0 is occupied by
    // Vacation through Wed-Thu.
    const r = computeMultiDayBands(monFri, [
      allDay('Vacation', '2026-05-18', { eventId: 'v1' }),
      allDay('Vacation', '2026-05-19', { eventId: 'v1' }),
      allDay('Vacation', '2026-05-20', { eventId: 'v1' }),
      allDay('Vacation', '2026-05-21', { eventId: 'v1' }),
      allDay('Vacation', '2026-05-22', { eventId: 'v1' }),
      allDay('Conference', '2026-05-20', { eventId: 'c1' }),
      allDay('Conference', '2026-05-21', { eventId: 'c1' })
    ]);
    expect(r.bands).toHaveLength(2);
    const vacation = r.bands.find((b) => b.event.title === 'Vacation')!;
    const conference = r.bands.find((b) => b.event.title === 'Conference')!;
    expect(vacation.lane).toBe(0);
    expect(conference.lane).toBe(1);
  });

  it('reuses an earlier lane once a band ends', () => {
    // Mon-Tue: event A. Wed-Thu: event B (no overlap). Both should
    // get lane 0 — A ends before B starts.
    const r = computeMultiDayBands(monFri, [
      allDay('A', '2026-05-18', { eventId: 'a1' }),
      allDay('A', '2026-05-19', { eventId: 'a1' }),
      allDay('B', '2026-05-20', { eventId: 'b1' }),
      allDay('B', '2026-05-21', { eventId: 'b1' })
    ]);
    expect(r.bands).toHaveLength(2);
    for (const b of r.bands) {
      expect(b.lane).toBe(0);
    }
  });

  it('marks every spanned (clusterKey, dayKey) in consumed', () => {
    const r = computeMultiDayBands(monFri, [
      allDay('Vacation', '2026-05-18', { eventId: 'v1' }),
      allDay('Vacation', '2026-05-19', { eventId: 'v1' }),
      allDay('Vacation', '2026-05-20', { eventId: 'v1' })
    ]);
    expect(r.consumed.size).toBe(3);
    // Same idiom the caller uses for the lookup.
    const sampleEv = allDay('Vacation', '2026-05-19', { eventId: 'v1' });
    expect(r.consumed.has(consumedKey(sampleEv, '2026-05-19'))).toBe(true);
  });

  it('drops events whose date is outside the visible days[]', () => {
    // Event spans Sun-Tue. Visible days are Mon-Fri. Sun gets
    // dropped (no column); the remaining 2 days (Mon, Tue) still
    // form a band.
    const r = computeMultiDayBands(monFri, [
      allDay('Trip', '2026-05-17', { eventId: 't1' }),
      allDay('Trip', '2026-05-18', { eventId: 't1' }),
      allDay('Trip', '2026-05-19', { eventId: 't1' })
    ]);
    expect(r.bands).toHaveLength(1);
    expect(r.bands[0]).toMatchObject({ startCol: 0, endCol: 1 });
  });

  it('skips events with no date field (defensive)', () => {
    const r = computeMultiDayBands(monFri, [
      { type: 'event', title: 'noop' } as CalendarEvent,
      { type: 'event', title: 'noop' } as CalendarEvent
    ]);
    expect(r.bands).toEqual([]);
  });
});

describe('consumedKey', () => {
  it('matches the cluster-key + dayKey shape computeMultiDayBands uses', () => {
    const ev = allDay('Vacation', '2026-05-18', { eventId: 'v1' });
    expect(consumedKey(ev, '2026-05-18')).toBe('id:v1|2026-05-18');
  });

  it('falls back to title+source when no eventId', () => {
    const ev = allDay('Vacation', '2026-05-18', { source: 'work.ics' });
    expect(consumedKey(ev, '2026-05-18')).toBe('tit:Vacation|src:work.ics|2026-05-18');
  });

  it('treats undefined source as empty string', () => {
    const ev = allDay('Vacation', '2026-05-18');
    expect(consumedKey(ev, '2026-05-18')).toBe('tit:Vacation|src:|2026-05-18');
  });
});
