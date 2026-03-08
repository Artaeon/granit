package tui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CursorPos represents a cursor position in the editor.
type CursorPos struct {
	Line int
	Col  int
}

type editorSnapshot struct {
	content []string
	cursor  int
	col     int
}

type Editor struct {
	content      []string
	cursor       int
	col          int
	scroll       int
	focused      bool
	height       int
	width        int
	filePath     string
	modified     bool
	wordCount    int
	undoStack    []editorSnapshot
	redoStack    []editorSnapshot
	lastSnapshot time.Time

	// Multi-cursor support
	cursors   []CursorPos // additional cursors (the main cursor is cursor/col)
	multiWord string      // the word selected for Ctrl+D matching

	// Config-driven
	showLineNumbers      bool
	highlightCurrentLine bool
	autoCloseBrackets    bool
	tabSize              int

	// Horizontal scroll for long lines
	hscroll int

	// Ghost text (AI inline completion)
	ghostText string // dimmed suggestion shown after cursor

	// Heading/code-fence folding
	foldState *FoldState

	// Vim search highlights
	searchMatches      []SearchMatch
	currentSearchMatch int
}

func NewEditor() Editor {
	return Editor{
		content: []string{""},
	}
}

func (e *Editor) SetSize(width, height int) {
	e.width = width
	e.height = height
}

func (e *Editor) LoadContent(content string, filePath string) {
	e.content = strings.Split(content, "\n")
	if len(e.content) == 0 {
		e.content = []string{""}
	}
	e.filePath = filePath
	e.cursor = 0
	e.col = 0
	e.scroll = 0
	e.hscroll = 0
	e.modified = false
	e.countWords()
}

func (e *Editor) GetContent() string {
	return strings.Join(e.content, "\n")
}

func (e *Editor) IsModified() bool {
	return e.modified
}

func (e *Editor) GetCursor() (int, int) {
	return e.cursor, e.col
}

func (e *Editor) GetWordCount() int {
	return e.wordCount
}

// SetFoldState sets the fold state used for heading/code-fence folding.
func (e *Editor) SetFoldState(fs *FoldState) {
	e.foldState = fs
}

// isLineFolded returns true if the given line is hidden inside a folded region.
func (e *Editor) isLineFolded(line int) bool {
	if e.foldState == nil {
		return false
	}
	return e.foldState.IsFolded(line)
}

// SetGhostText sets the dimmed AI suggestion text to show after the cursor.
func (e *Editor) SetGhostText(text string) {
	e.ghostText = text
}

// GetGhostText returns the current ghost text suggestion.
func (e *Editor) GetGhostText() string {
	return e.ghostText
}

// SetSearchHighlights sets the search match positions for rendering.
func (e *Editor) SetSearchHighlights(matches []SearchMatch, currentIdx int) {
	e.searchMatches = matches
	e.currentSearchMatch = currentIdx
}

// ClearSearchHighlights removes all search highlights.
func (e *Editor) ClearSearchHighlights() {
	e.searchMatches = nil
	e.currentSearchMatch = -1
}

func (e *Editor) SetContent(content string) {
	e.saveSnapshot()
	e.content = strings.Split(content, "\n")
	if len(e.content) == 0 {
		e.content = []string{""}
	}
	e.modified = true
	e.countWords()
}

func (e *Editor) InsertText(text string) {
	e.saveSnapshot()
	for _, ch := range text {
		if ch == '\n' {
			// Split current line
			line := e.content[e.cursor]
			before := line[:e.col]
			after := line[e.col:]
			e.content[e.cursor] = before
			newLines := append(e.content[:e.cursor+1], append([]string{after}, e.content[e.cursor+1:]...)...)
			e.content = newLines
			e.cursor++
			e.col = 0
		} else {
			line := e.content[e.cursor]
			if e.col > len(line) {
				e.col = len(line)
			}
			e.content[e.cursor] = line[:e.col] + string(ch) + line[e.col:]
			e.col++
		}
	}
	e.modified = true
	e.countWords()
}

func (e *Editor) countWords() {
	total := 0
	for _, line := range e.content {
		words := strings.Fields(line)
		total += len(words)
	}
	e.wordCount = total
}

func (e *Editor) saveSnapshot() {
	now := time.Now()
	if now.Sub(e.lastSnapshot) < 500*time.Millisecond {
		return
	}
	contentCopy := make([]string, len(e.content))
	copy(contentCopy, e.content)
	snap := editorSnapshot{
		content: contentCopy,
		cursor:  e.cursor,
		col:     e.col,
	}
	e.undoStack = append(e.undoStack, snap)
	if len(e.undoStack) > 100 {
		e.undoStack = e.undoStack[len(e.undoStack)-100:]
	}
	e.lastSnapshot = now
}

func (e *Editor) Undo() {
	if len(e.undoStack) == 0 {
		return
	}
	// Push current state to redo stack
	contentCopy := make([]string, len(e.content))
	copy(contentCopy, e.content)
	current := editorSnapshot{
		content: contentCopy,
		cursor:  e.cursor,
		col:     e.col,
	}
	e.redoStack = append(e.redoStack, current)
	if len(e.redoStack) > 100 {
		e.redoStack = e.redoStack[len(e.redoStack)-100:]
	}

	// Pop from undo stack and restore
	snap := e.undoStack[len(e.undoStack)-1]
	e.undoStack = e.undoStack[:len(e.undoStack)-1]
	e.content = snap.content
	e.cursor = snap.cursor
	e.col = snap.col
	e.modified = true
	e.countWords()
}

func (e *Editor) Redo() {
	if len(e.redoStack) == 0 {
		return
	}
	// Push current state to undo stack
	contentCopy := make([]string, len(e.content))
	copy(contentCopy, e.content)
	current := editorSnapshot{
		content: contentCopy,
		cursor:  e.cursor,
		col:     e.col,
	}
	e.undoStack = append(e.undoStack, current)
	if len(e.undoStack) > 100 {
		e.undoStack = e.undoStack[len(e.undoStack)-100:]
	}

	// Pop from redo stack and restore
	snap := e.redoStack[len(e.redoStack)-1]
	e.redoStack = e.redoStack[:len(e.redoStack)-1]
	e.content = snap.content
	e.cursor = snap.cursor
	e.col = snap.col
	e.modified = true
	e.countWords()
}

// getAllCursors returns the main cursor combined with all additional cursors.
func (e *Editor) getAllCursors() []CursorPos {
	all := []CursorPos{{Line: e.cursor, Col: e.col}}
	all = append(all, e.cursors...)
	return all
}

