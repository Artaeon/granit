// Shared core for the in-app "agents" — small structured-action
// AI surfaces that take a free-text intent, stream typed actions,
// and let the user accept/skip each. The first such surface is
// the Task Agent (lib/tasks/agent.ts + TaskAgent.svelte); the
// Project Agent uses the same shape.
//
// Pure utilities only. No DOM, no API calls. Component-side
// concerns (focus, dialog state, streaming) live in each Agent
// component; entity-specific concerns (prompt strings, action
// validators, revert patches) live in the per-entity modules.
//
// Why a shared module instead of per-entity duplication: the
// merge semantics for re-streamed proposals are subtle (preserve
// applied/rejected rows even when the new parse drops them;
// freeze action args once engaged so the row doesn't lie about
// what was applied). Bugs in this logic should be fixed in ONE
// place — not three, six months from now.

/** Flags the dialog UI adds to a proposed action as the user
 *  interacts with it. The pure agent modules never set these;
 *  they live on the component-side row state. Shared here so
 *  every agent merges proposals identically. */
export interface ProposalFlags {
	applied?: boolean;
	applying?: boolean;
	rejected?: boolean;
}

/** mergeProposals — re-stream merge. When the model streams JSON
 *  we re-parse on every chunk; the validated action list grows
 *  as more arrives. Two guarantees the naive "replace proposals"
 *  does NOT give:
 *
 *  1. A row the user already accepted / rejected must stay
 *     visible, even if a later chunk no longer mentions it
 *     (validateActions may drop a row whose entity left scope
 *     after apply, or the model may retract a suggestion).
 *  2. The action args for an already-engaged row are FROZEN.
 *     The accepted patch went out with the old args; redisplaying
 *     the row with new args would lie about what was applied.
 *
 *  Engaged rows that the new parse no longer mentions are
 *  appended after the live ones (visual sanity — live rows
 *  on top, frozen rows underneath so they don't move around).
 *  PENDING rows the new parse dropped are intentional retractions
 *  by the model — we let them go to reduce churn.
 *
 *  Generic over the action type. The caller supplies a key
 *  function so the merge identifies "the same action proposal"
 *  by entity id + kind (different entities use different id
 *  fields — Task.id vs Project.name). */
export function mergeProposals<A>(
	prev: (A & ProposalFlags)[],
	next: A[],
	key: (a: A) => string
): (A & ProposalFlags)[] {
	const prevMap = new Map<string, A & ProposalFlags>();
	for (const r of prev) prevMap.set(key(r), r);

	const seen = new Set<string>();
	const merged: (A & ProposalFlags)[] = [];
	for (const a of next) {
		const k = key(a);
		seen.add(k);
		const old = prevMap.get(k);
		if (old && (old.applied || old.rejected)) {
			// Frozen — keep the old row verbatim, including its
			// recorded args. Don't overwrite with the new parse.
			merged.push(old);
		} else {
			merged.push({ ...a, applied: old?.applied, rejected: old?.rejected } as A &
				ProposalFlags);
		}
	}
	for (const r of prev) {
		const k = key(r);
		if (!seen.has(k) && (r.applied || r.rejected)) {
			merged.push(r);
		}
	}
	return merged;
}

/** extractActions — generic JSON-array extraction for agent
 *  responses. The model is told to return STRICT JSON of the form
 *  {"actions":[...]}; in practice it sometimes wraps with prose
 *  or fences. This strips both, slices to the first/last brace,
 *  and JSON.parse — returns the actions array or []. The
 *  per-entity validator drops malformed entries from there.
 *
 *  Centralised so a model quirk that surfaces in one agent
 *  (e.g. trailing commas, a stray markdown header) gets fixed
 *  for all agents at once. */
export function extractActions(raw: string): unknown[] {
	let s = (raw ?? '').trim();
	if (s.startsWith('```')) {
		s = s.replace(/^```(?:json)?\s*/i, '').replace(/```\s*$/, '').trim();
	}
	const firstBrace = s.indexOf('{');
	const lastBrace = s.lastIndexOf('}');
	if (firstBrace >= 0 && lastBrace > firstBrace) {
		s = s.slice(firstBrace, lastBrace + 1);
	}
	let parsed: unknown;
	try {
		parsed = JSON.parse(s);
	} catch {
		return [];
	}
	if (!parsed || typeof parsed !== 'object') return [];
	const arr = (parsed as { actions?: unknown }).actions;
	if (!Array.isArray(arr)) return [];
	return arr;
}
