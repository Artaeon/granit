<script lang="ts">
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';
  import { api } from '$lib/api';
  import { errorMessage } from '$lib/util/errorMessage';
  import WikilinkHoverPreview from './WikilinkHoverPreview.svelte';
  import {
    preprocess,
    postprocess,
    escHtml
  } from './markdown/transforms';
  import { createEmbedHydrator } from './markdown/embeds';
  import { renderMermaidBlocks } from './markdown/mermaid';

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


  // Render is wrapped in try/catch so a malformed token (corner-case
  // marked bug, half-edited fence, weird unicode) shows the source
  // verbatim instead of taking the page hostage. Without this, a
  // throwing marked.parse would bubble up through the compute pass
  // and the preview would silently render nothing.
  //
  // Rendering is async + generation-guarded so toggling reading mode
  // on a large note (10k words, code blocks, mermaid, embeds) doesn't
  // block the main thread for hundreds of ms. The flow:
  //
  //   1. Bump the generation counter and flip `rendering` true.
  //   2. Yield one rAF tick so Svelte can paint the loading skeleton
  //      BEFORE marked + DOMPurify burn CPU. Without the yield the
  //      whole compute runs in the same microtask as the effect and
  //      the user sees the original freeze.
  //   3. Drive marked in async mode — it yields between blocks so
  //      huge docs no longer monopolise the thread for the whole
  //      parse.
  //   4. Generation check after each await: if `body` changed mid-
  //      compute (user toggled back or edited), abandon the stale
  //      pass instead of clobbering the newer html.
  let html = $state('');
  let rendering = $state(false);
  let renderGen = 0;

  // Parse-and-sanitize cache, INSTANCE-scope. Keyed by the raw body
  // string; value is the post-purify HTML. LRU semantics via Map
  // insertion order — a hit re-inserts to the tail, evictions take
  // the head.
  //
  // This used to be module-scope (shared across every renderer in
  // the app). That caused a freeze under a real workload: a streaming
  // AI summary renders a fresh body string every rAF tick — 60+
  // insertions/sec — turning the 20-entry cache over in under a
  // second and evicting the user's preview cache on every AI stream.
  // The preview was forced to re-parse on its next paint while the
  // summary stream was still active. Instance-scope keeps each
  // renderer's cache isolated; cross-tab/cross-mount instant repaint
  // is lost but that was nice-to-have, not a correctness path.
  const HTML_CACHE_CAP = 20;
  const HTML_CACHE = new Map<string, string>();

  async function computeHtml(src: string): Promise<void> {
    const myGen = ++renderGen;
    if (!src) {
      html = '';
      rendering = false;
      return;
    }
    // Instance-scope LRU cache hit (see HTML_CACHE declaration above):
    // a view-mode toggle or rapid keystroke-then-revert paints
    // instantly instead of re-running marked + purify (~50–150ms on
    // a 600-line doc). Cross-instance sharing was reverted on purpose
    // — see the cap-comment above.
    const cached = HTML_CACHE.get(src);
    if (cached !== undefined) {
      HTML_CACHE.delete(src);
      HTML_CACHE.set(src, cached);
      html = cached;
      rendering = false;
      return;
    }
    rendering = true;
    try {
      // Yield to the browser so the loading state paints before
      // marked + purify burn CPU. requestAnimationFrame is only
      // available client-side; SSR falls back to a microtask yield
      // which still lets the runtime breathe.
      await new Promise<void>((r) => {
        if (typeof requestAnimationFrame === 'function') {
          requestAnimationFrame(() => r());
        } else {
          queueMicrotask(() => r());
        }
      });
      if (myGen !== renderGen) return;
      // Preprocess + carry the state object through to postprocess
      // so the carved-out caches (diagrams, images, footnotes) are
      // scoped to THIS render pass. Concurrent renders (split-view)
      // each carry their own state and can't clobber each other.
      const { preprocessed, state } = preprocess(src);
      // marked.parse with { async: true } returns a Promise and the
      // library yields between blocks, so a long doc no longer pins
      // the main thread for the whole parse.
      const out = (await marked.parse(preprocessed, { async: true })) as string;
      if (myGen !== renderGen) return;
      // purify() is the load-bearing XSS defence — marked itself
      // dropped its `sanitize` option in v7 and explicitly recommends
      // DOMPurify downstream. The threat surface is small (self-
      // hosted single-user vault) but real once notes sync across
      // devices: a `<script>` slipped into a note on device A would
      // execute on device B without this layer. PURIFY_CONFIG keeps
      // our data-wikilink / data-tag etc. hooks alive so click
      // delegation still works after sanitisation.
      const sanitized = purify(postprocess(out, state, src));
      if (myGen !== renderGen) return;
      html = sanitized;
      // Memoize for future tab switches / dup renders. Evict the
      // oldest entry once we exceed the cap; insertion order is
      // the LRU dimension because cache hits above re-insert at the
      // tail. Cap is small (~20) — markdown HTML for a 600-line
      // doc is typically 100–500 KB, so 20 × 500 KB = ~10 MB worst
      // case, well below any reasonable budget for a single tab.
      HTML_CACHE.set(src, sanitized);
      if (HTML_CACHE.size > HTML_CACHE_CAP) {
        const oldest = HTML_CACHE.keys().next().value;
        if (oldest !== undefined) HTML_CACHE.delete(oldest);
      }
    } catch (e) {
      if (myGen !== renderGen) return;
      const msg = errorMessage(e);
      html =
        `<div class="prose-note__error">` +
        `<p><strong>Preview render failed:</strong> ${escHtml(msg)}</p>` +
        `<p class="text-dim text-sm">Showing source. Edit mode still works.</p>` +
        `<pre><code>${escHtml(src)}</code></pre>` +
        `</div>`;
    } finally {
      if (myGen === renderGen) rendering = false;
    }
  }

  // `void computeHtml(body)` keeps `body` the only tracked dep — we
  // intentionally don't want `html`/`rendering` reads inside the
  // function to re-trigger this effect.
  $effect(() => {
    void computeHtml(body);
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

  // Mermaid + embed containers share the same DOM root — the rendered
  // markdown lives inside `mermaidContainer` (legacy name kept so the
  // bind:this in the template stays a stable reference). The two
  // post-render side effects (mermaid hydration, embed hydration)
  // each walk this container looking for their own placeholders.
  // Mermaid lazy-loads via $lib/notes/markdown/mermaid — its module-
  // scope state (the import Promise, the id counter) means two
  // simultaneous renderers can't clash on diagram ids and we only
  // fetch the ~1MB library once per session.
  let mermaidContainer: HTMLDivElement | undefined = $state();

  // Inline note-embed hydrator — walks `.transclude-card` placeholders
  // and replaces them with rendered embed bodies. Implementation +
  // module-scope embed cache live in $lib/notes/markdown/embeds. The
  // factory pattern lets us bind the container ref + purify pass
  // without those leaking into the embeds module.
  const { hydrateEmbeds } = createEmbedHydrator({
    getContainer: () => mermaidContainer,
    purify
  });

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
      void renderMermaidBlocks(mermaidContainer);
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
<div class="prose-note relative" bind:this={mermaidContainer} onclick={onClickContainer}>
  {#if rendering && !html}
    <!-- Initial-render skeleton — three pulsing lines, no layout shift.
         Shown only while there is no previous html to fall back to;
         re-renders (stale html + rendering) dim the existing tree
         instead so the reader keeps their context. -->
    <div class="space-y-3 pt-2">
      <div class="h-3 w-3/4 bg-surface1 rounded animate-pulse"></div>
      <div class="h-3 w-full bg-surface1 rounded animate-pulse"></div>
      <div class="h-3 w-5/6 bg-surface1 rounded animate-pulse"></div>
    </div>
  {/if}
  <div class:opacity-50={rendering && html} class="transition-opacity duration-150">
    {@html html}
  </div>
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
