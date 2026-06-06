<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { fly, fade } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { api } from '$lib/api';
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
  import {
    createAIStatusLoader,
    createAISnapshotLoader
  } from '$lib/chat/aiOverlayStatusLoader.svelte';
  import { createAIOverlayState } from '$lib/chat/aiOverlayState.svelte';
  import { buildPrelude } from '$lib/chat/prelude';
  import { commitParsedAction } from '$lib/chat/commitAction';
  import {
    deriveCurrentNotePath,
    deriveCurrentProjectName,
    deriveCurrentGoalId,
    deriveOnCalendarPage,
    derivePageAgent,
    buildPageAgentTarget
  } from '$lib/chat/pageContext';
  import { installOverlayShortcuts } from '$lib/chat/overlayShortcuts';
  import { createQuickActionService } from '$lib/chat/quickActionService.svelte';
  import {
    createSaveNoteService,
    type SaveNoteRefs
  } from '$lib/chat/saveNoteService.svelte';
  import { installAIContextDefaults } from '$lib/chat/aiOverlayContextDefaults.svelte';
  import { persistActiveThreadId } from '$lib/chat/history';
  import {
    createChatHistoryManager,
    type ChatHistoryRefs
  } from '$lib/chat/chatHistoryManager.svelte';
  import {
    createChatSessionManager,
    type ChatSessionRefs,
    type PreludeBundle
  } from '$lib/chat/chatSessionManager.svelte';
  import { persistOverlayHistory } from '$lib/chat/overlaySessionHistory';
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
  import ChatHistoryRail from '$lib/components/ChatHistoryRail.svelte';
  import ChatComposer from '$lib/components/aioverlay/ChatComposer.svelte';
  import ChatMessageList from '$lib/components/aioverlay/ChatMessageList.svelte';
  import AIOverlayHeader from '$lib/components/aioverlay/AIOverlayHeader.svelte';
  import AIOverlayQuickActions from '$lib/components/aioverlay/AIOverlayQuickActions.svelte';
  import AIOverlayContextChips from '$lib/components/aioverlay/AIOverlayContextChips.svelte';
  import AIOverlayComposerStrip from '$lib/components/aioverlay/AIOverlayComposerStrip.svelte';
  import AIOverlayEmptyBody from '$lib/components/aioverlay/AIOverlayEmptyBody.svelte';
  import AIOverlayRagStrip from '$lib/components/aioverlay/AIOverlayRagStrip.svelte';
  import AIOverlayMentionedRefs from '$lib/components/aioverlay/AIOverlayMentionedRefs.svelte';
  import AIOverlayCrossSourceRecents from '$lib/components/aioverlay/AIOverlayCrossSourceRecents.svelte';
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

  // All the parent's shared mutable state — busy, input, messages,
  // quickTitle/Result, save-as-note slots, save-to-library slots,
  // thread history, RAG attribution, mentioned refs — lives in one
  // controller. Each service receives `aiState` (or a small composite
  // wrapping it with aiCtx/page derives) instead of its own
  // hand-rolled getter/setter refs block. See aiOverlayState.svelte.ts
  // for the field list + the why on the consolidation.
  const aiState = createAIOverlayState();
  // Local read-only aliases for the template + script-side reads.
  // Writes go through aiState.X directly; bind: directives use
  // bind:X={aiState.X} since $derived can't be l-value bound.
  const busy = $derived(aiState.busy);
  const input = $derived(aiState.input);
  const messages = $derived(aiState.messages);
  const mentionedRefs = $derived(aiState.mentionedRefs);
  const quickTitle = $derived(aiState.quickTitle);
  const quickResult = $derived(aiState.quickResult);
  const saving = $derived(aiState.saving);
  const savingMessageIdx = $derived(aiState.savingMessageIdx);
  const copiedMessageIdx = $derived(aiState.copiedMessageIdx);
  const savingLibraryIdx = $derived(aiState.savingLibraryIdx);
  const savingLibraryBusy = $derived(aiState.savingLibraryBusy);
  const activeThreadId = $derived(aiState.activeThreadId);
  const historyOpen = $derived(aiState.historyOpen);
  const pinnedIndex = $derived(aiState.pinnedIndex);
  const editingUserIdx = $derived(aiState.editingUserIdx);
  const lastRagHits = $derived(aiState.lastRagHits);
  const perTurnRagHits = $derived(aiState.perTurnRagHits);

  // Status pill — provider · model · sabbath. Loader owns the
  // monotonic-gen stale-response guard so a slow earlier call can't
  // clobber a fresh later one.
  const statusLoader = createAIStatusLoader();
  // Vault snapshot — non-note routes use it as default chat context.
  // Same gen guard pattern as the status loader; same "leave null on
  // failure, surface unavailable chip" UX.
  const snapshotLoader = createAISnapshotLoader();

  // Persist the in-flight chat thread to sessionStorage so closing the
  // overlay (Esc / outside-click / Mod+J) doesn't lose it. Survives
  // navigation within the tab; cleared on tab close or explicit reset.
  $effect(() => {
    void aiState.messages.length;
    persistOverlayHistory(aiState.messages);
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
  $effect(() => {
    void aiState.messages.length;
    refreshCrossRecentInlinePrompts();
  });

  // Quick-action service — owns its own AbortController so a quick
  // action cancel does NOT touch chat send()'s abort lifecycle.
  // aiState is a structural superset of QuickActionRefs so it passes
  // directly; no hand-rolled refs object.
  const quickActions = createQuickActionService({ refs: aiState });
  const {
    runBriefing,
    runSynopsis,
    runTriage,
    runDeadlines,
    cancel: cancelQuickAction
  } = quickActions;

  // Save-note service — owns the save / copy / library flows.
  // SaveNoteRefs needs a handful of fields beyond aiState's surface
  // (mode + rag from aiCtx, page-context derives), so the parent
  // builds a small composite: every shared slot delegates to aiState,
  // the derived ones project lazily. Getters resolve at call-time so
  // forward references to currentProjectName / aiCtx are safe.
  const saveNoteRefs: SaveNoteRefs = {
    get saving() { return aiState.saving; },
    set saving(v) { aiState.saving = v; },
    get savingMessageIdx() { return aiState.savingMessageIdx; },
    set savingMessageIdx(v) { aiState.savingMessageIdx = v; },
    get copiedMessageIdx() { return aiState.copiedMessageIdx; },
    set copiedMessageIdx(v) { aiState.copiedMessageIdx = v; },
    get savingLibraryIdx() { return aiState.savingLibraryIdx; },
    set savingLibraryIdx(v) { aiState.savingLibraryIdx = v; },
    get savingLibraryLabel() { return aiState.savingLibraryLabel; },
    set savingLibraryLabel(v) { aiState.savingLibraryLabel = v; },
    get savingLibraryBusy() { return aiState.savingLibraryBusy; },
    set savingLibraryBusy(v) { aiState.savingLibraryBusy = v; },
    get messages() { return aiState.messages; },
    set messages(v) { aiState.messages = v; },
    get quickTitle() { return aiState.quickTitle; },
    set quickTitle(v) { aiState.quickTitle = v; },
    get quickResult() { return aiState.quickResult; },
    set quickResult(v) { aiState.quickResult = v; },
    get lastRagHits() { return aiState.lastRagHits; },
    set lastRagHits(v) { aiState.lastRagHits = v; },
    get modeId() { return aiCtx.mode.id; },
    set modeId(_v) { /* read-only — saveNote never writes mode */ },
    get modeLabel() { return aiCtx.mode.label; },
    set modeLabel(_v) { /* read-only — derived from mode */ },
    get rag() { return aiCtx.rag; },
    set rag(v) { aiCtx.setRag(v); },
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
  // sessionStorage on aiState is the in-flight buffer (cleared on tab
  // close); this layer survives tab close + browser restart. Threads
  // get auto-saved on every full user→assistant exchange so the user
  // doesn't have to remember to "save". A "new thread" button stashes
  // the current state and starts fresh; the picker below restores any
  // past thread.
  let historyRailRef: ChatHistoryRail | undefined = $state();

  // ChatHistoryRefs needs one field beyond aiState's surface — modeId
  // (read aiCtx, write goes through aiCtx.restoreMode so the persist
  // + RAG-reset side-effects fire). Every other slot delegates to
  // aiState; modeId is the only composite getter.
  const historyRefs: ChatHistoryRefs = {
    get messages() { return aiState.messages; },
    set messages(v) { aiState.messages = v; },
    get activeThreadId() { return aiState.activeThreadId; },
    set activeThreadId(v) { aiState.activeThreadId = v; },
    get input() { return aiState.input; },
    set input(v) { aiState.input = v; },
    get pinnedIndex() { return aiState.pinnedIndex; },
    set pinnedIndex(v) { aiState.pinnedIndex = v; },
    get perTurnRagHits() { return aiState.perTurnRagHits; },
    set perTurnRagHits(v) { aiState.perTurnRagHits = v; },
    get expandedSources() { return aiState.expandedSources; },
    set expandedSources(v) { aiState.expandedSources = v; },
    get quickTitle() { return aiState.quickTitle; },
    set quickTitle(v) { aiState.quickTitle = v; },
    get quickResult() { return aiState.quickResult; },
    set quickResult(v) { aiState.quickResult = v; },
    get lastRagHits() { return aiState.lastRagHits; },
    set lastRagHits(v) { aiState.lastRagHits = v; },
    get historyOpen() { return aiState.historyOpen; },
    set historyOpen(v) { aiState.historyOpen = v; },
    get editingUserIdx() { return aiState.editingUserIdx; },
    set editingUserIdx(v) { aiState.editingUserIdx = v; },
    get editingUserDraft() { return aiState.editingUserDraft; },
    set editingUserDraft(v) { aiState.editingUserDraft = v; },
    get modeId() { return aiCtx.modeId; },
    set modeId(v) { aiCtx.restoreMode(v); }
  };
  const history = createChatHistoryManager({
    refs: historyRefs,
    getHistoryRail: () => historyRailRef,
    isBusy: () => aiState.busy,
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
  // RAG attribution state — lastRagHits, perTurnRagHits, expandedSources
  // — lives in aiState (used by chat session + save-note + history
  // manager). Snapshot loader owns its own state via snapshotLoader.

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
  // Page-context derivations — the route-parsing logic lives in
  // pageContext.ts as pure functions; we just wire $page through
  // them so the chip / mode auto-switch / page-agent button stay
  // reactive as the user navigates while the overlay is open.
  const currentNotePath = $derived(deriveCurrentNotePath($page.url.pathname));
  const currentProjectName = $derived(deriveCurrentProjectName($page.url.pathname, $page.url.searchParams));
  const currentGoalId = $derived(deriveCurrentGoalId($page.url.pathname, $page.url.searchParams));
  const onCalendarPage = $derived(deriveOnCalendarPage($page.url.pathname));
  const pageAgent = $derived(derivePageAgent($page.url.pathname, currentProjectName));
  function launchPageAgent() {
    if (!pageAgent) return;
    void goto(buildPageAgentTarget(pageAgent.path, $page.url.searchParams), { keepFocus: true });
    close();
  }

  function close() {
    // Abort any in-flight chat stream via the session manager's
    // controller. cancelInflight() is a no-op when nothing's running.
    cancelInflight();
    // Quick-action fetches are an independent abort lifecycle from
    // chat streams. Without this, hitting Esc on a slow Briefing
    // call left the fetch running; on completion it flipped busy=false
    // and wrote quickResult, which then resurfaced when the user
    // reopened the panel — a ghost result they explicitly cancelled.
    cancelQuickAction();
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
        } else if (attachSnapshot && !snapshotLoader.snapshotData) {
          void snapshotLoader.load();
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
          aiState.input = seed.text;
          if (seed.send) {
            tick().then(() => { void send(); });
          }
        }
      });
      void statusLoader.load();
      refreshPinnedIndex();
      tick().then(() => inputEl?.focus());
    }
  });

  // Mod+J / Esc / Mod+1..9 — wired through installOverlayShortcuts
  // so the listener attach/detach + textarea-guard live in one
  // tested module. The layered Esc dismissal stays here because it
  // owns the picker / history state.
  onMount(() =>
    installOverlayShortcuts({
      isOpen: () => open,
      toggle,
      selectMode: (id) => aiCtx.selectMode(id),
      onEscape: () => {
        // Order: pickers (mention, slash, mode) → history slide-
        // over → the overlay itself. So a single Esc only ever
        // unwinds one layer.
        if (mentionPickerOpen) mentionPickerOpen = false;
        else if (slashPickerOpen) slashPickerOpen = false;
        else if (modePickerOpen) modePickerOpen = false;
        else if (aiState.historyOpen) {
          aiState.historyOpen = false;
          refocusComposer();
        } else close();
      }
    })
  );

  // loadStatus moves into createAIStatusLoader — call
  // statusLoader.load() instead.

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
  // (the onEscape callback wired into installOverlayShortcuts above
  // unwinds picker → history → overlay in order) and so onInputKey
  // can chain mention → slash →
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
        aiState.messages = [
          ...aiState.messages,
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
        aiState.messages = [
          ...aiState.messages,
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
        aiState.mentionedRefs = [];
        toast.success('Context detached for the next message.');
        announce('Context detached for next message');
      },
      usageError: (msg) => toast.info(msg),
      refocusComposer
    });
    if (handled) aiState.input = '';
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
  // mentionedRefs lives in aiState — shared with the chat session
  // manager (consumed by send() as a strict system message, then
  // cleared so a follow-up doesn't repeat them).

  function removeMention(idx: number) {
    aiState.mentionedRefs = aiState.mentionedRefs.filter((_, i) => i !== idx);
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
    getInput: () => aiState.input,
    setInput: (next) => { aiState.input = next; }
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
  // Path → mode auto-suggestion lives in
  // $lib/chat/aiOverlayContextDefaults — the installer owns its
  // applied-this-open gate + reset-on-close, so the parent just
  // wires the four getters/setter.
  installAIContextDefaults({
    isOpen: () => $aiOverlayOpen,
    getPathname: () => $page.url.pathname,
    isOnGeneralMode: () => aiCtx.mode.id === 'general',
    selectMode: (id) => aiCtx.selectMode(id)
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
  // ChatSessionRefs is a pure subset of aiState's surface — pass it
  // directly. TS accepts the wider object.
  const sessionRefs: ChatSessionRefs = aiState;

  async function buildPreludeForSession(
    query: string,
    isFirstTurn: boolean
  ): Promise<PreludeBundle> {
    const { messages: preludeMessages, ragHits } = await buildPrelude({
      mode: aiCtx.mode,
      aiMemoryFacts: aiCtx.aiMemoryFacts,
      mentionedRefs: aiState.mentionedRefs,
      currentNotePath,
      currentProjectName,
      currentGoalId,
      onCalendarPage,
      attachSnapshot,
      snapshotData: snapshotLoader.snapshotData,
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
      aiState.expandedSources = {};
      aiState.pinnedIndex = {};
      aiState.activeThreadId = '';
      persistActiveThreadId('');
    },
    getScrollEl: () => scrollEl
  });
  const { send, sendFollowup, cancelInflight, clearChat } = chat;

  $effect(() => {
    void aiState.messages.length;
    void aiState.quickResult;
    tick().then(() => {
      if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
    });
  });

  // Composer handlers + auto-grow live in <ChatComposer />. Parent
  // owns `input`, `inputEl`, picker open/refs, and `voice` because
  // surfaces outside the composer (Esc handlers, starter-button
  // strip, send() prelude builder) need to read or mutate them.

  // Mode quick-switch (Mod+1..9) lives inside installOverlayShortcuts
  // above — the previous in-line $effect duplicated the listener and
  // shadowed the global onKey symbol, which the latest audit flagged
  // as a foot-gun for future edits.
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
    <AIOverlayHeader
      {aiCtx}
      statusInfo={statusLoader.statusInfo}
      {busy}
      bind:modePickerOpen
      bind:historyOpen={aiState.historyOpen}
      pinned={$aiOverlayPinned}
      {currentProjectName}
      {currentGoalId}
      {onCalendarPage}
      onCancelInflight={cancelInflight}
      onStartNewThread={startNewThread}
      onTogglePinned={toggleAIOverlayPinned}
      onClose={close}
    />

    {#if statusLoader.statusInfo?.sabbath || $sabbath}
      <div class="mx-4 mt-3 px-3 py-2 text-[11px] bg-warning text-on-primary rounded inline-flex items-center gap-2">
        <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M12 3v3"/>
          <path d="M9 9c0-2 1.5-3 3-3s3 1 3 3v2H9z"/>
          <rect x="8" y="11" width="8" height="9" rx="1"/>
        </svg>
        <span>Sabbath mode — AI requests are paused today.</span>
      </div>
    {/if}

    <AIOverlayQuickActions
      {pageAgent}
      {currentProjectName}
      {currentGoalId}
      {onCalendarPage}
      {busy}
      sabbathActive={$sabbath}
      hasContent={messages.length > 0 || !!quickResult}
      {saving}
      onLaunchAgent={launchPageAgent}
      onPickChip={(p) => { aiState.input = p; }}
      onBriefing={runBriefing}
      onSynopsis={runSynopsis}
      onTriage={runTriage}
      onDeadlines={runDeadlines}
      onSaveThread={() => void saveThreadAsNote()}
      onClear={clearChat}
    />

    <ChatHistoryRail
      bind:this={historyRailRef}
      bind:open={aiState.historyOpen}
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
          bind:expandedSources={aiState.expandedSources}
          {committedActions}
          {savingLibraryIdx}
          bind:savingLibraryLabel={aiState.savingLibraryLabel}
          {savingLibraryBusy}
          {editingUserIdx}
          bind:editingUserDraft={aiState.editingUserDraft}
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
        <AIOverlayEmptyBody
          {voiceSupported}
          onSendHelp={() => { aiState.input = '/help'; void send(); }}
          onStartMention={() => { aiState.input = '@'; refocusComposer(); mentionPickerRef?.detectTrigger(); }}
          onToggleVoice={toggleVoice}
        />
      {/if}
    </div>

    <AIOverlayRagStrip hits={lastRagHits} />

    <AIOverlayContextChips
      {currentNotePath}
      bind:attachNote
      bind:attachSnapshot
      snapshotLoading={snapshotLoader.snapshotLoading}
      snapshotData={snapshotLoader.snapshotData}
      rag={aiCtx.rag}
      onSetRag={aiCtx.setRag}
      onLoadSnapshot={() => void snapshotLoader.load()}
    />

    <AIOverlayMentionedRefs refs={mentionedRefs} onRemove={removeMention} />

    {#if !busy && !$sabbath && messages.length === 0 && input.trim().length === 0}
      <!-- Visibility gate stays in the parent — we only want to
           render the recents strip on a fresh thread with an empty
           composer so chips don't drift in/out mid-conversation. -->
      <AIOverlayCrossSourceRecents
        prompts={crossRecentInlinePrompts}
        onPick={(p) => { aiState.input = p; inputEl?.focus(); }}
      />
    {/if}

    <AIOverlayComposerStrip
      {aiCtx}
      bind:modePickerOpen
      {busy}
      sabbathActive={$sabbath}
      onBriefing={() => { void runBriefing(); }}
      onTriage={() => { void runTriage(); }}
      onDeadlines={() => { void runDeadlines(); }}
      onStartNewThread={startNewThread}
    />

    <ChatComposer
      bind:input={aiState.input}
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
      onMentionPick={(ref) => { aiState.mentionedRefs = [...aiState.mentionedRefs, ref]; }}
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
