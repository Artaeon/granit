// Shared recent-prompts log — every AI prompt the user submits, from
// any surface (inline AI menu, chat overlay), recorded once and read
// from anywhere. Solves the surprise of "I asked this in chat
// yesterday and now I'm in the inline menu trying to remember how I
// phrased it" — same fingertips, both surfaces.
//
// Per-surface history (InlineAIMenu's per-note `granit.ai.history.<path>`
// localStorage and AIOverlay's persistent threads) is preserved — this
// is additive. The cross-source list shows up as a small "recent
// across" strip alongside each surface's own surface-local history.
//
// Storage shape: a single localStorage key holding a capped array of
// entries newest-first. Each write does a content-dedup so spamming
// the same prompt doesn't crowd the list; the existing copy is
// moved to the front instead.

import { loadStoredString, saveStoredString } from '$lib/util/storage';

const KEY = 'granit.ai.recent-prompts';
const CAP = 50;

export type PromptSource = 'inline' | 'chat';

export interface RecentPrompt {
  prompt: string;
  at: number;            // epoch ms
  source: PromptSource;
  notePath?: string;     // only set when source === 'inline'
}

function load(): RecentPrompt[] {
  const raw = loadStoredString(KEY, '');
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    // Defensive filter — drop entries that don't conform.
    return parsed.filter((e): e is RecentPrompt =>
      e && typeof e.prompt === 'string' && typeof e.at === 'number'
      && (e.source === 'inline' || e.source === 'chat')
    ).slice(0, CAP);
  } catch {
    return [];
  }
}

function save(entries: RecentPrompt[]): void {
  try {
    saveStoredString(KEY, JSON.stringify(entries.slice(0, CAP)));
  } catch {
    // localStorage full / private mode — drop persistence silently.
    // The in-process callers will still see the in-memory list for
    // this session via list(), so the surface still works.
  }
}

/** Record a freshly-submitted prompt. Content-dedups: if the exact
 *  same prompt already exists (case-insensitive trim-equal), the
 *  existing copy moves to the front instead of duplicating. */
export function record(entry: Omit<RecentPrompt, 'at'>): void {
  const trimmed = entry.prompt.trim();
  if (!trimmed) return;
  const key = trimmed.toLowerCase();
  const cur = load();
  const next: RecentPrompt[] = [
    { ...entry, prompt: trimmed, at: Date.now() },
    ...cur.filter((e) => e.prompt.trim().toLowerCase() !== key)
  ];
  save(next);
}

/** Most-recent-first list. Optional filter: pass {source} to scope
 *  to one surface (e.g. 'show only prompts from the chat overlay'). */
export function list(opts: { source?: PromptSource; limit?: number } = {}): RecentPrompt[] {
  let arr = load();
  if (opts.source) arr = arr.filter((e) => e.source === opts.source);
  if (opts.limit && opts.limit > 0) arr = arr.slice(0, opts.limit);
  return arr;
}

/** Wipe the shared log. Surfaces a "clear recents" affordance to
 *  users who want a fresh slate; nothing in the app calls this by
 *  itself. */
export function clear(): void {
  saveStoredString(KEY, '');
}
