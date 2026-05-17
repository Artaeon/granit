import type { CalendarEvent } from '$lib/api';
import { fmtDateISO } from '$lib/util/date';

// Re-exported so calendar files that already import via `./utils`
// keep working without a touch-everything refactor.
export { fmtDateISO };

export function startOfWeek(d: Date): Date {
  const x = new Date(d);
  x.setDate(d.getDate() - d.getDay());
  x.setHours(0, 0, 0, 0);
  return x;
}

export function endOfWeek(d: Date): Date {
  const s = startOfWeek(d);
  s.setDate(s.getDate() + 6);
  s.setHours(23, 59, 59, 999);
  return s;
}

export function startOfMonth(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), 1);
}

export function endOfMonth(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth() + 1, 0);
}

export function addDays(d: Date, n: number): Date {
  const x = new Date(d);
  x.setDate(d.getDate() + n);
  return x;
}

export function isSameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

export function fmtTime(d: Date): string {
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });
}

export function eventStartDate(ev: CalendarEvent): Date | null {
  if (ev.start) return new Date(ev.start);
  if (ev.date) {
    const [y, m, d] = ev.date.split('-').map(Number);
    return new Date(y, m - 1, d);
  }
  return null;
}

export function eventEndDate(ev: CalendarEvent): Date | null {
  if (ev.end) return new Date(ev.end);
  if (ev.start && ev.durationMinutes) {
    return new Date(new Date(ev.start).getTime() + ev.durationMinutes * 60_000);
  }
  return null;
}

export function eventDayKey(ev: CalendarEvent): string {
  const s = eventStartDate(ev);
  return s ? fmtDateISO(s) : '';
}

export function isAllDay(ev: CalendarEvent): boolean {
  return !ev.start;
}

// "P1 critical" → text-error, etc.
export function priorityColor(p: number): string {
  if (p === 1) return 'var(--color-error)';
  if (p === 2) return 'var(--color-warning)';
  if (p === 3) return 'var(--color-info)';
  return 'var(--color-dim)';
}

// Group concurrent events into stacked columns so they render side-by-side.
export interface LaidOutEvent {
  ev: CalendarEvent;
  startMin: number; // minutes from 00:00
  endMin: number;
  col: number; // 0-indexed column within group
  groupSize: number; // total columns in this overlap group
}

export function layoutDay(events: CalendarEvent[]): LaidOutEvent[] {
  const timed = events
    .filter((e) => !!e.start)
    .map((ev) => {
      const s = eventStartDate(ev)!;
      const e = eventEndDate(ev) ?? new Date(s.getTime() + 30 * 60_000);
      return {
        ev,
        startMin: s.getHours() * 60 + s.getMinutes(),
        endMin: e.getHours() * 60 + e.getMinutes()
      };
    })
    .sort((a, b) => a.startMin - b.startMin);

  // Greedy column assignment for overlapping events
  const cols: number[] = []; // each entry = endMin of last event in that column
  const out: LaidOutEvent[] = timed.map((t) => {
    let col = -1;
    for (let i = 0; i < cols.length; i++) {
      if (cols[i] <= t.startMin) {
        col = i;
        cols[i] = t.endMin;
        break;
      }
    }
    if (col === -1) {
      col = cols.length;
      cols.push(t.endMin);
    }
    return { ...t, col, groupSize: 1 };
  });

  // Compute groupSize: cluster events whose intervals overlap and share columns
  for (let i = 0; i < out.length; i++) {
    let groupStart = i;
    let groupEnd = out[i].endMin;
    let j = i;
    while (j + 1 < out.length && out[j + 1].startMin < groupEnd) {
      j++;
      groupEnd = Math.max(groupEnd, out[j].endMin);
    }
    const maxCol = Math.max(...out.slice(groupStart, j + 1).map((e) => e.col));
    for (let k = groupStart; k <= j; k++) out[k].groupSize = maxCol + 1;
    i = j;
  }

  return out;
}

