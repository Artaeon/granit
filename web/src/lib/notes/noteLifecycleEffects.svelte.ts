// Cross-surface lifecycle wiring for the notes editor route.
//
// Four effects that previously lived inline at the top of the page
// — all glue between the route and global stores / browser events:
//
//   1. registerActiveEditor — registers the current CodeMirror view
//      so cross-surface features (AIOverlay "insert at cursor",
//      future drop-into-note actions) can target this note's cursor
//      without each feature knowing about the editor page.
//      Deregisters on unmount or when the editor binding goes away.
//
//   2. recordOpenNote — feeds the global open-note tray (mounted in
//      the root layout) so a "jump back" chip can render from
//      anywhere. Tracks note.path so a same-note refresh (WS reload)
//      doesn't re-write the entry on every server bounce — only
//      navigation does.
//
//   3. beforeunload — synchronously snapshots scroll position before
//      a tab close + prompts when a dirty save would be lost. The
//      draft layer protects against actual data loss on the worst
//      case; this just gives the browser a moment to flush.
//
//   4. beforeNavigate — SPA-internal sibling of beforeunload.
//      Fire-and-forget save + scroll-position remember. We can't
//      synchronously block the navigation (await isn't honoured by
//      browser navigations).
//
// Bundled into one install so the page just lists deps once.
// `beforeNavigate` is a SvelteKit hook that must be called during
// component init; `$effect` registrations bind to the calling
// component's lifecycle — both match the existing installXxx pattern
// in $lib/notes/noteAutosave / noteKeyboardShortcuts.

import { beforeNavigate } from '$app/navigation';
import { rememberScroll } from '$lib/notes/noteHistory';
import { recordOpenNote, updateOpenNoteScroll } from '$lib/stores/open-note';
import { registerActiveEditor } from '$lib/stores/active-editor';
import type { EditorView } from '@codemirror/view';
import type { Note } from '$lib/api';

export interface NoteLifecycleDeps {
  getNote: () => Note | null;
  getDirty: () => boolean;
  getSaving: () => boolean;
  getEditorView: () => EditorView | undefined;
  getScrollTop: () => number | undefined;
  /** Fire-and-forget save invoked from beforeNavigate. */
  save: (opts: { silent: boolean }) => Promise<boolean>;
}

export function installNoteLifecycleEffects(deps: NoteLifecycleDeps): void {
  // 1. Active editor registration.
  $effect(() => {
    const view = deps.getEditorView();
    if (!view) return;
    registerActiveEditor(view);
    return () => registerActiveEditor(null);
  });

  // 2. Open-note tray.
  $effect(() => {
    const note = deps.getNote();
    if (!note) return;
    recordOpenNote({
      path: note.path,
      title: note.title || note.path,
      scrollPos: deps.getScrollTop() ?? 0
    });
  });

  // 3. beforeunload — synchronous scroll snapshot + dirty prompt.
  $effect(() => {
    if (typeof window === 'undefined') return;
    const handler = (e: BeforeUnloadEvent) => {
      const note = deps.getNote();
      const top = deps.getScrollTop();
      if (note && top !== undefined) {
        rememberScroll(note.path, top);
      }
      if (deps.getDirty()) {
        e.preventDefault();
        e.returnValue = '';
      }
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  });

  // 4. beforeNavigate — SPA-internal sibling of beforeunload.
  beforeNavigate(() => {
    const note = deps.getNote();
    if (deps.getDirty() && !deps.getSaving() && note) {
      // Body is already in localStorage via setDraft (synchronous
      // per-keystroke write — see the draft effect comment in
      // noteAutosave). Fire-and-forget the save; it'll race the
      // navigation but either outcome is safe (draft still on disk).
      void deps.save({ silent: true });
    }
    // Remember the scroll position so navigating back to this note
    // returns to where the user was reading. Saved synchronously so
    // even a forced reload catches it. We mirror the value onto the
    // open-note tray entry so the (optional) "resume at line N" hint
    // can render without consulting noteHistory.
    const top = deps.getScrollTop();
    if (note && top !== undefined) {
      rememberScroll(note.path, top);
      updateOpenNoteScroll(note.path, top);
    }
  });
}
