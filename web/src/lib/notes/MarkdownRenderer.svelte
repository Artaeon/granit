<script lang="ts">
  import { marked } from 'marked';

  let { body, onWikilink }: { body: string; onWikilink?: (target: string) => void } = $props();

  // Pre-process the source so wikilinks survive as inline tokens marked
  // doesn't recognize natively. We use a sentinel form that's unlikely to
  // appear in user content, then replace it post-render. Same trick for
  // inline #tags so they get colored without breaking heading IDs.
  const WIKI_OPEN = 'WL[';
  const WIKI_CLOSE = ']';
  const TAG_OPEN = 'TG[';
  const TAG_CLOSE = ']';

  function preprocess(src: string): string {
    let s = src.replace(/\[\[([^\]\n]+?)\]\]/g, (_, inner) => {
      const target = String(inner).split('|')[0].trim();
      const display = String(inner).split('|').pop()!.trim();
      return `${WIKI_OPEN}${target}|${display}${WIKI_CLOSE}`;
    });
    // Avoid replacing # tags inside fenced code blocks
    const fences: string[] = [];
    s = s.replace(/```[\s\S]*?```/g, (m) => {
      fences.push(m);
      return `F${fences.length - 1}`;
    });
    s = s.replace(/(^|[\s(])#([\p{L}\p{N}_/-]+)/gu, (_, pre, tag) => `${pre}${TAG_OPEN}${tag}${TAG_CLOSE}`);
    s = s.replace(/F(\d+)/g, (_, i) => fences[Number(i)]);
    return s;
  }

  function postprocess(html: string): string {
    let s = html;
    // Wikilinks → anchors
    s = s.replace(
      new RegExp(`${esc(WIKI_OPEN)}([^|]+)\\|([^${esc(WIKI_CLOSE)}]+)${esc(WIKI_CLOSE)}`, 'g'),
      (_, target: string, display: string) =>
        `<a class="wikilink" data-wikilink="${escAttr(target)}">${escHtml(display)}</a>`
    );
    // Tags → spans
    s = s.replace(
      new RegExp(`${esc(TAG_OPEN)}([^${esc(TAG_CLOSE)}]+)${esc(TAG_CLOSE)}`, 'g'),
      (_, tag: string) => `<span class="hashtag">#${escHtml(tag)}</span>`
    );
    return s;
  }

  function esc(s: string): string {
    return s.replace(/[\\^$*+?.()|[\]{}]/g, '\\$&');
  }
  function escAttr(s: string): string {
    return s.replace(/"/g, '&quot;').replace(/&/g, '&amp;');
  }
  function escHtml(s: string): string {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  // marked options — GFM, tables, breaks
  marked.setOptions({ gfm: true, breaks: false });

  let html = $derived.by(() => {
    if (!body) return '';
    const pre = preprocess(body);
    const out = marked.parse(pre, { async: false }) as string;
    return postprocess(out);
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
