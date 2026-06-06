// Pure formatters shared across the finance surface.
//
// Pulled out of FinancePane.svelte so the modal / form extractions
// can import them without dragging the whole page in. Stateless —
// no runes, no controllers, just functions over numbers + strings.
//
//   • accColor       — finance account "color" tag → CSS variable.
//                      Unknown tags fall through to --color-surface2 so
//                      the row pip is just visible without yelling.
//   • fmtMoney       — render integer cents in the user's locale; falls
//                      back to "<CCY> <amount>" if Intl doesn't know
//                      the currency.
//   • relDate        — turn a YYYY-MM-DD into "today / tomorrow / in N
//                      days / N days ago" relative to the local clock.
//   • statusTone     — income stream status → status-pill colors +
//                      label, matched against the rest of the surface.
//   • dayOf          — day-of-month from a YYYY-MM-DD (timeline pip
//                      layout helper).
//   • dayLabel       — YYYY-MM-DD → "Mon 12" style short label for the
//                      cashflow timeline rows.
//
// All helpers tolerate junk input (NaN, empty strings, unknown status
// codes) and return safe fallbacks — they run on every render frame.
//
// The known account color palette is exported so the New-account modal
// can render the swatch row without duplicating the list.

export const ACCOUNT_COLORS = [
  'red',
  'orange',
  'yellow',
  'green',
  'blue',
  'purple',
  'cyan'
] as const;
export type AccountColor = (typeof ACCOUNT_COLORS)[number];

// Account color → CSS variable. Empty / unknown falls through to
// surface1 so the row pip is just visible without yelling.
export function accColor(c: string | undefined): string {
  if (!c) return 'var(--color-surface2)';
  const map: Record<string, string> = {
    red: 'var(--color-error)',
    orange: 'var(--color-accent)',
    yellow: 'var(--color-warning)',
    green: 'var(--color-success)',
    blue: 'var(--color-secondary)',
    purple: 'var(--color-primary)',
    cyan: 'var(--color-info)'
  };
  return map[c] ?? 'var(--color-surface2)';
}

// Render integer cents in the user's locale. Falls back to
// "<CCY> <amount>" if the browser doesn't know the code.
export function fmtMoney(cents: number, currency: string): string {
  if (!Number.isFinite(cents)) return '—';
  const value = cents / 100;
  if (!currency) return value.toFixed(2);
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency,
      currencyDisplay: 'narrowSymbol'
    }).format(value);
  } catch {
    return `${currency} ${value.toFixed(2)}`;
  }
}

export function relDate(iso: string): string {
  if (!iso) return '';
  const d = new Date(iso + 'T00:00:00');
  if (Number.isNaN(d.getTime())) return iso;
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const diff = Math.round((d.getTime() - today.getTime()) / 86400000);
  if (diff === 0) return 'today';
  if (diff === 1) return 'tomorrow';
  if (diff === -1) return 'yesterday';
  if (diff > 0) return `in ${diff} days`;
  return `${-diff} days ago`;
}

export type StatusTone = { bg: string; text: string; label: string };

export function statusTone(s: string): StatusTone {
  switch (s) {
    case 'active':  return { bg: 'bg-surface0', text: 'text-success', label: 'Active' };
    case 'planned': return { bg: 'bg-surface0',    text: 'text-info',    label: 'Planned' };
    case 'idea':    return { bg: 'bg-primary/15', text: 'text-primary', label: 'Idea' };
    case 'paused':  return { bg: 'bg-surface1',   text: 'text-dim',     label: 'Paused' };
    default:        return { bg: 'bg-surface1',   text: 'text-subtext', label: s || '—' };
  }
}

// Day-of-month from a YYYY-MM-DD; used for the timeline pip layout.
export function dayOf(iso: string): number {
  const m = iso.match(/-(\d{2})$/);
  return m ? parseInt(m[1], 10) : 0;
}

export function dayLabel(iso: string): string {
  const d = new Date(iso + 'T00:00:00');
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}