// sortCursorsBottomUp returns cursors sorted from bottom-to-top (highest line
// first, then highest col first within the same line) so that edits applied in
// order don't shift earlier positions.
func sortCursorsBottomUp(cursors []CursorPos) []CursorPos {
	sorted := make([]CursorPos, len(cursors))
	copy(sorted, cursors)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Line != sorted[j].Line {
			return sorted[i].Line > sorted[j].Line
		}
		return sorted[i].Col > sorted[j].Col
	})
	return sorted
}

// clearMultiCursors removes all additional cursors and clears the multi-word.
func (e *Editor) clearMultiCursors() {
	e.cursors = nil
	e.multiWord = ""
}

// HasMultiCursors reports whether the editor has additional cursors active.
func (e *Editor) HasMultiCursors() bool {
	return len(e.cursors) > 0
}

// wordUnderCursor returns the word under the main cursor position and its
// start column index. A word is a contiguous run of letters, digits, or
// underscores.
func (e *Editor) wordUnderCursor() (string, int) {
	if e.cursor >= len(e.content) {
		return "", 0
	}
	line := e.content[e.cursor]
	if len(line) == 0 || e.col > len(line) {
		return "", 0
	}
	col := e.col
	if col >= len(line) {
		col = len(line) - 1
	}
	if col < 0 {
		return "", 0
	}
	r := rune(line[col])
	if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
		// Try one position to the left (cursor may be just past the word)
		if col > 0 {
			col--
			r = rune(line[col])
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
				return "", 0
			}
		} else {
			return "", 0
		}
	}
	start := col
	for start > 0 {
		r := rune(line[start-1])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		start--
	}
	end := col
	for end < len(line)-1 {
		r := rune(line[end+1])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		end++
	}
	return line[start : end+1], start
}

// isMultiCursorAt checks if any additional cursor is at the given position.
func (e *Editor) isMultiCursorAt(line, col int) bool {
	for _, c := range e.cursors {
		if c.Line == line && c.Col == col {
			return true
		}
	}
	return false
}

