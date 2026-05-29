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
  import { PANEL_WIDTH_MIN, PANEL_WIDTH_MAX } from './ai-overlay-geometry';
  import { createOverlayChrome } from './aioverlay/overlayChrome.svelte';
  import { rafThrottle } from '$lib/util/streamThrottle';
  import { isMobile } from '$lib/util/breakpoint';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import {
    AGENT_MODES,
    PERSONAS,
    findMode,
    loadModeId,
    persistModeId
  } from '$lib/ai/agents';
  import { buildPrelude } from '$lib/chat/prelude';
  import { commitParsedAction } from '$lib/chat/commitAction';
  import {
    buildSaveThreadPayload,
    buildAssistantNotePayload,
    buildAssistantNoteRetryPath
  } from '$lib/chat/saveToNote';
  import {
    QUICK_ACTION_TITLES,
    renderTriageProposals,
    renderDeadlineProposals
  } from '$lib/chat/quickActions';
  import { suggestedModeForPath } from '$lib/chat/contextDefaults';
  import {
    projectContextChips,
    CALENDAR_CONTEXT_CHIPS,
    GOAL_CONTEXT_CHIPS
  } from '$lib/chat/contextChips';
  import { deriveDraftTitle } from '$lib/ai/draftTitle';
  import {
    loadActiveThreadId,
    persistActiveThreadId,
    deriveLibraryLabel
  } from '$lib/chat/history';
  import {
    createChatHistoryManager,
    type ChatHistoryRefs
  } from '$lib/chat/chatHistoryManager.svelte';
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
    get modeId() { return modeId; },
    set modeId(v) { modeId = v; },
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
    // Remember the last persona the user actively chose so the
    // inline persona chip can keep showing it even when the mode
    // is currently posture-only (Coach, Research, ...).
    if (m.kind === 'persona') lastPersonaId = id;
    announce(`Mode: ${m.label}. ${m.tagline}`);
  }
  // Persistent "last persona" so the inline persona chip above the
  // composer keeps showing a meaningful label even when the active
  // mode is a generic posture. Seeded from the loaded mode if it's
  // already a persona; otherwise the first persona in PERSONAS.
  let lastPersonaId = $state<string>(
    findMode(loadModeId()).kind === 'persona'
      ? loadModeId()
      : (PERSONAS[0]?.id ?? '')
  );
  let lastPersona = $derived(lastPersonaId ? findMode(lastPersonaId) : null);
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
    await runQuick(QUICK_ACTION_TITLES.briefing, async (s) => {
      const r = await api.aiDailyBriefing(s);
      return r.markdown;
    });
  }
  async function runSynopsis() {
    await runQuick(QUICK_ACTION_TITLES.synopsis, async (s) => {
      const r = await api.aiWeeklyReview(s);
      return r.markdown;
    });
  }
  async function runTriage() {
    await runQuick(QUICK_ACTION_TITLES.triage, async (s) => {
      const r = await api.aiInboxTriage(s);
      return renderTriageProposals(r.proposals ?? []);
    });
  }
  async function runDeadlines() {
    await runQuick(QUICK_ACTION_TITLES.deadlines, async (s) => {
      const r = await api.aiDeadlineDetect(s);
      return renderDeadlineProposals(r.proposals ?? []);
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
          await loadAIMemory();
          toast.success(`Remembered · ${f.content.slice(0, 60)}`);
        } catch (err) {
          toast.error('Memory add failed: ' + errorMessage(err));
        }
      },
      showMemory: async (userText) => {
        await loadAIMemory();
        messages = [
          ...messages,
          { role: 'user', content: userText },
          { role: 'assistant', content: formatMemoryAsAssistantContent(aiMemoryFacts) }
        ];
      },
      forgetFact: async (idPrefix) => {
        await loadAIMemory();
        const match = aiMemoryFacts.find((f) => f.id.toLowerCase().startsWith(idPrefix.toLowerCase()));
        if (!match) {
          toast.error(`No fact id starts with "${idPrefix}"`);
          return;
        }
        try {
          await api.deleteAIMemory(match.id);
          await loadAIMemory();
          toast.success(`Forgot · ${match.content.slice(0, 50)}`);
        } catch (err) {
          toast.error('Forget failed: ' + errorMessage(err));
        }
      },
      selectModeAndToast: (m) => {
        selectMode(m.id);
        toast.success(`${m.glyph} ${m.label} — ${m.tagline}`);
      },
      unknownModeOrPersona: (kind, arg) => {
        toast.error(`Unknown ${kind}: ${arg}`);
      },
      toggleRag: () => {
        rag = !rag;
        toast.success(`RAG ${rag ? 'on' : 'off'} for the next turn.`);
        announce(`RAG ${rag ? 'enabled' : 'disabled'}`);
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

  // ── Save thread as note ────────────────────────────────────────
  // Persists the current overlay conversation as a markdown note
  // under chat-history/YYYY-MM-DD-HHmm-<slug>.md. Useful when a
  // chat lands on a real insight worth keeping; the dedicated
  // /chat page is for long-running threads, this is the quick
  // 'this was a good answer, save it' move from any page. The
  // payload assembly (path stem, frontmatter, body lines) lives
  // in $lib/chat/saveToNote; the createNote call + toasts stay
  // here so the loading state + error surfaces are local.
  let saving = $state(false);
  async function saveThreadAsNote() {
    if (saving) return;
    const payload = buildSaveThreadPayload({
      messages,
      quickTitle,
      quickResult,
      modeId: mode.id,
      modeLabel: mode.label,
      rag,
      lastRagHits,
      now: new Date()
    });
    if (!payload) {
      toast.info('Nothing to save yet.');
      return;
    }
    saving = true;
    try {
      await api.createNote(payload);
      toast.success('Saved · ' + payload.path);
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
    const { basePath, folder, baseSlug, frontmatter } = buildAssistantNotePayload({
      cleanedContent: cleaned,
      title,
      modeId: mode.id,
      currentProjectName,
      currentGoalId,
      onCalendarPage
    });
    try {
      let finalPath = basePath;
      try {
        await api.createNote({ path: basePath, frontmatter, body: cleaned });
      } catch (err) {
        // 409 Conflict — file exists. Retry with a time suffix.
        // Any other error rethrows to the outer toast handler.
        const msg = errorMessage(err);
        if (!/already exists|409/i.test(msg)) throw err;
        finalPath = buildAssistantNoteRetryPath(folder, baseSlug, new Date());
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
      onMemoryAdded: loadAIMemory
    });
    if (ok) committedActions = { ...committedActions, [key]: true };
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
    // Path → mode mapping lives in $lib/chat/contextDefaults; we only
    // override the user's pick when they're parked on the default
    // 'general' mode so a deliberate choice never gets clobbered.
    const suggested = suggestedModeForPath($page.url.pathname);
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
    // Build the prelude — system messages prepended to every turn.
    // Posture every turn; capabilities + memory + page context +
    // snapshot first turn only; RAG every turn the toggle is on.
    // All the policy + loader composition lives in $lib/chat/prelude.
    const isFirstTurn = messages.length === 0;
    const { messages: prelude, ragHits } = await buildPrelude({
      mode,
      aiMemoryFacts,
      mentionedRefs,
      currentNotePath,
      currentProjectName,
      currentGoalId,
      onCalendarPage,
      attachSnapshot,
      snapshotData,
      rag,
      isFirstTurn,
      query: text,
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
    lastRagHits = ragHits;
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
          <ChatModePicker
            {modeId}
            {currentProjectName}
            {currentGoalId}
            {onCalendarPage}
            onSelect={selectMode}
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
        title="Active mode — click to change ({mode.tagline})"
        aria-haspopup="listbox"
        aria-expanded={modePickerOpen}
      >
        <span class="{mode.kind === 'persona' ? 'text-secondary' : mode.kind === 'contextual' ? 'text-primary' : 'text-subtext'}">●</span>
        <span class="truncate">{mode.label}</span>
      </button>
      {#if lastPersona}
        <button
          type="button"
          onclick={() => (modePickerOpen = !modePickerOpen)}
          class="inline-flex items-center gap-1 text-[11px] px-1.5 py-0.5 rounded transition-colors max-w-[10rem] {modeId === lastPersona.id ? 'bg-secondary text-on-primary' : 'text-dim hover:text-text bg-surface0 hover:bg-surface1'}"
          title="Persona — {modeId === lastPersona.id ? 'active' : 'last used'}. Click to open picker."
        >
          <span class="text-[9px] font-mono">{lastPersona.glyph}</span>
          <span class="truncate">{lastPersona.label}</span>
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
