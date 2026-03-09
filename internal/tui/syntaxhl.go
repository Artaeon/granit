package tui

import "strings"

// highlightCodeLine returns a syntax-highlighted rendering of a single code line.
// lang is the language identifier from the opening fence (e.g. "go", "python").
// If lang is empty or unknown, a generic fallback is applied.
// The actual highlighting is performed by chroma via highlight.go.
func highlightCodeLine(line string, lang string) string {
	return HighlightCodeLine(lang, line)
}

// parseFenceLang extracts the language from a fenced code block opening line.
// E.g. "```go" returns "go", "```python" returns "python", "```" returns "".
func parseFenceLang(line string) string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "```") {
		return ""
	}
	lang := strings.TrimPrefix(trimmed, "```")
	lang = strings.TrimSpace(lang)
	// Remove any trailing text after whitespace (e.g. ```python title="example")
	if idx := strings.IndexByte(lang, ' '); idx >= 0 {
		lang = lang[:idx]
	}
	return strings.ToLower(lang)
}
