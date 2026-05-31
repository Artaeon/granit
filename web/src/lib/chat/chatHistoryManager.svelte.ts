// Thread-history orchestration for AIOverlay.
//
// AIOverlay's thread CRUD (start new / load / delete / auto-save /
// replay / regen / branch / pin / edit-user) is ~250 LOC of glue that
// coordinates persisted history (./history.ts), branch math
// (./branch.ts), and 13 reactive state slots in the overlay. None of
// the glue is AI logic — it's just orchestration on top of helpers
// that already exist. Lifted here so:
//
//   1. The orchestration is unit-testable in isolation.
//   2. A future /chat page rewrite can call the same functions
//      (today /chat has its own simpler thread persistence).
//   3. AIOverlay loses 250 LOC of script noise.
//
// Why a `refs` object with getter/setter properties: Svelte 5 $state
// in a component must be reassigned through its original binding —
// you can't hand the parent a single setter. The cleanest pattern
// is an object that exposes each reactive slot as a getter/setter
// pair; the factory reads `refs.messages` (always live), writes
// `refs.messages = next` (calls the setter, which reassigns the
// parent's $state). The parent supplies this object once.
//
// The factory also owns one piece of internal state: `savingThread`,
// the race-condition gate that prevents auto-save from re-entering
// while a previous auto-save is mid-flight. Writes are tiny (one
// JSON.stringify of <60 messages) and synchronous in every modern
// browser's localStorage, but a re-entrant call from rapid send()
// chains could clobber pinned-index reads. The gate is cheap
// insurance.

import { tick } from 'svelte';
import type { ChatMessage } from '$lib/api';
import type { RagHit } from './rag';
import {
  getThread,
  upsertThread,
  deleteThread,
  togglePin,
  isPinned,
  deriveThreadTitle,
  persistActiveThreadId
} from './history';
import { findPrecedingUserIndex, buildBranchTitle, pruneNumKeyedRecord } from './branch';
import { persistModeId } from '$lib/ai/agents';
import { toast } from '$lib/components/toast';

// Reactive state slots the manager reads + writes. Parent supplies
// an object with getter/setter properties bound to its $state.
export interface ChatHistoryRefs {
  messages: ChatMessage[];
  activeThreadId: string;
  modeId: string;
  input: string;
  pinnedIndex: Record<number, boolean>;
  perTurnRagHits: Record<number, RagHit[]>;
  expandedSources: Record<number, boolean>;
  quickTitle: string;
  quickResult: string;
  lastRagHits: RagHit[];
  historyOpen: boolean;
  editingUserIdx: number | null;
  editingUserDraft: string;
}

// Side-effect bridges — concerns the manager cannot own itself.
export interface ChatHistoryManagerOptions {
  refs: ChatHistoryRefs;
  /** Live ref to the ChatHistoryRail component instance (or undefined
   *  if it isn't mounted). The manager calls .refresh() after pin /
   *  branch / delete so the rail's open tab reflects the new state. */
  getHistoryRail: () => { refresh(): void } | undefined;
  /** True while a chat send is streaming. Replay/regen/edit refuse
   *  while busy (would race the stream). */
  isBusy: () => boolean;
  /** Trigger the parent's send() — used during replay/regen/edit. */
  send: () => Promise<void> | void;
  /** Focus the composer textarea (tick-deferred — caller owns the
   *  ref, this just wraps inputEl?.focus()). */
  refocusInput: () => void;
  /** Scroll the message list to the bottom (tick-deferred). */
  scrollToBottom: () => void;
}

export interface ChatHistoryManager {
  /** True while an auto-save write is in flight. Read so nothing
   *  re-enters autoSaveThread before the previous call lands. */
  readonly saving: boolean;
  startNewThread(): void;
  loadSavedThread(id: string): void;
  deleteSavedThread(id: string): void;
  /** Idempotent. Returns immediately if nothing to save or a save is
   *  already in flight. */
  autoSaveThread(): void;
  /** Resolves once any in-flight autoSaveThread call has settled.
   *  ChatSessionManager awaits this at the START of every send() so a
   *  rapid resend can't race the previous turn's persistence write —
   *  important once the save layer gains async work (server sync,
   *  IndexedDB) beyond the current sync localStorage path. Resolves
   *  immediately when nothing is in flight. */
  awaitSave(): Promise<void>;
  replayFromUserMessage(userIdx: number, content: string): void;
  regenAssistantMessage(assistantIdx: number): void;
  branchFromMessage(idx: number): void;
  pinAssistantMessage(idx: number): void;
  startEditUser(idx: number): void;
  cancelEditUser(): void;
  submitEditUser(): void;
  /** Recompute pinnedIndex from localStorage. Called by the parent on
   *  thread switch + after pin toggle (the manager already calls it
   *  internally for those paths; exposed for the mount-time refresh). */
  refreshPinnedIndex(): void;
}

