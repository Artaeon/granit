// Natural-language event parser. Powers the quick-create bar at
// the top of /calendar — single text input → CalendarEventEntry
// shape. Deterministic regex parsing, no LLM call: limited but
// fast and reliable for the common cases.
//
// Recognised patterns (any order, case-insensitive):
//   date  : today | tomorrow | yesterday
//           mon|tue|wed|thu|fri|sat|sun  (resolves to next match)
//           monday … sunday
//           YYYY-MM-DD
//   time  : HH:MM | HHh | HHam | HHpm | H:MMam | H:MMpm
//   range : "<time> to <time>"  |  "<time>-<time>"
//   dur   : <N>h | <N>min | <N>m | <N>h<M>m  (only when end time absent)
//
// Anything left over after stripping dates/times becomes the title.
// If no date is given we default to today; if no time, we leave
// start/end blank (caller treats as all-day).

export interface ParsedEvent {
  title: string;
  date: string; // YYYY-MM-DD
  startTime: string; // HH:MM or ''
  endTime: string;   // HH:MM or ''
}

export interface ParseResult {
  ok: boolean;
  event?: ParsedEvent;
  /** Hints to show under the input — what we recognised, what's missing. */
  hint?: string;
}

const DOW: Record<string, number> = {
  sun: 0, sunday: 0,
  mon: 1, monday: 1,
  tue: 2, tues: 2, tuesday: 2,
  wed: 3, weds: 3, wednesday: 3,
  thu: 4, thurs: 4, thursday: 4,
  fri: 5, friday: 5,
  sat: 6, saturday: 6
};

function pad2(n: number): string {
  return n < 10 ? `0${n}` : `${n}`;
}
function fmtDate(d: Date): string {
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
}

/** Resolve a day-of-week token to the next occurrence ≥ today. */
function nextDow(ref: Date, dow: number): Date {
  const t = new Date(ref);
  t.setHours(0, 0, 0, 0);
  const diff = (dow - t.getDay() + 7) % 7;
  // If today matches, keep today (user typed "fri" on a Friday → today).
  t.setDate(t.getDate() + diff);
  return t;
}

/** Normalise a time fragment into HH:MM (24-hour) or '' on parse failure. */
function normaliseTime(raw: string): string {
  const s = raw.trim().toLowerCase();
  // 14:30 / 9:00
  const m1 = /^(\d{1,2}):(\d{2})(am|pm)?$/.exec(s);
  if (m1) {
    let h = parseInt(m1[1], 10);
    const min = parseInt(m1[2], 10);
    if (h > 23 || min > 59) return '';
    if (m1[3] === 'pm' && h < 12) h += 12;
    if (m1[3] === 'am' && h === 12) h = 0;
    return `${pad2(h)}:${pad2(min)}`;
  }
  // 14h / 9h
  const m2 = /^(\d{1,2})h$/.exec(s);
  if (m2) {
    const h = parseInt(m2[1], 10);
    if (h > 23) return '';
    return `${pad2(h)}:00`;
  }
  // 14 / 9 (only when context made it a time — caller decides)
  const m3 = /^(\d{1,2})$/.exec(s);
  if (m3) {
    const h = parseInt(m3[1], 10);
    if (h > 23) return '';
    return `${pad2(h)}:00`;
  }
  // 9am / 12pm / 9:30am / 12:00pm
  const m4 = /^(\d{1,2})(?::(\d{2}))?(am|pm)$/.exec(s);
  if (m4) {
    let h = parseInt(m4[1], 10);
    const min = m4[2] ? parseInt(m4[2], 10) : 0;
    if (h > 12 || min > 59) return '';
    if (m4[3] === 'pm' && h < 12) h += 12;
    if (m4[3] === 'am' && h === 12) h = 0;
    return `${pad2(h)}:${pad2(min)}`;
  }
  return '';
}

/** Add a duration like "1h" / "30m" / "1h30m" to an HH:MM time. */
function addDuration(start: string, dur: string): string {
  const m = /^(?:(\d+)h)?(?:(\d+)(?:m|min))?$/.exec(dur.trim().toLowerCase());
  if (!m || (!m[1] && !m[2])) return '';
  const h = m[1] ? parseInt(m[1], 10) : 0;
  const min = m[2] ? parseInt(m[2], 10) : 0;
  const [sh, sm] = start.split(':').map((x) => parseInt(x, 10));
  let total = sh * 60 + sm + h * 60 + min;
  if (total >= 24 * 60) total = 24 * 60 - 1; // clamp to same-day
  return `${pad2(Math.floor(total / 60))}:${pad2(total % 60)}`;
}

