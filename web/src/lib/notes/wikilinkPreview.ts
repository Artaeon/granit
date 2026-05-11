// Wikilink hover preview — text-only helpers, shared by every surface
// that shows wikilinks (MarkdownRenderer preview pane, embedded note
// cards, future hover-on-editor wikilinks). The DOM positioning + the
// fetch live in WikilinkHoverPreview.svelte; the bits here are pure
// functions a vitest run can pin without rendering anything.
//
// The "first paragraph" view is a deliberate scope choice over "first
// 200 chars": users have a strong prose-level expectation that a
// preview shows them a self-contained idea, not a truncated sentence.

/** stripFrontmatter removes a leading YAML frontmatter block from a
 *  note body so the preview doesn't display "title: Foo\ntype: ..."
 *  as the lead text. Returns the body unchanged when no frontmatter
 *  is present, so callers can pipe through unconditionally. */
export function stripFrontmatter(body: string): string {
	if (!body.startsWith('---')) return body;
	const m = body.match(/^---\n([\s\S]*?)\n---\n?/);
	if (!m) return body;
	return body.slice(m[0].length);
}

/** extractFirstParagraph picks the first meaningful prose paragraph
 *  from a note body. Skips:
 *    - the leading H1 (its title is shown separately in the preview)
 *    - empty lines
 *    - markdown list / heading / code-fence markers
 *    - <html>-style HTML embed lines
 *  Returns up to ~400 chars so the floating tooltip stays a tooltip,
 *  not a wall of text. */
export function extractFirstParagraph(body: string): string {
	const lines = stripFrontmatter(body).split(/\r?\n/);
	const buf: string[] = [];
	let inFence = false;
	let started = false;
	for (const raw of lines) {
		const trimmed = raw.trim();
		// Code-fence boundary toggles. Skip the fenced content entirely
		// — code as a "preview" is hostile UX in a tooltip.
		if (trimmed.startsWith('```') || trimmed.startsWith('~~~')) {
			inFence = !inFence;
			if (started) break;
			continue;
		}
		if (inFence) continue;
		// Empty line ends the paragraph once we have content.
		if (trimmed === '') {
			if (started) break;
			continue;
		}
		// Skip a single leading H1 — its content typically duplicates
		// the note title that the tooltip header already shows.
		if (!started && /^#\s+/.test(trimmed)) continue;
		// Skip other heading lines without ending the paragraph search.
		if (/^#{1,6}\s+/.test(trimmed)) continue;
		// Skip list markers / blockquote pointers as the LEAD line —
		// they're rarely the right "what is this note about" preview.
		// Once we've started a paragraph, follow-up list lines are
		// OK because they're the natural continuation.
		if (!started && /^[-*+>]\s+/.test(trimmed)) continue;
		// Skip frontmatter-style key: value lines that escaped the
		// stripper (e.g. a note that starts with "key: value" outside
		// a YAML block — still noisy as a preview).
		if (!started && /^[A-Za-z_][\w-]*:\s/.test(trimmed)) continue;

		buf.push(trimmed);
		started = true;
		// Cap aggressively. The tooltip is for orientation, not reading.
		if (buf.join(' ').length > 400) break;
	}
	const out = buf.join(' ').trim();
	if (out.length <= 400) return out;
	// Don't break mid-word — snap to the previous space.
	const cut = out.slice(0, 400);
	const lastSpace = cut.lastIndexOf(' ');
	return (lastSpace > 300 ? cut.slice(0, lastSpace) : cut) + '…';
}

/** stripInlineMarkdown gives the tooltip plain text — wikilinks,
 *  bold, italic, inline code, footnote refs all collapse to their
 *  content. We never render markdown in the tooltip itself (would
 *  invite XSS via {@html} and clutter the layout). */
export function stripInlineMarkdown(s: string): string {
	let out = s;
	// Image embeds ![[file]] → drop entirely (no text equivalent).
	// MUST run before the wikilink rule below or the `[[file]]` part
	// gets unwrapped first and leaves an orphan "!".
	out = out.replace(/!\[\[[^\]]+\]\]/g, '');
	// Wikilinks [[Note|alias]] / [[Note]] → alias or Note.
	out = out.replace(/\[\[([^\]|]+)(?:\|([^\]]+))?\]\]/g, (_, t, alias) => alias || t);
	// Footnote refs [^id] → drop.
	out = out.replace(/\[\^[^\]]+\]/g, '');
	// Markdown links [text](url) → text.
	out = out.replace(/\[([^\]]+)\]\([^)]*\)/g, '$1');
	// Inline code `x` → x.
	out = out.replace(/`([^`]+)`/g, '$1');
	// Bold/italic/strike *x* **x** _x_ ~x~ → x.
	out = out.replace(/\*\*([^*]+)\*\*/g, '$1');
	out = out.replace(/\*([^*]+)\*/g, '$1');
	out = out.replace(/_([^_]+)_/g, '$1');
	out = out.replace(/~~([^~]+)~~/g, '$1');
	// Collapse leftover whitespace.
	return out.replace(/\s+/g, ' ').trim();
}