func (e Editor) Update(msg tea.Msg) (Editor, tea.Cmd) {
	if !e.focused {
		return e, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if len(e.cursors) > 0 {
				// Move all cursors up by 1
				if e.cursor-1 < 0 {
					break
				}
				e.cursor--
				if e.col > len(e.content[e.cursor]) {
					e.col = len(e.content[e.cursor])
				}
				for i := range e.cursors {
					e.cursors[i].Line--
					if e.cursors[i].Line < 0 {
						e.cursors[i].Line = 0
					}
					if e.cursors[i].Col > len(e.content[e.cursors[i].Line]) {
						e.cursors[i].Col = len(e.content[e.cursors[i].Line])
					}
				}
				if e.cursor < e.scroll {
					e.scroll = e.cursor
				}
			} else {
				if e.cursor > 0 {
					e.cursor--
					// Skip folded lines
					for e.cursor > 0 && e.isLineFolded(e.cursor) {
						e.cursor--
					}
					if e.col > len(e.content[e.cursor]) {
						e.col = len(e.content[e.cursor])
					}
					if e.cursor < e.scroll {
						e.scroll = e.cursor
					}
				}
			}
		case "down":
			if len(e.cursors) > 0 {
				maxLine := len(e.content) - 1
				if e.cursor+1 > maxLine {
					break
				}
				canMove := true
				for _, c := range e.cursors {
					if c.Line+1 > maxLine {
						canMove = false
						break
					}
				}
				if !canMove {
					break
				}
				e.cursor++
				if e.col > len(e.content[e.cursor]) {
					e.col = len(e.content[e.cursor])
				}
				for i := range e.cursors {
					e.cursors[i].Line++
					if e.cursors[i].Col > len(e.content[e.cursors[i].Line]) {
						e.cursors[i].Col = len(e.content[e.cursors[i].Line])
					}
				}
				visibleHeight := e.height - 4
				if e.cursor >= e.scroll+visibleHeight {
					e.scroll = e.cursor - visibleHeight + 1
				}
			} else {
				if e.cursor < len(e.content)-1 {
					e.cursor++
					// Skip folded lines
					for e.cursor < len(e.content)-1 && e.isLineFolded(e.cursor) {
						e.cursor++
					}
					if e.col > len(e.content[e.cursor]) {
						e.col = len(e.content[e.cursor])
					}
					visibleHeight := e.height - 4
					if e.cursor >= e.scroll+visibleHeight {
						e.scroll = e.cursor - visibleHeight + 1
					}
				}
			}
		case "left":
			if len(e.cursors) > 0 {
				if e.col > 0 {
					e.col--
				}
				for i := range e.cursors {
					if e.cursors[i].Col > 0 {
						e.cursors[i].Col--
					}
				}
			} else {
				if e.col > 0 {
					e.col--
				} else if e.cursor > 0 {
					e.cursor--
					e.col = len(e.content[e.cursor])
				}
			}
		case "right":
			if len(e.cursors) > 0 {
				if e.col < len(e.content[e.cursor]) {
					e.col++
				}
				for i := range e.cursors {
					if e.cursors[i].Col < len(e.content[e.cursors[i].Line]) {
						e.cursors[i].Col++
					}
				}
			} else {
				if e.col < len(e.content[e.cursor]) {
					e.col++
				} else if e.cursor < len(e.content)-1 {
					e.cursor++
					e.col = 0
				}
			}
		case "home", "ctrl+a":
			e.col = 0
			for i := range e.cursors {
				e.cursors[i].Col = 0
			}
		case "end":
			e.col = len(e.content[e.cursor])
			for i := range e.cursors {
				e.cursors[i].Col = len(e.content[e.cursors[i].Line])
			}
		case "pgup":
			e.clearMultiCursors()
			visH := e.height - 4
			if visH < 1 {
				visH = 1
			}
			e.cursor -= visH
			if e.cursor < 0 {
				e.cursor = 0
			}
			e.scroll -= visH
			if e.scroll < 0 {
				e.scroll = 0
			}
			if e.col > len(e.content[e.cursor]) {
				e.col = len(e.content[e.cursor])
			}
		case "pgdown":
			e.clearMultiCursors()
			visH := e.height - 4
			if visH < 1 {
				visH = 1
			}
			e.cursor += visH
			if e.cursor >= len(e.content) {
				e.cursor = len(e.content) - 1
			}
			e.scroll += visH
			maxScroll := len(e.content) - visH
			if maxScroll < 0 {
				maxScroll = 0
			}
			if e.scroll > maxScroll {
				e.scroll = maxScroll
			}
			if e.col > len(e.content[e.cursor]) {
				e.col = len(e.content[e.cursor])
			}
		case "ctrl+u":
			e.clearMultiCursors()
			e.Undo()
			return e, nil
		case "ctrl+y":
			e.clearMultiCursors()
			e.Redo()
			return e, nil

		case "ctrl+d":
			// Multi-cursor: select word under cursor / add next occurrence
			word, _ := e.wordUnderCursor()
			if word == "" {
				break
			}
			if e.multiWord == "" {
				// First press: identify the word
				e.multiWord = word
			} else {
				// Subsequent presses: find next occurrence and add cursor there
				e.findAndAddNextOccurrence()
			}
			return e, nil

		case "ctrl+shift+up":
			// Add cursor on line above
			if e.cursor > 0 {
				newLine := e.cursor - 1
				newCol := e.col
				if newCol > len(e.content[newLine]) {
					newCol = len(e.content[newLine])
				}
				alreadyExists := false
				for _, c := range e.cursors {
					if c.Line == newLine && c.Col == newCol {
						alreadyExists = true
						break
					}
				}
				if !alreadyExists {
					e.cursors = append(e.cursors, CursorPos{Line: newLine, Col: newCol})
				}
			}
			return e, nil

		case "ctrl+shift+down":
			// Add cursor on line below
			if e.cursor < len(e.content)-1 {
				newLine := e.cursor + 1
				newCol := e.col
				if newCol > len(e.content[newLine]) {
					newCol = len(e.content[newLine])
				}
				alreadyExists := false
				for _, c := range e.cursors {
					if c.Line == newLine && c.Col == newCol {
						alreadyExists = true
						break
					}
				}
				if !alreadyExists {
					e.cursors = append(e.cursors, CursorPos{Line: newLine, Col: newCol})
				}
			}
			return e, nil

		case "enter":
			e.saveSnapshot()
			e.redoStack = nil
			if len(e.cursors) == 0 && e.isInTable() {
				e.enterInTable()
			} else if len(e.cursors) > 0 {
				// Multi-cursor enter: process from bottom to top
				allCursors := sortCursorsBottomUp(e.getAllCursors())
				for _, c := range allCursors {
					if c.Line >= len(e.content) {
						continue
					}
					line := e.content[c.Line]
					col := c.Col
					if col > len(line) {
						col = len(line)
					}
					before := line[:col]
					after := line[col:]
					e.content[c.Line] = before
					newContent := make([]string, 0, len(e.content)+1)
					newContent = append(newContent, e.content[:c.Line+1]...)
					newContent = append(newContent, after)
					newContent = append(newContent, e.content[c.Line+1:]...)
					e.content = newContent
				}
				e.cursor++
				e.col = 0
				e.clearMultiCursors()
				e.modified = true
				e.countWords()
			} else {
				line := e.content[e.cursor]
				before := line[:e.col]
				after := line[e.col:]
				e.content[e.cursor] = before
				newContent := make([]string, 0, len(e.content)+1)
				newContent = append(newContent, e.content[:e.cursor+1]...)
				newContent = append(newContent, after)
				newContent = append(newContent, e.content[e.cursor+1:]...)
				e.content = newContent
				e.cursor++
				e.col = 0
				e.modified = true
				e.countWords()
			}
		case "backspace":
			e.saveSnapshot()
			e.redoStack = nil
			if len(e.cursors) > 0 {
				// Multi-cursor backspace: process from bottom to top
				allCursors := sortCursorsBottomUp(e.getAllCursors())
				for ci, c := range allCursors {
					if c.Line >= len(e.content) {
						continue
					}
					if c.Col > 0 {
						line := e.content[c.Line]
						col := c.Col
						if col > len(line) {
							col = len(line)
						}
						e.content[c.Line] = line[:col-1] + line[col:]
						// Adjust cursors on the same line with higher col
						for ci2 := ci + 1; ci2 < len(allCursors); ci2++ {
							if allCursors[ci2].Line == c.Line && allCursors[ci2].Col > col {
								allCursors[ci2].Col--
							}
						}
					} else if c.Line > 0 {
						prevLen := len(e.content[c.Line-1])
						e.content[c.Line-1] += e.content[c.Line]
						e.content = append(e.content[:c.Line], e.content[c.Line+1:]...)
						// Adjust subsequent cursor line numbers
						for ci2 := ci + 1; ci2 < len(allCursors); ci2++ {
							if allCursors[ci2].Line >= c.Line {
								allCursors[ci2].Line--
							}
						}
						_ = prevLen
					}
				}
				// Update main cursor position
				if e.col > 0 {
					e.col--
				}
				e.clearMultiCursors()
				e.modified = true
				e.countWords()
			} else {
				if e.col > 0 {
					line := e.content[e.cursor]
					e.content[e.cursor] = line[:e.col-1] + line[e.col:]
					e.col--
					e.modified = true
					e.countWords()
				} else if e.cursor > 0 {
					prevLen := len(e.content[e.cursor-1])
					e.content[e.cursor-1] += e.content[e.cursor]
					e.content = append(e.content[:e.cursor], e.content[e.cursor+1:]...)
					e.cursor--
					e.col = prevLen
					e.modified = true
					e.countWords()
				}
			}
		case "delete":
			e.saveSnapshot()
			e.redoStack = nil
			line := e.content[e.cursor]
			if e.col < len(line) {
				e.content[e.cursor] = line[:e.col] + line[e.col+1:]
				e.modified = true
				e.countWords()
			} else if e.cursor < len(e.content)-1 {
				e.content[e.cursor] += e.content[e.cursor+1]
				e.content = append(e.content[:e.cursor+1], e.content[e.cursor+2:]...)
				e.modified = true
				e.countWords()
			}
		case "ctrl+k":
			// Kill to end of line
			e.saveSnapshot()
			e.redoStack = nil
			line := e.content[e.cursor]
			if e.col < len(line) {
				e.content[e.cursor] = line[:e.col]
				e.modified = true
				e.countWords()
			}
		case "tab":
			e.saveSnapshot()
			e.redoStack = nil
			if len(e.cursors) == 0 && e.isInTable() {
				e.tabInTable()
			} else if len(e.cursors) > 0 {
				// Insert tab as spaces (multi-cursor)
				allCursors := sortCursorsBottomUp(e.getAllCursors())
				tabStr := strings.Repeat(" ", e.tabSize)
				for _, c := range allCursors {
					if c.Line >= len(e.content) {
						continue
					}
					line := e.content[c.Line]
					col := c.Col
					if col > len(line) {
						col = len(line)
					}
					e.content[c.Line] = line[:col] + tabStr + line[col:]
				}
				e.col += e.tabSize
				for i := range e.cursors {
					e.cursors[i].Col += e.tabSize
				}
				e.modified = true
			} else {
				// Insert tab as spaces (single cursor)
				tabStr := strings.Repeat(" ", e.tabSize)
				line := e.content[e.cursor]
				e.content[e.cursor] = line[:e.col] + tabStr + line[e.col:]
				e.col += e.tabSize
				e.modified = true
			}
		default:
			char := msg.String()
			if len(char) == 1 && char[0] >= 32 {
				e.saveSnapshot()
				e.redoStack = nil

				if len(e.cursors) > 0 {
					// Multi-cursor character insertion: process bottom to top
					allCursors := sortCursorsBottomUp(e.getAllCursors())
					for _, c := range allCursors {
						if c.Line >= len(e.content) {
							continue
						}
						line := e.content[c.Line]
						col := c.Col
						if col > len(line) {
							col = len(line)
						}
						e.content[c.Line] = line[:col] + char + line[col:]
					}
					e.col++
					for i := range e.cursors {
						e.cursors[i].Col++
					}
					e.modified = true
					e.countWords()
				} else {
					line := e.content[e.cursor]

					// Auto-close brackets
					closeChar := ""
					if e.autoCloseBrackets {
						switch char {
						case "(":
							closeChar = ")"
						case "[":
							closeChar = "]"
						case "{":
							closeChar = "}"
						case "\"":
							closeChar = "\""
						case "'":
							closeChar = "'"
						case "`":
							closeChar = "`"
						}
					}

					e.content[e.cursor] = line[:e.col] + char + closeChar + line[e.col:]
					e.col++
					e.modified = true
					e.countWords()
				}
			}
		}
	}
	return e, nil
}

