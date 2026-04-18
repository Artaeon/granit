package tui

// Search-query parser shared by ContentSearch and (eventually) other
// search surfaces. Translates a raw query string like
//
//	tag:work path:projects/ -draft "shipped on Tuesday" review
//
// into structured filters that callers can apply to candidate files.
// Designed to be called once per query, before the actual scan, so
// neither the index path nor the linear scan need to re-parse.

import (
	"strings"
)

// SearchQuery captures the parsed view of a user query.
//
//   - Terms: free-form keywords (treated as substrings or fuzzy hits)
//   - Phrases: exact substrings extracted from "double-quoted" runs
//   - Tags: must-have hashtags (#foo, written as tag:foo in the query)
//   - Paths: substring filters on the file's vault-relative path
//   - Excludes: terms that must NOT appear in the matching line/file
//
// All slices are non-nil when filters are present and nil otherwise so
// `len(q.Tags) > 0` is the natural "any tag filter?" check.
type SearchQuery struct {
	Raw      string
	Terms    []string
	Phrases  []string
	Tags     []string
	Paths    []string
	Excludes []string
}

// ParseSearchQuery splits raw into filters. Recognises:
//
//	tag:NAME      — file must contain "#NAME" (case-insensitive)
//	path:SUBSTR   — file's relative path must contain SUBSTR (case-insensitive)
//	-WORD         — line/file must NOT contain WORD
//	"PHRASE"      — exact substring (preserves spaces)
//	WORD          — plain term (any other token)
//
// Quotes that don't pair fall back to plain terms; no error is reported
// because a half-typed query is normal mid-typing.
func ParseSearchQuery(raw string) SearchQuery {
	q := SearchQuery{Raw: raw}
	tokens := tokenizeSearchQuery(raw)
	for _, tok := range tokens {
		switch {
		case strings.HasPrefix(tok, `"`) && strings.HasSuffix(tok, `"`) && len(tok) >= 2:
			body := tok[1 : len(tok)-1]
			if body != "" {
				q.Phrases = append(q.Phrases, body)
			}
		case strings.HasPrefix(tok, "tag:"):
			if v := strings.TrimPrefix(tok, "tag:"); v != "" {
				q.Tags = append(q.Tags, strings.TrimPrefix(v, "#"))
			}
		case strings.HasPrefix(tok, "path:"):
			if v := strings.TrimPrefix(tok, "path:"); v != "" {
				q.Paths = append(q.Paths, v)
			}
		case strings.HasPrefix(tok, "-") && len(tok) > 1:
			q.Excludes = append(q.Excludes, tok[1:])
		default:
			q.Terms = append(q.Terms, tok)
		}
	}
	return q
}

// tokenizeSearchQuery splits raw on whitespace, respecting double-quoted
// runs as single tokens (preserving the surrounding quotes so the caller
// can distinguish "phrases" from terms).
func tokenizeSearchQuery(raw string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for _, r := range raw {
		switch {
		case r == '"':
			current.WriteRune(r)
			if inQuote {
				flush()
			}
			inQuote = !inQuote
		case (r == ' ' || r == '\t') && !inQuote:
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return tokens
}

// HasOperators reports whether the query uses any structured filter.
// Useful for fast-pathing simple queries through the existing scan.
func (q SearchQuery) HasOperators() bool {
	return len(q.Phrases) > 0 || len(q.Tags) > 0 || len(q.Paths) > 0 || len(q.Excludes) > 0
}

// PlainQuery rebuilds the unfiltered substring intent — used to feed the
// existing inverted-index lookup, which doesn't itself understand
// operators. Tag/path filters are applied as a post-step.
func (q SearchQuery) PlainQuery() string {
	if len(q.Terms) == 0 && len(q.Phrases) == 0 {
		return ""
	}
	parts := make([]string, 0, len(q.Terms)+len(q.Phrases))
	parts = append(parts, q.Terms...)
	parts = append(parts, q.Phrases...)
	return strings.Join(parts, " ")
}

// MatchesPath reports whether path satisfies any path: filter (case-
// insensitive substring). Returns true when no path filter is set.
func (q SearchQuery) MatchesPath(path string) bool {
	if len(q.Paths) == 0 {
		return true
	}
	lo := strings.ToLower(path)
	for _, p := range q.Paths {
		if strings.Contains(lo, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// MatchesTags reports whether content satisfies every tag: filter. All
// tags must be present (AND), matching the user's likely intent — multi-
// tag is "give me notes with both work and review," not "either."
func (q SearchQuery) MatchesTags(content string) bool {
	if len(q.Tags) == 0 {
		return true
	}
	lo := strings.ToLower(content)
	for _, t := range q.Tags {
		needle := "#" + strings.ToLower(t)
		if !strings.Contains(lo, needle) {
			return false
		}
	}
	return true
}

// MatchesExcludes reports whether content avoids every excluded term.
// One excluded hit is enough to reject the candidate.
func (q SearchQuery) MatchesExcludes(content string) bool {
	if len(q.Excludes) == 0 {
		return true
	}
	lo := strings.ToLower(content)
	for _, e := range q.Excludes {
		if strings.Contains(lo, strings.ToLower(e)) {
			return false
		}
	}
	return true
}

// MatchesPhrases reports whether content contains every quoted phrase
// (case-insensitive substring; AND semantics).
func (q SearchQuery) MatchesPhrases(content string) bool {
	if len(q.Phrases) == 0 {
		return true
	}
	lo := strings.ToLower(content)
	for _, p := range q.Phrases {
		if !strings.Contains(lo, strings.ToLower(p)) {
			return false
		}
	}
	return true
}
