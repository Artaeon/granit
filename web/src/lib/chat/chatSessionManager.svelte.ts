// Chat-session orchestration for AIOverlay.
//
// Owns the streaming send pipeline: composer text → prelude assembly
// → SSE chat stream → tokens appended into the assistant message →
// finally (busy off, auto-save, scroll-to-bottom). Plus the
// supporting flows: cancelInflight, sendFollowup, clearChat.
//
// Extracted from AIOverlay because send() is the heart of the chat
// surface — too much state, too many race paths, too coupled to the
// abort lifecycle to leave inline. With it pulled out the parent is
// pure wiring: refs + the few callbacks the manager can't own.
//
// Three race fixes ship with this extraction. They were latent in
// the inlined version and called out in the workstream plan:
//
//   1. onError silenced once the controller is aborted. An old
//      stream whose fetch raced past abort() (browser already
//      returned the reader) used to overwrite the NEW assistant
//      message with `_error:_`. We now check `signal.aborted` first
//      and bail out without touching messages.
//
//   2. scrollHeight read AFTER Svelte renders the new chunk, not
//      before. Previously the 80px "stick to bottom" check measured
//      stale DOM (the chunk had been queued into the messages array
//      but not yet painted), so a fat chunk that grew the doc by
//      500px would yank the user back even after they'd scrolled
//      well outside the sticky band. Reading inside tick().then()
//      measures the new bottom against the latest paint.
//
//   3. New send() awaits any in-flight autoSaveThread before
//      starting. Sync today (localStorage), but the gate keeps the
//      session manager forward-compatible with an async save layer
//      (server thread sync / IndexedDB).
//
// Plus a fourth fix that surfaced during extraction: the finally
// block guards `if (abort === controller)` before clearing busy. A
// rapid second send aborts the first via abort?.abort() and installs
// its own controller; the first's finally must NOT flip busy=false
// or clear the abort handle, or the new send loses its kill switch.
//
// State convention: parent owns the reactive slots (input / busy /
// messages / mentionedRefs / lastRagHits / perTurnRagHits /
// quickTitle / quickResult) because the composer + message list +
// quick-action panel + thread persistence + history rail all read or
// write them too. The manager talks to those through `refs`. The
// AbortController is owned INTERNALLY — never exposed — so a
// quick-action cancel can't reach it (the abort-split shipped with
// PR A5). chatStream + buildPrelude are injected through opts so the
// manager has zero dependency on $lib/api at compile time — tests
// pass deterministic fakes without any vi.mock indirection.

import { tick } from 'svelte';
import type { ChatMessage } from '$lib/api';
import { rafThrottle } from '$lib/util/streamThrottle';
import type { RagHit } from './rag';
import type { MentionRef } from '$lib/components/MentionPicker.svelte';
import { record as recordSharedPrompt } from '$lib/ai/recentPrompts';

/** Result of one prelude build. The parent's closure captures all
 *  the policy (mode / memory / page context / loaders) and returns
 *  the three things send() needs: the system messages to prepend,
 *  the RAG hits to surface against the assistant turn, and the
 *  notePath param to forward to the chat stream. */
export interface PreludeBundle {
  messages: ChatMessage[];
  ragHits: RagHit[];
  notePathForStream: string | null;
}

export interface ChatStreamHandlers {
  onChunk: (chunk: string) => void;
  onDone?: () => void;
  onError?: (err: Error) => void;
}

/** Same shape as api.chatStream — re-declared here so the manager
 *  doesn't import $lib/api directly. Tests pass a controllable fake. */
export type ChatStreamFn = (
  history: ChatMessage[],
  notePath: string | undefined,
  handlers: ChatStreamHandlers,
  signal: AbortSignal
) => Promise<void>;

export interface ChatSessionRefs {
  input: string;
  busy: boolean;
  messages: ChatMessage[];
  mentionedRefs: MentionRef[];
  lastRagHits: RagHit[];
  perTurnRagHits: Record<number, RagHit[]>;
  /** Cleared at the start of every send so chat takes over the body
   *  pane from a previous quick-action result. */
  quickTitle: string;
  quickResult: string;
}

export interface ChatSessionManagerOptions {
  refs: ChatSessionRefs;
  /** Build the per-turn prelude. Closure captures the parent's mode /
   *  aiMemoryFacts / page context / loaders. */
  buildPrelude: (text: string, isFirstTurn: boolean) => Promise<PreludeBundle>;
  /** SSE chat-stream call. Injected so the manager has no $lib/api
   *  dependency — tests pass a controllable fake. */
  chatStream: ChatStreamFn;
  /** Returns true if `text` was a recognised slash command — send
   *  returns early without firing a stream. */
  handleSlashCommand: (text: string) => boolean;
  /** Persist the active thread to local storage (fire-and-forget). */
  autoSaveThread: () => void;
  /** Resolves once any in-flight autoSaveThread call has settled. */
  awaitSave: () => Promise<void>;
  /** Extra state reset for clearChat — pinnedIndex / expandedSources /
   *  activeThreadId etc. owned by the parent and not part of refs. */
  resetForClear: () => void;
  /** Scroll container for the smart-stick-to-bottom logic. */
  getScrollEl: () => HTMLElement | undefined;
}

export interface ChatSessionManager {
  send(): Promise<void>;
  sendFollowup(prompt: string): void;
  /** Aborts the in-flight stream (if any). The finally block in
   *  send() runs as normal — flushes the throttle, fires auto-save,
   *  scrolls. */
  cancelInflight(): void;
  /** Snapshot the current thread to history, then empty the body
   *  pane. The "Clear" toolbar button is wired to startNewThread on
   *  the history manager instead; this method exists because slash
   *  commands (/clear) call it. */
  clearChat(): void;
}

