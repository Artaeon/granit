// Load a note from the server, reconciling against any local draft
// and honouring the same etag-based optimistic-concurrency contract
// the save path uses. Three correctness lessons are baked into this
// flow and listed where they live:
//
// 1. Snapshot the editor's live content before the network call. The
//    bind:value `body` mirror is microtask-deferred and can lag the
//    actual CodeMirror state; reading the lag-prone value would
//    false-positive a "no in-flight edits" check on a WS-triggered
//    same-note reload and silently overwrite the user's keystrokes.
//
// 2. Always prefer the local draft when it diverges from the server,
//    even when the server modTime is newer — the most common reason
//    for that combination is "user typed during the autosave PUT,
//    and the draft on disk has those after-PUT keystrokes". Toasting
//    a warning when the modTime is newer keeps the rare cross-device
//    conflict visible.
//
// 3. On 404 or network error, surface a local draft if one exists,
//    and DROP the dedupe guard so a refetch of the same path is
//    allowed — otherwise the user is stuck on the error banner with
//    no way to retry short of a full reload.

import { api, ApiError, type Note } from '$lib/api';
import { getDraft, clearDraft, draftDivergesFromServer } from '$lib/notes/drafts';
import { recallScroll } from '$lib/notes/noteHistory';
import { rejectInlineAI } from '$lib/editor/inline-ai';
import { toast } from '$lib/components/toast';
import type { EditorView } from '@codemirror/view';
import type { SaveFrontmatterState } from '$lib/notes/saveFrontmatter';

/** Mutable view onto the page's load-side $state — superset of the
 *  save proxy (load resets save-side flags on success) plus the
 *  load-specific bits. */
export interface LoadState extends SaveFrontmatterState {
  notFound: boolean;
  lastLoadedPath: string;
  draftRestored: boolean;
}

export interface LoadCtx {
  /** Editor liveness — same shape as saveCtx. */
  getLiveBody: () => string;
  /** Editor view for the rejectInlineAI call on real navigation. */
  getEditorView: () => EditorView | undefined;
  /** Scroll-to-line on the editor (used by ?line=N and #heading). */
  scrollToLine: (n: number) => void;
  /** Scroll restore for the remembered position. */
  setScrollTop: (top: number) => void;
  /** Drawer close — the user opens trees / info on a drawer on
   *  mobile; landing on a new note must close them. */
  closeDrawers: () => void;
  /** breadcrumbExpanded reset on real navigation. */
  setBreadcrumbExpanded: (v: boolean) => void;
  /** ?line=N and #heading sources — URL searchParams + hash. */
  getLineParam: () => string | null;
  getRawHash: () => string;
  /** Trailing save() reference — used to push a draft-restored body
   *  through the wire on the same tick we restore it. */
  save: (opts: { silent?: boolean }) => Promise<boolean>;
}

export interface LoadOpts {
  force?: boolean;
}