// findAndAddNextOccurrence searches for the next occurrence of multiWord
// after the last cursor position and adds a new cursor there.
func (e *Editor) findAndAddNextOccurrence() {
	allCursors := e.getAllCursors()
	searchWord := e.multiWord

	// Find the last cursor (bottom-most, then right-most)
	lastCursor := allCursors[0]
	for _, c := range allCursors {
		if c.Line > lastCursor.Line || (c.Line == lastCursor.Line && c.Col > lastCursor.Col) {
			lastCursor = c
		}
	}

	// Search forward from after last cursor
	if pos, ok := e.findNextWholeWord(searchWord, lastCursor.Line, lastCursor.Col+1, allCursors); ok {
		e.cursors = append(e.cursors, pos)
		return
	}

	// Wrap around: search from beginning
	if pos, ok := e.findNextWholeWord(searchWord, 0, 0, allCursors); ok {
		// Only add if it's before the last cursor (otherwise we already searched there)
		if pos.Line < lastCursor.Line || (pos.Line == lastCursor.Line && pos.Col <= lastCursor.Col) {
			e.cursors = append(e.cursors, pos)
		}
	}
}

// findNextWholeWord finds the next whole-word occurrence of word starting from
// the given line and column. It skips positions that already have a cursor.
func (e *Editor) findNextWholeWord(word string, startLine, startCol int, existing []CursorPos) (CursorPos, bool) {
	for li := startLine; li < len(e.content); li++ {
		line := e.content[li]
		col := 0
		if li == startLine {
			col = startCol
		}
		for col <= len(line)-len(word) {
			idx := strings.Index(line[col:], word)
			if idx < 0 {
				break
			}
			checkCol := col + idx
			if e.isWholeWord(li, checkCol, len(word)) {
				alreadyExists := false
				for _, c := range existing {
					if c.Line == li && c.Col == checkCol {
						alreadyExists = true
						break
					}
				}
				if !alreadyExists {
					return CursorPos{Line: li, Col: checkCol}, true
				}
			}
			col = checkCol + 1
		}
	}
	return CursorPos{}, false
}

// isWholeWord checks if the substring at line[col:col+length] is a whole word.
func (e *Editor) isWholeWord(lineIdx, col, length int) bool {
	if lineIdx >= len(e.content) {
		return false
	}
	line := e.content[lineIdx]
	if col+length > len(line) {
		return false
	}
	if col > 0 {
		r := rune(line[col-1])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return false
		}
	}
	if col+length < len(line) {
		r := rune(line[col+length])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return false
		}
	}
	return true
}

// hasAnyCursorOnLine checks if any additional cursor is on the given line.
func (e *Editor) hasAnyCursorOnLine(lineIdx int) bool {
	for _, c := range e.cursors {
		if c.Line == lineIdx {
			return true
		}
	}
	return false
}

// renderLineWithCursors renders a line with multiple cursor positions highlighted.
// The main cursor uses CursorStyle, additional cursors use the provided multiCursorStyle.
func (e *Editor) renderLineWithCursors(displayLine string, lineIdx, fmStart, fmEnd int, codeBlocks map[int]string, multiCursorStyle lipgloss.Style, maxWidth int) string {
	// Collect cursor columns on this line
	type cursorInfo struct {
		col     int
		isMain  bool
	}
	var cursorsOnLine []cursorInfo

	if lineIdx == e.cursor {
		cursorsOnLine = append(cursorsOnLine, cursorInfo{col: e.col, isMain: true})
	}
	for _, c := range e.cursors {
		if c.Line == lineIdx {
			cursorsOnLine = append(cursorsOnLine, cursorInfo{col: c.Col, isMain: false})
		}
	}

	// Sort by column position
	sort.Slice(cursorsOnLine, func(i, j int) bool {
		return cursorsOnLine[i].col < cursorsOnLine[j].col
	})

	// Build the line segment by segment
	var result strings.Builder
	prevCol := 0
	for _, ci := range cursorsOnLine {
		col := ci.col
		if col > len(displayLine) {
			col = len(displayLine)
		}
		if col < prevCol {
			continue // overlapping cursor, skip
		}

		// Render text before this cursor
		if col > prevCol {
			segment := displayLine[prevCol:col]
			result.WriteString(highlightLine(segment, lineIdx, fmStart, fmEnd, codeBlocks))
		}

		// Render the cursor character
		cursorChar := " "
		if col < len(displayLine) {
			cursorChar = string(displayLine[col])
		}
		if ci.isMain {
			result.WriteString(CursorStyle.Render(cursorChar))
		} else {
			result.WriteString(multiCursorStyle.Render(cursorChar))
		}

		if col < len(displayLine) {
			prevCol = col + 1
		} else {
			prevCol = col
		}
	}

	// Render remaining text after last cursor
	if prevCol < len(displayLine) {
		segment := displayLine[prevCol:]
		result.WriteString(highlightLine(segment, lineIdx, fmStart, fmEnd, codeBlocks))
	}

	lineContent := result.String()
	if e.highlightCurrentLine && lineIdx == e.cursor {
		lineContent = lipgloss.NewStyle().Background(surface0).Width(maxWidth).Render(lineContent)
	}
	return lineContent
}

