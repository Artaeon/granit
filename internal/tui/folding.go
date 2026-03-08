package tui

import "strings"

// FoldState tracks which sections of a document are folded (collapsed).
// Folding is keyed by the line number of the heading or code fence that
// begins the foldable block.
type FoldState struct {
	// folds maps a heading/fence line number to the (inclusive) end line
	// of the region it hides.  If a line is present in the map, the block
	// starting on the *next* line through folds[line] is hidden.
	folds map[int]int
}

// NewFoldState returns an empty FoldState.
func NewFoldState() FoldState {
	return FoldState{folds: make(map[int]int)}
}

// ---------- public API ----------

// ToggleFold toggles the fold starting at the given line.
// The line must be a heading (# through ######) or a code fence (```).
// content is the full document as a slice of lines.
func (fs *FoldState) ToggleFold(line int, content []string) {
	if line < 0 || line >= len(content) {
		return
	}

	// Already folded? Unfold.
	if _, ok := fs.folds[line]; ok {
		delete(fs.folds, line)
		return
	}

	trimmed := strings.TrimSpace(content[line])

	// Try heading fold
	if lvl := headingLevel(trimmed); lvl > 0 {
		end := headingFoldEnd(content, line, lvl)
		if end > line {
			fs.folds[line] = end
		}
		return
	}

	// Try code fence fold
	if strings.HasPrefix(trimmed, "```") {
		end := codeFenceFoldEnd(content, line)
		if end > line {
			fs.folds[line] = end
		}
		return
	}
}

// IsFolded returns true if the given line is hidden because it falls
// inside a folded region.  The fold-trigger line itself is *not*
// considered folded (it remains visible so the user can toggle it).
func (fs *FoldState) IsFolded(line int) bool {
	for start, end := range fs.folds {
		if line > start && line <= end {
			return true
		}
	}
	return false
}

// FoldAll collapses every heading and code fence in the document.
func (fs *FoldState) FoldAll(content []string) {
	fs.folds = make(map[int]int)
	skip := -1
	for i := 0; i < len(content); i++ {
		if i <= skip {
			continue
		}
		trimmed := strings.TrimSpace(content[i])
		if lvl := headingLevel(trimmed); lvl > 0 {
			end := headingFoldEnd(content, i, lvl)
			if end > i {
				fs.folds[i] = end
				skip = end
			}
		} else if strings.HasPrefix(trimmed, "```") {
			end := codeFenceFoldEnd(content, i)
			if end > i {
				fs.folds[i] = end
				skip = end
			}
		}
	}
}

// UnfoldAll expands every folded section.
func (fs *FoldState) UnfoldAll() {
	fs.folds = make(map[int]int)
}

// GetFoldEnd returns the end line of a fold starting at the given line.
func (fs *FoldState) GetFoldEnd(line int) (int, bool) {
	end, ok := fs.folds[line]
	return end, ok
}

// GetFoldIndicator returns a gutter indicator for the given line:
//   - "▶" when the line is a folded heading/fence (collapsed)
//   - "▼" when the line is a heading/fence with content below (expanded)
//   - ""  for all other lines
func (fs *FoldState) GetFoldIndicator(line int, content []string) string {
	if line < 0 || line >= len(content) {
		return ""
	}

	trimmed := strings.TrimSpace(content[line])

	isHeading := headingLevel(trimmed) > 0
	isFence := strings.HasPrefix(trimmed, "```")

	if !isHeading && !isFence {
		return ""
	}

	if _, ok := fs.folds[line]; ok {
		return "▶"
	}

	// Expanded heading/fence that has content underneath
	if isHeading {
		lvl := headingLevel(trimmed)
		end := headingFoldEnd(content, line, lvl)
		if end > line {
			return "▼"
		}
	}
	if isFence {
		end := codeFenceFoldEnd(content, line)
		if end > line {
			return "▼"
		}
	}

	return ""
}

// ---------- helpers ----------

// headingLevel returns the heading level (1-6) for a markdown heading
// line, or 0 if the line is not a heading.
func headingLevel(trimmed string) int {
	level := 0
	for _, ch := range trimmed {
		if ch == '#' {
			level++
		} else {
			break
		}
	}
	if level < 1 || level > 6 {
		return 0
	}
	// Must be followed by a space (e.g. "## Title")
	if level >= len(trimmed) || trimmed[level] != ' ' {
		return 0
	}
	return level
}

// headingFoldEnd returns the (inclusive) last line that should be hidden
// when folding the heading at startLine.  The range is everything until
// the next heading of equal or higher level (fewer #), or the end of the
// document.
func headingFoldEnd(content []string, startLine, level int) int {
	end := len(content) - 1
	for i := startLine + 1; i < len(content); i++ {
		trimmed := strings.TrimSpace(content[i])
		otherLevel := headingLevel(trimmed)
		if otherLevel > 0 && otherLevel <= level {
			end = i - 1
			break
		}
	}
	if end <= startLine {
		return startLine // nothing to fold
	}
	return end
}

// codeFenceFoldEnd returns the (inclusive) last line that should be
// hidden when folding the code fence starting at startLine (the opening
// ```).  The range covers everything up to and including the closing ```.
func codeFenceFoldEnd(content []string, startLine int) int {
	for i := startLine + 1; i < len(content); i++ {
		if strings.HasPrefix(strings.TrimSpace(content[i]), "```") {
			return i
		}
	}
	return startLine // no closing fence found — nothing to fold
}
