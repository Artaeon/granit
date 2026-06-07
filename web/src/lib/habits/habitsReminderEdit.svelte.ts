// Per-habit inline reminder-time editor.
//
// Mirrors the local rename / target-edit pattern in /habits/+page.svelte:
// one habit edits at a time (keyed by name), a draft buffer, a cancel
// sentinel that closes without persisting, and a submit that delegates
// the network round-trip to the parent via `onPatch` so this controller
// stays unaware of the API client.
//
// Validation is HH:MM (24h, leading zeros). Empty input is allowed and
// means "clear the reminder" — the badge disappears on the card.
// Anything else is rejected silently (caller can read `.error` if they
// want surface validation; KISS keeps it simple).

import { toast } from '$lib/components/toast';

/** HH:MM 24h regex. 00:00–23:59. Leading zeros required so the
 *  surface matches what `<input type="time">` emits. */
const HHMM_RE = /^([01]\d|2[0-3]):[0-5]\d$/;

export interface ReminderEditController {
  /** Habit name currently being edited, or null when nothing is open. */
  readonly editing: string | null;
  /** Buffered HH:MM input. Bind directly to the time input. */
  draft: string;
  /** True while a submit is in flight; gates double-clicks. */
  readonly busy: boolean;
  /** Last validation / network error message. Empty when clean. */
  readonly error: string;

  /** Open the editor on `name` with its current `reminderTime` (or "" if unset). */
  start(name: string, current: string | undefined): void;
  /** Close without saving. Same as pressing Esc on the input. */
  cancel(): void;
  /** Validate + persist the draft via onPatch. On success, closes the
   *  editor and clears state. On validation failure, sets `.error`
   *  and leaves the editor open. */
  submit(name: string): Promise<void>;
}

export type ReminderEditDeps = {
  /** Persist the partial patch and return when reload has run. */
  onPatch: (name: string, patch: { reminderTime: string }) => Promise<void>;
};

export function createReminderEditCtl(deps: ReminderEditDeps): ReminderEditController {
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
    const v = draft.trim();
    if (v !== '' && !HHMM_RE.test(v)) {
      error = 'time must be HH:MM (24h)';
      return;
    }
    busy = true;
    error = '';
    try {
      await deps.onPatch(name, { reminderTime: v });
      editing = null;
      draft = '';
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      error = msg;
      toast.error(`reminder update failed: ${msg}`);
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

/** Exported for tests / pickers that want the same shape. */
export function isValidReminderTime(v: string): boolean {
  return v === '' || HHMM_RE.test(v);
}
