<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { goto, beforeNavigate } from '$app/navigation';
  import { page } from '$app/stores';
  import { api, ApiError, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
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
  import { getDraft, setDraft, clearDraft, draftDivergesFromServer } from '$lib/notes/drafts';
  import { rememberScroll, recallScroll } from '$lib/notes/noteHistory';
  import { createPreviewScrollTracker } from '$lib/notes/previewScrollTracker.svelte';
  import { installNoteShortcuts } from '$lib/notes/noteKeyboardShortcuts.svelte';
  import { openAIOverlay, aiOverlayPinned } from '$lib/stores/ai-overlay';
  import ExtractToNoteDialog from '$lib/notes/ExtractToNoteDialog.svelte';
  import { createExtractController } from '$lib/notes/extractToNote.svelte';
  import { offerAIChapterGeneration } from '$lib/notes/aiChapterGeneration';
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
  import { acceptInlineAI, inlineAIObserver, rejectInlineAI, type InlineAIState } from '$lib/editor/inline-ai';
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
  // Cap on the body excerpt seeded into the AI overlay's Research
  // Mode context. Bigger excerpts pay for themselves at the model
  // tier but cost overlay context budget elsewhere.
  const RESEARCH_EXCERPT_MAX = 800;

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
    if (draftWriteRaf) {
      cancelAnimationFrame(draftWriteRaf);
      draftWriteRaf = 0;
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

  async function load(p: string, opts: { force?: boolean } = {}) {
    error = '';
    notFound = false;
    draftRestored = false;
    if (!opts.force && lastLoadedPath === p) return;
    // Clear the stale-note concurrency token up-front. If the fetch
    // below throws (network blip, 5xx, abort), the previous note's
    // etag would otherwise linger on the freshly-navigated path and
    // get sent as If-Match on the next save → spurious 412 on a note
    // that had no real conflict. A successful load refills it from
    // the response a few lines down.
    noteEtag = null;
    // Same-note reloads (WS-triggered note.changed) must not clobber
    // in-flight typing. Snapshot the body before the await; if the user
    // types during the fetch, abort the body overwrite and let the
    // auto-save effect persist their edits. For navigation to a
    // different note (note?.path !== p), we always want to overwrite.
    const isSameNoteReload = note?.path === p;
    // Reset breadcrumb-expand only on real navigation. A same-note
    // WS reload (sync from another device, metadata reindex) must
    // not collapse a deep-path breadcrumb the user expanded by hand.
    if (!isSameNoteReload) breadcrumbExpanded = false;
    // Cancel any in-flight inline-AI stream on real navigation. The
    // streaming controller in inline-ai.ts is module-scope; without
    // this kill, a stream started on note A keeps dispatching ghost
    // tokens into the editor view after it's been retargeted to
    // note B, and the tokens land at the original anchor offset in
    // an unrelated doc. Same-note reloads keep the ghost so an
    // unrelated WS rescan can't yank it from under the user.
    if (!isSameNoteReload) {
      const view = editor?.getView?.();
      if (view) rejectInlineAI(view);
    }
    // Reset the per-load draft watermark so the first keystroke on the
    // newly-opened note triggers a draft write. Without this, opening a
    // note whose body happens to equal the previous note's last drafted
    // body would skip the very first draft persistence.
    lastDraftedBody = null;
    // Snapshot from the EDITOR, not the bound `body` mirror. The
    // bind:value write from CodeMirror's updateListener is microtask-
    // deferred; on a slow render frame the parent's `body` can lag
    // the actual doc by 10s of ms. If a WS reload fires inside that
    // lag window we'd false-positive "no in-flight edits", replace
    // the editor with the server's older body, and silently discard
    // every keystroke the user added after the last autosave.
    // editor.getContent() reads CodeMirror's state.doc directly,
    // which is updated synchronously inside dispatch().
    const liveAtStart = editor?.getContent?.() ?? body;
    lastLoadedPath = p;
    try {
      const { data: fresh, etag: freshEtag } = await api.getNoteWithEtag(p);
      if (isSameNoteReload) {
        const liveNow = editor?.getContent?.() ?? body;
        if (liveNow !== liveAtStart) return;
      }
      // Anchor the optimistic-concurrency token. save() sends this back
      // as `If-Match`; a foreign edit since this load surfaces as 412.
      noteEtag = freshEtag;
      forceNextSave = false;
      const serverBody = fresh.body ?? '';

      // Restore a local draft if it diverges from the server. We ALWAYS
      // prefer the draft when it has unsaved typing, even when the
      // server's modTime is newer — losing the user's work silently is
      // worse than the rare case of overwriting a TUI/other-device edit.
      // The most common reason the server is "newer" while a draft
      // diverges is the user typing during the autosave (the draft was
      // written with the pre-save modTime, then save bumped the server's
      // modTime; the draft's body has the keystrokes that came in after
      // the save fired). Discarding it is exactly the wrong move.
      //
      // We still warn the user when the modTime says they may be
      // working from a stale base, so they can manually reconcile if
      // they actually have a multi-device conflict (the rare case).
      const draft = getDraft(p);
      if (draft && draftDivergesFromServer(draft, serverBody)) {
        const serverNewer = new Date(fresh.modTime) > new Date(draft.baseModTime);
        prev = draft.body;
        body = draft.body;
        note = fresh;
        dirty = true;
        draftRestored = true;
        treeDrawerOpen = false;
        infoDrawerOpen = false;
        if (serverNewer) {
          toast.warning('Restored unsaved draft — server moved forward since your last edit. Your version will overwrite on next save.');
        } else {
          toast.info('Restored unsaved draft');
        }
        save({ silent: true });
        return;
      } else if (draft) {
        // Draft matches server — stale, clean up.
        clearDraft(p);
        lastDraftedBody = null;
      }

      note = fresh;
      body = serverBody;
      prev = body;
      dirty = false;
      // A successful load is the canonical "we are anchored to the
      // server again" event — reset every error/conflict flag so a
      // stale 412 from a previous version can't keep conflictDetected
      // sticky and silently disable the autosave loop. The conflict
      // banner's "Reload server version" button reaches this branch;
      // without these resets, the user types after reload and
      // nothing saves (the trySave bail-on-conflictDetected guard
      // would short-circuit forever).
      saveFailed = false;
      saveFailCount = 0;
      lastSaveError = '';
      pendingFrontmatter = null;
      treeDrawerOpen = false;
      infoDrawerOpen = false;
      // Restore the scroll position (per-note, pixel-accurate). Defer
      // a frame so the editor has finished mounting and the scroller
      // has its content height — without the defer the setScrollTop
      // call lands at 0 because the doc just got swapped.
      const remembered = recallScroll(p);
      if (remembered > 0) {
        requestAnimationFrame(() => {
          editor?.setScrollTop?.(remembered);
        });
      }
      // ?line=<n> — incoming jump from /search. Wins over remembered
      // scroll position so a user clicking a search hit lands on the
      // matched line, not yesterday's reading position. We let the
      // editor mount fully before dispatching the scroll.
      const lineParam = $page.url.searchParams.get('line');
      if (lineParam) {
        const ln = parseInt(lineParam, 10);
        if (Number.isFinite(ln) && ln > 0) {
          requestAnimationFrame(() => editor?.scrollToLine?.(ln));
        }
      }
      // Block-level wikilink target — when arriving via [[Note#H]] the
      // url hash carries the heading text. Scroll to the matching
      // line, overriding any remembered scroll position. Only fires
      // when the hash is non-empty so the regular reopen flow keeps
      // its remembered position. Heading-match is case-insensitive
      // and whitespace-collapsed so "  Plan  " in the hash still
      // matches "## Plan" in the body.
      const rawHash = $page.url.hash ? decodeURIComponent($page.url.hash.slice(1)) : '';
      if (rawHash) {
        const target = rawHash.toLowerCase().replace(/\s+/g, ' ').trim();
        const lines = (body ?? '').split('\n');
        let found = -1;
        let inFence = false;
        for (let i = 0; i < lines.length; i++) {
          const t = lines[i].trim();
          if (t.startsWith('```') || t.startsWith('~~~')) { inFence = !inFence; continue; }
          if (inFence) continue;
          const m = /^(#{1,6})\s+(.+?)\s*$/.exec(t);
          if (m && m[2].toLowerCase().replace(/\s+/g, ' ').trim() === target) {
            found = i + 1; // CodeMirror is 1-based
            break;
          }
        }
        if (found > 0) {
          requestAnimationFrame(() => editor?.scrollToLine?.(found));
        }
      }
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      // If we have a local draft, surface it instead of an error so an
      // offline reload doesn't lose work.
      const draft = getDraft(p);
      if (draft) {
        prev = draft.body;
        body = draft.body;
        note = {
          path: p,
          title: p.split('/').pop()!.replace(/\.md$/, ''),
          modTime: new Date().toISOString(),
          size: draft.body.length,
          frontmatter: {},
          body: draft.body
        } as Note;
        dirty = true;
        draftRestored = true;
        toast.warning('offline — showing your local draft');
        return;
      }
      // 404 from getNote means the note path doesn't exist yet —
      // almost always the user following an unresolved wikilink or
      // navigating to a path they're about to create. Surface a
      // create-affordance instead of an error banner.
      if (e instanceof ApiError && e.status === 404) {
        notFound = true;
      } else {
        error = msg;
      }
      note = null;
      body = '';
      prev = '';
      dirty = false;
      // Critical: drop the dedupe guard so a refetch of the SAME path
      // is allowed. Without this, the user lands on a 404/network-error
      // note, the page renders the error banner, and any subsequent
      // navigation back to that URL (browser back, retry click,
      // sidebar re-click) silently no-ops because `lastLoadedPath ===
      // p` returns early. The user concludes the page is frozen and
      // hits reload.
      lastLoadedPath = '';
    }
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

  async function save(opts: { silent?: boolean } = {}): Promise<boolean> {
    if (!note || !dirty || saving) return !dirty;
    saving = true;
    error = '';
    // Capture the body at the start of the save. If the user types
    // during the await, body will diverge from sentBody — we must NOT
    // mark the editor clean in that case, or those keystrokes are lost
    // forever (server only got sentBody, prev=body would mask the gap,
    // and the next typing wouldn't trigger a fresh save). Compare body
    // to sentBody after the await to decide whether more work remains.
    const sentBody = body;
    // Capture the note we started saving so we can detect navigation
    // mid-await. With the surgical-mutation strategy, mutating after
    // the user has navigated to another note would silently corrupt
    // the new note's modTime/size/links/title with values from the
    // old note's save response. Identity check on the proxy survives
    // intermediate property mutations and only fails on reassignment.
    const savedNote = note;
    // Optimistic-concurrency: send the etag captured on load (or the
    // last successful save). The backend 412s if the file on disk has
    // moved forward since — we surface that as a toast and leave the
    // note dirty so no work is lost. `forceNextSave` is the user's
    // "overwrite anyway" opt-in after they've seen the conflict.
    const etagToSend = forceNextSave ? undefined : (noteEtag ?? undefined);
    try {
      const { data: updated, etag: newEtag } = await api.putNoteWithEtag(
        note.path,
        { frontmatter: note.frontmatter as Record<string, unknown>, body: sentBody },
        etagToSend
      );
      // Navigation guard: if the user moved to another note while we
      // were awaiting, the server-side save still succeeded for the
      // original note — we just stop applying its response to the
      // active state. The localStorage draft was already cleared
      // when we entered save() with prev=sentBody (next pass), and
      // a fresh load() ran for the new path.
      if (!note || note !== savedNote) {
        return true;
      }
      // Refresh the concurrency token from the response so the next
      // save is again guarded against foreign edits between now and
      // then. Clear the force-flag (it was a one-shot opt-in) and
      // any pending frontmatter — the body save is the canonical
      // "we are anchored again" event and any pending tag/field
      // change should be re-toggled by the user against the new
      // base rather than blindly re-fired against stale state.
      noteEtag = newEtag;
      forceNextSave = false;
      pendingFrontmatter = null;
      // ─────────────────────────────────────────────────────────────────
      // CRITICAL: surgical property mutation instead of `note = updated`.
      //
      // The previous code reassigned the whole `note` object, which
      // invalidated every reactive consumer of `note` even when only
      // the modTime changed. The notes view passes `note.path` to
      // ~10 panels (AskThisNotePanel, SectionQuestionsPanel,
      // LocalGraph, BacklinksPanel, ReferenceNotePanel, LinkSuggestPanel,
      // AnnotationsPanel, etc.) — each of those re-evaluated its props
      // and re-ran its own $effects, which in some panels fire API
      // calls (api.listBacklinks, api.getNote for the reference, ...).
      // A single autosave fanned out into a wave of work that jammed
      // Svelte's microtask scheduler for hundreds of ms — long enough
      // that clicks queued behind it never dispatched (the user-
      // visible "after autosave everything freezes I can't click
      // anywhere" symptom).
      //
      // Svelte 5 wraps $state objects in a Proxy with per-property
      // reactivity. Mutating just `modTime`/`size`/`links`/`title`
      // (the fields that actually changed during the save) invalidates
      // only the effects that read those specific fields, not every
      // effect that reads `note.path` (the most common case).
      //
      // Path, frontmatter, and body are byte-for-byte identical to
      // what we sent — leaving them alone keeps panel identity stable
      // through autosave.
      note.modTime = updated.modTime;
      note.size = updated.size;
      note.links = updated.links;
      note.title = updated.title;
      prev = sentBody;
      // Trust the editor's view state over the Svelte-bound `body`.
      // Bind-prop writes from CodeMirror's updateListener back to
      // `value` are microtask-deferred; during a heavy reactive
      // cascade they may not have landed by the time we check here,
      // making the cheap `body !== sentBody` check produce a false
      // "clean" result while the user has in-flight keystrokes. The
      // editor's doc is updated synchronously inside CodeMirror's
      // dispatch — reading from it always returns the truth.
      const liveNow = editor?.getContent?.() ?? body;
      dirty = liveNow !== sentBody;
      lastSavedAt = Date.now();
      saveFailed = false;
      saveFailCount = 0;
      lastSaveError = '';
      if (!dirty) {
        clearDraft(updated.path);
        lastDraftedBody = null;
      } else {
        // User typed during the save. The draft on disk still has
        // the OLD modTime as baseModTime, which would cause the
        // "server has newer content" branch to trip on a mid-edit
        // reload. Refresh the draft synchronously with the post-save
        // modTime so a crash / reload in the next 100ms (debounce
        // window) doesn't fall into that path. Use the live editor
        // content, not `body` — same lag concern as the dirty check.
        setDraft(updated.path, liveNow, updated.modTime);
      }
      draftRestored = false;
      if (!opts.silent && !dirty) toast.success('saved');
      return true;
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      // 412 Precondition Failed — the file on disk moved forward since
      // we loaded it. Leave the note dirty and the etag stale; the
      // banner-driven "Overwrite anyway" path sets forceNextSave=true
      // for one save, and "Reload server version" calls load(force:
      // true) which discards local changes for the server's. Don't
      // increment saveFailCount — this isn't a transient error, it's
      // a state mismatch that needs explicit user input.
      if (e instanceof ApiError && e.status === 412) {
        saveFailed = true;
        lastSaveError = 'Conflict: this note was changed elsewhere since you loaded it.';
        if (!opts.silent) {
          toast.warning('Conflict: this note was changed elsewhere. Choose Reload or Overwrite in the banner.');
        }
        return false;
      }
      error = msg;
      saveFailed = true;
      saveFailCount++;
      lastSaveError = msg;
      // Explicit save (Mod-S) → always toast.
      // Silent autosave → toast ONLY on the first failure of a burst
      // (saveFailCount transitioned 0 → 1 by the increment above).
      // Subsequent silent failures stay quiet — the sticky banner is
      // their surface — but a single transient failure that recovers
      // immediately would otherwise be invisible to the user.
      if (!opts.silent || saveFailCount === 1) {
        toast.error(`save failed: ${msg}`);
      }
      return false;
    } finally {
      saving = false;
    }
  }

  let prev = $state('');
  $effect(() => {
    if (body !== prev) {
      dirty = true;
      prev = body;
    }
  });

  // Auto-save: debounce 2s after last edit. If save fails, the next edit
  // re-triggers the timer so we keep retrying as the user continues typing.
  //
  // Hostile-UX guard: while the autocomplete picker is open (user mid-
  // snippet like /callout, mid-wikilink, or mid-tag), saving causes the
  // editor's doc to be re-set on the WS bounce-back, which closes the
  // picker and interrupts what the user was composing. We back off
  // every 1s in that case and only save once the picker is closed.
  // This pattern preserves a single in-flight timer ref (cleaned up
  // properly on effect re-run) instead of leaking re-scheduled timers.
  $effect(() => {
    void body;
    if (!dirty || saving || !note) return;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const trySave = () => {
      timer = null;
      if (!dirty || saving || !note) return;
      // Unresolved conflict — re-PUTting would just bounce off 412
      // again. Wait for the user to choose Reload or Overwrite via
      // the conflict banner.
      if (conflictDetected) return;
      if (editor?.isCompletionActive?.()) {
        // Picker open — back off and re-check in 1s.
        timer = setTimeout(trySave, 1000);
        return;
      }
      save({ silent: true });
    };
    timer = setTimeout(trySave, AUTOSAVE_DEBOUNCE_MS);
    return () => {
      if (timer) clearTimeout(timer);
    };
  });

  // Persist the body to localStorage with rAF coalescing. Prior
  // iterations: a 600ms debounce reset on every keystroke and never
  // fired during continuous typing, losing everything since the last
  // pause on a crash; then a fully-synchronous per-keystroke write
  // restored that guarantee but charged O(N) doc.toString + O(N)
  // JSON.stringify + a blocking localStorage.setItem per character,
  // which on a 100 KB note compounded into the editor-freeze the
  // user reported. The rAF path collapses N keystrokes per frame
  // into one write — at most 16 ms of typing is at risk on a hard
  // crash, and the pagehide / beforeunload flush below still
  // captures the latest body on tab-close exactly.
  //
  // Skip when the body hasn't actually changed since the last write.
  // The effect re-runs whenever `note` is reassigned (every successful
  // save creates a new note reference), and writing the same body to
  // localStorage in that case is wasted work — for a multi-MB note
  // the JSON.stringify + setItem can take 10ms+, which adds up when
  // a save bounces every 2s.
  let lastDraftedBody: string | null = null;
  let draftWriteRaf = 0;
  $effect(() => {
    void body;
    if (!note || !dirty) return;
    // Coalesce N keystrokes per frame into a single draft write.
    // The previous per-keystroke effect ran editor.getContent()
    // (O(N) doc.toString()), JSON.stringify(body) (O(N) escape +
    // allocation), and a synchronous localStorage.setItem (main-
    // thread blocking I/O) on every typed character — measured at
    // 5–15 ms per keystroke for a 100 KB note, the dominant cost
    // behind the freeze the user reported on long notes. The
    // pagehide / beforeunload flush below still catches tab-close
    // exactly; the only loss-case the coalescer introduces is a
    // hard crash mid-frame, which costs at most one frame (≤16 ms)
    // of typing — well below human-perceptible data loss and the
    // same trade-off bodyForPreview makes for the preview path.
    if (draftWriteRaf) return;
    // Capture the path the schedule was for. If `note` swaps between
    // schedule and fire (e.g. SPA navigation lands a new note in the
    // same frame the user finished typing), the pending rAF must NOT
    // commit the old body under the new path.
    const scheduledPath = note.path;
    draftWriteRaf = requestAnimationFrame(() => {
      draftWriteRaf = 0;
      if (!note || !dirty || note.path !== scheduledPath) return;
      // Read the editor's authoritative content. The body mirror
      // can lag CodeMirror's actual doc during a slow reactive
      // frame, and writing the stale mirror to localStorage was
      // the silent-data-loss path: the user typed "ABCDEF" but
      // only "AB" landed in the draft, so a crash before the next
      // autosave restored "AB".
      const current = editor?.getContent?.() ?? body;
      if (lastDraftedBody === current) return;
      lastDraftedBody = current;
      setDraft(note.path, current, note.modTime);
    });
  });

  // Force-flush draft on tab hide / before unload. Belt-and-suspenders
  // since we already write synchronously per keystroke — but covers
  // the unlikely case of a body change that hasn't propagated to the
  // $effect yet (e.g. an in-flight CodeMirror dispatch the moment
  // the OS suspends the page). localStorage writes are synchronous,
  // so this guarantees the latest body lands before the page goes away.
  $effect(() => {
    if (typeof window === 'undefined') return;
    const flush = () => {
      // If a streamed AI ghost is sitting unaccepted, commit it into
      // the doc before snapshotting the draft. Ghost text lives in
      // CodeMirror's StateField, not the parent's `body` mirror, and
      // is invisible to getContent() until accepted. Without this
      // step the user's just-finished AI suggestion is silently
      // discarded when the tab closes / OS suspends the page. The
      // user can still undo via Cmd-Z when they return.
      const view = editor?.getView?.();
      const accepted = view ? acceptInlineAI(view) : false;
      // Pull from the editor's view state — `body` may be lagging
      // when the OS suspends the tab. This is the last-resort save
      // path before everything goes away; reading the wrong source
      // here costs the user their final keystrokes. The `accepted`
      // fallback covers the case where the ghost was the ONLY
      // pending change (no prior dirty=true), so the dirty check
      // would otherwise short-circuit and skip the draft write.
      if (note && (dirty || accepted)) setDraft(note.path, editor?.getContent?.() ?? body, note.modTime);
    };
    const onVis = () => { if (document.visibilityState === 'hidden') flush(); };
    window.addEventListener('beforeunload', flush);
    window.addEventListener('pagehide', flush);
    document.addEventListener('visibilitychange', onVis);
    return () => {
      window.removeEventListener('beforeunload', flush);
      window.removeEventListener('pagehide', flush);
      document.removeEventListener('visibilitychange', onVis);
    };
  });

  // When the network comes back, retry any pending save. Skip when a
  // conflict is unresolved — silently re-PUTting would just hit 412
  // again. The conflict banner is the user's required next step.
  $effect(() => {
    const onOnline = () => {
      if (saveFailed && dirty && !saving && !conflictDetected) save({ silent: true });
    };
    window.addEventListener('online', onOnline);
    return () => window.removeEventListener('online', onOnline);
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
    // the save result — the localStorage draft (synchronous per-keystroke)
    // already preserves the body, and beforeNavigate flushes again. If the
    // user is offline, save will fail; the draft is still on disk and gets
    // retried automatically when 'online' fires.
    if (dirty) void save({ silent: true });
    // Block-level wikilink: [[Note#Heading]] — split off the fragment
    // and pass it through the URL hash. The receiving page (i.e. this
    // same component on a fresh mount) reads $page.url.hash and
    // scrolls to the heading after the doc loads.
    const [titleRaw, ...frag] = target.split('#');
    const title = titleRaw.trim();
    const hash = frag.length > 0 ? `#${frag.join('#').trim()}` : '';
    try {
      const list = await api.listNotes({ q: title, limit: 5 });
      const exact = list.notes.find((n) => n.title.toLowerCase() === title.toLowerCase());
      const t = exact ?? list.notes[0];
      if (t) {
        goto(`/notes/${encodeURIComponent(t.path)}${hash}`);
        return;
      }
    } catch {}
    // Unresolved wikilink. If the user is on a note with substantial
    // content + multiple wikilinks (likely a research outline / study
    // plan), offer to generate the missing chapter via AI before
    // falling back to "open empty note". The body has to be non-
    // trivial because we ship it as the parent-outline context.
    //
    // Pass the LITERAL targetPath the wikilink would resolve to —
    // `<title>.md` at the vault root — so the saved chapter lands
    // exactly where the wikilink expects. Otherwise the chapter
    // gets nested under <parent>/<slug>.md and the wikilink would
    // still resolve to nothing on the next click.
    const targetPath = title + '.md';
    const handled = await offerAIChapterGeneration({
      parentPath: note?.path ?? '',
      parentBody: body ?? '',
      chapterTitle: title,
      targetPath
    });
    if (handled) {
      // The offer either navigated us to the new note or stayed
      // put (user declined). Either way the navigation decision is
      // already made.
      return;
    }
    goto(`/notes/${encodeURIComponent(title + '.md')}${hash}`);
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

  // Live-reload current note from WS, but never clobber unsaved edits
  // OR our own just-completed save.
  //
  // The own-save guard (lastSavedAt within ~3s) suppresses the reload
  // that the server fires back after WE save: even when bodies match
  // byte-for-byte, the body=serverBody assignment in load() can
  // disturb the editor's autocomplete state (re-running the value
  // effect, even with an equality guard, occasionally clobbers a
  // mid-snippet picker). Skipping our own bounce-back keeps the
  // composing user's flow intact.
  //
  // Reloads from a cross-device save still come through — the user's
  // own save sets lastSavedAt within milliseconds before the bounce-
  // back, so the 3s window is short enough that an external edit
  // arriving moments later still wins.
  //
  // Two correctness guards on top of the original lastSavedAt window:
  //
  // 1. body !== prev — the SYNCHRONOUS "user typed something we
  //    haven't saved yet" signal. The `dirty` flag is updated by an
  //    $effect that runs in a microtask AFTER body changes, leaving
  //    a small race window where a WS event arriving mid-keystroke
  //    saw `dirty=false` and triggered load(). The reload then
  //    overwrote the user's keystrokes with the server's body
  //    (which itself was the pre-edit version). Comparing body to
  //    prev directly catches the in-flight typing without waiting
  //    for the effect microtask.
  //
  // 2. Coalesce reloads to a single trailing-edge call per ~600ms.
  //    The server fires `note.changed` from BOTH the PUT handler
  //    AND the file-watcher (after the PUT writes the file), so a
  //    single autosave produces ≥2 WS events in close succession.
  //    Without coalescing, we'd schedule two `load()` calls and
  //    flash the editor twice. Same trailing-edge pattern that
  //    NotesTree.svelte adopted in 8cf45ba.
  let wsReloadTimer: ReturnType<typeof setTimeout> | null = null;
  function scheduleWsReload(p: string) {
    if (wsReloadTimer) clearTimeout(wsReloadTimer);
    wsReloadTimer = setTimeout(() => {
      wsReloadTimer = null;
      // Re-evaluate the guards at the moment of reload — the user
      // could have started typing during the coalesce window.
      // `editor?.getContent?.() ?? body` reads CodeMirror's state.doc
      // directly so the "is the user currently typing?" check is
      // immune to the Svelte microtask lag on bind:value (same shape
      // as every other liveness check in this file).
      if (!note || note.path !== p) return;
      if ((editor?.getContent?.() ?? body) !== prev || saving) return;
      if (lastSavedAt && Date.now() - lastSavedAt < OWN_SAVE_QUIET_MS) return;
      void load(p, { force: true });
    }, WS_RELOAD_COALESCE_MS);
  }
  onMount(() => {
    const off = onWsEvent((ev) => {
      if (ev.type !== 'note.changed') return;
      if (!note || ev.path !== note.path) return;
      // Cheap synchronous-only guards; the timed evaluation re-checks
      // the rest at fire time. The editor-content read suppresses
      // reloads while in-flight typing hasn't propagated to `body`.
      if ((editor?.getContent?.() ?? body) !== prev || saving) return;
      if (lastSavedAt && Date.now() - lastSavedAt < OWN_SAVE_QUIET_MS) return;
      scheduleWsReload(note.path);
    });
    return () => {
      off();
      if (wsReloadTimer) { clearTimeout(wsReloadTimer); wsReloadTimer = null; }
    };
  });

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

  // Research Mode — pins the AI overlay as a side rail seeded with
  // this note's context, framed as exploration rather than action.
  // Stays visible while the user navigates the note + backlinks +
  // annotations so the AI is the running thinking partner. Mirrors
  // the ProjectDetail Research Mode button; the body excerpt gives
  // the AI enough to engage without dumping the full note unless
  // the user asks.
  function openResearchMode(): void {
    if (!note) return;
    // Trim first, then measure + slice the trimmed text. The ellipsis
    // marker should reflect whether content was actually truncated,
    // not whether the raw body (which may include leading/trailing
    // whitespace) crosses the cap.
    const trimmed = (body ?? '').trim();
    const excerpt = trimmed.slice(0, RESEARCH_EXCERPT_MAX);
    const truncated = trimmed.length > RESEARCH_EXCERPT_MAX;
    const lines = [
      `I'm in research mode on this note:`,
      '',
      `- ${note.title || note.path}`
    ];
    const tags = (note.tags ?? []);
    if (tags.length > 0) lines.push(`- tags: ${tags.map((t) => '#' + t).join(' ')}`);
    if (excerpt) {
      lines.push('', 'Excerpt:', excerpt + (truncated ? '…' : ''));
    }
    lines.push(
      '',
      `Help me think about this. What angles haven't I considered? What questions should I be asking? Don't rush to recommendations — explore with me.`
    );
    aiOverlayPinned.set(true);
    openAIOverlay({ text: lines.join('\n'), send: false });
  }

  async function saveFrontmatter(next: Record<string, unknown>): Promise<boolean> {
    if (!note) return false;
    // Snapshot what we sent so a body keystroke during the await
    // doesn't get falsely marked clean below.
    const sentBody = body;
    const savedNote = note;
    const etagToSend = forceNextSave ? undefined : (noteEtag ?? undefined);
    try {
      const { data: updated, etag: newEtag } = await api.putNoteWithEtag(
        note.path,
        { frontmatter: next, body: sentBody },
        etagToSend
      );
      if (!note || note !== savedNote) return false;
      // Surgical mutation (same invariant as save() above) — full
      // reassignment fans out a re-render across every panel keyed
      // off note.path. Frontmatter is the only field that actually
      // changed locally; body wasn't touched server-side beyond what
      // we sent, modTime/size/title come back from the response.
      note.frontmatter = updated.frontmatter;
      note.modTime = updated.modTime;
      note.size = updated.size;
      note.title = updated.title;
      noteEtag = newEtag;
      forceNextSave = false;
      pendingFrontmatter = null;
      // Clear the failure flags unconditionally on a successful PUT.
      // The previous shape gated these on `!dirty` (the user didn't
      // type during the await), which left saveFailed=true and the
      // failure banner sticky whenever a frontmatter save resolved
      // a conflict but the user happened to keep typing in the body.
      saveFailed = false;
      saveFailCount = 0;
      lastSaveError = '';
      const liveNow = editor?.getContent?.() ?? body;
      // dirty stays true if the user typed during the await — the
      // body we sent matches sentBody, but the editor has moved on.
      dirty = liveNow !== sentBody;
      prev = sentBody;
      if (!dirty) {
        clearDraft(updated.path);
        lastDraftedBody = null;
        lastSavedAt = Date.now();
      } else {
        setDraft(updated.path, liveNow, updated.modTime);
      }
      return true;
    } catch (e) {
      if (e instanceof ApiError && e.status === 412) {
        // Hold the pending frontmatter so the banner's "Overwrite
        // anyway" routes BACK through saveFrontmatter — not the
        // body-save path — preserving the user's tag/field change.
        pendingFrontmatter = next;
        saveFailed = true;
        lastSaveError = 'Conflict: this note was changed elsewhere since you loaded it.';
        toast.warning('Conflict: this note was changed elsewhere. Choose Reload or Overwrite in the banner.');
        return false;
      }
      error = e instanceof Error ? e.message : String(e);
      return false;
    }
  }

  // ----- Link-suggester glue -----
  // Tags chip → append to frontmatter.tags (de-duplicated).
  // Link chip → insert markup at the editor cursor; if the editor isn't
  // mounted (e.g. preview view), append to the end of the body so the
  // user still gets a working insertion.
  //
  // Frontmatter `tags:` lands as either an array (idiomatic) or a
  // string (legacy / hand-typed YAML, e.g. `tags: a, b, c`). One
  // helper handles both shapes so the existingTagList derivation and
  // the addSuggestedTag append-path agree on the parse.
  function parseTagsField(fm: Record<string, unknown> | undefined | null): string[] {
    if (!fm) return [];
    const t = fm.tags;
    if (Array.isArray(t)) return t.map((x) => String(x));
    if (typeof t === 'string') return t.split(/[,\s]+/).filter(Boolean);
    return [];
  }
  let existingTagList = $derived(
    parseTagsField(note?.frontmatter as Record<string, unknown> | undefined)
  );

  async function addSuggestedTag(tag: string) {
    if (!note) return;
    const clean = tag.trim().replace(/^#/, '').toLowerCase();
    if (!clean) return;
    const fm = { ...(note.frontmatter ?? {}) } as Record<string, unknown>;
    const arr = parseTagsField(fm);
    if (arr.includes(clean)) {
      toast.success(`#${clean} already on this note`);
      return;
    }
    arr.push(clean);
    fm.tags = arr;
    // Only toast success when the save actually committed. Before
    // saveFrontmatter exposed an outcome, a 412 silently dropped the
    // tag (saveFrontmatter swallows the error and shows the conflict
    // banner) but the success toast fired anyway — the user saw both
    // "Conflict" and "+ #tag" simultaneously, and the chip vanished
    // from the suggester even though no save happened.
    if (await saveFrontmatter(fm)) toast.success(`+ #${clean}`);
  }

  function insertSuggestedLink(markup: string) {
    if (editor?.insertAtCursor) {
      editor.insertAtCursor(' ' + markup + ' ');
    } else {
      // Fallback: append + mark dirty so save picks it up.
      body = body + (body.endsWith('\n') ? '' : '\n') + markup + '\n';
      dirty = true;
    }
    toast.success('link inserted');
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

  // Folder breadcrumbs — derived once so the template stays
  // declarative. Each crumb carries its own folder filter URL so a
  // mid-path click goes "show me everything in <root>/a/b/" without
  // recomputing the prefix in markup. When the user expands a
  // collapsed deep path we flip `breadcrumbExpanded` to render every
  // segment instead of first/…/last.
  // Reset is folded into load() — the only path that swaps note
  // identity. The previous version used a $effect tracking
  // `note?.path`, which depended on the autosave path NEVER
  // reassigning `note` (it doesn't — surgical mutation), an
  // invariant that's brittle to encode as a reactive dep.
  let breadcrumbExpanded = $state(false);
  interface Crumb { label: string; href: string }
  let allCrumbs = $derived.by<Crumb[]>(() => {
    if (!note) return [];
    const segs = note.path.split('/').slice(0, -1);
    return segs.map((seg, i) => ({
      label: seg,
      href: `/notes?folder=${encodeURIComponent(segs.slice(0, i + 1).join('/'))}`
    }));
  });
  // When the path has more than 3 folder segments we collapse the
  // middle ones into a clickable ellipsis so the bar stays one-line
  // even on deeply-nested paths (e.g. work/projects/2026/q1/notes).
  // Showing the first two + last keeps the most relevant context
  // (top-level area + immediate parent) without truncating the title.
  const CRUMB_COLLAPSE_THRESHOLD = 4;
  let visibleCrumbs = $derived.by<Crumb[]>(() => {
    if (breadcrumbExpanded) return allCrumbs;
    if (allCrumbs.length <= CRUMB_COLLAPSE_THRESHOLD) return allCrumbs;
    // Keep the first two and the last segment; expansion shows all.
    return [...allCrumbs.slice(0, 2), ...allCrumbs.slice(-1)];
  });
  let crumbsCollapsed = $derived(
    !breadcrumbExpanded && allCrumbs.length > CRUMB_COLLAPSE_THRESHOLD
  );

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
    onAddSuggestedTag={addSuggestedTag}
    onInsertSuggestedLink={insertSuggestedLink}
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
