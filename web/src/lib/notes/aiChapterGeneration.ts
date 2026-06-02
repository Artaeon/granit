// AI-chapter generation helper for wikilink → missing-note navigation.
//
// When the user clicks a wikilink whose target doesn't exist, the
// route page asks this helper first. If the current note looks like
// an outline (>= 3 wikilinks AND >= 80 chars of prose), we offer to
// generate the chapter with AI using the parent as context. The
// user can confirm (we POST /notes/generate-chapter and navigate to
// the result) or decline (caller falls through to the standard
// "open empty note" flow).
//
// Module-scope `inFlight` gate: the LLM round-trip can take 30+
// seconds. A second click during that window would kick off a
// duplicate generation. One notes page mounts at a time in the SPA,
// so module scope is sufficient — no need to thread state through
// the page or worry about cross-instance leakage.
//
// Return convention: `true` means the helper handled the navigation
// (success, re-entry blocked, or anything where the caller should
// NOT continue). `false` means "fall through to your default
// open-empty path" — used for: user declined, prose too short to
// ground a generation, API error.

import { goto } from '$app/navigation';
import { api } from '$lib/api';
import { toast } from '$lib/components/toast';
import { errorMessage } from '$lib/util/errorMessage';

export interface OfferChapterOpts {
  /** The parent note's path — passed to the server as the context source. */
  parentPath: string;
  /** The parent's current body. We heuristic-gate on it (>= 3 wikilinks
   *  + >= 80 chars) and pass implicitly via parentPath; the server
   *  reads the file. */
  parentBody: string;
  /** Title of the missing note the user clicked. Used in the confirm
   *  dialog and the toast caption. */
  chapterTitle: string;
  /** Resolved target path — the server saves at this exact path
   *  rather than reinventing the routing logic. */
  targetPath: string;
}

let inFlight = false;

export async function offerAIChapterGeneration(opts: OfferChapterOpts): Promise<boolean> {
  const { parentPath, parentBody, chapterTitle, targetPath } = opts;
  if (!parentPath) return false;
  // Re-entry guard: chapter generation can take 30+ seconds. While
  // it's in flight the wikilink stays clickable and the user might
  // double-tap or click a different wikilink, kicking off a second
  // generation. The persistent toast (sticky ttl=0) tells the user
  // the request is alive; this flag prevents duplicate fires.
  if (inFlight) {
    toast.info('Another chapter is still generating — please wait.');
    return true;
  }
  // Heuristic gate: parent must have at least 3 wikilinks (so it
  // really is a TOC-like note) and >= 80 chars of non-fluff
  // content. Below that the model has nothing to ground in and
  // we'd produce a generic chapter anyway.
  const wikilinkCount = (parentBody.match(/\[\[/g) ?? []).length;
  if (wikilinkCount < 3 || parentBody.trim().length < 80) return false;
  const ok = confirm(
    `The note "${chapterTitle}" doesn't exist yet.\n\n` +
      `Generate it with AI using this outline as context?\n\n` +
      `(This typically takes 15-60 seconds — a banner will show progress.)`
  );
  if (!ok) {
    // User explicitly declined — fall through to the standard
    // "open empty" path so they can write the chapter manually.
    return false;
  }
  inFlight = true;
  // Sticky toast (ttl=0) so the user sees ongoing progress instead
  // of a fire-and-forget message that would vanish mid-call,
  // leaving them staring at a still page wondering if the app froze.
  const toastId = toast.info(`Generating "${chapterTitle}"…`, { ttl: 0 });
  try {
    const r = await api.generateChapter({
      parentPath,
      chapterTitle,
      targetPath, // critical: tells the server where to save
      save: true
    });
    if (r.path) {
      toast.success(`Wrote ${r.path}`);
      goto(`/notes/${encodeURIComponent(r.path)}`);
      return true;
    }
    // No path returned — server didn't save for some reason.
    // Fall through to empty-note creation so the user isn't stranded.
    return false;
  } catch (e) {
    toast.error('Generation failed: ' + errorMessage(e));
    return false;
  } finally {
    toast.dismiss(toastId);
    inFlight = false;
  }
}