export function createChatSessionManager(
  opts: ChatSessionManagerOptions
): ChatSessionManager {
  const { refs } = opts;

  // Owned internally — NEVER exposed. Distinct from
  // quickActionService's controller so cancelling a quick action
  // can't touch a streaming chat send and vice versa (the abort-
  // split shipped with PR A5).
  let abort: AbortController | null = null;

  async function send(): Promise<void> {
    const text = refs.input.trim();
    if (!text || refs.busy) return;
    if (text.startsWith('/') && opts.handleSlashCommand(text)) return;

    // Race fix #3 — wait for the previous send's autoSaveThread to
    // settle BEFORE starting. Resolves on the next microtask today
    // (sync localStorage); becomes a real wait once the save layer
    // gains async work.
    await opts.awaitSave();

    refs.quickTitle = '';
    refs.quickResult = '';
    refs.busy = true;
    abort?.abort();
    const controller = new AbortController();
    abort = controller;
    const signal = controller.signal;

    const userMsg: ChatMessage = { role: 'user', content: text };
    // Non-fatal — the inline AI menu reads this list to offer chat
    // prompts as recents on a note surface.
    recordSharedPrompt({ prompt: text, source: 'chat' });

    const isFirstTurn = refs.messages.length === 0;
    const prelude = await opts.buildPrelude(text, isFirstTurn);
    refs.lastRagHits = prelude.ragHits;
    const payload = [...prelude.messages, ...refs.messages, userMsg];
    refs.messages = [
      ...refs.messages,
      userMsg,
      { role: 'assistant', content: '' }
    ];
    refs.input = '';
    // Refs were attached in the prelude; drop them so a follow-up
    // doesn't repeat the same context block.
    refs.mentionedRefs = [];
    let acc = '';
    const assistantIdx = refs.messages.length - 1;
    if (prelude.ragHits.length > 0) {
      refs.perTurnRagHits = {
        ...refs.perTurnRagHits,
        [assistantIdx]: prelude.ragHits.slice()
      };
    }

    try {
      // rAF throttle + smart-stick-to-bottom.
      //
      // Throttle commits at most once per animation frame so a fast
      // LLM emitting 100 tokens/sec doesn't trigger 100 re-renders
      // + 100 markdown re-parses. flush() forces a synchronous
      // commit at onDone / onError so the final buffer lands before
      // auto-save fires.
      //
      // Race fix #2 — read scrollHeight INSIDE tick().then(), AFTER
      // Svelte has rendered the latest chunk. Previously the read
      // happened before the chunk was painted, so the 80px sticky
      // band measured stale DOM. With the deferred read, a fat
      // chunk that grows the doc by 500px no longer yanks a user
      // who's already scrolled away.
      const t = rafThrottle((full) => {
        // Race fix #3 — drop a scheduled paint frame if the stream
        // was aborted between schedule and fire. Otherwise an
        // already-queued rAF callback from a stream we just
        // cancelled writes its stale accumulator into the NEW
        // send's assistant slot, briefly replacing the new partial
        // with the old one. signal.aborted is the same identity
        // the onDone/onError handlers below already gate on.
        if (signal.aborted) return;
        acc = full;
        refs.messages = refs.messages.map((m, i) =>
          i === assistantIdx ? { ...m, content: full } : m
        );
        tick().then(() => {
          const el = opts.getScrollEl();
          if (!el) return;
          const distToBottom =
            el.scrollHeight - el.scrollTop - el.clientHeight;
          if (distToBottom < 80) el.scrollTop = el.scrollHeight;
        });
      });

      await opts.chatStream(
        payload,
        prelude.notePathForStream ?? undefined,
        {
          onChunk: t.onChunk,
          onDone: () => {
            t.flush();
          },
          onError: (err) => {
            // Race fix #1 — silence onError once our own controller
            // is aborted. A new send installs a fresh assistant
            // slot; an old stream's late error must not overwrite
            // it with `_error:_`.
            if (signal.aborted) return;
            t.flush();
            refs.messages = refs.messages.map((m, i) =>
              i === assistantIdx
                ? { ...m, content: `_error:_ ${err.message}` }
                : m
            );
          }
        },
        signal
      );
    } finally {
      // Only flip busy + clear abort if this send is STILL the
      // latest one. A rapid resend aborts us and installs its own
      // controller; clobbering busy=false here would let the user
      // fire a third send before the second finished setup.
      if (abort === controller) {
        refs.busy = false;
        abort = null;
      }
      // Skip auto-save when the reply is empty (an aborted-before-
      // first-chunk send, or an error that ran before flush). The
      // history picker shouldn't grow a row for "u: hi / a: ''".
      if (acc.trim().length > 0) opts.autoSaveThread();
      tick().then(() => {
        const el = opts.getScrollEl();
        if (el) el.scrollTop = el.scrollHeight;
      });
    }
  }

  function sendFollowup(prompt: string): void {
    // Don't clobber the user's current input when a stream is in
    // flight — send() bails on `refs.busy`, but writing to refs.input
    // BEFORE the bail wipes whatever the user was typing while
    // waiting for the prior reply. Mirror the same guard at the
    // outer boundary so the input mutation is also blocked.
    if (refs.busy) return;
    refs.input = prompt;
    void send();
  }

  function cancelInflight(): void {
    abort?.abort();
  }

  function clearChat(): void {
    if (refs.messages.length === 0) return;
    // Snapshot first — "clear" shouldn't destroy a useful thread.
    // The user can still hard-delete from the history picker if
    // they really want it gone.
    opts.autoSaveThread();
    refs.messages = [];
    refs.quickTitle = '';
    refs.quickResult = '';
    refs.perTurnRagHits = {};
    opts.resetForClear();
  }

  return {
    send,
    sendFollowup,
    cancelInflight,
    clearChat
  };
}
