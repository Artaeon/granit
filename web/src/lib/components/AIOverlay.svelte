<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { fly, fade } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { page } from '$app/stores';
  import { api, type ChatMessage } from '$lib/api';
  import { sabbath } from '$lib/stores/sabbath';
  import { aiOverlayOpen, takeAIOverlaySeed } from '$lib/stores/ai-overlay';
  import { toast } from '$lib/components/toast';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import {
    AGENT_MODES,
    GENERIC_MODES,
    PERSONAS,
    findMode,
    loadModeId,
    persistModeId
  } from '$lib/ai/agents';
  import {
    listThreads,
    getThread,
    upsertThread,
    deleteThread,
    searchThreads,
    listPinned,
    togglePin,
    isPinned,
    deriveThreadTitle,
    type ChatThread,
    type PinnedMessage,
    type ThreadSearchHit
  } from '$lib/chat/history';
  import {
    loadRagIndex,
    retrieveForRag,
    getRagIndex,
    isRagIndexLoaded,
    type RagHit
  } from '$lib/chat/rag';
  import SlashCommandPicker from '$lib/components/SlashCommandPicker.svelte';

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
  const open = $derived($aiOverlayOpen);
  let panelEl: HTMLDivElement | undefined = $state();
  let inputEl: HTMLTextAreaElement | undefined = $state();
  let scrollEl: HTMLDivElement | undefined = $state();

  // ── Resizable panel (desktop only) ──────────────────────────────
  // The default 420px panel is comfortable for short chats but tight
  // when the history rail is open or the user is reading a long
  // assistant reply with a code block. Users drag the left edge to
  // widen the panel; the chosen width persists in localStorage so a
  // single tweak sticks across sessions. Mobile (bottom-sheet) ignores
  // this — width is always 100vw there.
  const PANEL_WIDTH_KEY = 'granit.chat.overlay.width';
  const PANEL_WIDTH_MIN = 360;
  const PANEL_WIDTH_MAX = 720;
  const PANEL_WIDTH_DEFAULT = 420;
  function loadPanelWidth(): number {
    if (typeof localStorage === 'undefined') return PANEL_WIDTH_DEFAULT;
    try {
      const raw = localStorage.getItem(PANEL_WIDTH_KEY);
      const n = raw ? parseInt(raw, 10) : NaN;
      if (!Number.isFinite(n)) return PANEL_WIDTH_DEFAULT;
      return Math.min(PANEL_WIDTH_MAX, Math.max(PANEL_WIDTH_MIN, n));
    } catch {
      return PANEL_WIDTH_DEFAULT;
    }
  }
  let panelWidth = $state<number>(loadPanelWidth());
  let resizing = $state(false);
  function persistPanelWidth(n: number) {
    if (typeof localStorage === 'undefined') return;
    try { localStorage.setItem(PANEL_WIDTH_KEY, String(n)); } catch {}
  }
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
      // increases (window.innerWidth - clientX). Clamp to min/max.
      const w = window.innerWidth - ev.clientX;
      panelWidth = Math.min(PANEL_WIDTH_MAX, Math.max(PANEL_WIDTH_MIN, w));
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
    // Keyboard fallback for accessibility — left/right arrow nudges
    // the panel by 16px steps. Home/End jumps to min/max.
    let next = panelWidth;
    if (e.key === 'ArrowLeft') next = Math.min(PANEL_WIDTH_MAX, panelWidth + 16);
    else if (e.key === 'ArrowRight') next = Math.max(PANEL_WIDTH_MIN, panelWidth - 16);
    else if (e.key === 'Home') next = PANEL_WIDTH_MAX;
    else if (e.key === 'End') next = PANEL_WIDTH_MIN;
    else return;
    e.preventDefault();
    panelWidth = next;
    persistPanelWidth(next);
  }

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

  // ── Long-term thread history (localStorage, LRU 30) ──────────────
  // sessionStorage above is the in-flight buffer (cleared on tab
  // close); this layer survives tab close + browser restart. Threads
  // get auto-saved on every full user→assistant exchange so the user
  // doesn't have to remember to "save". A "new thread" button stashes
  // the current state and starts fresh; the picker below restores
  // any past thread.
  const ACTIVE_THREAD_KEY = 'granit.chat.overlay.activeId';
  function loadActiveThreadId(): string {
    if (typeof sessionStorage === 'undefined') return '';
    try { return sessionStorage.getItem(ACTIVE_THREAD_KEY) ?? ''; } catch { return ''; }
  }
  function persistActiveThreadId(id: string) {
    if (typeof sessionStorage === 'undefined') return;
    try {
      if (id) sessionStorage.setItem(ACTIVE_THREAD_KEY, id);
      else sessionStorage.removeItem(ACTIVE_THREAD_KEY);
    } catch {}
  }
  let activeThreadId = $state<string>(loadActiveThreadId());
  let historyOpen = $state(false);
  let historyTab = $state<'threads' | 'pinned'>('threads');
  let savedThreads = $state<ChatThread[]>([]);
  let pinnedItems = $state<PinnedMessage[]>([]);
  let historySearch = $state('');
  let historyHits = $state<ThreadSearchHit[]>([]);
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

  function refreshHistoryLists() {
    savedThreads = listThreads();
    pinnedItems = listPinned();
  }

  $effect(() => {
    if (historyOpen) refreshHistoryLists();
  });

  $effect(() => {
    void historySearch;
    if (!historySearch.trim()) {
      historyHits = [];
      return;
    }
    historyHits = searchThreads(historySearch);
  });

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
      refreshHistoryLists();
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
    refreshHistoryLists();
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
    const newTitle = (sourceTitle.length > 60 ? sourceTitle.slice(0, 60) : sourceTitle) + ' (branch)';
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
    if (historyOpen) refreshHistoryLists();
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
    if (historyOpen && historyTab === 'pinned') refreshHistoryLists();
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
  // Persist mode change + reset RAG default when user picks a new
  // mode, but DON'T reset on every render (that would clobber the
  // user's explicit override). Only seed when the user actively
  // changes mode.
  function selectMode(id: string) {
    if (id === modeId) return;
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

  function close() {
    abort?.abort();
    if (recording) stopVoice();
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
        const msg = err instanceof Error ? err.message : String(err);
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
  // Type "@" in the composer and a small popup lists tasks, goals,
  // projects, deadlines, events, and notes. Selecting one stamps the
  // input with @<title> for the user's eyes, and stashes a structured
  // reference object that gets folded into the next send() as a
  // strict system message ("the user is referencing Task Txx: ...
  // due 2026-05-12, priority P1"). This is cleaner than splicing
  // raw entity bodies into the user message — it lets the model
  // ground its reply on real fields instead of the user's prose
  // glossing them.
  type MentionKind = 'task' | 'goal' | 'project' | 'deadline' | 'event' | 'note';
  interface MentionRef {
    kind: MentionKind;
    /** Stable id (task id, goal id, deadline id, project name, note path...). */
    id: string;
    /** Display title. */
    title: string;
    /** Pre-formatted system-prompt fragment describing the entity's
     *  key fields. Built at pick time so we don't need a second fetch
     *  on send. */
    contextLine: string;
  }
  type MentionCandidate = {
    kind: MentionKind;
    id: string;
    title: string;
    subtitle: string;
    contextLine: string;
  };
  let mentionPickerOpen = $state(false);
  let mentionQuery = $state('');
  // Anchor: where in input the @ sits (start) — replaced on pick.
  let mentionAnchor = $state(-1);
  let mentionCandidates = $state<MentionCandidate[]>([]);
  let mentionLoading = $state(false);
  let mentionSelectedIdx = $state(0);
  // The list of attached entity refs for the next outgoing message.
  // Cleared after each send. The user can also clear it manually
  // from the chip strip below the composer.
  let mentionedRefs = $state<MentionRef[]>([]);
  // Cached entity index — populated lazily on first @-mention. Same
  // shape as ragIndex (small enough to hold full list per type).
  let mentionIndex = $state<{
    tasks: { id: string; text: string; priority: number; dueDate?: string; done: boolean }[];
    goals: { id: string; title: string; status?: string; target_date?: string }[];
    projects: { name: string; description?: string; status?: string }[];
    deadlines: { id: string; title: string; date: string; importance: string }[];
    events: { id: string; title: string; date: string; start_time?: string }[];
  }>({ tasks: [], goals: [], projects: [], deadlines: [], events: [] });
  let mentionIndexLoaded = $state(false);

  async function loadMentionIndex() {
    if (mentionIndexLoaded) return;
    mentionLoading = true;
    try {
      // Parallel fetch — each endpoint is small. Failures fall through
      // (the user still gets the working subset). Notes piggy-back on
      // the RAG index so we don't double-fetch them.
      const [tasks, goals, projects, deadlines, events] = await Promise.all([
        api.listTasks({ status: 'open' }).catch(() => ({ tasks: [], total: 0 })),
        api.listGoals().catch(() => ({ goals: [], total: 0 })),
        api.listProjects().catch(() => ({ projects: [], total: 0 })),
        api.listDeadlines().catch(() => ({ deadlines: [], total: 0 })),
        api.listEvents().catch(() => ({ events: [], total: 0 }))
      ]);
      // Pre-warm the note index for @-mention note matches. Fire-and-
      // forget — note results show up once the index lands.
      void loadRagIndex();
      mentionIndex = {
        tasks: tasks.tasks.map((t) => ({
          id: t.id,
          text: t.text,
          priority: t.priority,
          dueDate: t.dueDate,
          done: t.done
        })),
        goals: goals.goals.map((g) => ({
          id: g.id,
          title: g.title,
          status: g.status,
          target_date: g.target_date
        })),
        projects: projects.projects.map((p) => ({
          name: p.name,
          description: p.description,
          status: p.status
        })),
        deadlines: deadlines.deadlines.map((d) => ({
          id: d.id,
          title: d.title,
          date: d.date,
          importance: d.importance
        })),
        events: events.events.map((e) => ({
          id: e.id,
          title: e.title,
          date: e.date,
          start_time: e.start_time
        }))
      };
    } finally {
      mentionIndexLoaded = true;
      mentionLoading = false;
    }
  }

  // Score a candidate against the user's typed query. Substring match
  // on title/text wins over prefix; everything is lowercase. Empty
  // query returns the most recent / highest-priority entries per type.
  function buildMentionCandidates(q: string): MentionCandidate[] {
    const ql = q.trim().toLowerCase();
    const out: MentionCandidate[] = [];
    // Tasks
    for (const t of mentionIndex.tasks) {
      const text = t.text.toLowerCase();
      if (ql && !text.includes(ql)) continue;
      const due = t.dueDate ? ` · due ${t.dueDate}` : '';
      const prio = t.priority > 0 ? `P${t.priority}` : '';
      out.push({
        kind: 'task',
        id: t.id,
        title: t.text,
        subtitle: `${prio}${due || ' · no due'}`.trim(),
        contextLine:
          `Task ${t.id}: ${t.text}` +
          (t.dueDate ? ` (due ${t.dueDate})` : '') +
          (t.priority > 0 ? ` (priority P${t.priority})` : '') +
          (t.done ? ' [done]' : '')
      });
    }
    // Goals
    for (const g of mentionIndex.goals) {
      if (ql && !g.title.toLowerCase().includes(ql)) continue;
      out.push({
        kind: 'goal',
        id: g.id,
        title: g.title,
        subtitle: `${g.status ?? 'active'}${g.target_date ? ' · ' + g.target_date : ''}`,
        contextLine:
          `Goal ${g.id}: ${g.title}` +
          (g.target_date ? ` (target ${g.target_date})` : '') +
          (g.status ? ` [status: ${g.status}]` : '')
      });
    }
    // Projects
    for (const p of mentionIndex.projects) {
      if (ql && !p.name.toLowerCase().includes(ql)) continue;
      out.push({
        kind: 'project',
        id: p.name,
        title: p.name,
        subtitle: p.status || 'project',
        contextLine:
          `Project "${p.name}"` +
          (p.description ? ` — ${p.description.slice(0, 200)}` : '')
      });
    }
    // Deadlines
    for (const d of mentionIndex.deadlines) {
      if (ql && !d.title.toLowerCase().includes(ql)) continue;
      out.push({
        kind: 'deadline',
        id: d.id,
        title: d.title,
        subtitle: `${d.date} · ${d.importance}`,
        contextLine: `Deadline "${d.title}" on ${d.date} (importance: ${d.importance})`
      });
    }
    // Events
    for (const e of mentionIndex.events) {
      if (ql && !e.title.toLowerCase().includes(ql)) continue;
      const when = e.start_time ? `${e.date} ${e.start_time}` : e.date;
      out.push({
        kind: 'event',
        id: e.id,
        title: e.title,
        subtitle: when,
        contextLine: `Event "${e.title}" on ${when}`
      });
    }
    // Notes — reuse the RAG index. Cheap subset; we only show the top
    // 8 by title match so the picker isn't dominated by 5k notes.
    if (isRagIndexLoaded()) {
      for (const n of getRagIndex()) {
        if (ql && !n.title.toLowerCase().includes(ql)) continue;
        out.push({
          kind: 'note',
          id: n.path,
          title: n.title,
          subtitle: n.path,
          // Note context is a back-pointer. Body injection is handled
          // separately via attachNote / RAG; this just tells the
          // model "the user is asking about this specific note".
          contextLine: `Note "${n.title}" at path \`${n.path}\``
        });
      }
    }
    // Cap total candidates so the picker stays scannable.
    const limit = 12;
    if (out.length <= limit) return out;
    // Prefer exact-prefix matches when query has content.
    if (ql) {
      out.sort((a, b) => {
        const ap = a.title.toLowerCase().startsWith(ql) ? 0 : 1;
        const bp = b.title.toLowerCase().startsWith(ql) ? 0 : 1;
        return ap - bp;
      });
    }
    return out.slice(0, limit);
  }

  function detectMentionTrigger() {
    if (!inputEl) return;
    const caret = inputEl.selectionStart ?? -1;
    if (caret < 0) {
      mentionPickerOpen = false;
      return;
    }
    // Walk back from caret to find a leading "@" with no whitespace
    // between it and the caret; bail if we hit whitespace first.
    let i = caret - 1;
    while (i >= 0) {
      const c = input[i];
      if (c === '@') {
        const prev = i > 0 ? input[i - 1] : ' ';
        if (prev === ' ' || prev === '\n' || prev === '\t' || i === 0) {
          mentionAnchor = i;
          mentionQuery = input.slice(i + 1, caret);
          if (!mentionPickerOpen) {
            mentionPickerOpen = true;
            mentionSelectedIdx = 0;
            void loadMentionIndex().then(() => {
              if (mentionPickerOpen) mentionCandidates = buildMentionCandidates(mentionQuery);
            });
          } else {
            mentionCandidates = buildMentionCandidates(mentionQuery);
            mentionSelectedIdx = 0;
          }
          return;
        }
        break;
      }
      if (c === ' ' || c === '\n' || c === '\t') break;
      i--;
    }
    mentionPickerOpen = false;
  }

  function pickMention(c: MentionCandidate) {
    if (mentionAnchor < 0) {
      mentionPickerOpen = false;
      return;
    }
    // Splice "@<query>" → "@<title> " in the input, and stash the ref.
    const before = input.slice(0, mentionAnchor);
    const after = input.slice((inputEl?.selectionStart ?? mentionAnchor) ?? mentionAnchor);
    const insert = `@${c.title} `;
    input = before + insert + after;
    const newCaret = before.length + insert.length;
    mentionedRefs = [...mentionedRefs, { kind: c.kind, id: c.id, title: c.title, contextLine: c.contextLine }];
    mentionPickerOpen = false;
    mentionAnchor = -1;
    mentionQuery = '';
    tick().then(() => {
      if (inputEl) {
        inputEl.focus();
        inputEl.setSelectionRange(newCaret, newCaret);
      }
    });
  }

  function removeMention(idx: number) {
    mentionedRefs = mentionedRefs.filter((_, i) => i !== idx);
  }

  function onMentionKey(e: KeyboardEvent): boolean {
    if (!mentionPickerOpen || mentionCandidates.length === 0) return false;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      mentionSelectedIdx = (mentionSelectedIdx + 1) % mentionCandidates.length;
      return true;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      mentionSelectedIdx = (mentionSelectedIdx - 1 + mentionCandidates.length) % mentionCandidates.length;
      return true;
    }
    if (e.key === 'Enter' || e.key === 'Tab') {
      const c = mentionCandidates[mentionSelectedIdx];
      if (c) {
        e.preventDefault();
        pickMention(c);
        return true;
      }
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      mentionPickerOpen = false;
      return true;
    }
    return false;
  }

  // ── Voice input ────────────────────────────────────────────────
  // Click the mic, the browser's SpeechRecognition fills the input
  // as you speak. Same Web Speech API used by the voice-note modal;
  // graceful fallback when unsupported (Firefox).
  type RecognitionCtor = new () => SpeechRecognition;
  interface SpeechRecognition extends EventTarget {
    continuous: boolean;
    interimResults: boolean;
    lang: string;
    onresult: ((this: SpeechRecognition, ev: SpeechRecognitionEvent) => unknown) | null;
    onerror: ((this: SpeechRecognition, ev: Event) => unknown) | null;
    onend: ((this: SpeechRecognition, ev: Event) => unknown) | null;
    start: () => void;
    stop: () => void;
    abort: () => void;
  }
  interface SpeechRecognitionEvent extends Event {
    resultIndex: number;
    results: {
      length: number;
      [i: number]: { isFinal: boolean; [j: number]: { transcript: string } };
    };
  }
  function getRecognitionCtor(): RecognitionCtor | null {
    if (typeof window === 'undefined') return null;
    const w = window as unknown as { SpeechRecognition?: RecognitionCtor; webkitSpeechRecognition?: RecognitionCtor };
    return w.SpeechRecognition ?? w.webkitSpeechRecognition ?? null;
  }
  let voiceSupported = $derived(typeof window !== 'undefined' && getRecognitionCtor() !== null);
  let recording = $state(false);
  let recognition: SpeechRecognition | null = null;
  let voiceBaseline = ''; // input value when recording started — finals append to this

  function startVoice() {
    const Ctor = getRecognitionCtor();
    if (!Ctor || recording) return;
    voiceBaseline = input.endsWith(' ') || input.length === 0 ? input : input + ' ';
    recognition = new Ctor();
    recognition.continuous = true;
    recognition.interimResults = true;
    recognition.lang = navigator.language || 'en-US';
    recognition.onresult = (ev) => {
      let interim = '';
      let final = '';
      for (let i = ev.resultIndex; i < ev.results.length; i++) {
        const res = ev.results[i];
        const text = res[0].transcript;
        if (res.isFinal) final += text + ' ';
        else interim += text;
      }
      if (final) voiceBaseline += final;
      input = (voiceBaseline + interim).replace(/\s+/g, ' ').trim();
    };
    recognition.onerror = () => {};
    recognition.onend = () => {
      // Chrome auto-ends on silence — restart while we're still
      // in recording mode so a long thought continues.
      if (recording && recognition) {
        try { recognition.start(); } catch {}
      }
    };
    try {
      recognition.start();
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
  function slugify(s: string): string {
    return s
      .toLowerCase()
      .replace(/[^\w\s-]/g, '')
      .replace(/\s+/g, '-')
      .slice(0, 60)
      .replace(/^-+|-+$/g, '');
  }
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
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      saving = false;
    }
  }

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
      await api.chatStream(
        history,
        attachNote && currentNotePath ? currentNotePath : undefined,
        {
          onChunk: (c) => {
            acc += c;
            // Reassign through map so $state picks up the change.
            messages = messages.map((m, i) => (i === idx ? { ...m, content: acc } : m));
          },
          onError: (err) => {
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
    if (onMentionKey(e)) return;
    if (slashPickerRef?.handleKey(e)) return;
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      void send();
    }
  }
  function onInputChange() {
    slashPickerRef?.detectTrigger();
    detectMentionTrigger();
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
    detectMentionTrigger();
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
  <button
    type="button"
    aria-label="close AI overlay"
    onclick={close}
    transition:fade={{ duration: 150 }}
    class="md:hidden fixed inset-0 z-40 bg-black/40 backdrop-blur-sm"
  ></button>

  <div
    bind:this={panelEl}
    data-ai-overlay
    role="dialog"
    aria-label="AI assistant"
    style:--ai-panel-w="{panelWidth}px"
    in:fly={{ duration: 200, easing: cubicOut, ...panelTransitionParams() }}
    out:fly={{ duration: 150, easing: cubicOut, ...panelTransitionParams() }}
    class="ai-overlay-panel fixed z-50 flex flex-col bg-base border-surface1 shadow-2xl
           inset-x-0 bottom-0 max-h-[85dvh] rounded-t-xl border-t pb-safe
           md:inset-y-0 md:right-0 md:left-auto md:bottom-auto md:top-0 md:h-full md:max-h-none md:rounded-none md:border-l md:border-t-0 md:pb-0 {resizing ? 'ai-overlay-resizing' : ''}"
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
      class="hidden md:block absolute left-0 top-0 bottom-0 w-1.5 -ml-0.5 z-50 cursor-col-resize group {resizing ? 'bg-primary/40' : 'hover:bg-primary/30 focus-visible:bg-primary/40'} transition-colors"
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
         top; both layouts get title + status pill + close. -->
    <div class="md:hidden flex justify-center pt-2 pb-1">
      <span class="block w-10 h-1 rounded-full bg-surface2"></span>
    </div>
    <header class="px-4 py-3 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
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
          title={`Mode: ${mode.label} — ${mode.tagline}`}
        >
          <span class="text-base leading-none">{mode.glyph}</span>
          <span class="text-sm font-semibold truncate max-w-[8rem] sm:max-w-none">{mode.label}</span>
          <svg viewBox="0 0 24 24" class="w-3 h-3 opacity-60 flex-shrink-0" fill="none" stroke="currentColor" stroke-width="2">
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
            <div class="px-3 pt-1 pb-1 text-[9px] uppercase tracking-widest text-dim">Modes</div>
            {#each GENERIC_MODES as m (m.id)}
              <button
                type="button"
                role="option"
                aria-selected={m.id === modeId}
                onclick={() => { selectMode(m.id); modePickerOpen = false; }}
                class="w-full flex items-start gap-2 px-3 py-2 hover:bg-surface0 text-left {m.id === modeId ? 'bg-primary/10' : ''}"
              >
                <span class="text-base leading-tight flex-shrink-0">{m.glyph}</span>
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium text-text">{m.label}</div>
                  <div class="text-[11px] text-dim leading-snug">{m.tagline}</div>
                </div>
                {#if m.id === modeId}
                  <span class="text-primary text-xs flex-shrink-0">✓</span>
                {/if}
              </button>
            {/each}
            {#if PERSONAS.length > 0}
              <!-- Personas group — sharper, named voices. Visually
                   distinguished by a divider, a section header, an
                   accent-coloured glyph background, and an italic
                   tagline so the user reads "this is a character,
                   not a generic posture" at a glance. -->
              <div class="border-t border-surface1 mt-1"></div>
              <div class="px-3 pt-2 pb-1 text-[9px] uppercase tracking-widest text-secondary">Personas</div>
              {#each PERSONAS as m (m.id)}
                <button
                  type="button"
                  role="option"
                  aria-selected={m.id === modeId}
                  onclick={() => { selectMode(m.id); modePickerOpen = false; }}
                  class="w-full flex items-start gap-2 px-3 py-2 hover:bg-surface0 text-left {m.id === modeId ? 'bg-primary/10' : ''}"
                >
                  <span class="text-base leading-tight flex-shrink-0 inline-flex items-center justify-center w-6 h-6 rounded-full bg-secondary/15 text-secondary">{m.glyph}</span>
                  <div class="flex-1 min-w-0">
                    <div class="text-sm font-medium text-text">{m.label}</div>
                    <div class="text-[11px] text-dim leading-snug italic">{m.tagline}</div>
                  </div>
                  {#if m.id === modeId}
                    <span class="text-primary text-xs flex-shrink-0">✓</span>
                  {/if}
                </button>
              {/each}
            {/if}
          </div>
        {/if}
      </div>
      {#if statusInfo && panelWidth >= 480}
        <!-- Status pill shows on desktop panels >=480px wide. On
             narrower panels the mode picker label + 3 right-side
             icons + close button already fill the row; the pill
             gets hidden so the layout doesn't compress the mode
             label into ellipsis. Mobile keeps it hidden too —
             the user opened the overlay to chat, not to read a
             provider name. -->
        <span
          class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-surface1 text-subtext truncate hidden md:inline-block max-w-[10rem]"
          title="Default backend (per-feature overrides apply individually)"
        >{statusInfo.provider} · {statusInfo.model}</span>
      {/if}
      <span class="flex-1"></span>
      {#if busy}
        <button
          onclick={cancelInflight}
          class="px-2 py-1 text-[11px] text-warning hover:underline"
          title="Cancel the in-flight request"
        >cancel</button>
      {/if}
      <!-- History toggle. The side rail beneath shows saved threads
           + pinned messages; auto-saves so the user never loses a
           good chat. New-thread button starts fresh while preserving
           the previous one in history. -->
      <button
        type="button"
        onclick={() => { historyOpen = !historyOpen; if (historyOpen) refreshHistoryLists(); }}
        aria-pressed={historyOpen}
        aria-label="Chat history"
        title="Chat history (saved threads + pinned messages)"
        class="tap-target inline-flex items-center justify-center px-1.5 py-1 rounded text-dim hover:text-text hover:bg-surface0 active:bg-surface1 transition-colors {historyOpen ? 'text-primary bg-primary/10' : ''}"
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
      <button
        onclick={close}
        aria-label="close"
        class="tap-target inline-flex items-center justify-center text-dim hover:text-text hover:bg-surface0 active:bg-surface1 rounded px-2 py-1 text-lg leading-none transition-colors"
      >×</button>
    </header>

    {#if statusInfo?.sabbath || $sabbath}
      <div class="mx-4 mt-3 px-3 py-2 text-[11px] bg-warning/10 border border-warning/30 rounded text-warning">
        🕯️ Sabbath mode — AI requests are paused today.
      </div>
    {/if}

    <!-- Quick actions row. Wraps on small viewports so it never
         pushes the body off-screen. -->
    <div class="px-4 py-3 border-b border-surface1 flex flex-wrap gap-1.5 flex-shrink-0">
      <button
        onclick={runBriefing}
        disabled={busy || $sabbath}
        class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary disabled:opacity-50"
      >Briefing</button>
      <button
        onclick={runSynopsis}
        disabled={busy || $sabbath}
        class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary disabled:opacity-50"
      >Weekly synopsis</button>
      <button
        onclick={runTriage}
        disabled={busy || $sabbath}
        class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary disabled:opacity-50"
      >Triage</button>
      <button
        onclick={runDeadlines}
        disabled={busy || $sabbath}
        class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary disabled:opacity-50"
      >Deadlines</button>
      <span class="flex-1"></span>
      {#if messages.length > 0 || quickResult}
        <button
          onclick={() => void saveThreadAsNote()}
          disabled={saving}
          class="px-2 py-1 text-[11px] text-secondary hover:underline disabled:opacity-50 inline-flex items-center gap-1"
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
          class="px-2 py-1 text-[11px] text-dim hover:text-error"
          title="Clear the overlay"
        >clear</button>
      {/if}
    </div>

    {#if historyOpen}
      <!-- History panel. Two tabs:
             - threads: chronological list of saved chats. Click to
               load (current thread is auto-saved before swapping).
             - pinned: flat list of starred assistant replies across
               all threads. Persists even if the parent thread was
               pruned by LRU.
           Layout split between mobile + desktop: on phones, history
           is a full-screen slide-over that COVERS the chat (one tap
           to a thread, no half-screen-of-chat-pushed-down nonsense);
           on desktop, it stays inline as a top strip beneath the
           toolbar so the chat below is still visible. The panel is
           positioned `absolute inset-0` on mobile within the dialog
           — z-30 sits above the chat body but below the header
           (z-50 on the resize handle) so the user can still see what
           thread they came from. -->
      <div
        class="ai-history-panel border-surface1 bg-mantle/95 backdrop-blur-sm flex flex-col
               absolute inset-0 z-30 border-t
               md:static md:bg-mantle/40 md:backdrop-blur-none md:border-b md:border-t-0 md:max-h-[40dvh]"
      >
        <div class="flex items-center gap-1 px-3 pt-3 pb-1 text-[11px] flex-shrink-0">
          <button
            type="button"
            onclick={() => (historyTab = 'threads')}
            class="tap-target px-2.5 py-1.5 rounded transition-colors {historyTab === 'threads' ? 'bg-surface1 text-text font-medium' : 'text-dim hover:text-text hover:bg-surface0'}"
          >Threads <span class="opacity-60">({savedThreads.length})</span></button>
          <button
            type="button"
            onclick={() => (historyTab = 'pinned')}
            class="tap-target px-2.5 py-1.5 rounded transition-colors {historyTab === 'pinned' ? 'bg-surface1 text-text font-medium' : 'text-dim hover:text-text hover:bg-surface0'}"
          >Pinned <span class="opacity-60">({pinnedItems.length})</span></button>
          <span class="flex-1"></span>
          <button
            type="button"
            onclick={() => (historyOpen = false)}
            class="tap-target inline-flex items-center justify-center text-dim hover:text-text hover:bg-surface0 active:bg-surface1 rounded px-2 py-1 text-base leading-none transition-colors"
            aria-label="Close history"
          >×</button>
        </div>
        <div class="flex-1 min-h-0 overflow-y-auto">
        {#if historyTab === 'threads'}
          <div class="px-3 pt-2 pb-1">
            <input
              type="text"
              bind:value={historySearch}
              placeholder="Search threads…"
              class="w-full bg-surface0 border border-surface1 rounded px-2 py-1 text-xs text-text placeholder-dim focus:outline-none focus:border-primary"
            />
          </div>
          <ul class="px-2 pb-2">
            {#if historySearch.trim()}
              {#if historyHits.length === 0}
                <li class="px-2 py-3 text-center text-[11px] text-dim italic">No matches.</li>
              {:else}
                {#each historyHits as hit (hit.thread.id)}
                  <li>
                    <button
                      type="button"
                      onclick={() => loadSavedThread(hit.thread.id)}
                      class="w-full text-left px-2 py-1.5 rounded hover:bg-surface0 group {hit.thread.id === activeThreadId ? 'bg-surface0' : ''}"
                    >
                      <div class="flex items-baseline gap-2">
                        <span class="text-xs text-text font-medium truncate flex-1">{hit.thread.title}</span>
                        <span class="text-[9px] text-dim flex-shrink-0">{findMode(hit.thread.modeId).glyph}</span>
                      </div>
                      <div class="text-[10px] text-dim leading-snug truncate mt-0.5">{hit.excerpt}</div>
                    </button>
                  </li>
                {/each}
              {/if}
            {:else if savedThreads.length === 0}
              <li class="px-2 py-3 text-center text-[11px] text-dim italic">No saved threads yet. Send a message to start one.</li>
            {:else}
              {#each savedThreads as t (t.id)}
                <li class="group flex items-stretch gap-1">
                  <button
                    type="button"
                    onclick={() => loadSavedThread(t.id)}
                    class="flex-1 min-w-0 text-left px-2 py-1.5 rounded hover:bg-surface0 {t.id === activeThreadId ? 'bg-surface0' : ''}"
                  >
                    <div class="flex items-baseline gap-2">
                      <span class="text-xs text-text font-medium truncate flex-1">{t.title}</span>
                      <span class="text-[9px] text-dim flex-shrink-0" title={findMode(t.modeId).label}>{findMode(t.modeId).glyph}</span>
                    </div>
                    <div class="text-[10px] text-dim mt-0.5 flex items-center gap-2">
                      <span>{new Date(t.updatedAt).toLocaleDateString()} {new Date(t.updatedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
                      <span>· {t.messages.filter((m) => m.role !== 'system').length} msgs</span>
                    </div>
                  </button>
                  <button
                    type="button"
                    onclick={() => { if (confirm('Delete this thread?')) deleteSavedThread(t.id); }}
                    class="px-1 text-dim hover:text-error opacity-0 group-hover:opacity-100 transition-opacity"
                    aria-label="Delete thread"
                    title="Delete thread"
                  >
                    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13" stroke-linecap="round" stroke-linejoin="round"/>
                    </svg>
                  </button>
                </li>
              {/each}
            {/if}
          </ul>
        {:else}
          <ul class="px-2 pt-2 pb-2">
            {#if pinnedItems.length === 0}
              <li class="px-2 py-3 text-center text-[11px] text-dim italic">No pinned replies yet. Click ☆ on any assistant message to keep it.</li>
            {:else}
              {#each pinnedItems as p (p.threadId + ':' + p.messageIndex + ':' + p.pinnedAt)}
                <li class="group px-2 py-1.5 rounded hover:bg-surface0">
                  <div class="flex items-baseline gap-2 mb-1">
                    <span class="text-[10px] text-dim flex-1 truncate">{p.threadTitle}</span>
                    <span class="text-[9px] text-dim flex-shrink-0" title={findMode(p.modeId).label}>{findMode(p.modeId).glyph}</span>
                    <button
                      type="button"
                      onclick={() => {
                        togglePin({ threadId: p.threadId, threadTitle: p.threadTitle, modeId: p.modeId, messageIndex: p.messageIndex, content: p.content });
                        refreshHistoryLists();
                        if (p.threadId === activeThreadId) refreshPinnedIndex();
                      }}
                      class="text-warning hover:text-error opacity-60 group-hover:opacity-100 transition-opacity"
                      title="Unpin"
                      aria-label="Unpin"
                    >
                      <svg viewBox="0 0 24 24" class="w-3 h-3" fill="currentColor"><polygon points="12 2 15 9 22 9 17 14 19 22 12 17 5 22 7 14 2 9 9 9"/></svg>
                    </button>
                  </div>
                  <div class="text-[11px] text-subtext leading-snug line-clamp-3">{p.content}</div>
                </li>
              {/each}
            {/if}
          </ul>
        {/if}
        </div>
      </div>
    {/if}

    <!-- Body — quick-action result OR chat thread. Mutually
         exclusive: firing a quick action clears the chat, sending
         a chat message clears the quick result. Keeps the overlay
         single-purpose at any moment. -->
    <div bind:this={scrollEl} class="flex-1 overflow-y-auto px-4 py-3">
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
                {#if m.role === 'assistant' && m.content && !busy}
                  <span class="ml-auto inline-flex items-center gap-1">
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
                  </span>
                {/if}
              </div>
              {#if m.role === 'user'}
                <div class="text-sm text-text whitespace-pre-wrap">{m.content}</div>
              {:else}
                <div class="prose prose-sm max-w-none">
                  <MarkdownRenderer body={m.content || '_…_'} />
                </div>
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
              onclick={() => { input = '@'; refocusComposer(); detectMentionTrigger(); }}
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
      <div class="border-t border-surface1 px-4 py-1.5 flex items-center gap-1.5 flex-wrap text-[11px] flex-shrink-0 bg-mantle/40">
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
          <span class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-secondary/10 text-secondary border border-secondary/30">
            <span class="text-[9px] uppercase tracking-wider opacity-70">{r.kind}</span>
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

    <!-- Chat input. Sits at the bottom, growable up to a few rows.
         Enter sends, Shift+Enter inserts a newline. Disabled
         during Sabbath since the request would just be refused. -->
    <form
      onsubmit={send}
      class="border-t border-surface1 px-4 py-3 flex items-end gap-2 flex-shrink-0"
    >
      <div class="flex-1 relative">
        <textarea
          bind:this={inputEl}
          bind:value={input}
          onkeydown={onInputKey}
          oninput={onInputChange}
          onclick={onInputClick}
          rows="2"
          placeholder={$sabbath ? 'Sabbath active — AI paused' : recording ? 'Listening… speak freely' : 'Ask anything, /help for commands, @ to reference an item'}
          disabled={busy || $sabbath}
          class="w-full bg-surface0 border border-surface1 rounded px-3 py-2 text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-none disabled:opacity-60 transition-colors {recording ? 'border-error' : ''}"
          style="min-height: 2.5rem;"
        ></textarea>
        <!-- Slash-command picker. Mutually exclusive with the mention
             picker (slash always wins because the input must start
             with /). The picker hides itself when its open flag is
             false; the mention picker is wrapped in {:else if} below
             so they can never both render at once. -->
        <SlashCommandPicker
          bind:this={slashPickerRef}
          bind:value={input}
          bind:open={slashPickerOpen}
          {inputEl}
          onSubmit={() => { void send(); }}
        />
        {#if !slashPickerOpen && mentionPickerOpen}
          <!-- @-mention picker. Floats above the composer; arrow
               keys navigate, Enter / Tab picks, Esc dismisses.
               Candidates pulled from a cached entity index loaded
               on first @-trigger. -->
          <div
            role="listbox"
            class="absolute left-0 right-0 bottom-full mb-1 bg-mantle border border-surface1 rounded-lg shadow-xl z-30 max-h-64 overflow-y-auto"
          >
            {#if mentionLoading && mentionCandidates.length === 0}
              <div class="px-3 py-2 text-[11px] text-dim italic">Loading…</div>
            {:else if mentionCandidates.length === 0}
              <div class="px-3 py-2 text-[11px] text-dim italic">No matches for "{mentionQuery}".</div>
            {:else}
              {#each mentionCandidates as c, i (c.kind + ':' + c.id)}
                <button
                  type="button"
                  role="option"
                  aria-selected={i === mentionSelectedIdx}
                  onmousedown={(e) => { e.preventDefault(); pickMention(c); }}
                  onmouseenter={() => (mentionSelectedIdx = i)}
                  class="w-full flex items-baseline gap-2 px-3 py-1.5 text-left hover:bg-surface0 {i === mentionSelectedIdx ? 'bg-surface0' : ''}"
                >
                  <span class="text-[9px] uppercase tracking-wider text-secondary flex-shrink-0 w-12">{c.kind}</span>
                  <span class="text-xs text-text truncate flex-1">{c.title}</span>
                  <span class="text-[10px] text-dim truncate max-w-[40%]">{c.subtitle}</span>
                </button>
              {/each}
            {/if}
          </div>
        {/if}
      </div>
      {#if voiceSupported}
        <!-- Voice input: tap to start, tap again to stop. Live
             transcript fills the input as the user speaks. Same
             SpeechRecognition shape as the voice-note modal. -->
        <button
          type="button"
          onclick={toggleVoice}
          disabled={busy || $sabbath}
          aria-pressed={recording}
          class="tap-target px-3 py-2 text-sm rounded font-medium disabled:opacity-40 inline-flex items-center justify-center transition-colors {recording ? 'bg-error text-white animate-pulse' : 'bg-surface0 border border-surface1 text-subtext hover:border-primary'}"
          title={recording ? 'Stop dictating' : 'Dictate (browser speech-to-text)'}
          aria-label={recording ? 'Stop dictating' : 'Dictate'}
        >
          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="9" y="3" width="6" height="12" rx="3"/>
            <path d="M5 11a7 7 0 0014 0M12 18v3" stroke-linecap="round"/>
          </svg>
        </button>
      {/if}
      <button
        type="submit"
        disabled={busy || !input.trim() || $sabbath}
        class="tap-target px-3 py-2 text-sm bg-primary text-on-primary rounded font-medium disabled:opacity-40 hover:bg-primary/90 active:bg-primary/80 transition-colors"
      >Send</button>
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
</style>
