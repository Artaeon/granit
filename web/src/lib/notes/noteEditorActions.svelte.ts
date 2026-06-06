// Small route-side action helpers for the notes editor: thin glue
// around existing $lib/notes helpers + the editor handle. Pulled
// together so the page's mid-script "miscellaneous functions" block
// shrinks to one controller call.
//
//   saveFrontmatter     — same conflict / draft / surgical-mutation
//                         contract as saveFrontmatter.ts; this just
//                         binds the pipe + saveCtx.
//   navigateWikilink    — best-effort dirty flush + wikilinkNav lookup
//                         + goto. The actual lookup + AI-offer +
//                         goto flow lives in $lib/notes/wikilinkNav.
//   openResearchMode    — forwards the active note + body to the
//                         researchMode seeder.
//   jumpToLine          — drives the editor's scrollToLine and closes
//                         the right info-drawer (mobile / tablet).

import { navigateWikilink as navigateWikilinkHelper } from '$lib/notes/wikilinkNav';
import { openResearchMode as openResearchModeFor } from '$lib/notes/researchMode';
import { saveFrontmatter as saveFrontmatterFn } from '$lib/notes/saveFrontmatter';
import type { NotePipelineController } from '$lib/notes/notePipelineState.svelte';
import type { EditorHandle } from '$lib/notes/editorHandle';
import type { NoteEditorOverlays } from '$lib/notes/noteEditorOverlays.svelte';

export interface NoteEditorActions {
  saveFrontmatter: (next: Record<string, unknown>) => Promise<boolean>;
  navigateWikilink: (target: string) => Promise<void>;
  openResearchMode: () => void;
  jumpToLine: (lineNum: number) => void;
}

export interface NoteEditorActionsOpts {
  pipe: NotePipelineController;
  overlays: NoteEditorOverlays;
  getEditor: () => EditorHandle | undefined;
  /** Best-effort save invoked from navigateWikilink. */
  save: (opts: { silent?: boolean }) => Promise<boolean>;
}

export function createNoteEditorActions(
  opts: NoteEditorActionsOpts
): NoteEditorActions {
  const saveCtx = {
    getLiveBody: () => opts.getEditor()?.getContent?.() ?? opts.pipe.body
  };

  async function saveFrontmatter(
    next: Record<string, unknown>
  ): Promise<boolean> {
    return saveFrontmatterFn(next, opts.pipe, saveCtx);
  }

  async function navigateWikilink(target: string): Promise<void> {
    // Best-effort flush of any pending edit. We never block navigation
    // on the save result — the localStorage draft already preserves
    // the body and beforeNavigate flushes again. If offline, save
    // will fail; the draft is on disk and gets retried automatically
    // when 'online' fires. The lookup + AI-offer + goto flow lives
    // in wikilinkNav so this surface only owns the dirty-flush + ctx
    // wiring.
    if (opts.pipe.dirty) void opts.save({ silent: true });
    await navigateWikilinkHelper(target, {
      parentPath: opts.pipe.note?.path ?? '',
      parentBody: opts.pipe.body ?? ''
    });
  }

  function openResearchMode(): void {
    const note = opts.pipe.note;
    if (!note) return;
    openResearchModeFor(note, opts.pipe.body);
  }

  function jumpToLine(lineNum: number): void {
    opts.getEditor()?.scrollToLine(lineNum);
    opts.overlays.infoDrawerOpen = false;
  }

  return {
    saveFrontmatter,
    navigateWikilink,
    openResearchMode,
    jumpToLine
  };
}
