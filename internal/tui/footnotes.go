package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// FootnoteRef represents a parsed footnote — either a reference in the text
// ([^id]) or its definition ([^id]: content), or both when matched.
type FootnoteRef struct {
	ID      string // footnote identifier (e.g. "1", "note")
	RefLine int    // line where the reference [^id] appears (-1 if definition-only)
	DefLine int    // line where the definition [^id]: ... appears (-1 if reference-only)
	Content string // definition text (empty if no definition found)
}

// Regex patterns for footnote references and definitions.
var (
	footnoteRefRe = regexp.MustCompile(`\[\^([^\]]+)\]`)
	footnoteDefRe = regexp.MustCompile(`^\[\^([^\]]+)\]:\s*(.*)$`)
)

// ParseFootnotes scans the document and returns all footnotes found,
// matching references to their definitions where possible.
func ParseFootnotes(content []string) []FootnoteRef {
	refs := make(map[string]*FootnoteRef)
	var order []string

	// First pass: find definitions ([^id]: content)
	for i, line := range content {
		trimmed := strings.TrimSpace(line)
		m := footnoteDefRe.FindStringSubmatch(trimmed)
		if m != nil {
			id := m[1]
			if _, exists := refs[id]; !exists {
				refs[id] = &FootnoteRef{
					ID:      id,
					RefLine: -1,
					DefLine: i,
					Content: m[2],
				}
				order = append(order, id)
			} else {
				refs[id].DefLine = i
				refs[id].Content = m[2]
			}
		}
	}

	// Second pass: find references ([^id]) that are NOT at the start of
	// a definition line
	for i, line := range content {
		trimmed := strings.TrimSpace(line)
		// Skip definition lines themselves
		if footnoteDefRe.MatchString(trimmed) {
			continue
		}
		matches := footnoteRefRe.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			id := m[1]
			if _, exists := refs[id]; !exists {
				refs[id] = &FootnoteRef{
					ID:      id,
					RefLine: i,
					DefLine: -1,
				}
				order = append(order, id)
			} else if refs[id].RefLine == -1 {
				refs[id].RefLine = i
			}
		}
	}

	result := make([]FootnoteRef, 0, len(order))
	for _, id := range order {
		result = append(result, *refs[id])
	}
	return result
}

// FindFootnoteDefinition locates the line and text of a footnote definition
// by its id.  Returns (-1, "") if not found.
func FindFootnoteDefinition(content []string, id string) (line int, text string) {
	target := "[^" + id + "]:"
	for i, l := range content {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, target) {
			rest := strings.TrimSpace(trimmed[len(target):])
			return i, rest
		}
	}
	return -1, ""
}

// FindFootnoteReference locates the first occurrence of a footnote
// reference [^id] in the document.  Returns (-1, -1) if not found.
func FindFootnoteReference(content []string, id string) (line int, col int) {
	needle := "[^" + id + "]"
	for i, l := range content {
		// Skip definition lines
		trimmed := strings.TrimSpace(l)
		if footnoteDefRe.MatchString(trimmed) {
			continue
		}
		idx := strings.Index(l, needle)
		if idx >= 0 {
			return i, idx
		}
	}
	return -1, -1
}

// RenderFootnoteMarker returns a styled superscript-like rendering for a
// footnote marker.  Uses the peach colour from the theme.
func RenderFootnoteMarker(id string) string {
	style := lipgloss.NewStyle().
		Foreground(peach).
		Bold(true)
	return style.Render("[^" + id + "]")
}

// HighlightFootnoteRefs processes a line and replaces footnote references
// ([^id]) with styled markers.  This is intended to be called from
// highlightInline in editor.go.
func HighlightFootnoteRefs(line string) string {
	if !strings.Contains(line, "[^") {
		return line
	}

	indices := footnoteRefRe.FindAllStringIndex(line, -1)
	if len(indices) == 0 {
		return line
	}

	var b strings.Builder
	prev := 0
	for _, loc := range indices {
		b.WriteString(line[prev:loc[0]])
		matched := line[loc[0]:loc[1]]
		// Extract id from [^id]
		sub := footnoteRefRe.FindStringSubmatch(matched)
		if sub != nil {
			b.WriteString(RenderFootnoteMarker(sub[1]))
		} else {
			b.WriteString(matched)
		}
		prev = loc[1]
	}
	b.WriteString(line[prev:])
	return b.String()
}