// Theme-aware event colors. Solid-fill events with white text + a
// stronger same-hue left border. The monochrome rebuild made the
// previous 18%-tint chips read as washed-out grey on both themes;
// going solid keeps each event distinct on the grid and follows the
// Apple Calendar / Google Calendar visual model the user expects.
//
// For semantic categories we still route through the theme tokens
// (--color-error etc.) so deadlines / tasks pick up theme changes.
// For freeform user/ICS events we use a curated 12-hue palette of
// Apple system colors so the calendar reads as colorful regardless
// of the otherwise-monochrome surface palette.
export function eventTypeColor(ev: CalendarEvent): { bg: string; fg: string; border: string } {
  const tone = (token: string) => ({
    bg: `var(--color-${token})`,
    fg: '#ffffff',
    border: `var(--color-${token})`
  });
  const hex = (h: string) => ({ bg: h, fg: '#ffffff', border: h });

  // Deadlines: color by importance — critical (red), high (yellow),
  // normal (purple). Highest specificity, evaluated FIRST so a future
  // user-color field can never accidentally overwrite the
  // miss-this-and-suffer signal.
  if (ev.type === 'deadline') {
    switch (ev.importance) {
      case 'critical':
        return tone('error');
      case 'high':
        return tone('warning');
      default:
        return tone('secondary');
    }
  }

  // ICS events: color by source filename FIRST so faith.ics, training.ics,
  // work.ics get distinct hues on the grid. Hash → index into the
  // sourcePalette so the same file always lands on the same tone (a
  // user dropping new .ics files in the vault gets a stable color
  // without manual setup). This must run BEFORE the named-color check
  // because the server hard-codes Color="cyan" on every ICS event as a
  // legacy default — without this ordering every ICS event would land
  // on the same cyan tone and the per-source palette would be dead code.
  if (ev.type === 'ics_event' && ev.source) {
    return hex(EVENT_HUES[hashStr(ev.source) % EVENT_HUES.length]);
  }

  // Granit's events.json `color` field — map common names to the
  // curated event-hue palette. Explicit user choice; honored before
  // the hash-by-title fallback.
  const named: Record<string, string> = {
    red: '#ff3b30',
    orange: '#ff9500',
    yellow: '#ffcc00',
    green: '#34c759',
    mint: '#00c7be',
    teal: '#5ac8fa',
    blue: '#007aff',
    indigo: '#5856d6',
    purple: '#af52de',
    pink: '#ff2d55',
    brown: '#a2845e',
    gray: '#8e8e93'
  };
  if (ev.type === 'event' && ev.color && named[ev.color]) {
    return hex(named[ev.color]);
  }

  // Untouched user events fall here. Hash by title (or eventId when
  // present) into the 12-hue palette so a fresh calendar with five
  // drag-created events shows five distinct colors for free.
  if (ev.type === 'event') {
    const seed = ev.title || ev.eventId || '';
    return hex(EVENT_HUES[hashStr(seed) % EVENT_HUES.length]);
  }

  switch (ev.type) {
    case 'ics_event':
      return tone('info');
    case 'task_scheduled':
      return tone('primary');
    case 'task_due':
      return ev.done ? tone('success') : tone('warning');
    case 'daily':
      return tone('secondary');
    case 'goal_target':
      // Mauve mirrors GoalsProgressWidget's primary tint — same
      // visual identity for "this is a goal" across dashboard and
      // calendar.
      return hex('#af52de');
    default:
      return tone('subtext');
  }
}

// EVENT_HUES is the curated 12-hue rotation used for ICS sources and
// untouched user events. Apple system colors — high contrast against
// white text, distinct from each other at small sizes (calendar grid
// cells), and recognisable across both light and dark themes since
// they don't track the page palette.
//
// Order is hand-tuned: adjacent indices land on different visual
// families (red→orange→yellow→green→teal→blue→…) so a hash-by-name
// rarely puts two-adjacent-in-the-week events on visually close hues.
const EVENT_HUES = [
  '#ff3b30', // red
  '#ff9500', // orange
  '#ffcc00', // yellow
  '#34c759', // green
  '#00c7be', // mint
  '#5ac8fa', // teal
  '#007aff', // blue
  '#5856d6', // indigo
  '#af52de', // purple
  '#ff2d55', // pink
  '#a2845e', // brown
  '#8e8e93'  // gray
] as const;

// Legacy palette retained for the sourceColorToken() export below —
// the source-list legend in the sidebar maps to a single token name.
const sourcePalette = [
  'info',
  'success',
  'warning',
  'primary',
  'secondary',
  'accent',
  'error',
  'subtext'
] as const;

// sourceColorToken returns the palette token a given ICS source maps
// to. Exposed so the source-list legend in the calendar sidebar can
// match the on-grid event color exactly — same input always lands on
// the same token.
export function sourceColorToken(source: string): string {
  if (!source) return 'info';
  return sourcePalette[hashStr(source) % sourcePalette.length];
}

// hashStr is a tiny FNV-1a hash. Deterministic + sufficient distribution
// across the small palette; we don't need cryptographic strength here,
// just "same input always → same output".
function hashStr(s: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h;
}
