<script lang="ts">
  import { marked } from 'marked';

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

  // Diagrams + image transclusions stashed during preprocess and
  // restored as styled elements in postprocess. Module-scoped so the
  // helpers can share state across one render pass — $derived re-runs
  // the whole function so there's no cross-render leakage.
  let diagramCache: { kind: string; source: string }[] = [];
  let imageCache: string[] = [];

  // Image extensions we know how to embed via the file API. Anything
  // else (PDF, audio, video, .canvas) falls through to the plain
  // wikilink path so the user at least gets a click target.
  const IMAGE_EXTS = /\.(png|jpe?g|gif|webp|svg|avif|bmp|ico)$/i;

  function preprocess(src: string): string {
    let s = stripFrontmatter(src);
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
    // Diagrams → styled cards.
    const dgRe = new RegExp(`${esc(DIAGRAM_OPEN)}(\\d+)${esc(DIAGRAM_CLOSE)}`, 'g');
    s = s.replace(dgRe, (_, idx: string) => {
      const d = diagramCache[Number(idx)];
      if (!d) return '';
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
    // Obsidian callouts last so the regex sees the final tree.
    s = rewriteCallouts(s);
    return s;
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
      return postprocess(out);
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
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
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="prose-note" onclick={onClickContainer}>
  {@html html}
</div>

<style>
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
