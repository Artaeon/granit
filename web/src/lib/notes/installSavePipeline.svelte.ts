// Single installer that wires the two persistence sub-pipelines the
// notes editor route needs:
//
//   1. installNoteAutosave — dirty tracker + 2 s debounce + draft
//      rAF + tab-hide flush + online retry + unmount cleanup.
//   2. installWsReload — coalesced trailing-edge reload on
//      `note.changed` WS bursts, honouring every clobber guard
//      (live editor content, in-flight save, own-save quiet window).
//
// Both surfaces read the same pipe + editor handles, so collapsing
// the wiring into one install drops ~30 LOC of duplicated dep
// plumbing from the route. The defaults for OWN_SAVE_QUIET_MS,
// WS_RELOAD_COALESCE_MS, and AUTOSAVE_DEBOUNCE_MS now live here too;
// the page no longer carries the magic numbers next to unrelated
// route concerns.

import { onMount } from 'svelte';
import { installNoteAutosave } from '$lib/notes/noteAutosave.svelte';
import { installWsReload } from '$lib/notes/wsReload.svelte';
import type { NotePipelineController } from '$lib/notes/notePipelineState.svelte';
import type { EditorHandle } from '$lib/notes/editorHandle';

export interface SavePipelineOpts {
  pipe: NotePipelineController;
  /** Editor handle accessor — late-binding because the editor isn't
   *  mounted at install time. */
  getEditor: () => EditorHandle | undefined;
  /** Save wrapper from the page (it owns the no-op short-circuit on
   *  already-saving + the post-save draftRestored clear). */
  save: (opts: { silent?: boolean }) => Promise<boolean>;
  /** Reload wrapper from the page — wsReload calls into it on a
   *  coalesced trailing edge. */
  reload: (path: string) => void;
}

// Autosave debounce — 2 s after the last keystroke when the picker
// isn't open. See installNoteAutosave for the picker-backoff path.
const AUTOSAVE_DEBOUNCE_MS = 2000;
// Window after our own save during which an inbound `note.changed`
// WS event is suppressed — the event is almost certainly the echo of
// our own write bouncing through the file watcher. 3 s covers the
// worst-case file-watcher debounce + scan + broadcast latency.
const OWN_SAVE_QUIET_MS = 3000;
// Trailing-edge coalesce on `note.changed` bursts — a TUI save or
// sync drop can fire 5+ events in a burst. One reload per burst.
const WS_RELOAD_COALESCE_MS = 600;

export function installSavePipeline(opts: SavePipelineOpts): void {
  const { pipe, getEditor, save, reload } = opts;

  installNoteAutosave({
    getNote: () => pipe.note,
    getBody: () => pipe.body,
    getLiveBody: () => getEditor()?.getContent?.() ?? pipe.body,
    getDirty: () => pipe.dirty,
    getSaving: () => pipe.saving,
    getSaveFailed: () => pipe.saveFailed,
    getConflictDetected: () => pipe.conflictDetected,
    getPrev: () => pipe.prev,
    setDirty: (v) => { pipe.dirty = v; },
    setPrev: (v) => { pipe.prev = v; },
    getLastDraftedBody: () => pipe.lastDraftedBody,
    setLastDraftedBody: (v) => { pipe.lastDraftedBody = v; },
    getEditorView: () => getEditor()?.getView?.(),
    isCompletionActive: () => getEditor()?.isCompletionActive?.() ?? false,
    save,
    autosaveDebounceMs: AUTOSAVE_DEBOUNCE_MS
  });

  onMount(() =>
    installWsReload({
      getActivePath: () => pipe.note?.path ?? null,
      getLiveBody: () => getEditor()?.getContent?.() ?? pipe.body,
      getSavedBody: () => pipe.prev,
      isSaving: () => pipe.saving,
      getLastSavedAt: () => pipe.lastSavedAt,
      reload,
      ownSaveQuietMs: OWN_SAVE_QUIET_MS,
      coalesceMs: WS_RELOAD_COALESCE_MS
    })
  );
}
