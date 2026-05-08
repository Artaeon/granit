// Per-note local drafts in localStorage.
//
// Why: auto-save fires the network 2s after every keystroke. If the user is
// offline (or the server is down), those saves fail. Without a draft layer
// the editor body lives only in component memory — a tab reload or a phone
// lock could lose minutes of work. Drafts persist the body byte-for-byte
// to localStorage so it survives both.
//
// Lifecycle:
//   - Every body change → setDraft(path, body, baseModTime)
//   - Note load → check getDraft(path); if present and newer than the
//     server copy, pre-load that into the editor and queue an
//     immediate save attempt to push it through.
//   - Successful save → clearDraft(path).
//
// Trim: caps at the 12 most-recent drafts so a long power loss + lots of
// editing won't blow past the localStorage quota (~5MB on most browsers).

const PREFIX = 'granit.draft:';
const INDEX_KEY = 'granit.drafts.index';
const MAX_DRAFTS = 12;

export interface Draft {
  body: string;
  /** epoch ms — when the local edit was made */
  savedAt: number;
  /** ISO string — server's modTime when this draft was started; used to
   *  detect divergence from the server side */
  baseModTime: string;
}

function readIndex(): string[] {
  if (typeof localStorage === 'undefined') return [];
  try {
    const raw = localStorage.getItem(INDEX_KEY);
    if (!raw) return [];
    const arr = JSON.parse(raw);
    return Array.isArray(arr) ? arr.filter((p) => typeof p === 'string') : [];
  } catch {
    return [];
  }
}

function writeIndex(paths: string[]): void {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(INDEX_KEY, JSON.stringify(paths));
  } catch {
    /* quota exceeded — give up */
  }
}

function key(path: string): string {
  return PREFIX + path;
}

export function setDraft(path: string, body: string, baseModTime: string): void {
  if (typeof localStorage === 'undefined') return;
  const draft: Draft = { body, savedAt: Date.now(), baseModTime };
  try {
    localStorage.setItem(key(path), JSON.stringify(draft));
  } catch {
    // Likely quota exceeded — try evicting the oldest draft and retry once.
    const idx = readIndex();
    const oldest = idx[idx.length - 1];
    if (oldest && oldest !== path) {
      localStorage.removeItem(key(oldest));
      writeIndex(idx.slice(0, -1));
      try {
        localStorage.setItem(key(path), JSON.stringify(draft));
      } catch {
        return;
      }
    } else {
      return;
    }
  }
  // Move this path to the front of the index (most-recently-touched).
  // Skip the read-modify-write when the path is already at the front
  // — autosave fires once per keystroke now (no debounce), and
  // re-writing a 12-entry array on every keystroke is wasted IO.
  // Only re-index when this is genuinely a new or moved entry.
  const idx0 = readIndex();
  if (idx0[0] === path && idx0.length <= MAX_DRAFTS) return;
  let idx = idx0.filter((p) => p !== path);
  idx.unshift(path);
  if (idx.length > MAX_DRAFTS) {
    for (const p of idx.slice(MAX_DRAFTS)) {
      try { localStorage.removeItem(key(p)); } catch {}
    }
    idx = idx.slice(0, MAX_DRAFTS);
  }
  writeIndex(idx);
}

export function getDraft(path: string): Draft | null {
  if (typeof localStorage === 'undefined') return null;
  try {
    const raw = localStorage.getItem(key(path));
    if (!raw) return null;
    const d = JSON.parse(raw) as Draft;
    if (typeof d.body !== 'string' || typeof d.savedAt !== 'number') return null;
    return d;
  } catch {
    return null;
  }
}

export function clearDraft(path: string): void {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.removeItem(key(path));
  } catch {}
  const idx = readIndex().filter((p) => p !== path);
  writeIndex(idx);
}

export function listDrafts(): { path: string; draft: Draft }[] {
  return readIndex()
    .map((p) => ({ path: p, draft: getDraft(p) }))
    .filter((x): x is { path: string; draft: Draft } => x.draft !== null);
}

// clearAllDrafts wipes every cached draft body + the index. Called on
// sign-out so a shared device doesn't leak note bodies to whoever logs
// in next. Best-effort: per-key removeItem may throw quota errors on
// some browsers, but the index removal at the end ensures listDrafts()
// returns empty even if a few keys linger.
export function clearAllDrafts(): void {
  if (typeof localStorage === 'undefined') return;
  for (const path of readIndex()) {
    try { localStorage.removeItem(key(path)); } catch {}
  }
  try { localStorage.removeItem(INDEX_KEY); } catch {}
}

/** Returns true if the local draft is meaningfully different from the
 *  server's body (not just trailing whitespace). */
export function draftDivergesFromServer(draft: Draft, serverBody: string): boolean {
  return normalize(draft.body) !== normalize(serverBody);
}

function normalize(s: string): string {
  return s.replace(/\r\n/g, '\n').replace(/[ \t]+\n/g, '\n').replace(/\n+$/g, '');
}

// ─── Reconnect flush ────────────────────────────────────────────────
//
// Before today, drafts for the currently-open note kept retrying via
// the editor's auto-save loop, but drafts for OTHER notes (saved
// while offline, then user navigated away) just sat in localStorage.
// When the user came back online, those drafts didn't push until
// they manually re-opened each affected note. flushDrafts() is the
// reconnect hook: walk the index and PUT each draft, clearing on
// success. Skips notes the user is currently editing — let the open
// editor's own dirty-tracking handle those (it has the live body,
// the draft on disk might be slightly older).
//
// On a network outage / 5xx server, we leave the draft in place so a
// later flush can retry. 4xx (typically 409 conflict / 401 unauth)
// drops the draft because the user needs to resolve it manually —
// silent retries on a real conflict would loop forever.

export interface DraftFlushReport {
  attempted: number;
  succeeded: number;
  failed: number;
}

export type DraftPutFn = (
  path: string,
  body: { body: string; frontmatter?: Record<string, unknown> }
) => Promise<{ modTime: string }>;

let flushInFlight = false;

/**
 * Push every queued draft to the server. Pass an optional
 * `currentlyOpenPath` so the active editor's draft isn't double-PUT
 * (the editor has its own retry loop with the live body).
 *
 * Returns a brief report; callers can surface "synced N notes" toasts.
 */
export async function flushDrafts(
  put: DraftPutFn,
  currentlyOpenPath?: string
): Promise<DraftFlushReport> {
  // Deduplicate concurrent calls — wasOffline + a manual "retry now"
  // click could both fire flush at the same instant.
  if (flushInFlight) return { attempted: 0, succeeded: 0, failed: 0 };
  flushInFlight = true;
  let attempted = 0;
  let succeeded = 0;
  let failed = 0;
  try {
    const drafts = listDrafts();
    for (const { path, draft } of drafts) {
      if (path === currentlyOpenPath) continue; // editor handles its own
      attempted++;
      try {
        await put(path, { body: draft.body });
        clearDraft(path);
        succeeded++;
      } catch (err) {
        // 4xx (status >= 400 < 500) — manual resolution required.
        // 5xx / network — keep draft for the next flush attempt.
        const status = (err as { status?: number })?.status;
        if (typeof status === 'number' && status >= 400 && status < 500) {
          clearDraft(path);
        }
        failed++;
      }
    }
  } finally {
    flushInFlight = false;
  }
  return { attempted, succeeded, failed };
}

/** Count of drafts currently queued. Read from the index, doesn't
 *  validate each entry — cheaper than listDrafts() for the badge in
 *  the offline pill that reads on every render. */
export function queuedDraftCount(): number {
  return readIndex().length;
}
