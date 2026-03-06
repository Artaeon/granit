package tui

import (
	"strings"
)

// AutoLinker detects potential wikilink targets in text and suggests them.
// It maintains a set of known note names and checks editor content for
// unlinked references.
type AutoLinker struct {
	noteNames []string // lowercase note names (without .md)
	notePaths []string // original paths
}

// NewAutoLinker creates an AutoLinker from the vault's note paths.
func NewAutoLinker() *AutoLinker {
	return &AutoLinker{}
}

// SetNotes updates the known note list.
func (al *AutoLinker) SetNotes(paths []string) {
	al.notePaths = paths
	al.noteNames = make([]string, len(paths))
	for i, p := range paths {
		name := p
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		name = strings.TrimSuffix(name, ".md")
		al.noteNames[i] = strings.ToLower(name)
	}
}

// LinkSuggestion represents an unlinked mention that could be a wikilink.
type LinkSuggestion struct {
	NotePath string // the note it should link to
	NoteName string // display name
	Line     int    // line number in editor
	Col      int    // column position
	Length   int    // length of the matched text
}

// FindUnlinkedMentions scans content for note names that appear in the text
// but are not already wrapped in [[...]].
func (al *AutoLinker) FindUnlinkedMentions(content string, currentNote string) []LinkSuggestion {
	if len(al.noteNames) == 0 {
		return nil
	}

	lines := strings.Split(content, "\n")
	var suggestions []LinkSuggestion
	seen := make(map[string]bool)

	currentBase := strings.TrimSuffix(currentNote, ".md")
	if idx := strings.LastIndex(currentBase, "/"); idx >= 0 {
		currentBase = currentBase[idx+1:]
	}
	currentBaseLower := strings.ToLower(currentBase)

	for lineNum, line := range lines {
		lineLower := strings.ToLower(line)

		for i, nameLower := range al.noteNames {
			// Skip self-references and very short names (< 3 chars)
			if len(nameLower) < 3 || nameLower == currentBaseLower {
				continue
			}
			// Skip if already suggested this note
			if seen[nameLower] {
				continue
			}

			// Find the name in this line
			searchFrom := 0
			for {
				idx := strings.Index(lineLower[searchFrom:], nameLower)
				if idx < 0 {
					break
				}
				col := searchFrom + idx

				// Check it's a whole word (not part of a larger word)
				if !isWordBoundary(lineLower, col, len(nameLower)) {
					searchFrom = col + 1
					continue
				}

				// Check it's not already inside [[...]]
				if isInsideWikilink(line, col) {
					searchFrom = col + 1
					continue
				}

				// Check it's not in a code block or frontmatter
				if isInCodeSpan(line, col) {
					searchFrom = col + 1
					continue
				}

				// Extract the original case version
				originalName := al.notePaths[i]
				baseName := originalName
				if idx := strings.LastIndex(baseName, "/"); idx >= 0 {
					baseName = baseName[idx+1:]
				}
				baseName = strings.TrimSuffix(baseName, ".md")

				suggestions = append(suggestions, LinkSuggestion{
					NotePath: originalName,
					NoteName: baseName,
					Line:     lineNum,
					Col:      col,
					Length:   len(nameLower),
				})
				seen[nameLower] = true
				break
			}
		}
	}

	return suggestions
}

// ApplyLink wraps the text at the given position with [[ ]].
func ApplyLink(content []string, line, col, length int) {
	if line >= len(content) {
		return
	}
	l := content[line]
	if col+length > len(l) {
		return
	}
	content[line] = l[:col] + "[[" + l[col:col+length] + "]]" + l[col+length:]
}

// isWordBoundary checks if the match at position pos with given length
// is surrounded by non-alphanumeric characters.
func isWordBoundary(line string, pos, length int) bool {
	if pos > 0 {
		ch := line[pos-1]
		if isAlphaNum(ch) {
			return false
		}
	}
	end := pos + length
	if end < len(line) {
		ch := line[end]
		if isAlphaNum(ch) {
			return false
		}
	}
	return true
}

func isAlphaNum(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-'
}

// isInsideWikilink checks if position col is inside a [[...]] wikilink.
func isInsideWikilink(line string, col int) bool {
	// Look backwards for [[ without ]]
	before := line[:col]
	lastOpen := strings.LastIndex(before, "[[")
	if lastOpen < 0 {
		return false
	}
	lastClose := strings.LastIndex(before, "]]")
	return lastOpen > lastClose
}

// isInCodeSpan checks if position col is inside a `...` code span.
func isInCodeSpan(line string, col int) bool {
	inCode := false
	for i := 0; i < col && i < len(line); i++ {
		if line[i] == '`' {
			inCode = !inCode
		}
	}
	return inCode
}
