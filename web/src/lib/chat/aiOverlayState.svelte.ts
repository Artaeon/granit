// Shared mutable state for the AI overlay panel.
//
// Before this consolidation, AIOverlay maintained four separate
// getter/setter-pair refs objects — one per service (QuickAction,
// SaveNote, ChatHistory, ChatSession) — each wrapping the same
// underlying parent $state. ~85 LOC of boilerplate, and a constant
// risk of two refs disagreeing about whose getter to use.
//
// One container, four structural subsets. Each service's refs
// interface is now a subset of these fields, so the parent passes
// the same `aiState` object to every service — TypeScript accepts a
// wider object than the refs interface requires.
//
// Conventions preserved:
//   • Every mutable slot is exposed as a getter/setter pair so the
//     services keep their existing read/write idiom (refs.busy =
//     true / const m = refs.messages).
//   • $state lives inside the factory closure — never exported as a
//     raw variable. That keeps Svelte 5 reactivity intact across the
//     services + the parent template.
//   • Fields strictly local to the parent (modePickerOpen,
//     slashPickerOpen, mentionPickerOpen, liveRegion, attachNote,
//     attachSnapshot, committedActions, modePickerRef, historyRailRef,
//     scrollEl/inputEl/panelEl) STAY in the parent. They're either
//     UI-only or owned by the panel chrome and don't belong in the
//     service surface.

import type { ChatMessage } from '$lib/api';
import type { RagHit } from './rag';
import type { MentionRef } from '$lib/components/MentionPicker.svelte';
import { loadOverlayHistory } from './overlaySessionHistory';
import { loadActiveThreadId } from './history';

export interface AIOverlayState {
  // ── Chat session ─────────────────────────────────────────────────
  busy: boolean;
  input: string;
  messages: ChatMessage[];
  mentionedRefs: MentionRef[];

  // ── Quick-action result ──────────────────────────────────────────
  quickTitle: string;
  quickResult: string;

  // ── Save-as-note + library ───────────────────────────────────────
  saving: boolean;
  savingMessageIdx: number | null;
  copiedMessageIdx: number | null;
  savingLibraryIdx: number | null;
  savingLibraryLabel: string;
  savingLibraryBusy: boolean;

  // ── Thread history rail ──────────────────────────────────────────
  activeThreadId: string;
  historyOpen: boolean;
  pinnedIndex: Record<number, boolean>;
  editingUserIdx: number | null;
  editingUserDraft: string;

  // ── RAG attribution ──────────────────────────────────────────────
  lastRagHits: RagHit[];
  perTurnRagHits: Record<number, RagHit[]>;
  expandedSources: Record<number, boolean>;
}

export function createAIOverlayState(): AIOverlayState {
  let busy = $state(false);
  let input = $state('');
  let messages = $state<ChatMessage[]>(loadOverlayHistory());
  let mentionedRefs = $state<MentionRef[]>([]);

  let quickTitle = $state('');
  let quickResult = $state('');

  let saving = $state(false);
  let savingMessageIdx = $state<number | null>(null);
  let copiedMessageIdx = $state<number | null>(null);
  let savingLibraryIdx = $state<number | null>(null);
  let savingLibraryLabel = $state('');
  let savingLibraryBusy = $state(false);

  let activeThreadId = $state<string>(loadActiveThreadId());
  let historyOpen = $state(false);
  let pinnedIndex = $state<Record<number, boolean>>({});
  let editingUserIdx = $state<number | null>(null);
  let editingUserDraft = $state('');

  let lastRagHits = $state<RagHit[]>([]);
  let perTurnRagHits = $state<Record<number, RagHit[]>>({});
  let expandedSources = $state<Record<number, boolean>>({});

  return {
    get busy() { return busy; },
    set busy(v) { busy = v; },
    get input() { return input; },
    set input(v) { input = v; },
    get messages() { return messages; },
    set messages(v) { messages = v; },
    get mentionedRefs() { return mentionedRefs; },
    set mentionedRefs(v) { mentionedRefs = v; },

    get quickTitle() { return quickTitle; },
    set quickTitle(v) { quickTitle = v; },
    get quickResult() { return quickResult; },
    set quickResult(v) { quickResult = v; },

    get saving() { return saving; },
    set saving(v) { saving = v; },
    get savingMessageIdx() { return savingMessageIdx; },
    set savingMessageIdx(v) { savingMessageIdx = v; },
    get copiedMessageIdx() { return copiedMessageIdx; },
    set copiedMessageIdx(v) { copiedMessageIdx = v; },
    get savingLibraryIdx() { return savingLibraryIdx; },
    set savingLibraryIdx(v) { savingLibraryIdx = v; },
    get savingLibraryLabel() { return savingLibraryLabel; },
    set savingLibraryLabel(v) { savingLibraryLabel = v; },
    get savingLibraryBusy() { return savingLibraryBusy; },
    set savingLibraryBusy(v) { savingLibraryBusy = v; },

    get activeThreadId() { return activeThreadId; },
    set activeThreadId(v) { activeThreadId = v; },
    get historyOpen() { return historyOpen; },
    set historyOpen(v) { historyOpen = v; },
    get pinnedIndex() { return pinnedIndex; },
    set pinnedIndex(v) { pinnedIndex = v; },
    get editingUserIdx() { return editingUserIdx; },
    set editingUserIdx(v) { editingUserIdx = v; },
    get editingUserDraft() { return editingUserDraft; },
    set editingUserDraft(v) { editingUserDraft = v; },

    get lastRagHits() { return lastRagHits; },
    set lastRagHits(v) { lastRagHits = v; },
    get perTurnRagHits() { return perTurnRagHits; },
    set perTurnRagHits(v) { perTurnRagHits = v; },
    get expandedSources() { return expandedSources; },
    set expandedSources(v) { expandedSources = v; }
  };
}
