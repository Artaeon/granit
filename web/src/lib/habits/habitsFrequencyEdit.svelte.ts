// Per-habit inline frequency / cadence editor.
//
// Same shape as habitsReminderEdit: keyed by habit name, single open
// row at a time, draft buffer, cancel sentinel that closes without
// saving, submit that delegates persistence to the parent via
// `onPatch`. The picker component itself owns the chips + weekday
// toggles; this controller just holds the open/close state and the
// validated draft string.
//
// Accepted cadence strings (matches the formatter in
// habitsFrequencyFormat.ts):
//   • "daily" / "weekdays" / "weekends"
//   • "Nx-week" for 1 ≤ N ≤ 7
//   • CSV of weekday tokens ("mon", "tue", ...) — order-insensitive,
//     duplicates collapsed
//   • "" — clears the cadence (UI shows nothing)

import { toast } from '$lib/components/toast';
import { WEEKDAY_KEYS } from './habitsFrequencyFormat';

const PRESETS = new Set(['daily', 'weekdays', 'weekends']);
const X_WEEK_RE = /^[1-7]x-week$/;

/** Validate any cadence string the picker (or a future API) might
 *  hand us. Empty counts as valid — it's the "clear" sentinel. */
export function isValidFrequency(v: string): boolean {
  const raw = v.trim().toLowerCase();
  if (raw === '') return true;
  if (PRESETS.has(raw)) return true;
  if (X_WEEK_RE.test(raw)) return true;
  if (raw.includes(',') || WEEKDAY_KEYS.includes(raw)) {
    const tokens = raw.split(',').map((t) => t.trim()).filter(Boolean);
    if (tokens.length === 0) return false;
    return tokens.every((t) => WEEKDAY_KEYS.includes(t));
  }
  return false;
}

/** Canonicalise a weekday-CSV cadence so the persisted value is
 *  Sun→Sat and deduped. Non-CSV inputs pass through untouched. */
export function canonicaliseFrequency(v: string): string {
  const raw = v.trim().toLowerCase();
  if (!raw || PRESETS.has(raw) || X_WEEK_RE.test(raw)) return raw;
  const tokens = raw.split(',').map((t) => t.trim()).filter(Boolean);
  if (tokens.length === 0) return '';
  const idx = Array.from(new Set(tokens.map((t) => WEEKDAY_KEYS.indexOf(t))))
    .filter((i) => i >= 0)
    .sort((a, b) => a - b);
  return idx.map((i) => WEEKDAY_KEYS[i]).join(',');
}

export interface FrequencyEditController {
  /** Habit name with picker open, or null. */
  readonly editing: string | null;
  /** Buffered cadence string. Bind from picker chips / weekday row. */
  draft: string;
  /** True while submit() is in flight. */
  readonly busy: boolean;
  /** Last validation / network error. Empty when clean. */
  readonly error: string;

  /** Open the picker for `name` seeded with its current cadence. */
  start(name: string, current: string | undefined): void;
  /** Close without saving. */
  cancel(): void;
  /** Validate + persist via onPatch. Canonicalises CSV inputs before
   *  sending so "fri,mon" → "mon,fri". */
  submit(name: string): Promise<void>;
}

export type FrequencyEditDeps = {
  onPatch: (name: string, patch: { frequency: string }) => Promise<void>;
};

export function createFrequencyEditCtl(deps: FrequencyEditDeps): FrequencyEditController {
  let editing = $state<string | null>(null);
  let draft = $state('');
  let busy = $state(false);
  let error = $state('');

  function start(name: string, current: string | undefined): void {
    editing = name;
    draft = current ?? '';
    error = '';
  }

  function cancel(): void {
    editing = null;
    draft = '';
    error = '';
  }

  async function submit(name: string): Promise<void> {
    if (busy) return;
    if (!isValidFrequency(draft)) {
      error = 'unrecognised cadence';
      return;
    }
    busy = true;
    error = '';
    try {
      const canon = canonicaliseFrequency(draft);
      await deps.onPatch(name, { frequency: canon });
      editing = null;
      draft = '';
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      error = msg;
      toast.error(`frequency update failed: ${msg}`);
    } finally {
      busy = false;
    }
  }

  return {
    get editing() { return editing; },
    get draft() { return draft; },
    set draft(v) { draft = v; },
    get busy() { return busy; },
    get error() { return error; },
    start,
    cancel,
    submit
  };
}
