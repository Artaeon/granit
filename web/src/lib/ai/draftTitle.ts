// Pure helpers for the "save AI reply as note" flow in
// AIOverlay. Extracted so the title-derivation logic can be
// unit-tested independently of the dialog state — it's the
// piece most likely to surprise the user ("why did it pick THAT
// as the filename?"), and locking the precedence with tests
// keeps regressions out.

/** deriveDraftTitle — pick the best human-readable title for
 *  the saved note. Precedence:
 *    1. First H1 heading in the body
 *    2. First non-empty, non-quote, non-fence line
 *    3. Fallback: "AI draft <today's date>" so the file lands
 *       under a discoverable name even for one-line drafts.
 *
 *  Long titles are clipped at 60 chars. The caller is
 *  responsible for slugifying the result into a filename.
 *
 *  todayISO is dep-injected so tests don't depend on Date.now(). */
export function deriveDraftTitle(body: string, todayISO: string): string {
	const lines = (body ?? '').split('\n');
	// Pass 1 — H1 (with or without trailing whitespace, no leading
	// indentation tolerance because real model output is consistent).
	for (const ln of lines) {
		const m = ln.match(/^#\s+(.+?)\s*$/);
		if (m && m[1].trim()) return clip(m[1].trim());
	}
	// Pass 2 — first non-empty line that isn't a quote / fence /
	// list marker / horizontal rule. We accept emphasis markers
	// inline (the trim catches surrounding asterisks naturally).
	for (const ln of lines) {
		const t = ln.trim();
		if (!t) continue;
		if (t.startsWith('>')) continue;
		if (t.startsWith('```')) continue;
		if (t === '---' || t === '***') continue;
		// Strip a leading list marker so "- write the brief" yields
		// "write the brief", not "- write the brief".
		const stripped = t.replace(/^[-*+]\s+/, '').replace(/^\d+[.)]\s+/, '');
		if (!stripped) continue;
		return clip(stripped);
	}
	return `AI draft ${todayISO}`;
}

function clip(s: string): string {
	if (s.length <= 60) return s;
	return s.slice(0, 60).trimEnd();
}
