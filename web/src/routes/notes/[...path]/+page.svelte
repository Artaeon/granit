<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { beforeNavigate } from '$app/navigation';
  import { page } from '$app/stores';
  import { api, type Note } from '$lib/api';
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
  import { createNoteSaveStatusCtl } from '$lib/notes/noteSaveStatusCtl.svelte';
  import { createNoteVersionCount } from '$lib/notes/noteVersionCount.svelte';
  import { createNotePipelineState } from '$lib/notes/notePipelineState.svelte';
  import ExtractToNoteDialog from '$lib/notes/ExtractToNoteDialog.svelte';
  import { createExtractController } from '$lib/notes/extractToNote.svelte';
  import { navigateWikilink as navigateWikilinkHelper } from '$lib/notes/wikilinkNav';
  import { parseTagsField, addSuggestedTag, insertSuggestedLink } from '$lib/notes/frontmatterTagOps';
  import { saveFrontmatter as saveFrontmatterFn } from '$lib/notes/saveFrontmatter';
  import { saveNote as saveNoteFn } from '$lib/notes/saveNote';
  import { loadNote as loadNoteFn } from '$lib/notes/loadNote';
  import { installNoteAutosave } from '$lib/notes/noteAutosave.svelte';
  import { createViewModeController } from '$lib/notes/viewModes.svelte';
  import { createViewportBreakpoints } from '$lib/notes/viewportBreakpoints.svelte';
  import { createPreviewBodyMirror } from '$lib/notes/previewBodyMirror.svelte';
  import { createNoteWordStats } from '$lib/notes/noteWordStats.svelte';
  import { shiftDate } from '$lib/notes/dailyNote';
  import { createDailyNoteNav } from '$lib/notes/dailyNoteNav.svelte';
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
  import NoteEditorBanners from '$lib/notes/NoteEditorBanners.svelte';
  import { ensurePinnedLoaded } from '$lib/notes/pinnedNotes';
  import { createNotePinAction } from '$lib/notes/notePinAction.svelte';
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

  // Viewport tracking for the rail/tree mount strategy. Lives in
  // $lib/notes/viewportBreakpoints — see there for the full story
  // on why we mount each rail to ONLY one location at a time. The
  // page reads `isLg` / `isXl` via reactive getter aliases and lets
  // onMount install the live MQL listeners.
  const breakpoints = createViewportBreakpoints();
  let isLg = $derived(breakpoints.isLg);
  let isXl = $derived(breakpoints.isXl);
  onMount(() => {
    // Boot the pinned-notes store so the toolbar's pin star (and any
    // other pin-aware surface mounted after this) reflects the
    // server-authoritative list without each component re-fetching.
    ensurePinnedLoaded();
    return breakpoints.install();
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

  // The 17 $state slots the save / load / autosave / wsReload /
  // frontmatter helpers all touch live in notePipelineState — see
  // there for the SaveState / SaveFrontmatterState contract. The
  // controller IS the proxy; the route reads via `pipe.X` getters
  // (re-aliased to local names below so existing template + script
  // references stay), writes via `pipe.X = …`, and the helpers
  // accept `pipe` as their state argument.
  const pipe = createNotePipelineState();
  let note = $derived(pipe.note);
  let body = $derived(pipe.body);
  // Adaptive rAF / debounce mirror of `body` that drives the
  // MarkdownRenderer, status-bar counters, summary card, etc. Lives
  // in $lib/notes/previewBodyMirror — see there for the tier table
  // and the first-paint fast path. The page reads bodyForPreview via
  // a $derived alias and schedules from a $effect that tracks body;
  // the controller owns the rAF + timer lifecycle.
  const previewMirror = createPreviewBodyMirror();
  let bodyForPreview = $derived(previewMirror.bodyForPreview);
  $effect(() => {
    previewMirror.schedule(body);
    // Note: NO cleanup return here. $effect cleanup fires on every
    // dep change, not just unmount — cancelling the pending rAF on
    // each body keystroke would defeat the coalescer (every
    // keystroke would cancel + reschedule instead of riding the
    // already-queued frame). Unmount cancellation is handled
    // separately via onDestroy below so the pending frame is
    // killed exactly once, when the component goes away.
  });
  onDestroy(() => previewMirror.destroy());
  let saving = $derived(pipe.saving);
  let dirty = $derived(pipe.dirty);
  let error = $derived(pipe.error);
  let notFound = $derived(pipe.notFound);
  let creatingNote = $state(false);

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

  // Pin / unpin lives in notePinAction — the controller subscribes
  // to the shared pinnedNotes store + owns the busy flag. The page
  // calls `pinAction.togglePin()` from the header.
  const pinAction = createNotePinAction({ getNote: () => note });
  let pinned = $derived(pinAction.pinned);
  let pinBusy = $derived(pinAction.pinBusy);
  const togglePin = pinAction.togglePin;

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

  let draftRestored = $derived(pipe.draftRestored);

  // Load now lives in $lib/notes/loadNote — see there for the
  // draft-reconciliation contract, the "always prefer the draft"
  // rule on divergence, and the 404 / network-error fallbacks.
  async function load(p: string, opts: { force?: boolean } = {}) {
    return loadNoteFn(p, opts, pipe, {
      getLiveBody: () => editor?.getContent?.() ?? pipe.body,
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
      pipe.notFound = false;
      pipe.lastLoadedPath = '';
      await load(cleanPath, { force: true });
    } catch (e) {
      toast.error(`Couldn't create note: ${e instanceof Error ? e.message : String(e)}`);
    } finally {
      creatingNote = false;
    }
  }

  let lastSavedAt = $derived(pipe.lastSavedAt);
  let pendingFrontmatter = $derived(pipe.pendingFrontmatter);
  let conflictDetected = $derived(pipe.conflictDetected);
  let saveFailed = $derived(pipe.saveFailed);
  let saveFailCount = $derived(pipe.saveFailCount);
  let lastSaveError = $derived(pipe.lastSaveError);

  // Save-status presentation surface (nowTick + saveStatus +
  // lastSavedDisplay + saveFlash) lives in noteSaveStatusCtl. The
  // page reads each via a $derived alias and runs install() inside
  // a single $effect for lifecycle binding.
  const saveStatusCtl = createNoteSaveStatusCtl({
    getSaving: () => saving,
    getDirty: () => dirty,
    getSaveFailed: () => saveFailed,
    getLastSavedAt: () => lastSavedAt
  });
  let saveFlash = $derived(saveStatusCtl.saveFlash);
  $effect(() => saveStatusCtl.install());

  // Body+frontmatter save now lives in $lib/notes/saveNote — see
  // there for the conflict / draft / surgical-mutation contract.
  // Thin wrapper just plumbs in the shared pipe proxy and clears
  // the draftRestored badge on success.
  async function save(opts: { silent?: boolean } = {}): Promise<boolean> {
    if (pipe.saving) return !pipe.dirty;
    return saveNoteFn(opts, pipe, saveCtx, () => { pipe.draftRestored = false; });
  }

  const saveCtx = { getLiveBody: () => editor?.getContent?.() ?? pipe.body };

  // Install dirty + autosave + draft-rAF + tab-hide flush + online
  // retry — six effects worth of plumbing live in noteAutosave so
  // this surface keeps the deps wiring at one glance.
  installNoteAutosave({
    getNote: () => pipe.note,
    getBody: () => pipe.body,
    getLiveBody: () => editor?.getContent?.() ?? pipe.body,
    getDirty: () => pipe.dirty,
    getSaving: () => pipe.saving,
    getSaveFailed: () => pipe.saveFailed,
    getConflictDetected: () => pipe.conflictDetected,
    getPrev: () => pipe.prev,
    setDirty: (v) => { pipe.dirty = v; },
    setPrev: (v) => { pipe.prev = v; },
    getLastDraftedBody: () => pipe.lastDraftedBody,
    setLastDraftedBody: (v) => { pipe.lastDraftedBody = v; },
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

  let saveStatus = $derived(saveStatusCtl.saveStatus);

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

  // Snapshot-count fetcher lives in noteVersionCount — refreshes on
  // note swap and every save (modTime tick), with a generation guard
  // so stale fetches can't write into the current note's chip.
  const versionCountCtl = createNoteVersionCount({ getNote: () => note });
  let versionCount = $derived(versionCountCtl.versionCount);
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
      getActivePath: () => pipe.note?.path ?? null,
      getLiveBody: () => editor?.getContent?.() ?? pipe.body,
      getSavedBody: () => pipe.prev,
      isSaving: () => pipe.saving,
      getLastSavedAt: () => pipe.lastSavedAt,
      reload: (p) => void load(p, { force: true }),
      ownSaveQuietMs: OWN_SAVE_QUIET_MS,
      coalesceMs: WS_RELOAD_COALESCE_MS
    })
  );

  // Status-bar counters + word goal live in $lib/notes/noteWordStats —
  // see there for the rationale on tying everything to bodyForPreview
  // (rAF-coalesced) rather than the raw body. The page reads each
  // metric via a $derived alias.
  const wordStats = createNoteWordStats({
    getBodyForPreview: () => bodyForPreview,
    getNote: () => note
  });
  let wordCount = $derived(wordStats.wordCount);
  let charCount = $derived(wordStats.charCount);
  let lineCount = $derived(wordStats.lineCount);
  let readingMinutes = $derived(wordStats.readingMinutes);
  let wordGoal = $derived(wordStats.wordGoal);
  let wordGoalPct = $derived(wordStats.wordGoalPct);

  // Cursor position state — populated by the Editor's onCursor
  // callback. line:col is 1-indexed (matches what every editor
  // status bar shows). selLen > 0 means the user has a selection;
  // we surface a "{N} selected" badge in that case so the user
  // knows how much they're about to act on.
  let cursorLine = $state(1);
  let cursorCol = $state(1);
  let cursorSelLen = $state(0);

  let lastSavedDisplay = $derived(saveStatusCtl.lastSavedDisplay);

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
  // The page just plumbs its reactive state via the pipe proxy above.
  async function saveFrontmatter(next: Record<string, unknown>): Promise<boolean> {
    return saveFrontmatterFn(next, pipe, saveCtx);
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
        pipe.body = pipe.body + (pipe.body.endsWith('\n') ? '' : '\n') + m + '\n';
        pipe.dirty = true;
      }
    });
  }

  // ----- Daily-note navigation -----
  // Detection + date math + gotoDaily live in $lib/notes/dailyNoteNav.
  // The page reads dailyDate / isDaily / dayActivitySegments /
  // dailyLabel via $derived aliases and calls dailyNav.gotoDaily()
  // from the daily-quick-add + the Mod-Shift-←/→ shortcut.
  const dailyNav = createDailyNoteNav({
    getNote: () => note,
    getBodyForPreview: () => bodyForPreview,
    getDirty: () => dirty,
    save: (o) => save(o)
  });
  let dailyDate = $derived(dailyNav.dailyDate);
  let isDaily = $derived(dailyNav.isDaily);
  let dayActivitySegments = $derived(dailyNav.dayActivitySegments);
  const gotoDaily = dailyNav.gotoDaily;

  // Folder breadcrumbs. Reset on real navigation is folded into
  // load() since it's the only path that swaps note identity; the
  // pure derivation + collapse rule lives in noteBreadcrumbs.
  let breadcrumbExpanded = $state(false);
  let allCrumbs = $derived(noteCrumbs(note?.path));
  let visibleCrumbs = $derived(visibleCrumbsFn(allCrumbs, breadcrumbExpanded));
  let crumbsCollapsed = $derived(crumbsCollapsedFn(allCrumbs, breadcrumbExpanded));

  let dailyLabel = $derived(dailyNav.dailyLabel);
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
          onclick={() => { pipe.lastLoadedPath = ''; load(decodeURIComponent($page.params.path ?? '')); }}
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
        <DailyContext onChanged={async () => { pipe.lastLoadedPath = ''; await load(np); }} />
        <DailyQuickAdd notePath={np} dailyDate={dailyDate} onAdded={async () => { pipe.lastLoadedPath = ''; await load(np); }} />
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
      <!-- Save-related banners (draft-restored / conflict / autosave-
           failing) extracted to <NoteEditorBanners>. Each renders
           nothing on a clean state; the conflict banner gates on its
           own derived flag, so unrelated failures can't hide it. -->
      <NoteEditorBanners
        {draftRestored}
        {conflictDetected}
        {saveFailCount}
        {lastSaveError}
        {saving}
        onDismissDraftBadge={() => (pipe.draftRestored = false)}
        onReload={() => { pipe.lastLoadedPath = ''; void load(note!.path, { force: true }); }}
        onOverwrite={() => {
          pipe.forceNextSave = true;
          // Route Overwrite back through the SAME save shape that
          // hit 412. If the conflict was a tag-chip / frontmatter
          // change, replaying through save() (body) would drop the
          // user's intended edit and stomp the server's body too;
          // we re-run saveFrontmatter with the held next-payload
          // and forceNextSave so the originally-attempted change is
          // the one that lands.
          if (pendingFrontmatter) {
            const next = pendingFrontmatter;
            void saveFrontmatter(next);
          } else {
            void save({ silent: false });
          }
        }}
        onRetry={() => save({ silent: false })}
      />
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
          <Editor bind:value={pipe.body} bind:this={editor} onSave={save} onNavigate={navigateWikilink} onExtract={extractCtl.handleExtract} onCursor={(c) => { cursorLine = c.line; cursorCol = c.col; cursorSelLen = c.selLen; }} onScroll={(s) => { const denom = Math.max(1, s.height - s.viewport); readProgress = Math.max(0, Math.min(1, s.top / denom)); }} extraExtensions={editorAIExtensions} />
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
                  onPrepend={(text) => { pipe.body = text + pipe.body; pipe.dirty = true; }}
                />
              {/if}
              {@render previewBody()}
            </div>
          </div>
        {:else}
          <!-- split (desktop only) -->
          <div class="h-full grid grid-cols-1 lg:grid-cols-2 gap-2">
            <Editor bind:value={pipe.body} bind:this={editor} onSave={save} onNavigate={navigateWikilink} onExtract={extractCtl.handleExtract} onCursor={(c) => { cursorLine = c.line; cursorCol = c.col; cursorSelLen = c.selLen; }} onScroll={(s) => { const denom = Math.max(1, s.height - s.viewport); readProgress = Math.max(0, Math.min(1, s.top / denom)); }} extraExtensions={editorAIExtensions} />
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
      pipe.body = restoredBody;
      pipe.dirty = true;
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
