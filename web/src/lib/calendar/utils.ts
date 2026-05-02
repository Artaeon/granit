import type { CalendarEvent } from '$lib/api';

export function fmtDateISO(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

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

// Theme-aware event colors. Uses CSS custom properties + color-mix() so the
// foreground stays readable in both light and dark themes.
//
// We map to the existing palette tokens (--color-error, --color-warning, ...)
// instead of hardcoded hexes — the dark/light palettes both define them, so
// switching themes recolors automatically and chips never become
// invisible-on-bg.
export function eventTypeColor(ev: CalendarEvent): { bg: string; fg: string; border: string } {
  const tone = (token: string) => ({
    bg: `color-mix(in srgb, var(--color-${token}) 18%, transparent)`,
    fg: `var(--color-${token})`,
    border: `color-mix(in srgb, var(--color-${token}) 65%, transparent)`
  });

  // Granit's events.json `color` field — map names → palette tokens.
  // Explicit user choice; honored before any auto-coloring rule.
  const named: Record<string, string> = {
    red: 'error',
    yellow: 'warning',
    orange: 'accent',
    green: 'success',
    blue: 'secondary',
    purple: 'primary',
    cyan: 'info'
  };
  if ((ev.type === 'event' || ev.type === 'ics_event') && ev.color && named[ev.color]) {
    return tone(named[ev.color]);
  }

  // ICS events: color by source filename so faith.ics, training.ics,
  // work.ics get distinct hues on the grid. Hash → index into the
  // sourcePalette so the same file always lands on the same tone (a
  // user dropping new .ics files in the vault gets a stable color
  // without manual setup). Falls back to the type-default when the
  // server didn't provide a source (older binaries, events.json).
  if (ev.type === 'ics_event' && ev.source) {
    return tone(sourcePalette[hashStr(ev.source) % sourcePalette.length]);
  }

  switch (ev.type) {
    case 'event':
      return tone('info');
    case 'ics_event':
      return tone('info');
    case 'task_scheduled':
      return tone('primary');
    case 'task_due':
      return ev.done ? tone('success') : tone('warning');
    case 'daily':
      return tone('secondary');
    default:
      return tone('subtext');
  }
}

// sourcePalette is the per-ICS-source rotation. Eight tones picked so
// no two adjacent indices clash visually. Order matters less than
// the count — bigger palette = more distinguishable sources, with
// diminishing returns past ~8 (humans don't reliably distinguish
// 12+ pastels at a glance).
const sourcePalette = [
  'info',       // teal/cyan
  'success',    // green
  'warning',    // yellow
  'primary',    // mauve/purple
  'secondary',  // blue
  'accent',     // peach/orange
  'error',      // red/pink
  'subtext'     // muted grey — fallback "other"
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
