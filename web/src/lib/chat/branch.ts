// Pure helpers for AIOverlay's branch / regenerate / replay flow.
// Extracted out of the component so the small bits of state-free
// logic can be tested in isolation — the orchestrators themselves
// (replayFromUserMessage, regenAssistantMessage, branchFromMessage,
// submitEditUser) stay in AIOverlay.svelte because they mutate
// reactive $state inside closures, but the index math, title
// rules, and map pruning are pure and easy to silently break.

import type { ChatMessage } from '$lib/api';

/**
 * Find the user message that immediately precedes the assistant
 * message at `assistantIdx`. Walks backwards from assistantIdx-1
 * and returns the first user-role index, or -1 if there isn't one
 * (defensive — the caller should never have called this on a
 * malformed thread, but a -1 sentinel beats throwing).
 *
 * Used by regenAssistantMessage to figure out which user turn to
 * re-send.
 */
export function findPrecedingUserIndex(messages: ChatMessage[], assistantIdx: number): number {
  if (assistantIdx < 0 || assistantIdx >= messages.length) return -1;
  for (let j = assistantIdx - 1; j >= 0; j--) {
    if (messages[j].role === 'user') return j;
  }
  return -1;
}

/**
 * Build the title for a branched thread from the source thread's
 * title. Caps the source portion at 60 chars before appending
 * " (branch)" so the resulting title doesn't grow without bound
 * across repeated branchings of branches.
 *
 * 60 was chosen so "title (branch) (branch)" stays inside the
 * thread-picker's single-line render even at the narrowest
 * sidebar width.
 */
export function buildBranchTitle(sourceTitle: string): string {
  const cap = sourceTitle.length > 60 ? sourceTitle.slice(0, 60) : sourceTitle;
  return cap + ' (branch)';
}

/**
 * Drop entries whose numeric key is >= cutoff. Used to prune
 * per-turn data (RAG hits, expanded-sources flags) after a
 * replay/truncate so dangling keys don't point at messages that
 * no longer exist. Non-numeric keys are dropped too — they
 * shouldn't be in this record, but defensive against future
 * shape drift.
 *
 * Returns a new object; the caller assigns the result to its
 * reactive state. Stable enough to test against deep equal.
 */
export function pruneNumKeyedRecord<T>(rec: Record<number, T>, cutoff: number): Record<number, T> {
  const out: Record<number, T> = {};
  for (const [k, v] of Object.entries(rec)) {
    const n = Number(k);
    if (!Number.isFinite(n)) continue;
    if (n < cutoff) out[n] = v;
  }
  return out;
}