// searchMatchesOnLine returns the search matches that fall on the given line,
// adjusted for horizontal scroll offset.
func (e *Editor) searchMatchesOnLine(lineIdx, colOffset int) []SearchMatch {
	var matches []SearchMatch
	for _, m := range e.searchMatches {
		if m.Line == lineIdx {
			adj := SearchMatch{
				Line:     m.Line,
				StartCol: m.StartCol - colOffset,
				EndCol:   m.EndCol - colOffset,
			}
			if adj.EndCol <= 0 {
				continue
			}
			if adj.StartCol < 0 {
				adj.StartCol = 0
			}
			matches = append(matches, adj)
		}
	}
	return matches
}

// renderWithSearchHighlights renders a line with search match highlighting applied.
// It splits the line into segments and styles matched regions.
func (e *Editor) renderWithSearchHighlights(line string, lineIdx, fmStart, fmEnd int, codeBlocks map[int]string, colOffset int) string {
	matches := e.searchMatchesOnLine(lineIdx, colOffset)
	if len(matches) == 0 {
		return highlightLine(line, lineIdx, fmStart, fmEnd, codeBlocks)
	}

	searchStyle := lipgloss.NewStyle().Background(yellow).Foreground(base).Bold(true)
	currentStyle := lipgloss.NewStyle().Background(peach).Foreground(base).Bold(true)

	var result strings.Builder
	prevCol := 0
	for _, m := range matches {
		sc := m.StartCol
		ec := m.EndCol
		if sc >= len(line) {
			continue
		}
		if ec > len(line) {
			ec = len(line)
		}
		if sc < prevCol {
			sc = prevCol
		}
		if sc >= ec {
			continue
		}

		// Render text before the match
		if sc > prevCol {
			segment := line[prevCol:sc]
			result.WriteString(highlightLine(segment, lineIdx, fmStart, fmEnd, codeBlocks))
		}

		// Render the match with highlight
		matchText := line[sc:ec]
		// Check if this is the current/active match
		isCurrentMatch := false
		for mi, sm := range e.searchMatches {
			if sm.Line == lineIdx && sm.StartCol-colOffset == sc && mi == e.currentSearchMatch {
				isCurrentMatch = true
				break
			}
		}
		if isCurrentMatch {
			result.WriteString(currentStyle.Render(matchText))
		} else {
			result.WriteString(searchStyle.Render(matchText))
		}
		prevCol = ec
	}

	// Render remaining text after last match
	if prevCol < len(line) {
		segment := line[prevCol:]
		result.WriteString(highlightLine(segment, lineIdx, fmStart, fmEnd, codeBlocks))
	}

	return result.String()
}