export function createChatHistoryManager(opts: ChatHistoryManagerOptions): ChatHistoryManager {
  const { refs } = opts;

  // ── savingThread race-gate (the race fix this PR ships) ────────
  // A rapid double-send used to be able to fire autoSaveThread twice
  // back-to-back. localStorage writes are sync in practice but the
  // intervening reads (deriveThreadTitle scan, mode persistence)
  // could race against pinnedIndex rehydration. The flag gates re-
  // entry so a save in flight blocks the next one — the in-flight
  // write owns the localStorage slot until it returns.
  let savingThread = $state(false);
  // Tracks the latest autoSaveThread call as a Promise. awaitSave()
  // returns it so consumers (chatSessionManager) can gate on the save
  // before starting new work. The wrapping IIFE makes the body
  // awaitable even though upsertThread is sync today — that way the
  // future async path (server sync, IndexedDB) doesn't need a second
  // refactor here.
  let savePromise: Promise<void> | null = null;

  function refreshPinnedIndex() {
    if (!refs.activeThreadId) {
      refs.pinnedIndex = {};
      return;
    }
    const next: Record<number, boolean> = {};
    for (let i = 0; i < refs.messages.length; i++) {
      if (refs.messages[i].role !== 'assistant') continue;
      if (isPinned(refs.activeThreadId, i)) next[i] = true;
    }
    refs.pinnedIndex = next;
  }

  function startNewThread() {
    // Snapshot current thread first so the user doesn't lose work.
    autoSaveThread();
    refs.messages = [];
    refs.activeThreadId = '';
    persistActiveThreadId('');
    refs.pinnedIndex = {};
    refs.quickTitle = '';
    refs.quickResult = '';
    refs.lastRagHits = [];
    refs.perTurnRagHits = {};
    opts.refocusInput();
  }

  function loadSavedThread(id: string) {
    autoSaveThread();
    const t = getThread(id);
    if (!t) {
      toast.error('Thread no longer exists.');
      opts.getHistoryRail()?.refresh();
      return;
    }
    refs.messages = t.messages.slice();
    refs.activeThreadId = t.id;
    persistActiveThreadId(t.id);
    if (t.modeId && t.modeId !== refs.modeId) {
      refs.modeId = t.modeId;
      persistModeId(t.modeId);
    }
    refs.quickTitle = '';
    refs.quickResult = '';
    refs.lastRagHits = [];
    refs.perTurnRagHits = {};
    refs.historyOpen = false;
    refreshPinnedIndex();
    opts.scrollToBottom();
    opts.refocusInput();
  }

  function deleteSavedThread(id: string) {
    deleteThread(id);
    if (refs.activeThreadId === id) {
      refs.activeThreadId = '';
      persistActiveThreadId('');
      refs.messages = [];
      refs.pinnedIndex = {};
    }
    opts.getHistoryRail()?.refresh();
  }

  // Auto-save the current thread to localStorage. Called after every
  // assistant turn lands and before swapping threads. Cheap — JSON-
  // stringify of <60 messages — but re-entrant guards via savingThread.
  function autoSaveThread() {
    if (savingThread) return;
    if (refs.messages.length === 0) return;
    // Only save once we have at least one user message AND one
    // assistant reply — empty-or-system-only threads aren't worth
    // a row in the picker.
    const hasUser = refs.messages.some((m) => m.role === 'user');
    const hasAssistant = refs.messages.some(
      (m) => m.role === 'assistant' && m.content.trim().length > 0
    );
    if (!hasUser || !hasAssistant) return;
    savingThread = true;
    savePromise = (async () => {
      try {
        const t = upsertThread({
          id: refs.activeThreadId || undefined,
          title: deriveThreadTitle(refs.messages),
          modeId: refs.modeId,
          messages: refs.messages
        });
        if (!refs.activeThreadId) {
          refs.activeThreadId = t.id;
          persistActiveThreadId(t.id);
        }
      } finally {
        savingThread = false;
      }
    })();
  }

  async function awaitSave(): Promise<void> {
    if (savePromise) await savePromise;
  }

  // ── Replay / regenerate / edit ────────────────────────────────
  // Three closely-related affordances on individual messages:
  //
  //   replayFromUserMessage — the shared kernel. Truncates the
  //     thread at the chosen user message, sets `input` to the
  //     content, calls send(). send() then re-pushes the user
  //     message, opens a fresh assistant slot, and streams. RAG /
  //     mentions / snapshot all run again as on any first-class
  //     send.
  //
  //   regenAssistantMessage — find the user message that produced
  //     this assistant reply, call replay with the same content.
  //     Same prompt, fresh attempt.
  //
  //   startEditUser / submitEditUser — inline-edit a user message
  //     and resubmit. Truncates everything after, the same way
  //     ChatGPT / Claude's web UIs do it.
  function replayFromUserMessage(userIdx: number, content: string) {
    if (opts.isBusy()) {
      toast.info('Wait for the current response to finish.');
      return;
    }
    if (userIdx < 0 || userIdx >= refs.messages.length || refs.messages[userIdx].role !== 'user') return;
    // Truncate everything from the user message onward — send() will
    // re-push the user message + open the assistant slot. Per-turn
    // data keyed by gone indices gets pruned so the inline-sources
    // block doesn't dangle pointing at messages that no longer exist.
    refs.messages = refs.messages.slice(0, userIdx);
    refs.perTurnRagHits = pruneNumKeyedRecord(refs.perTurnRagHits, userIdx);
    refs.expandedSources = pruneNumKeyedRecord(refs.expandedSources, userIdx);
    refs.input = content;
    void opts.send();
  }

  function regenAssistantMessage(assistantIdx: number) {
    if (assistantIdx < 0 || assistantIdx >= refs.messages.length) return;
    if (refs.messages[assistantIdx].role !== 'assistant') return;
    const userIdx = findPrecedingUserIndex(refs.messages, assistantIdx);
    if (userIdx === -1) return;
    replayFromUserMessage(userIdx, refs.messages[userIdx].content);
  }

  function startEditUser(idx: number) {
    if (opts.isBusy()) {
      toast.info('Wait for the current response to finish.');
      return;
    }
    if (refs.messages[idx]?.role !== 'user') return;
    refs.editingUserIdx = idx;
    refs.editingUserDraft = refs.messages[idx].content;
  }

  function cancelEditUser() {
    refs.editingUserIdx = null;
    refs.editingUserDraft = '';
  }

  function submitEditUser() {
    if (refs.editingUserIdx === null) return;
    const idx = refs.editingUserIdx;
    const content = refs.editingUserDraft.trim();
    // Cancel-on-empty: editing down to nothing is the same gesture
    // as cancel — preserves the original message rather than
    // submitting an empty turn.
    if (!content) {
      cancelEditUser();
      return;
    }
    // No-op when the user didn't actually change the text.
    if (content === refs.messages[idx].content) {
      cancelEditUser();
      return;
    }
    refs.editingUserIdx = null;
    refs.editingUserDraft = '';
    replayFromUserMessage(idx, content);
  }

  // Branch from a specific assistant message: copy the conversation
  // up to and including that message into a new thread, then load
  // it as the active thread. The original is preserved in history.
  function branchFromMessage(idx: number) {
    if (idx < 0 || idx >= refs.messages.length) return;
    if (refs.messages[idx].role !== 'assistant') return;
    // Persist the current thread as-is BEFORE forking so the fork
    // point lives in history (otherwise the source thread might
    // never have been saved if the user is fast).
    autoSaveThread();
    const upto = refs.messages.slice(0, idx + 1);
    const sourceTitle = refs.activeThreadId
      ? getThread(refs.activeThreadId)?.title ?? deriveThreadTitle(refs.messages)
      : deriveThreadTitle(refs.messages);
    const newTitle = buildBranchTitle(sourceTitle);
    const branched = upsertThread({
      title: newTitle,
      modeId: refs.modeId,
      messages: upto
    });
    refs.messages = upto.slice();
    refs.activeThreadId = branched.id;
    persistActiveThreadId(branched.id);
    refs.perTurnRagHits = {};
    refs.expandedSources = {};
    refreshPinnedIndex();
    if (refs.historyOpen) opts.getHistoryRail()?.refresh();
    toast.success('Branched into a new thread.');
    opts.scrollToBottom();
    opts.refocusInput();
  }

  function pinAssistantMessage(idx: number) {
    if (!refs.messages[idx] || refs.messages[idx].role !== 'assistant') return;
    // Make sure the thread is persisted before pinning so the pin can
    // reference a real thread id (the snapshotted content survives
    // the thread anyway, but the back-link is useful UX).
    autoSaveThread();
    const tid = refs.activeThreadId || 'orphan-' + Date.now().toString(36);
    const nowPinned = togglePin({
      threadId: tid,
      threadTitle: deriveThreadTitle(refs.messages),
      modeId: refs.modeId,
      messageIndex: idx,
      content: refs.messages[idx].content
    });
    refs.pinnedIndex = { ...refs.pinnedIndex, [idx]: nowPinned };
    // Rail tracks its own tab; if it's open, prod a refresh so a
    // toggled pin shows up in the Pinned tab without waiting for the
    // rail's own open-effect to re-fire.
    if (refs.historyOpen) opts.getHistoryRail()?.refresh();
  }

  return {
    get saving() {
      return savingThread;
    },
    startNewThread,
    loadSavedThread,
    deleteSavedThread,
    autoSaveThread,
    awaitSave,
    replayFromUserMessage,
    regenAssistantMessage,
    branchFromMessage,
    pinAssistantMessage,
    startEditUser,
    cancelEditUser,
    submitEditUser,
    refreshPinnedIndex
  };
}
