// Thin route-side adapter around $lib/notes/loadNote that closes
// over the editor handle, drawer state, breadcrumbs controller, and
// URL accessor — same shape the route used to inline at the call
// site. Lifted here so the route's load() reads as one line.
//
// loadNote.ts owns the draft-reconciliation contract, the "always
// prefer the draft" rule on divergence, and the 404 / network-error
// fallbacks; this is purely a deps-passing convenience.

import { loadNote as loadNoteFn } from '$lib/notes/loadNote';
import type { NotePipelineController } from '$lib/notes/notePipelineState.svelte';
import type { EditorHandle } from '$lib/notes/editorHandle';
import type { NoteEditorOverlays } from '$lib/notes/noteEditorOverlays.svelte';
import type { NoteBreadcrumbsController } from '$lib/notes/noteBreadcrumbsCtl.svelte';

export interface LoadNoteWrapperOpts {
  pipe: NotePipelineController;
  overlays: NoteEditorOverlays;
  breadcrumbs: NoteBreadcrumbsController;
  getEditor: () => EditorHandle | undefined;
  getLineParam: () => string | null;
  getRawHash: () => string;
  save: (opts: { silent?: boolean }) => Promise<boolean>;
}

export function createLoadNoteWrapper(opts: LoadNoteWrapperOpts) {
  return async function load(
    p: string,
    o: { force?: boolean } = {}
  ): Promise<unknown> {
    const editor = opts.getEditor();
    return loadNoteFn(p, o, opts.pipe, {
      getLiveBody: () => editor?.getContent?.() ?? opts.pipe.body,
      getEditorView: () => editor?.getView?.(),
      scrollToLine: (n) => editor?.scrollToLine?.(n),
      setScrollTop: (top) => editor?.setScrollTop?.(top),
      closeDrawers: () => {
        opts.overlays.treeDrawerOpen = false;
        opts.overlays.infoDrawerOpen = false;
      },
      setBreadcrumbExpanded: (v) => {
        if (v) opts.breadcrumbs.expand();
        else opts.breadcrumbs.reset();
      },
      getLineParam: opts.getLineParam,
      getRawHash: opts.getRawHash,
      save: opts.save
    });
  };
}
