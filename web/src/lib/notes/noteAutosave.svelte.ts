// Autosave pipeline for the notes editor — six concerns wired
// through a single install function so the page just lists deps and
// stays free of the per-effect plumbing:
//
//   1. dirty tracker — body !== prev → mark unsaved.
//   2. autosave debounce — fire save({silent:true}) 2s after the
//      last keystroke, but back off while the autocomplete picker
//      is open (saving causes a doc reset that closes the picker).
//   3. draft persistence — rAF-coalesce per-keystroke draft writes
//      to localStorage. Pre-rAF the synchronous JSON.stringify +
//      setItem was the dominant cost behind the "long-note editor
//      freeze" the user reported; capture the path so a navigation
//      mid-frame can't commit the old body under the new path.
//   4. tab-hide / unload flush — write the latest body synchronously
//      before the OS suspends the page. Belt-and-suspenders for the
//      rAF coalescer + accepts any unaccepted inline-AI ghost first
//      so the user's just-finished suggestion isn't silently lost.
//   5. online retry — when the network comes back, fire any pending
//      save once. Skip on unresolved conflict (re-PUT would 412).
//   6. unmount — cancel the pending draft rAF so a torn-down page
//      doesn't write a stale snapshot after its state is gone.
//
// All six are installed by one call from the page's <script> body.
// `$effect` registrations bind to the calling component's lifecycle,
// so a $effect-using function called from component-init scope ends
// up with effects owned by that component.

import { setDraft } from '$lib/notes/drafts';
import { acceptInlineAI } from '$lib/editor/inline-ai';
import type { Note } from '$lib/api';
import type { EditorView } from '@codemirror/view';

export interface AutosaveDeps {
  /** Reactive reads — getters so the $effect tracks them. */
  getNote: () => Note | null;
  getBody: () => string;
  getLiveBody: () => string;
  getDirty: () => boolean;
  getSaving: () => boolean;
  getSaveFailed: () => boolean;
  getConflictDetected: () => boolean;
  /** dirty tracker writes via these — the page owns the underlying
   *  $state but we keep the diff calculation here. */
  getPrev: () => string;
  setDirty: (v: boolean) => void;
  setPrev: (v: string) => void;
  /** Draft watermark — shared with save() and load() resets. */
  getLastDraftedBody: () => string | null;
  setLastDraftedBody: (v: string | null) => void;
  /** Editor handles for the picker-backoff and the unload flush. */
  getEditorView: () => EditorView | undefined;
  isCompletionActive: () => boolean;
  /** Save callback — both autosave debounce and online-retry route
   *  through this. */
  save: (opts: { silent?: boolean }) => Promise<boolean>;
  /** Tuning knobs. */
  autosaveDebounceMs: number;
}

export function installNoteAutosave(deps: AutosaveDeps): void {
  let draftWriteRaf = 0;

  // 1. Dirty tracker.
  $effect(() => {
    const b = deps.getBody();
    if (b !== deps.getPrev()) {
      deps.setDirty(true);
      deps.setPrev(b);
    }
  });

  // 2. Autosave debounce.
  $effect(() => {
    void deps.getBody();
    if (!deps.getDirty() || deps.getSaving() || !deps.getNote()) return;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const trySave = () => {
      timer = null;
      if (!deps.getDirty() || deps.getSaving() || !deps.getNote()) return;
      // Unresolved conflict — re-PUTting would just bounce off 412
      // again. Wait for the user to choose Reload or Overwrite via
      // the conflict banner.
      if (deps.getConflictDetected()) return;
      if (deps.isCompletionActive()) {
        // Picker open — back off and re-check in 1s.
        timer = setTimeout(trySave, 1000);
        return;
      }
      void deps.save({ silent: true });
    };
    timer = setTimeout(trySave, deps.autosaveDebounceMs);
    return () => {
      if (timer) clearTimeout(timer);
    };
  });

  // 3. Draft rAF coalesce.
  $effect(() => {
    void deps.getBody();
    const note = deps.getNote();
    if (!note || !deps.getDirty()) return;
    if (draftWriteRaf) return;
    // Capture the path so a navigation between schedule and fire
    // can't commit the old body under the new path.
    const scheduledPath = note.path;
    draftWriteRaf = requestAnimationFrame(() => {
      draftWriteRaf = 0;
      const n = deps.getNote();
      if (!n || !deps.getDirty() || n.path !== scheduledPath) return;
      const current = deps.getLiveBody();
      if (deps.getLastDraftedBody() === current) return;
      deps.setLastDraftedBody(current);
      setDraft(n.path, current, n.modTime);
    });
  });

  // 4. Tab-hide / unload flush.
  $effect(() => {
    if (typeof window === 'undefined') return;
    const flush = () => {
      // If a streamed AI ghost is sitting unaccepted, commit it into
      // the doc before snapshotting the draft. Ghost text lives in
      // CodeMirror's StateField, invisible to getContent() until
      // accepted. Without this the user's just-finished suggestion
      // is silently lost on tab-close; the `accepted` fallback also
      // covers ghost-only edits where dirty would otherwise short-
      // circuit and skip the draft write.
      const view = deps.getEditorView();
      const accepted = view ? acceptInlineAI(view) : false;
      const n = deps.getNote();
      if (n && (deps.getDirty() || accepted)) {
        setDraft(n.path, deps.getLiveBody(), n.modTime);
      }
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

  // 5. Online retry.
  $effect(() => {
    if (typeof window === 'undefined') return;
    const onOnline = () => {
      if (
        deps.getSaveFailed() &&
        deps.getDirty() &&
        !deps.getSaving() &&
        !deps.getConflictDetected()
      ) {
        void deps.save({ silent: true });
      }
    };
    window.addEventListener('online', onOnline);
    return () => window.removeEventListener('online', onOnline);
  });

  // 6. Unmount — cancel the pending draft rAF. We DON'T return a
  // cleanup from the rAF effect (that fires on every body change and
  // would defeat the coalescer); instead we own a dedicated no-deps
  // effect whose cleanup only fires when the component tears down.
  $effect(() => {
    return () => {
      if (draftWriteRaf) {
        cancelAnimationFrame(draftWriteRaf);
        draftWriteRaf = 0;
      }
    };
  });
}
