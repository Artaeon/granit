// Body+frontmatter save path. PUT with optimistic-concurrency
// (If-Match etag); a 412 surfaces as a sticky conflict banner so the
// user can choose Reload or Overwrite. Surgical mutation of the
// returned note keeps every panel keyed off note.path stable — a
// full reassignment fanned out a re-render across ~10 sibling
// panels and was the load-bearing piece of the previously-reported
// "after autosave everything freezes" symptom.

import { api, ApiError } from '$lib/api';
import { setDraft, clearDraft } from '$lib/notes/drafts';
import { toast } from '$lib/components/toast';
import type { SaveFrontmatterState as SaveState, SaveFrontmatterCtx as SaveCtx } from '$lib/notes/saveFrontmatter';

// Reuse the same proxy shape — saveNote and saveFrontmatter touch
// the exact same set of $state fields. Naming them identically
// keeps a single page-side proxy serving both.
export type { SaveState, SaveCtx };

export interface SaveOpts {
  silent?: boolean;
}

export async function saveNote(
  opts: SaveOpts,
  s: SaveState,
  ctx: SaveCtx,
  /** Optional hook called after the body+modTime have been mutated
   *  onto the note proxy. Currently used by the page to flip
   *  `draftRestored` off — kept as a callback so this module
   *  stays unaware of UI-only flags. */
  onSavedClean?: () => void
): Promise<boolean> {
  if (!s.note || !s.dirty) return !s.dirty;
  // Initial guard: another save() already started.
  // Mirrors the original `if (saving) return` short-circuit.
  // (The page coordinates concurrent save() calls — only one fires
  //  at a time via the autosave debounce + explicit save buttons.)

  // Capture body and note at start so navigation/typing during the
  // await can be detected and handled correctly.
  const sentBody = s.body;
  const savedNote = s.note;
  // Optimistic-concurrency token. `forceNextSave` is the user's
  // one-shot "overwrite anyway" opt-in after the conflict banner.
  const etagToSend = s.forceNextSave ? undefined : (s.noteEtag ?? undefined);

  // Pair the saving-flag set with the finally-block restore so an
  // unexpected synchronous throw before the await still releases the
  // gate. Save is single-flighted at the call-site (the dirty effect
  // bails on s.saving=true) so racing concurrent saves are not a
  // concern.
  s.error = '';
  s.saving = true;
  try {
    const { data: updated, etag: newEtag } = await api.putNoteWithEtag(
      s.note.path,
      { frontmatter: s.note.frontmatter as Record<string, unknown>, body: sentBody },
      etagToSend
    );
    // Navigation guard: if the user moved to another note while we
    // were awaiting, the server-side save still succeeded — we just
    // stop applying its response to the active state.
    if (!s.note || s.note !== savedNote) {
      return true;
    }
    s.noteEtag = newEtag;
    s.forceNextSave = false;
    s.pendingFrontmatter = null;
    // ─────────────────────────────────────────────────────────────────
    // CRITICAL: surgical property mutation instead of `note = updated`.
    // Full reassignment invalidates every reactive consumer of `note`
    // even when only modTime changed, fanning out a wave of work
    // across ~10 sibling panels. Svelte 5's per-property reactivity
    // means writing just the changed fields keeps panel identity
    // stable through autosave. See the page's save-effects block for
    // the long-form history.
    s.note.modTime = updated.modTime;
    s.note.size = updated.size;
    s.note.links = updated.links;
    s.note.title = updated.title;
    s.prev = sentBody;
    // Editor's view state wins over the bind:value mirror — bind
    // writes from CodeMirror's updateListener are microtask-deferred
    // and lag during heavy reactive cascades.
    const liveNow = ctx.getLiveBody();
    s.dirty = liveNow !== sentBody;
    s.lastSavedAt = Date.now();
    s.saveFailed = false;
    s.saveFailCount = 0;
    s.lastSaveError = '';
    if (!s.dirty) {
      clearDraft(updated.path);
      s.lastDraftedBody = null;
    } else {
      // User typed during the save. Refresh the draft synchronously
      // with the post-save modTime so a crash / reload in the next
      // debounce window doesn't trip the "server has newer content"
      // branch on reload.
      setDraft(updated.path, liveNow, updated.modTime);
    }
    onSavedClean?.();
    if (!opts.silent && !s.dirty) toast.success('saved');
    return true;
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    if (e instanceof ApiError && e.status === 412) {
      // 412 Precondition Failed — the file on disk moved forward.
      // Leave the note dirty + etag stale; the banner's "Overwrite"
      // re-fires with forceNextSave=true, "Reload" calls load(force).
      // Don't increment saveFailCount — this is a state mismatch
      // requiring explicit user input, not a transient failure.
      s.saveFailed = true;
      s.lastSaveError = 'Conflict: this note was changed elsewhere since you loaded it.';
      if (!opts.silent) {
        toast.warning('Conflict: this note was changed elsewhere. Choose Reload or Overwrite in the banner.');
      }
      return false;
    }
    s.error = msg;
    s.saveFailed = true;
    s.saveFailCount++;
    s.lastSaveError = msg;
    // Explicit save → always toast. Silent autosave → toast only on
    // the FIRST failure of a burst (count transitioned 0 → 1 by the
    // increment above); subsequent silent failures stay quiet — the
    // sticky banner is their surface.
    if (!opts.silent || s.saveFailCount === 1) {
      toast.error(`save failed: ${msg}`);
    }
    return false;
  } finally {
    s.saving = false;
  }
}
