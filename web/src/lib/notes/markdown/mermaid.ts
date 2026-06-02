// Mermaid diagram renderer for MarkdownRenderer.
//
// The transforms module lays down `<pre class="mermaid-host">`
// placeholders for every ```mermaid block. This module walks them
// post-render and replaces each with the rendered SVG.
//
// Two reasons this lives here rather than inline in
// MarkdownRenderer:
//
//   1. The mermaid library is ~1MB. Lazy-loading it means users who
//      never write a mermaid block don't pay the bundle cost. The
//      import promise lives at TRUE module scope here, so the
//      library is fetched once per session no matter how many
//      MarkdownRenderer instances mount.
//
//   2. The id counter for mermaid.render is module-scope too — two
//      simultaneous renderers (split view, embed recursion) can't
//      clash on `mermaid-1` because the counter monotonically
//      increases across the whole session.
//
// initialize() runs on every renderMermaidBlocks call so a runtime
// theme toggle (data-theme attribute on <html>) takes effect on the
// next render without a page reload. The call is idempotent so the
// redundant init is cheap.

import { errorMessage } from '$lib/util/errorMessage';
import { escHtml } from './transforms';

let mermaidPromise: Promise<typeof import('mermaid').default> | null = null;
let mermaidIdCounter = 0;

/** The body's theme controls mermaid's palette. Read off the
 *  <html data-theme> attribute the parent app sets — observation
 *  via MutationObserver would be overkill since we re-detect on
 *  every render anyway. */
function detectMermaidTheme(): 'dark' | 'default' {
  if (typeof document === 'undefined') return 'default';
  const t = document.documentElement.getAttribute('data-theme');
  if (t === 'light') return 'default';
  return 'dark';
}

async function getMermaid(): Promise<typeof import('mermaid').default> {
  if (!mermaidPromise) {
    mermaidPromise = import('mermaid').then((m) => m.default);
  }
  return mermaidPromise;
}

/** Walk every `pre.mermaid-host[data-mermaid-source]` in `container`
 *  and replace it with the rendered SVG. A render failure keeps the
 *  source visible alongside a small error caption — better than a
 *  silent blank. */
export async function renderMermaidBlocks(container: HTMLElement | undefined): Promise<void> {
  if (!container) return;
  const hosts = Array.from(
    container.querySelectorAll<HTMLPreElement>('pre.mermaid-host[data-mermaid-source]')
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
    // Detachment guard: the await inside this loop yields the event
    // loop. If a body update lands during the yield (common during
    // streaming AI responses where the renderer's html effect fires
    // per-token), MarkdownRenderer's {@html} replaces the whole
    // container's children before the await resolves. The `host` ref
    // we captured is now disconnected; replaceWith on a node with no
    // parent throws and would leave half-rendered diagrams.
    // The next render pass (already scheduled) walks the fresh
    // hosts, so skipping detached ones is safe.
    if (!host.isConnected) continue;
    const source = host.getAttribute('data-mermaid-source') ?? '';
    const id = `mermaid-${++mermaidIdCounter}`;
    try {
      const { svg, bindFunctions } = await mermaid.render(id, source);
      if (!host.isConnected) continue;
      const wrapper = document.createElement('div');
      wrapper.className = 'mermaid-rendered';
      wrapper.innerHTML = svg;
      // bindFunctions wires up click handlers in flowcharts that
      // use `click NodeId callback "tooltip"`. Skipping it would
      // silently break interactivity in user-authored diagrams.
      bindFunctions?.(wrapper);
      host.replaceWith(wrapper);
    } catch (err) {
      if (!host.isConnected) continue;
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
