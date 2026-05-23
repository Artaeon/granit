// Parse a single-line task input that may contain inline metadata.
// Mirrors the markers granit's TaskStore parses, so what the user types here
// is byte-for-byte the line that ends up in the daily note.
//
//   buy milk !2 due:2026-05-15 #errand
//   →  text="buy milk", priority=2, dueDate="2026-05-15", tags=["errand"]
//
// Returns the cleaned text and the extracted markers — the caller serializes
// them back to canonical form via buildTaskTextLine on the server.

export interface ParsedTask {
  text: string;
  priority: number; // 0 if absent
  dueDate: string; // '' if absent
  tags: string[];
}

const reTag = /(^|\s)#([\p{L}\p{N}_/-]+)/gu;
const rePriority = /(^|\s)!([1-3])(\s|$)/;
const reDue = /(^|\s)due:(\d{4}-\d{2}-\d{2})(\s|$)/;

// Low-ambiguity natural-language date words. Used as a gate before
// calling smartDate so the parser doesn't false-positive on common
// English/German words. Excludes:
//   - "do" / "mi" (German short for Donnerstag/Mittwoch) — collide
//     with the English verb "do" and the noun "mi"; "do laundry"
//     must not parse as Donnerstag
//   - "mo"/"di"/"fr"/"sa"/"so" — same risk; "mo" appears in "more",
//     "fr" in slugs, "so" in "so what" etc. Users wanting weekday
//     abbreviations should use the English short forms (mon/tue/...)
//     which the SET below DOES include because their English usage
//     pattern is narrower.
const NL_DATE_WORDS = new Set([
  'today', 'tomorrow', 'tmrw', 'yesterday',
  'heute', 'morgen', 'übermorgen', 'gestern',
  'mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun',
  'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday',
  'montag', 'dienstag', 'mittwoch', 'donnerstag', 'freitag', 'samstag', 'sonntag'
]);

export function parseTaskInput(raw: string): ParsedTask {
  let text = raw;
  const tags: string[] = [];
  let priority = 0;
  let dueDate = '';

  for (const m of text.matchAll(reTag)) {
    const t = m[2];
    if (!tags.includes(t)) tags.push(t);
  }
  text = text.replace(reTag, '$1');

  const pm = text.match(rePriority);
  if (pm) {
    priority = Number(pm[2]);
    text = text.replace(rePriority, '$1$3');
  }

  const dm = text.match(reDue);
  if (dm) {
    dueDate = dm[2];
    text = text.replace(reDue, '$1$3');
  }

  // Natural-language date recognition. Only runs if no explicit
  // due:YYYY-MM-DD was already captured. Scans words against
  // NL_DATE_WORDS (low-ambiguity set), takes the first hit, drops
  // the token from the title text. Examples that now parse:
  //   "review PR morgen #work"     → dueDate=tomorrow
  //   "meeting friday with j"      → dueDate=next friday
  //   "gym samstag !2"             → dueDate=next saturday, p=2
  // The "next" prefix nudges to the following week's instance,
  // mirroring smartDate's existing behaviour.
  if (dueDate === '') {
    const words = text.split(/\s+/);
    // Skip empty entries from leading/trailing whitespace.
    for (let i = 0; i < words.length; i++) {
      if (!words[i]) continue;
      // Honour an explicit "next" prefix: "next mon" / "next freitag".
      let probe = words[i].toLowerCase();
      let probeNext = false;
      if (probe === 'next' && i + 1 < words.length) {
        probeNext = true;
        probe = words[i + 1].toLowerCase();
      }
      if (!NL_DATE_WORDS.has(probe)) continue;
      const resolved = smartDate(probeNext ? `next ${probe}` : probe);
      if (resolved) {
        dueDate = resolved;
        if (probeNext) {
          // Drop both "next" and the weekday word.
          words.splice(i, 2);
        } else {
          words.splice(i, 1);
        }
        text = words.join(' ');
        break;
      }
    }
  }

  text = text.trim().replace(/\s+/g, ' ');
  return { text, priority, dueDate, tags };
}

