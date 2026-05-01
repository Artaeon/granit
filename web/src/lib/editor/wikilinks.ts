import { EditorView, Decoration, ViewPlugin } from '@codemirror/view';
import type { DecorationSet, ViewUpdate } from '@codemirror/view';
import type { CompletionContext, CompletionResult } from '@codemirror/autocomplete';
import { api } from '$lib/api';

const wikilinkRe = /\[\[([^\]\n]+?)\]\]/g;
const openWikilinkRe = /\[\[([^\]\n]*)$/;

const wikilinkMark = Decoration.mark({ class: 'cm-wikilink' });

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
      const ranges: ReturnType<typeof wikilinkMark.range>[] = [];
      for (const { from, to } of view.visibleRanges) {
        const text = view.state.doc.sliceString(from, to);
        wikilinkRe.lastIndex = 0;
        let m: RegExpExecArray | null;
        while ((m = wikilinkRe.exec(text)) !== null) {
          const start = from + m.index;
          const end = start + m[0].length;
          ranges.push(wikilinkMark.range(start, end));
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

async function ensureTitles() {
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

export async function wikilinkComplete(ctx: CompletionContext): Promise<CompletionResult | null> {
  const before = ctx.state.doc.sliceString(0, ctx.pos);
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
