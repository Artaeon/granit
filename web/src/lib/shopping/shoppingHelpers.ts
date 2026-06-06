// Pure helpers + constants for the shopping surface.
//
// First extraction step out of routes/shopping/+page.svelte (707 LOC).
// Pulls the stateless bits — formatters, the canonical category /
// cadence lists, and the line-total math — into a plain `.ts` file so
// the route shell can import them without the `.svelte.ts` runes
// machinery.
//
// CategorySuggestions matches the server-side canonical order. If the
// Go side ever extends the list, mirror it here so the Plan-view
// grouping keeps sorting "known categories" before user-added ones.
//
// CadenceOption mirrors finance.IncomeStream cadences — the picker
// label `— no schedule —` matches granit's wider "—" sentinel for
// "no value yet".

import type { ShoppingItem } from '$lib/api';

export type Cadence = '' | 'weekly' | 'biweekly' | 'monthly' | 'quarterly' | 'yearly';

export const CADENCE_OPTIONS: { value: Cadence; label: string }[] = [
  { value: '', label: '— no schedule —' },
  { value: 'weekly', label: 'weekly' },
  { value: 'biweekly', label: 'biweekly' },
  { value: 'monthly', label: 'monthly' },
  { value: 'quarterly', label: 'quarterly' },
  { value: 'yearly', label: 'yearly' }
];

export const CATEGORY_SUGGESTIONS = [
  'groceries',
  'household',
  'clothing',
  'health',
  'electronics',
  'books',
  'gifts',
  'other'
];

const CADENCE_VALUES: readonly Cadence[] = ['weekly', 'biweekly', 'monthly', 'quarterly', 'yearly'];

/** Coerce an arbitrary server-string cadence onto the canonical union.
 *  Anything not in the canonical set (legacy values, typos) becomes
 *  `''` so the picker shows "no schedule" rather than a broken option. */
export function normalizeCadence(raw: string | undefined): Cadence {
  const c = (raw ?? '') as Cadence;
  return CADENCE_VALUES.includes(c) ? c : '';
}

/** Approx monthly factor for a cadence — used to project a recurring
 *  line into the monthly estimate chip shown in the inline edit. */
export function cadenceMonthlyFactor(c: Cadence): number {
  switch (c) {
    case 'weekly': return 52 / 12;
    case 'biweekly': return 26 / 12;
    case 'monthly': return 1;
    case 'quarterly': return 1 / 3;
    case 'yearly': return 1 / 12;
    default: return 0;
  }
}

/** Pretty currency. We don't know the user's currency upfront —
 *  hard-coding € would surprise non-EU users; using Intl with no
 *  currency renders just numbers; defaulting to EUR is a UX bet
 *  appropriate for granit's primary user base. A future settings
 *  toggle for currency lands cleanly here. */
export function fmtMoney(n: number | undefined): string {
  if (n === undefined || n === null || n === 0) return '—';
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency: 'EUR',
      maximumFractionDigits: 2
    }).format(n);
  } catch {
    return String(n);
  }
}

export function lineTotal(it: ShoppingItem): number {
  const qty = it.quantity && it.quantity > 0 ? it.quantity : 1;
  return (it.price ?? 0) * qty;
}

export function categoryLabel(c: string): string {
  return c === '—' ? 'uncategorised' : c;
}
