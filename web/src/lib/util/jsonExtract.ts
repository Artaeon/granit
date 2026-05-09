// Extract the first {...} JSON object from a streaming LLM reply.
//
// Models occasionally wrap structured output in ```json fences or
// add a sentence of commentary before/after the object. The strict
// "JSON only" prompt cuts most of this, but we still see partial
// fences while a stream is mid-flight, or trailing chatter on
// older local models. This helper:
//
//   • peels a fenced block if one is present (json-tagged or not)
//   • finds the first '{' and walks brace depth to its match
//   • returns null if the structure isn't yet complete
//
// The "walk depth" form (rather than a single-shot regex) lets the
// caller call this on every streamed chunk — most chunks return
// null until the closing brace lands, then one chunk completes.
// Cheap enough to run per-token (no regex backtracking risk).
//
// Used by the tasks-page Plan-my-day stream and could be reused by
// any future structured-output AI surface.

export function extractJsonBlock(s: string): string | null {
  if (!s) return null;
  const fence = s.match(/```(?:json)?\s*([\s\S]*?)```/);
  const candidate = fence ? fence[1] : s;
  const start = candidate.indexOf('{');
  if (start < 0) return null;
  let depth = 0;
  for (let i = start; i < candidate.length; i++) {
    const c = candidate[i];
    if (c === '{') depth++;
    else if (c === '}') {
      depth--;
      if (depth === 0) return candidate.slice(start, i + 1);
    }
  }
  return null;
}
