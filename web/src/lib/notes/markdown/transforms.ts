// Pure markdown-transform pipeline for note preview rendering.
//
// Two phases, both pure functions:
//
//   preprocess(src) → { preprocessed, state }
//     • Strips frontmatter.
//     • Carves out granit-specific syntax (wikilinks, tag chips,
//       diagram blocks, image transclusions, footnotes, highlights,
//       Obsidian callouts) and replaces them with control-char
//       sentinels that survive marked unchanged.
//     • Returns the cleaned source ready for marked.parse() PLUS a
//       TransformState bundle of caches (diagrams/images/footnotes)
//       needed by the postprocess pass.
//
//   postprocess(html, state, src) → html
//     • Maps every sentinel back to its real HTML (anchors for
//       wikilinks, <img> for image transclusions, <pre> placeholders
//       for mermaid, footnote list, etc.).
//     • Hoists Obsidian callout markers into class names.
//     • Injects data-heading-line attributes so the Outline panel
//       and reading-progress tracker can map headings → source line.
//
// Sentinel chars are NUL + SO/SI control bytes — they never appear
// in user markdown and survive marked tokenisation as opaque text.
// The pipe `|` is deliberately AVOIDED inside sentinels because GFM
// tables use it as a column separator; the old `WL[a|b]` form
// shredded every wikilink inside a table row.
//
// The state object is passed explicitly between phases rather than
// living on module scope so concurrent renders (two MarkdownRenderer
// instances in split-view) can't trample each other.

import { marked } from 'marked';

// ── Sentinel pairs ─────────────────────────────────────────────
// C0 control bytes — never appear in user markdown, survive marked
// tokenisation as opaque text, and stay out of attribute values.
// The previous plaintext / empty-string forms (`WL`/``/``, etc.)
// produced regexes like `/WL([^]+)([^]+)/` whose greedy capture ran
// across whole paragraphs and false-matched any user text containing
// the bare `WL`/`TG`/`DG`/`IM` substrings.
export const WIKI_OPEN = '\x01';
export const WIKI_SEP = '\x02';
export const WIKI_CLOSE = '\x03';
export const TAG_OPEN = '\x04';
export const TAG_CLOSE = '\x05';
export const DIAGRAM_OPEN = '\x06';
export const DIAGRAM_CLOSE = '\x07';
export const IMG_OPEN = '\x10';
export const IMG_CLOSE = '\x11';
// Footnote sentinels — sandwich the id between SO/SI (\x0E and \x0F)
// to keep them distinct from the WL/TG/DG/IM markers above.
export const FNR_OPEN = '\x0EFNR\x0F';
export const FNR_CLOSE = '\x0E/FNR\x0F';
// Highlight sentinel — pre/postprocess pair turns ==text== into
// <mark>. Same control-char sandwich as the others so marked sees
// it as opaque text and never tokenizes it.
export const HL_OPEN = '\x0EHL\x0F';
export const HL_CLOSE = '\x0E/HL\x0F';

// Image extensions we know how to embed via the file API. Anything
// else (PDF, audio, video, .canvas) falls through to the plain
// wikilink path so the user at least gets a click target.
const IMAGE_EXTS = /\.(png|jpe?g|gif|webp|svg|avif|bmp|ico)$/i;

// Obsidian callout types — `> [!note] Title` blocks. Postprocess
// hoists the `[!type]` marker into a class on the blockquote and
// pulls the title into a styled header.
const CALLOUT_TYPES: Record<string, { label: string; tone: string }> = {
  note: { label: 'Note', tone: 'info' },
  info: { label: 'Info', tone: 'info' },
  tip: { label: 'Tip', tone: 'success' },
  success: { label: 'Success', tone: 'success' },
  quote: { label: 'Quote', tone: 'subtext' },
  abstract: { label: 'Abstract', tone: 'subtext' },
  summary: { label: 'Summary', tone: 'subtext' },
  todo: { label: 'TODO', tone: 'primary' },
  question: { label: 'Question', tone: 'primary' },
  warning: { label: 'Warning', tone: 'warning' },
  caution: { label: 'Caution', tone: 'warning' },
  danger: { label: 'Danger', tone: 'error' },
  error: { label: 'Error', tone: 'error' },
  failure: { label: 'Failure', tone: 'error' },
  bug: { label: 'Bug', tone: 'error' },
  example: { label: 'Example', tone: 'accent' }
};

