<script lang="ts">
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';
  import { api } from '$lib/api';
  import { errorMessage } from '$lib/util/errorMessage';
  import WikilinkHoverPreview from './WikilinkHoverPreview.svelte';

  // DOMPurify config — single shared profile for every sanitize() call
  // in this component. We allow our wikilink / tag / diagram / image
  // sentinel attributes (`data-wikilink`, `data-tag`, `data-diagram`,
  // `data-img-src`) because postprocess() decorates spans with them
  // for the click-delegation handler. Without explicit ADD_DATA_URI_TAGS
  // and ADD_ATTR, DOMPurify would strip the data-* hooks and our
  // wikilink navigation would silently break.
  //
  // Why USE_PROFILES.html: we explicitly want body HTML allowed
  // (paragraphs, lists, tables, code blocks, headings — everything
  // marked emits). The default whitelist covers it. We do NOT enable
  // SVG/MathML because the editor doesn't ship those as input
  // formats; opening that surface would expand the attack vector
  // without payoff.
  //
  // FORBID_TAGS: `script` is already excluded by the default profile,
  // but listing it (plus iframe + object + embed) is belt-and-braces
  // — three layers cheaper than one DOMPurify CVE escape.
  const PURIFY_CONFIG: Parameters<typeof DOMPurify.sanitize>[1] = {
    USE_PROFILES: { html: true },
    ADD_ATTR: ['data-wikilink', 'data-tag', 'data-diagram', 'data-img-src', 'data-mermaid-src'],
    FORBID_TAGS: ['script', 'iframe', 'object', 'embed'],
    FORBID_ATTR: ['onerror', 'onload', 'onclick', 'onmouseover', 'onfocus', 'onblur']
  };

  function purify(html: string): string {
    return DOMPurify.sanitize(html, PURIFY_CONFIG) as string;
  }

  let { body, onWikilink }: { body: string; onWikilink?: (target: string) => void } = $props();

  // Pre-process the source so wikilinks survive as inline tokens marked
  // doesn't recognize natively. The sentinels MUST use characters that
  // never appear in normal markdown — particularly NOT the pipe `|`,
  // which doubles as the GFM table cell delimiter. The previous
  // sentinel form `WL[target|display]` shredded any wikilink-in-table
  // because marked saw the embedded `|` as a column boundary mid-cell.
  // Using NUL () for both bracket markers and the
  // target/display separator is safe — it never appears in source
  // markdown and survives marked unchanged.
  const WIKI_OPEN = 'WL';
  const WIKI_CLOSE = '';
  const WIKI_SEP = '';
  const TAG_OPEN = 'TG';
  const TAG_CLOSE = '';
  const DIAGRAM_OPEN = 'DG';
  const DIAGRAM_CLOSE = '';
  const IMG_OPEN = 'IM';
  const IMG_CLOSE = '';

  // Strip YAML frontmatter so marked doesn't render it as a horizontal
  // rule + paragraph. Some note GETs include frontmatter, others don't —
  // be defensive either way.
  function stripFrontmatter(src: string): string {
    if (!src.startsWith('---')) return src;
    const end = src.indexOf('\n---', 3);
    if (end === -1) return src;
    return src.slice(end + 4).replace(/^\r?\n/, '');
  }

  // Footnote sentinels. Sandwich the ref id between SO/SI control bytes
  // (\x0E and \x0F) to keep them distinct from the WL/TG/DG/IM markers
  // above. Footnotes use Pandoc/PHP-Markdown-Extra syntax: `[^id]`
  // inline reference, `[^id]: text` line-start definition.
  const FNR_OPEN = '\x0EFNR\x0F';
  const FNR_CLOSE = '\x0E/FNR\x0F';
  // Highlight sentinel — pre-/postprocess pair turns ==text== into
  // <mark>. Same control-char sandwich as the others so marked sees
  // it as opaque text and never tokenizes it.
  const HL_OPEN = '\x0EHL\x0F';
  const HL_CLOSE = '\x0E/HL\x0F';

  // Diagrams + image transclusions stashed during preprocess and
  // restored as styled elements in postprocess. Module-scoped so the
  // helpers can share state across one render pass — $derived re-runs
  // the whole function so there's no cross-render leakage.
  let diagramCache: { kind: string; source: string }[] = [];
  let imageCache: string[] = [];
  // Footnote state: defs is the line-start `[^id]: body` map; refs is
  // the order of first-occurrence references so we can number them in
  // appearance order regardless of definition order in the source.
  let footnoteDefs: Map<string, string> = new Map();
  let footnoteRefOrder: string[] = [];

  // Image extensions we know how to embed via the file API. Anything
  // else (PDF, audio, video, .canvas) falls through to the plain
  // wikilink path so the user at least gets a click target.
  const IMAGE_EXTS = /\.(png|jpe?g|gif|webp|svg|avif|bmp|ico)$/i;

  function preprocess(src: string): string {
    let s = stripFrontmatter(src);
    // Footnote definitions FIRST so subsequent passes don't try to
    // tokenize the definition body (which is just markdown that ends
    // up rendered in the footnote section, not inline). Match line-
    // start `[^id]: text` taking the rest of the line as the body —
    // simple form; pandoc allows multi-line continuations but those
    // are rare in practice and a single-line def covers the common
    // case. Strip the line entirely from the doc; postprocess
    // reinjects the footnotes section at the bottom.
    footnoteDefs = new Map();
    footnoteRefOrder = [];
    s = s.replace(/^\[\^([^\]\s]+)\]:[ \t]+(.*)$/gm, (_, id: string, body: string) => {
      footnoteDefs.set(String(id), String(body).trim());
      return ''; // remove the def line — keeps the visible body clean
    });
    // Footnote references: replace `[^id]` (inline) with a sentinel
    // pair carrying the id. Order of first-appearance drives the
    // displayed number (matches Pandoc behaviour). A reference whose
    // id has no matching definition stays sentinel-wrapped — postprocess
    // renders it as a clearly-broken superscript so the user sees the
    // gap rather than a silent disappearance.
    s = s.replace(/\[\^([^\]\s]+)\]/g, (_, id: string) => {
      const sid = String(id);
      if (!footnoteRefOrder.includes(sid)) footnoteRefOrder.push(sid);
      return `${FNR_OPEN}${sid}${FNR_CLOSE}`;
    });
    // Carve out granit-specific fenced blocks (diagram/mermaid/mindmap/
    // chart/flow) BEFORE wikilink+tag substitution so their bodies
    // don't get clobbered.
    diagramCache = [];
    s = s.replace(/```(diagram|mermaid|mindmap|chart|flow)\n([\s\S]*?)```/g, (_, kind, source) => {
      diagramCache.push({ kind: String(kind), source: String(source) });
      return `${DIAGRAM_OPEN}${diagramCache.length - 1}${DIAGRAM_CLOSE}`;
    });
    // Image transclusions: `![[file.png]]` → <img>. Matches Obsidian
    // semantics. Has to run BEFORE the wikilink rule below, otherwise
    // the leading `!` would be stripped and the image rendered as a
    // plain link. The wikilink rule lives one regex below, so
    // matching `!\[\[...\]\]` first wins.
    imageCache = [];
    s = s.replace(/!\[\[([^\]\n]+?)\]\]/g, (_, inner) => {
      const target = String(inner).split('|')[0].trim();
      if (!IMAGE_EXTS.test(target)) {
        // Non-image transclusion (note embed, PDF, etc.): fall back to
        // a styled placeholder card; we don't have an embed render
        // pipeline yet but at least show the link clearly.
        imageCache.push(target);
        return `${IMG_OPEN}note:${imageCache.length - 1}${IMG_CLOSE}`;
      }
      imageCache.push(target);
      return `${IMG_OPEN}img:${imageCache.length - 1}${IMG_CLOSE}`;
    });
    // Wikilinks `[[X]]` / `[[X|Y]]` → sentinel. The `` separator
    // avoids the table-pipe collision that previously broke every
    // dashboard table containing wikilinks.
    s = s.replace(/\[\[([^\]\n]+?)\]\]/g, (_, inner) => {
      const parts = String(inner).split('|');
      const target = parts[0].trim();
      const display = (parts.length > 1 ? parts[parts.length - 1] : parts[0]).trim();
      return `${WIKI_OPEN}${target}${WIKI_SEP}${display}${WIKI_CLOSE}`;
    });
    // Avoid replacing # tags inside fenced code blocks
    const fences: string[] = [];
    s = s.replace(/```[\s\S]*?```/g, (m) => {
      fences.push(m);
      return `F${fences.length - 1}`;
    });
    s = s.replace(/(^|[\s(])#([\p{L}\p{N}_/-]+)/gu, (_, pre, tag) => `${pre}${TAG_OPEN}${tag}${TAG_CLOSE}`);
    s = s.replace(/F(\d+)/g, (_, i) => fences[Number(i)]);
    // Highlights: ==text== → <mark>text</mark>. Sentinel-wrapped so
    // marked doesn't see the bare ==…== (which it'd pass through as
    // text but might mangle inside list items in some edge cases).
    // Re-fence-shielded above already, so we won't match inside code.
    s = s.replace(/==([^=\n][^=]*?)==/g, (_, inner) => `${HL_OPEN}${inner}${HL_CLOSE}`);
    return s;
  }

  // Obsidian callouts: `> [!note] Title` (optionally `[!note]+` /
  // `[!note]-` for default-collapsed). Marked turns the markdown into
  // a plain blockquote with literal `[!type]` text inside. Postprocess
  // hoists the `[!type]` marker into a class on the blockquote and
  // pulls the title into a styled header. Keeps the user's content
  // verbatim while making the callout actually look like one.
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
    example: { label: 'Example', tone: 'accent' },
  };

  function rewriteCallouts(html: string): string {
    // Match the callout `[!type]` marker + the SAME LINE only as the
    // title. Marked emits the full callout body inside a single <p>
    // when there are no blank lines between the `> [!note]` header
    // and the body lines, which means a naive `[^<]*` greedy capture
    // dragged body content into the title and produced an empty
    // body. Splitting at the first newline keeps the title as the
    // first line and pushes everything after into the body section.
    return html.replace(
      /<blockquote>\s*<p>\[!([a-zA-Z]+)\]([+-]?)([^<]*)<\/p>([\s\S]*?)<\/blockquote>/g,
      (_match, rawType: string, _toggle: string, head: string, rest: string) => {
        const t = rawType.toLowerCase();
        const meta = CALLOUT_TYPES[t] ?? { label: rawType, tone: 'subtext' };
        // First line is the title; remaining lines belong in the body
        // (re-wrapped in a <p> so spacing matches a normal paragraph).
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
        const bodyParagraph = leftover
          ? `<p>${escHtml(leftover)}</p>`
          : '';
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

  function postprocess(html: string): string {
    let s = html;
    // Diagrams → styled cards. `mermaid` blocks become <pre> placeholders
    // that the post-render effect (below) hydrates with rendered SVG.
    // Other diagram kinds (mindmap/chart/flow) keep the source-card
    // fallback because we don't have a renderer for them yet.
    const dgRe = new RegExp(`${esc(DIAGRAM_OPEN)}(\\d+)${esc(DIAGRAM_CLOSE)}`, 'g');
    s = s.replace(dgRe, (_, idx: string) => {
      const d = diagramCache[Number(idx)];
      if (!d) return '';
      if (d.kind === 'mermaid') {
        // Carry the source on a data attribute so the hydrator can
        // pick it up after the {@html} drop. Wrapped in <pre> for
        // graceful degradation: if the lazy-import or the render call
        // fails, the user sees their source code, not a blank box.
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
    // Image transclusions → <img>; non-image transclusions → placeholder.
    const imRe = new RegExp(`${esc(IMG_OPEN)}(img|note):(\\d+)${esc(IMG_CLOSE)}`, 'g');
    s = s.replace(imRe, (_, kind: string, idx: string) => {
      const target = imageCache[Number(idx)];
      if (!target) return '';
      if (kind === 'img') {
        // Resolve via the file API. Encode each segment so `Files/My
        // Image.png` round-trips correctly.
        const encoded = target.split('/').map(encodeURIComponent).join('/');
        return `<img class="note-image" src="/api/v1/files/${encoded}" alt="${escAttr(target)}" loading="lazy">`;
      }
      // Note transclusion — render a clickable card so the user can
      // jump to the embedded note. Real inline-render is a future
      // upgrade; a working link beats a literal `!WL[...]` blob.
      return (
        `<div class="transclude-card">` +
        `<div class="transclude-card__label">embed</div>` +
        `<a class="wikilink" data-wikilink="${escAttr(target)}">${escHtml(target)}</a>` +
        `</div>`
      );
    });
    // Wikilinks → anchors. The new sentinel uses  between target
    // and display, so the target may safely contain `|`, hyphens,
    // spaces — none of which collide with marked's tokenizer.
    // Highlights → <mark>. The HL sentinel is fence-shielded by the
    // preprocess step so we won't ever match inside code blocks.
    const hlRe = new RegExp(`${esc(HL_OPEN)}([\\s\\S]*?)${esc(HL_CLOSE)}`, 'g');
    s = s.replace(hlRe, (_, inner: string) => `<mark class="md-highlight">${escHtml(inner)}</mark>`);
    const wlRe = new RegExp(
      `${esc(WIKI_OPEN)}([^${esc(WIKI_SEP)}]+)${esc(WIKI_SEP)}([^${esc(WIKI_CLOSE)}]+)${esc(WIKI_CLOSE)}`,
      'g'
    );
    s = s.replace(wlRe, (_, target: string, display: string) =>
      `<a class="wikilink" data-wikilink="${escAttr(target)}">${escHtml(display)}</a>`
    );
    // Tags → spans
    const tgRe = new RegExp(`${esc(TAG_OPEN)}([^${esc(TAG_CLOSE)}]+)${esc(TAG_CLOSE)}`, 'g');
    s = s.replace(tgRe, (_, tag: string) => `<span class="hashtag">#${escHtml(tag)}</span>`);
    // Footnote references: superscript anchor that jumps to the
    // matching <li id="fn-<id>"> in the footnotes section. Includes a
    // back-link (↩) on the definition side so the reader can hop both
    // directions, matching Pandoc's default footnote rendering.
    const fnrRe = new RegExp(`${esc(FNR_OPEN)}([^${esc(FNR_CLOSE)[0]}]+?)${esc(FNR_CLOSE)}`, 'g');
    s = s.replace(fnrRe, (_, id: string) => {
      const idx = footnoteRefOrder.indexOf(id);
      const num = idx === -1 ? '?' : String(idx + 1);
      const safeId = escAttr(id);
      const broken = !footnoteDefs.has(id);
      const cls = broken ? 'footnote-ref footnote-ref--broken' : 'footnote-ref';
      const title = broken ? `unresolved footnote: ${escHtml(id)}` : '';
      return `<sup class="${cls}"><a href="#fn-${safeId}" id="fnref-${safeId}" title="${title}">[${num}]</a></sup>`;
    });
    // Footnote definitions → ordered list at the bottom. Only emit the
    // section when at least one ref was seen; a doc with definitions
    // but no refs is probably mid-edit (user wrote the def first), so
    // hide them rather than dumping orphan footnotes that look like
    // garbage trailing text.
    if (footnoteRefOrder.length > 0) {
      const items: string[] = [];
      for (const id of footnoteRefOrder) {
        const body = footnoteDefs.get(id) ?? '<em class="footnote-missing">no definition</em>';
        const safeId = escAttr(id);
        // Body has been through marked already? No — we stripped the
        // definition line in preprocess, so body is raw markdown.
        // Inline-render it through marked.parseInline so `**bold**` and
        // links inside footnotes work, but we don't get nested <p>
        // wrappers that would break the <li> layout.
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
    // Obsidian callouts last so the regex sees the final tree.
    s = rewriteCallouts(s);
    // Inject heading line metadata so the Outline panel + reading-
    // progress tracker can map headings → source line and observe
    // them with IntersectionObserver. We pair headings in
    // appearance order with the headings parsed from the source body
    // (cheap O(n)). Only adds attributes when no existing id/data-
    // heading-line is present, so footnote headings (none) or
    // user-authored ones survive untouched.
    s = injectHeadingMeta(s, body);
    return s;
  }

  // Build a list of heading lines from the source body, fence-aware.
  // Returns parallel arrays: line numbers (1-based), heading text.
  // The text is normalised (collapsed whitespace, lowercased) so the
  // slug below is stable across the same heading reappearing later.
  function collectHeadingMetaFromBody(src: string): { line: number; slug: string }[] {
    const out: { line: number; slug: string }[] = [];
    if (!src) return out;
    const stripped = stripFrontmatter(src);
    const offset = src.length - stripped.length;
    // Compute line offset from frontmatter byte stripping. Cheap: count
    // newlines in the stripped portion of the source.
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
      // Skip headings inside callout/embed cards — their bodies pass
      // through marked too and we don't want phantom doubles in the
      // observer. Cheap heuristic: marked never inserts an existing
      // id/data-heading-line on a heading, so if one's already there
      // we trust it (e.g. embed-card sub-render).
      if (/\sid=|\sdata-heading-line=/.test(attrs)) return m;
      if (i >= meta.length) return m;
      const { line, slug } = meta[i++];
      return `<${tag} id="h-${slug}" data-heading-line="${line}" data-heading-slug="${slug}"${attrs}>`;
    });
  }

  function esc(s: string): string {
    return s.replace(/[\\^$*+?.()|[\]{}]/g, '\\$&');
  }
  function escAttr(s: string): string {
    return s.replace(/&/g, '&amp;').replace(/"/g, '&quot;');
  }
  function escHtml(s: string): string {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  // marked options — GFM, tables, breaks
  marked.setOptions({ gfm: true, breaks: false });

  // Render is wrapped in try/catch so a malformed token (corner-case
  // marked bug, half-edited fence, weird unicode) shows the source
  // verbatim instead of taking the page hostage. Without this, a
  // throwing marked.parse would bubble up through the $derived
  // computation and the preview would silently render nothing.
  let html = $derived.by(() => {
    if (!body) return '';
    try {
      const pre = preprocess(body);
      const out = marked.parse(pre, { async: false }) as string;
      // purify() is the load-bearing XSS defence — marked itself
      // dropped its `sanitize` option in v7 and explicitly recommends
      // DOMPurify downstream. The threat surface is small (self-
      // hosted single-user vault) but real once notes sync across
      // devices: a `<script>` slipped into a note on device A would
      // execute on device B without this layer. PURIFY_CONFIG keeps
      // our data-wikilink / data-tag etc. hooks alive so click
      // delegation still works after sanitisation.
      return purify(postprocess(out));
    } catch (e) {
      const msg = errorMessage(e);
      return (
        `<div class="prose-note__error">` +
        `<p><strong>Preview render failed:</strong> ${escHtml(msg)}</p>` +
        `<p class="text-dim text-sm">Showing source. Edit mode still works.</p>` +
        `<pre><code>${escHtml(body)}</code></pre>` +
        `</div>`
      );
    }
  });

  function onClickContainer(e: MouseEvent) {
    const target = e.target as HTMLElement;
    const wl = target.closest('[data-wikilink]');
    if (wl) {
      e.preventDefault();
      const t = wl.getAttribute('data-wikilink');
      if (t) onWikilink?.(t);
    }
  }

  // ── Mermaid renderer ────────────────────────────────────────────
  // Lazy-loaded so users who never write a mermaid block don't pay
  // the ~1MB bundle cost. The library is pulled in only when at
  // least one .mermaid-host placeholder is in the DOM, then cached
  // at module scope so subsequent renders reuse the same instance.

  let mermaidContainer: HTMLDivElement | undefined = $state();
  // Module-scoped — the import promise persists across re-renders so
  // we don't re-fetch on every doc keystroke.
  let mermaidPromise: Promise<typeof import('mermaid').default> | null = null;
  let mermaidIdCounter = 0;

  // The body's theme controls mermaid's palette. We re-init mermaid
  // when the user flips the theme so light-mode diagrams come back
  // with the right colours; observation via MutationObserver on the
  // <html> data-theme attribute is overkill here — we just call init
  // before each render and pass the live theme.
  function detectMermaidTheme(): 'dark' | 'default' {
    if (typeof document === 'undefined') return 'default';
    const t = document.documentElement.getAttribute('data-theme');
    if (t === 'light') return 'default';
    return 'dark';
  }

  async function getMermaid() {
    if (!mermaidPromise) {
      mermaidPromise = import('mermaid').then((m) => m.default);
    }
    return mermaidPromise;
  }

  async function renderMermaidBlocks() {
    if (!mermaidContainer) return;
    const hosts = Array.from(
      mermaidContainer.querySelectorAll<HTMLPreElement>('pre.mermaid-host[data-mermaid-source]')
    );
    if (hosts.length === 0) return;
    const mermaid = await getMermaid();
    // initialize is idempotent — calling per render lets a theme
    // toggle take effect without reloading the page.
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: 'strict', // no <foreignObject> raw HTML — bound to vault content
      theme: detectMermaidTheme(),
      fontFamily: 'inherit'
    });
    for (const host of hosts) {
      const source = host.getAttribute('data-mermaid-source') ?? '';
      const id = `mermaid-${++mermaidIdCounter}`;
      try {
        const { svg, bindFunctions } = await mermaid.render(id, source);
        const wrapper = document.createElement('div');
        wrapper.className = 'mermaid-rendered';
        wrapper.innerHTML = svg;
        // bindFunctions wires up click handlers in flowcharts that
        // use `click NodeId callback "tooltip"`. Skipping it would
        // silently break interactivity in user-authored diagrams.
        bindFunctions?.(wrapper);
        host.replaceWith(wrapper);
      } catch (err) {
        // Render failure → keep the source visible and surface a
        // small error caption. Better than a silent blank.
        const msg = errorMessage(err);
        const errBox = document.createElement('div');
        errBox.className = 'mermaid-error';
        errBox.innerHTML =
          `<div class="mermaid-error__caption">mermaid render failed: ${escHtml(msg)}</div>` +
          `<pre class="mermaid-error__source"><code>${escHtml(source)}</code></pre>`;
        host.replaceWith(errBox);
      }
    }
  }

  // ── Inline note embed renderer ────────────────────────────────
  // The preprocessor lays down `<div class="transclude-card">` for
  // every ![[path]] in the source; this post-render pass walks
  // those placeholders and replaces them with the actual rendered
  // body of the linked note. Recursion is bounded — we strip
  // ![[…]] from the embedded content before parsing it so an
  // embed-of-an-embed doesn't snowball into a fetch storm or an
  // infinite loop.
  //
  // Shape of a hydrated embed:
  //   <aside class="embed-card">
  //     <header class="embed-card__header">
  //       <a class="wikilink" data-wikilink="…">embedded · Title</a>
  //     </header>
  //     <div class="embed-card__body prose-note">…rendered body…</div>
  //   </aside>
  // The wikilink in the header still triggers onWikilink so the
  // user can jump into the embedded note for full edit access.
  const embedCache = new Map<string, string>();

  async function hydrateEmbeds() {
    if (!mermaidContainer) return;
    const cards = Array.from(mermaidContainer.querySelectorAll('.transclude-card'));
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
          const exact = list.notes.find((n) => n.title.toLowerCase() === target.toLowerCase()) ??
                         list.notes.find((n) => n.path === target || n.path === `${target}.md`);
          const note = exact ?? list.notes[0];
          if (!note) {
            embedCache.set(target, '');
            continue;
          }
          const full = await api.getNote(note.path);
          // Strip ![[…]] from the embed's own body before parsing —
          // otherwise A→B→A or any cycle re-fetches forever. One
          // level of embed is enough; deeper embeds appear as plain
          // wikilink cards inside the embedded content.
          const stripped = (full.body ?? '').replace(/!\[\[[^\]]+\]\]/g, '');
          const pre = preprocess(stripped);
          // Same sanitisation pass as the top-level body. The embed
          // is just a recursive call into the same render — anything
          // unsafe in target.md should be neutralised here BEFORE we
          // assign to aside.innerHTML below, since innerHTML doesn't
          // re-trigger the framework's render-time defences.
          bodyHtml = purify(postprocess(marked.parse(pre, { async: false }) as string));
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
        // Leave the card in place on failure so the user still has
        // a clickable target; the placeholder shape ("embed" label
        // + wikilink) already conveys the intent.
      }
    }
  }

  // Re-run after every html update. Two kinds of work happen here:
  //
  //   - renderMermaidBlocks: cheap if there are no diagrams, fires
  //     immediately on every render. Mermaid hosts are stable
  //     placeholders, so re-walking is OK.
  //
  //   - hydrateEmbeds: walks the DOM and may make API calls. This
  //     used to fire on every keystroke when the user was in
  //     split-mode, which (combined with the DOM mutations from
  //     replaceWith) broke the editor's selection state mid-type.
  //     Now debounced: we only hydrate 350ms after the last body
  //     change, so live typing doesn't thrash the embed list. The
  //     embedCache makes the actual fetch a no-op for already-seen
  //     targets, but the DOM walk + replaceWith was the issue.
  let embedTimer: ReturnType<typeof setTimeout> | null = null;
  $effect(() => {
    void html;
    queueMicrotask(() => {
      void renderMermaidBlocks();
    });
    if (embedTimer) clearTimeout(embedTimer);
    embedTimer = setTimeout(() => {
      embedTimer = null;
      void hydrateEmbeds();
    }, 350);
    return () => {
      if (embedTimer) {
        clearTimeout(embedTimer);
        embedTimer = null;
      }
    };
  });
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="prose-note" bind:this={mermaidContainer} onclick={onClickContainer}>
  {@html html}
</div>
<!-- Wikilink hover preview — listens on the prose container for
     [data-wikilink] hover events, fetches the target note's first
     paragraph (with a session-level cache), and floats a tooltip
     near the link. Click-to-navigate is unchanged; this is purely
     an additive cross-reference aid. -->
<WikilinkHoverPreview host={mermaidContainer} />

<style>
  /* Mermaid rendering — wrapper centres the diagram and lets it
     overflow horizontally when the chart is wider than the column. */
  :global(.mermaid-rendered) {
    margin: 0.9rem 0;
    overflow-x: auto;
    text-align: center;
  }
  :global(.mermaid-rendered svg) {
    max-width: 100%;
    height: auto;
  }
  :global(.mermaid-host) {
    /* Pre-render placeholder. Visible briefly while mermaid is
       lazy-loading so the user sees something instead of a flash of
       empty space. Subdued styling matches diagram-card so the
       transition into the rendered SVG isn't jarring. */
    margin: 0.9rem 0;
    padding: 0.75rem 1rem;
    border: 1px dashed color-mix(in srgb, var(--color-secondary) 40%, transparent);
    border-radius: 0.375rem;
    background: color-mix(in srgb, var(--color-secondary) 4%, transparent);
    color: var(--color-dim);
    font-family: var(--font-mono);
    font-size: 0.85em;
    overflow-x: auto;
  }
  :global(.mermaid-error) {
    margin: 0.9rem 0;
    padding: 0.75rem 1rem;
    border: 1px solid var(--color-error);
    border-radius: 0.375rem;
    background: color-mix(in srgb, var(--color-error) 8%, transparent);
  }
  :global(.mermaid-error__caption) {
    color: var(--color-error);
    font-size: 0.85em;
    font-weight: 600;
    margin-bottom: 0.4rem;
  }
  :global(.mermaid-error__source) {
    margin: 0;
    background: var(--color-surface0);
    border-radius: 0.25rem;
    padding: 0.5rem 0.75rem;
    overflow-x: auto;
  }

  /* Footnote rendering — Pandoc-compatible markup. */
  :global(.footnote-ref a) {
    color: var(--color-secondary);
    text-decoration: none;
    font-size: 0.85em;
    padding: 0 0.1em;
  }
  :global(.footnote-ref a:hover) { text-decoration: underline; }
  :global(.footnote-ref--broken a) {
    color: var(--color-error);
    border-bottom: 1px dotted var(--color-error);
  }
  :global(.footnote-sep) {
    margin: 1.5rem 0 0.5rem;
    border: none;
    border-top: 1px solid var(--color-surface1);
  }
  :global(.footnote-list) {
    margin: 0;
    padding-left: 1.5rem;
    color: var(--color-subtext);
    font-size: 0.92em;
    line-height: 1.55;
  }
  :global(.footnote-list li) { margin: 0.3rem 0; }
  :global(.footnote-list li:target) {
    background: color-mix(in srgb, var(--color-primary) 12%, transparent);
    border-radius: 0.25rem;
    padding: 0.1rem 0.4rem;
  }
  :global(.footnote-back) {
    color: var(--color-dim);
    text-decoration: none;
    margin-left: 0.3em;
  }
  :global(.footnote-back:hover) { color: var(--color-primary); }
  :global(.footnote-missing) { color: var(--color-error); font-style: italic; }

  /* Diagram placeholder */
  :global(.diagram-card) {
    border: 1px dashed color-mix(in srgb, var(--color-secondary) 50%, transparent);
    border-radius: 0.375rem;
    background: color-mix(in srgb, var(--color-secondary) 6%, transparent);
    margin: 0.75rem 0;
    overflow: hidden;
  }
  :global(.diagram-card__header) {
    padding: 0.4rem 0.75rem;
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-secondary);
    border-bottom: 1px dashed color-mix(in srgb, var(--color-secondary) 30%, transparent);
    background: color-mix(in srgb, var(--color-secondary) 8%, transparent);
  }
  :global(.diagram-card__hint) {
    color: var(--color-dim);
    font-weight: normal;
    text-transform: none;
    letter-spacing: normal;
  }
  :global(.diagram-card__source) {
    margin: 0;
    padding: 0.6rem 0.75rem;
    background: transparent;
    border: none;
    overflow-x: auto;
    color: var(--color-subtext);
    font-size: 0.85em;
  }

  /* Image transclusion */
  :global(.note-image) {
    max-width: 100%;
    height: auto;
    border-radius: 0.375rem;
    margin: 0.5rem 0;
    background: var(--color-surface0);
  }

  /* Note transclusion (non-image embed) */
  :global(.transclude-card) {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.75rem;
    margin: 0.5rem 0;
    border: 1px dashed color-mix(in srgb, var(--color-info) 40%, transparent);
    border-radius: 0.375rem;
    background: color-mix(in srgb, var(--color-info) 5%, transparent);
  }
  :global(.transclude-card__label) {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-info);
  }

  /* Hydrated embed — replaces the transclude-card placeholder when
     the embedded note's body lands. The header carries the
     navigation affordance (the same wikilink that triggered the
     embed); the body wraps the rendered sub-note in a subtly
     boxed surface so the boundary is visible without competing
     with the host content. */
  :global(.embed-card) {
    margin: 0.75rem 0;
    padding: 0.75rem 1rem;
    border-left: 3px solid var(--color-info);
    border-radius: 0.375rem;
    background: color-mix(in srgb, var(--color-info) 5%, transparent);
  }
  :global(.embed-card__header) {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
    margin-bottom: 0.5rem;
    padding-bottom: 0.4rem;
    border-bottom: 1px solid color-mix(in srgb, var(--color-info) 25%, transparent);
  }
  :global(.embed-card__label) {
    font-size: 0.65rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--color-info);
    flex-shrink: 0;
  }
  :global(.embed-card__body) {
    /* The body inherits prose-note styling so the embedded content
       reads identically to a directly-rendered note — same heading
       sizes, same callouts, same wikilinks. */
    font-size: 0.95em;
  }

  /* Obsidian callouts */
  :global(.callout) {
    margin: 0.75rem 0;
    padding: 0.6rem 0.85rem;
    border-left: 3px solid var(--color-subtext);
    border-radius: 0.375rem;
    background: color-mix(in srgb, var(--color-subtext) 6%, transparent);
  }
  :global(.callout--info) {
    border-left-color: var(--color-info);
    background: color-mix(in srgb, var(--color-info) 8%, transparent);
  }
  :global(.callout--success) {
    border-left-color: var(--color-success);
    background: color-mix(in srgb, var(--color-success) 8%, transparent);
  }
  :global(.callout--warning) {
    border-left-color: var(--color-warning);
    background: color-mix(in srgb, var(--color-warning) 8%, transparent);
  }
  :global(.callout--error) {
    border-left-color: var(--color-error);
    background: color-mix(in srgb, var(--color-error) 8%, transparent);
  }
  :global(.callout--primary) {
    border-left-color: var(--color-primary);
    background: color-mix(in srgb, var(--color-primary) 8%, transparent);
  }
  :global(.callout--accent) {
    border-left-color: var(--color-accent);
    background: color-mix(in srgb, var(--color-accent) 8%, transparent);
  }
  :global(.callout__header) {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font-weight: 600;
    color: var(--color-text);
    margin-bottom: 0.25rem;
  }
  :global(.callout__icon) {
    color: inherit;
    font-size: 0.7em;
    opacity: 0.7;
  }
  :global(.callout p:last-child) {
    margin-bottom: 0;
  }

  /* Error fallback */
  :global(.prose-note__error) {
    border: 1px solid var(--color-error);
    border-radius: 0.375rem;
    background: color-mix(in srgb, var(--color-error) 8%, transparent);
    padding: 0.75rem;
    margin: 0.75rem 0;
  }
</style>
