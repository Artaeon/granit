// Draft autosave — localStorage-backed safety net for in-progress
// edits. The notes editor has its own server-side autosave loop
// (2s debounce in routes/notes/[...path]/+page.svelte); this
// utility is for surfaces where save requires user confirmation
// (e.g., vision edits need a reason; legal forms have validation)
// so server autosave doesn't fit but we still want "reload mid-edit"
// not to lose data.
//
// Contract: write the in-progress value to a localStorage key as
// the user types. Restore the draft on next entry to the edit form.
// Clear the draft on successful save or explicit cancel. Stale
// drafts from older sessions get overwritten by fresh edits and
// cleared on the next save — they don't accumulate unless the
// user keeps abandoning forms, in which case manual storage
// management is the right escape valve.
//
// SSR-safe: every function no-ops if `localStorage` is undefined
// (server render path). Quota / disabled-storage errors are
// swallowed silently — draft is a safety net, not a hard
// dependency. The worst case if writes fail is the same as
// pre-draft behavior: reload loses unsaved work.

type DraftCache = {
  /** ISO timestamp of last write, so the UI can render "draft saved 12s ago". */
  savedAt: string;
  /** The actual draft payload — shape is caller-defined. */
  value: unknown;
};

function storageKey(key: string): string {
  // Prefix all draft keys so they're recognisable in DevTools and
  // a future "clear all drafts" command can find them with a
  // single startsWith() scan.
  return `granit.draft.${key}`;
}

/**
 * Read a draft from localStorage. Returns `fallback` if no draft
 * exists, parsing fails, or storage is unavailable.
 *
 * Typical use: in your edit-mode entry function, call this BEFORE
 * populating the form buffer from the server-loaded record. The
 * draft wins because it's strictly more recent than the server
 * record under the user's perspective.
 */
export function loadDraft<T>(key: string, fallback: T): T {
  if (typeof localStorage === 'undefined') return fallback;
  try {
    const raw = localStorage.getItem(storageKey(key));
    if (raw === null) return fallback;
    const parsed = JSON.parse(raw);
    // Runtime-validate the shape — old builds (or a future
    // build that changes the schema) may have stored a raw string
    // or a different envelope. Without this guard, parsed.value
    // would be undefined and slip past callers that test
    // `!== null && !== ''` (undefined passes both). Treat any
    // non-conforming entry as if no draft existed.
    if (parsed && typeof parsed === 'object' && 'value' in parsed) {
      return (parsed as DraftCache).value as T;
    }
    return fallback;
  } catch {
    return fallback;
  }
}

/**
 * Return the ISO timestamp of the last draft write for `key`, or
 * `null` if none. UI can use this to render a "draft from 2 min ago"
 * indicator next to the form.
 */
export function loadDraftSavedAt(key: string): string | null {
  if (typeof localStorage === 'undefined') return null;
  try {
    const raw = localStorage.getItem(storageKey(key));
    if (raw === null) return null;
    const parsed = JSON.parse(raw) as DraftCache;
    return parsed.savedAt ?? null;
  } catch {
    return null;
  }
}

/**
 * Write a draft value to localStorage. Non-debounced — for the
 * common "save on every keystroke" pattern use `makeDraftWriter`
 * below which debounces.
 */
export function saveDraft(key: string, value: unknown): void {
  if (typeof localStorage === 'undefined') return;
  try {
    const payload: DraftCache = { savedAt: new Date().toISOString(), value };
    localStorage.setItem(storageKey(key), JSON.stringify(payload));
  } catch {
    // Quota exceeded / storage disabled — best-effort.
  }
}

/**
 * Drop the draft for `key`. Call this after a successful server
 * save (the canonical state is now on the server, draft is
 * obsolete) or on explicit cancel.
 */
export function clearDraft(key: string): void {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.removeItem(storageKey(key));
  } catch {}
}

/**
 * Returns a debounced writer + cancel function. Pass the result of
 * `writer.save(key, value)` to a $effect so it runs on every change;
 * the writer collapses bursts of writes into one localStorage write
 * after `debounceMs` of quiet.
 *
 * Components should also call `writer.cancel()` on destroy to avoid
 * a trailing write firing after the form is gone. Not catastrophic
 * if missed — the next mount will overwrite or clear — but cheap to
 * do right.
 */
export function makeDraftWriter(debounceMs = 500): {
  save: (key: string, value: unknown) => void;
  cancel: () => void;
  flushNow: () => void;
} {
  let timer: ReturnType<typeof setTimeout> | null = null;
  let pendingKey: string | null = null;
  let pendingValue: unknown = null;

  function flush() {
    if (pendingKey !== null) {
      saveDraft(pendingKey, pendingValue);
    }
    pendingKey = null;
    pendingValue = null;
    timer = null;
  }

  return {
    save(key, value) {
      // If a different key is pending, flush it FIRST before rescheduling
      // for the new key. Without this, switching contexts within the
      // debounce window (user types in tab A, switches to tab B before
      // 400ms is up) would lose A's pending write — pendingKey would be
      // overwritten without ever firing for A. The flush is synchronous,
      // so by the time we exit this branch, A's draft is on disk.
      if (pendingKey !== null && pendingKey !== key) {
        if (timer) clearTimeout(timer);
        saveDraft(pendingKey, pendingValue);
      }
      pendingKey = key;
      pendingValue = value;
      if (timer) clearTimeout(timer);
      timer = setTimeout(flush, debounceMs);
    },
    cancel() {
      if (timer) clearTimeout(timer);
      timer = null;
      pendingKey = null;
      pendingValue = null;
    },
    /** Force the pending write immediately. Useful before navigation
     *  away from the form ("save draft, then jump to /vision"). */
    flushNow() {
      if (timer) clearTimeout(timer);
      flush();
    }
  };
}