// marked options — GFM, tables, no autolinking of bare newlines.
// Called once at module load instead of on every renderer mount
// (the previous in-component setOptions ran on every instance).
marked.setOptions({ gfm: true, breaks: false });

// ── Escape helpers ─────────────────────────────────────────────
export function esc(s: string): string {
  return s.replace(/[\\^$*+?.()|[\]{}]/g, '\\$&');
}
export function escAttr(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/"/g, '&quot;');
}
export function escHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// ── Frontmatter strip ─────────────────────────────────────────
// Some note GETs include the leading YAML frontmatter block, others
// don't — be defensive either way so marked doesn't render the YAML
// as a horizontal rule + paragraph.
export function stripFrontmatter(src: string): string {
  if (!src.startsWith('---')) return src;
  const end = src.indexOf('\n---', 3);
  if (end === -1) return src;
  return src.slice(end + 4).replace(/^\r?\n/, '');
}

// ── Transform state ────────────────────────────────────────────
export interface TransformState {
  /** Diagram blocks (mermaid/mindmap/chart/flow) carved out at
   *  preprocess time. Postprocess turns them back into <pre> hosts
   *  (mermaid) or source-card fallbacks (everything else). */
  diagrams: { kind: string; source: string }[];
  /** Image / non-image transclusion targets. Indexed by the
   *  sentinel id. */
  images: string[];
  /** Footnote definitions keyed by id. */
  footnoteDefs: Map<string, string>;
  /** Order of first occurrence — drives the displayed footnote
   *  numbers, regardless of definition order. */
  footnoteRefs: string[];
}

function newState(): TransformState {
  return {
    diagrams: [],
    images: [],
    footnoteDefs: new Map(),
    footnoteRefs: []
  };
}

