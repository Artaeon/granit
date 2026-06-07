// Pure formatter for a habit's `frequency` field. The backend stores
// cadence as a canonical string (one of a small known set, or a CSV
// of weekday tokens). The UI surfaces it as a glanceable human label
// — "Daily", "Weekdays", "3×/week", "Mon Wed Fri", etc.
//
// Kept dependency-free so it can be reused by both the inline-card
// label and the picker preview without dragging in Svelte runtime.
// All matching is case-insensitive; whitespace is trimmed.
//
// Canonical inputs handled:
//   • "daily"                 → "Daily"
//   • "weekdays"              → "Weekdays"
//   • "weekends"              → "Weekends"
//   • "<N>x-week" (1≤N≤7)     → "N×/week"
//   • weekday CSV             → "Mon Wed Fri" (capitalised, deduped,
//                               re-sorted Sun→Sat for stable read)
//
// Unrecognised non-empty input → "Custom". Empty / undefined → "".

const WEEKDAY_TOKENS = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'] as const;
const WEEKDAY_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'] as const;

export function formatFrequency(freq: string | undefined | null): string {
  if (!freq) return '';
  const raw = freq.trim().toLowerCase();
  if (!raw) return '';

  if (raw === 'daily') return 'Daily';
  if (raw === 'weekdays') return 'Weekdays';
  if (raw === 'weekends') return 'Weekends';

  // "Nx-week" → "N×/week". Matches the cadence shape used by the TUI.
  const xWeek = raw.match(/^([1-7])x-week$/);
  if (xWeek) return `${xWeek[1]}×/week`;

  // Weekday CSV. Each token must be one of the three-letter day
  // names; ordering / duplicates are normalised so "fri,mon,mon" and
  // "mon,fri" render the same.
  if (raw.includes(',')) {
    const tokens = raw.split(',').map((t) => t.trim()).filter(Boolean);
    if (tokens.length > 0 && tokens.every((t) => WEEKDAY_TOKENS.includes(t as (typeof WEEKDAY_TOKENS)[number]))) {
      const idx = Array.from(new Set(tokens.map((t) => WEEKDAY_TOKENS.indexOf(t as (typeof WEEKDAY_TOKENS)[number]))))
        .sort((a, b) => a - b);
      return idx.map((i) => WEEKDAY_LABELS[i]).join(' ');
    }
  }

  // Single weekday token also accepted (so "mon" reads as "Mon").
  if (WEEKDAY_TOKENS.includes(raw as (typeof WEEKDAY_TOKENS)[number])) {
    return WEEKDAY_LABELS[WEEKDAY_TOKENS.indexOf(raw as (typeof WEEKDAY_TOKENS)[number])];
  }

  return 'Custom';
}

/** Lowercase three-letter weekday tokens in canonical Sun→Sat order. */
export const WEEKDAY_KEYS: readonly string[] = WEEKDAY_TOKENS;
/** Capitalised three-letter weekday labels, Sun→Sat. */
export const WEEKDAY_DISPLAY: readonly string[] = WEEKDAY_LABELS;
