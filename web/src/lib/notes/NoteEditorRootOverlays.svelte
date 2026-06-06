<script lang="ts">
  // Page-root overlay + floating-toolbar cluster for the notes
  // editor. Ten self-contained surfaces previously mounted as
  // siblings under the route's main flex container, all rendered
  // here so the route's template doesn't carry the props pass-
  // through for each one:
  //
  //   • <ExtractToNoteDialog>   — Mod-Shift-X extract flow
  //   • <PrintPreview>          — PDF preview (renders nothing closed)
  //   • <HistoryPanel>          — snapshot browser
  //   • <ShortcutsHelpOverlay>  — keyboard cheat sheet
  //   • <NotePresentation>      — fullscreen slideshow
  //   • <SelectionToolbar>      — selection-anchored floating toolbar
  //   • <MobileEditorToolbar>   — on-screen-keyboard formatting bar
  //   • <AIActionBar>           — inline-AI ghost Keep / Try again /
  //                                Discard / Stop chip
  //   • <InlineAIMenu>          — Cmd-/ command palette
  //   • <NoteOverflowMenu>      — ⋯ overflow popover
  //
  // Each surface still owns its own open / closed state via the
  // overlays controller; this component is purely the markup
  // pass-through.

  import type { Note } from '$lib/api';
  import type { EditorView } from '@codemirror/view';
  import type { ExtractRequest } from '$lib/editor/extract-note';
  import type { NoteEditorOverlays } from '$lib/notes/noteEditorOverlays.svelte';
  import type { ViewModeController } from '$lib/notes/viewModes.svelte';
  import type { InlineAITriggerEvent } from '$lib/editor/inline-ai-trigger';
  import type { InlineAIState } from '$lib/editor/inline-ai';

  import ExtractToNoteDialog from '$lib/notes/ExtractToNoteDialog.svelte';
  import PrintPreview from '$lib/notes/PrintPreview.svelte';
  import HistoryPanel from '$lib/notes/HistoryPanel.svelte';
  import ShortcutsHelpOverlay from '$lib/notes/ShortcutsHelpOverlay.svelte';
  import NotePresentation from '$lib/notes/NotePresentation.svelte';
  import SelectionToolbar from '$lib/editor/SelectionToolbar.svelte';
  import MobileEditorToolbar from '$lib/editor/MobileEditorToolbar.svelte';
  import AIActionBar from '$lib/notes/AIActionBar.svelte';
  import InlineAIMenu from '$lib/notes/InlineAIMenu.svelte';
  import NoteOverflowMenu from '$lib/notes/NoteOverflowMenu.svelte';

  interface Props {
    note: Note | null;
    bodyForPreview: string;
    overlays: NoteEditorOverlays;
    viewModes: ViewModeController;
    /** Editor handle methods exposed as one-off callbacks (the
     *  bind:this is on the parent so we can't pass the handle
     *  itself).  */
    editorDOM: HTMLElement | undefined;
    editorView: EditorView | undefined;
    dispatchChord: (chord: string) => void;
    insertAtCursor: (text: string) => void;
    openFind: () => void;
    overflowTriggerEl: HTMLButtonElement | undefined;
    // Reading / focus mode echoes for the overflow menu's icons.
    readingMode: boolean;
    focusMode: boolean;
    // Extract dialog state from the route's controller.
    extractRequest: ExtractRequest | null;
    onExtractConfirm: (args: { title: string; path: string; tags: string[] }) => Promise<void>;
    onExtractDismiss: () => void;
    // History restore — writes back into the page's body / dirty.
    onHistoryRestore: (restoredBody: string) => void;
    // Inline-AI bridge state + clear-on-close.
    aiTriggerEvent: InlineAITriggerEvent | null;
    aiGhostState: InlineAIState | null;
    onAITriggerClose: () => void;
    // Flashcards action — busy flag + run callback.
    schedulingFlashcards: boolean;
    onScheduleFlashcards: () => void;
  }

  const {
    note,
    bodyForPreview,
    overlays,
    viewModes,
    editorDOM,
    editorView,
    dispatchChord,
    insertAtCursor,
    openFind,
    overflowTriggerEl,
    readingMode,
    focusMode,
    extractRequest,
    onExtractConfirm,
    onExtractDismiss,
    onHistoryRestore,
    aiTriggerEvent,
    aiGhostState,
    onAITriggerClose,
    schedulingFlashcards,
    onScheduleFlashcards
  }: Props = $props();
</script>

<!-- Extract-to-note dialog. Renders nothing while request is null. -->
<ExtractToNoteDialog
  request={extractRequest}
  sourcePath={note?.path ?? ''}
  onConfirm={onExtractConfirm}
  onDismiss={onExtractDismiss}
/>

{#if note}
  <PrintPreview
    bind:open={overlays.printOpen}
    title={note.title || note.path}
    body={bodyForPreview}
    sourcePath={note.path}
    onClose={() => (overlays.printOpen = false)}
  />
{/if}

{#if note}
  <HistoryPanel
    bind:open={overlays.historyOpen}
    notePath={note.path}
    currentBody={bodyForPreview}
    onRestore={onHistoryRestore}
  />
{/if}

<ShortcutsHelpOverlay
  bind:open={overlays.helpOpen}
  onClose={() => (overlays.helpOpen = false)}
/>

{#if note}
  <NotePresentation
    body={bodyForPreview}
    title={note.title || note.path}
    open={overlays.presentationOpen}
    onClose={() => (overlays.presentationOpen = false)}
  />
{/if}

<SelectionToolbar
  container={editorDOM}
  onCommand={dispatchChord}
/>

<MobileEditorToolbar
  contentDOM={editorDOM}
  onCommand={dispatchChord}
  onInsert={insertAtCursor}
/>

<AIActionBar view={editorView} aiState={aiGhostState} />

{#if aiTriggerEvent && note}
  <InlineAIMenu
    event={aiTriggerEvent}
    notePath={note.path}
    body={bodyForPreview}
    onClose={onAITriggerClose}
  />
{/if}

{#if note}
  <NoteOverflowMenu
    bind:open={overlays.overflowOpen}
    triggerEl={overflowTriggerEl}
    audioOpen={overlays.audioOpen}
    {readingMode}
    {focusMode}
    {schedulingFlashcards}
    onOpenFind={openFind}
    onOpenHistory={() => (overlays.historyOpen = true)}
    onOpenPrint={() => (overlays.printOpen = true)}
    onOpenPresentation={() => (overlays.presentationOpen = true)}
    onToggleAudio={() => (overlays.audioOpen = !overlays.audioOpen)}
    onToggleReadingMode={viewModes.toggleReadingMode}
    onToggleFocusMode={viewModes.toggleFocusMode}
    onScheduleFlashcards={onScheduleFlashcards}
    onOpenHelp={() => (overlays.helpOpen = true)}
  />
{/if}
