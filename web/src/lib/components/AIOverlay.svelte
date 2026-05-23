<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { fly, fade } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { api, type ChatMessage, type AIMemoryFact } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { sabbath } from '$lib/stores/sabbath';
  import {
    aiOverlayOpen,
    aiOverlayPinned,
    takeAIOverlaySeed,
    toggleAIOverlayPinned
  } from '$lib/stores/ai-overlay';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import { saveStoredString } from '$lib/util/storage';
  import {
    PANEL_WIDTH_MIN,
    PANEL_WIDTH_MAX,
    type SheetSnap,
    SHEET_SNAP_KEY,
    clampPanelWidth,
    loadPanelWidth,
    persistPanelWidth,
    nextPanelWidthForKey,
    loadSheetSnap,
    snapHeightPx,
    clampSheetHeight,
    nearestSnap
  } from './ai-overlay-geometry';
  import { rafThrottle } from '$lib/util/streamThrottle';
  import { isMobile } from '$lib/util/breakpoint';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import {
    AGENT_MODES,
    GENERIC_MODES,
    CONTEXTUAL_MODES,
    PERSONAS,
    findMode,
    loadModeId,
    persistModeId
  } from '$lib/ai/agents';
  import {
    loadProjectContext,
    renderProjectContext
  } from '$lib/ai/projectManagerContext';
  import {
    loadGoalContext,
    renderGoalContext
  } from '$lib/ai/goalManagerContext';
  import {
    loadCalendarContext,
    renderCalendarContext
  } from '$lib/ai/calendarManagerContext';
  import { deriveDraftTitle } from '$lib/ai/draftTitle';
  import {
    getThread,
    upsertThread,
    deleteThread,
    togglePin,
    isPinned,
    deriveThreadTitle,
    loadActiveThreadId,
    persistActiveThreadId,
    deriveLibraryLabel
  } from '$lib/chat/history';
  import { findPrecedingUserIndex, buildBranchTitle, pruneNumKeyedRecord } from '$lib/chat/branch';
  import { retrieveForRag, type RagHit } from '$lib/chat/rag';
  import {
    parseFollowups,
    parseActions,
    stripStructuredBlocks,
    actionKey,
    type ParsedAction
  } from '$lib/chat/actionParser';
  import { slugifyTitle } from '$lib/util/slug';
  import { todayISO } from '$lib/util/date';
  import {
    createSpeechRecognition,
    isSpeechRecognitionSupported,
    type SpeechRecognitionLike
  } from '$lib/util/speechRecognition';
  import SlashCommandPicker from '$lib/components/SlashCommandPicker.svelte';
  import MentionPicker, { type MentionRef } from '$lib/components/MentionPicker.svelte';
  import ChatHistoryRail from '$lib/components/ChatHistoryRail.svelte';
  import { focusOnMount } from '$lib/util/focusOnMount';
  import { hasActiveEditor, insertAtCursor } from '$lib/stores/active-editor';
  import { record as recordSharedPrompt, list as listSharedPrompts, type RecentPrompt } from '$lib/ai/recentPrompts';

  // AIOverlay — global AI panel. Slides in from the right on
  // desktop, becomes a bottom sheet on mobile. Triggered with
  // Mod+J from anywhere (and Esc to close). The body is split
  // into two modes:
  //   1. Quick actions  — four buttons that dispatch the existing
  //      Tier 1 features (briefing / triage / deadlines /
  //      synopsis). Result renders inline as markdown or a JSON
  //      block.
  //   2. Chat — a streaming conversation against the configured
  //      LLM via /api/v1/chat/stream. History is in-memory only
  //      so the overlay stays disposable; the dedicated /chat page
  //      is the place for long-running threads + saving.
  //
  // The component listens for its own keyboard shortcut so the
  // layout doesn't have to know it exists — drop a single
  // <AIOverlay /> in +layout.svelte and you're done.

  // open is a $derived view of the global store so any UI surface
  // (sidebar button, command palette, future mobile entry) can flip
  // the overlay without prop-drilling. We write back via store
  // setters when the user closes / Mod+J-toggles, keeping the
  // store as the single source of truth.
  // When pinned on desktop, the panel is always rendered (forced
  // open) so it acts like a permanent sidebar. Otherwise it follows
  // the store flag. Pinned state on mobile is ignored (the sheet
  // gesture model would conflict with a "permanent" sidebar).
  const open = $derived($aiOverlayOpen || $aiOverlayPinned);
  let panelEl: HTMLDivElement | undefined = $state();
  let inputEl: HTMLTextAreaElement | undefined = $state();
  let scrollEl: HTMLDivElement | undefined = $state();

  // ── Mobile body-scroll lock ──────────────────────────────────
  // When the sheet is open on mobile, lock the document body so iOS
  // Safari can't scroll the page to bring the focused composer above
  // the keyboard — that scroll is what historically dragged the
  // position: fixed panel up with it, leaving the input mid-screen
  // with a fat gap to the keyboard top. Paired with the
  // `interactive-widget=resizes-content` viewport meta this kills
  // the bug on every supported iOS / Android browser. The classic
  // "save scrollY → fix body → restore" recipe so the user lands
  // back at the same scroll position when the overlay closes.
  //
  // Desktop (md+) is exempt: the panel is a side rail, not a sheet,
  // and locking body scroll there would be obnoxious. The breakpoint
  // is tracked reactively via $lib/util/breakpoint so an iPad rotate
  // (portrait/landscape crosses the 768px line) or a touch-laptop
  // resize re-runs this effect and the body lock follows the new
  // mode — previously we read matchMedia once at effect time and a
  // user crossing the breakpoint with the overlay open would have
  // the body stay locked on desktop or unlocked on mobile.
  let savedScrollY = 0;
  $effect(() => {
    if (typeof window === 'undefined') return;
    // Pinned desktop panel doesn't trigger this lock; mobile pinned
    // isn't a thing (pinned is forced false on mobile breakpoints).
    if (open && $isMobile && !$aiOverlayPinned) {
      savedScrollY = window.scrollY;
      const body = document.body;
      const html = document.documentElement;
      // Body lock (fixed + saved scrollY) — the load-bearing piece.
      body.style.position = 'fixed';
      body.style.top = `-${savedScrollY}px`;
      body.style.left = '0';
      body.style.right = '0';
      body.style.width = '100%';
      // html overflow:hidden as second-stage defence against iOS rubber-
      // band scrolling that can leak through a fixed body in some
      // Safari builds (the touch-action: none alternative is too
      // aggressive — it would kill tap handling on the panel itself).
      html.style.overflow = 'hidden';
      return () => {
        body.style.position = '';
        body.style.top = '';
        body.style.left = '';
        body.style.right = '';
        body.style.width = '';
        html.style.overflow = '';
        window.scrollTo(0, savedScrollY);
      };
    }
  });

  // ── Resizable panel (desktop only) ──────────────────────────────
  // Geometry helpers + constants live in ./ai-overlay-geometry.ts.
  // This component owns the panel's reactive $state (panelWidth /
  // resizing / sheetSnap / sheetDragging) and the pointer-event
  // handlers; the math + load/persist + snap rules come from the
  // helper.
  let panelWidth = $state<number>(loadPanelWidth());
  let resizing = $state(false);

  // When pinned, push the current panel width up to documentElement
  // so +layout.svelte can reserve a matching right gutter on <main>
  // via the --ai-pinned-w variable. Cleared (set to 0) when unpinned
  // so content reclaims the space.
  $effect(() => {
    if (typeof document === 'undefined') return;
    if ($aiOverlayPinned) {
      document.documentElement.style.setProperty('--ai-pinned-w', `${panelWidth}px`);
    } else {
      document.documentElement.style.setProperty('--ai-pinned-w', '0px');
    }
  });
  function onResizeStart(e: PointerEvent) {
    // Pointer-events lets one handler cover mouse + touch + pen. We
    // capture so the user can drag past the panel edge without losing
    // the gesture if their cursor strays into the chat content.
    e.preventDefault();
    resizing = true;
    const target = e.currentTarget as HTMLElement;
    target.setPointerCapture(e.pointerId);
    function onMove(ev: PointerEvent) {
      // Panel is right-anchored; widening means pulling LEFT, which
      // increases (window.innerWidth - clientX). Clamp via helper.
      panelWidth = clampPanelWidth(window.innerWidth - ev.clientX);
    }
    function onUp() {
      resizing = false;
      target.releasePointerCapture(e.pointerId);
      target.removeEventListener('pointermove', onMove);
      target.removeEventListener('pointerup', onUp);
      target.removeEventListener('pointercancel', onUp);
      persistPanelWidth(panelWidth);
    }
    target.addEventListener('pointermove', onMove);
    target.addEventListener('pointerup', onUp);
    target.addEventListener('pointercancel', onUp);
  }
  function onResizeKey(e: KeyboardEvent) {
    // Keyboard fallback for accessibility — ArrowLeft widens (right-
    // anchored panel), ArrowRight narrows; Home/End jump to extremes.
    // Rule lives in nextPanelWidthForKey so both surfaces (mouse drag,
    // keyboard) agree on bounds.
    const next = nextPanelWidthForKey(panelWidth, e.key);
    if (next === null) return;
    e.preventDefault();
    panelWidth = next;
    persistPanelWidth(next);
  }

  // ── Mobile bottom-sheet snap points ────────────────────────────
  // Three snap heights — peek (~35%), mid (~65%), full (~92%) —
  // defined in ai-overlay-geometry.ts. The drag handle bar at the
  // top of the sheet is the affordance; release snaps to nearest.
  // Desktop ignores all this (the left-edge resize handle keeps
  // governing width).
  let sheetSnap = $state<SheetSnap>(loadSheetSnap());
  $effect(() => saveStoredString(SHEET_SNAP_KEY, sheetSnap));
  // While the user drags, follow the finger 1:1. On release the
  // pointerup handler picks the nearest snap and zeroes this out.
  let sheetDragHeight = $state<number | null>(null);
  let sheetDragging = $state(false);

  function onSheetHandleDown(e: PointerEvent) {
    // Desktop has its own left-edge resize handle; ignore the
    // mobile-only drag-handle on >=md.
    if (typeof window === 'undefined' || window.innerWidth >= 768) return;
    e.preventDefault();
    const startY = e.clientY;
    const viewportH = window.innerHeight;
    const startH = panelEl?.getBoundingClientRect().height ?? snapHeightPx(sheetSnap, viewportH);
    sheetDragging = true;
    sheetDragHeight = startH;
    const target = e.currentTarget as HTMLElement;
    target.setPointerCapture(e.pointerId);
    function move(ev: PointerEvent) {
      // Pulling UP grows the sheet; clientY decreases as the finger
      // moves up, so dy is negative and we subtract. Clamp via helper.
      const dy = ev.clientY - startY;
      sheetDragHeight = clampSheetHeight(startH - dy, viewportH);
    }
    function up() {
      target.releasePointerCapture(e.pointerId);
      target.removeEventListener('pointermove', move);
      target.removeEventListener('pointerup', up);
      target.removeEventListener('pointercancel', up);
      // Snap to nearest. window.innerHeight read fresh in case a soft
      // keyboard shifted vh between drag start and end.
      const finalH = sheetDragHeight ?? startH;
      sheetSnap = nearestSnap(finalH, window.innerHeight);
      sheetDragHeight = null;
      sheetDragging = false;
    }
    target.addEventListener('pointermove', move);
    target.addEventListener('pointerup', up);
    target.addEventListener('pointercancel', up);
  }

  // ── iOS keyboard-safe compose ──────────────────────────────────
  // Without this, iOS Safari floats the on-screen keyboard OVER the
  // bottom-anchored panel, hiding the compose textarea. visualViewport
  // shrinks when the keyboard opens; the delta against window.innerHeight
  // is the obscured strip height. We lift the panel's `bottom` by that
  // amount so the compose stays just above the keyboard. Also snap to
  // full so the user gets the whole conversation visible while typing.
  let keyboardOffset = $state(0);
  let keyboardOpen = $state(false);
  $effect(() => {
    if (typeof window === 'undefined') return;
    const vv = window.visualViewport;
    if (!vv) return;
    function update() {
      const obscured = Math.max(0, window.innerHeight - (vv?.height ?? window.innerHeight));
      keyboardOffset = obscured;
      // ~120px threshold = the keyboard is up (URL bar / floating UI
      // can shrink VV by 40–80px in normal scroll without the keyboard
      // being involved; 120 cleanly separates "keyboard" from "chrome").
      keyboardOpen = obscured > 120;
    }
    vv.addEventListener('resize', update);
    vv.addEventListener('scroll', update);
    update();
    return () => {
      vv.removeEventListener('resize', update);
      vv.removeEventListener('scroll', update);
    };
  });

  // Snap to full whenever the keyboard pops up — minimises the
  // moment-of-friction where the user taps in compose and then can
  // barely see their own conversation history. Restores to whatever
  // they had before once the keyboard closes (we don't write back to
  // sheetSnap during keyboard-open so the restore is implicit).
  let savedSnapBeforeKeyboard: SheetSnap | null = $state(null);
  $effect(() => {
    if (keyboardOpen) {
      if (savedSnapBeforeKeyboard === null) {
        savedSnapBeforeKeyboard = sheetSnap;
        sheetSnap = 'full';
      }
    } else if (savedSnapBeforeKeyboard !== null) {
      sheetSnap = savedSnapBeforeKeyboard;
      savedSnapBeforeKeyboard = null;
    }
  });

  // The actual height we render at — drag value while a drag is in
  // flight, otherwise the snap target. Empty string lets the desktop
  // override class (md:h-full md:max-h-none) win unchanged.
  //
  // Why visualViewport.height + not innerHeight - keyboardOffset:
  // iOS Safari keeps window.innerHeight FIXED when the keyboard
  // opens (only visualViewport shrinks). Chrome Android in recent
  // versions does the same. So `innerHeight - keyboardOffset` is
  // correct on iOS but double-subtracts on Android (where the
  // keyboard already isn't in innerHeight). The earlier fix worked
  // on iOS Safari, broke on Android Chrome: panel collapsed to
  // ~innerHeight - 2×keyboardOffset, input field floated mid-
  // screen with a big gap above the keyboard.
  //
  // visualViewport.height is the visible viewport on every modern
  // mobile browser — the area between viewport top and keyboard
  // top (or just viewport bottom when keyboard is closed). Using
  // it as the snap-base gives "fill the room I have" semantics
  // that work everywhere.
  let mobileSheetHeight = $derived.by(() => {
    if (typeof window === 'undefined') return `${snapHeightPx(sheetSnap, 800)}px`;
    if (sheetDragging && sheetDragHeight !== null) {
      return `${Math.round(sheetDragHeight)}px`;
    }
    // visualViewport.height = visible-above-keyboard space on iOS +
    // Android. Falls back to innerHeight on desktop / older browsers
    // where the var doesn't exist.
    const visibleH = window.visualViewport?.height ?? window.innerHeight;
    return `${snapHeightPx(sheetSnap, visibleH)}px`;
  });

  let busy = $state(false);
  let abort: AbortController | null = null;

  // Status pill — what model the chat / actions will route to.
  let statusInfo = $state<{ provider: string; model: string; sabbath: boolean } | null>(null);

  // Quick-action result. Cleared every time the user fires a new
  // action OR sends a chat message (chat takes over the body).
  let quickTitle = $state('');
  let quickResult = $state('');

  // Chat history — persisted to sessionStorage so closing the
  // overlay (Esc / outside-click / Mod+J) doesn't lose the
  // thread. Survives navigation within the tab; cleared on tab
  // close or explicit reset. The full /chat page is still the
  // place for save-as-note and long-running multi-day threads;
  // this layer keeps a quick question alive long enough to come
  // back to it after a tangent.
  const HISTORY_KEY = 'granit.ai.overlay.messages';
  function loadHistory(): ChatMessage[] {
    if (typeof sessionStorage === 'undefined') return [];
    try {
      const raw = sessionStorage.getItem(HISTORY_KEY);
      if (!raw) return [];
      const parsed = JSON.parse(raw);
      if (!Array.isArray(parsed)) return [];
      return parsed.filter(
        (m): m is ChatMessage =>
          m && typeof m === 'object' && typeof m.role === 'string' && typeof m.content === 'string'
      );
    } catch {
      return [];
    }
  }
  function persistHistory(list: ChatMessage[]) {
    if (typeof sessionStorage === 'undefined') return;
    try {
      // Cap to ~30 messages to keep sessionStorage tidy. Older
      // turns drop quietly; the user is unlikely to want a
      // 100-turn quick-overlay thread (that's what /chat is for).
      const trimmed = list.length > 30 ? list.slice(-30) : list;
      sessionStorage.setItem(HISTORY_KEY, JSON.stringify(trimmed));
    } catch {}
  }
  let messages = $state<ChatMessage[]>(loadHistory());
  let input = $state('');
  $effect(() => {
    void messages.length;
    persistHistory(messages);
  });

  // Cross-source recents the chat overlay surfaces above the composer
  // on a fresh thread — prompts the user wrote in the inline AI menu
  // on a note. Computed once on open + after each send so a chat reply
  // doesn't blank the recents until the user starts a new thread.
  // Capped at 3 so it doesn't crowd the composer; source='inline' so
  // we don't echo the user's own chat prompts.
  let crossRecentInlinePrompts = $state<RecentPrompt[]>([]);
  function refreshCrossRecentInlinePrompts() {
    crossRecentInlinePrompts = listSharedPrompts({ source: 'inline', limit: 3 });
  }
  // Re-derive whenever the conversation state changes — opens a fresh
  // thread → recents become visible again; sends a message → fine to
  // leave as-is since the strip won't render until messages clears.
  $effect(() => {
    void messages.length;
    refreshCrossRecentInlinePrompts();
  });

  // ── Save-to-library state ───────────────────────────────────────
  // The "+" button next to a user message opens an inline form for
  // a short label. On submit we GET the current library, append a
  // new entry with this message's content as the prompt, PUT the
  // whole thing back. Per-message rather than per-thread because the
  // user might want to save several specific prompts from one chat.
  let savingLibraryIdx = $state<number | null>(null);
  let savingLibraryLabel = $state('');
  let savingLibraryBusy = $state(false);
  function openSaveLibrary(idx: number, content: string) {
    savingLibraryIdx = idx;
    // Seed label with the first few words so the user has something
    // to edit rather than starting from blank — most labels are a
    // quick tweak of the seed. Helper lives in chat/history.
    savingLibraryLabel = deriveLibraryLabel(content);
  }
  function cancelSaveLibrary() {
    savingLibraryIdx = null;
    savingLibraryLabel = '';
  }
  async function confirmSaveLibrary(promptContent: string) {
    const label = savingLibraryLabel.trim();
    if (!label || savingLibraryBusy) return;
    savingLibraryBusy = true;
    try {
      const cur = await api.getAIPrompts();
      // 'either' as the default scope — the user can edit scope later
      // if they want to constrain when the entry surfaces. Most chat
      // prompts apply equally to selection + cursor surfaces.
      const next = {
        entries: [
          ...(cur.entries ?? []),
          { label, prompt: promptContent.trim(), scope: 'either' as const }
        ]
      };
      await api.putAIPrompts(next);
      toast.success(`Saved "${label}" to library`);
      cancelSaveLibrary();
    } catch (err) {
      toast.error('save failed: ' + errorMessage(err));
    } finally {
      savingLibraryBusy = false;
    }
  }

  // ── Long-term thread history (localStorage, LRU 30) ──────────────
  // sessionStorage above is the in-flight buffer (cleared on tab
  // close); this layer survives tab close + browser restart. Threads
  // get auto-saved on every full user→assistant exchange so the user
  // doesn't have to remember to "save". A "new thread" button stashes
  // the current state and starts fresh; the picker below restores
  // any past thread.
  // Active-thread-id persistence shims + the key constant live in
  // $lib/chat/history alongside the rest of the thread store. Same
  // sessionStorage scope (per-tab) so a duplicated tab gets its
  // own draft conversation.
  let activeThreadId = $state<string>(loadActiveThreadId());
  let historyOpen = $state(false);
  let historyRailRef: ChatHistoryRail | undefined = $state();
  // Pinned-state for the current thread's assistant messages, recomputed
  // on thread change + pin toggle. Keyed by message index. Avoids hitting
  // localStorage on every render of the chat list.
  let pinnedIndex = $state<Record<number, boolean>>({});

  function refreshPinnedIndex() {
    if (!activeThreadId) {
      pinnedIndex = {};
      return;
    }
    const next: Record<number, boolean> = {};
    for (let i = 0; i < messages.length; i++) {
      if (messages[i].role !== 'assistant') continue;
      if (isPinned(activeThreadId, i)) next[i] = true;
    }
    pinnedIndex = next;
  }

  function startNewThread() {
    // Snapshot current thread first so the user doesn't lose work.
    autoSaveThread();
    messages = [];
    activeThreadId = '';
    persistActiveThreadId('');
    pinnedIndex = {};
    quickTitle = '';
    quickResult = '';
    lastRagHits = [];
    perTurnRagHits = {};
    tick().then(() => inputEl?.focus());
  }

  function loadSavedThread(id: string) {
    autoSaveThread();
    const t = getThread(id);
    if (!t) {
      toast.error('Thread no longer exists.');
      historyRailRef?.refresh();
      return;
    }
    messages = t.messages.slice();
    activeThreadId = t.id;
    persistActiveThreadId(t.id);
    if (t.modeId && t.modeId !== modeId) {
      modeId = t.modeId;
      persistModeId(t.modeId);
    }
    quickTitle = '';
    quickResult = '';
    lastRagHits = [];
    perTurnRagHits = {};
    historyOpen = false;
    refreshPinnedIndex();
    tick().then(() => {
      if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
      inputEl?.focus();
    });
  }

  function deleteSavedThread(id: string) {
    deleteThread(id);
    if (activeThreadId === id) {
      activeThreadId = '';
      persistActiveThreadId('');
      messages = [];
      pinnedIndex = {};
    }
    historyRailRef?.refresh();
  }

  // Auto-save the current thread to localStorage. Called after every
  // assistant turn lands (so the thread is visible in history even if
  // the user closes the overlay mid-conversation) and before swapping
  // threads. Cheap — JSON-stringify of <60 messages.
  function autoSaveThread() {
    if (messages.length === 0) return;
    // Only save once we have at least one user message AND one
    // assistant reply — empty-or-system-only threads aren't worth
    // a row in the picker.
    const hasUser = messages.some((m) => m.role === 'user');
    const hasAssistant = messages.some(
      (m) => m.role === 'assistant' && m.content.trim().length > 0
    );
    if (!hasUser || !hasAssistant) return;
    const t = upsertThread({
      id: activeThreadId || undefined,
      title: deriveThreadTitle(messages),
      modeId,
      messages
    });
    if (!activeThreadId) {
      activeThreadId = t.id;
      persistActiveThreadId(t.id);
    }
  }

  // Branch from a specific assistant message: copy the conversation
  // up to and including that message into a new thread, then load it
  // as the active thread. The original is preserved in history so a
  // user can come back to it. Useful when the user wants to ask a
  // different follow-up without losing the path they already took.
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
  //     Same prompt, fresh attempt — covers the standard "give me
  //     a different answer" affordance.
  //
  //   startEditUser / submitEditUser — inline-edit a user message
  //     and resubmit. Truncates everything after, the same way
  //     ChatGPT / Claude's web UIs do it.
  function replayFromUserMessage(userIdx: number, content: string) {
    if (busy) {
      toast.info('Wait for the current response to finish.');
      return;
    }
    if (userIdx < 0 || userIdx >= messages.length || messages[userIdx].role !== 'user') return;
    // Truncate everything from the user message onward — send() will
    // re-push the user message + open the assistant slot. Per-turn
    // data keyed by gone indices gets pruned so the inline-sources
    // block doesn't dangle pointing at messages that no longer exist.
    messages = messages.slice(0, userIdx);
    perTurnRagHits = pruneNumKeyedRecord(perTurnRagHits, userIdx);
    expandedSources = pruneNumKeyedRecord(expandedSources, userIdx);
    input = content;
    void send();
  }

  function regenAssistantMessage(assistantIdx: number) {
    if (assistantIdx < 0 || assistantIdx >= messages.length) return;
    if (messages[assistantIdx].role !== 'assistant') return;
    const userIdx = findPrecedingUserIndex(messages, assistantIdx);
    if (userIdx === -1) return;
    replayFromUserMessage(userIdx, messages[userIdx].content);
  }

  // Inline-edit state for user messages. Null = nobody being edited.
  let editingUserIdx = $state<number | null>(null);
  let editingUserDraft = $state('');
  function startEditUser(idx: number) {
    if (busy) {
      toast.info('Wait for the current response to finish.');
      return;
    }
    if (messages[idx]?.role !== 'user') return;
    editingUserIdx = idx;
    editingUserDraft = messages[idx].content;
  }
  function cancelEditUser() {
    editingUserIdx = null;
    editingUserDraft = '';
  }
  function submitEditUser() {
    if (editingUserIdx === null) return;
    const idx = editingUserIdx;
    const content = editingUserDraft.trim();
    // Cancel-on-empty: editing down to nothing is the same gesture
    // as cancel — preserves the original message rather than
    // submitting an empty turn.
    if (!content) {
      cancelEditUser();
      return;
    }
    // No-op when the user didn't actually change the text.
    if (content === messages[idx].content) {
      cancelEditUser();
      return;
    }
    editingUserIdx = null;
    editingUserDraft = '';
    replayFromUserMessage(idx, content);
  }

  function branchFromMessage(idx: number) {
    if (idx < 0 || idx >= messages.length) return;
    if (messages[idx].role !== 'assistant') return;
    // Persist the current thread as-is BEFORE forking so the fork
    // point lives in history (otherwise the source thread might
    // never have been saved if the user is fast).
    autoSaveThread();
    // Slice INCLUDING the assistant message at idx, so the user sees
    // the same context as before and the branch starts fresh from
    // that point.
    const upto = messages.slice(0, idx + 1);
    const sourceTitle = activeThreadId
      ? getThread(activeThreadId)?.title ?? deriveThreadTitle(messages)
      : deriveThreadTitle(messages);
    const newTitle = buildBranchTitle(sourceTitle);
    const branched = upsertThread({
      // No id ⇒ new thread.
      title: newTitle,
      modeId,
      messages: upto
    });
    messages = upto.slice();
    activeThreadId = branched.id;
    persistActiveThreadId(branched.id);
    perTurnRagHits = {};
    expandedSources = {};
    refreshPinnedIndex();
    if (historyOpen) historyRailRef?.refresh();
    toast.success('Branched into a new thread.');
    tick().then(() => {
      if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
      inputEl?.focus();
    });
  }

  function pinAssistantMessage(idx: number) {
    if (!messages[idx] || messages[idx].role !== 'assistant') return;
    // Make sure the thread is persisted before pinning so the pin can
    // reference a real thread id (the snapshotted content survives the
    // thread anyway, but the back-link is useful UX).
    autoSaveThread();
    const tid = activeThreadId || 'orphan-' + Date.now().toString(36);
    const nowPinned = togglePin({
      threadId: tid,
      threadTitle: deriveThreadTitle(messages),
      modeId,
      messageIndex: idx,
      content: messages[idx].content
    });
    pinnedIndex = { ...pinnedIndex, [idx]: nowPinned };
    // Rail tracks its own tab; if it's open, prod a refresh so a
    // toggled pin shows up in the Pinned tab without waiting for the
    // rail's own open-effect to re-fire.
    if (historyOpen) historyRailRef?.refresh();
  }

  // ── Page-aware context ──────────────────────────────────────────
  // Two attach modes, mutually exclusive depending on the route:
  //
  //   /notes/<path>  → attachNote  (server expands the note body
  //                    into a system prompt via chatStream's
  //                    notePath param — see handlers_chat.go).
  //   anywhere else  → attachSnapshot  (fetches the Context Engine
  //                    snapshot and prepends a system message
  //                    with today's events + open tasks +
  //                    recent notes + active goals + deadlines).
  //
  // Mutual exclusion keeps the prompt clean: a notes page already
  // has a primary doc; non-note surfaces benefit from the broader
  // "what's going on right now" view that the snapshot provides.
  let attachSnapshot = $state(true);

  // ── Agent modes + RAG ──────────────────────────────────────────
  // Mode = posture (system prompt). RAG = grounding (retrieved
  // notes prepended as context). They're independent: a Writer mode
  // user might want vault retrieval for facts; a Research mode user
  // might want bare LLM if working with a paper they pasted in.
  // The mode picker is the headline UX; RAG is a secondary toggle
  // that defaults from the mode's preference but the user overrides.
  let modeId = $state<string>(loadModeId());
  let mode = $derived(findMode(modeId));
  // Tracks the context-driven auto-switch state. Four values:
  //   - ''           : user's chosen mode is in effect (persisted)
  //   - 'project'    : project-manager (on a project page)
  //   - 'goal'       : goal-manager   (focused goal on /goals)
  //   - 'calendar'   : calendar-manager (on /calendar)
  // Each contextual switch is NOT persisted; leaving the context
  // reverts to loadModeId() so the user's normal preference
  // doesn't get clobbered.
  let autoMode = $state<'' | 'project' | 'goal' | 'calendar'>('');

  // Tracks which page-context we already auto-switched FOR.
  // Without this, the effect loops when the user manually picks
  // a different mode while on a project page: selectMode clears
  // autoMode, the effect re-runs, sees `modeId !== 'project-manager'`
  // and yanks them right back into PM. With this guard, we only
  // auto-switch ONCE per page-context entry — after the user has
  // chosen (or accepted) a mode for this page, no more nagging.
  // Cleared on context exit so re-entering still triggers the
  // initial auto-switch.
  let lastAutoSwitchedFor = $state<string>('');

  $effect(() => {
    // Precedence: most-specific entity wins. Project > goal >
    // calendar — only one is ever truly active in practice.
    const inProject = !!currentProjectName;
    const inGoal = !inProject && !!currentGoalId;
    const inCalendar = !inProject && !inGoal && onCalendarPage;
    const key = inProject
      ? `project:${currentProjectName}`
      : inGoal
      ? `goal:${currentGoalId}`
      : inCalendar
      ? 'calendar'
      : '';

    if (key) {
      if (lastAutoSwitchedFor !== key) {
        // First entry into this specific page-context. Switch
        // mode + remember we did so for this key.
        const targetMode = inProject
          ? 'project-manager'
          : inGoal
          ? 'goal-manager'
          : 'calendar-manager';
        if (modeId !== targetMode) {
          autoMode = inProject ? 'project' : inGoal ? 'goal' : 'calendar';
          modeId = targetMode;
        }
        lastAutoSwitchedFor = key;
      }
      // Otherwise: user has already engaged this page-context.
      // Their mode choice (PM, or whatever they picked manually)
      // sticks until they leave.
    } else {
      // Out of every context. Revert if we're still parked in an
      // auto-set mode; otherwise leave the user's mode alone.
      if (
        autoMode &&
        (modeId === 'project-manager' ||
          modeId === 'goal-manager' ||
          modeId === 'calendar-manager')
      ) {
        autoMode = '';
        modeId = loadModeId();
      }
      lastAutoSwitchedFor = '';
    }
  });
  // Legacy alias for the picker's auto-badge — true when ANY
  // contextual switch is in effect. Cheaper than threading the
  // string through every template branch.
  let autoPMActive = $derived(autoMode !== '');

  // Persist mode change + reset RAG default when user picks a new
  // mode, but DON'T reset on every render (that would clobber the
  // user's explicit override). Only seed when the user actively
  // changes mode.
  function selectMode(id: string) {
    if (id === modeId) return;
    // User is taking control — clear the contextual auto-switch
    // so a future project/goal-page exit doesn't yank them back.
    autoMode = '';
    modeId = id;
    persistModeId(id);
    const m = findMode(id);
    rag = m.ragDefault;
    announce(`Mode: ${m.label}. ${m.tagline}`);
  }
  // Initial seed: read the loaded mode's RAG default. We use the
  // module helper rather than `modeId` (which Svelte's analyzer
  // flags as a non-reactive read) so the warning stays clean. The
  // user's later mode-changes flow through selectMode() above.
  let rag = $state(findMode(loadModeId()).ragDefault);
  let modePickerOpen = $state(false);
  // Last retrieval result for transparency: 'AI saw notes A, B, C'.
  // Cleared on every send so the user sees fresh attribution per
  // turn rather than stale. The retrieval algorithm + the per-tab
  // vault index live in $lib/chat/rag.ts.
  let lastRagHits = $state<RagHit[]>([]);
  // Per-turn map from assistant message index → RAG hits used for that
  // turn. Lets each assistant reply render its own collapsible Sources
  // list inline rather than only the most recent hits at the bottom of
  // the panel. Cleared when the thread is reset; never persisted (the
  // thread storage carries the messages; sources can be re-derived if
  // needed and aren't worth bloating localStorage).
  let perTurnRagHits = $state<Record<number, RagHit[]>>({});
  // Which assistant indices have their Sources expanded. Closed by
  // default — the strip shows count + the user clicks to expand.
  let expandedSources = $state<Record<number, boolean>>({});
  let snapshotLoading = $state(false);
  // Use unknown so we don't lock the consumer into the snapshot
  // shape — the backend evolves it independently.
  let snapshotData = $state<unknown>(null);

  async function loadSnapshot() {
    if (snapshotLoading) return;
    snapshotLoading = true;
    try {
      const r = await api.getAISnapshot();
      snapshotData = r.snapshot ?? null;
    } catch {
      snapshotData = null;
    } finally {
      snapshotLoading = false;
    }
  }

  // Note-aware chat. When the overlay opens on a /notes/<path>
  // page, we offer to attach that note as context to the chat
  // request (chatStream's notePath parameter — server expands it
  // into the system prompt). Default ON when opening on a note
  // page; once opened the user owns the toggle, so manual changes
  // stick. We deliberately don't drive this from a $effect because
  // attachNote being a dependency of its own auto-enable causes
  // the toggle to immediately re-enable when the user un-checks it
  // (the effect re-fires the moment attachNote flips false). The
  // open-transition is the right moment to make the call.
  let attachNote = $state(false);
  // $derived view of the current path so the chip + outgoing
  // chatStream call always reflect the page the user is on, even
  // if they navigate while the overlay is open.
  const currentNotePath = $derived.by(() => {
    const p = $page.url.pathname;
    if (!p.startsWith('/notes/')) return '';
    return decodeURIComponent(p.slice('/notes/'.length));
  });
  // Same shape for the projects route. When set, the first turn of
  // a new thread pulls the project's description + open-task list
  // into a system message so the assistant is grounded without the
  // user re-explaining what project they're talking about.
  const currentProjectName = $derived.by(() => {
    const p = $page.url.pathname;
    // /projects (list view) selects a project via the ?p=<name>
    // query param — that's the canonical "I'm looking at project X"
    // signal because the project detail opens as a drawer, not a
    // separate route. Without this, PM mode never auto-fired on
    // the most common path users actually use.
    if (p === '/projects' || p.startsWith('/projects?')) {
      const q = $page.url.searchParams.get('p');
      return q ? decodeURIComponent(q) : '';
    }
    // Older /projects/<name>/ deep links (if anything still routes
    // there) also count. Strip any trailing path so /projects/X/edit
    // still resolves to X.
    if (p.startsWith('/projects/')) {
      const tail = p.slice('/projects/'.length);
      const name = tail.split('/')[0];
      if (name) return decodeURIComponent(name);
    }
    return '';
  });
  // Goal page selection — /goals?focus=<id>. When set, the
  // sidebar enters Goal Manager mode and injects a per-goal
  // prelude on first turn (mirror of the project flow). The
  // page uses ?focus rather than a path segment so there's no
  // pathname-tail to parse — just check the searchParam.
  const currentGoalId = $derived.by(() => {
    const p = $page.url.pathname;
    if (!p.startsWith('/goals')) return '';
    return $page.url.searchParams.get('focus') ?? '';
  });
  // Calendar page — no specific entity to focus on. Presence
  // alone (path starts with /calendar) is enough to enter the
  // Calendar Manager mode and inject the date-window prelude.
  const onCalendarPage = $derived($page.url.pathname.startsWith('/calendar'));
  // Page-agent launcher — drives the "Run X Agent" sidebar entry
  // point. Each entity page (/tasks /projects /goals /calendar) owns
  // its own embedded agent dialog; the sidebar opens it by navigating
  // there with ?agent=1. Returns null on pages without an agent so
  // the button hides cleanly.
  const pageAgent = $derived.by(() => {
    const p = $page.url.pathname;
    if (p.startsWith('/tasks')) return { path: '/tasks', label: 'Task Agent', glyph: 'TA' };
    if (p === '/projects' || p.startsWith('/projects?') || p.startsWith('/projects/')) {
      return {
        path: '/projects',
        label: currentProjectName ? `Project Agent · ${currentProjectName}` : 'Project Agent',
        glyph: 'PA'
      };
    }
    if (p.startsWith('/goals')) return { path: '/goals', label: 'Goal Agent', glyph: 'GA' };
    if (p.startsWith('/calendar')) return { path: '/calendar', label: 'Calendar Agent', glyph: 'CA' };
    return null;
  });
  function launchPageAgent() {
    if (!pageAgent) return;
    const params = new URLSearchParams($page.url.searchParams);
    params.set('agent', '1');
    const qs = params.toString();
    void goto(`${pageAgent.path}${qs ? '?' + qs : ''}`, { keepFocus: true });
    close();
  }

  function close() {
    abort?.abort();
    if (recording) stopVoice();
    // When pinned, the close button instead unpins (so the user
    // gets an obvious "remove this panel" action). Without this,
    // close-via-X would do nothing visible since `open` stays true
    // while pinned.
    if ($aiOverlayPinned) {
      aiOverlayPinned.set(false);
    }
    aiOverlayOpen.set(false);
  }

  // Direction-aware open/close transition. On desktop the panel
  // slides in from the right edge (it lives anchored md:right-0);
  // on mobile it rises from the bottom (it's a sheet with
  // inset-x-0 bottom-0). We pick the axis from window.innerWidth
  // at transition time rather than at first render so a user
  // resizing their browser between mobile and desktop breakpoints
  // gets the right animation. 768 matches Tailwind's md:.
  function panelTransitionParams() {
    if (typeof window === 'undefined') return { x: 24, y: 0 };
    const isDesktop = window.innerWidth >= 768;
    return isDesktop ? { x: 24, y: 0 } : { x: 0, y: 24 };
  }
  function toggle() {
    aiOverlayOpen.update((v) => !v);
    // The $effect below handles focus + status + note-attach on
    // open-transitions, so no duplication here.
  }
  // Also handle external opens (sidebar button, etc.) — load
  // status + focus input when the store flips us to true. Two
  // tracking rules apply here:
  //
  //   - DON'T read attachNote: doing so would put it in the
  //     effect's deps and the user un-checking it would re-fire
  //     this effect, which would re-enable it (regression of the
  //     earlier flicker bug). Just write unconditionally.
  //
  //   - DON'T track currentNotePath either: navigating while the
  //     overlay is open would re-fire this effect and yank focus
  //     into the chat input, even if the user was typing in the
  //     destination page. untrack reads the path without
  //     subscribing, so the effect only re-fires on open changes.
  $effect(() => {
    if (open) {
      untrack(() => {
        // On note pages, prefer attachNote (the page has a
        // primary doc the AI should anchor to). Elsewhere,
        // pre-fetch the vault snapshot so the chat can route
        // through general "what's going on" context. Both
        // toggles can be flipped by the user once open.
        if (currentNotePath) {
          attachNote = true;
        } else if (attachSnapshot && !snapshotData) {
          void loadSnapshot();
        }
        // Pending seed from a sidebar quick-action chip: switch
        // mode if requested, drop the text into the composer,
        // and (if .send) fire it on the next tick once the input
        // ref + mode have settled. Take-and-clear semantics: the
        // store helper consumes the seed so a later open() with
        // no args won't re-trigger it.
        const seed = takeAIOverlaySeed();
        if (seed) {
          if (seed.modeId) {
            modeId = seed.modeId;
            persistModeId(seed.modeId);
            const m = findMode(seed.modeId);
            rag = m.ragDefault;
          }
          input = seed.text;
          if (seed.send) {
            tick().then(() => { void send(); });
          }
        }
      });
      void loadStatus();
      refreshPinnedIndex();
      tick().then(() => inputEl?.focus());
    }
  });

  // Global Mod+J shortcut + Esc to close. Fires from anywhere
  // including inside text inputs / contentEditable editors —
  // "ask AI about the note I'm currently writing" is the killer
  // use case, so we deliberately steal the keystroke from
  // editors. Mod+J has no strong default in inputs (browsers use
  // it for downloads, which we override the same way Mod+P
  // overrides print).
  function onKey(e: KeyboardEvent) {
    if (open && e.key === 'Escape') {
      e.preventDefault();
      // Layered Esc dismissal: close the most-recently-opened
      // sub-surface first so a single Esc never accidentally throws
      // away the user's whole panel state. Order: pickers (mention,
      // slash, mode) → history slide-over → the overlay itself.
      if (mentionPickerOpen) {
        mentionPickerOpen = false;
      } else if (slashPickerOpen) {
        slashPickerOpen = false;
      } else if (modePickerOpen) {
        modePickerOpen = false;
      } else if (historyOpen) {
        historyOpen = false;
        refocusComposer();
      } else {
        close();
      }
      return;
    }
    if ((e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 'j') {
      e.preventDefault();
      toggle();
    }
  }

  onMount(() => {
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  async function loadStatus() {
    try {
      const s = await api.getAIStatus();
      statusInfo = {
        provider: s.global_provider,
        model: s.global_model,
        sabbath: !!s.sabbath_active
      };
    } catch {
      statusInfo = null;
    }
  }

  // ── Quick actions ──────────────────────────────────────────────
  // Each one: cancel any in-flight call, fire the API, render
  // markdown (briefing / synopsis) or a JSON block of proposals
  // (triage / deadlines). Proposals are NOT applied from here —
  // the dedicated tasks page is the place for that flow because
  // it has the full task context. The overlay just shows the
  // model's suggestions so the user can decide whether to navigate
  // there. Keeps the overlay simple.
  async function runBriefing() {
    await runQuick('Daily briefing', async (s) => {
      const r = await api.aiDailyBriefing(s);
      return r.markdown;
    });
  }
  async function runSynopsis() {
    await runQuick('Weekly synopsis', async (s) => {
      const r = await api.aiWeeklyReview(s);
      return r.markdown;
    });
  }
  async function runTriage() {
    await runQuick('Inbox triage', async (s) => {
      const r = await api.aiInboxTriage(s);
      const props = r.proposals ?? [];
      if (props.length === 0) return '_No untriaged tasks to review._';
      const lines = props.map(
        (p) =>
          `- **${p.priority === 0 ? 'drop' : `P${p.priority}`}** · ${p.schedule} · ${p.rationale} _(${p.id})_`
      );
      return `${lines.length} suggestion${lines.length === 1 ? '' : 's'} — open /tasks → inbox to apply:\n\n${lines.join('\n')}`;
    });
  }
  async function runDeadlines() {
    await runQuick('Detect deadlines', async (s) => {
      const r = await api.aiDeadlineDetect(s);
      const props = r.proposals ?? [];
      if (props.length === 0) return '_No clear deadlines detected._';
      const lines = props.map((p) => `- **${p.due_date}** · ${p.rationale} _(${p.id})_`);
      return `${lines.length} deadline${lines.length === 1 ? '' : 's'} detected — open /tasks → inbox to apply:\n\n${lines.join('\n')}`;
    });
  }

  async function runQuick(title: string, fn: (signal: AbortSignal) => Promise<string>) {
    if (busy) return;
    abort?.abort();
    abort = new AbortController();
    busy = true;
    quickTitle = title;
    quickResult = '_running…_';
    messages = []; // chat clears when a quick action runs
    try {
      quickResult = await fn(abort.signal);
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        quickResult = '_cancelled_';
      } else {
        const msg = errorMessage(err);
        quickResult = /disabled in AI preferences/i.test(msg)
          ? `_${msg}_  \n\n[Open settings →](/settings)`
          : `_failed:_ ${msg}`;
      }
    } finally {
      busy = false;
      abort = null;
    }
  }

  // ── Chat ──────────────────────────────────────────────────────
  // Streaming via /api/v1/chat/stream so the user sees tokens
  // arriving — important on slow local LLMs where a 30s wait
  // with no signal feels broken. Cancel button aborts mid-stream.
  // ── Slash commands ──────────────────────────────────────────────
  // Type-driven shortcuts that bypass the chat round-trip when
  // possible. Power-user surface; the buttons above the chat
  // pane stay for click-first users. Recognised commands:
  //
  //   /help              show this list
  //   /clear             reset the conversation
  //   /briefing          fire the daily briefing (same as button)
  //   /synopsis          fire the weekly synopsis
  //   /triage            run inbox triage
  //   /deadlines         detect deadlines
  //   /detach            drop the snapshot/note attach for next turn
  //
  // A leading slash that doesn't match falls through to normal
  // chat — so a user pasting code with a leading "/" doesn't get
  // accidentally intercepted unless the first word is a real cmd.
  const SLASH_HELP = `**Modes** (top-left in this panel)

  - **General** — balanced help across writing, planning, questions
  - **Research** — grounded answers, named sources, no invention (RAG on)
  - **Writer** — drafting partner, matches your voice
  - **Coach** — Socratic, questions over answers (RAG on)
  - **Analyst** — evidence-first, what would falsify the claim (RAG on)
  - **Architect** — trade-offs + recommendations for system design

**Personas** (sharper voices in the same picker)

  - **Lewis** — C.S. Lewis-style writing critic
  - **Aurelius** — Stoic counsel: brief, stern, kind
  - **Socrates** — questions over answers, sharpens half-formed thoughts
  - **Chrysostom** — scripture commentator in the classical tradition
  - **Founder** — operator coach, ship-this-week energy
  - **Magister** — patient tutor for technical concepts, slow and concrete
  - **Examen** — gentle bedtime companion, soft examen-style questions

  Toggle **RAG** to search the vault for relevant notes per question.

**Shortcuts**

  - <kbd>Mod+J</kbd> — toggle this panel
  - <kbd>Mod+1..9</kbd> — switch agent mode/persona by position
  - **🎤 mic** in the input row — voice dictation (browser STT)
  - **save** in the header — write the thread to \`chat-history/\` as a note

**Slash commands**

  - \`/help\` — show this list
  - \`/clear\` — reset the conversation (saves to history first)
  - \`/new\` — start a fresh thread (current is preserved)
  - \`/save\` — save the current thread under \`chat-history/\`
  - \`/briefing\` — daily briefing (today's events + tasks)
  - \`/synopsis\` — weekly synopsis (Wins / Setbacks / Learned / Next)
  - \`/triage\` — inbox triage proposals
  - \`/deadlines\` — detect deadlines in untimed tasks
  - \`/mode <id>\` — switch agent mode (general/research/writer/coach/analyst/architect)
  - \`/persona <id>\` — switch persona (lewis/aurelius/socrates/...)
  - \`/rag\` — toggle RAG retrieval for the next turn
  - \`/forget\` — drop snapshot/note attachment + queued @-mentions

**Reference vault entities**

  Type \`@\` in the composer to pop a picker for tasks, goals,
  projects, deadlines, events, and notes. Pick → the entity's
  fields (id, title, due date, status…) fold into a strict system
  message so the assistant grounds its reply in real data.

**Thread history**

  Every thread auto-saves to local storage (last 30, oldest drop).
  Click the clock icon in the header to browse + search your saved
  chats and pinned replies. Click ☆ on any assistant message to
  pin it across thread eviction. Click the fork glyph to branch
  the thread from that message into a new conversation.

**Where AI lives in granit**

  - **Note editor** — \`Mod-Shift-A\` ask about selection · \`Mod-Shift-/\` ask about section · \`Mod-Alt-Space\` continue writing · link suggester in the right rail
  - **/morning** — "Suggest from tasks" picks today's #1 focus
  - **/tasks** — "Top 3" focus picker · inbox triage · deadline detect
  - **/calendar** — "Plan my week" agent
  - **/goals** — "Suggest milestones" on goal detail
  - **/projects** — AI summary on project detail
  - **/vision** — "Harden vision" critic
  - **/examen** — gentle reflection prompts per section
  - **/people** — "Suggest 3" reach-outs based on cadence + notes
  - **/habits** — pattern insights from last 30 days

  Press <kbd>Mod+J</kbd> to toggle this panel anywhere in granit.`;

  // Slash-command picker — extracted to $lib/components/SlashCommandPicker.svelte.
  // Owns the dropdown UI, filter logic, and keyboard navigation. The
  // parent keeps the open flag so Esc-from-anywhere can dismiss it
  // (see onKey below) and so onInputKey can chain mention → slash →
  // fall-through-send in the right order.
  let slashPickerOpen = $state(false);
  let slashPickerRef: SlashCommandPicker | undefined = $state();

  // aria-live announcements for screen readers + power-user feedback.
  // Slash commands like /rag and /forget have no visual reply (they
  // just toggle state); without a live-region update a screen reader
  // user has no idea anything happened. Toast already announces for
  // sighted users; this layer mirrors it polite-channel for AT.
  let liveRegion = $state('');
  function announce(msg: string) {
    liveRegion = '';
    // Tick lets the same string re-announce — empty-then-set is the
    // standard hack for back-to-back identical announcements.
    tick().then(() => { liveRegion = msg; });
  }
  // Convenience: focus the composer after a slash-command run, after
  // a thread loads, after the panel opens. Centralises the "where
  // does the user look next" decision so it's consistent.
  function refocusComposer() {
    tick().then(() => inputEl?.focus());
  }

  function handleSlashCommand(raw: string): boolean {
    const trimmed = raw.trim();
    const parts = trimmed.split(/\s+/);
    const cmd = parts[0].toLowerCase();
    const arg = parts.slice(1).join(' ').trim();
    switch (cmd) {
      case '/help':
        // Render help inline as an assistant message — keeps the
        // result in the persisted thread so a follow-up ("ok now
        // briefing") still sees the user's prior context.
        messages = [
          ...messages,
          { role: 'user', content: raw },
          { role: 'assistant', content: SLASH_HELP }
        ];
        input = '';
        return true;
      case '/clear':
        clearChat();
        input = '';
        return true;
      case '/new':
        startNewThread();
        input = '';
        return true;
      case '/save':
        input = '';
        void saveThreadAsNote();
        return true;
      case '/briefing':
        input = '';
        void runBriefing();
        return true;
      case '/synopsis':
        input = '';
        void runSynopsis();
        return true;
      case '/triage':
        input = '';
        void runTriage();
        return true;
      case '/deadlines':
        input = '';
        void runDeadlines();
        return true;
      case '/remember':
        // Persist a fact to long-term memory. Same flow as a
        // remember-this action chip but driven by the user — they
        // can capture something the model said, or a fact they
        // want recorded directly without an AI round-trip.
        input = '';
        if (!arg) {
          toast.info('usage: /remember <fact about yourself>');
          return true;
        }
        (async () => {
          try {
            const f = await api.addAIMemory(arg, []);
            await loadAIMemory();
            toast.success(`Remembered · ${f.content.slice(0, 60)}`);
          } catch (err) {
            toast.error('Memory add failed: ' + errorMessage(err));
          }
        })();
        return true;
      case '/memory':
        // Surface the current memory inline so the user can audit
        // what the model is being told about them, and copy fact
        // IDs for /forget-fact. Renders as an assistant message so
        // it ends up in the persisted thread.
        input = '';
        (async () => {
          await loadAIMemory();
          if (aiMemoryFacts.length === 0) {
            messages = [
              ...messages,
              { role: 'user', content: raw },
              { role: 'assistant', content: '_(no long-term memory recorded yet. Use `/remember <fact>` to add one.)_' }
            ];
            return;
          }
          const lines = aiMemoryFacts
            .map((f, i) => `${i + 1}. ${f.content}${f.tags && f.tags.length > 0 ? ` _(${f.tags.join(', ')})_` : ''} · \`${f.id.slice(0, 6)}\``)
            .join('\n');
          messages = [
            ...messages,
            { role: 'user', content: raw },
            {
              role: 'assistant',
              content: `**Long-term memory (${aiMemoryFacts.length} fact${aiMemoryFacts.length === 1 ? '' : 's'}):**\n\n${lines}\n\n_Use \`/forget-fact <id-prefix>\` to remove one._`
            }
          ];
        })();
        return true;
      case '/forget-fact':
        input = '';
        if (!arg) {
          toast.info('usage: /forget-fact <id-prefix>');
          return true;
        }
        (async () => {
          await loadAIMemory();
          const match = aiMemoryFacts.find((f) => f.id.toLowerCase().startsWith(arg.toLowerCase()));
          if (!match) {
            toast.error(`No fact id starts with "${arg}"`);
            return;
          }
          try {
            await api.deleteAIMemory(match.id);
            await loadAIMemory();
            toast.success(`Forgot · ${match.content.slice(0, 50)}`);
          } catch (err) {
            toast.error('Forget failed: ' + errorMessage(err));
          }
        })();
        return true;
      case '/mode':
      case '/persona': {
        if (!arg) {
          toast.info(`usage: ${cmd} <id>`);
          input = '';
          return true;
        }
        const wanted = arg.toLowerCase();
        const target = AGENT_MODES.find((m) => m.id.toLowerCase() === wanted || m.label.toLowerCase() === wanted);
        if (!target) {
          toast.error(`Unknown ${cmd === '/mode' ? 'mode' : 'persona'}: ${arg}`);
          input = '';
          return true;
        }
        selectMode(target.id);
        toast.success(`${target.glyph} ${target.label} — ${target.tagline}`);
        input = '';
        return true;
      }
      case '/rag':
        rag = !rag;
        toast.success(`RAG ${rag ? 'on' : 'off'} for the next turn.`);
        announce(`RAG ${rag ? 'enabled' : 'disabled'}`);
        input = '';
        refocusComposer();
        return true;
      case '/forget':
      case '/detach':
        attachNote = false;
        attachSnapshot = false;
        mentionedRefs = [];
        input = '';
        toast.success('Context detached for the next message.');
        announce('Context detached for next message');
        refocusComposer();
        return true;
      default:
        return false;
    }
  }

  // ── @-mention picker ───────────────────────────────────────────
  // Owns the dropdown UI, candidate scoring, and entity index loading
  // — extracted to $lib/components/MentionPicker.svelte. The parent
  // holds the open flag (so Esc-from-anywhere can dismiss it) and the
  // queued mentionedRefs (consumed by send() as a strict system
  // message, then cleared so a follow-up doesn't repeat them). Picks
  // come back via onPick.
  let mentionPickerOpen = $state(false);
  let mentionPickerRef: MentionPicker | undefined = $state();
  let mentionedRefs = $state<MentionRef[]>([]);

  function removeMention(idx: number) {
    mentionedRefs = mentionedRefs.filter((_, i) => i !== idx);
  }

  // ── Voice input ────────────────────────────────────────────────
  // Click the mic, the browser's SpeechRecognition fills the input
  // as you speak. Same shared wrapper used by the dashboard's
  // QuickCaptureWidget — graceful fallback when unsupported
  // (Firefox desktop). Auto-restart on Chrome's silence-end so a
  // long thought continues to capture without the user re-clicking.
  let voiceSupported = $derived(isSpeechRecognitionSupported());
  let recording = $state(false);
  let recognition: SpeechRecognitionLike | null = null;
  let voiceBaseline = ''; // input value when recording started — finals append to this

  function startVoice() {
    if (recording) return;
    const r = createSpeechRecognition();
    if (!r) return;
    voiceBaseline = input.endsWith(' ') || input.length === 0 ? input : input + ' ';
    recognition = r;
    r.continuous = true;
    r.interimResults = true;
    r.lang = navigator.language || 'en-US';
    r.onresult = (ev) => {
      let interim = '';
      let final = '';
      for (let i = ev.resultIndex; i < ev.results.length; i++) {
        const res = ev.results[i];
        if (!res || !res[0]) continue;
        const text = res[0].transcript;
        if (res.isFinal) final += text + ' ';
        else interim += text;
      }
      if (final) voiceBaseline += final;
      input = (voiceBaseline + interim).replace(/\s+/g, ' ').trim();
    };
    r.onerror = () => {};
    r.onend = () => {
      // Chrome auto-ends on silence — restart while we're still
      // in recording mode so a long thought continues.
      if (recording && recognition) {
        try { recognition.start(); } catch {}
      }
    };
    try {
      r.start();
      recording = true;
    } catch {}
  }
  function stopVoice() {
    recording = false;
    try { recognition?.stop(); } catch {}
    recognition = null;
  }
  function toggleVoice() {
    if (recording) stopVoice();
    else startVoice();
  }

  // ── Save thread as note ────────────────────────────────────────
  // Persists the current overlay conversation as a markdown note
  // under chat-history/YYYY-MM-DD-HHmm-<slug>.md. Useful when a
  // chat lands on a real insight worth keeping; the dedicated
  // /chat page is for long-running threads, this is the quick
  // 'this was a good answer, save it' move from any page.
  let saving = $state(false);
  // Use the shared slugifier — the inline copy was slightly looser
  // (kept underscores, capped at 60 instead of 80) but the diff is
  // negligible for chat-thread filenames.
  const slugify = slugifyTitle;
  async function saveThreadAsNote() {
    if (saving) return;
    if (messages.length === 0 && !quickResult) {
      toast.info('Nothing to save yet.');
      return;
    }
    saving = true;
    const now = new Date();
    const yyyy = now.getFullYear();
    const mm = String(now.getMonth() + 1).padStart(2, '0');
    const dd = String(now.getDate()).padStart(2, '0');
    const hh = String(now.getHours()).padStart(2, '0');
    const mi = String(now.getMinutes()).padStart(2, '0');
    const firstUser = messages.find((m) => m.role === 'user')?.content ?? quickTitle ?? 'chat';
    const slug = slugify(firstUser) || 'chat';
    const path = `chat-history/${yyyy}-${mm}-${dd}-${hh}${mi}-${slug}.md`;
    // Body: human-readable transcript with mode + RAG metadata.
    const lines: string[] = [
      '# ' + (firstUser.length > 80 ? firstUser.slice(0, 80) + '…' : firstUser),
      '',
      `> mode: **${mode.label}** · ${rag ? 'RAG on' : 'RAG off'} · captured ${now.toLocaleString()}`,
      ''
    ];
    if (quickResult) {
      lines.push('## ' + (quickTitle || 'Quick result'), '', quickResult, '');
    }
    for (const m of messages) {
      lines.push(m.role === 'user' ? '## You' : '## Assistant', '', m.content, '');
    }
    if (lastRagHits.length > 0) {
      lines.push('## Sources retrieved', '');
      for (const h of lastRagHits) lines.push(`- [[${h.path}|${h.title}]]`);
    }
    try {
      await api.createNote({
        path,
        frontmatter: {
          type: 'chat',
          mode: mode.id,
          rag,
          captured_at: now.toISOString(),
          tags: ['chat', mode.id]
        },
        body: lines.join('\n')
      });
      toast.success('Saved · ' + path);
    } catch (e) {
      toast.error('save failed: ' + (errorMessage(e)));
    } finally {
      saving = false;
    }
  }

  // ── Save one assistant reply as a vault note ───────────────────
  // Cuts the "draft a brief → copy-paste into a new note" loop.
  // Common path: PM mode drafts a brief, user accepts, saves under
  // Projects/<name>/<derived-title>.md. When not in a project the
  // file lands under Drafts/. The path is exposed via the toast's
  // "open" action so the user can verify what got written.
  let savingMessageIdx = $state<number | null>(null);

  // copiedMessageIdx — tracks which assistant message just got
  // copied so the button can flash a checkmark for ~1.2s before
  // reverting to the copy icon. Single-slot (only one feedback at
  // a time) — a fresh copy on another row resets the previous.
  let copiedMessageIdx = $state<number | null>(null);
  let copyResetTimer: ReturnType<typeof setTimeout> | null = null;
  async function copyAssistantMessage(content: string, idx: number): Promise<void> {
    // stripStructuredBlocks is the same cleaner saveAssistantAsNote
    // uses — drops action / suggestion blocks so the clipboard
    // contains only the human-readable reply, not the JSON the
    // assistant emitted alongside.
    const cleaned = stripStructuredBlocks(content || '').trim();
    if (!cleaned) {
      toast.info('Nothing to copy.');
      return;
    }
    try {
      if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(cleaned);
      } else {
        // Fallback for non-secure contexts / older browsers: temporary
        // textarea + execCommand. Deprecated but still works on iOS
        // Safari served over plain HTTP (rare for granit but possible
        // on a LAN).
        const ta = document.createElement('textarea');
        ta.value = cleaned;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
      }
      copiedMessageIdx = idx;
      if (copyResetTimer) clearTimeout(copyResetTimer);
      copyResetTimer = setTimeout(() => {
        copiedMessageIdx = null;
      }, 1200);
    } catch {
      toast.error('Copy failed — your browser blocked clipboard access.');
    }
  }

  async function saveAssistantAsNote(idx: number) {
    const m = messages[idx];
    if (!m || m.role !== 'assistant') return;
    if (savingMessageIdx !== null) return;
    const cleaned = stripStructuredBlocks(m.content || '').trim();
    if (!cleaned) {
      toast.info('Nothing to save in this reply.');
      return;
    }
    savingMessageIdx = idx;
    const title = deriveDraftTitle(cleaned, todayISO());
    // Folder picked by context. Project takes precedence (the
    // Projects/<name> folder is where its notes tab looks);
    // goal-mode drafts land in a goal-scoped subfolder so they're
    // discoverable per goal; calendar drafts go to Calendar/.
    // Everything else lands under Drafts/.
    const folder = currentProjectName
      ? `Projects/${slugify(currentProjectName) || currentProjectName}`
      : currentGoalId
      ? `Goals/${slugify(currentGoalId) || currentGoalId}`
      : onCalendarPage
      ? 'Calendar/Drafts'
      : 'Drafts';
    const baseSlug = slugify(title) || 'draft';
    const basePath = `${folder}/${baseSlug}.md`;
    // Auto-suffix retry on path collision so saving the same
    // draft twice doesn't surface a scary "save failed" toast
    // and doesn't silently overwrite the first note. The suffix
    // is "HHmm" from the current time — short, sortable, and
    // unique enough for a single user.
    // Cross-link frontmatter — carries the source context so the
    // saved note can render a "from chat about X" badge AND so
    // the project's notes tab (or future goal/calendar notes
    // surfaces) can list AI drafts even when they live outside
    // the entity's natural folder. project / goal / calendar are
    // mutually exclusive in autoMode but the user might also be
    // in a contextual mode manually — capture what's actually
    // in scope, not what autoMode says.
    const frontmatter: Record<string, unknown> = {
      type: 'ai-draft',
      mode: mode.id,
      captured_at: new Date().toISOString(),
      tags: ['ai-draft', mode.id]
    };
    if (currentProjectName) frontmatter.project = currentProjectName;
    if (currentGoalId) frontmatter.goal = currentGoalId;
    if (onCalendarPage) frontmatter.calendar_window = true;
    try {
      let finalPath = basePath;
      try {
        await api.createNote({ path: basePath, frontmatter, body: cleaned });
      } catch (err) {
        // 409 Conflict — file exists. Retry with a time suffix.
        // Any other error rethrows to the outer toast handler.
        const msg = errorMessage(err);
        if (!/already exists|409/i.test(msg)) throw err;
        const now = new Date();
        const suffix = `${String(now.getHours()).padStart(2, '0')}${String(now.getMinutes()).padStart(2, '0')}`;
        finalPath = `${folder}/${baseSlug}-${suffix}.md`;
        await api.createNote({ path: finalPath, frontmatter, body: cleaned });
      }
      toast.success(`Saved · ${finalPath}`, {
        action: { label: 'Open', href: `/notes/${encodeURIComponent(finalPath)}` }
      });
    } catch (e) {
      toast.error('Save failed: ' + errorMessage(e));
    } finally {
      savingMessageIdx = null;
    }
  }
  // deriveDraftTitle lives in $lib/ai/draftTitle (10 tests pin
  // the precedence rules) — imported above.

  // ── Long-term AI memory ─────────────────────────────────────────
  // The user's persistent facts ("wife is Anna", "vegetarian", etc.)
  // get folded into every thread's first-turn prelude so the model
  // doesn't need them re-stated. Loaded lazily on overlay open +
  // refreshed when the server broadcasts ai-memory.json changes.
  let aiMemoryFacts = $state<AIMemoryFact[]>([]);
  let aiMemoryLoaded = $state(false);

  async function loadAIMemory() {
    try {
      const r = await api.listAIMemory();
      aiMemoryFacts = r.facts;
      aiMemoryLoaded = true;
    } catch {
      // Silent: an empty/failed memory load shouldn't block the
      // chat. The user simply doesn't get memory-augmented replies
      // until the next successful fetch.
    }
  }
  // Refresh on WS broadcasts so a successful action-chip click on
  // a "remember-this" proposal in one tab updates other tabs.
  onMount(() => {
    void loadAIMemory();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/ai-memory.json') {
        void loadAIMemory();
      }
    });
  });

  // ── Agent-capabilities system instruction ──────────────────────
  // One system message describing the structured-output channels
  // (follow-ups + action chips). Injected on the FIRST turn of every
  // new thread, like the vault snapshot — re-injecting on every turn
  // burns tokens for instructions the model has already internalised.
  //
  // Kept terse: each shape gets a one-line example, no prose
  // elaboration. The shorter this block, the lower the token cost
  // per new thread.
  const AGENT_CAPABILITIES_SYSTEM = `Granit gives you two structured-output channels that turn parts of your reply into one-tap UI in the user's chat overlay. Use them when they would help the user act on your answer — not speculatively.

FOLLOW-UPS — at the very end of your reply (no other text after), append a single <followups>...</followups> block with up to 3 short prompts the user might naturally want next, one per line. Skip entirely when nothing useful comes to mind.

<followups>
Want me to break this into subtasks?
Should I draft the email reply?
</followups>

VAULT ACTIONS — when you propose creating something in the user's vault, emit a fenced "granit-action" JSON block:

\`\`\`granit-action
{"type":"task","text":"Call Anna about the contract","dueDate":"2026-05-12","priority":2}
\`\`\`

\`\`\`granit-action
{"type":"event","title":"Lunch with Sarah","start":"2026-05-12T13:00:00","end":"2026-05-12T14:00:00","location":"Centro"}
\`\`\`

\`\`\`granit-action
{"type":"note","title":"Reading list","body":"- Meditations\\n- The Brothers Karamazov\\n","folder":"Lists"}
\`\`\`

\`\`\`granit-action
{"type":"remember","content":"User's wife is named Anna","tags":["family"]}
\`\`\`

Fields: task.text required; dueDate/priority/notePath optional. event.title+start required; end/location optional. note.title+body required; folder optional. remember.content required; tags optional. priority is 1 (low) to 3 (high). Dates use YYYY-MM-DD; datetimes use floating ISO (no Z). Emit zero or many; the user picks which to commit.`;

  // Already-committed action chips per message id — keyed by the
  // action's stable signature (see $lib/chat/actionParser.actionKey)
  // so a click doesn't double-commit and a regen with the same
  // proposal stays "fresh" until clicked. Parsing rules live in the
  // dedicated module + are pinned by actionParser.test.ts.
  let committedActions = $state<Record<string, boolean>>({});

  async function commitAction(msgIdx: number, a: ParsedAction) {
    const key = actionKey(msgIdx, a);
    if (committedActions[key]) return;
    try {
      if (a.type === 'task') {
        // Default notePath: the user's current page when it's a
        // note (so the task lives where they're working), else
        // today's daily — same pattern QuickCaptureFab uses. The
        // create-task endpoint auto-materialises Daily/<date>.md
        // when it doesn't exist yet.
        const np = a.notePath || (currentNotePath || `Daily/${todayISO()}.md`);
        await api.createTask({
          notePath: np,
          text: a.text,
          dueDate: a.dueDate || undefined,
          priority: a.priority,
          section: '## Tasks'
        });
        toast.success(`Task added · ${a.text.slice(0, 50)}`);
      } else if (a.type === 'event') {
        // Parse the start to derive the date + HH:MM split events.json
        // wants. Floating ISO ("2026-05-12T13:00:00") parses cleanly
        // as local; if the model emitted Z we strip the offset so we
        // store the wall-clock the user typed, matching how native
        // events round-trip.
        const startStr = a.start.replace(/Z$/, '').replace(/[+-]\d{2}:\d{2}$/, '');
        const startDate = startStr.slice(0, 10);
        const startTime = startStr.slice(11, 16);
        const endStr = (a.end ?? '').replace(/Z$/, '').replace(/[+-]\d{2}:\d{2}$/, '');
        const endTime = endStr.slice(11, 16);
        await api.createEvent({
          title: a.title,
          date: startDate,
          start_time: startTime,
          end_time: endTime || undefined,
          location: a.location || undefined
        });
        toast.success(`Event added · ${a.title}`);
      } else if (a.type === 'note') {
        const folder = (a.folder ?? '').trim();
        const slug = a.title.replace(/[^\w\s-]/g, '').replace(/\s+/g, '-').toLowerCase() || 'note';
        const path = folder ? `${folder.replace(/\/+$/, '')}/${slug}.md` : `${slug}.md`;
        await api.createNote({
          path,
          frontmatter: { title: a.title },
          body: a.body
        });
        toast.success(`Note created · ${path}`, {
          action: { label: 'Open', href: `/notes/${encodeURIComponent(path)}` }
        });
      } else if (a.type === 'remember') {
        await api.addAIMemory(a.content, a.tags);
        toast.success('Saved to memory');
        await loadAIMemory();
      }
      committedActions = { ...committedActions, [key]: true };
    } catch (err) {
      toast.error('Action failed: ' + errorMessage(err));
    }
  }

  function sendFollowup(prompt: string) {
    input = prompt;
    void send();
  }

  // ── Context-aware defaults ──────────────────────────────────────
  // When the overlay opens for the first time on a route, pick a
  // mode that fits the page so the user doesn't have to flip the
  // mode picker themselves. Skipped if the user has explicitly
  // chosen a non-'general' mode this session (don't undo intent).
  let contextDefaultsApplied = $state(false);
  function applyContextDefaults() {
    if (contextDefaultsApplied) return;
    if (typeof window === 'undefined') return;
    const path = $page.url.pathname;
    // Map routes → mode. Conservative: only flip on routes where the
    // mode shift is obviously useful. /notes/X already has its own
    // attach-current-note logic; the rest live here.
    let suggested: string | null = null;
    if (path.startsWith('/tasks')) suggested = 'analyst';
    else if (path.startsWith('/calendar')) suggested = 'analyst';
    else if (path.startsWith('/goals') || path.startsWith('/ventures')) suggested = 'coach';
    else if (path.startsWith('/projects')) suggested = 'architect';
    else if (path.startsWith('/examen')) suggested = 'examen';
    else if (path.startsWith('/prayer') || path.startsWith('/bible') || path.startsWith('/scripture')) suggested = 'aurelius';
    if (suggested && mode.id === 'general') {
      const target = AGENT_MODES.find((m) => m.id === suggested);
      if (target) {
        selectMode(target.id);
      }
    }
    contextDefaultsApplied = true;
  }
  // Apply on every fresh open so the user navigating /tasks → open
  // chat → /goals → open chat gets the right mode each time. The
  // flag resets via the open-effect chain.
  $effect(() => {
    if ($aiOverlayOpen) {
      // tick() lets the open transition settle before we possibly
      // flip the mode (avoids a single-frame flash of the wrong
      // header label).
      tick().then(() => applyContextDefaults());
    } else {
      contextDefaultsApplied = false;
    }
  });

  async function send(e?: Event) {
    e?.preventDefault();
    const text = input.trim();
    if (!text || busy) return;
    if (text.startsWith('/') && handleSlashCommand(text)) return;
    quickTitle = '';
    quickResult = '';
    busy = true;
    abort?.abort();
    abort = new AbortController();
    const userMsg: ChatMessage = { role: 'user', content: text };
    // Record to the shared cross-source recents log so the inline
    // AI menu can offer this prompt as a chat-source recent next
    // time the user opens it on a note. Non-fatal if storage fails.
    recordSharedPrompt({ prompt: text, source: 'chat' });
    // Build the prelude — a system message containing the
    // active agent mode's posture, optionally the vault snapshot
    // (on non-note routes when attached), optionally retrieved
    // RAG hits (when rag=true). Posture stays for the whole
    // thread; snapshot/RAG inject on the first turn only since
    // re-injecting on every turn burns tokens for facts the
    // assistant has already seen.
    const prelude: ChatMessage[] = [];
    const isFirstTurn = messages.length === 0;
    // Mode posture — every turn (cheap; one paragraph). Keeps the
    // mode active even after history is long.
    prelude.push({ role: 'system', content: mode.system });
    // Agent capabilities — describes the structured-output channels
    // (follow-ups + granit-action chips). First turn only since the
    // model internalises it after one example; re-injecting on every
    // turn would burn ~400 tokens per turn for no marginal lift.
    if (isFirstTurn) {
      prelude.push({ role: 'system', content: AGENT_CAPABILITIES_SYSTEM });
    }
    // Long-term memory — the user's persistent facts. First turn
    // only for the same token-cost reason; the model carries them
    // forward in its context after that. Skipped when the store is
    // empty so a fresh vault doesn't pay for an empty prelude.
    if (isFirstTurn && aiMemoryFacts.length > 0) {
      const lines = aiMemoryFacts.map((f) => `- ${f.content}`);
      prelude.push({
        role: 'system',
        content:
          "These are persistent facts the user has told Granit to remember about themselves. Use them when relevant — don't re-ask for context they've already given.\n\n" +
          lines.join('\n')
      });
    }
    // @-mentioned entity context. Strict, structured system message
    // — gives the model real fields (id, title, due date, status…)
    // rather than relying on the user's prose to convey them.
    // Injected only on the turn the mentions are attached, then
    // cleared so a follow-up doesn't spam the same context.
    if (mentionedRefs.length > 0) {
      const lines = mentionedRefs.map((r) => `- ${r.contextLine}`);
      prelude.push({
        role: 'system',
        content:
          'The user has explicitly referenced these vault entities in their message. Use these fields when answering — do not invent ids or dates.\n\n' +
          lines.join('\n')
      });
    }
    // Project-scoped context — when the user opens chat from
    // /projects/<name>, grab the project's description + open-task
    // list and inject as a system message on the first turn. Saves
    // the user from re-explaining "I'm asking about the Granite
    // project" on every fresh thread. Skipped after first turn so
    // a long thread doesn't burn tokens re-asserting the context.
    if (isFirstTurn && currentProjectName) {
      try {
        // Use the shared Project-Manager context loader so the
        // chat surface and the Project Manager mode share one
        // ground-truth bundle (linked goals, open + recently-done
        // tasks, notes under the project folder). Tested in
        // projectManagerContext.test.ts so the prelude shape
        // can't silently drift.
        const bundle = await loadProjectContext(currentProjectName, {
          getProject: (n) => api.getProject(n),
          listTasksForProject: async (n, s) => {
            const r = await api.listTasks({ project: n, status: s });
            return r.tasks;
          },
          listGoalsForProject: async (n) => {
            const r = await api.listGoals();
            return r.goals.filter((g) => g.project === n);
          },
          listNotesInFolder: async (folder) => {
            const r = await api.listNotes({ folder, limit: 50 });
            return r.notes;
          }
        });
        prelude.push({
          role: 'system',
          content:
            "The user is currently looking at this project in Granit. Use it as the default subject of their messages — they don't need to re-state which project they mean.\n\n" +
            renderProjectContext(bundle)
        });
      } catch {
        // Project fetch failure — skip the injection silently and
        // let the chat run as a generic thread.
      }
    }
    // Calendar context — date-window flavour rather than per-entity.
    // Fires when the user opens the chat from /calendar and we're
    // not already loading project/goal scope. Window defaults to
    // 14 days ahead; the formatter surfaces the range so the AI
    // refuses questions about events outside it.
    if (isFirstTurn && onCalendarPage && !currentProjectName && !currentGoalId) {
      try {
        const bundle = await loadCalendarContext(
          {
            listEvents: async () => {
              const r = await api.listEvents();
              return r.events;
            },
            listTasks: async () => {
              const r = await api.listTasks({});
              return r.tasks;
            }
          },
          { todayISO: todayISO() }
        );
        prelude.push({
          role: 'system',
          content:
            "The user is currently looking at their calendar in Granit. Use this date-window context as the default subject of their messages.\n\n" +
            renderCalendarContext(bundle)
        });
      } catch {
        // Listing failure — skip silently.
      }
    }
    // Goal-scoped context — mirror of the project flow above.
    // Fires only when a goal is focused (URL `?focus=<id>`) and
    // we're not already loading project context (project wins
    // for the rare case of an overlapping URL). Uses the shared
    // loadGoalContext so both Goal Manager mode and the prelude
    // see the same goal bundle.
    if (isFirstTurn && currentGoalId && !currentProjectName) {
      try {
        const bundle = await loadGoalContext(currentGoalId, {
          getGoal: async (id) => {
            const r = await api.listGoals();
            const g = r.goals.find((x) => x.id === id);
            if (!g) throw new Error('goal not found');
            return g;
          },
          listTasksForGoal: async (id, s) => {
            const r = await api.listTasks({ goal: id, status: s });
            return r.tasks;
          }
        });
        prelude.push({
          role: 'system',
          content:
            "The user is currently focused on this goal in Granit. Use it as the default subject of their messages — they don't need to re-state which goal they mean.\n\n" +
            renderGoalContext(bundle)
        });
      } catch {
        // Goal not found / fetch failure — skip silently.
      }
    }
    if (isFirstTurn && attachSnapshot && snapshotData && !currentNotePath) {
      prelude.push({
        role: 'system',
        content:
          "Here's a snapshot of the user's vault — today's events, " +
          'open tasks, recent notes, active goals, and deadlines. ' +
          'Refer to it when relevant; do not invent content beyond it.\n\n' +
          '```json\n' + JSON.stringify(snapshotData, null, 2) + '\n```'
      });
    }
    // RAG — runs on every turn the toggle is on, so a follow-up
    // question about a different topic retrieves different notes.
    // We pass currentNotePath so retrieveForRag skips it (no point
    // re-injecting the note already on the prompt via notePath).
    // Composing both: attachNote=true (current note in system) +
    // rag=true (related notes in system) is supported and useful
    // for 'explain this concept using my other notes too'.
    lastRagHits = [];
    if (rag) {
      try {
        const hits = await retrieveForRag(text, currentNotePath);
        if (hits.length > 0) {
          lastRagHits = hits;
          const formatted = hits
            .map((h, i) => `### Note ${i + 1}: ${h.title}\nPath: \`${h.path}\`\n\n${h.excerpt}`)
            .join('\n\n---\n\n');
          prelude.push({
            role: 'system',
            content:
              `RAG retrieved ${hits.length} note(s) from the user's vault that match this query. Quote from these when relevant; cite the note title in your reply. Do NOT invent content beyond what's here. If they don't actually answer the question, say so plainly.\n\n${formatted}`
          });
        }
      } catch {
        // Retrieval failure shouldn't block the chat — fall through
        // and let the model answer without RAG context.
      }
    }
    const history = [...prelude, ...messages, userMsg];
    messages = [...messages, userMsg, { role: 'assistant', content: '' }];
    input = '';
    // Refs were attached in the prelude; drop them so a follow-up
    // doesn't repeat the same context block.
    mentionedRefs = [];
    let acc = '';
    const idx = messages.length - 1;
    // Record this turn's RAG hits against the assistant message index so
    // the inline Sources block renders next to the right reply, not the
    // most-recent one. Set even when empty — the renderer keys on
    // presence, not truthiness.
    if (lastRagHits.length > 0) {
      perTurnRagHits = { ...perTurnRagHits, [idx]: lastRagHits.slice() };
    }
    try {
      // rAF throttle — the assistant message is rendered live as a
      // MarkdownRenderer block (one per message in the thread).
      // Pre-fix the messages array was rebuilt + re-rendered + the
      // assistant message's markdown re-parsed PER token, freezing
      // long replies. The throttle commits the latest buffer once
      // per animation frame; flush() runs on stream completion so
      // the final state lands before auto-save fires.
      //
      // Smart auto-scroll: if the user is within 80px of the
      // bottom we keep them pinned to the streaming edge so a long
      // reply doesn't run off-screen. If they scrolled up to re-
      // read older content, we respect that — no yank-back when a
      // chunk lands. 80px is generous enough that the user clearly
      // intended to stay near the bottom; smaller windows would
      // disengage from the stream on a single trackpad nudge.
      const t = rafThrottle((full) => {
        const stickBottom = !!scrollEl &&
          (scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight) < 80;
        acc = full;
        messages = messages.map((m, i) => (i === idx ? { ...m, content: full } : m));
        if (stickBottom) {
          tick().then(() => {
            if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
          });
        }
      });
      await api.chatStream(
        history,
        attachNote && currentNotePath ? currentNotePath : undefined,
        {
          onChunk: t.onChunk,
          onDone: () => { t.flush(); },
          onError: (err) => {
            t.flush();
            messages = messages.map((m, i) =>
              i === idx ? { ...m, content: `_error:_ ${err.message}` } : m
            );
          }
        },
        abort.signal
      );
    } finally {
      busy = false;
      abort = null;
      // Auto-save the thread once the assistant reply lands — gives
      // the history picker a row even if the user closes the overlay
      // before a follow-up. Skip if the reply was an error stub.
      if (acc.trim().length > 0) autoSaveThread();
      tick().then(() => {
        if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
      });
    }
  }

  function cancelInflight() {
    abort?.abort();
  }

  function clearChat() {
    if (messages.length === 0) return;
    // Snapshot to history before nuking — "clear" should not destroy
    // a useful conversation. The user can still hard-delete from the
    // history picker if they really want it gone.
    autoSaveThread();
    messages = [];
    quickTitle = '';
    quickResult = '';
    perTurnRagHits = {};
    expandedSources = {};
    pinnedIndex = {};
    activeThreadId = '';
    persistActiveThreadId('');
  }

  $effect(() => {
    void messages.length;
    void quickResult;
    tick().then(() => {
      if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
    });
  });

  function onInputKey(e: KeyboardEvent) {
    // Mention + slash pickers swallow arrow/enter/tab while open so
    // the user navigates the popup before falling through to
    // send-on-enter. handleKey returns true when the picker swallows
    // the event; the slash picker also returns false on the
    // exact-match-Enter case so this fall-through still calls send().
    if (mentionPickerRef?.handleKey(e)) return;
    if (slashPickerRef?.handleKey(e)) return;
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      void send();
    }
  }
  function onInputChange() {
    slashPickerRef?.detectTrigger();
    mentionPickerRef?.detectTrigger();
    autosizeInput();
  }
  // Composer auto-grow. textarea[rows=2] is the resting height; as the
  // user types newlines (or pastes a multi-line prompt) we expand the
  // element up to ~50% of the panel's height before falling back to
  // internal scrolling. Without this the textarea stays fixed at 2
  // rows and longer prompts hide their own bottom — frustrating on
  // both mobile and desktop. Implementation: reset to height:auto so
  // scrollHeight reads the natural content height, then clamp + write
  // back. Cheap (one layout read per keystroke).
  function autosizeInput() {
    if (!inputEl) return;
    const ta = inputEl;
    const panel = panelEl?.getBoundingClientRect().height ?? window.innerHeight;
    const cap = Math.max(120, Math.floor(panel * 0.5));
    ta.style.height = 'auto';
    const next = Math.min(cap, ta.scrollHeight);
    ta.style.height = next + 'px';
    ta.style.overflowY = ta.scrollHeight > cap ? 'auto' : 'hidden';
  }
  // Re-run autosize on every input mutation (typing, voice, slash
  // pick, mention pick). $effect tracks `input` as a dep so any
  // programmatic write — voice transcript, mention insert, /help —
  // also triggers a resize without relying on individual call sites.
  $effect(() => {
    void input;
    tick().then(() => autosizeInput());
  });
  function onInputClick() {
    // Caret moved without typing — re-evaluate mention/slash context.
    slashPickerRef?.detectTrigger();
    mentionPickerRef?.detectTrigger();
  }

  // Mode quick-switch: Mod+1..9 picks the matching entry by
  // position in AGENT_MODES (generic modes first, then personas).
  // Power-user shortcut; only fires while the overlay is open +
  // the user isn't typing into the chat input (numbers there
  // should land as numbers, not mode jumps). Entries beyond
  // position 9 are reachable via the picker only — keypad-style
  // 1..9 is the practical ceiling for a single-key shortcut.
  $effect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      const mod = e.metaKey || e.ctrlKey;
      if (!mod || e.shiftKey || e.altKey) return;
      const target = e.target as HTMLElement | null;
      if (target instanceof HTMLTextAreaElement || target instanceof HTMLInputElement) return;
      const idx = parseInt(e.key, 10);
      if (Number.isNaN(idx) || idx < 1 || idx > Math.min(9, AGENT_MODES.length)) return;
      e.preventDefault();
      selectMode(AGENT_MODES[idx - 1].id);
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });
</script>