// Strip every inline-property marker the backend parser knows
// about, so the rendered task title shows just the human-readable
// text. The markers themselves stay in the underlying `task.text`
// (the authoritative on-disk markdown) and the parsed values live
// in fields like priority / dueDate / tags — the UI shouldn't
// duplicate them in the title.
//
// User report: a task rendering as "buy milk !2 due:2026-05-07"
// instead of just "buy milk". Fix is purely cosmetic — strip on
// render only. Markers preserved in `text` so editing round-trips
// safely.
//
// Patterns mirror internal/tasks/parser.go regex-for-regex.
const STRIP_PATTERNS: RegExp[] = [
  // ASCII shorthands
  /(?:^|\s)!([1-3])(?=\s|$)/g,
  /(?:^|\s)due:\d{4}-\d{2}-\d{2}(?=\s|$)/g,
  /(?:^|\s)snooze:\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(?=\s|$)/g,
  /(?:^|\s)goal:[A-Za-z0-9_-]+(?=\s|$)/g,
  /(?:^|\s)deadline:[0-9a-z]{26}(?=\s|$)/g,
  /(?:^|\s)depends:"[^"]+"(?=\s|$)/g,
  /(?:^|\s)depends:[^\s]+(?=\s|$)/g,
  /(?:^|\s)~\d+[mh](?=\s|$)/g,
  // Emoji markers — literal glyphs so the file stays greppable.
  /\s*📅\s*\d{4}-\d{2}-\d{2}/g,
  /\s*⏰\s*\d{2}:\d{2}-\d{2}:\d{2}/g,
  /\s*🔁\s*(?:daily|weekly|monthly|3x-week)/g,
  /\s*🔺/g,
  /\s*⏫/g,
  /\s*🔼/g,
  /\s*🔽/g,
  // Tags — rendered as chips elsewhere, drop from title.
  /(?:^|\s)#[\p{L}\p{N}_/-]+/gu
];

export function cleanTaskText(raw: string): string {
  if (!raw) return '';
  let s = raw;
  for (const re of STRIP_PATTERNS) s = s.replace(re, ' ');
  return s.trim().replace(/\s+/g, ' ');
}

// English + German date shortcuts → YYYY-MM-DD.
//   today / heute              → ref
//   tomorrow / morgen / tmrw   → ref + 1
//   übermorgen                 → ref + 2
//   yesterday / gestern        → ref - 1
//   mon / monday / montag …    → next occurrence of that weekday
//   next mon / next freitag …  → following week's instance
// Bare weekday returns the SAME day this week if today matches; the
// `next ` prefix always pushes to the next week. Bare weekdays in
// the past relative to today roll forward to this coming week.
export function smartDate(token: string, ref = new Date()): string | null {
  const t = token.toLowerCase().trim();
  const day = (n: number) => {
    const d = new Date(ref);
    d.setDate(d.getDate() + n);
    return iso(d);
  };
  // Single-word shortcuts.
  if (t === 'today' || t === 'heute') return iso(ref);
  if (t === 'tomorrow' || t === 'tmrw' || t === 'morgen') return day(1);
  if (t === 'übermorgen') return day(2);
  if (t === 'yesterday' || t === 'gestern') return day(-1);
  // Weekday lookup: English (short + full) and German full. German
  // 2-letter abbreviations (mo/di/mi/do/fr/sa/so) are intentionally
  // excluded — they collide with too many ordinary German words. See
  // NL_DATE_WORDS comment.
  const wdMap: Record<string, number> = {
    sun: 0, sunday: 0, sonntag: 0,
    mon: 1, monday: 1, montag: 1,
    tue: 2, tuesday: 2, dienstag: 2,
    wed: 3, wednesday: 3, mittwoch: 3,
    thu: 4, thursday: 4, donnerstag: 4,
    fri: 5, friday: 5, freitag: 5,
    sat: 6, saturday: 6, samstag: 6
  };
  const next = t.startsWith('next ');
  const word = next ? t.slice(5) : t;
  const idx = wdMap[word];
  if (idx !== undefined) {
    const cur = ref.getDay();
    let delta = (idx - cur + 7) % 7;
    if (delta === 0 || next) delta += 7;
    return day(delta);
  }
  return null;
}

function iso(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}