func (e Editor) View() string {
	var b strings.Builder
	contentWidth := e.width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Header
	headerIcon := " "
	if e.modified {
		headerIcon = " "
	}
	headerText := headerIcon + " " + e.filePath
	if e.modified {
		headerText += "  " + lipgloss.NewStyle().Foreground(yellow).Render("[modified]")
	}
	b.WriteString(HeaderStyle.Render(headerText))
	b.WriteString("\n")

	// Separator
	b.WriteString(DimStyle.Render(strings.Repeat("─", contentWidth)))
	b.WriteString("\n")

	visibleHeight := e.height - 4
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	end := e.scroll + visibleHeight
	if end > len(e.content) {
		end = len(e.content)
	}

	// Detect frontmatter region
	fmStart, fmEnd := -1, -1
	if len(e.content) > 0 && strings.TrimSpace(e.content[0]) == "---" {
		fmStart = 0
		for j := 1; j < len(e.content); j++ {
			if strings.TrimSpace(e.content[j]) == "---" {
				fmEnd = j
				break
			}
		}
	}

	// Detect code block regions and track language per line
	inCodeBlock := false
	currentLang := ""
	codeBlockLines := make(map[int]string)
	for j := 0; j < len(e.content); j++ {
		trimmed := strings.TrimSpace(e.content[j])
		if strings.HasPrefix(trimmed, "```") {
			if !inCodeBlock {
				currentLang = parseFenceLang(e.content[j])
				inCodeBlock = true
				codeBlockLines[j] = "" // fence line itself gets no language highlight
			} else {
				inCodeBlock = false
				codeBlockLines[j] = "" // closing fence
				currentLang = ""
			}
		} else if inCodeBlock {
			codeBlockLines[j] = currentLang
		}
	}

	// Style for additional multi-cursors (mauve background)
	multiCursorStyle := lipgloss.NewStyle().
		Background(mauve).
		Foreground(base)

	// Ensure scroll doesn't start inside a folded region
	for e.scroll > 0 && e.isLineFolded(e.scroll) {
		e.scroll--
	}

	visibleCount := 0
	lastRenderedLine := -1
	for i := e.scroll; i < len(e.content) && visibleCount < visibleHeight; i++ {
		// Skip folded (hidden) lines
		if e.isLineFolded(i) {
			continue
		}
		visibleCount++
		lastRenderedLine = i

		line := e.content[i]
		isActiveLine := (i == e.cursor && e.focused)
		hasMultiCursor := e.focused && e.hasAnyCursorOnLine(i)

		// Line number (conditional)
		gutterWidth := 0
		if e.showLineNumbers {
			lineNum := fmt.Sprintf("%4d ", i+1)
			if isActiveLine || hasMultiCursor {
				b.WriteString(ActiveLineNumStyle.Render(lineNum))
			} else {
				b.WriteString(LineNumStyle.Render(lineNum))
			}

			// Fold indicator in gutter
			if e.foldState != nil {
				indicator := e.foldState.GetFoldIndicator(i, e.content)
				if indicator != "" {
					b.WriteString(lipgloss.NewStyle().Foreground(peach).Render(indicator))
				} else {
					b.WriteString(" ")
				}
			} else {
				b.WriteString(" ")
			}
			gutterWidth = 7
		}

		// Determine max display width
		maxWidth := contentWidth - gutterWidth
		if maxWidth < 5 {
			maxWidth = 5
		}

		// Apply horizontal scroll for active line, truncate others
		displayLine := line
		colOffset := 0
		if isActiveLine {
			// Adjust hscroll to keep cursor visible
			if e.col < e.hscroll {
				e.hscroll = e.col
			} else if e.col >= e.hscroll+maxWidth {
				e.hscroll = e.col - maxWidth + 1
			}
			colOffset = e.hscroll
			if colOffset > 0 && colOffset < len(displayLine) {
				displayLine = displayLine[colOffset:]
			} else if colOffset >= len(displayLine) {
				displayLine = ""
			}
		}
		if len(displayLine) > maxWidth {
			displayLine = displayLine[:maxWidth]
		}

		if isActiveLine && !hasMultiCursor {
			// Render with main cursor only
			ghostStyle := lipgloss.NewStyle().Foreground(surface2).Italic(true)
			ghostSuffix := ""
			if e.ghostText != "" {
				ghostSuffix = ghostStyle.Render(e.ghostText)
			}
			adjCol := e.col - colOffset
			if adjCol < 0 {
				adjCol = 0
			}
			if adjCol <= len(displayLine) {
				before := displayLine[:adjCol]
				cursorChar := " "
				if adjCol < len(displayLine) {
					cursorChar = string(displayLine[adjCol])
				}
				after := ""
				if adjCol+1 < len(displayLine) {
					after = displayLine[adjCol+1:]
				}
				var styledBefore, styledAfter string
				if len(e.searchMatches) > 0 {
					styledBefore = e.renderWithSearchHighlights(before, i, fmStart, fmEnd, codeBlockLines, colOffset)
					styledAfter = e.renderWithSearchHighlights(after, i, fmStart, fmEnd, codeBlockLines, colOffset+adjCol+1)
				} else {
					styledBefore = highlightLine(before, i, fmStart, fmEnd, codeBlockLines)
					styledAfter = highlightLine(after, i, fmStart, fmEnd, codeBlockLines)
				}
				lineContent := styledBefore + CursorStyle.Render(cursorChar) + styledAfter + ghostSuffix
				if e.highlightCurrentLine {
					lineContent = lipgloss.NewStyle().Background(surface0).Width(maxWidth).Render(lineContent)
				}
				b.WriteString(lineContent)
			} else {
				var styledLine string
				if len(e.searchMatches) > 0 {
					styledLine = e.renderWithSearchHighlights(displayLine, i, fmStart, fmEnd, codeBlockLines, colOffset)
				} else {
					styledLine = highlightLine(displayLine, i, fmStart, fmEnd, codeBlockLines)
				}
				lineContent := styledLine + CursorStyle.Render(" ") + ghostSuffix
				if e.highlightCurrentLine {
					lineContent = lipgloss.NewStyle().Background(surface0).Width(maxWidth).Render(lineContent)
				}
				b.WriteString(lineContent)
			}

			// Show horizontal scroll indicator
			if colOffset > 0 {
				b.WriteString(DimStyle.Render(" ←"))
			}
		} else if isActiveLine || hasMultiCursor {
			// Line has cursor(s): render character by character to place cursor highlights
			b.WriteString(e.renderLineWithCursors(displayLine, i, fmStart, fmEnd, codeBlockLines, multiCursorStyle, maxWidth))
		} else {
			if len(e.searchMatches) > 0 {
				b.WriteString(e.renderWithSearchHighlights(displayLine, i, fmStart, fmEnd, codeBlockLines, 0))
			} else {
				b.WriteString(highlightLine(displayLine, i, fmStart, fmEnd, codeBlockLines))
			}
		}

		// Show folded line count after heading/fence content
		if e.foldState != nil {
			if endLine, ok := e.foldState.GetFoldEnd(i); ok {
				count := endLine - i
				b.WriteString(DimStyle.Render(fmt.Sprintf(" ··· %d lines", count)))
			}
		}

		if visibleCount < visibleHeight && lastRenderedLine < len(e.content)-1 {
			b.WriteString("\n")
		}
	}

	// Bottom info
	foldedCount := 0
	if e.foldState != nil {
		for i := 0; i < len(e.content); i++ {
			if e.isLineFolded(i) {
				foldedCount++
			}
		}
	}
	if len(e.content) > visibleHeight || foldedCount > 0 {
		b.WriteString("\n")
		pct := float64(e.scroll) / float64(maxInt(1, len(e.content)-visibleHeight)) * 100
		info := fmt.Sprintf("  %d lines  %.0f%%", len(e.content), pct)
		if foldedCount > 0 {
			info += fmt.Sprintf("  [%d folded]", foldedCount)
		}
		b.WriteString(DimStyle.Render(info))
	}

	return b.String()
}

func highlightLine(line string, lineIdx int, fmStart, fmEnd int, codeBlocks map[int]string) string {
	if line == "" {
		return ""
	}

	// Frontmatter
	if fmStart >= 0 && fmEnd >= 0 && lineIdx >= fmStart && lineIdx <= fmEnd {
		return FrontmatterStyle.Render(line)
	}

	// Code blocks
	if lang, inBlock := codeBlocks[lineIdx]; inBlock {
		// Fence lines (``` markers) keep the plain code block style
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			return CodeBlockStyle.Render(line)
		}
		// Content lines get language-aware syntax highlighting
		return highlightCodeLine(line, lang)
	}

	trimmed := strings.TrimSpace(line)

	// Headings
	if strings.HasPrefix(trimmed, "# ") {
		return TitleStyle.Render(line)
	}
	if strings.HasPrefix(trimmed, "## ") {
		return H2Style.Render(line)
	}
	if strings.HasPrefix(trimmed, "### ") || strings.HasPrefix(trimmed, "#### ") ||
		strings.HasPrefix(trimmed, "##### ") || strings.HasPrefix(trimmed, "###### ") {
		return H3Style.Render(line)
	}

	// Horizontal rule
	if trimmed == "---" || trimmed == "***" || trimmed == "___" {
		return DimStyle.Render(line)
	}

	// Blockquote
	if strings.HasPrefix(trimmed, "> ") {
		return BlockquoteStyle.Render(line)
	}

	// Checkboxes
	if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
		return CheckboxDone.Render(line)
	}
	if strings.HasPrefix(trimmed, "- [ ] ") {
		return CheckboxTodo.Render(line)
	}

	// List items
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		marker := string(trimmed[0]) + " "
		rest := trimmed[2:]
		indent := line[:len(line)-len(trimmed)]
		return indent + ListMarkerStyle.Render(marker) + highlightInline(rest)
	}

	// Numbered list
	for i, ch := range trimmed {
		if ch == '.' && i > 0 && i < 4 {
			if i+1 < len(trimmed) && trimmed[i+1] == ' ' {
				allDigits := true
				for j := 0; j < i; j++ {
					if trimmed[j] < '0' || trimmed[j] > '9' {
						allDigits = false
						break
					}
				}
				if allDigits {
					indent := line[:len(line)-len(trimmed)]
					marker := trimmed[:i+2]
					rest := trimmed[i+2:]
					return indent + ListMarkerStyle.Render(marker) + highlightInline(rest)
				}
			}
			break
		}
		if ch < '0' || ch > '9' {
			break
		}
	}

	return highlightInline(line)
}

