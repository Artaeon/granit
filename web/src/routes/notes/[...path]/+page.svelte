<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/stores';
  import { api, type Note } from '$lib/api';
  import { installSavePipeline } from '$lib/notes/installSavePipeline.svelte';
  import NotesTree from '$lib/notes/NotesTree.svelte';
  import DailyQuickAdd from '$lib/notes/DailyQuickAdd.svelte';
  import DailyContext from '$lib/notes/DailyContext.svelte';
  import NoteDeadlinesStrip from '$lib/deadlines/NoteDeadlinesStrip.svelte';
  import Drawer from '$lib/components/Drawer.svelte';
  import { toast } from '$lib/components/toast';
  import { createFlashcardsAction } from '$lib/notes/flashcardsAction.svelte';
  import { installNoteLifecycleEffects } from '$lib/notes/noteLifecycleEffects.svelte';
  import { installPreviewScrollWiring } from '$lib/notes/previewScrollWiring.svelte';
  import { installNoteShortcuts } from '$lib/notes/noteKeyboardShortcuts.svelte';
  import { createNoteEditorActions } from '$lib/notes/noteEditorActions.svelte';
  import { createNoteBreadcrumbsCtl } from '$lib/notes/noteBreadcrumbsCtl.svelte';
  import { createNoteSaveStatusCtl } from '$lib/notes/noteSaveStatusCtl.svelte';
  import { createNoteVersionCount } from '$lib/notes/noteVersionCount.svelte';
  import { createNotePipelineState } from '$lib/notes/notePipelineState.svelte';
  import { createMissingNoteCtl } from '$lib/notes/createMissingNote.svelte';
  import NoteEditorRootOverlays from '$lib/notes/NoteEditorRootOverlays.svelte';
  import { createExtractController } from '$lib/notes/extractToNote.svelte';
  import { createNoteLinkSuggester } from '$lib/notes/noteLinkSuggester.svelte';
  import { createLoadNoteWrapper } from '$lib/notes/loadNoteWrapper.svelte';
  import { createViewModeController } from '$lib/notes/viewModes.svelte';
  import { createViewportBreakpoints } from '$lib/notes/viewportBreakpoints.svelte';
  import { createPreviewBodyMirror } from '$lib/notes/previewBodyMirror.svelte';
  import { createNoteWordStats } from '$lib/notes/noteWordStats.svelte';
  import { shiftDate } from '$lib/notes/dailyNote';
  import { createDailyNoteNav } from '$lib/notes/dailyNoteNav.svelte';
  import { createInlineAIBridge } from '$lib/notes/inlineAIBridge.svelte';
  import NoteAudioPlayer from '$lib/notes/NoteAudioPlayer.svelte';
  import NoteStatusBar from '$lib/notes/NoteStatusBar.svelte';
  import NoteInfoRail from '$lib/notes/NoteInfoRail.svelte';
  import NoteHeader from '$lib/notes/NoteHeader.svelte';
  import NoteEditorBanners from '$lib/notes/NoteEditorBanners.svelte';
  import NoteEmptyState from '$lib/notes/NoteEmptyState.svelte';
  import NoteEditorPane from '$lib/notes/NoteEditorPane.svelte';
  import type { EditorHandle } from '$lib/notes/editorHandle';
  import { ensurePinnedLoaded } from '$lib/notes/pinnedNotes';
  import { createNotePinAction } from '$lib/notes/notePinAction.svelte';
  import { createEditorCallbacks } from '$lib/notes/editorCallbacks.svelte';
  import { createNoteEditorOverlays } from '$lib/notes/noteEditorOverlays.svelte';

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

  // Inline-AI bridge — CodeMirror trigger extension + state observer
  // forwarding into reactive $state slots. Lives in inlineAIBridge.
  const aiBridge = createInlineAIBridge();
  let aiTriggerEvent = $derived(aiBridge.triggerEvent);
  let aiGhostState = $derived(aiBridge.ghostState);
  const editorAIExtensions = aiBridge.extensions;

  // Flashcard-scheduling lives in flashcardsAction — same Q:/A: parse
  // + 5-step (1/3/7/14/30 day) schedule + toast branches. The page
  // hands the action a getBody + closeOverflow and fires .run() from
  // the overflow menu.
  const flashcards = createFlashcardsAction({
    getBody: () => body,
    closeOverflow: () => { overlays.overflowOpen = false; }
  });
  let schedulingFlashcards = $derived(flashcards.schedulingFlashcards);

  // Cursor (line/col/selLen) + scroll-progress for the editor.
  // Controller exposes ready-made onCursor / onScroll callbacks the
  // Editor wires to, so the page template no longer has to allocate
  // inline closures on every render.
  const editorCb = createEditorCallbacks();
  let cursorLine = $derived(editorCb.cursorLine);
  let cursorCol = $derived(editorCb.cursorCol);
  let cursorSelLen = $derived(editorCb.cursorSelLen);
  let readProgress = $derived(editorCb.readProgress);

  // Preview-pane scroll container. Bound via bind:this below — must
  // stay an lvalue here. Outline uses it as the IntersectionObserver
  // root for active-heading tracking; the previewScrollTracker uses
  // it for the heading-checkpoint walk + progress bar.
  let previewContainer = $state<HTMLElement | null>(null);

  // Visited-headings tracker + scroll-progress (visited set + scroll
  // 0..1) — controller in previewScrollTracker; wiring (loadFor on
  // path swap, attach while container ref exists, resetVisited) lives
  // in installPreviewScrollWiring.
  const previewScroll = installPreviewScrollWiring({
    getNotePath: () => note?.path ?? null,
    getContainer: () => previewContainer
  });
  let visitedHeadings = $derived(previewScroll.visitedHeadings);
  let previewProgress = $derived(previewScroll.previewProgress);
  const resetVisited = previewScroll.resetVisited;
  let editor = $state<EditorHandle | undefined>();
  // Re-derived after every render so the SelectionToolbar can scope
  // its selection detection to the editor's contentDOM specifically.
  // The CodeMirror DOM exists only after mount, so this stays
  // `undefined` until then and the toolbar simply doesn't render.
  let editorDOM = $derived(editor?.getDOM());

  // Per-note scroll position cache lives in $lib/notes/noteHistory.
  // Pixel-accurate (not line-accurate) because line tracking
  // misbehaves once the user changes font size or window width —
  // pixels survive reflow because we restore on the same note (same
  // width, same font) only.

  // Overlay / drawer / annotation-count cluster lives in
  // noteEditorOverlays — eight one-line slots previously declared
  // independently at the top of the route, now exposed via a single
  // controller. Template binds use `bind:open={overlays.X}`;
  // imperative opens use `overlays.X = true`.
  const overlays = createNoteEditorOverlays();

  // Pin / unpin lives in notePinAction — the controller subscribes
  // to the shared pinnedNotes store + owns the busy flag. The page
  // calls `pinAction.togglePin()` from the header.
  const pinAction = createNotePinAction({ getNote: () => note });
  let pinned = $derived(pinAction.pinned);
  let pinBusy = $derived(pinAction.pinBusy);
  const togglePin = pinAction.togglePin;

  let draftRestored = $derived(pipe.draftRestored);

  // Folder breadcrumbs controller. Declared here (not next to its
  // derivations further down) because the loadNote wrapper below takes
  // it as a dep — referencing it later would read it inside its
  // temporal dead zone and crash every mount of this page with
  // "Cannot access 'breadcrumbs' before initialization". Depends only
  // on `note` via a lazy getter, so its position doesn't matter.
  const breadcrumbs = createNoteBreadcrumbsCtl({ getNote: () => note });

  // Route-side adapter around loadNote — closes over the editor +
  // drawer state + breadcrumbs + URL accessors. See loadNoteWrapper
  // for the deps wiring; loadNote.ts owns the actual draft +
  // 404 + scroll-restore contract.
  const load = createLoadNoteWrapper({
    pipe,
    overlays,
    breadcrumbs,
    getEditor: () => editor,
    getLineParam: () => $page.url.searchParams.get('line'),
    getRawHash: () => $page.url.hash ? decodeURIComponent($page.url.hash.slice(1)) : '',
    save: (o) => save(o)
  });

  $effect(() => {
    const path = $page.params.path;
    if (path) load(decodeURIComponent(path));
  });

  // "Create note" action for the not-found state + the derived
  // header title. Lives in createMissingNote — owns the busy gate,
  // the path-cleaning, and the post-create force-reload.
  const missingNote = createMissingNoteCtl({
    pipe,
    getRawPath: () => $page.params.path ?? '',
    load
  });
  let creatingNote = $derived(missingNote.creatingNote);
  let notFoundTitle = $derived(missingNote.notFoundTitle);

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

  // Persistence sub-pipelines (autosave + WS reload) + the save()
  // wrapper itself live behind installSavePipeline. The returned
  // `save` is the same wrapper other controllers (extractCtl,
  // dailyNav, actions, lifecycle effects) wire as their save
  // callback.
  const { save } = installSavePipeline({
    pipe,
    getEditor: () => editor,
    reload: (p) => void load(p, { force: true })
  });

  // Cross-surface lifecycle effects (active-editor registration +
  // open-note tray + beforeunload prompt + beforeNavigate scroll
  // snapshot) live in noteLifecycleEffects.
  installNoteLifecycleEffects({
    getNote: () => pipe.note,
    getDirty: () => pipe.dirty,
    getSaving: () => pipe.saving,
    getEditorView: () => editor?.getView?.(),
    getScrollTop: () => editor?.getScrollTop?.(),
    save: (o) => save(o)
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

  // Snapshot-count fetcher lives in noteVersionCount — refreshes on
  // note swap and every save (modTime tick), with a generation guard
  // so stale fetches can't write into the current note's chip.
  const versionCountCtl = createNoteVersionCount({ getNote: () => note });
  let versionCount = $derived(versionCountCtl.versionCount);
  // Overflow trigger ref — the menu needs the button's DOMRect to
  // compute its viewport-clamped position. Lives here (not in the
  // overlays controller) because it's bound via `bind:overflowTriggerEl`
  // on the NoteHeader and the parent has to own the lvalue.
  let overflowTriggerEl: HTMLButtonElement | undefined = $state();


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
      openHelp: () => { overlays.helpOpen = true; },
      openPresentation: () => { overlays.presentationOpen = true; }
    })
  );

  // Four small route-side action helpers — saveFrontmatter,
  // navigateWikilink, openResearchMode, jumpToLine — bundled into
  // noteEditorActions. Each one's a thin closure over pipe + editor
  // + overlays; centralising the binding drops ~30 LOC from the
  // route. The wikilinkNav / researchMode / saveFrontmatter helpers
  // still own the real work.
  const actions = createNoteEditorActions({
    pipe,
    overlays,
    getEditor: () => editor,
    save: (o) => save(o)
  });
  const saveFrontmatter = actions.saveFrontmatter;
  const navigateWikilink = actions.navigateWikilink;
  const openResearchMode = actions.openResearchMode;
  const jumpToLine = actions.jumpToLine;

  // Link-suggester glue (tag chip + link chip insert) lives in
  // noteLinkSuggester — the route hands current-note + saveFrontmatter
  // + the editor's insertAtCursor + the appendToBody fallback closure.
  const linkSuggester = createNoteLinkSuggester({
    getNote: () => note,
    saveFrontmatter,
    getInsertAtCursor: () => editor?.insertAtCursor,
    appendToBody: (m) => {
      pipe.body = pipe.body + (pipe.body.endsWith('\n') ? '' : '\n') + m + '\n';
      pipe.dirty = true;
    }
  });
  let existingTagList = $derived(linkSuggester.existingTagList);

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

  // Folder breadcrumbs derivations (allCrumbs / visibleCrumbs /
  // crumbsCollapsed). The `breadcrumbs` controller itself is created
  // earlier — above the loadNote wrapper, which takes it as a dep —
  // so it must exist before that point. load() calls
  // breadcrumbs.reset() on real navigation; the header calls
  // breadcrumbs.expand() from the "+N more" affordance.
  let allCrumbs = $derived(breadcrumbs.allCrumbs);
  let visibleCrumbs = $derived(breadcrumbs.visibleCrumbs);
  let crumbsCollapsed = $derived(breadcrumbs.crumbsCollapsed);

  let dailyLabel = $derived(dailyNav.dailyLabel);
