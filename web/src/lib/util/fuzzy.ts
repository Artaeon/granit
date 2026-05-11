// Tiny subsequence-style fuzzy matcher for the Mod-K / Mod-P quick
// switcher. Lives outside the component so we can unit-test it
// directly and reuse it from anywhere a small client-side filter
// would otherwise reach for fuse.js (we don't want the bundle weight
// for a 200-item, hot-key-driven list).
//
// Match rules — descending score:
//   1) exact case-insensitive equality                                 → very high
//   2) prefix match (haystack starts with needle)                      → high
//   3) substring match (needle appears contiguously anywhere)          → medium
//   4) subsequence match (every needle char appears in order in hay)   → low
//   5) no match                                                        → null
//
// Ties within a band break by haystack length (shorter wins — the
// closer the match is to "all" of the candidate, the more likely it's
// the user's intent). Subsequence matches also weight by run length
// — a tight cluster ("proj" hitting "Projects") scores higher than a
// spread one ("pjs" against "Project Settings") so the obvious match
// floats up.
//
// Why not Fuse: fuse.js is 12kb gzipped and configurable in ten
// directions we don't need. This is 50 lines and exactly fits the
// shape (mostly-prefix muscle memory, with subsequence as the safety
// net for typos / partial recall).

/** Match score in the closed interval [0, 1000]. Higher = better.
 *  Returns null when the needle doesn't match at all (no subsequence
 *  pass). An empty needle returns a small neutral score so callers
 *  can keep every candidate in an empty-query list. */
export function fuzzyScore(needle: string, haystack: string): number | null {
  if (!needle) return 1; // neutral — keep everything in empty-query lists
  const n = needle.toLowerCase();
  const h = haystack.toLowerCase();

  if (h === n) return 1000;
  if (h.startsWith(n)) return 800 - Math.min(h.length, 200);
  const idx = h.indexOf(n);
  if (idx >= 0) {
    // Substring: penalise by where in the string it sits (matches
    // earlier are more useful) and by length (shorter haystack is a
    // tighter match).
    return 600 - idx - Math.min(h.length, 200);
  }

  // Subsequence: every char of n must appear in h, in order. We
  // reward tight clusters (consecutive run length) and matches that
  // land at word boundaries.
  let hi = 0;
  let runLen = 0;
  let totalRun = 0;
  let boundaryHits = 0;
  let lastMatchIdx = -1;
  for (let ni = 0; ni < n.length; ni++) {
    const c = n.charCodeAt(ni);
    let found = -1;
    while (hi < h.length) {
      if (h.charCodeAt(hi) === c) {
        found = hi;
        hi++;
        break;
      }
      hi++;
    }
    if (found < 0) return null;
    // Boundary check: previous char in haystack is a separator,
    // or this is the first char. Word-boundary subsequence beats
    // mid-word subsequence (e.g. "pn" should prefer "project notes"
    // over "open").
    if (found === 0 || isSeparator(h.charCodeAt(found - 1))) boundaryHits++;
    if (found === lastMatchIdx + 1) runLen++;
    else runLen = 1;
    totalRun += runLen;
    lastMatchIdx = found;
  }
  // Base subseq score under substring (max 400 here vs 600 floor
  // above) so a substring always wins a subsequence. Bonuses:
  //   +12 per boundary hit (clamped) — rewards "pn"→"project notes"
  //   +2  per total-run unit       — rewards tight clusters
  //   −1  per haystack char        — shorter beats longer on ties
  return (
    300 +
    Math.min(boundaryHits * 12, 60) +
    Math.min(totalRun * 2, 40) -
    Math.min(h.length, 200)
  );
}

/** Character codes that count as word boundaries for the
 *  subsequence-bonus calculation. Space, dash, underscore, slash,
 *  dot, colon — the separators we actually see in note paths,
 *  project names, and page labels. */
function isSeparator(code: number): boolean {
  return (
    code === 32 || // space
    code === 45 || // -
    code === 95 || // _
    code === 47 || // /
    code === 46 || // .
    code === 58 || // :
    code === 9     // tab
  );
}

/** Convenience: best score across multiple haystacks for the same
 *  candidate (e.g. label + detail). Returns null only when every
 *  haystack fails. */
export function fuzzyScoreMulti(needle: string, haystacks: readonly string[]): number | null {
  let best: number | null = null;
  for (const h of haystacks) {
    const s = fuzzyScore(needle, h);
    if (s === null) continue;
    if (best === null || s > best) best = s;
  }
  return best;
}
