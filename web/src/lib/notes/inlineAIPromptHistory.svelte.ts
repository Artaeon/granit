// Prompt history controller for InlineAIMenu — extracted out of the
// menu component to keep that surface focused on layout + streaming.
// Owns three coupled pieces of state:
//
//   history       — per-note recents the user has actually sent from
//                   THIS surface. Capped at HISTORY_LIMIT (20). Backed
//                   by localStorage with a key derived from notePath
//                   so a parent that swaps notes without remounting
//                   the menu still gets the right bucket.
//   historyIdx    — Up/Down arrow cursor through `history`. -1 means
//                   "live input" (the user is typing fresh); 0 is the
//                   most recent entry. Reset whenever the user types,
//                   whenever the bucket changes, or whenever the menu
//                   is reopened.
//   crossRecents  — prompts the user wrote in the Cmd+J chat overlay
//                   that this note's per-note history hasn't already
//                   captured. Surfaces the "I phrased this question
//                   yesterday in chat" affordance one click away from
//                   the inline menu. Computed once at construction;
//                   not reactive because the menu's lifecycle is
//                   short (open + close per trigger).
//
// historyKey is reactive ($derived from notePath) so a remount-less
// note swap doesn't strand pushHistory writing to the wrong bucket.
// An $effect inside the factory keeps `history` and `historyIdx` in
// sync with the current bucket. Without this, the displayed recents
// would be note A's while pushHistory wrote to note B's slot.
//
// pushHistory ALSO writes to the shared recent-prompts log so the
// chat overlay can offer this prompt as a "recent across surfaces"
// pill. Non-fatal if that write fails — per-note history above is
// the primary record for the inline menu.

import { record as recordSharedPrompt, list as listSharedPrompts } from '$lib/ai/recentPrompts';

const HISTORY_LIMIT = 20;

export interface PromptHistoryController {
  /** Reactive — most-recent-first, capped at 20. */
  readonly history: string[];
  /** Reactive cursor through `history`. -1 = live input. */
  historyIdx: number;
  /** Reactive — chat-sourced recents the per-note list hasn't seen.
   *  Computed once via refreshCrossRecents(); the menu calls that
   *  on mount. Capped at 2 to keep the recents row compact. */
  readonly crossRecents: { prompt: string }[];
  /** Push a prompt to the front of history (and the shared log).
   *  De-dupes against existing entries. No-op on empty/whitespace. */
  pushHistory(prompt: string): void;
  /** Recompute the chat-sourced recents from the shared log. Called
   *  by the menu on mount; cheap, so the menu could call it again if
   *  it wanted reactivity, but in practice the menu's lifecycle is
   *  short and once-on-mount is enough. */
  refreshCrossRecents(): void;
}

export interface PromptHistoryOpts {
  /** Reactive — current note path. The controller derives its
   *  localStorage bucket key from this and rehydrates `history`
   *  whenever it changes. */
  getNotePath: () => string;
}

function readStorage(key: string): string[] {
  if (typeof window === 'undefined') return [];
  try {
    const raw = window.localStorage.getItem(key);
    if (!raw) return [];
    const arr = JSON.parse(raw);
    return Array.isArray(arr)
      ? arr.filter((x) => typeof x === 'string').slice(0, HISTORY_LIMIT)
      : [];
  } catch {
    return [];
  }
}

export function createPromptHistoryController(
  opts: PromptHistoryOpts
): PromptHistoryController {
  // historyKey is $derived from notePath so a parent reusing this
  // controller across notes (without remount) gets the right per-
  // note storage bucket.
  let historyKey = $derived(`granit.ai.history.${opts.getNotePath()}`);
  let history = $state<string[]>(readStorage(historyKey));
  let historyIdx = $state(-1);
  let crossRecents = $state<{ prompt: string }[]>([]);

  // Keep `history` in sync with `historyKey` if notePath changes
  // mid-life. Without this, the displayed recents would be note A's
  // while pushHistory wrote to note B's bucket.
  $effect(() => {
    void historyKey;
    history = readStorage(historyKey);
    historyIdx = -1;
  });

  function refreshCrossRecents() {
    const seen = new Set(history.map((h) => h.toLowerCase()));
    crossRecents = listSharedPrompts({ source: 'chat', limit: 6 })
      .filter((r) => !seen.has(r.prompt.toLowerCase()))
      .slice(0, 2);
  }

  function pushHistory(prompt: string) {
    const p = prompt.trim();
    if (!p) return;
    // De-dupe — push to front, drop existing copies elsewhere in
    // the list. Keeps the most-recent-first ordering monotonic.
    history = [p, ...history.filter((x) => x !== p)].slice(0, HISTORY_LIMIT);
    try {
      window.localStorage.setItem(historyKey, JSON.stringify(history));
    } catch {
      // localStorage can throw in private mode or when full; drop
      // persistence, keep the in-memory list usable.
    }
    // Also write to the shared recent-prompts log so the chat
    // overlay can offer this prompt as a recent. Non-fatal if the
    // log write fails for any reason.
    recordSharedPrompt({ prompt: p, source: 'inline', notePath: opts.getNotePath() });
  }

  return {
    get history() {
      return history;
    },
    get historyIdx() {
      return historyIdx;
    },
    set historyIdx(v: number) {
      historyIdx = v;
    },
    get crossRecents() {
      return crossRecents;
    },
    pushHistory,
    refreshCrossRecents
  };
}
