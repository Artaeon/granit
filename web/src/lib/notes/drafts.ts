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
  let idx = readIndex().filter((p) => p !== path);
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

/** Returns true if the local draft is meaningfully different from the
 *  server's body (not just trailing whitespace). */
export function draftDivergesFromServer(draft: Draft, serverBody: string): boolean {
  return normalize(draft.body) !== normalize(serverBody);
}

function normalize(s: string): string {
  return s.replace(/\r\n/g, '\n').replace(/[ \t]+\n/g, '\n').replace(/\n+$/g, '');
}
