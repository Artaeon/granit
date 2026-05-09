// Chat thread history — localStorage-backed LRU of saved overlay
// conversations. Lives outside ai-overlay.ts so the dedicated /chat
// page (and any future chat surface) can share it.
//
// Design principles:
//   - sessionStorage holds the in-flight thread (cleared on tab
//     close); localStorage holds the long-term history (survives).
//     The overlay's existing sessionStorage flow is unchanged — we
//     just snapshot threads into the persistent store on first
//     user→assistant exchange and on every subsequent turn.
//   - Cap at MAX_THREADS (LRU). The user shouldn't accumulate
//     hundreds of half-finished chats; older ones drop quietly.
//   - Cap message count per thread (MAX_MESSAGES_PER_THREAD) to keep
//     localStorage bounded. ~30 turns per thread is plenty for the
//     overlay; the dedicated /chat page is the place for marathon
//     threads, but it gets the same treatment for symmetry.
//   - Thread id is a short base36 timestamp, so a casual glance at
//     localStorage shows them in sort order even raw.
//   - Pinned messages are stored alongside the thread (per-thread
//     pin set) and ALSO in a flat top-level pinboard so a user can
//     scan their best replies across every thread without loading
//     each one.

import type { ChatMessage } from '$lib/api';
import { loadStored, saveStored } from '$lib/util/storage';

const THREADS_KEY = 'granit.chat.threads';
const PINNED_KEY = 'granit.chat.pinned';
const MAX_THREADS = 30;
const MAX_MESSAGES_PER_THREAD = 60;

export interface ChatThread {
  /** Stable id — base36 of createdAt millis. */
  id: string;
  /** First user message, slugged for the picker. Falls back to the
   *  full content if the message is short enough. Updated only on
   *  the first user message (subsequent renames would be jarring). */
  title: string;
  /** Mode id active when the thread was created. Surfaced in the
   *  picker so the user remembers the posture. */
  modeId: string;
  /** Message log. Capped to MAX_MESSAGES_PER_THREAD with oldest dropped. */
  messages: ChatMessage[];
  /** Created + last-touched timestamps. updatedAt drives LRU sort. */
  createdAt: number;
  updatedAt: number;
}

export interface PinnedMessage {
  /** Which thread the message lived in. The thread may have since been
   *  pruned (LRU); the pin survives via the snapshotted content here. */
  threadId: string;
  threadTitle: string;
  modeId: string;
  /** Message index within the thread when pinned. Used for "jump to"
   *  if the thread still exists, otherwise informational. */
  messageIndex: number;
  /** Snapshot of the assistant message's content at pin time. We
   *  snapshot rather than reference because the underlying message
   *  might be re-rolled (a future feature) or the thread pruned. */
  content: string;
  /** When the user pinned it. */
  pinnedAt: number;
}

function readThreads(): ChatThread[] {
  const parsed = loadStored<unknown>(THREADS_KEY, []);
  if (!Array.isArray(parsed)) return [];
  return parsed.filter(
    (t): t is ChatThread =>
      t &&
      typeof t === 'object' &&
      typeof t.id === 'string' &&
      typeof t.title === 'string' &&
      Array.isArray(t.messages)
  );
}

function writeThreads(list: ChatThread[]): void {
  // Sort newest-first and cap. LRU semantics — least recently
  // updated gets evicted. Quota errors are absorbed silently by
  // saveStored; the user still has the thread in sessionStorage
  // for the current tab session.
  const sorted = [...list].sort((a, b) => b.updatedAt - a.updatedAt).slice(0, MAX_THREADS);
  saveStored(THREADS_KEY, sorted);
}

/** Read all saved threads, newest-first. Lazy — only called when the
 *  user opens the history panel. */
export function listThreads(): ChatThread[] {
  return readThreads().sort((a, b) => b.updatedAt - a.updatedAt);
}

/** Look up a single thread by id. Returns null if pruned. */
export function getThread(id: string): ChatThread | null {
  return readThreads().find((t) => t.id === id) ?? null;
}

/** Upsert. If id is null/empty, creates a new thread; otherwise
 *  updates the existing one. Returns the resolved thread (with id
 *  populated for the caller to pin to its in-memory state). */