func highlightInline(line string) string {
	if line == "" {
		return ""
	}

	var result strings.Builder
	i := 0
	runes := []rune(line)
	n := len(runes)

	for i < n {
		// WikiLinks [[...]]
		if i+1 < n && runes[i] == '[' && runes[i+1] == '[' {
			end := findClosing(runes, i+2, ']', ']')
			if end != -1 {
				result.WriteString(LinkStyle.Render(string(runes[i : end+1])))
				i = end + 1
				continue
			}
		}

		// Footnote references [^id]
		if i+1 < n && runes[i] == '[' && runes[i+1] == '^' {
			end := findSingle(runes, i+2, ']')
			if end != -1 && end > i+2 {
				id := string(runes[i+2 : end])
				result.WriteString(RenderFootnoteMarker(id))
				i = end + 1
				continue
			}
		}

		// Inline code `...`
		if runes[i] == '`' {
			end := findSingle(runes, i+1, '`')
			if end != -1 {
				result.WriteString(CodeStyle.Render(string(runes[i : end+1])))
				i = end + 1
				continue
			}
		}

		// Bold **...**
		if i+1 < n && runes[i] == '*' && runes[i+1] == '*' {
			end := findDouble(runes, i+2, '*')
			if end != -1 {
				result.WriteString(BoldTextStyle.Render(string(runes[i : end+1])))
				i = end + 1
				continue
			}
		}

		// Italic *...*
		if runes[i] == '*' && (i+1 < n && runes[i+1] != '*') {
			end := findSingle(runes, i+1, '*')
			if end != -1 && end > i+1 {
				result.WriteString(ItalicTextStyle.Render(string(runes[i : end+1])))
				i = end + 1
				continue
			}
		}

		// Tags #tag
		if runes[i] == '#' && (i == 0 || runes[i-1] == ' ') {
			// But not headings (already handled)
			end := i + 1
			for end < n && runes[end] != ' ' && runes[end] != '\t' {
				end++
			}
			if end > i+1 {
				result.WriteString(lipgloss.NewStyle().Foreground(blue).Render(string(runes[i:end])))
				i = end
				continue
			}
		}

		result.WriteRune(runes[i])
		i++
	}

	return result.String()
}

func findClosing(runes []rune, start int, c1, c2 rune) int {
	for i := start; i+1 < len(runes); i++ {
		if runes[i] == c1 && runes[i+1] == c2 {
			return i + 1
		}
	}
	return -1
}

func findSingle(runes []rune, start int, ch rune) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == ch {
			return i
		}
	}
	return -1
}