</script>

{#snippet treeContent()}
  <div class="px-2 pt-2 pb-1 text-xs uppercase tracking-wider text-dim flex-shrink-0">Vault</div>
  <NotesTree currentPath={note?.path} onSelect={() => (overlays.treeDrawerOpen = false)} />
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
    bind:annotationCount={overlays.annotationCount}
    {existingTagList}
    onJumpToLine={jumpToLine}
    onNavigateWikilink={navigateWikilink}
    onResetVisited={resetVisited}
    onSaveFrontmatter={saveFrontmatter}
    onAddSuggestedTag={linkSuggester.addSuggestedTag}
    onInsertSuggestedLink={linkSuggester.insertSuggestedLink}
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
    <Drawer bind:open={overlays.treeDrawerOpen} side="left" responsive width="w-72 sm:w-80">
      <div class="h-full flex flex-col">
        {@render treeContent()}
      </div>
    </Drawer>
  {/if}

  <!-- Center: editor -->
  <div class="flex-1 flex flex-col min-w-0">
    <!-- Empty / not-found / error states extracted to <NoteEmptyState>.
         Renders the three load-failure branches (404 → create
         affordance, error-stuck → retry header, transient error →
         strip) or nothing when a note is loaded. -->
    <NoteEmptyState
      notFound={notFound}
      error={error}
      hasNote={note !== null}
      {notFoundTitle}
      rawPath={decodeURIComponent($page.params.path ?? '')}
      {creatingNote}
      onCreate={missingNote.create}
      onOpenTreeDrawer={() => (overlays.treeDrawerOpen = true)}
      onRetry={() => { pipe.lastLoadedPath = ''; load(decodeURIComponent($page.params.path ?? '')); }}
    />
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
        overflowOpen={overlays.overflowOpen}
        bind:overflowTriggerEl
        onOpenTreeDrawer={() => (overlays.treeDrawerOpen = true)}
        onOpenInfoDrawer={() => (overlays.infoDrawerOpen = true)}
        onExpandBreadcrumbs={breadcrumbs.expand}
        onSetViewMode={viewModes.setViewMode}
        onTogglePin={togglePin}
        onGotoDaily={gotoDaily}
        onShiftDate={shiftDate}
        onDispatchAI={() => editor?.dispatchChord('Mod-/')}
        onOpenResearchMode={openResearchMode}
        onToggleOverflow={() => (overlays.overflowOpen = !overlays.overflowOpen)}
        onSave={() => save()}
        {versionCount}
        onOpenHistory={() => (overlays.historyOpen = true)}
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
      {#if overlays.audioOpen}
        <NoteAudioPlayer
          body={bodyForPreview}
          title={note.title || note.path}
          onClose={() => (overlays.audioOpen = false)}
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
      <!-- Edit / preview / split switch lives in <NoteEditorPane>;
           the page hands a bind:this passthrough (bindEditor /
           bindPreviewContainer) so the editor handle + preview
           container ref reach the route's lifecycle effects, scroll
           tracker, and selection toolbar. -->
      <NoteEditorPane
        {note}
        {viewMode}
        bind:body={pipe.body}
        {bodyForPreview}
        {dayActivitySegments}
        {dailyDate}
        {editorAIExtensions}
        onSave={save}
        onNavigateWikilink={navigateWikilink}
        onExtract={extractCtl.handleExtract}
        onCursor={editorCb.onCursor}
        onScroll={editorCb.onScroll}
        onSaveFrontmatter={saveFrontmatter}
        onPrepend={(text) => { pipe.body = text + pipe.body; pipe.dirty = true; }}
        bindEditor={(h) => (editor = h)}
        bindPreviewContainer={(el) => (previewContainer = el)}
      />
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
    <Drawer bind:open={overlays.infoDrawerOpen} side="right" responsive width="w-80 sm:w-96">
      {@render infoContent()}
    </Drawer>
  {/if}
</div>

<!-- Page-root overlay + floating-toolbar cluster (ExtractDialog,
     PrintPreview, HistoryPanel, ShortcutsHelp, NotePresentation,
     SelectionToolbar, MobileEditorToolbar, AIActionBar, InlineAIMenu,
     NoteOverflowMenu). Extracted to <NoteEditorRootOverlays>. -->
<NoteEditorRootOverlays
  {note}
  {bodyForPreview}
  {overlays}
  {viewModes}
  editorDOM={editorDOM}
  editorView={editor?.getView?.()}
  dispatchChord={(chord) => editor?.dispatchChord(chord)}
  insertAtCursor={(text) => editor?.insertAtCursor(text)}
  openFind={() => editor?.openFind()}
  {overflowTriggerEl}
  {readingMode}
  {focusMode}
  extractRequest={extractCtl.request}
  onExtractConfirm={extractCtl.confirm}
  onExtractDismiss={extractCtl.dismiss}
  onHistoryRestore={(restoredBody) => { pipe.body = restoredBody; pipe.dirty = true; }}
  {aiTriggerEvent}
  {aiGhostState}
  onAITriggerClose={aiBridge.clearTrigger}
  {schedulingFlashcards}
  onScheduleFlashcards={flashcards.run}
/>

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
