<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { goto, beforeNavigate } from '$app/navigation';
  import { page } from '$app/stores';
  import { api, ApiError, type Note } from '$lib/api';
  import { installWsReload } from '$lib/notes/wsReload.svelte';
  import Editor from '$lib/editor/Editor.svelte';
  import NotesTree from '$lib/notes/NotesTree.svelte';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import DayActivityInline from '$lib/notes/DayActivityInline.svelte';
  import DailyQuickAdd from '$lib/notes/DailyQuickAdd.svelte';
  import DailyContext from '$lib/notes/DailyContext.svelte';
  import NoteDeadlinesStrip from '$lib/deadlines/NoteDeadlinesStrip.svelte';
  import Drawer from '$lib/components/Drawer.svelte';
  import { toast } from '$lib/components/toast';
  import { scheduleFlashcards } from '$lib/util/scheduleFlashcards';
  import { rememberScroll } from '$lib/notes/noteHistory';
  import { createPreviewScrollTracker } from '$lib/notes/previewScrollTracker.svelte';
  import { installNoteShortcuts } from '$lib/notes/noteKeyboardShortcuts.svelte';
  import { openResearchMode as openResearchModeFor } from '$lib/notes/researchMode';
  import { noteCrumbs, visibleCrumbs as visibleCrumbsFn, crumbsCollapsed as crumbsCollapsedFn } from '$lib/notes/noteBreadcrumbs';
  import ExtractToNoteDialog from '$lib/notes/ExtractToNoteDialog.svelte';
  import { createExtractController } from '$lib/notes/extractToNote.svelte';
  import { navigateWikilink as navigateWikilinkHelper } from '$lib/notes/wikilinkNav';
  import { parseTagsField, addSuggestedTag, insertSuggestedLink } from '$lib/notes/frontmatterTagOps';
  import { saveFrontmatter as saveFrontmatterFn } from '$lib/notes/saveFrontmatter';
  import { saveNote as saveNoteFn } from '$lib/notes/saveNote';
  import { loadNote as loadNoteFn } from '$lib/notes/loadNote';
  import { installNoteAutosave } from '$lib/notes/noteAutosave.svelte';
  import { createViewModeController } from '$lib/notes/viewModes.svelte';
  import {
    parseDailyDate,
    shiftDate,
    splitDayActivity,
    formatRelativeDailyLabel,
    todayLocalISO
  } from '$lib/notes/dailyNote';
  import PrintPreview from '$lib/notes/PrintPreview.svelte';
  import HistoryPanel from '$lib/notes/HistoryPanel.svelte';
  import ShortcutsHelpOverlay from '$lib/notes/ShortcutsHelpOverlay.svelte';
  import SelectionToolbar from '$lib/editor/SelectionToolbar.svelte';
  import MobileEditorToolbar from '$lib/editor/MobileEditorToolbar.svelte';
  import InlineAIMenu from '$lib/notes/InlineAIMenu.svelte';
  import AIActionBar from '$lib/notes/AIActionBar.svelte';
  import {
    inlineAITriggerExtension,
    type InlineAITriggerEvent
  } from '$lib/editor/inline-ai-trigger';
  import { inlineAIObserver, type InlineAIState } from '$lib/editor/inline-ai';
  import type { EditorView } from '@codemirror/view';
  import NoteSummaryCard from '$lib/notes/NoteSummaryCard.svelte';
  import NoteAudioPlayer from '$lib/notes/NoteAudioPlayer.svelte';
  import NotePresentation from '$lib/notes/NotePresentation.svelte';
  import NoteStatusBar from '$lib/notes/NoteStatusBar.svelte';
  import NoteOverflowMenu from '$lib/notes/NoteOverflowMenu.svelte';
  import NoteInfoRail from '$lib/notes/NoteInfoRail.svelte';
  import NoteHeader from '$lib/notes/NoteHeader.svelte';
  import { ensurePinnedLoaded, pinnedNotes, togglePin as togglePinPath } from '$lib/notes/pinnedNotes';
  import { recordOpenNote, updateOpenNoteScroll } from '$lib/stores/open-note';
  import { registerActiveEditor } from '$lib/stores/active-editor';

  // viewMode + focusMode + readingMode now live in a single
  // controller. See $lib/notes/viewModes for the contract; the
  // page reads ctrl.viewMode etc. via reactive getters and routes
  // every mutation through ctrl methods so persistence + the
  // prior-state snapshot logic for reading-mode stay together.
  const viewModes = createViewModeController();
  // Reactive aliases — template + downstream consumers read these
  // by name. Assignments go through the controller methods
  // (viewModes.toggleFocusMode, etc.).
  let viewMode = $derived(viewModes.viewMode);
  let focusMode = $derived(viewModes.focusMode);
  let readingMode = $derived(viewModes.readingMode);

  // Viewport tracking for the rail/tree mount strategy. Tailwind's
  // lg breakpoint is 1024px (left tree threshold) and xl is 1280px
  // (right info-rail threshold). Previously each rail was rendered
  // TWICE — once in a desktop `<aside class="hidden md:flex">` and
  // once in a `<Drawer>` wrapped by `md:hidden contents`. Both DOM
  // trees were always mounted; CSS just hid one. That meant every
  // panel's $derived/$effect ran twice, doubling the per-keystroke
  // cost of body-derived recomputation in the rail panels — a
  // meaningful chunk of the save-time freeze on long notes. We
  // track each breakpoint here and render the rail / tree to ONLY
  // one location at a time.
  // Initial values from synchronous matchMedia. SvelteKit hydrates
  // this component on the client only after the bundle loads, so
  // window is always defined here — but the typeof guard keeps SSR
  // (if it ever happens) from throwing. The onMount block below
  // wires up live updates; this initializer just avoids a one-frame
  // flash where the wrong layout renders before the listener fires.
  let isLg = $state(
    typeof window !== 'undefined' && window.matchMedia('(min-width: 1024px)').matches
  );
  let isXl = $state(
    typeof window !== 'undefined' && window.matchMedia('(min-width: 1280px)').matches
  );
  onMount(() => {
    // Boot the pinned-notes store so the toolbar's pin star (and any
    // other pin-aware surface mounted after this) reflects the
    // server-authoritative list without each component re-fetching.
    ensurePinnedLoaded();
    // MQL listeners for the lg + xl breakpoints.
    const lgMql = window.matchMedia('(min-width: 1024px)');
    const xlMql = window.matchMedia('(min-width: 1280px)');
    isLg = lgMql.matches;
    isXl = xlMql.matches;
    const onLg = (e: MediaQueryListEvent) => { isLg = e.matches; };
    const onXl = (e: MediaQueryListEvent) => { isXl = e.matches; };
    lgMql.addEventListener('change', onLg);
    xlMql.addEventListener('change', onXl);
    return () => {
      lgMql.removeEventListener('change', onLg);
      xlMql.removeEventListener('change', onXl);
    };
  });

  // Window after our own save during which an inbound `note.changed`
  // WS event is suppressed — the event is almost certainly the echo
  // of our own write bouncing through the file watcher. Used at both
  // the synchronous fast-path and the timed coalesce. 3 s covers the
  // worst-case file-watcher debounce + scan + broadcast latency.
  const OWN_SAVE_QUIET_MS = 3000;
  // Trailing-edge coalesce on `note.changed` bursts (a TUI save or
  // sync drop can fire 5+ events in a burst). One reload per burst.
  const WS_RELOAD_COALESCE_MS = 600;
  // Autosave debounce — fires `save({silent: true})` 2 s after the
  // last keystroke when the picker isn't open.
  const AUTOSAVE_DEBOUNCE_MS = 2000;

  let note = $state<Note | null>(null);
  let body = $state('');
  // bodyForPreview is the rAF-throttled mirror that drives the
  // MarkdownRenderer. Without this, every keystroke would re-parse
  // the full document (60–200×/sec on fast typing for a 600-line
  // note), since CodeMirror's updateListener writes `body` at
  // microtask speed. The throttle coalesces multiple keystrokes per
  // frame down to one parse, while keeping `body` live for save
  // logic, dirty tracking, and slash-command parsing that need the
  // unthrottled value. The rAF callback reads `body` at flush time
  // (not at effect-run time) so it always commits the latest text
  // even when 5+ keystrokes fall inside one 16ms frame.
  let bodyForPreview = $state('');
  let previewBodyRaf = 0;
  let previewBodyTimer: ReturnType<typeof setTimeout> | null = null;
  // Bodies above this size switch from rAF (16 ms) coalescing to a
  // 250 ms idle debounce for the preview mirror. At rAF cadence a
  // 100 KB note triggers ~60 marked.parse + synchronous DOMPurify
  // sweeps per second in MarkdownRenderer — each ~50–200 ms — and
  // the queue compounds faster than it drains. Settling on typing-
  // pause is the standard preview UX for long-form editors; the
  // editor's textarea stays at full rAF responsiveness because the
  // CodeMirror view never reads bodyForPreview.
  const PREVIEW_HEAVY_BODY = 32 * 1024;
  const PREVIEW_HEAVY_DEBOUNCE_MS = 250;
  $effect(() => {
    // First-paint fast path: when bodyForPreview is still empty but
    // body has loaded, sync synchronously instead of waiting for the
    // next rAF. Without this, opening a note flashes an empty
    // preview for ~16ms while the throttle's first frame is
    // pending — visible as a brief blank on every load and tab
    // switch. After init, bodyForPreview tracks body via the rAF
    // path, so this branch fires at most once per mount + once per
    // explicit clear-then-type cycle.
    if (bodyForPreview === '' && body !== '') {
      bodyForPreview = body;
      return;
    }
    if (body.length >= PREVIEW_HEAVY_BODY) {
      if (previewBodyRaf) {
        cancelAnimationFrame(previewBodyRaf);
        previewBodyRaf = 0;
      }
      if (previewBodyTimer) clearTimeout(previewBodyTimer);
      previewBodyTimer = setTimeout(() => {
        previewBodyTimer = null;
        bodyForPreview = body;
      }, PREVIEW_HEAVY_DEBOUNCE_MS);
      return;
    }
    if (previewBodyTimer) {
      clearTimeout(previewBodyTimer);
      previewBodyTimer = null;
    }
    if (previewBodyRaf) return;
    previewBodyRaf = requestAnimationFrame(() => {
      previewBodyRaf = 0;
      bodyForPreview = body;
    });
    // Note: NO cleanup return here. $effect cleanup fires on every
    // dep change, not just unmount — cancelling the pending rAF on
    // each body keystroke would defeat the coalescer (every
    // keystroke would cancel + reschedule instead of riding the
    // already-queued frame). Unmount cancellation is handled
    // separately via onDestroy below so the pending frame is
    // killed exactly once, when the component goes away.
  });
  onDestroy(() => {
    if (previewBodyRaf) {
      cancelAnimationFrame(previewBodyRaf);
      previewBodyRaf = 0;
    }
    if (previewBodyTimer) {
      clearTimeout(previewBodyTimer);
      previewBodyTimer = null;
    }
  });
  let saving = $state(false);
  let dirty = $state(false);
  let error = $state('');
  // True when the requested path 404s on load. Distinct from `error`
  // so we can render a "create this note?" affordance instead of an
  // error banner — a 404 on this surface is almost always the user
  // following an unresolved wikilink or typing a URL for a note
  // they're about to create.
  let notFound = $state(false);
  let creatingNote = $state(false);
  let lastLoadedPath = $state('');

  // Inline AI menu — populated by the inline-ai-trigger extension when
  // the user hits Cmd-K or types "/ai". Cleared when the menu closes.
  let aiTriggerEvent = $state<InlineAITriggerEvent | null>(null);

  // Schedule flashcard reviews on the calendar. Parses Q:/A: pairs
  // out of the current body (the format the InlineAIMenu
  // "flashcards" preset emits) and creates a 5-step spaced-rep
  // series per card at 1/3/7/14/30 day offsets — same cadence as
  // the scripture memory-verse drill. Busy flag keeps the overflow
  // menu from queueing duplicate runs.
  let schedulingFlashcards = $state(false);
  async function runScheduleFlashcards() {
    if (schedulingFlashcards) return;
    schedulingFlashcards = true;
    overflowOpen = false;
    try {
      const r = await scheduleFlashcards(body);
      if (r.cards === 0) {
        toast.info('No Q:/A: flashcards found in this note.');
        return;
      }
      if (r.failed === 0) {
        toast.success(`Scheduled ${r.cards} card${r.cards === 1 ? '' : 's'} × 5 reviews (1/3/7/14/30 days).`);
      } else {
        toast.info(`Scheduled ${r.scheduled} of ${r.cards * 5} reviews — ${r.failed} failed.`);
      }
    } catch (e) {
      toast.error('Schedule failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      schedulingFlashcards = false;
    }
  }

  // Inline AI ghost state — observed by inlineAIObserver, drives the
  // floating <AIActionBar> that surfaces Keep / Try again / Discard /
  // Stop buttons next to the ghost text. Null when no ghost is active.
  let aiGhostState = $state<InlineAIState | null>(null);

  // Reading progress 0..1 — driven by the editor's onScroll callback
  // (rAF-throttled there). When the doc fits in viewport the
  // denominator is 0 and we clamp to 0; once the user scrolls down a
  // long doc, we tint a 2px line at the top of the editor pane to
  // surface 'how far through am I'. Cheap, no polling.
  let readProgress = $state(0);

  // Preview-pane scroll container. Bound via bind:this below — must
  // stay an lvalue here. Outline uses it as the IntersectionObserver
  // root for active-heading tracking; the previewScrollTracker uses
  // it for the heading-checkpoint walk + progress bar.
  let previewContainer = $state<HTMLElement | null>(null);

  // Scroll-side state (visited headings + preview progress) lives
  // in the tracker. See $lib/notes/previewScrollTracker for the
  // attach() + loadFor() contract. We wire two thin effects: load
  // the visited set when the active note path changes, attach the
  // scroll listener while a container ref exists.
  const previewScroll = createPreviewScrollTracker();
  let visitedHeadings = $derived(previewScroll.visitedHeadings);
  let previewProgress = $derived(previewScroll.previewProgress);
  function resetVisited() {
    previewScroll.resetVisited(note?.path ?? null);
  }
  $effect(() => {
    previewScroll.loadFor(note?.path ?? null);
  });
  $effect(() => {
    const c = previewContainer;
    if (!c) return;
    return previewScroll.attach(c, () => note?.path ?? null);
  });
  let editor:
    | {
        scrollToLine: (n: number) => void;
        getScrollTop: () => number;
        setScrollTop: (top: number) => void;
        isCompletionActive: () => boolean;
        dispatchChord: (chord: string) => void;
        getDOM: () => HTMLElement | undefined;
        getView: () => EditorView | undefined;
        openFind: () => void;
        insertAtCursor: (text: string) => void;
        getContent: () => string;
      }
    | undefined = $state();
  // Re-derived after every render so the SelectionToolbar can scope
  // its selection detection to the editor's contentDOM specifically.
  // The CodeMirror DOM exists only after mount, so this stays
  // `undefined` until then and the toolbar simply doesn't render.
  let editorDOM = $derived(editor?.getDOM());

  // Register the current editor view as the "active editor" so
  // cross-surface features (AIOverlay's "insert at cursor", future
  // drop-into-note actions) can target this note's cursor without
  // each feature needing to know about the editor page. Deregisters
  // on unmount or when the editor binding goes away.
  $effect(() => {
    const view = editor?.getView?.();
    if (view) {
      registerActiveEditor(view);
      return () => registerActiveEditor(null);
    }
    return undefined;
  });

  // Per-note scroll position cache lives in $lib/notes/noteHistory —
  // see the imports at the top. Pixel-accurate (not line-accurate)
  // because line tracking misbehaves once the user changes font size
  // or window width — pixels survive reflow because we restore on
  // the same note (same width, same font) only.

  let treeDrawerOpen = $state(false);
  let infoDrawerOpen = $state(false);
  // Margin-note count for the active note. Surfaced via the
  // section header badge so the user sees at a glance how many
  // annotations the current note carries without scrolling. The
  // AnnotationsPanel owns the load + WS refresh; we receive
  // updates via its onCountChange prop.
  let annotationCount = $state(0);

  // Read the pinned set straight off the shared store — same source
  // of truth the notes tree, dashboard widget, and TUI all subscribe
  // to. ensurePinnedLoaded() (called in onMount above) handles the
  // initial fetch + the WS subscription that keeps the store fresh
  // when another tab / device toggles a pin, so this surface no
  // longer needs its own load + re-fetch wiring.
  let pinned = $derived($pinnedNotes);
  let pinBusy = $state(false);
  async function togglePin() {
    if (!note) return;
    pinBusy = true;
    try {
      await togglePinPath(note.path);
    } finally {
      pinBusy = false;
    }
  }

  $effect(() => {
    const path = $page.params.path;
    if (path) load(decodeURIComponent(path));
  });

  // Feed the global open-note tray. Whenever the loaded note swaps to
  // a new path, record it so the tray (mounted in the root layout)
  // can surface a "jump back" chip from anywhere in the app. Triggers
  // on note.path so a same-note refresh (WS reload) doesn't re-write
  // the entry on every server bounce — only navigation does.
  $effect(() => {
    if (!note) return;
    recordOpenNote({
      path: note.path,
      title: note.title || note.path,
      scrollPos: editor?.getScrollTop?.() ?? 0
    });
  });

  let draftRestored = $state(false);

  // Load now lives in $lib/notes/loadNote — see there for the
  // draft-reconciliation contract, the "always prefer the draft"
  // rule on divergence, and the 404 / network-error fallbacks.
  async function load(p: string, opts: { force?: boolean } = {}) {
    return loadNoteFn(p, opts, noteState, {
      getLiveBody: () => editor?.getContent?.() ?? body,
      getEditorView: () => editor?.getView?.(),
      scrollToLine: (n) => editor?.scrollToLine?.(n),
      setScrollTop: (top) => editor?.setScrollTop?.(top),
      closeDrawers: () => { treeDrawerOpen = false; infoDrawerOpen = false; },
      setBreadcrumbExpanded: (v) => { breadcrumbExpanded = v; },
      getLineParam: () => $page.url.searchParams.get('line'),
      getRawHash: () => $page.url.hash ? decodeURIComponent($page.url.hash.slice(1)) : '',
      save: (o) => save(o)
    });
  }

  // Title inferred from the path for the not-found state. Strips the
  // .md extension and the folder prefix so "/notes/projects/foo.md"
  // shows as "foo".
  let notFoundTitle = $derived.by(() => {
    const path = $page.params.path ?? '';
    if (!path) return '';
    return decodeURIComponent(path).split('/').pop()?.replace(/\.md$/, '') ?? '';
  });

  async function createMissingNote() {
    const path = decodeURIComponent($page.params.path ?? '');
    if (!path || creatingNote) return;
    const cleanPath = path.endsWith('.md') ? path : path + '.md';
    creatingNote = true;
    try {
      await api.createNote({ path: cleanPath, body: '' });
      notFound = false;
      lastLoadedPath = '';
      await load(cleanPath, { force: true });
    } catch (e) {
      toast.error(`Couldn't create note: ${e instanceof Error ? e.message : String(e)}`);
    } finally {
      creatingNote = false;
    }
  }

  let lastSavedAt = $state<number | null>(null);
  // ETag from the most recent successful load / save. Sent as `If-Match`
  // on every PUT so a concurrent edit from another tab / TUI / sync
  // surfaces as a 412 instead of being silently overwritten. Reset to
  // null whenever the active note changes — a fresh load() will refill
  // it from the response. Bumped on every successful save() so a long
  // edit session stays anchored to the latest server state.
  let noteEtag = $state<string | null>(null);
  // When the user has chosen to overwrite a detected conflict, the next
  // save skips the If-Match header. The flag clears after one save so
  // a subsequent edit is again guarded.
  let forceNextSave = $state(false);
  // Frontmatter that hit 412 — held verbatim so the conflict banner's
  // Overwrite button can re-run saveFrontmatter() with the original
  // payload + forceNextSave. Without this, an Overwrite after a tag-
  // chip conflict ran the body-save path and silently dropped the
  // pending frontmatter edit.
  let pendingFrontmatter = $state<Record<string, unknown> | null>(null);
  // Derived from the last save error — drives the conflict banner.
  // The 412 catch branch in save() sets lastSaveError to a string
  // starting with "Conflict:"; nothing else uses that prefix.
  let conflictDetected = $derived(saveFailed && lastSaveError.startsWith('Conflict'));
  // Single 1s tick that both relative-time labels (saveStatus in
  // the header + lastSavedDisplay in the status bar) derive from.
  // Previously each surface had its own setInterval (1s + 5s) plus
  // a manual `void nowTick` keep-alive — now there's one interval
  // and both labels stay reactive purely through $derived deps.
  let nowTick = $state(Date.now());
  let saveFailed = $state(false);
  // Consecutive save-failure counter. Resets to 0 on any success.
  // Used by the in-page banner below to show a sticky, dismiss-only-
  // by-fixing surface so the user always knows when their edits
  // aren't reaching the server.
  let saveFailCount = $state(0);
  let lastSaveError = $state('');

  // Tick once per second so "saved Ns ago" stays accurate.
  $effect(() => {
    const t = setInterval(() => (nowTick = Date.now()), 1000);
    return () => clearInterval(t);
  });

  // Body+frontmatter save now lives in $lib/notes/saveNote — see
  // there for the conflict / draft / surgical-mutation contract.
  // Thin wrapper just plumbs in the shared state proxy and clears
  // the draftRestored badge on success.
  async function save(opts: { silent?: boolean } = {}): Promise<boolean> {
    if (saving) return !dirty;
    return saveNoteFn(opts, noteState, saveCtx, () => { draftRestored = false; });
  }

  // dirty + prev are declared here so the noteState proxy below can
  // reference them. The actual tracker effect, the autosave debounce,
  // the rAF-coalesced draft write, the tab-hide / unload flush, and
  // the online-retry effect are all installed by installNoteAutosave
  // further down — see $lib/notes/noteAutosave for the contract.
  let prev = $state('');
  let lastDraftedBody: string | null = null;

  // Single mutable view onto the page's note-pipeline $state.
  // Every save/load/frontmatter helper in $lib/notes accepts this
  // proxy so they can read and write each field reactively without
  // re-spelling the same getter/setter pairs at every call site.
  const noteState = {
    get note() { return note; }, set note(v) { note = v; },
    get body() { return body; }, set body(v) { body = v; },
    get prev() { return prev; }, set prev(v) { prev = v; },
    get saving() { return saving; }, set saving(v) { saving = v; },
    get dirty() { return dirty; }, set dirty(v) { dirty = v; },
    get error() { return error; }, set error(v) { error = v; },
    get lastSavedAt() { return lastSavedAt; }, set lastSavedAt(v) { lastSavedAt = v; },
    get noteEtag() { return noteEtag; }, set noteEtag(v) { noteEtag = v; },
    get forceNextSave() { return forceNextSave; }, set forceNextSave(v) { forceNextSave = v; },
    get pendingFrontmatter() { return pendingFrontmatter; }, set pendingFrontmatter(v) { pendingFrontmatter = v; },
    get saveFailed() { return saveFailed; }, set saveFailed(v) { saveFailed = v; },
    get saveFailCount() { return saveFailCount; }, set saveFailCount(v) { saveFailCount = v; },
    get lastSaveError() { return lastSaveError; }, set lastSaveError(v) { lastSaveError = v; },
    get lastDraftedBody() { return lastDraftedBody; }, set lastDraftedBody(v) { lastDraftedBody = v; },
    get notFound() { return notFound; }, set notFound(v) { notFound = v; },
    get lastLoadedPath() { return lastLoadedPath; }, set lastLoadedPath(v) { lastLoadedPath = v; },
    get draftRestored() { return draftRestored; }, set draftRestored(v) { draftRestored = v; }
  };
  const saveCtx = { getLiveBody: () => editor?.getContent?.() ?? body };

  // Install dirty + autosave + draft-rAF + tab-hide flush + online
  // retry — six effects worth of plumbing live in noteAutosave so
  // this surface keeps the deps wiring at one glance.
  installNoteAutosave({
    getNote: () => note,
    getBody: () => body,
    getLiveBody: () => editor?.getContent?.() ?? body,
    getDirty: () => dirty,
    getSaving: () => saving,
    getSaveFailed: () => saveFailed,
    getConflictDetected: () => conflictDetected,
    getPrev: () => prev,
    setDirty: (v) => { dirty = v; },
    setPrev: (v) => { prev = v; },
    getLastDraftedBody: () => lastDraftedBody,
    setLastDraftedBody: (v) => { lastDraftedBody = v; },
    getEditorView: () => editor?.getView?.(),
    isCompletionActive: () => editor?.isCompletionActive?.() ?? false,
    save: (o) => save(o),
    autosaveDebounceMs: AUTOSAVE_DEBOUNCE_MS
  });

  // Best-effort flush on SPA navigation. We can't synchronously block
  // the navigation (await isn't honored by browser navigations), but the
  // draft layer protects against data loss on the worst case — this just
  // tries to push edits over the wire faster than the 2s debounce would.
  beforeNavigate(() => {
    if (dirty && !saving && note) {
      // Body is already in localStorage via setDraft (synchronous
      // per-keystroke write — see the draft effect comment).
      // Fire-and-forget the save; it'll race the navigation but either
      // outcome is safe (draft still on disk).
      void save({ silent: true });
    }
    // Remember the scroll position so navigating back to this note
    // returns to where the user was reading. Saved synchronously so
    // even a forced reload (close tab) catches it. We mirror the
    // value onto the open-note tray entry so the (optional) "resume
    // at line N" hint can render without consulting noteHistory.
    if (note && editor?.getScrollTop) {
      const top = editor.getScrollTop();
      rememberScroll(note.path, top);
      updateOpenNoteScroll(note.path, top);
    }
  });

  // Save status label that updates with the live tick. nowTick is
  // read implicitly via the subtraction so the derivation tracks
  // it without a `void nowTick` dance.
  let saveStatus = $derived.by(() => {
    if (saving) return 'saving…';
    if (saveFailed && dirty) return 'retry?';
    if (dirty) return 'unsaved';
    if (!lastSavedAt) return 'saved';
    const ago = Math.floor((nowTick - lastSavedAt) / 1000);
    if (ago < 4) return 'saved';
    if (ago < 60) return `saved ${ago}s ago`;
    if (ago < 3600) return `saved ${Math.floor(ago / 60)}m ago`;
    return 'saved';
  });

  // Brief flash after each successful autosave so the user can SEE
  // that an autosave actually fired. Without this, saves are invisible
  // — the status bar updates silently and the user has no positive
  // confirmation that their work made it to disk. The flash window is
  // 1.2s (long enough to register, short enough not to nag) and
  // doesn't fire when the save was triggered by an explicit Mod-S
  // (those already get a toast.success). The flash is a CSS-driven
  // outline pulse; the existing saveStatus label still drives the
  // text content of the button.
  let saveFlash = $state(false);
  let saveFlashTimer: ReturnType<typeof setTimeout> | null = null;
  $effect(() => {
    if (!lastSavedAt) return;
    saveFlash = true;
    if (saveFlashTimer) clearTimeout(saveFlashTimer);
    saveFlashTimer = setTimeout(() => {
      saveFlash = false;
      saveFlashTimer = null;
    }, 1200);
    return () => {
      if (saveFlashTimer) { clearTimeout(saveFlashTimer); saveFlashTimer = null; }
    };
  });

  // ----- Extract-to-note (Mod-Shift-X) -----
  // Controller owns the ExtractRequest state machine + create-then-
  // replace flow. See $lib/notes/extractToNote for the contract.
  // We pass getNote/save as closures so the controller stays free
  // of route state while we keep autosave in the page.
  const extractCtl = createExtractController({
    getNote: () => note,
    save: (o) => save(o)
  });

  let printOpen = $state(false);
  let historyOpen = $state(false);
  let helpOpen = $state(false);

  // Snapshot count for the current note — surfaces history existence
  // via a small chip in NoteHeader. Refreshes on note swap and after
  // every successful save (modTime ticks). The listHistory endpoint
  // is O(1) since the v3 manifest sidecar, so re-fetching per save is
  // cheap.
  let versionCount = $state(0);
  let versionCountGen = 0;
  $effect(() => {
    const path = note?.path;
    void note?.modTime;
    if (!path) {
      versionCount = 0;
      return;
    }
    const myGen = ++versionCountGen;
    void (async () => {
      try {
        const data = await api.listHistory(path);
        if (myGen === versionCountGen) versionCount = data.versions?.length ?? 0;
      } catch {
        if (myGen === versionCountGen) versionCount = 0;
      }
    })();
  });
  // Audio mode — read-aloud player for the current note. Browser
  // SpeechSynthesis only, no backend. Closed by default; opens via
  // the toolbar button.
  let audioOpen = $state(false);
  // Slideshow / presentation mode — fullscreen deck view of the
  // note, split on H2 boundaries. Closed by default; opens via the
  // toolbar button or Mod-Shift-P.
  let presentationOpen = $state(false);

  // Overflow menu — collapses the secondary header actions (find,
  // history, PDF, slideshow, audio, reading, focus, flashcards,
  // help) behind a single ⋯ trigger. State + positioning + click-
  // outside / Esc / resize wiring live inside <NoteOverflowMenu>.
  // We keep `overflowOpen` and the trigger ref here so the header
  // button still toggles the menu and `runScheduleFlashcards` can
  // close it before the long-running async job.
  let overflowOpen = $state(false);
  let overflowTriggerEl: HTMLButtonElement | undefined = $state();

  // ── Editor extra extensions ─────────────────────────────────────
  // Editor.svelte reads extraExtensions ONCE at setupView time, so
  // this array must not be re-created on every render. Two host
  // bridges live here:
  //   • inlineAITriggerExtension — turns Cmd-K / "/ai" into a menu
  //     open event the page renders via <InlineAIMenu>.
  //   • inlineAIObserver — fires whenever the inline-AI state field
  //     changes (start/stream/done/clear) so the page can render the
  //     floating <AIActionBar> with the right buttons.
  const editorAIExtensions = [
    inlineAITriggerExtension((e) => {
      aiTriggerEvent = e;
    }),
    inlineAIObserver((s) => {
      aiGhostState = s;
    })
  ];

  async function navigateWikilink(target: string) {
    // Best-effort flush of any pending edit. We never block navigation on
    // the save result — the localStorage draft already preserves the body
    // and beforeNavigate flushes again. If the user is offline, save will
    // fail; the draft is on disk and gets retried automatically when
    // 'online' fires. The lookup + AI-offer + goto flow lives in
    // wikilinkNav so this surface only owns the dirty-flush + ctx wiring.
    if (dirty) void save({ silent: true });
    await navigateWikilinkHelper(target, {
      parentPath: note?.path ?? '',
      parentBody: body ?? ''
    });
  }

  $effect(() => {
    const handler = (e: BeforeUnloadEvent) => {
      // Save scroll position synchronously — beforeunload is the last
      // chance before tab close. We also save on beforeNavigate
      // (SPA-internal nav) so the two cover both paths.
      if (note && editor?.getScrollTop) {
        rememberScroll(note.path, editor.getScrollTop());
      }
      if (dirty) {
        e.preventDefault();
        e.returnValue = '';
      }
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  });

  // Live-reload current note from WS via a small controller that
  // coalesces note.changed bursts (PUT handler + file watcher fire
  // separately) and honours every clobber guard: unsaved edits, an
  // in-flight save, our own just-completed save bouncing back.
  // See $lib/notes/wsReload for the contract.
  onMount(() =>
    installWsReload({
      getActivePath: () => note?.path ?? null,
      getLiveBody: () => editor?.getContent?.() ?? body,
      getSavedBody: () => prev,
      isSaving: () => saving,
      getLastSavedAt: () => lastSavedAt,
      reload: (p) => void load(p, { force: true }),
      ownSaveQuietMs: OWN_SAVE_QUIET_MS,
      coalesceMs: WS_RELOAD_COALESCE_MS
    })
  );

  // Drive the status-bar counters off the rAF-throttled mirror, not
  // the raw `body`. Each derivation here allocates a new array per
  // keystroke (trim+split for wordCount, split for lineCount), which
  // is O(N) in body length. On a 100 KB note that's ~3–8 ms per
  // keystroke just to refresh the status bar — a real contributor to
  // the editor-freeze the user reports on long notes. The status bar
  // can absolutely tolerate one frame of lag (16 ms); pinning it to
  // bodyForPreview coalesces the work with the preview parse instead
  // of firing it 60–200×/s on a fast typer.
  let wordCount = $derived.by(() => {
    const t = bodyForPreview.trim();
    return t ? t.split(/\s+/).length : 0;
  });
  let charCount = $derived(bodyForPreview.length);
  let lineCount = $derived(bodyForPreview ? bodyForPreview.split('\n').length : 0);
  // Reading time at ~225 wpm — average silent reading speed. Floor of
  // 1 minute so a short note doesn't read "0 min". Hidden under 50
  // words because "<1 min" on a tiny note is noise.
  let readingMinutes = $derived(Math.max(1, Math.round(wordCount / 225)));

  // Word-count goal — frontmatter `target_words: 1500` turns the
  // status-bar word count into a progress indicator. Common shape
  // for journaling / essay drafts where the user committed to a
  // target. We render a thin progress bar under the count + a
  // percentage label so progress is visible at a glance without
  // taking footer space when no target is set.
  let wordGoal = $derived.by<number | null>(() => {
    const fm = note?.frontmatter as Record<string, unknown> | undefined;
    if (!fm) return null;
    const v = fm.target_words ?? fm.word_goal;
    if (typeof v === 'number' && v > 0) return Math.floor(v);
    if (typeof v === 'string') {
      const n = parseInt(v, 10);
      if (!Number.isNaN(n) && n > 0) return n;
    }
    return null;
  });
  let wordGoalPct = $derived(
    wordGoal ? Math.min(100, Math.round((wordCount / wordGoal) * 100)) : 0
  );

  // Cursor position state — populated by the Editor's onCursor
  // callback. line:col is 1-indexed (matches what every editor
  // status bar shows). selLen > 0 means the user has a selection;
  // we surface a "{N} selected" badge in that case so the user
  // knows how much they're about to act on.
  let cursorLine = $state(1);
  let cursorCol = $state(1);
  let cursorSelLen = $state(0);

  // Last-saved relative time for the status bar. Shares the same
  // 1s nowTick as saveStatus above — the previous 5s setInterval
  // here was redundant; the actual displayed value only changes
  // on second/minute/hour boundaries anyway.
  let lastSavedDisplay = $derived.by(() => {
    if (!lastSavedAt) return '—';
    const sec = Math.round((nowTick - lastSavedAt) / 1000);
    if (sec < 5) return 'just now';
    if (sec < 60) return `${sec}s ago`;
    if (sec < 3600) return `${Math.round(sec / 60)}m ago`;
    return `${Math.round(sec / 3600)}h ago`;
  });

  // All window-level keyboard shortcuts (? / Mod-/ / Mod-Shift-Z /
  // Mod-Shift-R / Mod-Shift-P / Mod-Shift-←/→) live in a single
  // installer. See $lib/notes/noteKeyboardShortcuts for the wiring
  // contract; the page just plumbs current state via getters so
  // the installer stays free of the page's reactive scope.
  $effect(() =>
    installNoteShortcuts({
      viewModes,
      getEditorDOM: () => editor?.getDOM?.(),
      getIsDaily: () => isDaily,
      getDailyDate: () => dailyDate,
      hasNote: () => note !== null,
      shiftDate,
      gotoDaily,
      openHelp: () => { helpOpen = true; },
      openPresentation: () => { presentationOpen = true; }
    })
  );

  function jumpToLine(lineNum: number) {
    editor?.scrollToLine(lineNum);
    infoDrawerOpen = false;
  }

  // Research Mode — see $lib/notes/researchMode for the AI overlay
  // seeding contract. The page just forwards the active note + body.
  function openResearchMode(): void {
    if (!note) return;
    openResearchModeFor(note, body);
  }

  // Frontmatter save lives in $lib/notes/saveFrontmatter — see
  // there for the conflict + draft + surgical-mutation contract.
  // The page just plumbs its reactive state via noteState above.
  async function saveFrontmatter(next: Record<string, unknown>): Promise<boolean> {
    return saveFrontmatterFn(next, noteState, saveCtx);
  }

  // ----- Link-suggester glue -----
  // Tags chip → append to frontmatter.tags (de-duplicated, via the
  // existing saveFrontmatter pipeline). Link chip → insert at editor
  // cursor or append to body if the editor is unmounted (preview view).
  // Pure helpers live in $lib/notes/frontmatterTagOps.
  let existingTagList = $derived(
    parseTagsField(note?.frontmatter as Record<string, unknown> | undefined)
  );

  async function addSuggestedTagPage(tag: string) {
    if (!note) return;
    await addSuggestedTag(tag, { note, saveFrontmatter });
  }

  function insertSuggestedLinkPage(markup: string) {
    insertSuggestedLink(markup, {
      insertAtCursor: editor?.insertAtCursor,
      appendToBody: (m) => {
        body = body + (body.endsWith('\n') ? '' : '\n') + m + '\n';
        dirty = true;
      }
    });
  }

  // ----- Daily-note navigation -----
  // A note is "daily" when its basename is YYYY-MM-DD.md OR its
  // frontmatter has type=daily. The detection + date math live in
  // $lib/notes/dailyNote — here we just thread the derived values
  // through the reactive graph. dayActivitySegments derives from
  // bodyForPreview (the rAF-throttled mirror) so a fast typist on
  // a daily note doesn't pay for indexOf + slice per keystroke.
  let dailyDate = $derived(parseDailyDate(note));
  let isDaily = $derived(dailyDate !== null);
  let dayActivitySegments = $derived(
    isDaily ? splitDayActivity(bodyForPreview) : null
  );

  async function gotoDaily(date: string) {
    if (dirty) void save({ silent: true });
    try {
      // /api/v1/daily/<date> creates today's note if missing; for past/future
      // dates it just returns the existing note (we won't auto-materialize
      // an empty file for arbitrary historical dates).
      const n = await api.daily(date);
      goto(`/notes/${encodeURIComponent(n.path)}`);
    } catch {
      // If no existing daily for that date, just try the canonical path.
      goto(`/notes/${encodeURIComponent(date + '.md')}`);
    }
  }

  // Folder breadcrumbs. Reset on real navigation is folded into
  // load() since it's the only path that swaps note identity; the
  // pure derivation + collapse rule lives in noteBreadcrumbs.
  let breadcrumbExpanded = $state(false);
  let allCrumbs = $derived(noteCrumbs(note?.path));
  let visibleCrumbs = $derived(visibleCrumbsFn(allCrumbs, breadcrumbExpanded));
  let crumbsCollapsed = $derived(crumbsCollapsedFn(allCrumbs, breadcrumbExpanded));

  let dailyLabel = $derived.by(() => {
    if (!dailyDate) return '';
    return formatRelativeDailyLabel(dailyDate, todayLocalISO());
  });
</script>

{#snippet treeContent()}
  <div class="px-2 pt-2 pb-1 text-xs uppercase tracking-wider text-dim flex-shrink-0">Vault</div>
  <NotesTree currentPath={note?.path} onSelect={() => (treeDrawerOpen = false)} />
{/snippet}

{#snippet infoContent()}
  <!-- Right rail contents extracted to <NoteInfoRail> 2026-05-28.
       The snippet wrapper stays so the matchMedia-gated mount
       strategy (single mount of the rail to either the desktop
       aside or the drawer, never both) is preserved. -->
  <NoteInfoRail
    {note}
    body={bodyForPreview}
    {viewMode}
    {previewContainer}
    {visitedHeadings}
    {cursorLine}
    bind:annotationCount
    {existingTagList}
    onJumpToLine={jumpToLine}
    onNavigateWikilink={navigateWikilink}
    onResetVisited={resetVisited}
    onSaveFrontmatter={saveFrontmatter}
    onAddSuggestedTag={addSuggestedTagPage}
    onInsertSuggestedLink={insertSuggestedLinkPage}
  />
{/snippet}

<div class="h-full flex" class:focus-mode={focusMode} class:reading-mode={readingMode}>
  <!-- Tree — gated on the lg breakpoint. Same double-mount story as
       the right info rail: BOTH the desktop aside and the drawer
       used to render at every viewport with CSS hiding one, which
       wasted a NotesTree mount + its WS subscription per page mount.
       Render only the active one based on the live `isLg` flag. -->
  {#if isLg}
    <aside class="hidden lg:flex lg:flex-col lg:w-64 xl:w-72 border-r border-surface1 bg-mantle flex-shrink-0 focus-hide">
      {@render treeContent()}
    </aside>
  {:else}
    <Drawer bind:open={treeDrawerOpen} side="left" responsive width="w-72 sm:w-80">
      <div class="h-full flex flex-col">
        {@render treeContent()}
      </div>
    </Drawer>
  {/if}

  <!-- Center: editor -->
  <div class="flex-1 flex flex-col min-w-0">
    {#if notFound && !note}
      <!-- Empty / not-found state. Shows the would-be title with a
           one-click "Create" affordance — far better than the
           previous bare "loading…" or the error banner that fired
           when getNote 404'd. The clean-tree drawer link gives the
           user an escape if they didn't actually mean to create. -->
      <header class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0 bg-mantle sticky top-0 z-20">
        <a
          href="/notes"
          aria-label="back to notes"
          class="w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </a>
        <h1 class="text-base font-semibold text-text flex-1 truncate">{notFoundTitle || 'New note'}</h1>
      </header>
      <div class="flex-1 flex items-center justify-center p-8">
        <div class="max-w-md text-center">
          <div class="text-base text-text mb-1">This note doesn't exist yet</div>
          <div class="text-sm text-dim mb-5">
            Create <code class="text-subtext">{decodeURIComponent($page.params.path ?? '')}</code>
            with an empty body and start writing.
          </div>
          <button
            onclick={createMissingNote}
            disabled={creatingNote}
            class="px-4 py-2 rounded bg-primary text-on-primary text-sm font-medium hover:opacity-90 disabled:opacity-60"
          >
            {creatingNote ? 'Creating…' : 'Create note'}
          </button>
        </div>
      </div>
    {:else if error && !note}
      <!-- Stuck-on-error escape header. When the load failed and we
           have no note to render, the normal header below is hidden
           too — without this the user has no UI to navigate away
           except a full page reload. Keep it minimal: just a back
           link and the error message. -->
      <header class="flex items-center gap-2 px-3 py-2 border-b border-surface1 flex-shrink-0 bg-mantle sticky top-0 z-20">
        <a
          href="/notes"
          aria-label="back to notes"
          class="w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded flex-shrink-0"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </a>
        <button
          onclick={() => (treeDrawerOpen = true)}
          aria-label="vault tree"
          class="lg:hidden w-9 h-9 flex items-center justify-center text-subtext hover:bg-surface0 rounded flex-shrink-0"
        >
          <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M3 6h18M3 12h18M3 18h18" stroke-linecap="round" />
          </svg>
        </button>
        <h1 class="text-base font-semibold text-text flex-1 truncate">Couldn't open note</h1>
        <button
          onclick={() => { lastLoadedPath = ''; load(decodeURIComponent($page.params.path ?? '')); }}
          class="px-3 py-1.5 text-xs bg-surface0 border border-surface1 rounded hover:border-primary text-text"
        >Retry</button>
      </header>
      <div class="p-6 text-sm text-error">{error}</div>
    {:else if error}
      <div class="px-4 py-2 text-sm text-error border-b border-error bg-surface0 flex-shrink-0">{error}</div>
    {/if}
    {#if note}
      <!-- Header chrome extracted to <NoteHeader> 2026-05-28. The page
           still owns every piece of state behind the header (note,
           viewMode, breadcrumbs, daily-date helpers, pin set, save
           state, overflow state); the component is a presentational
           shell that takes the props and emits the callbacks. -->
      <NoteHeader
        {note}
        {viewMode}
        {isDaily}
        {dailyDate}
        {dailyLabel}
        {visibleCrumbs}
        {allCrumbs}
        {crumbsCollapsed}
        {pinned}
        {pinBusy}
        {wordCount}
        {readingMinutes}
        {previewProgress}
        {saveStatus}
        {saving}
        {dirty}
        {saveFailed}
        {saveFlash}
        {overflowOpen}
        bind:overflowTriggerEl
        onOpenTreeDrawer={() => (treeDrawerOpen = true)}
        onOpenInfoDrawer={() => (infoDrawerOpen = true)}
        onExpandBreadcrumbs={() => (breadcrumbExpanded = true)}
        onSetViewMode={viewModes.setViewMode}
        onTogglePin={togglePin}
        onGotoDaily={gotoDaily}
        onShiftDate={shiftDate}
        onDispatchAI={() => editor?.dispatchChord('Mod-/')}
        onOpenResearchMode={openResearchMode}
        onToggleOverflow={() => (overflowOpen = !overflowOpen)}
        onSave={() => save()}
        {versionCount}
        onOpenHistory={() => (historyOpen = true)}
      />
      {#if isDaily && note}
        {@const np = note.path}
        <!-- Carryover (yesterday's open) + habit checklist render
             above the quick-add bar so they're the first thing the
             user sees on the daily. Both collapse to a header line
             when the user wants the editor max-screen. -->
        <DailyContext onChanged={async () => { lastLoadedPath = ''; await load(np); }} />
        <DailyQuickAdd notePath={np} dailyDate={dailyDate} onAdded={async () => { lastLoadedPath = ''; await load(np); }} />
      {/if}
      <!-- Audio player strip — visible only when the user has
           toggled the audio button. Sits above the deadline strip
           so the controls are at the top of the reading surface,
           the natural place to find a transport bar. The player
           cleans up on unmount, so flipping the toggle off stops
           any in-flight reading. -->
      {#if audioOpen}
        <NoteAudioPlayer
          body={bodyForPreview}
          title={note.title || note.path}
          onClose={() => (audioOpen = false)}
        />
      {/if}
      <!-- Deadline strip — surfaces project/goal-linked deadlines for
           this note. Renders nothing when frontmatter has neither
           field, or none of the deadlines match. -->
      <NoteDeadlinesStrip frontmatter={note.frontmatter ?? null} />
      {#if draftRestored}
        <!-- Persistent affordance while editing on top of a localStorage
             draft. Previously the user only saw a 3s toast, then nothing —
             they couldn't tell "why are my changes here?" when revisiting
             a recovered note. Clears on next successful save. -->
        <div class="px-3 py-1.5 text-xs flex items-center gap-2 bg-warning/15 border-b border-warning/30 text-text">
          <svg viewBox="0 0 24 24" class="w-3.5 h-3.5 flex-shrink-0 text-warning" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M21 12.79A9 9 0 1 1 11.21 3a7 7 0 0 0 9.79 9.79z"/>
          </svg>
          <span class="flex-1">Editing a restored draft from this browser — saves on disk reflect what's typed here.</span>
          <button
            onclick={() => (draftRestored = false)}
            class="px-1.5 py-0.5 text-[10px] text-dim hover:text-text"
            aria-label="dismiss"
          >dismiss</button>
        </div>
      {/if}
      <!-- Conflict banner — the file on disk moved forward since we
           loaded it (412 Precondition Failed from putNote's If-Match
           guard). Two explicit choices, never silent overwrite:
           Reload server version (discard local changes for the
           server's body), or Overwrite anyway (skip If-Match on the
           next save and stomp the server's version). Lives above the
           transient-failure banner so it can't be hidden behind it. -->
      {#if conflictDetected && note}
        <div
          role="status"
          class="px-3 sm:px-4 py-2 border-b border-warning bg-warning/10 text-text text-xs sm:text-sm flex items-center gap-3"
        >
          <span class="flex-shrink-0 text-warning" aria-hidden="true">⚠</span>
          <span class="flex-1 min-w-0">
            <strong class="font-semibold">Conflict</strong> — this note was changed elsewhere (another tab, TUI, or sync) since you opened it. Your local edits are safe; choose how to resolve.
          </span>
          <button
            type="button"
            onclick={() => { lastLoadedPath = ''; void load(note!.path, { force: true }); }}
            class="px-2.5 py-1 rounded bg-surface0 hover:bg-surface1 text-text font-medium flex-shrink-0"
          >
            Reload server version
          </button>
          <button
            type="button"
            onclick={() => {
              forceNextSave = true;
              // Route Overwrite back through the SAME save shape that
              // hit 412. If the conflict was a tag-chip / frontmatter
              // change, replaying through save() (body) would drop the
              // user's intended edit and stomp the server's body too;
              // we re-run saveFrontmatter with the held next-payload
              // and forceNextSave so the originally-attempted change
              // is the one that lands.
              if (pendingFrontmatter) {
                const next = pendingFrontmatter;
                void saveFrontmatter(next);
              } else {
                void save({ silent: false });
              }
            }}
            disabled={saving}
            class="px-2.5 py-1 rounded bg-warning/30 hover:bg-warning/40 text-text font-medium flex-shrink-0 disabled:opacity-50"
          >
            {saving ? 'overwriting…' : 'Overwrite anyway'}
          </button>
        </div>
      {/if}
      <!-- Repeated-save-failure banner. Goes sticky after the 2nd
           consecutive failure — earlier failures are surfaced via
           the per-failure toast. The threshold avoids alarming the
           user on a one-off network blip while still making prolonged
           outages obvious. The banner exposes the actual error and a
           manual "retry now" button so the user has agency rather
           than waiting on the silent autosave loop. Drafts on
           localStorage protect their content meanwhile. -->
      {#if saveFailCount >= 2 && !conflictDetected && note}
        <div
          role="status"
          class="px-3 sm:px-4 py-2 border-b border-error bg-surface0 text-error text-xs sm:text-sm flex items-center gap-3"
        >
          <span class="flex-shrink-0" aria-hidden="true">⚠</span>
          <span class="flex-1 min-w-0">
            <strong class="font-semibold">Autosave failing</strong> ({saveFailCount} attempt{saveFailCount === 1 ? '' : 's'})
            {#if lastSaveError}<span class="text-error/80"> — {lastSaveError}</span>{/if}.
            Your edits are saved locally and will sync when the server is reachable.
          </span>
          <button
            type="button"
            onclick={() => save({ silent: false })}
            disabled={saving}
            class="px-2.5 py-1 rounded bg-surface0 hover:bg-surface1 text-error font-medium flex-shrink-0 disabled:opacity-50"
          >
            {saving ? 'retrying…' : 'retry now'}
          </button>
        </div>
      {/if}
      <!-- Reading-progress bar — thin tinted strip showing how far
           through the note the user has scrolled. Hidden when
           progress is essentially 0 (note fits in viewport) so it
           doesn't render a visible artifact on short notes. The
           transition smooths the value as we throttle the source
           on rAF. -->
      {@const activeProgress = viewMode === 'preview' ? previewProgress : readProgress}
      {#if activeProgress > 0.005}
        <div
          class="h-[2px] bg-primary/70 transition-[width] duration-100 ease-out"
          style="width: {(activeProgress * 100).toFixed(1)}%"
          aria-hidden="true"
        ></div>
      {/if}
      <!-- EditorAIBar removed — the inline AI menu (Cmd-/ / "/ai") is
           the only AI entry point now. See $lib/notes/InlineAIMenu.svelte
           and its trigger registration in editorAIExtensions above. -->
      {#snippet previewBody()}
        {#if dayActivitySegments && dailyDate}
          <MarkdownRenderer body={dayActivitySegments.before} onWikilink={navigateWikilink} />
          <DayActivityInline date={dailyDate} />
          <MarkdownRenderer body={dayActivitySegments.after} onWikilink={navigateWikilink} />
        {:else}
          <!-- Throttled body — rAF-coalesced via bodyForPreview above.
               Multiple keystrokes in one frame produce one parse, not 5+. -->
          <MarkdownRenderer body={bodyForPreview} onWikilink={navigateWikilink} />
        {/if}
      {/snippet}
      <div class="flex-1 min-h-0 p-2 sm:p-3">
        {#if viewMode === 'edit'}
          <Editor bind:value={body} bind:this={editor} onSave={save} onNavigate={navigateWikilink} onExtract={extractCtl.handleExtract} onCursor={(c) => { cursorLine = c.line; cursorCol = c.col; cursorSelLen = c.selLen; }} onScroll={(s) => { const denom = Math.max(1, s.height - s.viewport); readProgress = Math.max(0, Math.min(1, s.top / denom)); }} extraExtensions={editorAIExtensions} />
        {:else if viewMode === 'preview'}
          <div class="h-full overflow-y-auto bg-surface0 border border-surface1 rounded px-4 sm:px-6 py-4" bind:this={previewContainer}>
            <div class="max-w-3xl mx-auto">
              {#if note}
                <NoteSummaryCard
                  notePath={note.path}
                  title={note.title || note.path}
                  body={bodyForPreview}
                  frontmatter={(note.frontmatter ?? {}) as Record<string, unknown>}
                  onSaveFrontmatter={saveFrontmatter}
                  onPrepend={(text) => { body = text + body; dirty = true; }}
                />
              {/if}
              {@render previewBody()}
            </div>
          </div>
        {:else}
          <!-- split (desktop only) -->
          <div class="h-full grid grid-cols-1 lg:grid-cols-2 gap-2">
            <Editor bind:value={body} bind:this={editor} onSave={save} onNavigate={navigateWikilink} onExtract={extractCtl.handleExtract} onCursor={(c) => { cursorLine = c.line; cursorCol = c.col; cursorSelLen = c.selLen; }} onScroll={(s) => { const denom = Math.max(1, s.height - s.viewport); readProgress = Math.max(0, Math.min(1, s.top / denom)); }} extraExtensions={editorAIExtensions} />
            <div class="h-full overflow-y-auto bg-surface0 border border-surface1 rounded px-4 sm:px-6 py-4 hidden lg:block" bind:this={previewContainer}>
              {@render previewBody()}
            </div>
          </div>
        {/if}
      </div>
      <!-- Status bar — always visible (mobile + desktop). The
           previous version was md:hidden, which left desktop users
           with no live word/char/line/cursor readout. The desktop
           layout fits more datapoints; mobile collapses to the
           essentials. Extracted to <NoteStatusBar> 2026-05-28. -->
      <NoteStatusBar
        {wordCount}
        {charCount}
        {lineCount}
        {readingMinutes}
        {wordGoal}
        {wordGoalPct}
        {cursorLine}
        {cursorCol}
        {cursorSelLen}
        {viewMode}
        {lastSavedAt}
        {lastSavedDisplay}
      />
    {:else}
      <div class="p-6 text-sm text-dim">loading…</div>
    {/if}
  </div>

  <!-- Right info panel — gated on viewport. Previously BOTH the
       desktop aside AND the drawer rendered the same `infoContent`
       snippet, with CSS hiding one. Each panel inside that snippet
       therefore mounted twice and ran its $derived/$effect work
       twice on every keystroke. Now the snippet renders to exactly
       one of them based on the live `isXl` flag (matchMedia listener
       set on mount). Saves ~half the per-keystroke recompute when
       editing on any non-xl viewport, and keeps the desktop layout
       unchanged when isXl is true. Focus-mode still hides the rail. -->
  {#if isXl}
    <aside class="hidden xl:flex xl:flex-col xl:w-72 border-l border-surface1 bg-mantle flex-shrink-0 focus-hide">
      {@render infoContent()}
    </aside>
  {:else}
    <Drawer bind:open={infoDrawerOpen} side="right" responsive width="w-80 sm:w-96">
      {@render infoContent()}
    </Drawer>
  {/if}
</div>

<!-- Extract-to-note dialog. Lives at the page root so it overlays
     above the editor + sidebars on every viewport size. The
     ExtractRequest is null when no extraction is in flight, so the
     dialog renders nothing. -->
<ExtractToNoteDialog
  request={extractCtl.request}
  sourcePath={note?.path ?? ''}
  onConfirm={extractCtl.confirm}
  onDismiss={extractCtl.dismiss}
/>

<!-- Print preview overlay. Renders nothing while closed; mounted at
     the page root so its `body > *:not(.print-overlay)` print rule
     reliably hides everything else. -->
{#if note}
  <PrintPreview
    bind:open={printOpen}
    title={note.title || note.path}
    body={bodyForPreview}
    sourcePath={note.path}
    onClose={() => (printOpen = false)}
  />
{/if}

<!-- Version history overlay. Restore returns the body of the chosen
     snapshot; we set `body` so the editor reflects it immediately,
     mark dirty so the next autosave persists the restored content,
     and let the panel's own loadVersions() refresh the list (the
     pre-restore content was itself snapshotted server-side). -->
{#if note}
  <HistoryPanel
    bind:open={historyOpen}
    notePath={note.path}
    currentBody={bodyForPreview}
    onRestore={(restoredBody: string) => {
      body = restoredBody;
      dirty = true;
    }}
  />
{/if}

<!-- Keyboard cheat sheet. Triggered by "?" anywhere outside an
     editable surface, or via the toolbar help button. -->
<ShortcutsHelpOverlay
  bind:open={helpOpen}
  onClose={() => (helpOpen = false)}
/>

<!-- Slideshow / presentation mode — fullscreen deck view. Mounted
     at the page root so it overlays sidebars; component renders
     nothing while closed. -->
{#if note}
  <NotePresentation
    body={bodyForPreview}
    title={note.title || note.path}
    open={presentationOpen}
    onClose={() => (presentationOpen = false)}
  />
{/if}

<!-- Floating selection toolbar — appears above any text selection
     inside the editor. The chord-dispatch path means buttons take
     the same code route as the keyboard shortcuts (single source
     of truth: the keymap). Hidden on mobile via CSS and on print
     surfaces. -->
<SelectionToolbar
  container={editorDOM}
  onCommand={(chord) => editor?.dispatchChord(chord)}
/>

<!-- Mobile-only floating formatting bar — anchored above the on-screen
     keyboard while the editor is focused. Dispatches through the same
     chord path as desktop shortcuts (single source of truth: the
     keymap); also exposes literal-insert buttons for the highest-value
     mobile snippets (checkbox / wikilink / tag) that don't map to a
     keymap chord. Self-hides off-mobile via md:hidden inside the
     component. -->
<MobileEditorToolbar
  contentDOM={editorDOM}
  onCommand={(chord) => editor?.dispatchChord(chord)}
  onInsert={(text) => editor?.insertAtCursor(text)}
/>

<!-- Floating action bar that follows the inline-AI ghost. During
     streaming it offers a Stop button; after completion it offers
     Keep / Try again / Discard. Keyboard chords (Tab / ⌘R / Esc)
     still work — the bar is the click-discoverable surface for the
     same actions. -->
<AIActionBar view={editor?.getView?.()} aiState={aiGhostState} />

<!-- Inline AI menu — Notion-style command palette anchored at the
     cursor. Opens on Cmd-/ or when the user types "/ai" at the start
     of a line. Streams output as ghost text in the editor; the user
     accepts/rejects/regenerates without ever leaving the document. -->
{#if aiTriggerEvent && note}
  <InlineAIMenu
    event={aiTriggerEvent}
    notePath={note.path}
    body={bodyForPreview}
    onClose={() => (aiTriggerEvent = null)}
  />
{/if}

<!-- Overflow menu popover — secondary header actions (find,
     history, PDF, slideshow, audio, reading mode, focus mode,
     flashcards, keyboard shortcuts). Rendered with position:
     fixed and viewport-clamped coordinates so it escapes any
     ancestor overflow and never lands off-screen. Self-contained
     since the 2026-05-28 extraction. -->
{#if note}
  <NoteOverflowMenu
    bind:open={overflowOpen}
    triggerEl={overflowTriggerEl}
    {audioOpen}
    {readingMode}
    {focusMode}
    {schedulingFlashcards}
    onOpenFind={() => editor?.openFind()}
    onOpenHistory={() => (historyOpen = true)}
    onOpenPrint={() => (printOpen = true)}
    onOpenPresentation={() => (presentationOpen = true)}
    onToggleAudio={() => (audioOpen = !audioOpen)}
    onToggleReadingMode={viewModes.toggleReadingMode}
    onToggleFocusMode={viewModes.toggleFocusMode}
    onScheduleFlashcards={runScheduleFlashcards}
    onOpenHelp={() => (helpOpen = true)}
  />
{/if}

<style>
  /* save-flash animation lives in $lib/notes/NoteHeader.svelte alongside
     the button that wears it — Svelte 5 scopes class selectors to the
     declaring component, so the rule has to follow the element. */
  /* Focus mode: hide the side asides (tree on the left, info on the
     right) so the editor pane fills the available width. The header
     and footer stay — they're tightly bound to the editing flow
     (save state, word count, daily-nav buttons). */
  .focus-mode .focus-hide {
    display: none !important;
  }
  /* Touch tap-target floor — on coarse-pointer devices (phones /
     tablets without a precise mouse) every header button needs at
     least 40px tall so the interactive surface meets the WCAG
     2.5.5 minimum-target guideline (44×44 ideal, 40×40 acceptable
     for inline toolbar density). Desktop with a fine pointer keeps
     the denser 28-32px sizes the toolbar was designed around. */
  @media (pointer: coarse) {
    :global(header button),
    :global(header a) {
      min-height: 2.5rem;
    }
  }
  /* Reading mode: serif typography + narrower max-width on the
     preview pane so the user lands in a "I'm reading a book"
     posture, not a "I'm editing in a text box" one. We compose with
     focus-mode's hidden asides so reading mode reads as a single
     centered column. The :global selectors are needed because
     Svelte's scoped CSS would otherwise miss the MarkdownRenderer's
     internal elements. */
  .reading-mode :global(.prose-note),
  .reading-mode :global([class*="MarkdownRenderer"]) {
    font-family: 'Source Serif 4', 'Iowan Old Style', 'Georgia', serif;
  }
  .reading-mode :global(.prose-note) {
    max-width: 64ch;
    margin-left: auto;
    margin-right: auto;
    font-size: 1.05rem;
    line-height: 1.7;
  }
  .reading-mode :global(.prose-note h1) { font-size: 1.7rem; line-height: 1.25; }
  .reading-mode :global(.prose-note h2) { font-size: 1.35rem; line-height: 1.3; margin-top: 1.6em; }
  .reading-mode :global(.prose-note h3) { font-size: 1.15rem; margin-top: 1.4em; }
  .reading-mode :global(.prose-note p) { margin: 0.85em 0; }
  .reading-mode :global(.prose-note blockquote) {
    font-style: italic;
    border-left-width: 3px;
    padding-left: 1.1em;
  }
</style>
