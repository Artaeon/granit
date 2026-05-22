// Tiny inline-markdown formatter for short strings (task text, headings).
// HTML-escapes first, then applies bold / italic / code / wikilink styling.

const escMap: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;'
};

function escape(s: string): string {
  return s.replace(/[&<>"']/g, (c) => escMap[c]);
}

// Module-scope LRU. Tasks, goals, and reviewed-headings on the
// dashboard re-render the same handful of strings dozens of times
// per render pass (parent reactivity, $derived recompute, list-key
// churn). Each pass burns 4 regex replaces per call. With a cache,
// repeats become a Map lookup. Evict-oldest-first keeps memory
// bounded; 512 entries cover the active task set on a busy day
// without breaking on a notes-graph hover that might splash hundreds
// of unique strings through.
const INLINE_MD_CACHE_LIMIT = 512;
const inlineMdCache = new Map<string, string>();

/**
 * Returns sanitized HTML with **bold**, *italic*, `code`, and [[wikilinks]]
 * replaced by safe spans. Use with `{@html ...}`.
 */
export function inlineMd(text: string): string {
  if (!text) return '';
  const hit = inlineMdCache.get(text);
  if (hit !== undefined) {
    // LRU touch: re-insert so it moves to the tail of the iteration
    // order (Map preserves insertion order, so the first key is the
    // oldest entry we'd evict on overflow).
    inlineMdCache.delete(text);
    inlineMdCache.set(text, hit);
    return hit;
  }
  let s = escape(text);
  // wikilinks first so they render even inside bold
  s = s.replace(/\[\[([^\]\n]+?)\]\]/g, (_, inner) => {
    const target = inner.split('|')[0].trim();
    const display = inner.split('|').pop()!.trim();
    return `<span class="wl" data-target="${escape(target)}">${escape(display)}</span>`;
  });
  // **bold**
  s = s.replace(/\*\*([^*\n]+?)\*\*/g, '<strong>$1</strong>');
  // *italic* — avoid matching ** that we just consumed (negative lookbehind for *)
  s = s.replace(/(^|[^*])\*([^*\n]+?)\*(?!\*)/g, '$1<em>$2</em>');
  // `code`
  s = s.replace(/`([^`\n]+?)`/g, '<code class="px-1 rounded bg-surface1 text-accent text-[0.92em]">$1</code>');

  if (inlineMdCache.size >= INLINE_MD_CACHE_LIMIT) {
    const oldest = inlineMdCache.keys().next().value;
    if (oldest !== undefined) inlineMdCache.delete(oldest);
  }
  inlineMdCache.set(text, s);
  return s;
}