export function upsertThread(args: {
  id?: string;
  title: string;
  modeId: string;
  messages: ChatMessage[];
}): ChatThread {
  const list = readThreads();
  const now = Date.now();
  // Cap messages per thread — drop oldest, keep newest. Same rule
  // sessionStorage uses in the overlay (30) but doubled because the
  // history is the long-term surface.
  const trimmed =
    args.messages.length > MAX_MESSAGES_PER_THREAD
      ? args.messages.slice(-MAX_MESSAGES_PER_THREAD)
      : args.messages;
  if (args.id) {
    const existing = list.find((t) => t.id === args.id);
    if (existing) {
      existing.messages = trimmed;
      existing.modeId = args.modeId;
      existing.updatedAt = now;
      // Don't rewrite title once set — first user message wins. The
      // user can always start a new thread if they wanted a different
      // framing.
      writeThreads(list);
      return existing;
    }
  }
  const id = args.id || now.toString(36);
  const thread: ChatThread = {
    id,
    title: args.title.slice(0, 80),
    modeId: args.modeId,
    messages: trimmed,
    createdAt: now,
    updatedAt: now
  };
  writeThreads([thread, ...list]);
  return thread;
}

/** Hard-delete a thread by id. Pinned messages from that thread
 *  are kept (they snapshot content) but their threadId becomes a
 *  dangling reference. Surface uses thread-still-exists as a UI hint. */
export function deleteThread(id: string): void {
  const list = readThreads().filter((t) => t.id !== id);
  writeThreads(list);
}

/** Search threads by case-insensitive substring match across title +
 *  message content. Returns matched threads with the matching message
 *  index pre-computed for highlighting in the picker. */
export interface ThreadSearchHit {
  thread: ChatThread;
  /** Index of the first message that matched, or -1 if only the title matched. */
  matchIndex: number;
  /** Short excerpt around the match (title or message body). */
  excerpt: string;
}

export function searchThreads(query: string): ThreadSearchHit[] {
  const q = query.trim().toLowerCase();
  if (!q) return [];
  const out: ThreadSearchHit[] = [];
  for (const thread of listThreads()) {
    if (thread.title.toLowerCase().includes(q)) {
      out.push({ thread, matchIndex: -1, excerpt: thread.title });
      continue;
    }
    let matched = false;
    for (let i = 0; i < thread.messages.length; i++) {
      const m = thread.messages[i];
      const at = m.content.toLowerCase().indexOf(q);
      if (at >= 0) {
        const start = Math.max(0, at - 60);
        const end = Math.min(m.content.length, at + q.length + 60);
        const prefix = start > 0 ? '…' : '';
        const suffix = end < m.content.length ? '…' : '';
        out.push({
          thread,
          matchIndex: i,
          excerpt: prefix + m.content.slice(start, end) + suffix
        });
        matched = true;
        break;
      }
    }
    if (!matched) continue;
  }
  return out;
}

// ─── Pinned messages ──────────────────────────────────────────────
// Flat list across all threads. Cap to MAX_PINNED so the user can't
// accidentally fill localStorage with megabytes of saved replies. If
// they hit the cap, oldest pin gets evicted.
const MAX_PINNED = 50;

function readPins(): PinnedMessage[] {
  const parsed = loadStored<unknown>(PINNED_KEY, []);
  if (!Array.isArray(parsed)) return [];
  return parsed.filter(
    (p): p is PinnedMessage =>
      p && typeof p.threadId === 'string' && typeof p.content === 'string'
  );
}

function writePins(list: PinnedMessage[]): void {
  const sorted = [...list].sort((a, b) => b.pinnedAt - a.pinnedAt).slice(0, MAX_PINNED);
  saveStored(PINNED_KEY, sorted);
}

export function listPinned(): PinnedMessage[] {
  return readPins().sort((a, b) => b.pinnedAt - a.pinnedAt);
}

/** Pin or unpin a specific (thread, message-index) combination.
 *  Returns the new pinned state (true = now pinned). */
export function togglePin(args: {
  threadId: string;
  threadTitle: string;
  modeId: string;
  messageIndex: number;
  content: string;
}): boolean {
  const list = readPins();
  const existing = list.findIndex(
    (p) => p.threadId === args.threadId && p.messageIndex === args.messageIndex
  );
  if (existing >= 0) {
    list.splice(existing, 1);
    writePins(list);
    return false;
  }
  list.unshift({
    threadId: args.threadId,
    threadTitle: args.threadTitle,
    modeId: args.modeId,
    messageIndex: args.messageIndex,
    content: args.content,
    pinnedAt: Date.now()
  });
  writePins(list);
  return true;
}

export function isPinned(threadId: string, messageIndex: number): boolean {
  return readPins().some(
    (p) => p.threadId === threadId && p.messageIndex === messageIndex
  );
}

/** Render a thread title from its first user message. Public so the
 *  overlay's "first user message lands" code path uses the same rule. */
export function deriveThreadTitle(messages: ChatMessage[]): string {
  const firstUser = messages.find((m) => m.role === 'user');
  if (!firstUser) return 'New conversation';
  const text = firstUser.content.replace(/\s+/g, ' ').trim();
  if (text.length <= 60) return text;
  return text.slice(0, 60) + '…';
}
