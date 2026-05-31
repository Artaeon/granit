<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { fly, fade } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { api, type ChatMessage } from '$lib/api';
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
  import { PANEL_WIDTH_MIN, PANEL_WIDTH_MAX } from './ai-overlay-geometry';
  import { createOverlayChrome } from './aioverlay/overlayChrome.svelte';
  import { isMobile } from '$lib/util/breakpoint';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import { AGENT_MODES } from '$lib/ai/agents';
  import { createAIContextManager } from '$lib/chat/aiContextManager.svelte';
  import { buildPrelude } from '$lib/chat/prelude';
  import { commitParsedAction } from '$lib/chat/commitAction';
  import {
    createQuickActionService,
    type QuickActionRefs
  } from '$lib/chat/quickActionService.svelte';
  import {
    createSaveNoteService,
    type SaveNoteRefs
  } from '$lib/chat/saveNoteService.svelte';
  import { suggestedModeForPath } from '$lib/chat/contextDefaults';
  import {
    projectContextChips,
    CALENDAR_CONTEXT_CHIPS,
    GOAL_CONTEXT_CHIPS
  } from '$lib/chat/contextChips';
  import {
    loadActiveThreadId,
    persistActiveThreadId
  } from '$lib/chat/history';
  import {
    createChatHistoryManager,
    type ChatHistoryRefs
  } from '$lib/chat/chatHistoryManager.svelte';
  import {
    createChatSessionManager,
    type ChatSessionRefs,
    type PreludeBundle
  } from '$lib/chat/chatSessionManager.svelte';
  import type { RagHit } from '$lib/chat/rag';
  import { loadOverlayHistory, persistOverlayHistory } from '$lib/chat/overlaySessionHistory';
  import {
    handleSlashCommand as runSlashCommand,
    formatMemoryAsAssistantContent
  } from '$lib/chat/slashCommands';
  import {
    stripStructuredBlocks,
    actionKey,
    type ParsedAction
  } from '$lib/chat/actionParser';
  import { todayISO } from '$lib/util/date';
  import { createVoiceDictation } from '$lib/chat/voiceDictation.svelte';
  import type SlashCommandPicker from '$lib/components/SlashCommandPicker.svelte';
  import type MentionPicker from '$lib/components/MentionPicker.svelte';
  import type { MentionRef } from '$lib/components/MentionPicker.svelte';
  import ChatHistoryRail from '$lib/components/ChatHistoryRail.svelte';
  import ChatModePicker from '$lib/components/aioverlay/ChatModePicker.svelte';
  import ChatComposer from '$lib/components/aioverlay/ChatComposer.svelte';
  import ChatMessageList from '$lib/components/aioverlay/ChatMessageList.svelte';
  import { list as listSharedPrompts, type RecentPrompt } from '$lib/ai/recentPrompts';

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

  // Overlay chrome layer — desktop resize, mobile sheet snap/drag,
  // iOS keyboard offset, and body-scroll lock. State, effects, and
  // pointer handlers live in ./aioverlay/overlayChrome so this file
  // can stay focused on AI conversation logic. The wrapper element
  // is still owned here (bind:this={panelEl} below); the factory
  // reads it via getPanelEl for getBoundingClientRect at drag-start.
  const chrome = createOverlayChrome({
    isOpen: () => open,
    isPinned: () => $aiOverlayPinned,
    isMobileView: () => $isMobile,
    getPanelEl: () => panelEl
  });

  let busy = $state(false);

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
  // back to it after a tangent. The cap + key live in the
  // overlaySessionHistory helper so any future surface that wants
  // to share the same in-flight draft can use one definition.
  let messages = $state<ChatMessage[]>(loadOverlayHistory());
  let input = $state('');
  $effect(() => {
    void messages.length;
    persistOverlayHistory(messages);
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

  // Save-as-note state (saveThreadAsNote in flight + per-message
  // save / copy indicators). Owned here because the message list
  // template reads them; behavior lives in saveNoteService below.
  let saving = $state(false);
  let savingMessageIdx = $state<number | null>(null);
  let copiedMessageIdx = $state<number | null>(null);

  // Quick-action service — owns its own AbortController so a quick
  // action cancel does NOT touch chat send()'s abort lifecycle.
  // Same refs pattern as chatHistoryManager: getter/setter pairs
  // expose parent $state to the service; getter bodies are lazy so
  // referencing later-declared state (messages/quickTitle/etc.) is
  // safe.
  const quickActionRefs: QuickActionRefs = {
    get busy() { return busy; },
    set busy(v) { busy = v; },
    get quickTitle() { return quickTitle; },
    set quickTitle(v) { quickTitle = v; },
    get quickResult() { return quickResult; },
    set quickResult(v) { quickResult = v; },
    get messages() { return messages; },
    set messages(v) { messages = v; }
  };
  const quickActions = createQuickActionService({ refs: quickActionRefs });
  const {
    runBriefing,
    runSynopsis,
    runTriage,
    runDeadlines
  } = quickActions;

  // Save-note service — owns the save / copy / library flows.
  // currentProjectName / currentGoalId / onCalendarPage / mode are
  // declared further down; the getters resolve them lazily at call
  // time (user clicks a save button after onMount has run).
  const saveNoteRefs: SaveNoteRefs = {
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
    get messages() { return messages; },
    set messages(v) { messages = v; },
    get quickTitle() { return quickTitle; },
    set quickTitle(v) { quickTitle = v; },
    get quickResult() { return quickResult; },
    set quickResult(v) { quickResult = v; },
    get modeId() { return aiCtx.mode.id; },
    set modeId(_v) { /* read-only — saveNote never writes mode */ },
    get modeLabel() { return aiCtx.mode.label; },
    set modeLabel(_v) { /* read-only — derived from mode */ },
    get rag() { return aiCtx.rag; },
    set rag(v) { aiCtx.setRag(v); },
    get lastRagHits() { return lastRagHits; },
    set lastRagHits(v) { lastRagHits = v; },
    get currentProjectName() { return currentProjectName; },
    set currentProjectName(_v) { /* derived */ },
    get currentGoalId() { return currentGoalId; },
    set currentGoalId(_v) { /* derived */ },
    get onCalendarPage() { return onCalendarPage; },
    set onCalendarPage(_v) { /* derived */ }
  };
  const saveNote = createSaveNoteService({ refs: saveNoteRefs });
  const {
    saveThreadAsNote,
    saveAssistantAsNote,
    copyAssistantMessage,
    openSaveLibrary,
    cancelSaveLibrary,
    confirmSaveLibrary
  } = saveNote;

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

  // Inline-edit state for user messages. Null = nobody being edited.
  // Lives in the parent because the message list's inline form binds
  // editingUserDraft and the manager reads it on submit.
  let editingUserIdx = $state<number | null>(null);
  let editingUserDraft = $state('');

  // Thread CRUD lives in $lib/chat/chatHistoryManager. The refs object
  // exposes each reactive slot as a getter/setter pair so the manager
  // can read and write parent $state through one indirection. Getter
  // bodies are lazy — they're only evaluated when the manager methods
  // are actually called (after onMount), so referencing later-declared
  // state (messages/modeId/lastRagHits/perTurnRagHits/expandedSources)
  // is safe even though those `let`s appear further down this file.
  const historyRefs: ChatHistoryRefs = {
    get messages() { return messages; },
    set messages(v) { messages = v; },
    get activeThreadId() { return activeThreadId; },
    set activeThreadId(v) { activeThreadId = v; },
    get modeId() { return aiCtx.modeId; },
    set modeId(v) { aiCtx.restoreMode(v); },
    get input() { return input; },
    set input(v) { input = v; },
    get pinnedIndex() { return pinnedIndex; },
    set pinnedIndex(v) { pinnedIndex = v; },
    get perTurnRagHits() { return perTurnRagHits; },
    set perTurnRagHits(v) { perTurnRagHits = v; },
    get expandedSources() { return expandedSources; },
    set expandedSources(v) { expandedSources = v; },
    get quickTitle() { return quickTitle; },
    set quickTitle(v) { quickTitle = v; },
    get quickResult() { return quickResult; },
    set quickResult(v) { quickResult = v; },
    get lastRagHits() { return lastRagHits; },
    set lastRagHits(v) { lastRagHits = v; },
    get historyOpen() { return historyOpen; },
    set historyOpen(v) { historyOpen = v; },
    get editingUserIdx() { return editingUserIdx; },
    set editingUserIdx(v) { editingUserIdx = v; },
    get editingUserDraft() { return editingUserDraft; },
    set editingUserDraft(v) { editingUserDraft = v; }
  };
  const history = createChatHistoryManager({
    refs: historyRefs,
    getHistoryRail: () => historyRailRef,
    isBusy: () => busy,
    send: () => { void send(); },
    refocusInput: () => { tick().then(() => inputEl?.focus()); },
    scrollToBottom: () => {
      tick().then(() => {
        if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
      });
    }
  });
  const {
    startNewThread,
    loadSavedThread,
    deleteSavedThread,
    autoSaveThread,
    replayFromUserMessage,
    regenAssistantMessage,
    branchFromMessage,
    pinAssistantMessage,
    startEditUser,
    cancelEditUser,
    submitEditUser,
    refreshPinnedIndex
  } = history;

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

  // ── AI context manager (mode + RAG + memory) ───────────────────
  // Posture / grounding / long-term facts — the three things the
  // prelude assembler reads to decide what every turn sees. Their
  // policy + the page-aware auto-switch effect live in
  // $lib/chat/aiContextManager. The manager exposes mode + rag +
  // memory as getters and the two action verbs selectMode +
  // restoreMode; the parent reads through aiCtx.* and never owns
  // the underlying state.
  //
  // Sources are lazy getters into the parent's $derived page
  // context (currentProjectName etc. declared further down — JS
  // closure capture handles the forward reference, the deriveds
  // exist by the time the manager's $effect first fires).
  const aiCtx = createAIContextManager({
    sources: {
      currentProjectName: () => currentProjectName,
      currentGoalId: () => currentGoalId,
      onCalendarPage: () => onCalendarPage
    },
    announce: (msg) => announce(msg)
  });

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
    // Abort any in-flight chat stream via the session manager's
    // controller. cancelInflight() is a no-op when nothing's running.
    cancelInflight();
    if (voice.recording) stopVoice();
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
          // selectMode handles persist + RAG-reset + announce in one
          // step so we don't repeat the wiring here.
          if (seed.modeId) aiCtx.selectMode(seed.modeId);
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

  // Quick actions (briefing / synopsis / triage / deadlines) live in
  // $lib/chat/quickActionService — wired below. The service owns its
  // own AbortController so a quick action can't accidentally cancel
  // the chat stream and vice versa.

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
  // The router + SLASH_HELP text live in $lib/chat/slashCommands;
  // this component wires it to local state via the handler closures
  // below.
  //
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
    // The router lives in $lib/chat/slashCommands; we hand it the
    // local closures it needs and let it own the switch. The router
    // does NOT clear `input` itself (that's a Svelte $state concern)
    // — we do that up-front here so every path returns to a clean
    // composer the moment we know the command was recognised.
    const handled = runSlashCommand(raw, AGENT_MODES, {
      appendAssistantReply: (userText, assistantContent) => {
        messages = [
          ...messages,
          { role: 'user', content: userText },
          { role: 'assistant', content: assistantContent }
        ];
      },
      clearChat,
      startNewThread,
      saveThreadAsNote,
      runBriefing,
      runSynopsis,
      runTriage,
      runDeadlines,
      rememberFact: async (fact) => {
        try {
          const f = await api.addAIMemory(fact, []);
          await aiCtx.loadAIMemory();
          toast.success(`Remembered · ${f.content.slice(0, 60)}`);
        } catch (err) {
          toast.error('Memory add failed: ' + errorMessage(err));
        }
      },
      showMemory: async (userText) => {
        await aiCtx.loadAIMemory();
        messages = [
          ...messages,
          { role: 'user', content: userText },
          { role: 'assistant', content: formatMemoryAsAssistantContent(aiCtx.aiMemoryFacts) }
        ];
      },
      forgetFact: async (idPrefix) => {
        await aiCtx.loadAIMemory();
        const match = aiCtx.aiMemoryFacts.find((f) => f.id.toLowerCase().startsWith(idPrefix.toLowerCase()));
        if (!match) {
          toast.error(`No fact id starts with "${idPrefix}"`);
          return;
        }
        try {
          await api.deleteAIMemory(match.id);
          await aiCtx.loadAIMemory();
          toast.success(`Forgot · ${match.content.slice(0, 50)}`);
        } catch (err) {
          toast.error('Forget failed: ' + errorMessage(err));
        }
      },
      selectModeAndToast: (m) => {
        aiCtx.selectMode(m.id);
        toast.success(`${m.glyph} ${m.label} — ${m.tagline}`);
      },
      unknownModeOrPersona: (kind, arg) => {
        toast.error(`Unknown ${kind}: ${arg}`);
      },
      toggleRag: () => {
        const next = aiCtx.toggleRag();
        toast.success(`RAG ${next ? 'on' : 'off'} for the next turn.`);
        announce(`RAG ${next ? 'enabled' : 'disabled'}`);
      },
      detachContext: () => {
        attachNote = false;
        attachSnapshot = false;
        mentionedRefs = [];
        toast.success('Context detached for the next message.');
        announce('Context detached for next message');
      },
      usageError: (msg) => toast.info(msg),
      refocusComposer
    });
    if (handled) input = '';
    return handled;
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
  // The recognition lifecycle + baseline-merge math lives in
  // $lib/chat/voiceDictation.svelte.ts; this component just hands
  // it accessors to the `input` $state.
  const voice = createVoiceDictation({
    getInput: () => input,
    setInput: (next) => { input = next; }
  });
  const voiceSupported = voice.supported;
  function toggleVoice() { voice.toggle(); }
  function stopVoice() { voice.stop(); }

  // ── Long-term AI memory — loader lives in aiCtx ────────────────
  // Persistent facts ("wife is Anna", "vegetarian", etc.) get
  // folded into every thread's first-turn prelude by the prelude
  // assembler. aiCtx owns the state + the fetcher; the parent owns
  // the lifecycle (mount-time prime + WS refresh) because the WS
  // subscription needs the parent's onWsEvent helper anyway.
  onMount(() => {
    void aiCtx.loadAIMemory();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/ai-memory.json') {
        void aiCtx.loadAIMemory();
      }
    });
  });

  // ── Agent-capabilities system instruction ──────────────────────
  // Lives in $lib/chat/prelude.ts together with the per-turn prelude
  // assembly. Injected on the FIRST turn of every new thread, like
  // the vault snapshot — re-injecting on every turn burns tokens
  // for instructions the model has already internalised.

  // Already-committed action chips per message id — keyed by the
  // action's stable signature (see $lib/chat/actionParser.actionKey)
  // so a click doesn't double-commit and a regen with the same
  // proposal stays "fresh" until clicked. Parsing rules live in the
  // dedicated module + are pinned by actionParser.test.ts.
  let committedActions = $state<Record<string, boolean>>({});

  async function commitAction(msgIdx: number, a: ParsedAction) {
    const key = actionKey(msgIdx, a);
    if (committedActions[key]) return;
    const ok = await commitParsedAction(a, {
      api,
      toast,
      currentNotePath,
      defaultDailyNotePath: `Daily/${todayISO()}.md`,
      onMemoryAdded: aiCtx.loadAIMemory
    });
    if (ok) committedActions = { ...committedActions, [key]: true };
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
    // Path → mode mapping lives in $lib/chat/contextDefaults; we only
    // override the user's pick when they're parked on the default
    // 'general' mode so a deliberate choice never gets clobbered.
    const suggested = suggestedModeForPath($page.url.pathname);
    if (suggested && aiCtx.mode.id === 'general') {
      const target = AGENT_MODES.find((m) => m.id === suggested);
      if (target) {
        aiCtx.selectMode(target.id);
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

  // ── Chat session — send / cancel / clear lives here ────────────
  // The streaming send pipeline (composer text → prelude → SSE →
  // tokens into the assistant slot → finally) is owned by
  // $lib/chat/chatSessionManager. The factory takes:
  //   - `refs` for the parent state it reads + writes (matching the
  //     pattern used by chatHistoryManager and the save / quick-action
  //     services);
  //   - a `buildPrelude` closure that captures all the page-aware
  //     policy here (mode + memory + page context + loaders), so the
  //     manager itself is free of $lib/api / page-state knowledge;
  //   - a `chatStream` injection (the api function) so the manager is
  //     pure orchestration and unit-testable with a fake stream;
  //   - the side-effect bridges parent owns (autoSave, awaitSave,
  //     resetForClear, getScrollEl).
  //
  // Race fixes the extraction ships: onError silenced after abort,
  // scrollHeight read after tick, send() gated on in-flight save.
  // See chatSessionManager.svelte.ts for the why on each.
  const sessionRefs: ChatSessionRefs = {
    get input() { return input; },
    set input(v) { input = v; },
    get busy() { return busy; },
    set busy(v) { busy = v; },
    get messages() { return messages; },
    set messages(v) { messages = v; },
    get mentionedRefs() { return mentionedRefs; },
    set mentionedRefs(v) { mentionedRefs = v; },
    get lastRagHits() { return lastRagHits; },
    set lastRagHits(v) { lastRagHits = v; },
    get perTurnRagHits() { return perTurnRagHits; },
    set perTurnRagHits(v) { perTurnRagHits = v; },
    get quickTitle() { return quickTitle; },
    set quickTitle(v) { quickTitle = v; },
    get quickResult() { return quickResult; },
    set quickResult(v) { quickResult = v; }
  };

  async function buildPreludeForSession(
    query: string,
    isFirstTurn: boolean
  ): Promise<PreludeBundle> {
    const { messages: preludeMessages, ragHits } = await buildPrelude({
      mode: aiCtx.mode,
      aiMemoryFacts: aiCtx.aiMemoryFacts,
      mentionedRefs,
      currentNotePath,
      currentProjectName,
      currentGoalId,
      onCalendarPage,
      attachSnapshot,
      snapshotData,
      rag: aiCtx.rag,
      isFirstTurn,
      query,
      projectLoaders: {
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
      },
      goalLoaders: {
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
      },
      calendarLoaders: {
        listEvents: async () => {
          const r = await api.listEvents();
          return r.events;
        },
        listTasks: async () => {
          const r = await api.listTasks({});
          return r.tasks;
        }
      },
      todayISO: todayISO()
    });
    return {
      messages: preludeMessages,
      ragHits,
      notePathForStream: attachNote && currentNotePath ? currentNotePath : null
    };
  }

  const chat = createChatSessionManager({
    refs: sessionRefs,
    buildPrelude: buildPreludeForSession,
    chatStream: api.chatStream,
    handleSlashCommand,
    autoSaveThread,
    awaitSave: () => history.awaitSave(),
    resetForClear: () => {
      // The chat manager owns messages / perTurnRagHits / quickTitle /
      // quickResult. clearChat ALSO needs to reset state owned by
      // sibling concerns (history rail's pinnedIndex, ui's
      // expandedSources, the active thread id). Centralise here so
      // the manager doesn't need a wider refs surface.
      expandedSources = {};
      pinnedIndex = {};
      activeThreadId = '';
      persistActiveThreadId('');
    },
    getScrollEl: () => scrollEl
  });
  const { send, sendFollowup, cancelInflight, clearChat } = chat;

  $effect(() => {
    void messages.length;
    void quickResult;
    tick().then(() => {
      if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
    });
  });

  // Composer handlers + auto-grow live in <ChatComposer />. Parent
  // owns `input`, `inputEl`, picker open/refs, and `voice` because
  // surfaces outside the composer (Esc handlers, starter-button
  // strip, send() prelude builder) need to read or mutate them.

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
      aiCtx.selectMode(AGENT_MODES[idx - 1].id);
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
    style:--ai-panel-w="{chrome.panelWidth}px"
    style:--ai-sheet-h={chrome.mobileSheetHeight}
    style:--ai-sheet-lift="{chrome.keyboardOffset}px"
    in:fly={{ duration: $aiOverlayPinned ? 0 : 200, easing: cubicOut, ...panelTransitionParams() }}
    out:fly={{ duration: $aiOverlayPinned ? 0 : 150, easing: cubicOut, ...panelTransitionParams() }}
    class="ai-overlay-panel fixed z-50 flex flex-col bg-base border-surface1
           inset-x-0 rounded-t-xl border-t {chrome.keyboardOpen ? '' : 'pb-safe'} {chrome.keyboardOpen ? 'ai-overlay-kb-open' : ''}
           md:inset-y-0 md:right-0 md:left-auto md:bottom-auto md:top-0 md:h-full md:max-h-none md:rounded-none md:border-l md:border-t-0 md:pb-0 {$aiOverlayPinned ? 'md:shadow-none' : 'shadow-2xl'} {chrome.resizing ? 'ai-overlay-resizing' : ''} {chrome.sheetDragging ? 'ai-overlay-snapping' : ''}"
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
      aria-valuenow={chrome.panelWidth}
      aria-valuemin={PANEL_WIDTH_MIN}
      aria-valuemax={PANEL_WIDTH_MAX}
      role="slider"
      onpointerdown={chrome.onResizeStart}
      onkeydown={chrome.onResizeKey}
      class="hidden md:block absolute left-0 top-0 bottom-0 w-1.5 -ml-0.5 z-50 cursor-col-resize group {chrome.resizing ? 'bg-primary/40' : 'hover:bg-primary focus-visible:bg-primary/40'} transition-colors"
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
      onpointerdown={chrome.onSheetHandleDown}
      onclick={chrome.cycleSheetSnap}
      aria-label="Resize chat sheet — drag or tap to cycle"
    >
      <span class="block w-10 h-1.5 rounded-full bg-surface2 transition-colors {chrome.sheetDragging ? 'bg-primary' : ''}"></span>
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
          title={aiCtx.autoPMActive
            ? `Mode: ${aiCtx.mode.label} (auto — you're on a project page). Click to override.`
            : `Mode: ${aiCtx.mode.label} — ${aiCtx.mode.tagline}`}
        >
          <span class="text-[10px] font-semibold tracking-tight leading-none inline-flex items-center justify-center w-6 h-6 rounded-md {aiCtx.mode.kind === 'persona' ? 'bg-secondary text-on-primary' : aiCtx.mode.kind === 'contextual' ? 'bg-primary text-on-primary' : 'bg-surface1 text-subtext'}">{aiCtx.mode.glyph}</span>
          <span class="text-sm font-semibold truncate max-w-[8rem] sm:max-w-none">{aiCtx.mode.label}</span>
          {#if aiCtx.autoPMActive}
            <!-- Tiny "auto · <source>" badge. The source word makes
                 the contextual switch self-explanatory — the user
                 reads "auto · project" and knows where the mode
                 came from (and that picking anything else takes
                 control back). Clears the moment they pick. -->
            <span class="text-[9px] uppercase tracking-wider px-1 rounded bg-primary text-on-primary leading-tight whitespace-nowrap">auto · {aiCtx.autoMode}</span>
          {/if}
          <svg viewBox="0 0 24 24" class="w-3 h-3 text-dim flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="6 9 12 15 18 9" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </button>
        {#if modePickerOpen}
          <ChatModePicker
            modeId={aiCtx.modeId}
            {currentProjectName}
            {currentGoalId}
            {onCalendarPage}
            onSelect={aiCtx.selectMode}
            onDismiss={() => (modePickerOpen = false)}
          />
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
            {#each projectContextChips(currentProjectName) as c (c.label)}
              <button
                onclick={() => { input = c.prompt; }}
                class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
              >{c.label}</button>
            {/each}
          {:else if onCalendarPage}
            {#each CALENDAR_CONTEXT_CHIPS as c (c.label)}
              <button
                onclick={() => { input = c.prompt; }}
                class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
              >{c.label}</button>
            {/each}
          {:else if currentGoalId}
            {#each GOAL_CONTEXT_CHIPS as c (c.label)}
              <button
                onclick={() => { input = c.prompt; }}
                class="px-2.5 py-1 min-h-[32px] text-xs bg-surface0 rounded text-primary hover:bg-surface1 inline-flex items-center transition-colors"
              >{c.label}</button>
            {/each}
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
        <ChatMessageList
          {messages}
          {busy}
          {pinnedIndex}
          {perTurnRagHits}
          bind:expandedSources
          {committedActions}
          {savingLibraryIdx}
          bind:savingLibraryLabel
          {savingLibraryBusy}
          {editingUserIdx}
          bind:editingUserDraft
          {copiedMessageIdx}
          {savingMessageIdx}
          {currentProjectName}
          onOpenSaveLibrary={openSaveLibrary}
          onCancelSaveLibrary={cancelSaveLibrary}
          onConfirmSaveLibrary={confirmSaveLibrary}
          onRegen={regenAssistantMessage}
          onBranch={branchFromMessage}
          onSaveAsNote={(idx) => void saveAssistantAsNote(idx)}
          onCopy={copyAssistantMessage}
          onPin={pinAssistantMessage}
          onStartEditUser={startEditUser}
          onCancelEditUser={cancelEditUser}
          onSubmitEditUser={submitEditUser}
          onCommitAction={(idx, a) => void commitAction(idx, a)}
          onSendFollowup={sendFollowup}
        />
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
            checked={aiCtx.rag}
            onchange={(e) => aiCtx.setRag(e.currentTarget.checked)}
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
            checked={aiCtx.rag}
            onchange={(e) => aiCtx.setRag(e.currentTarget.checked)}
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

    <!-- Inline mode + persona chips above the composer. The full
         picker still lives in the header dropdown; these chips
         expose the live state so the user reads "what mode am I in
         and which persona is loaded" without opening a menu.
         Click either chip to toggle the mode picker. The picker
         already groups Modes + Personas so one entry-point is
         enough — duplicating the picker would only add weight. -->
    <div class="border-t border-surface1 px-3 pt-2 pb-1 flex items-center gap-1.5 flex-shrink-0 flex-wrap">
      <button
        type="button"
        onclick={() => (modePickerOpen = !modePickerOpen)}
        class="inline-flex items-center gap-1 text-[11px] text-dim hover:text-text px-1.5 py-0.5 rounded bg-surface0 hover:bg-surface1 transition-colors max-w-[10rem]"
        title="Active mode — click to change ({aiCtx.mode.tagline})"
        aria-haspopup="listbox"
        aria-expanded={modePickerOpen}
      >
        <span class="{aiCtx.mode.kind === 'persona' ? 'text-secondary' : aiCtx.mode.kind === 'contextual' ? 'text-primary' : 'text-subtext'}">●</span>
        <span class="truncate">{aiCtx.mode.label}</span>
      </button>
      {#if aiCtx.lastPersona}
        <button
          type="button"
          onclick={() => (modePickerOpen = !modePickerOpen)}
          class="inline-flex items-center gap-1 text-[11px] px-1.5 py-0.5 rounded transition-colors max-w-[10rem] {aiCtx.modeId === aiCtx.lastPersona.id ? 'bg-secondary text-on-primary' : 'text-dim hover:text-text bg-surface0 hover:bg-surface1'}"
          title="Persona — {aiCtx.modeId === aiCtx.lastPersona.id ? 'active' : 'last used'}. Click to open picker."
        >
          <span class="text-[9px] font-mono">{aiCtx.lastPersona.glyph}</span>
          <span class="truncate">{aiCtx.lastPersona.label}</span>
        </button>
      {/if}
      <span class="flex-1"></span>
      <!-- Quick-action strip — collapses the most-used slash commands
           into one-tap chips so the user doesn't have to type
           '/briefing' or remember /clear lives behind a slash.
           These funnel into the same handlers (runBriefing,
           runTriage, clearChat, startNewThread) the slash router
           already calls, so behaviour stays consistent. -->
      <button
        type="button"
        onclick={() => { void runBriefing(); }}
        disabled={busy || $sabbath}
        class="text-[11px] text-dim hover:text-text px-2 py-0.5 rounded hover:bg-surface1 transition-colors disabled:opacity-40"
        title="Run daily briefing (/briefing)"
      >Briefing</button>
      <button
        type="button"
        onclick={() => { void runTriage(); }}
        disabled={busy || $sabbath}
        class="text-[11px] text-dim hover:text-text px-2 py-0.5 rounded hover:bg-surface1 transition-colors disabled:opacity-40"
        title="Triage open tasks (/triage)"
      >Triage</button>
      <button
        type="button"
        onclick={() => { void runDeadlines(); }}
        disabled={busy || $sabbath}
        class="text-[11px] text-dim hover:text-text px-2 py-0.5 rounded hover:bg-surface1 transition-colors disabled:opacity-40"
        title="Surface upcoming deadlines (/deadlines)"
      >Deadlines</button>
      <button
        type="button"
        onclick={() => { startNewThread(); }}
        disabled={busy}
        class="text-[11px] text-dim hover:text-error px-2 py-0.5 rounded hover:bg-surface1 transition-colors disabled:opacity-40"
        title="Start a new thread (/new) — current thread is auto-saved to history"
      >Clear</button>
    </div>

    <ChatComposer
      bind:input
      bind:inputEl
      {panelEl}
      bind:slashPickerOpen
      bind:mentionPickerOpen
      bind:slashPickerRef
      bind:mentionPickerRef
      {voice}
      {busy}
      sabbathActive={$sabbath}
      onSubmit={() => { void send(); }}
      onMentionPick={(ref) => { mentionedRefs = [...mentionedRefs, ref]; }}
    />
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