export async function loadNote(
  p: string,
  opts: LoadOpts,
  s: LoadState,
  ctx: LoadCtx
): Promise<void> {
  s.error = '';
  s.notFound = false;
  s.draftRestored = false;
  if (!opts.force && s.lastLoadedPath === p) return;
  // Clear the stale-note concurrency token up-front. If the fetch
  // below throws (network blip, 5xx, abort), the previous note's
  // etag would otherwise linger on the freshly-navigated path and
  // get sent as If-Match on the next save → spurious 412.
  s.noteEtag = null;
  const isSameNoteReload = s.note?.path === p;
  if (!isSameNoteReload) ctx.setBreadcrumbExpanded(false);
  // Cancel any in-flight inline-AI stream on real navigation.
  // Same-note reloads keep the ghost so an unrelated WS rescan
  // can't yank it from under the user.
  if (!isSameNoteReload) {
    const view = ctx.getEditorView();
    if (view) rejectInlineAI(view);
  }
  // Reset the per-load draft watermark so the first keystroke on the
  // newly-opened note triggers a draft write. Without this, opening
  // a note whose body happens to equal the previous note's last
  // drafted body would skip the first draft persistence.
  s.lastDraftedBody = null;
  // Snapshot the EDITOR's content (not the bound `body` mirror) so a
  // WS-triggered same-note reload fired mid-typing can detect "user
  // is editing" without the microtask-deferred bind:value lag.
  const liveAtStart = ctx.getLiveBody();
  s.lastLoadedPath = p;
  try {
    const { data: fresh, etag: freshEtag } = await api.getNoteWithEtag(p);
    if (isSameNoteReload) {
      const liveNow = ctx.getLiveBody();
      if (liveNow !== liveAtStart) return;
    }
    s.noteEtag = freshEtag;
    s.forceNextSave = false;
    const serverBody = fresh.body ?? '';

    // Restore local draft when it diverges from the server — see the
    // file header for the "we always prefer the draft" rationale.
    const draft = getDraft(p);
    if (draft && draftDivergesFromServer(draft, serverBody)) {
      const serverNewer = new Date(fresh.modTime) > new Date(draft.baseModTime);
      s.prev = draft.body;
      s.body = draft.body;
      s.note = fresh;
      s.dirty = true;
      s.draftRestored = true;
      ctx.closeDrawers();
      if (serverNewer) {
        toast.warning(
          'Restored unsaved draft — server moved forward since your last edit. Your version will overwrite on next save.'
        );
      } else {
        toast.info('Restored unsaved draft');
      }
      void ctx.save({ silent: true });
      return;
    } else if (draft) {
      // Draft matches server — stale, clean up.
      clearDraft(p);
      s.lastDraftedBody = null;
    }

    s.note = fresh;
    s.body = serverBody;
    s.prev = serverBody;
    s.dirty = false;
    // Successful load = "anchored to the server again". Reset every
    // error/conflict flag so a stale 412 from a previous version
    // can't keep conflictDetected sticky and silently disable the
    // autosave loop.
    s.saveFailed = false;
    s.saveFailCount = 0;
    s.lastSaveError = '';
    s.pendingFrontmatter = null;
    ctx.closeDrawers();

    // Scroll restoration (per-note, pixel-accurate). Defer a frame so
    // the editor has finished mounting and the scroller has its
    // content height — without the defer setScrollTop lands at 0.
    const remembered = recallScroll(p);
    if (remembered > 0) {
      requestAnimationFrame(() => ctx.setScrollTop(remembered));
    }
    // ?line=N — incoming jump from /search. Wins over remembered
    // scroll so a user clicking a search hit lands on the matched
    // line, not yesterday's reading position.
    const lineParam = ctx.getLineParam();
    if (lineParam) {
      const ln = parseInt(lineParam, 10);
      if (Number.isFinite(ln) && ln > 0) {
        requestAnimationFrame(() => ctx.scrollToLine(ln));
      }
    }
    // [[Note#Heading]] → URL hash carries the heading text.
    // Case-insensitive whitespace-collapsed match against ## lines.
    const rawHash = ctx.getRawHash();
    if (rawHash) {
      const target = rawHash.toLowerCase().replace(/\s+/g, ' ').trim();
      const lines = serverBody.split('\n');
      let found = -1;
      let inFence = false;
      for (let i = 0; i < lines.length; i++) {
        const t = lines[i].trim();
        if (t.startsWith('```') || t.startsWith('~~~')) { inFence = !inFence; continue; }
        if (inFence) continue;
        const m = /^(#{1,6})\s+(.+?)\s*$/.exec(t);
        if (m && m[2].toLowerCase().replace(/\s+/g, ' ').trim() === target) {
          found = i + 1; // CodeMirror is 1-based
          break;
        }
      }
      if (found > 0) {
        requestAnimationFrame(() => ctx.scrollToLine(found));
      }
    }
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    // Local draft as the offline fallback — losing work silently is
    // never the right move.
    const draft = getDraft(p);
    if (draft) {
      s.prev = draft.body;
      s.body = draft.body;
      s.note = {
        path: p,
        title: p.split('/').pop()!.replace(/\.md$/, ''),
        modTime: new Date().toISOString(),
        size: draft.body.length,
        frontmatter: {},
        body: draft.body
      } as Note;
      s.dirty = true;
      s.draftRestored = true;
      toast.warning('offline — showing your local draft');
      return;
    }
    if (e instanceof ApiError && e.status === 404) {
      s.notFound = true;
    } else {
      s.error = msg;
    }
    s.note = null;
    s.body = '';
    s.prev = '';
    s.dirty = false;
    // Drop the dedupe guard so the SAME path can be refetched — the
    // user must be able to retry from the error banner without a
    // full page reload.
    s.lastLoadedPath = '';
  }
}
