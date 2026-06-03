// Resolve a [[Wikilink]] click to a navigation. Searches the index
// for an exact title match (falls back to the top fuzzy hit) and
// navigates to that note's path. When nothing matches we offer the
// AI-chapter-generation flow before falling back to "create empty
// note at <title>.md" — this is the only navigation site that gets
// that offer because it's the only one with the parent note's body
// to seed as context.
//
// Block-level wikilinks ([[Note#Heading]]) carry the fragment
// through the URL hash so the receiving page can scroll to the
// matching heading after mount.

import { goto } from '$app/navigation';
import { api } from '$lib/api';
import { offerAIChapterGeneration } from '$lib/notes/aiChapterGeneration';

export interface WikilinkNavCtx {
  /** Path of the note holding the link — seeds the AI chapter
   *  generation context. Empty string when no note is loaded. */
  parentPath: string;
  /** Body of the parent note — passed verbatim to the AI offer so
   *  it can ground the new chapter in the surrounding outline. */
  parentBody: string;
}

export async function navigateWikilink(target: string, ctx: WikilinkNavCtx): Promise<void> {
  // [[Note#Heading]] — split off the fragment and pass it through
  // the URL hash. The receiving page reads $page.url.hash and
  // scrolls to the heading after the doc loads.
  const [titleRaw, ...frag] = target.split('#');
  const title = titleRaw.trim();
  const hash = frag.length > 0 ? `#${frag.join('#').trim()}` : '';
  try {
    const list = await api.listNotes({ q: title, limit: 5 });
    const exact = list.notes.find((n) => n.title.toLowerCase() === title.toLowerCase());
    const t = exact ?? list.notes[0];
    if (t) {
      goto(`/notes/${encodeURIComponent(t.path)}${hash}`);
      return;
    }
  } catch {}
  // Unresolved wikilink — offer AI chapter generation before
  // falling back to "open empty note". The LITERAL targetPath
  // (<title>.md at the vault root) ensures the saved chapter
  // lands where the wikilink will resolve on the next click.
  const targetPath = title + '.md';
  const handled = await offerAIChapterGeneration({
    parentPath: ctx.parentPath,
    parentBody: ctx.parentBody,
    chapterTitle: title,
    targetPath
  });
  if (handled) return;
  goto(`/notes/${encodeURIComponent(title + '.md')}${hash}`);
}
