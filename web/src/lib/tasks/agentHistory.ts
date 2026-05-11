// Persisted intent history for the TaskAgent.
//
// Why: typing a sharp intent like "archive anything older than two
// weeks with no due date and no priority" is real work. If the
// agent's intent box wipes every reopen, users either repeat the
// effort or fall back to vague prompts. A localStorage-backed
// history lets the dialog show "last 5 things you asked" as
// one-click chips, so a successful intent becomes a reusable
// pattern.
//
// Pure module (no DOM, no Svelte) so vitest can pin the dedup +
// cap behaviour without rendering anything. The component owns
// the actual localStorage IO via $lib/util/storage helpers.

/** Max history entries we keep. Past 8, the chip row gets noisy
 *  and the user is better served by re-typing. */
export const MAX_HISTORY = 8;

/** Add a new intent to the front of the history, dedup against
 *  case-insensitive matches, drop empty/whitespace, cap at
 *  MAX_HISTORY. Pure — caller persists the result. */
export function addIntentToHistory(prev: string[], next: string): string[] {
	const trimmed = next.trim();
	if (!trimmed) return prev;
	const filtered = prev.filter((p) => p.trim().toLowerCase() !== trimmed.toLowerCase());
	return [trimmed, ...filtered].slice(0, MAX_HISTORY);
}

/** Normalise on load: drops non-strings, trims, dedupes (a corrupt
 *  localStorage value or a hand-edited one shouldn't crash the
 *  dialog). Returns an empty list when the input isn't an array. */
export function normaliseHistory(raw: unknown): string[] {
	if (!Array.isArray(raw)) return [];
	const seen = new Set<string>();
	const out: string[] = [];
	for (const item of raw) {
		if (typeof item !== 'string') continue;
		const t = item.trim();
		if (!t) continue;
		const k = t.toLowerCase();
		if (seen.has(k)) continue;
		seen.add(k);
		out.push(t);
		if (out.length >= MAX_HISTORY) break;
	}
	return out;
}
