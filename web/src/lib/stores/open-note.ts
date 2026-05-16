// Open-note tray store. Tracks the note the user last opened so the
// global tray bar (NoteTray.svelte) can offer "jump back" from any
// page. Persisted to localStorage so a reload (or a closed tab
// reopened later in the day) still surfaces the last reading
// context — the user described it as "a system tray for the open
// note": ambient, low-friction, always reachable.
//
// Three pieces of state live here:
//   • lastOpenNote — the most-recently-opened note, replaced on every
//     navigation into /notes/<path>. Single slot. Dismissed by the
//     user via the tray's "×" button (clears it for the session).
//   • pinned       — up to TRAY_PIN_CAP notes the user explicitly
//     pinned via the tray overflow menu. Survive dismissal and
//     reload. Distinct from lastOpenNote so the user can pin a
//     reference note + still jump back to whatever they were just
//     reading.
//   • enabled      — the settings opt-out. Tray hides entirely when
//     false. Default ON; flipping it doesn't clear the stored note,
//     so re-enabling restores the previous state.
//
// All three are localStorage-only — single-tenant app, per-device
// preference, no backend round-trip warranted.

import { get } from 'svelte/store';
import { persistedWritable } from '$lib/util/persistedWritable';

export interface OpenNoteEntry {
  /** Vault-relative note path, same shape the editor route consumes. */
  path: string;
  /** Display title — falls back to a derived stem when the frontmatter
   *  title is missing. Stored so the tray can render without a fetch. */
  title: string;
  /** ISO timestamp of when this entry was recorded. Used for stale-
   *  entry pruning + sort order in the (optional) multi-pin strip. */
  openedAt: string;
  /** Editor scroll-top at the moment we recorded it. Forwarded to the
   *  next visit when the editor mounts so the user lands where they
   *  left off. Best-effort — `noteHistory.recallScroll` is the actual
   *  authoritative source; this field is here so the tray can show a
   *  "resume" hint without reading two stores. */
  scrollPos?: number;
}

// Cap on tray pins. The user described the single-note case as the
// priority; supporting a small strip costs nothing once the store is
// in place but anything over ~3 starts to clutter the bar.
export const TRAY_PIN_CAP = 3;

const LAST_KEY = 'granit.openNote.last';
const PINNED_KEY = 'granit.openNote.pinned';
const ENABLED_KEY = 'granit.openNote.trayEnabled';

function isEntry(raw: unknown): raw is OpenNoteEntry {
  if (!raw || typeof raw !== 'object') return false;
  const r = raw as Record<string, unknown>;
  return typeof r.path === 'string' && typeof r.title === 'string' && typeof r.openedAt === 'string';
}

/** Most-recently opened note. `null` when there is no remembered note
 *  (fresh install, user dismissed, or store wiped). */
export const lastOpenNote = persistedWritable<OpenNoteEntry | null>(LAST_KEY, null, {
  validate: (raw) => (isEntry(raw) ? raw : null)
});

/** Pinned notes — small array, insertion-ordered. Items appear in the
 *  tray strip in pin order so the user's mental "first pin is first
 *  on the left" matches the visual. */
export const pinnedTrayNotes = persistedWritable<OpenNoteEntry[]>(PINNED_KEY, [], {
  validate: (raw) => {
    if (!Array.isArray(raw)) return [];
    return raw.filter(isEntry).slice(0, TRAY_PIN_CAP);
  }
});

/** Whether the tray is visible at all. Toggle in Settings → General. */
export const trayEnabled = persistedWritable<boolean>(ENABLED_KEY, true, {
  validate: (raw) => raw !== false
});

/** Stem-from-path fallback used when callers can't supply a title
 *  (offline draft, raw URL paste, etc.). Strips the `.md` extension
 *  and the folder prefix the same way the notes page does. */
export function titleFromPath(path: string): string {
  const stem = path.split('/').pop() ?? path;
  return stem.replace(/\.md$/i, '');
}

/**
 * Record `entry` as the most-recently opened note. Used by the
 * notes/[...path] page on every mount + path change. Same-path
 * re-records still update `openedAt` so the tray can show "just now"
 * vs "30m ago" later if we ever want a timestamp affordance.
 */
export function recordOpenNote(entry: Omit<OpenNoteEntry, 'openedAt'> & { openedAt?: string }): void {
  const next: OpenNoteEntry = {
    path: entry.path,
    title: entry.title || titleFromPath(entry.path),
    openedAt: entry.openedAt ?? new Date().toISOString(),
    scrollPos: entry.scrollPos
  };
  lastOpenNote.set(next);
}

/** Update only the scrollPos on the current `lastOpenNote`. No-op if
 *  the stored entry's path doesn't match (the user has moved on). */
export function updateOpenNoteScroll(path: string, scrollPos: number): void {
  const cur = get(lastOpenNote);
  if (!cur || cur.path !== path) return;
  lastOpenNote.set({ ...cur, scrollPos });
}

/** Drop the most-recently-opened note from the tray. The user clicked
 *  "× dismiss" or hit "Clear" in the overflow menu. */
export function clearOpenNote(): void {
  lastOpenNote.set(null);
}

/**
 * Pin a note to the tray. Idempotent — re-pinning the same path
 * moves the entry to the end of the list (most-recent pin sits at
 * the right edge of the strip), and capacity overflows drop the
 * oldest pin so a power user can keep cycling without manual
 * unpinning.
 */
export function pinOpenNote(entry: Omit<OpenNoteEntry, 'openedAt'> & { openedAt?: string }): void {
  const normalized: OpenNoteEntry = {
    path: entry.path,
    title: entry.title || titleFromPath(entry.path),
    openedAt: entry.openedAt ?? new Date().toISOString(),
    scrollPos: entry.scrollPos
  };
  pinnedTrayNotes.update((cur) => {
    const filtered = cur.filter((e) => e.path !== normalized.path);
    const next = [...filtered, normalized];
    // Drop oldest entries (front of list) when we overflow.
    if (next.length > TRAY_PIN_CAP) next.splice(0, next.length - TRAY_PIN_CAP);
    return next;
  });
}

/** Remove `path` from the pinned list. No-op when absent. */
export function unpinOpenNote(path: string): void {
  pinnedTrayNotes.update((cur) => cur.filter((e) => e.path !== path));
}

/** Read-only check used by the tray UI to render the correct
 *  "Pin" vs "Unpin" affordance without subscribing. */
export function isTrayPinned(path: string): boolean {
  return get(pinnedTrayNotes).some((e) => e.path === path);
}