/**
 * Parse a natural-language event description.
 *
 * @param input  user text, e.g. "lunch with raphael tomorrow 12pm 1h"
 * @param refDate reference date for "today/tomorrow/<dow>" — usually new Date()
 */
export function parseEventInput(input: string, refDate: Date = new Date()): ParseResult {
  const original = input.trim();
  if (!original) return { ok: false, hint: 'type something like "lunch tomorrow 12pm 1h"' };

  let working = ' ' + original.toLowerCase() + ' ';
  let date: Date | null = null;
  let startTime = '';
  let endTime = '';

  // ── ISO date ─────────────────────────────────────────────
  const isoRx = /\s(\d{4}-\d{2}-\d{2})\s/;
  const isoMatch = isoRx.exec(working);
  if (isoMatch) {
    const [y, m, d] = isoMatch[1].split('-').map((x) => parseInt(x, 10));
    date = new Date(y, m - 1, d);
    working = working.replace(isoRx, ' ');
  }

  // ── Keyword dates ─────────────────────────────────────────
  const todayRx = /\s(today|tonight)\s/;
  if (!date && todayRx.test(working)) {
    date = new Date(refDate);
    working = working.replace(todayRx, ' ');
  }
  const tomRx = /\stomorrow\s/;
  if (!date && tomRx.test(working)) {
    const t = new Date(refDate);
    t.setDate(t.getDate() + 1);
    date = t;
    working = working.replace(tomRx, ' ');
  }
  const yestRx = /\syesterday\s/;
  if (!date && yestRx.test(working)) {
    const t = new Date(refDate);
    t.setDate(t.getDate() - 1);
    date = t;
    working = working.replace(yestRx, ' ');
  }

  // ── Day-of-week ──────────────────────────────────────────
  if (!date) {
    for (const [name, dow] of Object.entries(DOW)) {
      // Word boundaries so "sunscreen" doesn't match "sun".
      const rx = new RegExp(`\\s${name}\\s`);
      if (rx.test(working)) {
        date = nextDow(refDate, dow);
        working = working.replace(rx, ' ');
        break;
      }
    }
  }

  if (!date) {
    date = new Date(refDate);
  }

  // ── Time range  "10:00 to 11:30" / "10am-11am" ───────────
  const rangeRx = /\s(\d{1,2}(?::\d{2})?(?:am|pm)?)\s*(?:to|-|–|—|until)\s*(\d{1,2}(?::\d{2})?(?:am|pm)?)\s/;
  const rangeMatch = rangeRx.exec(working);
  if (rangeMatch) {
    const a = normaliseTime(rangeMatch[1]);
    const b = normaliseTime(rangeMatch[2]);
    if (a && b) {
      startTime = a;
      endTime = b;
      working = working.replace(rangeRx, ' ');
    }
  }

  // ── Single time + optional duration ──────────────────────
  if (!startTime) {
    const timeRx = /\s(\d{1,2}(?::\d{2})?(?:am|pm)?|(?:\d{1,2})h)\s/;
    const m = timeRx.exec(working);
    if (m) {
      const t = normaliseTime(m[1]);
      if (t) {
        startTime = t;
        working = working.replace(timeRx, ' ');
      }
    }
  }
  if (startTime && !endTime) {
    const durRx = /\s(\d+h(?:\d+m(?:in)?)?|\d+m(?:in)?|\d+min)\s/;
    const m = durRx.exec(working);
    if (m) {
      const e = addDuration(startTime, m[1]);
      if (e) {
        endTime = e;
        working = working.replace(durRx, ' ');
      }
    }
  }

  // Title = whatever's left, original-cased. Reconstruct by
  // intersecting tokens — the working string was lower-cased so we
  // can't just trim-and-return; scan original tokens, drop any that
  // disappeared from working.
  const remaining = new Set(working.trim().split(/\s+/).filter(Boolean));
  const title = original
    .split(/\s+/)
    .filter((tok) => remaining.has(tok.toLowerCase()))
    .join(' ')
    .trim();

  if (!title) {
    return { ok: false, hint: 'add a title — e.g. "lunch tomorrow 12pm"' };
  }

  return {
    ok: true,
    event: {
      title,
      date: fmtDate(date),
      startTime,
      endTime
    }
  };
}