{#if open}
  <!-- Backdrop. Click-to-close on mobile; on desktop the panel
       sits next to content rather than over it, so the backdrop
       is hidden by md:hidden — desktop users dismiss with Esc or
       the close button. -->
  {#if !$aiOverlayPinned}
    <!-- Backdrop only when floating; pinned mode reserves layout
         space instead so there's no overlap to dim. -->
    <button
      type="button"
      aria-label="close AI overlay"
      onclick={close}
      transition:fade={{ duration: 150 }}
      class="md:hidden fixed inset-0 z-40 bg-black/60"
    ></button>
  {/if}

  <div
    bind:this={panelEl}
    data-ai-overlay
    role="dialog"
    aria-label="AI assistant"
    style:--ai-panel-w="{panelWidth}px"
    style:--ai-sheet-h={mobileSheetHeight}
    style:--ai-sheet-lift="{keyboardOffset}px"
    in:fly={{ duration: $aiOverlayPinned ? 0 : 200, easing: cubicOut, ...panelTransitionParams() }}
    out:fly={{ duration: $aiOverlayPinned ? 0 : 150, easing: cubicOut, ...panelTransitionParams() }}
    class="ai-overlay-panel fixed z-50 flex flex-col bg-base border-surface1
           inset-x-0 rounded-t-xl border-t {keyboardOpen ? '' : 'pb-safe'} {keyboardOpen ? 'ai-overlay-kb-open' : ''}
           md:inset-y-0 md:right-0 md:left-auto md:bottom-auto md:top-0 md:h-full md:max-h-none md:rounded-none md:border-l md:border-t-0 md:pb-0 {$aiOverlayPinned ? 'md:shadow-none' : 'shadow-2xl'} {resizing ? 'ai-overlay-resizing' : ''} {sheetDragging ? 'ai-overlay-snapping' : ''}"
  >
    <!-- Desktop-only drag handle on the LEFT edge of the panel. The
         panel is right-anchored; widening means pulling left so the
         handle sits exactly where the user expects (between content
         + panel). 6px wide hit area, transparent until hover so it
         doesn't add visual noise. PointerEvents capture so the drag
         survives the cursor straying off the strip. -->
    <button
      type="button"
      aria-label="Resize AI panel"
      aria-valuenow={panelWidth}
      aria-valuemin={PANEL_WIDTH_MIN}
      aria-valuemax={PANEL_WIDTH_MAX}
      role="slider"
      onpointerdown={onResizeStart}
      onkeydown={onResizeKey}
      class="hidden md:block absolute left-0 top-0 bottom-0 w-1.5 -ml-0.5 z-50 cursor-col-resize group {resizing ? 'bg-primary/40' : 'hover:bg-primary focus-visible:bg-primary/40'} transition-colors"
    >
      <span class="sr-only">Drag to resize panel</span>
    </button>
    <!-- Polite aria-live region for power-user feedback (slash
         commands, mode switches). Sighted users get a toast; AT
         users get this same message read aloud without yanking
         focus. Empty when idle so the SR doesn't spuriously
         announce on every render. -->
    <div role="status" aria-live="polite" aria-atomic="true" class="sr-only">{liveRegion}</div>

    <!-- Header. Mobile gets a drag-handle visual hint at the very
         top; both layouts get title + status pill + close. The
         handle is now a real drag affordance: pull up to grow the
         sheet to mid/full, pull down to shrink back to peek. Tap
         (no drag) cycles peek→mid→full→peek so users without a
         drag-precise hand still get all three positions. -->
    <button
      type="button"
      class="md:hidden flex justify-center pt-2 pb-2 w-full touch-none"
      onpointerdown={onSheetHandleDown}
      onclick={() => {
        const order: SheetSnap[] = ['peek', 'mid', 'full'];
        const idx = order.indexOf(sheetSnap);
        sheetSnap = order[(idx + 1) % order.length];
      }}
      aria-label="Resize chat sheet — drag or tap to cycle"
    >
      <span class="block w-10 h-1.5 rounded-full bg-surface2 transition-colors {sheetDragging ? 'bg-primary' : ''}"></span>
    </button>
    <header class="px-3 py-2 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
      <!-- Mode picker — replaces the static '✨ AI assistant'
           heading. Click to open a popover of agent modes, each
           with a one-line tagline. Mode is the headline UX choice
           in the overlay; status pill + cancel + close pack to
           the right. -->
      <div class="relative flex-shrink-0">
        <button
          type="button"
          onclick={() => (modePickerOpen = !modePickerOpen)}
          aria-haspopup="listbox"
          aria-expanded={modePickerOpen}
          class="tap-target inline-flex items-center gap-1.5 px-2 py-1 rounded hover:bg-surface0 active:bg-surface1 text-text transition-colors"
          title={autoPMActive
            ? `Mode: ${mode.label} (auto — you're on a project page). Click to override.`
            : `Mode: ${mode.label} — ${mode.tagline}`}
        >
          <span class="text-[10px] font-semibold tracking-tight leading-none inline-flex items-center justify-center w-6 h-6 rounded-md {mode.kind === 'persona' ? 'bg-secondary text-on-primary' : mode.kind === 'contextual' ? 'bg-primary text-on-primary' : 'bg-surface1 text-subtext'}">{mode.glyph}</span>
          <span class="text-sm font-semibold truncate max-w-[8rem] sm:max-w-none">{mode.label}</span>
          {#if autoPMActive}
            <!-- Tiny "auto · <source>" badge. The source word makes
                 the contextual switch self-explanatory — the user
                 reads "auto · project" and knows where the mode
                 came from (and that picking anything else takes
                 control back). Clears the moment they pick. -->
            <span class="text-[9px] uppercase tracking-wider px-1 rounded bg-primary text-on-primary leading-tight whitespace-nowrap">auto · {autoMode}</span>
          {/if}
          <svg viewBox="0 0 24 24" class="w-3 h-3 text-dim flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="6 9 12 15 18 9" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </button>
        {#if modePickerOpen}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <div
            role="presentation"
            class="fixed inset-0 z-40"
            onclick={() => (modePickerOpen = false)}
          ></div>
          <div
            role="listbox"
            class="absolute left-0 top-full mt-1 w-[min(18rem,calc(100vw-1rem))] bg-mantle border border-surface1 rounded-lg shadow-xl z-50 py-1 max-h-[70dvh] overflow-y-auto"
          >
            <!-- Generic modes group. The "modes" header is implicit
                 (the picker opens with them; no need to label what
                 the user is already looking at). The "personas"
                 header below makes the second group obvious. -->
            <div class="px-3 pt-2 pb-1 text-[10px] font-semibold uppercase tracking-[0.14em] text-dim">Modes</div>
            {#each GENERIC_MODES as m (m.id)}
              <button
                type="button"
                role="option"
                aria-selected={m.id === modeId}
                onclick={() => { selectMode(m.id); modePickerOpen = false; }}
                class="w-full flex items-center gap-2.5 px-3 py-2 hover:bg-surface0 text-left transition-colors {m.id === modeId ? 'bg-surface1' : ''}"
              >
                <span class="text-[11px] font-semibold tracking-tight leading-none flex-shrink-0 inline-flex items-center justify-center w-7 h-7 rounded-md bg-surface1 text-subtext">{m.glyph}</span>
                <div class="flex-1 min-w-0">
                  <div class="text-[13px] font-medium text-text leading-tight">{m.label}</div>
                  <div class="text-[11px] text-dim leading-snug mt-0.5">{m.tagline}</div>
                </div>
                {#if m.id === modeId}
                  <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 text-primary flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="20 6 9 17 4 12"/>
                  </svg>
                {/if}
              </button>
            {/each}
            {#if CONTEXTUAL_MODES.length > 0}
              <!-- Contextual modes — page-aware. They auto-switch
                   when the user is on a matching URL (project /
                   goal / calendar) and revert when the user leaves.
                   Visually distinguished with a primary-tinted
                   glyph background so the user reads them as
                   "tied to a page", not generic postures.
                   Out-of-scope modes are dimmed + carry a "needs
                   <X>" hint so the user knows the prelude won't
                   carry context — they can still pick (sometimes
                   useful for the system-prompt posture alone),
                   it just won't be the full PM/Goal/Calendar
                   manager experience. -->
              <div class="border-t border-surface1 mt-1"></div>
              <div class="px-3 pt-2 pb-1 text-[10px] font-semibold uppercase tracking-[0.14em] text-primary">Contextual</div>
              {#each CONTEXTUAL_MODES as m (m.id)}
                {@const inScope =
                  (m.id === 'project-manager' && !!currentProjectName) ||
                  (m.id === 'goal-manager' && !!currentGoalId) ||
                  (m.id === 'calendar-manager' && onCalendarPage)}
                {@const scopeHint =
                  m.id === 'project-manager'
                    ? 'open a project'
                    : m.id === 'goal-manager'
                    ? 'focus a goal'
                    : 'open the calendar'}
                <button
                  type="button"
                  role="option"
                  aria-selected={m.id === modeId}
                  onclick={() => { selectMode(m.id); modePickerOpen = false; }}
                  class="w-full flex items-center gap-2.5 px-3 py-2 hover:bg-surface0 text-left transition-colors {m.id === modeId ? 'bg-surface1' : ''} {inScope ? '' : 'text-dim'}"
                  title={inScope
                    ? m.tagline
                    : `Pick-able from any page, but the prelude won't carry context until you ${scopeHint}.`}
                >
                  <span class="text-[11px] font-semibold tracking-tight leading-none flex-shrink-0 inline-flex items-center justify-center w-7 h-7 rounded-md bg-primary text-on-primary">{m.glyph}</span>
                  <div class="flex-1 min-w-0">
                    <div class="text-[13px] font-medium text-text leading-tight inline-flex items-center gap-1.5">
                      {m.label}
                      {#if !inScope}
                        <span class="text-[9px] uppercase tracking-wider text-dim font-normal bg-surface1 px-1 py-0.5 rounded">needs {scopeHint}</span>
                      {/if}
                    </div>
                    <div class="text-[11px] text-dim leading-snug mt-0.5">{m.tagline}</div>
                  </div>
                  {#if m.id === modeId}
                    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 text-primary flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                      <polyline points="20 6 9 17 4 12"/>
                    </svg>
                  {/if}
                </button>
              {/each}
            {/if}
            {#if PERSONAS.length > 0}
              <!-- Personas group — sharper, named voices. Visually
                   distinguished by a divider, a section header, an
                   accent-coloured glyph background, and an italic
                   tagline so the user reads "this is a character,
                   not a generic posture" at a glance. -->
              <div class="border-t border-surface1 mt-1"></div>
              <div class="px-3 pt-2 pb-1 text-[10px] font-semibold uppercase tracking-[0.14em] text-secondary">Personas</div>
              {#each PERSONAS as m (m.id)}
                <button
                  type="button"
                  role="option"
                  aria-selected={m.id === modeId}
                  onclick={() => { selectMode(m.id); modePickerOpen = false; }}
                  class="w-full flex items-center gap-2.5 px-3 py-2 hover:bg-surface0 text-left transition-colors {m.id === modeId ? 'bg-surface1' : ''}"
                >
                  <span class="text-[11px] font-semibold tracking-tight leading-none flex-shrink-0 inline-flex items-center justify-center w-7 h-7 rounded-md bg-secondary text-on-primary">{m.glyph}</span>
                  <div class="flex-1 min-w-0">
                    <div class="text-[13px] font-medium text-text leading-tight">{m.label}</div>
                    <div class="text-[11px] text-dim leading-snug italic mt-0.5">{m.tagline}</div>
                  </div>
                  {#if m.id === modeId}
                    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 text-primary flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                      <polyline points="20 6 9 17 4 12"/>
                    </svg>
                  {/if}
                </button>
              {/each}
            {/if}
          </div>
        {/if}
      </div>
      {#if statusInfo}
        <!-- Status pill — provider · model. Visible on every viewport
             so the user always knows which backend the next turn will
             hit (matters for cost transparency on paid providers and
             for "why is this slow?" when ollama is local). On narrow
             panels we truncate harder (max-w-[5.5rem]) so the mode
             label doesn't ellipsize. The pill becomes a tap-target on
             mobile so a long-press / title shows the full string. -->
        <span
          class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-surface1 text-subtext truncate inline-block max-w-[5.5rem] sm:max-w-[10rem]"
          title="{statusInfo.provider} · {statusInfo.model} — default backend (per-feature overrides apply individually)"
        >{statusInfo.model}</span>
      {/if}
      <span class="flex-1"></span>
      {#if busy}
        <!-- Animated thinking-pill. Replaces the earlier tiny "cancel"
             text link. Three pulsing dots + label + clear cancel
             button make it obvious that work is in flight + give a
             prominent affordance to stop. Reduced-motion users get
             a static row instead of breathing dots. -->
        <div
          class="inline-flex items-center gap-2 px-2 py-0.5 rounded-md bg-surface1 text-[11px]"
          aria-live="polite"
        >
          <span class="ai-thinking-dots inline-flex items-center gap-0.5" aria-hidden="true">
            <span class="ai-thinking-dot block w-1 h-1 rounded-full bg-primary"></span>
            <span class="ai-thinking-dot block w-1 h-1 rounded-full bg-primary"></span>
            <span class="ai-thinking-dot block w-1 h-1 rounded-full bg-primary"></span>
          </span>
          <span class="text-subtext font-medium hidden sm:inline">thinking</span>
          <button
            onclick={cancelInflight}
            class="text-warning hover:text-error font-medium px-1 -mx-1 rounded hover:bg-surface0 transition-colors"
            title="Stop the in-flight request (Esc)"
          >stop</button>
        </div>
      {/if}
      <!-- History toggle. The side rail beneath shows saved threads
           + pinned messages; auto-saves so the user never loses a
           good chat. New-thread button starts fresh while preserving
           the previous one in history. -->
      <button
        type="button"
        onclick={() => { historyOpen = !historyOpen; }}
        aria-pressed={historyOpen}
        aria-label="Chat history"
        title="Chat history (saved threads + pinned messages)"
        class="tap-target inline-flex items-center justify-center px-1.5 py-1 rounded text-dim hover:text-text hover:bg-surface0 active:bg-surface1 transition-colors {historyOpen ? 'text-primary bg-surface1' : ''}"
      >
        <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="9"/>
          <polyline points="12 7 12 12 15 14" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      </button>
      <button
        type="button"
        onclick={startNewThread}
        aria-label="New thread"
        title="Start a new conversation (current one is saved)"
        class="tap-target inline-flex items-center justify-center px-1.5 py-1 rounded text-dim hover:text-text hover:bg-surface0 active:bg-surface1 transition-colors"
      >
        <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 4v16M4 12h16" stroke-linecap="round"/>
        </svg>
      </button>
      <!-- Pin to right — desktop only. When pinned, the panel becomes
           a fixed right column rather than an overlapping sheet; the
           main page reserves a right gutter (set via the
           --ai-pinned-w CSS variable on <html>) so content reflows
           around it instead of sliding underneath. -->
      <button
        type="button"
        onclick={toggleAIOverlayPinned}
        aria-label={$aiOverlayPinned ? 'Unpin AI panel' : 'Pin AI panel to right'}
        aria-pressed={$aiOverlayPinned}
        title={$aiOverlayPinned ? 'Unpin from right' : 'Pin to right edge'}
        class="hidden md:inline-flex tap-target items-center justify-center px-1.5 py-1 rounded transition-colors {$aiOverlayPinned ? 'text-primary bg-surface1' : 'text-dim hover:text-text hover:bg-surface0 active:bg-surface1'}"
      >
        <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M9 4h6l-1 7 4 3v2h-5v6l-1 1-1-1v-6H6v-2l4-3z"/>
        </svg>
      </button>
      <button
        onclick={close}
        aria-label="close"
        class="tap-target inline-flex items-center justify-center text-dim hover:text-text hover:bg-surface0 active:bg-surface1 rounded px-2 py-1 text-lg leading-none transition-colors"
      >×</button>
    </header>

    {#if statusInfo?.sabbath || $sabbath}
      <div class="mx-4 mt-3 px-3 py-2 text-[11px] bg-warning text-on-primary rounded inline-flex items-center gap-2">
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M12 3v3"/>
          <path d="M9 9c0-2 1.5-3 3-3s3 1 3 3v2H9z"/>
          <rect x="8" y="11" width="8" height="9" rx="1"/>
        </svg>
        <span>Sabbath mode — AI requests are paused today.</span>
      </div>
    {/if}

    <!-- Quick actions row. Two semantic groups separated by a hairline
         divider — context-scoped actions on top (page agent + PM/Cal/
         Goal chips), global utilities below (briefing/synopsis/triage/
         deadlines). Section labels stay subtle but enterprise-grade:
         tight uppercase caps with letter-spacing instead of bracketed
         "PM:" notation. -->
    <div class="px-4 py-2.5 border-b border-surface1 flex-shrink-0 space-y-2">
      {#if pageAgent || currentProjectName || onCalendarPage || currentGoalId}
        <div class="flex items-center gap-2 flex-wrap">
          <span class="text-[10px] font-semibold uppercase tracking-[0.14em] text-dim flex-shrink-0">{currentProjectName ? 'Project' : onCalendarPage ? 'Calendar' : currentGoalId ? 'Goal' : 'Context'}</span>
          {#if pageAgent}
            <!-- Run page-scoped Agent — replaces the per-page "Agent"
                 toolbar buttons. Navigates with ?agent=1 so the host
                 page hydrates and opens its own AgentDialog. -->
            <button
              onclick={launchPageAgent}
              title="Open the agent for this page"
              class="px-2.5 py-1 min-h-[32px] text-xs bg-primary text-on-primary rounded font-medium hover:opacity-90 inline-flex items-center gap-1.5"
            >
              <span class="text-[10px] font-bold tracking-tight inline-flex items-center justify-center w-4 h-4 rounded-sm bg-mantle text-primary leading-none">{pageAgent.glyph}</span>
              <span>Run agent</span>
            </button>
          {/if}
          {#if currentProjectName}
            <button
              onclick={() => { input = `Draft a one-page project brief for ${currentProjectName} — Why · Scope · Out of scope · Definition of done · Stakeholders. Markdown, paste-ready.`; }}
              class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
            >Draft brief</button>
            <button
              onclick={() => { input = `Write a crisp status update for ${currentProjectName} — what shipped, what's open, what's blocked, what's next. 1 short paragraph, no filler.`; }}
              class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
            >Status update</button>
            <button
              onclick={() => { input = `Brainstorm 3-5 distinct directions for ${currentProjectName}'s next milestone. For each: the move, the main risk, what would prove or kill it.`; }}
              class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
            >Brainstorm</button>
            <button
              onclick={() => { input = `Looking at the open tasks + linked goals on ${currentProjectName}, what's the ONE thing I should do next, and why? Pick one, defend it briefly.`; }}
              class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
            >What's next?</button>
          {:else if onCalendarPage}
            <button
              onclick={() => { input = `Describe what my week looks like — heaviest day, lightest day, where the deep-work blocks are or aren't, what's the dominant theme.`; }}
              class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
            >Week shape</button>
            <button
              onclick={() => { input = `Find me a 2-hour focus block in the next 5 days. Propose ONE specific day + start time + reasoning. Don't list options.`; }}
              class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
            >Find focus block</button>
            <button
              onclick={() => { input = `What's overdue and worth doing vs. worth declaring dead? Walk me through it.`; }}
              class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
            >Overdue triage</button>
            <button
              onclick={() => { input = `If I had to clear one meeting from this week to protect a deep-work block, which one and why? Name the trade-off explicitly.`; }}
              class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
            >Clear one meeting</button>
          {:else if currentGoalId}
            <button
              onclick={() => { input = `Write a goal review note for this goal — progress so far, what's working, what's stuck, what to change. 1 short paragraph each section.`; }}
              class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
            >Review note</button>
            <button
              onclick={() => { input = `Reframe this goal sharper — what does success look like specifically, by when, and how will I know I've hit it?`; }}
              class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
            >Reframe</button>
            <button
              onclick={() => { input = `Looking at the open tasks attached to this goal, which ONE moves it forward most this week? Pick one, defend it briefly.`; }}
              class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
            >Highest-leverage next step</button>
            <button
              onclick={() => { input = `Brainstorm 3-5 new milestones for this goal — concrete checkpoints I'd accept as proof of progress. For each: outcome statement + how I'd measure it.`; }}
              class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
            >New milestones</button>
          {/if}
        </div>
      {/if}
      <!-- Global utilities row — always present, separated from
           context chips above. Stays plain so context actions read
           as the headline. -->
      <div class="flex items-center gap-2 flex-wrap">
        <span class="text-[10px] font-semibold uppercase tracking-[0.14em] text-dim flex-shrink-0">Quick</span>
        <button
          onclick={runBriefing}
          disabled={busy || $sabbath}
          class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-text disabled:opacity-50 inline-flex items-center transition-colors"
        >Briefing</button>
        <button
          onclick={runSynopsis}
          disabled={busy || $sabbath}
          class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-text disabled:opacity-50 inline-flex items-center transition-colors"
        >Weekly synopsis</button>
        <button
          onclick={runTriage}
          disabled={busy || $sabbath}
          class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-text disabled:opacity-50 inline-flex items-center transition-colors"
        >Triage</button>
        <button
          onclick={runDeadlines}
          disabled={busy || $sabbath}
          class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary hover:text-text disabled:opacity-50 inline-flex items-center transition-colors"
        >Deadlines</button>
        <span class="flex-1"></span>
        {#if messages.length > 0 || quickResult}
          <button
            onclick={() => void saveThreadAsNote()}
            disabled={saving}
            class="px-2 py-1 text-[11px] text-secondary hover:text-subtext hover:underline disabled:opacity-50 inline-flex items-center gap-1"
            title="Save this thread as a markdown note under chat-history/"
          >
            <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M5 4h11l3 3v13H5z"/>
              <path d="M9 4v5h6V4M8 14h8M8 18h6" stroke-linecap="round"/>
            </svg>
            {saving ? 'saving…' : 'save'}
          </button>
          <button
            onclick={clearChat}
            class="px-2 py-1 text-[11px] text-dim hover:text-error transition-colors"
            title="Clear the overlay"
          >clear</button>
        {/if}
      </div>
    </div>

    <ChatHistoryRail
      bind:this={historyRailRef}
      bind:open={historyOpen}
      {activeThreadId}
      onLoadThread={loadSavedThread}
      onDeleteThread={deleteSavedThread}
      onUnpinForActive={refreshPinnedIndex}
    />

    <!-- Body — quick-action result OR chat thread. Mutually
         exclusive: firing a quick action clears the chat, sending
         a chat message clears the quick result. Keeps the overlay
         single-purpose at any moment. -->
    <!-- min-h-0 is the classic flexbox-with-overflow-auto guard: without
         it, a flex item with overflow:auto refuses to shrink below its
         intrinsic content height. On a tall message list that'd push
         the compose form below the panel's bottom edge (behind the
         keyboard); on a short one the layout's fine but the bug
         surfaces under stress. Safe to always have. -->
    <div bind:this={scrollEl} class="flex-1 min-h-0 overflow-y-auto px-4 py-3">
      {#if quickResult}
        <div class="text-[10px] uppercase tracking-wider text-secondary mb-2">{quickTitle}</div>
        <div class="prose prose-sm max-w-none">
          <MarkdownRenderer body={quickResult} />
        </div>
      {:else if messages.length > 0}
        <ul class="space-y-3">
          {#each messages as m, i (i)}
            <li>
              <div class="text-[10px] uppercase tracking-wider {m.role === 'user' ? 'text-secondary' : 'text-primary'} mb-0.5 flex items-center gap-2">
                <span>{m.role === 'user' ? 'you' : 'assistant'}</span>
                {#if m.role === 'user' && m.content && !busy && savingLibraryIdx !== i}
                  <!-- Save this user prompt to the library so it
                       becomes a one-click entry in the inline AI menu
                       too. Opens an inline label input below; saves
                       through api.putAIPrompts which is a full
                       upsert so we GET, append, PUT. -->
                  <button
                    type="button"
                    onclick={() => openSaveLibrary(i, m.content)}
                    class="tap-target inline-flex items-center justify-center w-6 h-6 rounded text-dim hover:text-secondary hover:bg-surface0 leading-none transition-colors text-[10px]"
                    aria-label="Save this prompt to your library"
                    title="Save this prompt to your AI library — one click to reuse from any surface"
                  >+</button>
                {/if}
                {#if m.role === 'assistant' && m.content && !busy}
                  <span class="ml-auto inline-flex items-center gap-1">
                    <!-- Regenerate — re-run the same user prompt to
                         get a different answer. Truncates the thread
                         at the preceding user message and re-fires
                         send(), so RAG / mentions / snapshot all
                         re-resolve on the fresh attempt. -->
                    <button
                      type="button"
                      onclick={() => regenAssistantMessage(i)}
                      class="tap-target inline-flex items-center justify-center w-7 h-7 rounded text-dim hover:text-primary hover:bg-surface0 active:bg-surface1 leading-none transition-colors"
                      aria-label="Regenerate this reply"
                      title="Re-run the prompt to get a different answer"
                    >
                      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <path d="M3 12a9 9 0 0 1 15-6.7L21 8"/>
                        <path d="M21 3v5h-5"/>
                        <path d="M21 12a9 9 0 0 1-15 6.7L3 16"/>
                        <path d="M3 21v-5h5"/>
                      </svg>
                    </button>
                    <!-- Branch — fork the conversation up to and
                         including this message into a new thread.
                         Original stays in history. -->
                    <button
                      type="button"
                      onclick={() => branchFromMessage(i)}
                      class="tap-target inline-flex items-center justify-center w-7 h-7 rounded text-dim hover:text-secondary hover:bg-surface0 active:bg-surface1 leading-none transition-colors"
                      aria-label="Branch from here"
                      title="Fork the thread from this message into a new conversation"
                    >
                      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2">
                        <circle cx="6" cy="6" r="2"/>
                        <circle cx="18" cy="6" r="2"/>
                        <circle cx="6" cy="18" r="2"/>
                        <path d="M6 8v8M6 12h6a4 4 0 004-4V8" stroke-linecap="round"/>
                      </svg>
                    </button>
                    <!-- Save as note — cuts the draft → copy → new
                         note loop. PM-drafted briefs / status reports
                         file under Projects/<name>/ when in project
                         scope, Drafts/ otherwise. The toast surfaces
                         the resulting path with an Open action. -->
                    <button
                      type="button"
                      onclick={() => void saveAssistantAsNote(i)}
                      disabled={savingMessageIdx !== null}
                      class="tap-target inline-flex items-center justify-center w-7 h-7 rounded text-dim hover:text-success hover:bg-surface0 active:bg-surface1 leading-none transition-colors disabled:opacity-50"
                      aria-label="Save this reply as a vault note"
                      title={currentProjectName
                        ? `Save under Projects/${currentProjectName}/`
                        : 'Save under Drafts/'}
                    >
                      {#if savingMessageIdx === i}
                        <span class="text-[10px]">…</span>
                      {:else}
                        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                          <path d="M5 4h11l3 3v13H5z"/>
                          <path d="M9 4v5h6V4M8 14h8M8 18h6"/>
                        </svg>
                      {/if}
                    </button>
                    <!-- Copy — drops the assistant reply's content
                         straight to clipboard so the user can paste
                         elsewhere without first saving it as a vault
                         note. Falls back silently when the Clipboard
                         API isn't available (HTTP context, ancient
                         browsers); toast confirms success or hints
                         at the failure mode. -->
                    <button
                      type="button"
                      onclick={() => copyAssistantMessage(m.content, i)}
                      class="tap-target inline-flex items-center justify-center w-7 h-7 rounded leading-none hover:bg-surface0 active:bg-surface1 transition-colors {copiedMessageIdx === i ? 'text-success' : 'text-dim hover:text-primary'}"
                      aria-label="Copy this reply to clipboard"
                      title={copiedMessageIdx === i ? 'Copied!' : 'Copy reply to clipboard'}
                    >
                      {#if copiedMessageIdx === i}
                        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                          <polyline points="20 6 9 17 4 12"/>
                        </svg>
                      {:else}
                        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                          <rect x="9" y="9" width="11" height="11" rx="2"/>
                          <path d="M5 15V5a2 2 0 0 1 2-2h10"/>
                        </svg>
                      {/if}
                    </button>
                    <!-- Pin star — toggles a per-message pin so the
                         reply can be retrieved from the Pinned tab.
                         Snapshots content at click time so a future
                         re-roll / thread prune doesn't lose the text. -->
                    <button
                      type="button"
                      onclick={() => pinAssistantMessage(i)}
                      class="tap-target inline-flex items-center justify-center w-7 h-7 rounded text-base leading-none hover:bg-surface0 active:bg-surface1 transition-colors {pinnedIndex[i] ? 'text-warning' : 'text-dim hover:text-warning'}"
                      aria-pressed={!!pinnedIndex[i]}
                      title={pinnedIndex[i] ? 'Unpin this reply' : 'Pin this reply (find it under History → Pinned)'}
                    >
                      {#if pinnedIndex[i]}★{:else}☆{/if}
                    </button>
                    <!-- Insert at cursor — only when a note editor is
                         actively mounted (notes/[...path] page). Drops
                         this reply's text where the cursor is, exactly
                         like a paste; replaces any selection. Lets the
                         user drag a chat answer into the doc without
                         leaving the conversation. -->
                    {#if $hasActiveEditor}
                      <button
                        type="button"
                        onclick={() => insertAtCursor(m.content)}
                        class="tap-target inline-flex items-center justify-center w-7 h-7 rounded text-dim hover:text-primary hover:bg-surface0 active:bg-surface1 leading-none transition-colors"
                        aria-label="Insert this reply at the editor's cursor"
                        title="Insert this reply at the current cursor position in the open note"
                      >
                        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                          <path d="M5 12h11"/>
                          <path d="M12 5l7 7-7 7"/>
                        </svg>
                      </button>
                    {/if}
                  </span>
                {/if}
              </div>
              {#if m.role === 'user' && savingLibraryIdx === i}
                <!-- Inline save-to-library form: small input for a label,
                     submit pushes the prompt into the user's library
                     so it surfaces in the inline AI menu's Library
                     section. The prompt body is the message content
                     verbatim. Esc closes without saving. -->
                <form
                  onsubmit={(e) => { e.preventDefault(); confirmSaveLibrary(m.content); }}
                  class="mt-1 flex items-center gap-1.5"
                >
                  <input
                    type="text"
                    bind:value={savingLibraryLabel}
                    placeholder="short name (e.g. 'tighten', 'my voice')"
                    onkeydown={(e) => { if (e.key === 'Escape') { e.preventDefault(); cancelSaveLibrary(); } }}
                    class="flex-1 px-2 py-1 text-xs bg-surface0 border border-surface1 rounded text-text focus:outline-none focus:border-secondary"
                    use:focusOnMount
                    disabled={savingLibraryBusy}
                  />
                  <button type="submit" disabled={savingLibraryBusy || !savingLibraryLabel.trim()} class="text-xs px-2 py-1 bg-secondary text-on-primary rounded font-medium hover:opacity-90 disabled:opacity-50">save</button>
                  <button type="button" onclick={cancelSaveLibrary} class="text-xs text-dim hover:text-text px-1">cancel</button>
                </form>
              {/if}
              {#if m.role === 'user'}
                {#if editingUserIdx === i}
                  <!-- Inline-edit a user message. Save truncates
                       everything after this turn, resubmits with the
                       edited content; cancel restores the original.
                       Mod-Enter / Enter (without shift) submits;
                       Esc cancels. Auto-grows up to ~10 lines, then
                       scrolls inside the textarea. -->
                  <div class="mt-1">
                    <textarea
                      bind:value={editingUserDraft}
                      class="w-full bg-surface0 border border-primary rounded p-2 text-base md:text-sm text-text resize-none focus:outline-none focus:border-primary"
                      rows={Math.min(10, Math.max(2, (editingUserDraft.match(/\n/g)?.length ?? 0) + 1))}
                      onkeydown={(e) => {
                        if (e.key === 'Escape') { e.preventDefault(); cancelEditUser(); return; }
                        if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
                          e.preventDefault();
                          submitEditUser();
                        }
                      }}
                      use:focusOnMount
                    ></textarea>
                    <div class="mt-1.5 flex items-center gap-2 text-[11px]">
                      <button
                        type="button"
                        onclick={submitEditUser}
                        class="tap-target px-2.5 py-1 rounded bg-primary text-on-primary font-medium hover:opacity-90"
                      >Save & resubmit</button>
                      <button
                        type="button"
                        onclick={cancelEditUser}
                        class="tap-target px-2.5 py-1 rounded bg-surface0 border border-surface1 text-subtext hover:bg-surface1"
                      >Cancel</button>
                      <span class="text-dim">Enter to submit · Esc to cancel</span>
                    </div>
                  </div>
                {:else}
                  <div class="group flex items-start gap-1.5">
                    <div class="flex-1 text-sm text-text whitespace-pre-wrap">{m.content}</div>
                    {#if !busy}
                      <!-- Edit pencil — visible on hover (desktop)
                           and always visible on touch (no hover) so
                           mobile users can still discover the
                           affordance. Resubmits the edited message,
                           truncating everything after this turn. -->
                      <button
                        type="button"
                        onclick={() => startEditUser(i)}
                        class="tap-target opacity-0 group-hover:opacity-100 focus:opacity-100 [@media(hover:none)]:opacity-100 inline-flex items-center justify-center w-7 h-7 rounded text-dim hover:text-text hover:bg-surface0 active:bg-surface1 transition-opacity"
                        aria-label="Edit and resubmit"
                        title="Edit this message and resubmit"
                      >
                        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                          <path d="M12 20h9"/>
                          <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4 12.5-12.5z"/>
                        </svg>
                      </button>
                    {/if}
                  </div>
                {/if}
              {:else}
                {@const cleaned = stripStructuredBlocks(m.content || '')}
                {@const followups = busy && i === messages.length - 1 ? [] : parseFollowups(m.content || '')}
                {@const actions = busy && i === messages.length - 1 ? [] : parseActions(m.content || '')}
                {@const inflight = busy && i === messages.length - 1}
                <div class="prose prose-sm max-w-none">
                  <MarkdownRenderer body={cleaned || '_…_'} />
                  {#if inflight}
                    <!-- Streaming caret. Inserted after the markdown
                         so the user has a continuous "still writing"
                         signal between chunks. Blinks at 1.06s so it
                         reads as live composition rather than a stuck
                         cursor. CSS in the style block at end of file. -->
                    <span class="ai-streaming-caret text-primary" aria-hidden="true"></span>
                  {/if}
                </div>
                {#if actions.length > 0}
                  <!-- Vault action chips proposed by the assistant —
                       one tap creates the task / event / note / memory
                       entry, with a confirmation toast. Each chip
                       de-dupes itself after click via committedActions
                       so a regen with the same proposal doesn't re-fire. -->
                  <div class="mt-2 flex flex-wrap gap-1.5">
                    {#each actions as a, ai (actionKey(i, a) + ai)}
                      {@const k = actionKey(i, a)}
                      {@const committed = !!committedActions[k]}
                      <button
                        type="button"
                        onclick={() => commitAction(i, a)}
                        disabled={committed}
                        class="tap-target inline-flex items-center gap-1 px-2 py-1 rounded text-[11px] transition-colors {committed
                          ? 'bg-success text-on-primary cursor-default'
                          : 'bg-surface0 text-text hover:bg-surface1'}"
                        title={committed ? 'Already committed' : 'Click to commit this action'}
                      >
                        {#if committed}✓{:else}+{/if}
                        {#if a.type === 'task'}
                          Task: {a.text}{a.dueDate ? ` (${a.dueDate})` : ''}
                        {:else if a.type === 'event'}
                          Event: {a.title} ({a.start.slice(11, 16)})
                        {:else if a.type === 'note'}
                          Note: {a.title}
                        {:else if a.type === 'remember'}
                          Remember: {a.content}
                        {/if}
                      </button>
                    {/each}
                  </div>
                {/if}
                {#if followups.length > 0}
                  <!-- Suggested follow-ups — one tap dispatches the
                       prompt through send(). Cheap to render: pure
                       text parsing of the assistant's reply suffix. -->
                  <div class="mt-2 flex flex-wrap gap-1.5">
                    {#each followups as fu, fi (i + ':fu:' + fi)}
                      <button
                        type="button"
                        onclick={() => sendFollowup(fu)}
                        class="tap-target inline-flex items-center gap-1 px-2 py-1 rounded text-[11px] bg-surface0 text-subtext hover:text-text hover:bg-surface1 transition-colors"
                        title="Send as next message"
                      >
                        <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                          <path d="M5 12h14M13 5l7 7-7 7"/>
                        </svg>
                        {fu}
                      </button>
                    {/each}
                  </div>
                {/if}
                {#if perTurnRagHits[i]?.length}
                  <!-- Inline Sources for this turn — a collapsible
                       strip below the assistant reply. The bottom
                       attribution strip shows the most recent set;
                       this lets the user scroll back through a long
                       thread and see exactly which notes grounded
                       each answer. -->
                  <details
                    open={!!expandedSources[i]}
                    class="mt-2 text-[11px]"
                    ontoggle={(e) => { expandedSources = { ...expandedSources, [i]: (e.currentTarget as HTMLDetailsElement).open }; }}
                  >
                    <summary class="cursor-pointer text-dim hover:text-text inline-flex items-center gap-1">
                      <svg viewBox="0 0 24 24" class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M4 4h12l4 4v12H4z"/>
                        <path d="M16 4v4h4M8 12h8M8 16h6" stroke-linecap="round"/>
                      </svg>
                      <span>Sources · {perTurnRagHits[i].length}</span>
                    </summary>
                    <ul class="mt-1.5 space-y-1.5 pl-4 border-l border-surface1">
                      {#each perTurnRagHits[i] as h (h.path)}
                        <li>
                          <a
                            href="/notes/{encodeURIComponent(h.path)}"
                            class="text-secondary hover:underline font-medium"
                            title={h.path}
                          >{h.title}</a>
                          <span class="text-dim font-mono ml-1.5 text-[10px]">{h.path}</span>
                          <p class="text-dim leading-snug mt-0.5 line-clamp-2">{h.excerpt}</p>
                        </li>
                      {/each}
                    </ul>
                  </details>
                {/if}
              {/if}
            </li>
          {/each}
        </ul>
      {:else}
        <!-- Empty state. Surface the few power-user shortcuts so a
             new user knows the surface is denser than it looks. The
             three shortcut chips below are tappable on mobile and
             pre-fill the composer; the user reads what they do, taps
             one, gets immediate momentum. -->
        <div class="text-xs text-dim leading-relaxed">
          <p class="mb-3 text-text">Ask anything, or pick a starter.</p>
          <div class="flex flex-wrap gap-1.5 mb-3">
            <button
              type="button"
              onclick={() => { input = '/help'; void send(); }}
              class="tap-target px-2.5 py-1 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary text-[11px] transition-colors"
            >See what's here</button>
            <button
              type="button"
              onclick={() => { input = '@'; refocusComposer(); mentionPickerRef?.detectTrigger(); }}
              class="tap-target px-2.5 py-1 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary text-[11px] transition-colors"
            >Reference an item</button>
            {#if voiceSupported}
              <button
                type="button"
                onclick={toggleVoice}
                class="tap-target px-2.5 py-1 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary text-[11px] transition-colors"
              >Dictate</button>
            {/if}
          </div>
          <p class="text-[11px] leading-relaxed">
            <kbd class="px-1 py-0.5 bg-surface1 rounded font-mono text-[10px]">Mod+J</kbd> toggle ·
            <kbd class="px-1 py-0.5 bg-surface1 rounded font-mono text-[10px]">/</kbd> commands ·
            <kbd class="px-1 py-0.5 bg-surface1 rounded font-mono text-[10px]">@</kbd> entities ·
            <kbd class="px-1 py-0.5 bg-surface1 rounded font-mono text-[10px]">Mod+1..9</kbd> mode
          </p>
        </div>
      {/if}
    </div>

    {#if lastRagHits.length > 0}
      <!-- RAG attribution strip — shows which vault notes the
           assistant saw on the last turn so the user can verify
           grounding. Click any to open the actual note. Compact
           by default; line-truncates on mobile. -->
      <div class="border-t border-surface1 px-4 py-1.5 flex items-center gap-1.5 flex-wrap text-[11px] flex-shrink-0">
        <span class="text-dim">retrieved:</span>
        {#each lastRagHits as h (h.path)}
          <a
            href="/notes/{encodeURIComponent(h.path)}"
            class="text-secondary hover:underline truncate max-w-[12rem]"
            title={h.path}
          >{h.title}</a>
        {/each}
      </div>
    {/if}

    {#if currentNotePath}
      <!-- Note-context chip. Lets the user toggle whether the
           current note is attached to the next chat message. The
           server-side notePath expander on /chat/stream injects
           the note's body into the system prompt; we only show
           the path here so the user knows what we're sending. -->
      <div class="border-t border-surface1 px-4 py-2 flex items-center gap-2 flex-shrink-0 text-[11px] flex-wrap">
        <label class="flex items-center gap-1.5 cursor-pointer flex-1 min-w-[10rem]">
          <input
            type="checkbox"
            bind:checked={attachNote}
            class="w-3.5 h-3.5 accent-primary cursor-pointer flex-shrink-0"
          />
          <span class="text-dim flex-shrink-0">attach</span>
          <span class="text-subtext font-mono truncate" title={currentNotePath}>{currentNotePath}</span>
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer flex-shrink-0" title="Search the vault for relevant notes and include their excerpts as grounding context">
          <input
            type="checkbox"
            bind:checked={rag}
            class="w-3.5 h-3.5 accent-primary cursor-pointer flex-shrink-0"
          />
          <span class="text-dim">RAG</span>
        </label>
      </div>
    {:else}
      <!-- Snapshot-context chip. On non-note routes the AI gets
           the Context Engine's snapshot — events, tasks, recent
           notes, goals, deadlines — so freeform questions like
           "what should I do next?" have actual data to lean on
           rather than guesses. Only injected on the first turn
           of a thread (subsequent turns lean on the model's own
           reply context to avoid burning tokens). RAG is the
           sibling toggle: search the full vault per turn and
           prepend the top matching notes' excerpts so cross-vault
           questions get grounded answers. -->
      <div class="border-t border-surface1 px-4 py-2 flex items-center gap-3 flex-shrink-0 text-[11px] flex-wrap">
        <label class="flex items-center gap-1.5 cursor-pointer flex-1 min-w-[10rem]">
          <input
            type="checkbox"
            bind:checked={attachSnapshot}
            disabled={snapshotLoading}
            class="w-3.5 h-3.5 accent-primary cursor-pointer flex-shrink-0 disabled:opacity-50"
          />
          <span class="text-dim flex-shrink-0">snapshot</span>
          <span class="text-subtext font-mono truncate">
            {#if snapshotLoading}
              loading…
            {:else if snapshotData}
              today's vault
            {:else}
              unavailable
            {/if}
          </span>
          {#if !snapshotLoading && !snapshotData}
            <button
              type="button"
              onclick={(e) => { e.preventDefault(); void loadSnapshot(); }}
              class="text-secondary hover:underline ml-1"
            >retry</button>
          {/if}
        </label>
        <label class="flex items-center gap-1.5 cursor-pointer flex-shrink-0" title="Search the vault for relevant notes per question and include their excerpts as grounding context">
          <input
            type="checkbox"
            bind:checked={rag}
            class="w-3.5 h-3.5 accent-primary cursor-pointer flex-shrink-0"
          />
          <span class="text-dim">RAG</span>
        </label>
      </div>
    {/if}

    {#if mentionedRefs.length > 0}
      <!-- Mentioned-entities chip strip. Surfaces above the composer
           so the user sees what entity context will be attached to
           the next send. Click × to drop one. Cleared automatically
           after the message goes out. -->
      <div class="border-t border-surface1 px-4 py-1.5 flex flex-wrap gap-1 text-[11px] flex-shrink-0">
        <span class="text-dim self-center">refs:</span>
        {#each mentionedRefs as r, i (r.kind + ':' + r.id + ':' + i)}
          <span class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-secondary text-on-primary">
            <span class="text-[9px] uppercase tracking-wider">{r.kind}</span>
            <span class="truncate max-w-[10rem]" title={r.title}>{r.title}</span>
            <button
              type="button"
              onclick={() => removeMention(i)}
              class="hover:text-error leading-none"
              aria-label="Remove reference"
            >×</button>
          </span>
        {/each}
      </div>
    {/if}

    <!-- Cross-source recents — prompts the user wrote in the inline
         AI menu on a note. Only renders when this is a fresh chat
         (no messages yet) AND the composer is empty, so we don't
         drift items in/out under the user's fingers mid-conversation.
         Click a chip → loads it into the composer; the user reviews
         then sends. We don't auto-send (the inline menu's context
         may not apply here, so the user might want to edit first). -->
    {#if !busy && !$sabbath && messages.length === 0 && input.trim().length === 0 && crossRecentInlinePrompts.length > 0}
      <div class="border-t border-surface1 px-4 py-1.5 flex flex-wrap items-center gap-1 text-[11px] flex-shrink-0">
        <span class="text-dim self-center" title="recent prompts from the inline AI menu in your notes">from notes:</span>
        {#each crossRecentInlinePrompts as r, i (r.prompt + ':' + i)}
          <button
            type="button"
            onclick={() => { input = r.prompt; inputEl?.focus(); }}
            class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-surface0 hover:bg-surface1 text-subtext max-w-[14rem] truncate"
            title={r.notePath ? `from ${r.notePath}: ${r.prompt}` : r.prompt}
          >↗ {r.prompt}</button>
        {/each}
      </div>
    {/if}

    <!-- Chat input. ChatGPT-style composer: textarea wraps the full
         width, mic + send sit as icon buttons inside the bottom-right
         corner. Disabled during Sabbath. -->
    <form
      onsubmit={send}
      class="border-t border-surface1 px-3 py-3 flex-shrink-0"
    >
      <div
        class="relative bg-surface0 border rounded-2xl px-3 py-2 transition-colors {recording ? 'border-error' : 'border-surface1 focus-within:border-primary'}"
      >
        <!-- font-size: 16px on mobile is CRITICAL. iOS Safari
             auto-zooms any focused input with font-size < 16px and,
             while zooming, scrolls the page to centre the input —
             dragging fixed-positioned ancestors (this whole AI panel)
             up with it. That's the long-standing "input field
             wanders to the top, big gap to the keyboard" bug the
             user kept reporting through three previous height-math
             fixes. The actual root cause is the 14px text-sm here.
             text-base md:text-sm gives 16px on mobile (no iOS zoom)
             + 14px on desktop (matches the rest of the chat). -->
        <textarea
          bind:this={inputEl}
          bind:value={input}
          onkeydown={onInputKey}
          oninput={onInputChange}
          onclick={onInputClick}
          rows="2"
          placeholder={$sabbath ? 'Sabbath active — AI paused' : recording ? 'Listening… speak freely' : 'Ask anything, /help for commands, @ to reference…'}
          disabled={busy || $sabbath}
          class="w-full bg-transparent border-0 text-base md:text-sm text-text placeholder-dim focus:outline-none resize-none disabled:opacity-60 pr-20"
          style="min-height: 2.5rem; max-height: 12rem;"
        ></textarea>
        <!-- Bottom-right action cluster — mic (optional) + send.
             Anchored inside the textarea wrapper so the input grows
             vertically while the buttons stay pinned to its corner. -->
        <div class="absolute right-2 bottom-2 flex items-center gap-1">
          {#if voiceSupported}
            <button
              type="button"
              onclick={toggleVoice}
              disabled={busy || $sabbath}
              aria-pressed={recording}
              class="w-8 h-8 inline-flex items-center justify-center rounded-full disabled:opacity-40 transition-colors {recording ? 'bg-error text-white animate-pulse' : 'text-subtext hover:bg-surface1 hover:text-text'}"
              title={recording ? 'Stop dictating' : 'Dictate'}
              aria-label={recording ? 'Stop dictating' : 'Dictate'}
            >
              <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <rect x="9" y="3" width="6" height="12" rx="3"/>
                <path d="M5 11a7 7 0 0014 0M12 18v3"/>
              </svg>
            </button>
          {/if}
          <button
            type="submit"
            disabled={busy || !input.trim() || $sabbath}
            aria-label="Send"
            title="Send (Enter)"
            class="w-8 h-8 inline-flex items-center justify-center rounded-full bg-primary text-on-primary disabled:opacity-30 hover:opacity-90 active:opacity-80 transition-opacity"
          >
            <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 19V5M5 12l7-7 7 7"/>
            </svg>
          </button>
        </div>
        <!-- Slash-command + mention pickers. Same wiring as before;
             rendered as children of the wrapper so their popovers
             anchor to the input box. -->
        <SlashCommandPicker
          bind:this={slashPickerRef}
          bind:value={input}
          bind:open={slashPickerOpen}
          {inputEl}
          onSubmit={() => { void send(); }}
        />
        {#if !slashPickerOpen}
          <MentionPicker
            bind:this={mentionPickerRef}
            bind:value={input}
            bind:open={mentionPickerOpen}
            {inputEl}
            onPick={(ref) => { mentionedRefs = [...mentionedRefs, ref]; }}
          />
        {/if}
      </div>
    </form>
  </div>
{/if}

<style>
  /* Desktop panel width is driven by a CSS variable so the user-
     dragged width sticks even across HMR. Tailwind's arbitrary-
     value classes don't reliably resolve var() for sizing utilities
     across all build configs, so we keep the width here in a
     scoped style block. Mobile (max-width 767px) gets full-width
     bottom-sheet behaviour from the Tailwind classes above. */
  @media (min-width: 768px) {
    :global(.ai-overlay-panel) {
      width: var(--ai-panel-w, 420px);
      transition: width 150ms ease-out;
    }
    :global(.ai-overlay-panel.ai-overlay-resizing) {
      transition: none;
      user-select: none;
    }
  }

  /* Mobile bottom-sheet height + keyboard-safe lift. --ai-sheet-h is
     written from the Svelte side as the snap-target height computed
     against visualViewport.height (always the visible area above the
     keyboard), so the panel height is already correct without any
     CSS clamp. --ai-sheet-lift is the obscured strip height — used
     for `bottom` so the panel anchors flush against the keyboard
     top, not the viewport bottom.

     Earlier code carried a `max-height: calc(100dvh - lift)` clamp
     as defence-in-depth. Removed: `100dvh` on iOS Safari does NOT
     shrink for the keyboard, so subtracting lift from 100dvh gave
     the correct visible area on iOS but accidentally aligned with
     the wrong reference on Chrome Android (where dvh DOES shrink).
     The JS-side height is now the single source of truth. */
  @media (max-width: 767px) {
    :global(.ai-overlay-panel) {
      height: var(--ai-sheet-h, 65dvh);
      bottom: var(--ai-sheet-lift, 0px);
      /* Tween height + bottom for the snap-into-place feeling; the
         snapping class below kills the transition during an active
         drag so the sheet follows the finger 1:1. */
      transition: height 220ms cubic-bezier(0.22, 1, 0.36, 1),
                  bottom 180ms ease-out,
                  top 220ms cubic-bezier(0.22, 1, 0.36, 1);
    }
    /* Keyboard-open override: anchor the panel by top + bottom
       (visualViewport.top = 0 always; bottom = lift to clear the
       keyboard). `height: auto` lets the browser compute it from
       the anchors, which is bulletproof across the iOS/Android
       split where dvh / innerHeight / vv.height each disagree on
       what "visible" means. The flex column inside renders header
       at top, body filling, compose form at bottom = above the
       keyboard. No snap-height math needed in this state. */
    :global(.ai-overlay-panel.ai-overlay-kb-open) {
      top: 0;
      height: auto;
      max-height: none;
    }
    :global(.ai-overlay-panel.ai-overlay-snapping) {
      transition: none;
      user-select: none;
    }
  }

  /* Thinking-dots animation. Three dots breathe in sequence so the
     row reads as "work in progress" without spinning busy-glyphs.
     Pure CSS — no rAF cost per frame. Respects prefers-reduced-
     motion: the dots stop pulsing and just hold at 0.6 opacity so
     the user still sees the pill but no animation runs. */
  .ai-thinking-dot {
    animation: ai-thinking-pulse 1.2s ease-in-out infinite;
    opacity: 0.4;
  }
  .ai-thinking-dot:nth-child(2) { animation-delay: 0.15s; }
  .ai-thinking-dot:nth-child(3) { animation-delay: 0.3s; }
  @keyframes ai-thinking-pulse {
    0%, 80%, 100% { opacity: 0.3; transform: scale(0.85); }
    40% { opacity: 1; transform: scale(1.15); }
  }
  @media (prefers-reduced-motion: reduce) {
    .ai-thinking-dot { animation: none; opacity: 0.6; transform: none; }
  }

  /* Streaming caret on the in-flight assistant message. Inserted as
     a final inline-block sibling so it tracks the last line's
     end-of-content position without disturbing the markdown
     renderer. Blinks at 1.06s — a touch slower than the OS default
     so it reads as "live writing" rather than "broken cursor". */
  :global(.ai-streaming-caret) {
    display: inline-block;
    width: 0.5em;
    height: 1em;
    margin-left: 1px;
    background: currentColor;
    vertical-align: -0.15em;
    opacity: 0.65;
    animation: ai-caret-blink 1.06s steps(1, end) infinite;
  }
  @keyframes ai-caret-blink {
    0%, 49% { opacity: 0.65; }
    50%, 100% { opacity: 0; }
  }
  @media (prefers-reduced-motion: reduce) {
    :global(.ai-streaming-caret) { animation: none; opacity: 0.4; }
  }
</style>