// ── Preprocess ────────────────────────────────────────────────
export function preprocess(src: string): { preprocessed: string; state: TransformState } {
  const state = newState();
  let s = stripFrontmatter(src);
  // Footnote definitions FIRST so subsequent passes don't tokenise
  // the definition body (which is just markdown that ends up
  // rendered in the footnote section, not inline). Strip the line
  // entirely from the doc; postprocess reinjects the footnote list
  // at the bottom.
  s = s.replace(/^\[\^([^\]\s]+)\]:[ \t]+(.*)$/gm, (_, id: string, body: string) => {
    state.footnoteDefs.set(String(id), String(body).trim());
    return '';
  });
  // Footnote references → sentinel. First-appearance order drives
  // the displayed number (matches Pandoc behaviour). A reference
  // with no matching definition stays sentinel-wrapped — postprocess
  // renders it as a clearly-broken superscript.
  s = s.replace(/\[\^([^\]\s]+)\]/g, (_, id: string) => {
    const sid = String(id);
    if (!state.footnoteRefs.includes(sid)) state.footnoteRefs.push(sid);
    return `${FNR_OPEN}${sid}${FNR_CLOSE}`;
  });
  // Carve out granit-specific fenced blocks BEFORE wikilink + tag
  // substitution so their bodies don't get clobbered.
  s = s.replace(/```(diagram|mermaid|mindmap|chart|flow)\n([\s\S]*?)```/g, (_, kind, source) => {
    state.diagrams.push({ kind: String(kind), source: String(source) });
    return `${DIAGRAM_OPEN}${state.diagrams.length - 1}${DIAGRAM_CLOSE}`;
  });
  // Image transclusions: `![[file.png]]` → <img>. Matches Obsidian
  // semantics. Runs BEFORE the wikilink rule so the leading `!` is
  // captured rather than stripped.
  s = s.replace(/!\[\[([^\]\n]+?)\]\]/g, (_, inner) => {
    const target = String(inner).split('|')[0].trim();
    if (!IMAGE_EXTS.test(target)) {
      // Non-image transclusion: fall back to a styled placeholder
      // card. EmbedHydrator picks these up post-render and walks
      // them into real inline note embeds.
      state.images.push(target);
      return `${IMG_OPEN}note:${state.images.length - 1}${IMG_CLOSE}`;
    }
    state.images.push(target);
    return `${IMG_OPEN}img:${state.images.length - 1}${IMG_CLOSE}`;
  });
  // Wikilinks `[[X]]` / `[[X|Y]]` → sentinel. The `` separator
  // avoids the table-pipe collision that previously broke every
  // dashboard table containing wikilinks.
  s = s.replace(/\[\[([^\]\n]+?)\]\]/g, (_, inner) => {
    const parts = String(inner).split('|');
    const target = parts[0].trim();
    const display = (parts.length > 1 ? parts[parts.length - 1] : parts[0]).trim();
    return `${WIKI_OPEN}${target}${WIKI_SEP}${display}${WIKI_CLOSE}`;
  });
  // Tag substitution must NOT clobber #-prefixes inside fenced
  // code. Shield fences with a placeholder, substitute, then
  // re-inject. Same fences-of-fences pattern as elsewhere.
  const fences: string[] = [];
  s = s.replace(/```[\s\S]*?```/g, (m) => {
    fences.push(m);
    return `F${fences.length - 1}`;
  });
  s = s.replace(/(^|[\s(])#([\p{L}\p{N}_/-]+)/gu, (_, pre, tag) => `${pre}${TAG_OPEN}${tag}${TAG_CLOSE}`);
  s = s.replace(/F(\d+)/g, (_, i) => fences[Number(i)]);
  // Highlights: ==text== → <mark>. Sentinel-wrapped so marked sees
  // it as opaque text. Fences are already shielded above so we
  // won't match inside code blocks.
  s = s.replace(/==([^=\n][^=]*?)==/g, (_, inner) => `${HL_OPEN}${inner}${HL_CLOSE}`);
  return { preprocessed: s, state };
}

// ── Callouts ──────────────────────────────────────────────────
function rewriteCallouts(html: string): string {
  // Match `[!type]` marker + the SAME LINE only as the title. A
  // naive `[^<]*` greedy capture dragged body content into the
  // title and produced an empty body — split at the first newline
  // so the title is the first line and everything after lands in
  // the body section.
  return html.replace(
    /<blockquote>\s*<p>\[!([a-zA-Z]+)\]([+-]?)([^<]*)<\/p>([\s\S]*?)<\/blockquote>/g,
    (_match, rawType: string, _toggle: string, head: string, rest: string) => {
      const t = rawType.toLowerCase();
      const meta = CALLOUT_TYPES[t] ?? { label: rawType, tone: 'subtext' };
      const newlineIdx = head.search(/\r?\n/);
      let title: string;
      let leftover: string;
      if (newlineIdx === -1) {
        title = head.trim();
        leftover = '';
      } else {
        title = head.slice(0, newlineIdx).trim();
        leftover = head.slice(newlineIdx).trim();
      }
      const headerTitle = title || meta.label;
      const bodyParagraph = leftover ? `<p>${escHtml(leftover)}</p>` : '';
      return (
        `<blockquote class="callout callout--${meta.tone}">` +
        `<div class="callout__header"><span class="callout__icon" aria-hidden="true">●</span>${escHtml(headerTitle)}</div>` +
        bodyParagraph +
        rest +
        `</blockquote>`
      );
    }
  );
}

// ── Heading metadata ──────────────────────────────────────────
// Parallel arrays of source-line + slug, in appearance order, used
// by injectHeadingMeta to stamp `<h*>` tags with data-heading-line
// + id attributes the Outline panel can target.
function collectHeadingMetaFromBody(src: string): { line: number; slug: string }[] {
  const out: { line: number; slug: string }[] = [];
  if (!src) return out;
  const stripped = stripFrontmatter(src);
  const offset = src.length - stripped.length;
  const removed = src.slice(0, offset);
  const lineOffset = removed ? removed.split('\n').length - 1 : 0;
  const lines = stripped.split('\n');
  let inFence = false;
  const slugCounts = new Map<string, number>();
  for (let i = 0; i < lines.length; i++) {
    const t = lines[i];
    if (/^```/.test(t.trim()) || /^~~~/.test(t.trim())) {
      inFence = !inFence;
      continue;
    }
    if (inFence) continue;
    const m = /^\s{0,3}(#{1,6})\s+(.+?)\s*#*$/.exec(t);
    if (!m) continue;
    const text = m[2].trim();
    const baseSlug = text
      .toLowerCase()
      .replace(/[^\w\s-]/g, '')
      .replace(/\s+/g, '-')
      .slice(0, 80) || 'h';
    const n = slugCounts.get(baseSlug) ?? 0;
    slugCounts.set(baseSlug, n + 1);
    const slug = n === 0 ? baseSlug : `${baseSlug}-${n}`;
    out.push({ line: i + 1 + lineOffset, slug });
  }
  return out;
}

function injectHeadingMeta(rendered: string, src: string): string {
  const meta = collectHeadingMetaFromBody(src);
  if (meta.length === 0) return rendered;
  let i = 0;
  return rendered.replace(/<(h[1-6])(\b[^>]*)>/g, (m, tag: string, attrs: string) => {
    // Skip headings inside callout / embed cards — their bodies
    // pass through marked too and we don't want phantom doubles
    // in the observer. Heuristic: marked never inserts an
    // existing id / data-heading-line on a heading.
    if (/\sid=|\sdata-heading-line=/.test(attrs)) return m;
    if (i >= meta.length) return m;
    const { line, slug } = meta[i++];
    return `<${tag} id="h-${slug}" data-heading-line="${line}" data-heading-slug="${slug}"${attrs}>`;
  });
}

// ── Postprocess ───────────────────────────────────────────────
export function postprocess(html: string, state: TransformState, src: string): string {
  let s = html;
  // Diagrams → styled cards. `mermaid` blocks become <pre> hosts
  // that the mermaid hydrator (a separate post-render pass) walks
  // and replaces with rendered SVG.
  const dgRe = new RegExp(`${esc(DIAGRAM_OPEN)}(\\d+)${esc(DIAGRAM_CLOSE)}`, 'g');
  s = s.replace(dgRe, (_, idx: string) => {
    const d = state.diagrams[Number(idx)];
    if (!d) return '';
    if (d.kind === 'mermaid') {
      return (
        `<pre class="mermaid-host" data-mermaid-source="${escAttr(d.source)}">` +
        `<code>${escHtml(d.source)}</code></pre>`
      );
    }
    return (
      `<div class="diagram-card">` +
      `<div class="diagram-card__header">${escHtml(d.kind)}<span class="diagram-card__hint"> · render in TUI</span></div>` +
      `<pre class="diagram-card__source"><code>${escHtml(d.source)}</code></pre>` +
      `</div>`
    );
  });
  // Image transclusions → <img>; non-image transclusions →
  // transclude-card placeholder for the embed hydrator to walk.
  const imRe = new RegExp(`${esc(IMG_OPEN)}(img|note):(\\d+)${esc(IMG_CLOSE)}`, 'g');
  s = s.replace(imRe, (_, kind: string, idx: string) => {
    const target = state.images[Number(idx)];
    if (!target) return '';
    if (kind === 'img') {
      const encoded = target.split('/').map(encodeURIComponent).join('/');
      return `<img class="note-image" src="/api/v1/files/${encoded}" alt="${escAttr(target)}" loading="lazy">`;
    }
    return (
      `<div class="transclude-card">` +
      `<div class="transclude-card__label">embed</div>` +
      `<a class="wikilink" data-wikilink="${escAttr(target)}">${escHtml(target)}</a>` +
      `</div>`
    );
  });
  // Highlights → <mark>. The HL sentinel is fence-shielded by the
  // preprocess step so we won't ever match inside code blocks.
  const hlRe = new RegExp(`${esc(HL_OPEN)}([\\s\\S]*?)${esc(HL_CLOSE)}`, 'g');
  s = s.replace(hlRe, (_, inner: string) => `<mark class="md-highlight">${escHtml(inner)}</mark>`);
  // Wikilinks → anchors.
  const wlRe = new RegExp(
    `${esc(WIKI_OPEN)}([^${esc(WIKI_SEP)}]+)${esc(WIKI_SEP)}([^${esc(WIKI_CLOSE)}]+)${esc(WIKI_CLOSE)}`,
    'g'
  );
  s = s.replace(wlRe, (_, target: string, display: string) =>
    `<a class="wikilink" data-wikilink="${escAttr(target)}">${escHtml(display)}</a>`
  );
  // Tags → spans.
  const tgRe = new RegExp(`${esc(TAG_OPEN)}([^${esc(TAG_CLOSE)}]+)${esc(TAG_CLOSE)}`, 'g');
  s = s.replace(tgRe, (_, tag: string) => `<span class="hashtag">#${escHtml(tag)}</span>`);
  // Footnote references → superscript anchor that jumps to the
  // matching <li id="fn-<id>"> in the footnotes section. Includes a
  // back-link (↩) on the definition side so the reader can hop
  // both directions, matching Pandoc's default footnote rendering.
  const fnrRe = new RegExp(`${esc(FNR_OPEN)}([^${esc(FNR_CLOSE)[0]}]+?)${esc(FNR_CLOSE)}`, 'g');
  s = s.replace(fnrRe, (_, id: string) => {
    const idx = state.footnoteRefs.indexOf(id);
    const num = idx === -1 ? '?' : String(idx + 1);
    const safeId = escAttr(id);
    const broken = !state.footnoteDefs.has(id);
    const cls = broken ? 'footnote-ref footnote-ref--broken' : 'footnote-ref';
    const title = broken ? `unresolved footnote: ${escHtml(id)}` : '';
    return `<sup class="${cls}"><a href="#fn-${safeId}" id="fnref-${safeId}" title="${title}">[${num}]</a></sup>`;
  });
  // Footnote definitions → ordered list at the bottom. Only emit
  // when at least one ref was seen; a doc with definitions but no
  // refs is probably mid-edit (user wrote the def first), so hide
  // them rather than dumping orphan footnotes that look like
  // garbage trailing text.
  if (state.footnoteRefs.length > 0) {
    const items: string[] = [];
    for (const id of state.footnoteRefs) {
      const body = state.footnoteDefs.get(id) ?? '<em class="footnote-missing">no definition</em>';
      const safeId = escAttr(id);
      // body is raw markdown (we stripped the definition line in
      // preprocess). Inline-render through marked.parseInline so
      // **bold** and links inside footnotes work, but we don't get
      // nested <p> wrappers that would break the <li> layout.
      let rendered: string;
      try {
        rendered = marked.parseInline(body, { async: false }) as string;
      } catch {
        rendered = escHtml(body);
      }
      items.push(
        `<li id="fn-${safeId}" class="footnote-item">${rendered} ` +
          `<a class="footnote-back" href="#fnref-${safeId}" title="back to text">↩</a></li>`
      );
    }
    s += `<hr class="footnote-sep"><ol class="footnote-list">${items.join('')}</ol>`;
  }
  // Callouts last so the regex sees the final tree.
  s = rewriteCallouts(s);
  // Inject heading line metadata so the Outline panel can map
  // headings → source line and observe them with
  // IntersectionObserver.
  s = injectHeadingMeta(s, src);
  return s;
}
