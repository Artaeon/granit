// Package textutil holds small string helpers shared across granit
// subsystems. The motivating case was UTF-8-safe truncation: the
// codebase had ~10 sites that capped a string with s[:N] + "…",
// which lands mid-codepoint on any multibyte character and corrupts
// the trailing rune. The codebase carries German / Hebrew / Greek /
// CJK content (notes, bible verses, scripture-tagged words, AI
// excerpts, push notification bodies), so the bug was reachable on
// real data even though it never tripped a test.
//
// Keep this package strictly tiny — string-level only, no I/O, no
// reflection. The reason for a package rather than per-call helpers
// is the truncation rule is one definition, and we want every site
// rounding the same way.
package textutil

// TruncateRunes caps a string by codepoint count rather than byte
// length, appending an ellipsis ("…") only when truncation actually
// happened. A maxRunes <= 0 returns "" — callers that want the
// original on a zero cap should branch upstream.
//
// O(n) over the input string (single range), which is fine for the
// 100-1000 char excerpts the callers use this for; large bodies
// would be better served by a streamed truncator.
func TruncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == maxRunes {
			return s[:i] + "…"
		}
		count++
	}
	return s
}
