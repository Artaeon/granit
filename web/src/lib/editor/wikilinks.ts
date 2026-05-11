import { EditorView, Decoration, ViewPlugin } from '@codemirror/view';
import type { DecorationSet, ViewUpdate } from '@codemirror/view';
import type { Range } from '@codemirror/state';
import type { CompletionContext, CompletionResult } from '@codemirror/autocomplete';
import { api } from '$lib/api';

const wikilinkRe = /\[\[([^\]\n]+?)\]\]/g;
const openWikilinkRe = /\[\[([^\]\n]*)$/;
// When the user has typed a `#` inside an open wikilink, switch from
// title-completion to heading-completion so [[Note#H]] surfaces the
// matching headings of `Note`. Captures: 1) the bare title before `#`
// 2) the heading-fragment query the user has typed so far.
const openHeadingRe = /\[\[([^\]\n#|]+)#([^\]\n|]*)$/;

// Per-occurrence decoration so the rendered span can carry the
// wikilink target as a data-wikilink attribute. WikilinkHoverPreview
// listens for that attribute on its host element, so emitting it on
// the editor surface gives us hover previews in the editor for free
// — same component, same fetch cache, same tooltip UI as the preview
// pane. The class stays for the existing :hover styling rule.
function markFor(target: string) {
  return Decoration.mark({
    class: 'cm-wikilink',
    attributes: { 'data-wikilink': target }
  });
}

export const wikilinkDecoration = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;
    constructor(view: EditorView) {
      this.decorations = this.build(view);
    }
    update(u: ViewUpdate) {
      if (u.docChanged || u.viewportChanged) this.decorations = this.build(u.view);
    }
    build(view: EditorView): DecorationSet {
      const ranges: Range<Decoration>[] = [];
      for (const { from, to } of view.visibleRanges) {
        const text = view.state.doc.sliceString(from, to);
        wikilinkRe.lastIndex = 0;
        let m: RegExpExecArray | null;
        while ((m = wikilinkRe.exec(text)) !== null) {
          const start = from + m.index;
          const end = start + m[0].length;
          // Inner: strip alias (|alias) + block anchor (#Heading) so
          // the hover preview's fetch and the click navigation
          // target identical titles. ([[Note|Alias]] → "Note";
          // [[Note#Heading]] → "Note".)
          const target = m[1].split('|')[0].split('#')[0].trim();
          ranges.push(markFor(target).range(start, end));
        }
      }
      return Decoration.set(ranges, true);
    }
  },
  { decorations: (v) => v.decorations }
);

export function wikilinkClickHandler(navigate: (target: string) => void) {
  return EditorView.domEventHandlers({
    click(e, view) {
      // require modifier so plain clicks just position the cursor
      if (!(e.metaKey || e.ctrlKey)) return false;
      const pos = view.posAtCoords({ x: e.clientX, y: e.clientY });
      if (pos === null) return false;
      const text = view.state.doc.toString();
      wikilinkRe.lastIndex = 0;
      let m: RegExpExecArray | null;
      while ((m = wikilinkRe.exec(text)) !== null) {
        const s = m.index;
        const en = s + m[0].length;
        if (pos >= s && pos <= en) {
          // The | strips the alias form ([[Note|alias]] → "Note");
          // the # carries the heading anchor through to navigate so
          // the host page can scroll to that section after open.
          const target = m[1].split('|')[0].trim();
          if (target) {
            navigate(target);
            e.preventDefault();
            return true;
          }
        }
      }
      return false;
    }
  });
}

// Cache vault note titles for fast autocomplete (single fetch per session).
let titleCache: { title: string; path: string }[] | null = null;
let titleCachePromise: Promise<{ title: string; path: string }[]> | null = null;

// Exported so editor extensions outside this module (autolink-suggest,
// any future similar feature) can share the cache instead of starting
// their own — invalidateTitleCache() drops both consumers in one shot.
export async function ensureTitles() {
  if (titleCache) return titleCache;
  if (titleCachePromise) return titleCachePromise;
  titleCachePromise = (async () => {
    try {
      const list = await api.listNotes({ limit: 5000 });
      titleCache = list.notes.map((n) => ({ title: n.title, path: n.path }));
      return titleCache;
    } finally {
      titleCachePromise = null;
    }
  })();
  return titleCachePromise;
}

// External call to invalidate (e.g., after a new note is created).
export function invalidateTitleCache() {
  titleCache = null;
}

// Headings live in the body of a note rather than the listNotes
// payload, so the editor fetches per-note on first heading-pick.
// Cached at module scope keyed by path so a user typing into
// [[Foo#] followed by [[Foo#Plan]] doesn't re-fetch. Cache is
// dropped when the title cache invalidates (a fresh note edit
// usually means the headings changed too).
const headingCache = new Map<string, string[]>();

/** Resolve a title (possibly the user's free-text input) to a note
 *  path. Used by both the heading completion and the click handler.
 *  Falls back to a "<title>.md" guess if no exact match exists. */
function resolveTitleToPath(title: string): string | null {
  const t = title.trim().toLowerCase();
  if (!titleCache) return null;
  const exact = titleCache.find((x) => x.title.toLowerCase() === t);
  if (exact) return exact.path;
  const partial = titleCache.find((x) => x.title.toLowerCase().includes(t));
  return partial?.path ?? null;
}

/** Read all `# … ###### ` headings out of a markdown blob, normalised
 *  to plain text (no leading hashes). Skips fenced code blocks so a
 *  bash sample with `# comment` doesn't pollute the heading list. */
function extractHeadings(body: string): string[] {
  const out: string[] = [];
  let inFence = false;
  for (const raw of body.split('\n')) {
    const line = raw.trim();
    if (line.startsWith('```') || line.startsWith('~~~')) {
      inFence = !inFence;
      continue;
    }
    if (inFence) continue;
    const m = /^(#{1,6})\s+(.+?)\s*$/.exec(line);
    if (m) out.push(m[2].trim());
  }
  return out;
}

async function fetchHeadingsForTitle(title: string): Promise<string[]> {
  await ensureTitles();
  const path = resolveTitleToPath(title);
  if (!path) return [];
  const cached = headingCache.get(path);
  if (cached) return cached;
  try {
    const note = await api.getNote(path);
    const headings = extractHeadings(note.body ?? '');
    headingCache.set(path, headings);
    return headings;
  } catch {
    return [];
  }
}

export async function wikilinkComplete(ctx: CompletionContext): Promise<CompletionResult | null> {
  const before = ctx.state.doc.sliceString(0, ctx.pos);
  // Heading-mode fires first so [[Foo#bar takes the heading branch
  // even though it also matches the title regex (the `#` is part of
  // the captured title group there). Order matters.
  const hm = before.match(openHeadingRe);
  if (hm) {
    const noteTitle = hm[1];
    const fragQuery = hm[2];
    const from = ctx.pos - fragQuery.length;
    const headings = await fetchHeadingsForTitle(noteTitle);
    if (headings.length === 0) return null;
    const ql = fragQuery.toLowerCase();
    const filtered = headings
      .map((h) => {
        const lo = h.toLowerCase();
        let score = -1;
        if (ql === '') score = 0;
        else if (lo === ql) score = 1000;
        else if (lo.startsWith(ql)) score = 500 - lo.length;
        else if (lo.includes(ql)) score = 100 - lo.length;
        return { h, score };
      })
      .filter((x) => x.score >= 0)
      .sort((a, b) => b.score - a.score)
      .slice(0, 30);
    if (filtered.length === 0) return null;
    return {
      from,
      options: filtered.map(({ h }) => ({
        label: h,
        detail: `${noteTitle} heading`,
        type: 'property',
        apply: (view, _completion, applyFrom, applyTo) => {
          // Same closing-bracket handling as the title branch — if
          // ]] is already there, don't double up.
          const after = view.state.doc.sliceString(applyTo, applyTo + 2);
          const insert = h + (after === ']]' ? '' : ']]');
          const cursor = applyFrom + insert.length;
          view.dispatch({
            changes: { from: applyFrom, to: applyTo, insert },
            selection: { anchor: cursor }
          });
        }
      })),
      // validFor allows free typing INSIDE the heading fragment but
      // bails on `]` so we close completion when the user types the
      // closing bracket manually.
      validFor: /^[^\]\n|]*$/
    };
  }
  const m = before.match(openWikilinkRe);
  if (!m) return null;
  const query = m[1];
  const from = ctx.pos - query.length;

  const titles = await ensureTitles();
  const ql = query.toLowerCase();
  const scored = titles
    .map((t) => {
      const lo = t.title.toLowerCase();
      let score = -1;
      if (ql === '') score = 0;
      else if (lo === ql) score = 1000;
      else if (lo.startsWith(ql)) score = 500 - lo.length;
      else if (lo.includes(ql)) score = 100 - lo.length;
      return { t, score };
    })
    .filter((s) => s.score >= 0)
    .sort((a, b) => b.score - a.score)
    .slice(0, 30);

  return {
    from,
    options: scored.map(({ t }) => ({
      label: t.title,
      detail: t.path,
      type: 'note',
      apply: (view, _completion, applyFrom, applyTo) => {
        const after = view.state.doc.sliceString(applyTo, applyTo + 2);
        const insert = t.title + (after === ']]' ? '' : ']]');
        const cursor = applyFrom + t.title.length + (after === ']]' ? 2 : 2);
        view.dispatch({
          changes: { from: applyFrom, to: applyTo, insert },
          selection: { anchor: cursor }
        });
      }
    })),
    validFor: /^[^\]\n]*$/
  };
}
