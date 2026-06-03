// Frontmatter-only save path. Uses the same PUT-with-If-Match
// optimistic-concurrency contract as the body save() — same 412
// banner flow, same surgical mutation strategy that avoids
// re-rendering every panel keyed off note.path. The split exists so
// a tag-chip edit doesn't have to wait for whatever body keystrokes
// the autosave debounce is queueing, and so a 412 on a frontmatter
// edit re-routes back through this path on "Overwrite anyway"
// instead of bouncing through the body save (which would silently
// drop the pending frontmatter).

import { api, ApiError, type Note } from '$lib/api';
import { setDraft, clearDraft } from '$lib/notes/drafts';
import { toast } from '$lib/components/toast';

/** Mutable view onto the page's reactive save state. Every property
 *  must be a $state binding (getter/setter) — the helper mutates
 *  through it and the page's reactivity picks the changes up.
 *  `note` is the proxied $state object; mutating its fields is the
 *  load-bearing optimisation that keeps panels stable through a
 *  save (full reassignment fans out a render across ~10 panels). */
export interface SaveFrontmatterState {
  note: Note | null;
  body: string;
  prev: string;
  saving: boolean;
  dirty: boolean;
  error: string;
  lastSavedAt: number | null;
  noteEtag: string | null;
  forceNextSave: boolean;
  pendingFrontmatter: Record<string, unknown> | null;
  saveFailed: boolean;
  saveFailCount: number;
  lastSaveError: string;
  lastDraftedBody: string | null;
}

export interface SaveFrontmatterCtx {
  /** Editor liveness — read CodeMirror's state.doc directly. The
   *  body mirror is microtask-deferred and can lag mid-edit. */
  getLiveBody: () => string;
}

export async function saveFrontmatter(
  next: Record<string, unknown>,
  s: SaveFrontmatterState,
  ctx: SaveFrontmatterCtx
): Promise<boolean> {
  if (!s.note) return false;
  // Snapshot what we sent so a body keystroke during the await
  // doesn't get falsely marked clean below.
  const sentBody = s.body;
  const savedNote = s.note;
  const etagToSend = s.forceNextSave ? undefined : (s.noteEtag ?? undefined);
  try {
    const { data: updated, etag: newEtag } = await api.putNoteWithEtag(
      s.note.path,
      { frontmatter: next, body: sentBody },
      etagToSend
    );
    if (!s.note || s.note !== savedNote) return false;
    // Surgical mutation (same invariant as save()) — full reassignment
    // would fan out a re-render across every panel keyed off note.path.
    s.note.frontmatter = updated.frontmatter;
    s.note.modTime = updated.modTime;
    s.note.size = updated.size;
    s.note.title = updated.title;
    s.noteEtag = newEtag;
    s.forceNextSave = false;
    s.pendingFrontmatter = null;
    // Clear failure flags unconditionally on a successful PUT — gating
    // these on `!dirty` left saveFailed=true sticky whenever a
    // frontmatter save resolved a conflict but the user happened to
    // keep typing in the body.
    s.saveFailed = false;
    s.saveFailCount = 0;
    s.lastSaveError = '';
    const liveNow = ctx.getLiveBody();
    s.dirty = liveNow !== sentBody;
    s.prev = sentBody;
    if (!s.dirty) {
      clearDraft(updated.path);
      s.lastDraftedBody = null;
      s.lastSavedAt = Date.now();
    } else {
      setDraft(updated.path, liveNow, updated.modTime);
    }
    return true;
  } catch (e) {
    if (e instanceof ApiError && e.status === 412) {
      // Hold the pending frontmatter so the banner's "Overwrite anyway"
      // routes BACK through saveFrontmatter — not the body-save path —
      // preserving the user's tag/field change.
      s.pendingFrontmatter = next;
      s.saveFailed = true;
      s.lastSaveError = 'Conflict: this note was changed elsewhere since you loaded it.';
      toast.warning('Conflict: this note was changed elsewhere. Choose Reload or Overwrite in the banner.');
      return false;
    }
    s.error = e instanceof Error ? e.message : String(e);
    return false;
  }
}
