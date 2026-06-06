// RAG (retrieval-augmented generation) — strict in-process retrieval over
// the user's vault. No embeddings, no extra service: a two-stage scoring
// pass with a recency tiebreaker, capped to 3 hits + 800-char excerpts so
// the prompt stays bounded on a 5k-note vault.
//
// Extracted from AIOverlay.svelte so the same retrieval can be reused
// from other surfaces (the dedicated /chat page, future inline RAG in
// the editor) without copy-pasting the algorithm. Pure functions over
// `api` — no Svelte state required.
//
// Future: swap retrieveForRag() for a real embedding lookup at the same
// call site. The shape (hits with title + path + excerpt + score) is
// what the consumers need; the scoring is the implementation detail.

import { api } from '$lib/api';
import { onWsEvent } from '$lib/ws';

export type RagHit = {
  path: string;
  title: string;
  excerpt: string;
  score: number;
};

export type RagIndexEntry = {
  path: string;
  title: string;
  modTime: string;
};

// Token-cleanup stopwords. Tiny English set — RAG queries are typically
// short, and dropping these lets the score reflect content words rather
// than 'the', 'a', etc.
const STOPWORDS = new Set([
  'the', 'a', 'an', 'of', 'to', 'in', 'for', 'on', 'and', 'or', 'is', 'it', 'be',
  'are', 'was', 'were', 'this', 'that', 'with', 'from', 'as', 'by', 'at', 'but',
  'not', 'if', 'so', 'do', 'does', 'did', 'have', 'has', 'had', 'can', 'will',
  'what', 'when', 'how', 'why', 'who', 'where', 'should', 'would', 'could',
  'about', 'into', 'over', 'than', 'then', 'them', 'they', 'their'
]);

// Per-tab cached vault index. The first call populates it; subsequent
// calls are cheap. WS-driven invalidation flips ragIndexLoaded back
// to false so the next retrieveForRag() call refreshes the index
// before scoring — previously a renamed or deleted note could keep
// surfacing as a stale title for the rest of the session.
let ragIndex: RagIndexEntry[] = [];
let ragIndexLoaded = false;

export function getRagIndex(): RagIndexEntry[] {
  return ragIndex;
}

export function isRagIndexLoaded(): boolean {
  return ragIndexLoaded;
}

/** Mark the cached index as stale so the next retrieveForRag triggers
 *  a fresh listNotes round-trip. Cheap — just flips a flag. */
function invalidateRagIndex(): void {
  ragIndexLoaded = false;
}

export async function loadRagIndex(): Promise<void> {
  if (ragIndexLoaded) return;
  // Only flip the loaded flag on actual success. The previous shape
  // used `finally`, so a transient 5xx / network blip during the
  // first send left ragIndex empty AND ragIndexLoaded=true — the
  // next retrieveForRag short-circuited the retry path forever for
  // the tab session, and the user got silent zero-hit RAG for the
  // rest of the session even after the network recovered.
  const r = await api.listNotes({ limit: 5000 });
  ragIndex = r.notes.map((n) => ({
    path: n.path,
    title: n.title || n.path.replace(/\.md$/, ''),
    modTime: n.modTime
  }));
  ragIndexLoaded = true;
}

// Wire vault-mutation events to invalidate the cache once the module
// loads in a browser. SSR / test imports skip this so the unit tests
// don't pick up a hanging WS subscription. The actual refetch waits
// until the next retrieveForRag call — cheap on note-write bursts,
// fresh by the time it matters (a user mid-RAG query).
if (typeof window !== 'undefined') {
  onWsEvent((ev) => {
    if (ev.type === 'note.changed' || ev.type === 'note.removed' || ev.type === 'vault.rescanned') {
      invalidateRagIndex();
    }
  });
}

// Retrieve top-K notes for the user's query. Two-stage:
//   1. Title-token match: every note whose title contains any of the
//      query tokens scores 2 per token. Cheap, exact, no I/O.
//   2. For the top ~12 by title score, fetch their bodies and add 1 per
//      body-token match (simple substring count).
// Recency bumps the final score slightly so a note touched yesterday
// wins over one untouched in 2024 when titles tie. We cap at 3 hits +
// clip each excerpt to 500 chars so the prompt stays bounded on a 5k-
// note vault. Earlier we used 800 chars per excerpt — empirically the
// match-anchored ±200 char window catches the relevant passage in 500
// chars too, and the lower cap saves ~150 tokens × 3 hits = 450 tokens
// per RAG-enabled chat turn.
export async function retrieveForRag(
  query: string,
  currentNote?: string
): Promise<RagHit[]> {
  if (!ragIndexLoaded) await loadRagIndex();
  const tokens = Array.from(
    new Set(
      query
        .toLowerCase()
        .replace(/[^\w\s/-]/g, ' ')
        .split(/\s+/)
        .filter((t) => t.length >= 3 && !STOPWORDS.has(t))
    )
  );
  if (tokens.length === 0) return [];
  const now = Date.now();
  const titleScored = ragIndex
    .map((n) => {
      if (n.path === currentNote) return null; // exclude the current note from RAG
      let s = 0;
      const title = n.title.toLowerCase();
      for (const t of tokens) {
        if (title.includes(t)) s += 2;
      }
      // Recency tiebreaker: +0..0.5 based on age vs 30-day window.
      const age = now - new Date(n.modTime).getTime();
      const recency = Math.max(0, Math.min(0.5, 0.5 - age / (30 * 86_400_000)));
      return s > 0 ? { ...n, score: s + recency } : null;
    })
    .filter((x): x is { path: string; title: string; modTime: string; score: number } => !!x)
    .sort((a, b) => b.score - a.score)
    .slice(0, 12);
  if (titleScored.length === 0) return [];
  // Body fetch top 12 in parallel; score each body match.
  const bodies = await Promise.all(
    titleScored.map((n) => api.getNote(n.path).catch(() => null))
  );
  const final: RagHit[] = [];
  for (let i = 0; i < titleScored.length; i++) {
    const meta = titleScored[i];
    const body = bodies[i]?.body ?? '';
    let bodyScore = 0;
    const lc = body.toLowerCase();
    for (const t of tokens) {
      // Count occurrences (capped at 5 per token to avoid one
      // word-spam note dominating).
      let count = 0;
      let idx = 0;
      while ((idx = lc.indexOf(t, idx)) >= 0 && count < 5) {
        count++;
        idx += t.length;
      }
      bodyScore += count;
    }
    const totalScore = meta.score + bodyScore;
    if (totalScore <= 0) continue;
    // Excerpt: find the first body line that mentions any token,
    // ±125 chars on either side. Falls back to the start of the body.
    // Total slice = 500 chars per hit (was 800).
    let excerpt = body.slice(0, 500);
    for (const t of tokens) {
      const at = lc.indexOf(t);
      if (at >= 0) {
        const start = Math.max(0, at - 125);
        excerpt = body.slice(start, start + 500);
        if (start > 0) excerpt = '…' + excerpt;
        break;
      }
    }
    final.push({ path: meta.path, title: meta.title, excerpt: excerpt.trim(), score: totalScore });
  }
  final.sort((a, b) => b.score - a.score);
  return final.slice(0, 3);
}