func findDouble(runes []rune, start int, ch rune) int {
	for i := start; i+1 < len(runes); i++ {
		if runes[i] == ch && runes[i+1] == ch {
			return i + 1
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// Markdown table editing support
// ---------------------------------------------------------------------------

// separatorLineRe matches table separator lines like |---|---|, | :---: | --- |, etc.
var separatorLineRe = regexp.MustCompile(`^\s*\|[\s:\-|]+\|\s*$`)

// isTableLine returns true if the line looks like part of a markdown table
// (starts with `|`, possibly with leading whitespace).
func isTableLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "|")
}

// isSeparatorLine returns true if the line is a table separator row (e.g. |---|---|).
func isSeparatorLine(line string) bool {
	return separatorLineRe.MatchString(line)
}

// isInTable returns true if the current cursor line is part of a markdown table.
func (e *Editor) isInTable() bool {
	if e.cursor < 0 || e.cursor >= len(e.content) {
		return false
	}
	return isTableLine(e.content[e.cursor])
}

// tableRange returns the start and end line indices (inclusive) of the contiguous
// table block containing the cursor line. Both start and end are guaranteed to
// be valid indices into e.content.
func (e *Editor) tableRange() (start, end int) {
	start = e.cursor
	for start > 0 && isTableLine(e.content[start-1]) {
		start--
	}
	end = e.cursor
	for end < len(e.content)-1 && isTableLine(e.content[end+1]) {
		end++
	}
	return start, end
}

// countTableColumns counts the number of data columns in a table line.
// It splits on `|`, trims the leading and trailing empty elements that result
// from the outer pipes, and returns the count. For "|a|b|c|" this returns 3.
func countTableColumns(line string) int {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || trimmed == "|" {
		return 0
	}
	// Remove leading and trailing pipe
	if trimmed[0] == '|' {
		trimmed = trimmed[1:]
	}
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '|' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	if trimmed == "" {
		return 1 // edge case: line was "||"
	}
	return strings.Count(trimmed, "|") + 1
}

// buildEmptyRow constructs an empty table row with the given number of columns,
// e.g. buildEmptyRow(3) returns "|  |  |  |".
func buildEmptyRow(cols int) string {
	if cols <= 0 {
		cols = 1
	}
	var b strings.Builder
	for i := 0; i < cols; i++ {
		b.WriteString("|  ")
	}
	b.WriteString("|")
	return b.String()
}

// tabInTable handles Tab key inside a table. It moves the cursor to the next
// cell. If the cursor is in the last cell of the last row, a new empty row is
// appended. After navigation it calls alignTableAt to tidy the table.
func (e *Editor) tabInTable() {
	line := e.content[e.cursor]
	col := e.col

	// Find the next `|` after the current cursor position.
	nextPipe := -1
	for i := col; i < len(line); i++ {
		if line[i] == '|' {
			nextPipe = i
			break
		}
	}

	if nextPipe >= 0 {
		// Check if there is another `|` after this one (i.e. another cell on this line).
		afterPipe := nextPipe + 1
		anotherPipe := strings.Index(line[afterPipe:], "|")
		if anotherPipe >= 0 {
			// Move cursor to just after the `|` we found, skipping one space if present.
			target := afterPipe
			if target < len(line) && line[target] == ' ' {
				target++
			}
			e.col = target
			e.alignTableAt()
			e.modified = true
			return
		}
	}

	// No more cells on this line — move to the next row.
	_, tableEnd := e.tableRange()
	nextRow := e.cursor + 1

	// Skip separator line
	if nextRow <= tableEnd && isSeparatorLine(e.content[nextRow]) {
		nextRow++
	}

	if nextRow <= tableEnd {
		// Move to the first cell of the next row.
		e.cursor = nextRow
		e.col = 0
		nextLine := e.content[e.cursor]
		firstPipe := strings.Index(nextLine, "|")
		if firstPipe >= 0 {
			target := firstPipe + 1
			if target < len(nextLine) && nextLine[target] == ' ' {
				target++
			}
			e.col = target
		}
	} else {
		// At or past the last row — insert a new empty row.
		cols := countTableColumns(e.content[e.cursor])
		newRow := buildEmptyRow(cols)
		insertIdx := tableEnd + 1
		newContent := make([]string, 0, len(e.content)+1)
		newContent = append(newContent, e.content[:insertIdx]...)
		newContent = append(newContent, newRow)
		newContent = append(newContent, e.content[insertIdx:]...)
		e.content = newContent
		e.cursor = insertIdx
		// Place cursor after the first `|` and optional space.
		e.col = 1
		if len(newRow) > 1 && newRow[1] == ' ' {
			e.col = 2
		}
	}

	e.alignTableAt()
	e.modified = true
	e.countWords()
}

// enterInTable handles Enter key inside a table. It inserts a new empty row
// below the current line (with matching column count) and moves the cursor to
// the first cell of the new row.
func (e *Editor) enterInTable() {
	cols := countTableColumns(e.content[e.cursor])
	newRow := buildEmptyRow(cols)

	insertIdx := e.cursor + 1
	newContent := make([]string, 0, len(e.content)+1)
	newContent = append(newContent, e.content[:insertIdx]...)
	newContent = append(newContent, newRow)
	newContent = append(newContent, e.content[insertIdx:]...)
	e.content = newContent

	e.cursor = insertIdx
	// Place cursor after first `|` and optional space.
	e.col = 1
	if len(newRow) > 1 && newRow[1] == ' ' {
		e.col = 2
	}

	e.alignTableAt()
	e.modified = true
	e.countWords()

	// Ensure the cursor is visible.
	visibleHeight := e.height - 4
	if visibleHeight < 1 {
		visibleHeight = 1
	}
	if e.cursor >= e.scroll+visibleHeight {
		e.scroll = e.cursor - visibleHeight + 1
	}
}

// alignTableAt aligns all columns in the markdown table surrounding the
// current cursor line. Each cell is padded with spaces so that all rows have
// uniform column widths. Separator lines are rebuilt with the correct dash
// counts.
func (e *Editor) alignTableAt() {
	start, end := e.tableRange()
	if start > end {
		return
	}

	// Parse every row into cells.
	type rowCells struct {
		cells       []string
		isSeparator bool
		// For separator cells, remember alignment markers.
		alignLeft  []bool
		alignRight []bool
	}

	rows := make([]rowCells, 0, end-start+1)
	maxCols := 0

	for i := start; i <= end; i++ {
		line := strings.TrimSpace(e.content[i])
		sep := isSeparatorLine(e.content[i])

		// Strip leading and trailing `|`.
		if len(line) > 0 && line[0] == '|' {
			line = line[1:]
		}
		if len(line) > 0 && line[len(line)-1] == '|' {
			line = line[:len(line)-1]
		}

		parts := strings.Split(line, "|")
		cells := make([]string, len(parts))
		alignL := make([]bool, len(parts))
		alignR := make([]bool, len(parts))

		for j, p := range parts {
			trimmed := strings.TrimSpace(p)
			cells[j] = trimmed
			if sep {
				alignL[j] = strings.HasPrefix(trimmed, ":")
				alignR[j] = strings.HasSuffix(trimmed, ":")
			}
		}
		if len(cells) > maxCols {
			maxCols = len(cells)
		}
		rows = append(rows, rowCells{
			cells:       cells,
			isSeparator: sep,
			alignLeft:   alignL,
			alignRight:  alignR,
		})
	}

	if maxCols == 0 {
		return
	}

	// Normalise: ensure every row has maxCols cells.
	for i := range rows {
		for len(rows[i].cells) < maxCols {
			rows[i].cells = append(rows[i].cells, "")
			rows[i].alignLeft = append(rows[i].alignLeft, false)
			rows[i].alignRight = append(rows[i].alignRight, false)
		}
	}

	// Determine the maximum width per column (ignoring separator rows).
	colWidths := make([]int, maxCols)
	for _, r := range rows {
		if r.isSeparator {
			continue
		}
		for j, c := range r.cells {
			if len(c) > colWidths[j] {
				colWidths[j] = len(c)
			}
		}
	}
	// Ensure a minimum width of 3 for each column (so separators look like ---).
	for j := range colWidths {
		if colWidths[j] < 3 {
			colWidths[j] = 3
		}
	}

	// Rebuild lines.
	for ri, r := range rows {
		var b strings.Builder
		for j, c := range r.cells {
			b.WriteByte('|')
			b.WriteByte(' ')
			if r.isSeparator {
				// Build separator cell.
				dashCount := colWidths[j]
				prefix := ""
				suffix := ""
				if r.alignLeft[j] {
					prefix = ":"
					dashCount--
				}
				if r.alignRight[j] {
					suffix = ":"
					dashCount--
				}
				if dashCount < 1 {
					dashCount = 1
				}
				b.WriteString(prefix)
				b.WriteString(strings.Repeat("-", dashCount))
				b.WriteString(suffix)
			} else {
				b.WriteString(c)
				padding := colWidths[j] - len(c)
				if padding > 0 {
					b.WriteString(strings.Repeat(" ", padding))
				}
			}
			b.WriteByte(' ')
		}
		b.WriteByte('|')
		e.content[start+ri] = b.String()
	}

	// Adjust the cursor column — make sure it doesn't exceed the new line length.
	if e.cursor >= start && e.cursor <= end {
		line := e.content[e.cursor]
		if e.col > len(line) {
			e.col = len(line)
		}
	}
}
