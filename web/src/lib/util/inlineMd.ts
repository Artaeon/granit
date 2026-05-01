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

/**
 * Returns sanitized HTML with **bold**, *italic*, `code`, and [[wikilinks]]
 * replaced by safe spans. Use with `{@html ...}`.
 */
export function inlineMd(text: string): string {
  if (!text) return '';
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
  return s;
}
