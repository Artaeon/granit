// Inline note-embed hydrator for MarkdownRenderer.
//
// The transforms module lays down a `<div class="transclude-card">`
// placeholder for every `![[path]]` in the source. This module walks
// those placeholders post-render and swaps them for the actual
// rendered body of the linked note, wrapped in an <aside class=
// "embed-card">.
//
// Recursion is bounded: we strip `![[…]]` from the embedded content
// before parsing it, so A→B→A or any cycle becomes a plain wikilink
// card inside the embedded content — no fetch storm, no infinite
// loop. One level of embed is the documented contract.
//
// embedCache is module-scope so a note that appears in multiple
// embed sites (or remounts of the same renderer) reuses the parsed
// HTML across calls. The cache is keyed by the raw wikilink target
// string the user wrote, after stripping a trailing `.md` — the
// same normalisation listNotes accepts as a search query.
//
// The hydrator depends on:
//   - the parent's container ref (for the DOM walk),
//   - marked + DOMPurify-wrapped purify (for the recursive render),
//   - the transforms preprocess/postprocess pair.
// These come in via `opts` so this file has zero dependency on
// MarkdownRenderer's reactive scope.

import { marked } from 'marked';
import { api } from '$lib/api';
import { preprocess, postprocess, escAttr, escHtml } from './transforms';

export interface EmbedHydratorOptions {
  /** Element to scan for `.transclude-card` placeholders. */
  getContainer: () => HTMLElement | undefined;
  /** Final sanitisation pass applied to the recursive embed render
   *  — the embed-card body is innerHTML'd, which doesn't re-trigger
   *  framework defences, so we sanitise here BEFORE the assignment. */
  purify: (html: string) => string;
}

/** Module-scope cache: raw wikilink target → final embed body HTML
 *  (post-purify). Survives across all MarkdownRenderer instances and
 *  remounts. Empty-string value means "no match found"; we cache the
 *  negative so an unresolved embed doesn't refire api.listNotes on
 *  every render. */
const embedCache = new Map<string, string>();

/** Build a hydrator bound to a specific MarkdownRenderer instance's
 *  container. Returns a single hydrateEmbeds() function the caller
 *  can debounce + invoke after every html update. */
export function createEmbedHydrator(opts: EmbedHydratorOptions) {
  async function hydrateEmbeds(): Promise<void> {
    const container = opts.getContainer();
    if (!container) return;
    const cards = Array.from(container.querySelectorAll('.transclude-card'));
    if (cards.length === 0) return;
    for (const card of cards) {
      const link = card.querySelector('[data-wikilink]') as HTMLElement | null;
      if (!link) continue;
      const target = link.getAttribute('data-wikilink');
      if (!target) continue;
      try {
        let bodyHtml = embedCache.get(target);
        if (bodyHtml === undefined) {
          // Resolve the target → path. listNotes search is forgiving;
          // we accept the first hit. A `.md` suffix already makes
          // exact-path links work directly via the backend.
          const list = await api.listNotes({ q: target.replace(/\.md$/, ''), limit: 5 });
          const exact =
            list.notes.find((n) => n.title.toLowerCase() === target.toLowerCase()) ??
            list.notes.find((n) => n.path === target || n.path === `${target}.md`);
          const note = exact ?? list.notes[0];
          if (!note) {
            embedCache.set(target, '');
            continue;
          }
          const full = await api.getNote(note.path);
          // Strip ![[…]] from the embed's own body before parsing
          // so a cycle (A→B→A) doesn't snowball. One level of embed
          // is the documented depth; deeper embeds appear as plain
          // wikilink cards inside the embedded content.
          const stripped = (full.body ?? '').replace(/!\[\[[^\]]+\]\]/g, '');
          // Recursive render via the same transforms pipeline. Each
          // call gets its own TransformState so the inner render
          // can't trample the outer's footnote refs / diagram caches.
          //
          // CRITICAL: async parse. The previous { async: false }
          // synchronously blocked the main thread per embed for the
          // entire marked.parse + DOMPurify pass — hundreds of ms
          // each on a large embedded note, serialized inside the
          // `for` loop. Clicking "read view" on a note with even one
          // big embed froze the UI; the user-visible "freeze when I
          // click read view" bug we kept chasing was here.
          const { preprocessed, state } = preprocess(stripped);
          const rawHtml = (await marked.parse(preprocessed, { async: true })) as string;
          bodyHtml = opts.purify(postprocess(rawHtml, state, stripped));
          embedCache.set(target, bodyHtml);
        }
        if (!bodyHtml) continue;
        const aside = document.createElement('aside');
        aside.className = 'embed-card';
        aside.innerHTML =
          `<header class="embed-card__header">` +
          `<span class="embed-card__label">embedded</span>` +
          `<a class="wikilink" data-wikilink="${escAttr(target)}">${escHtml(target)}</a>` +
          `</header>` +
          `<div class="embed-card__body prose-note">${bodyHtml}</div>`;
        card.replaceWith(aside);
      } catch {
        // Leave the card in place on failure so the user still has a
        // clickable target; the placeholder shape ("embed" label +
        // wikilink) already conveys the intent.
      }
    }
  }
  return { hydrateEmbeds };
}
