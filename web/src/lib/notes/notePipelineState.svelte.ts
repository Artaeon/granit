// Note-pipeline state cluster for the notes editor route.
//
// Owns every $state slot that the save / load / autosave / wsReload /
// frontmatter helpers in $lib/notes read or write — 17 fields that
// previously sat inline at the top of the +page.svelte and were
// re-exposed as a hand-rolled getter/setter proxy. Centralising them
// here:
//
//   1. drops ~70 LOC of route boilerplate.
//   2. gives every helper a single typed contract to bind against
//      (SaveState / SaveFrontmatterState / LoadNoteState all match
//      the same shape — see saveNote.ts / saveFrontmatter.ts /
//      loadNote.ts).
//   3. keeps the conflictDetected derivation next to the failure
//      fields it consumes.
//
// The controller IS the proxy — each property is a getter+setter
// pair backed by $state, so the route reads `pipe.body` reactively,
// writes via `pipe.body = ...`, and bindings work with
// `bind:value={pipe.body}` (Svelte 5 honours get+set property pairs
// as l-values). The saveNote / saveFrontmatter / loadNote helpers
// accept the same controller as their state argument.

import type { Note } from '$lib/api';

/** The exact field set the save / load / frontmatter helpers in
 *  $lib/notes consume. Keep in lockstep with SaveState in saveNote.ts
 *  and SaveFrontmatterState in saveFrontmatter.ts — all three
 *  surfaces share this single proxy shape. */
export interface NotePipelineState {
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
  notFound: boolean;
  lastLoadedPath: string;
  draftRestored: boolean;
}

export interface NotePipelineController extends NotePipelineState {
  /** True when the last save error came back as a 412 Precondition
   *  Failed. Drives the in-page conflict banner. */
  readonly conflictDetected: boolean;
}

export function createNotePipelineState(): NotePipelineController {
  let note = $state<Note | null>(null);
  let body = $state('');
  let prev = $state('');
  let saving = $state(false);
  let dirty = $state(false);
  let error = $state('');
  let lastSavedAt = $state<number | null>(null);
  // ETag from the most recent successful load / save. Sent as
  // `If-Match` on every PUT so a concurrent edit from another tab /
  // TUI / sync surfaces as a 412 instead of being silently
  // overwritten. Reset to null whenever the active note changes —
  // a fresh load() refills it from the response. Bumped on every
  // successful save() so a long edit session stays anchored to the
  // latest server state.
  let noteEtag = $state<string | null>(null);
  // When the user has chosen to overwrite a detected conflict, the
  // next save skips the If-Match header. The flag clears after one
  // save so a subsequent edit is again guarded.
  let forceNextSave = $state(false);
  // Frontmatter that hit 412 — held verbatim so the conflict
  // banner's Overwrite button can re-run saveFrontmatter() with the
  // original payload + forceNextSave. Without this, an Overwrite
  // after a tag-chip conflict ran the body-save path and silently
  // dropped the pending frontmatter edit.
  let pendingFrontmatter = $state<Record<string, unknown> | null>(null);
  let saveFailed = $state(false);
  // Consecutive save-failure counter. Resets to 0 on any success.
  // Used by the sticky in-page banner so the user always knows when
  // their edits aren't reaching the server.
  let saveFailCount = $state(0);
  let lastSaveError = $state('');
  // Draft watermark — same body the last setDraft() call wrote, so a
  // body unchanged since the last write doesn't re-stringify into
  // localStorage. Shared between save() (clears on success) and
  // installNoteAutosave (rAF-coalesced writes).
  let lastDraftedBody = $state<string | null>(null);
  // True when the requested path 404s on load. Distinct from `error`
  // so the page renders a "create this note?" affordance instead of
  // an error banner — a 404 here is almost always the user
  // following an unresolved wikilink or typing a URL for a note
  // they're about to create.
  let notFound = $state(false);
  let lastLoadedPath = $state('');
  let draftRestored = $state(false);

  // The 412 catch branch in saveNote sets lastSaveError to a string
  // starting with "Conflict:"; nothing else uses that prefix.
  const conflictDetected = $derived(
    saveFailed && lastSaveError.startsWith('Conflict')
  );

  return {
    get note() { return note; }, set note(v) { note = v; },
    get body() { return body; }, set body(v) { body = v; },
    get prev() { return prev; }, set prev(v) { prev = v; },
    get saving() { return saving; }, set saving(v) { saving = v; },
    get dirty() { return dirty; }, set dirty(v) { dirty = v; },
    get error() { return error; }, set error(v) { error = v; },
    get lastSavedAt() { return lastSavedAt; }, set lastSavedAt(v) { lastSavedAt = v; },
    get noteEtag() { return noteEtag; }, set noteEtag(v) { noteEtag = v; },
    get forceNextSave() { return forceNextSave; }, set forceNextSave(v) { forceNextSave = v; },
    get pendingFrontmatter() { return pendingFrontmatter; }, set pendingFrontmatter(v) { pendingFrontmatter = v; },
    get saveFailed() { return saveFailed; }, set saveFailed(v) { saveFailed = v; },
    get saveFailCount() { return saveFailCount; }, set saveFailCount(v) { saveFailCount = v; },
    get lastSaveError() { return lastSaveError; }, set lastSaveError(v) { lastSaveError = v; },
    get lastDraftedBody() { return lastDraftedBody; }, set lastDraftedBody(v) { lastDraftedBody = v; },
    get notFound() { return notFound; }, set notFound(v) { notFound = v; },
    get lastLoadedPath() { return lastLoadedPath; }, set lastLoadedPath(v) { lastLoadedPath = v; },
    get draftRestored() { return draftRestored; }, set draftRestored(v) { draftRestored = v; },
    get conflictDetected() { return conflictDetected; }
  };
}
