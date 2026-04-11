package tui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/config"
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
	insertTabs           bool
	autoIndent           bool

	// Horizontal scroll for long lines
	hscroll int

	// Word wrap
	wordWrap bool

	// Ghost text (AI inline completion)
	ghostText string // dimmed suggestion shown after cursor

	// Heading/code-fence folding
	foldState *FoldState

	// Bracket matching
	matchBracketLine    int
	matchBracketCol     int
	matchBracketValid   bool
	codeFenceCache      map[int]bool
	codeFenceCacheDirty bool

	// Cached code block and frontmatter detection for View()
	viewCodeBlockCache map[int]string // line index → language
	viewFMStart        int            // frontmatter start line (-1 = none)
	viewFMEnd          int            // frontmatter end line (-1 = none)
	viewCacheValid     bool           // invalidated alongside codeFenceCacheDirty

	// Shift+Arrow text selection
	selectionActive bool
	selectionStart  CursorPos // where selection began
	selectionEnd    CursorPos // current cursor position (selection extends to here)

	// Vim search highlights
	searchMatches      []SearchMatch
	currentSearchMatch int

	// Inline spell check: positions of misspelled characters (line -> col -> true)
	spellPositions map[int]map[int]bool
}

// SetWordWrap enables or disables word wrapping in the editor.
func (e *Editor) SetWordWrap(enabled bool) {
	e.wordWrap = enabled
	if enabled {
		e.hscroll = 0
	}
}

// SetEditorConfig applies editor configuration (tab size, insert tabs, auto indent).
func (e *Editor) SetEditorConfig(cfg config.EditorConfig) {
	e.tabSize = cfg.TabSize
	if e.tabSize < 1 {
		e.tabSize = 4
	}
	e.insertTabs = cfg.InsertTabs
	e.autoIndent = cfg.AutoIndent
}

func NewEditor() Editor {
	return Editor{
		content: []string{""},
	}
}

// rebuildBlockCaches rebuilds the code fence and code block caches.
// Call this whenever content changes to avoid O(n) rescanning in View().
func (e *Editor) rebuildBlockCaches() {
	// Rebuild codeFenceCache (used by bracket matching)
	inCodeFence := false
	e.codeFenceCache = make(map[int]bool)
	for j := 0; j < len(e.content); j++ {
		trimmed := strings.TrimSpace(e.content[j])
		if strings.HasPrefix(trimmed, "```") {
			inCodeFence = !inCodeFence
			e.codeFenceCache[j] = true
		} else if inCodeFence {
			e.codeFenceCache[j] = true
		}
	}
	e.codeFenceCacheDirty = false

	// Rebuild viewCodeBlockCache (used by View() for syntax highlighting)
	e.viewFMStart, e.viewFMEnd = -1, -1
	if len(e.content) > 0 && strings.TrimSpace(e.content[0]) == "---" {
		e.viewFMStart = 0
		for j := 1; j < len(e.content); j++ {
			if strings.TrimSpace(e.content[j]) == "---" {
				e.viewFMEnd = j
				break
			}
		}
	}

	inBlock := false
	currentLang := ""
	e.viewCodeBlockCache = make(map[int]string, len(e.content)/4)
	for j := 0; j < len(e.content); j++ {
		trimmed := strings.TrimSpace(e.content[j])
		if strings.HasPrefix(trimmed, "```") {
			if !inBlock {
				currentLang = parseFenceLang(e.content[j])
				inBlock = true
				e.viewCodeBlockCache[j] = ""
			} else {
				inBlock = false
				e.viewCodeBlockCache[j] = ""
				currentLang = ""
			}
		} else if inBlock {
			e.viewCodeBlockCache[j] = currentLang
		}
	}
	e.viewCacheValid = true
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
	e.selectionActive = false
	e.rebuildBlockCaches()
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

// CursorLine returns the current cursor line index.
func (e *Editor) CursorLine() int {
	return e.cursor
}

// CursorCol returns the current cursor column index.
func (e *Editor) CursorCol() int {
	return e.col
}

// ScrollOffset returns the current vertical scroll offset.
func (e *Editor) ScrollOffset() int {
	return e.scroll
}

// SetCursorPosition sets the cursor to the given line and column,
// clamping to valid ranges within the current content.
func (e *Editor) SetCursorPosition(line, col int) {
	if len(e.content) == 0 {
		e.content = []string{""}
	}
	if line < 0 {
		line = 0
	}
	if line >= len(e.content) {
		line = len(e.content) - 1
	}
	if col < 0 {
		col = 0
	}
	if col > len(e.content[line]) {
		col = len(e.content[line])
	}
	e.cursor = line
	e.col = col
}

// SetScroll sets the vertical scroll offset, clamping to valid range.
func (e *Editor) SetScroll(offset int) {
	if offset < 0 {
		offset = 0
	}
	e.scroll = offset
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
	e.rebuildBlockCaches()
	e.countWords()
}

func (e *Editor) InsertText(text string) {
	// Normalize line endings and strip null bytes
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\x00", "")
	if text == "" {
		return
	}

	e.saveSnapshot()
	e.rebuildBlockCaches()

	// Defensive guards: every other entry point that empties content
	// re-seeds it with []string{""}, but a paste with no surrounding
	// edit could in principle be the first call after a load that
	// returned an empty slice, and a stale cursor from a previous
	// content shape would index out of bounds. Normalise both before
	// the splice.
	if len(e.content) == 0 {
		e.content = []string{""}
	}
	if e.cursor < 0 {
		e.cursor = 0
	}
	if e.cursor >= len(e.content) {
		e.cursor = len(e.content) - 1
	}

	// Batch insert: split text into lines and splice into content
	lines := strings.Split(text, "\n")

	curLine := e.content[e.cursor]
	if e.col > len(curLine) {
		e.col = len(curLine)
	}
	if e.col < 0 {
		e.col = 0
	}
	before := curLine[:e.col]
	after := curLine[e.col:]

	if len(lines) == 1 {
		// Single line: just splice into current line
		e.content[e.cursor] = before + lines[0] + after
		e.col += len(lines[0])
	} else {
		// Multi-line: split current line, insert new lines between
		e.content[e.cursor] = before + lines[0]
		newContent := make([]string, 0, len(e.content)+len(lines)-1)
		newContent = append(newContent, e.content[:e.cursor+1]...)
		for i := 1; i < len(lines)-1; i++ {
			newContent = append(newContent, lines[i])
		}
		lastLine := lines[len(lines)-1] + after
		newContent = append(newContent, lastLine)
		newContent = append(newContent, e.content[e.cursor+1:]...)
		e.content = newContent
		e.cursor += len(lines) - 1
		e.col = len(lines[len(lines)-1])
	}

	e.modified = true
	e.countWords()
}

// isURL returns true when the trimmed string looks like a URL.
func isURL(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "ftp://") ||
		strings.HasPrefix(s, "www.")
}

// SmartPaste inserts clipboard text intelligently.
//
// Clipboard is URL + text selected     -> [selected text](url)
// Clipboard is URL + nothing selected  -> [url](url)
// Clipboard is text + selected URL     -> [clipboard text](selected_url)
//
// Returns false when none of the smart cases apply (caller should paste normally).
func (e *Editor) SmartPaste(clipboard string) bool {
	clipboard = strings.TrimSpace(clipboard)
	if !isURL(clipboard) {
		// Reverse case: clipboard is plain text, but selected text is a URL
		if e.HasSelection() {
			sel := strings.TrimSpace(e.GetSelectedText())
			if isURL(sel) {
				e.DeleteSelection()
				e.InsertText("[" + clipboard + "](" + sel + ")")
				return true
			}
		}
		return false
	}

	if e.HasSelection() {
		sel := e.GetSelectedText()
		e.DeleteSelection()
		e.InsertText("[" + sel + "](" + clipboard + ")")
		return true
	}

	// No selection — check if the word under the cursor is set via Ctrl+D
	if e.multiWord != "" {
		word := e.multiWord
		// Replace the word at each cursor with the markdown link.
		// For simplicity: clear multi-cursors, then replace at main cursor.
		startCol := -1
		if e.cursor < len(e.content) {
			line := e.content[e.cursor]
			idx := strings.Index(line, word)
			if idx >= 0 {
				startCol = idx
			}
		}
		if startCol >= 0 {
			e.saveSnapshot()
			e.clearMultiCursors()
			line := e.content[e.cursor]
			e.content[e.cursor] = line[:startCol] + "[" + word + "](" + clipboard + ")" + line[startCol+len(word):]
			e.col = startCol + len("["+word+"]("+clipboard+")")
			e.modified = true
			e.rebuildBlockCaches()
			e.countWords()
			return true
		}
	}

	// No selection, no multi-word — insert [url](url)
	e.InsertText("[" + clipboard + "](" + clipboard + ")")
	return true
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
	if len(e.undoStack) >= 100 {
		e.undoStack = e.undoStack[1:] // Remove oldest
	}
	e.undoStack = append(e.undoStack, snap)
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

	// Pop from undo stack and restore (deep copy to avoid corrupting history)
	snap := e.undoStack[len(e.undoStack)-1]
	e.undoStack = e.undoStack[:len(e.undoStack)-1]
	restored := make([]string, len(snap.content))
	copy(restored, snap.content)
	e.content = restored
	if len(e.content) == 0 {
		e.content = []string{""}
	}
	e.cursor, e.col = e.clampCursor(snap.cursor, snap.col)
	e.modified = true
	e.rebuildBlockCaches()
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

	// Pop from redo stack and restore (deep copy to avoid corrupting history)
	snap := e.redoStack[len(e.redoStack)-1]
	e.redoStack = e.redoStack[:len(e.redoStack)-1]
	restored := make([]string, len(snap.content))
	copy(restored, snap.content)
	e.content = restored
	if len(e.content) == 0 {
		e.content = []string{""}
	}
	e.cursor, e.col = e.clampCursor(snap.cursor, snap.col)
	e.modified = true
	e.rebuildBlockCaches()
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

// ClearSelection deactivates any active text selection.
func (e *Editor) ClearSelection() {
	e.selectionActive = false
}

// HasSelection reports whether the editor has an active text selection.
func (e *Editor) HasSelection() bool {
	return e.selectionActive
}

// SelectionRange returns the normalized selection range (start <= end).
func (e *Editor) SelectionRange() (startLine, startCol, endLine, endCol int) {
	s := e.selectionStart
	en := e.selectionEnd
	if s.Line < en.Line || (s.Line == en.Line && s.Col <= en.Col) {
		return s.Line, s.Col, en.Line, en.Col
	}
	return en.Line, en.Col, s.Line, s.Col
}

// GetSelectedText returns the text within the current selection.
func (e *Editor) GetSelectedText() string {
	if !e.selectionActive {
		return ""
	}
	sl, sc, el, ec := e.SelectionRange()
	if sl < 0 || el < 0 || sl >= len(e.content) || el >= len(e.content) {
		return ""
	}
	if sl == el {
		line := e.content[sl]
		if sc > len(line) {
			sc = len(line)
		}
		if ec > len(line) {
			ec = len(line)
		}
		return line[sc:ec]
	}
	var parts []string
	// First line: from startCol to end
	firstLine := e.content[sl]
	if sc > len(firstLine) {
		sc = len(firstLine)
	}
	parts = append(parts, firstLine[sc:])
	// Middle lines: full
	for i := sl + 1; i < el; i++ {
		parts = append(parts, e.content[i])
	}
	// Last line: from start to endCol
	lastLine := e.content[el]
	if ec > len(lastLine) {
		ec = len(lastLine)
	}
	parts = append(parts, lastLine[:ec])
	return strings.Join(parts, "\n")
}

// DeleteSelection removes the selected text and places the cursor at the
// start of the former selection.
func (e *Editor) DeleteSelection() {
	if !e.selectionActive {
		return
	}
	sl, sc, el, ec := e.SelectionRange()
	if sl < 0 || el < 0 || sl >= len(e.content) || el >= len(e.content) {
		e.selectionActive = false
		return
	}

	e.saveSnapshot()
	e.rebuildBlockCaches()
	e.redoStack = nil

	if sl == el {
		line := e.content[sl]
		if sc > len(line) {
			sc = len(line)
		}
		if ec > len(line) {
			ec = len(line)
		}
		e.content[sl] = line[:sc] + line[ec:]
	} else {
		firstLine := e.content[sl]
		if sc > len(firstLine) {
			sc = len(firstLine)
		}
		lastLine := e.content[el]
		if ec > len(lastLine) {
			ec = len(lastLine)
		}
		e.content[sl] = firstLine[:sc] + lastLine[ec:]
		e.content = append(e.content[:sl+1], e.content[el+1:]...)
	}

	if len(e.content) == 0 {
		e.content = []string{""}
	}
	e.cursor = sl
	e.col = sc
	e.selectionActive = false
	e.modified = true
	e.countWords()
}

// startOrExtendSelection begins a new selection at the current cursor if none
// is active, then updates selectionEnd to the current cursor position.
func (e *Editor) startOrExtendSelection() {
	if !e.selectionActive {
		e.selectionActive = true
		e.selectionStart = CursorPos{Line: e.cursor, Col: e.col}
	}
}

// updateSelectionEnd updates the selection end to the current cursor position.
func (e *Editor) updateSelectionEnd() {
	e.selectionEnd = CursorPos{Line: e.cursor, Col: e.col}
}

// clampCursor constrains a (line, col) pair to valid content bounds.
func (e *Editor) clampCursor(line, col int) (int, int) {
	if line < 0 {
		line = 0
	}
	if line >= len(e.content) {
		line = len(e.content) - 1
	}
	if line < 0 {
		return 0, 0
	}
	if col < 0 {
		col = 0
	}
	if col > len(e.content[line]) {
		col = len(e.content[line])
	}
	return line, col
}

// HasMultiCursors reports whether the editor has additional cursors active.
func (e *Editor) HasMultiCursors() bool {
	return len(e.cursors) > 0
}

// wordUnderCursor returns the word under the main cursor position and its
// start column index. A word is a contiguous run of letters, digits, or
// underscores. Returns the word and its starting byte offset on the current
// line. Operates on byte offsets so that multi-byte runes (emoji, accented
// characters, CJK) on the line do not produce wrong matches or out-of-range
// slicing.
func (e *Editor) wordUnderCursor() (string, int) {
	if e.cursor >= len(e.content) {
		return "", 0
	}
	line := e.content[e.cursor]
	if len(line) == 0 || e.col > len(line) {
		return "", 0
	}
	col := e.col
	if col == len(line) && col > 0 {
		// Cursor is just past end-of-line; back up one rune.
		_, sz := utf8.DecodeLastRuneInString(line[:col])
		col -= sz
	}
	if col < 0 {
		return "", 0
	}
	r, _ := utf8.DecodeRuneInString(line[col:])
	if !isWordRune(r) {
		// Cursor sits on whitespace/punctuation; try one rune to the left.
		if col == 0 {
			return "", 0
		}
		_, sz := utf8.DecodeLastRuneInString(line[:col])
		col -= sz
		r, _ = utf8.DecodeRuneInString(line[col:])
		if !isWordRune(r) {
			return "", 0
		}
	}
	// Walk backward to the start of the word.
	start := col
	for start > 0 {
		pr, sz := utf8.DecodeLastRuneInString(line[:start])
		if !isWordRune(pr) {
			break
		}
		start -= sz
	}
	// Walk forward to the end of the word.
	end := col
	for end < len(line) {
		nr, sz := utf8.DecodeRuneInString(line[end:])
		if !isWordRune(nr) {
			break
		}
		end += sz
	}
	return line[start:end], start
}

// isWordRune reports whether r is part of an identifier-style word (letters,
// digits, underscore).
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}


func (e Editor) Update(msg tea.Msg) (Editor, tea.Cmd) {
	if !e.focused {
		return e, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {

		// --- Shift+Arrow text selection ---
		case "shift+left":
			e.clearMultiCursors()
			e.startOrExtendSelection()
			// Move cursor left
			if e.col > 0 {
				e.col--
			} else if e.cursor > 0 {
				e.cursor--
				e.col = len(e.content[e.cursor])
			}
			e.updateSelectionEnd()
			return e, nil

		case "shift+right":
			e.clearMultiCursors()
			e.startOrExtendSelection()
			// Move cursor right
			if e.col < len(e.content[e.cursor]) {
				e.col++
			} else if e.cursor < len(e.content)-1 {
				e.cursor++
				e.col = 0
			}
			e.updateSelectionEnd()
			return e, nil

		case "shift+up":
			e.clearMultiCursors()
			e.startOrExtendSelection()
			if e.cursor > 0 {
				e.cursor--
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
			e.updateSelectionEnd()
			return e, nil

		case "shift+down":
			e.clearMultiCursors()
			e.startOrExtendSelection()
			if e.cursor < len(e.content)-1 {
				e.cursor++
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
			e.updateSelectionEnd()
			return e, nil

		case "shift+home":
			e.clearMultiCursors()
			e.startOrExtendSelection()
			e.col = 0
			e.updateSelectionEnd()
			return e, nil

		case "shift+end":
			e.clearMultiCursors()
			e.startOrExtendSelection()
			e.col = len(e.content[e.cursor])
			e.updateSelectionEnd()
			return e, nil

		case "up":
			e.ClearSelection()
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
					e.cursors[i].Line, e.cursors[i].Col = e.clampCursor(e.cursors[i].Line-1, e.cursors[i].Col)
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
			e.ClearSelection()
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
					e.cursors[i].Line, e.cursors[i].Col = e.clampCursor(e.cursors[i].Line+1, e.cursors[i].Col)
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
			e.ClearSelection()
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
			e.ClearSelection()
			if len(e.cursors) > 0 {
				if e.col < len(e.content[e.cursor]) {
					e.col++
				}
				for i := range e.cursors {
					if e.cursors[i].Line >= 0 && e.cursors[i].Line < len(e.content) &&
						e.cursors[i].Col < len(e.content[e.cursors[i].Line]) {
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
			e.ClearSelection()
			e.col = 0
			for i := range e.cursors {
				e.cursors[i].Col = 0
			}
		case "end":
			e.ClearSelection()
			e.col = len(e.content[e.cursor])
			for i := range e.cursors {
				if e.cursors[i].Line >= 0 && e.cursors[i].Line < len(e.content) {
					e.cursors[i].Col = len(e.content[e.cursors[i].Line])
				}
			}
		case "pgup":
			e.ClearSelection()
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
			e.ClearSelection()
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
			if e.selectionActive {
				e.DeleteSelection()
				return e, nil
			}
			e.saveSnapshot()
			e.redoStack = nil
			if len(e.cursors) == 0 && e.isInTable() {
				e.enterInTable()
			} else if len(e.cursors) > 0 {
				// Multi-cursor enter: process from bottom to top so that
				// line insertions don't shift indices of cursors still to process.
				allCursors := sortCursorsBottomUp(e.getAllCursors())
				primaryLine := e.cursor
				var indent string
				var insertedBelow int // count of newlines inserted at lines < primaryLine
				for _, c := range allCursors {
					if c.Line >= len(e.content) {
						continue
					}
					line := e.content[c.Line]
					col := c.Col
					if col > len(line) {
						col = len(line)
					}
					// Capture leading whitespace for auto-indent
					if e.autoIndent {
						indent = leadingWhitespace(line)
					}
					before := line[:col]
					after := indent + line[col:]
					e.content[c.Line] = before
					newContent := make([]string, 0, len(e.content)+1)
					newContent = append(newContent, e.content[:c.Line+1]...)
					newContent = append(newContent, after)
					newContent = append(newContent, e.content[c.Line+1:]...)
					e.content = newContent
					if c.Line < primaryLine {
						insertedBelow++
					}
				}
				e.cursor = primaryLine + 1 + insertedBelow
				if e.autoIndent && len(allCursors) > 0 {
					e.col = len(indent)
				} else {
					e.col = 0
				}
				e.clearMultiCursors()
				e.modified = true
				e.rebuildBlockCaches()
				e.countWords()
			} else {
				line := e.content[e.cursor]
				before := line[:e.col]
				after := line[e.col:]
				// Copy leading whitespace for auto-indent
				var indent string
				if e.autoIndent {
					indent = leadingWhitespace(line)
				}
				e.content[e.cursor] = before
				newContent := make([]string, 0, len(e.content)+1)
				newContent = append(newContent, e.content[:e.cursor+1]...)
				newContent = append(newContent, indent+after)
				newContent = append(newContent, e.content[e.cursor+1:]...)
				e.content = newContent
				e.cursor++
				e.col = len(indent)
				e.modified = true
				e.rebuildBlockCaches()
				e.countWords()
			}
		case "backspace":
			if e.selectionActive {
				e.DeleteSelection()
				return e, nil
			}
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
				e.rebuildBlockCaches()
				e.countWords()
			} else {
				if e.col > 0 {
					line := e.content[e.cursor]
					e.content[e.cursor] = line[:e.col-1] + line[e.col:]
					e.col--
					e.modified = true
					e.rebuildBlockCaches()
					e.countWords()
				} else if e.cursor > 0 {
					prevLen := len(e.content[e.cursor-1])
					e.content[e.cursor-1] += e.content[e.cursor]
					e.content = append(e.content[:e.cursor], e.content[e.cursor+1:]...)
					e.cursor--
					e.col = prevLen
					e.modified = true
					e.rebuildBlockCaches()
					e.countWords()
				}
			}
		case "delete":
			if e.selectionActive {
				e.DeleteSelection()
				return e, nil
			}
			e.saveSnapshot()
			e.redoStack = nil
			line := e.content[e.cursor]
			if e.col < len(line) {
				e.content[e.cursor] = line[:e.col] + line[e.col+1:]
				e.modified = true
				e.rebuildBlockCaches()
				e.countWords()
			} else if e.cursor < len(e.content)-1 {
				e.content[e.cursor] += e.content[e.cursor+1]
				e.content = append(e.content[:e.cursor+1], e.content[e.cursor+2:]...)
				e.modified = true
				e.rebuildBlockCaches()
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
				e.rebuildBlockCaches()
				e.countWords()
			}
		case "tab":
			e.saveSnapshot()
			e.redoStack = nil
			if len(e.cursors) == 0 && e.isInTable() {
				e.tabInTable()
			} else if len(e.cursors) > 0 {
				// Multi-cursor tab insertion
				allCursors := sortCursorsBottomUp(e.getAllCursors())
				var tabStr string
				var advance int
				if e.insertTabs {
					tabStr = "\t"
					advance = 1
				} else {
					tabStr = strings.Repeat(" ", e.tabSize)
					advance = e.tabSize
				}
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
				e.col += advance
				for i := range e.cursors {
					e.cursors[i].Col += advance
				}
				e.modified = true
				e.rebuildBlockCaches()
			} else {
				// Single cursor tab insertion
				var tabStr string
				var advance int
				if e.insertTabs {
					tabStr = "\t"
					advance = 1
				} else {
					tabStr = strings.Repeat(" ", e.tabSize)
					advance = e.tabSize
				}
				line := e.content[e.cursor]
				e.content[e.cursor] = line[:e.col] + tabStr + line[e.col:]
				e.col += advance
				e.modified = true
				e.rebuildBlockCaches()
			}
		default:
			char := msg.String()
			charRunes := []rune(char)
			if len(charRunes) == 1 && charRunes[0] >= 32 {
				// Replace selection with typed character if active
				if e.selectionActive {
					e.DeleteSelection()
				}
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
					e.rebuildBlockCaches()
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
					e.rebuildBlockCaches()
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
// col and length are byte offsets, matching the editor's cursor model. Uses
// utf8.DecodeLastRune / DecodeRune so the boundary checks work correctly when
// the line contains multi-byte runes (emoji, accented characters, CJK).
func (e *Editor) isWholeWord(lineIdx, col, length int) bool {
	if lineIdx >= len(e.content) {
		return false
	}
	line := e.content[lineIdx]
	if col < 0 || col+length > len(line) {
		return false
	}
	if col > 0 {
		r, _ := utf8.DecodeLastRuneInString(line[:col])
		if r == utf8.RuneError || unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return false
		}
	}
	end := col + length
	if end < len(line) {
		r, _ := utf8.DecodeRuneInString(line[end:])
		if r == utf8.RuneError || unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return false
		}
	}
	return true
}

// bracketPairs maps opening brackets to closing brackets and vice versa.
var bracketPairs = map[byte]byte{
	'(': ')', ')': '(',
	'[': ']', ']': '[',
	'{': '}', '}': '{',
}

// isOpenBracket returns true if the byte is an opening bracket.
func isOpenBracket(b byte) bool {
	return b == '(' || b == '[' || b == '{'
}

// isBracketChar returns true if the byte is any bracket character.
func isBracketChar(b byte) bool {
	_, ok := bracketPairs[b]
	return ok
}

// isInsideString checks if the given column position in a line is inside a
// quoted string (single or double quotes). This is a simple parity check.
func isInsideString(line string, col int) bool {
	inDouble := false
	inSingle := false
	for i := 0; i < col && i < len(line); i++ {
		ch := line[i]
		if ch == '"' && !inSingle {
			inDouble = !inDouble
		} else if ch == '\'' && !inDouble {
			inSingle = !inSingle
		}
	}
	return inDouble || inSingle
}

// findMatchingBracket searches for the bracket matching the one under (or just
// before) the cursor. It handles nesting and skips brackets inside strings.
// Returns (line, col, found).
func (e *Editor) findMatchingBracket() (int, int, bool) {
	if e.cursor >= len(e.content) {
		return 0, 0, false
	}
	curLine := e.content[e.cursor]

	// Check under cursor first, then one position before.
	bracketCol := -1
	if e.col < len(curLine) && isBracketChar(curLine[e.col]) {
		bracketCol = e.col
	} else if e.col > 0 && e.col-1 < len(curLine) && isBracketChar(curLine[e.col-1]) {
		bracketCol = e.col - 1
	}
	if bracketCol < 0 {
		return 0, 0, false
	}

	ch := curLine[bracketCol]
	target := bracketPairs[ch]

	// Use eagerly-rebuilt code fence cache to skip brackets inside fenced blocks.
	if e.codeFenceCache == nil {
		e.rebuildBlockCaches()
	}
	codeFenceLines := e.codeFenceCache

	// Skip if the bracket itself is inside a string.
	if isInsideString(curLine, bracketCol) {
		return 0, 0, false
	}

	if isOpenBracket(ch) {
		// Search forward
		depth := 1
		for ln := e.cursor; ln < len(e.content); ln++ {
			if codeFenceLines[ln] && !codeFenceLines[e.cursor] {
				continue
			}
			l := e.content[ln]
			from := 0
			if ln == e.cursor {
				from = bracketCol + 1
			}
			for c := from; c < len(l); c++ {
				if isInsideString(l, c) {
					continue
				}
				if l[c] == ch {
					depth++
				} else if l[c] == target {
					depth--
					if depth == 0 {
						return ln, c, true
					}
				}
			}
		}
	} else {
		// Search backward
		depth := 1
		for ln := e.cursor; ln >= 0; ln-- {
			if codeFenceLines[ln] && !codeFenceLines[e.cursor] {
				continue
			}
			l := e.content[ln]
			from := len(l) - 1
			if ln == e.cursor {
				from = bracketCol - 1
			}
			for c := from; c >= 0; c-- {
				if isInsideString(l, c) {
					continue
				}
				if l[c] == ch {
					depth++
				} else if l[c] == target {
					depth--
					if depth == 0 {
						return ln, c, true
					}
				}
			}
		}
	}

	return 0, 0, false
}

// updateBracketMatch recalculates the matching bracket position.
func (e *Editor) updateBracketMatch() {
	e.matchBracketLine, e.matchBracketCol, e.matchBracketValid = e.findMatchingBracket()
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
func (e *Editor) renderLineWithCursors(displayLine string, lineIdx, fmStart, fmEnd int, codeBlocks map[int]string, multiCursorStyle lipgloss.Style, maxWidth int, bracketStyle ...lipgloss.Style) string {
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
			if len(bracketStyle) > 0 && e.matchBracketValid && lineIdx == e.matchBracketLine {
				bracketRel := e.matchBracketCol - prevCol
				if bracketRel >= 0 && bracketRel < len(segment) {
					if bracketRel > 0 {
						result.WriteString(highlightLine(segment[:bracketRel], lineIdx, fmStart, fmEnd, codeBlocks))
					}
					result.WriteString(bracketStyle[0].Render(string(segment[bracketRel])))
					if bracketRel+1 < len(segment) {
						result.WriteString(highlightLine(segment[bracketRel+1:], lineIdx, fmStart, fmEnd, codeBlocks))
					}
				} else {
					result.WriteString(highlightLine(segment, lineIdx, fmStart, fmEnd, codeBlocks))
				}
			} else {
				result.WriteString(highlightLine(segment, lineIdx, fmStart, fmEnd, codeBlocks))
			}
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
		if len(bracketStyle) > 0 && e.matchBracketValid && lineIdx == e.matchBracketLine {
			bracketRel := e.matchBracketCol - prevCol
			if bracketRel >= 0 && bracketRel < len(segment) {
				if bracketRel > 0 {
					result.WriteString(highlightLine(segment[:bracketRel], lineIdx, fmStart, fmEnd, codeBlocks))
				}
				result.WriteString(bracketStyle[0].Render(string(segment[bracketRel])))
				if bracketRel+1 < len(segment) {
					result.WriteString(highlightLine(segment[bracketRel+1:], lineIdx, fmStart, fmEnd, codeBlocks))
				}
			} else {
				result.WriteString(highlightLine(segment, lineIdx, fmStart, fmEnd, codeBlocks))
			}
		} else {
			result.WriteString(highlightLine(segment, lineIdx, fmStart, fmEnd, codeBlocks))
		}
	}

	lineContent := result.String()
	if e.highlightCurrentLine && lineIdx == e.cursor {
		lineContent = lipgloss.NewStyle().Background(surface0).Width(maxWidth).Render(lineContent)
	}
	return lineContent
}

// isLineInSelection checks whether line lineIdx has any characters within the
// current selection. If so it returns (true, selStartCol, selEndCol) relative
// to the full line (before horizontal scroll / truncation).
func (e *Editor) isLineInSelection(lineIdx int) (bool, int, int) {
	if !e.selectionActive {
		return false, 0, 0
	}
	sl, sc, el, ec := e.SelectionRange()
	if lineIdx < sl || lineIdx > el {
		return false, 0, 0
	}
	lineLen := len(e.content[lineIdx])
	startCol := 0
	endCol := lineLen
	if lineIdx == sl {
		startCol = sc
	}
	if lineIdx == el {
		endCol = ec
	}
	if startCol > lineLen {
		startCol = lineLen
	}
	if endCol > lineLen {
		endCol = lineLen
	}
	return true, startCol, endCol
}

// renderLineWithSelectionHighlight renders a display line with selection
// highlighting applied. selStart/selEnd are column offsets into the original
// (pre-scroll) line; colOffset is the horizontal scroll offset that was applied.
func (e *Editor) renderLineWithSelectionHighlight(displayLine string, lineIdx, fmStart, fmEnd int, codeBlocks map[int]string, selStart, selEnd, colOffset int, selStyle lipgloss.Style) string {
	// Adjust selection columns for horizontal scroll
	adjStart := selStart - colOffset
	adjEnd := selEnd - colOffset
	if adjStart < 0 {
		adjStart = 0
	}
	if adjEnd < 0 {
		return highlightLine(displayLine, lineIdx, fmStart, fmEnd, codeBlocks)
	}
	if adjStart > len(displayLine) {
		return highlightLine(displayLine, lineIdx, fmStart, fmEnd, codeBlocks)
	}
	if adjEnd > len(displayLine) {
		adjEnd = len(displayLine)
	}
	if adjStart >= adjEnd {
		return highlightLine(displayLine, lineIdx, fmStart, fmEnd, codeBlocks)
	}

	var result strings.Builder
	// Before selection
	if adjStart > 0 {
		result.WriteString(highlightLine(displayLine[:adjStart], lineIdx, fmStart, fmEnd, codeBlocks))
	}
	// Selected text
	result.WriteString(selStyle.Render(displayLine[adjStart:adjEnd]))
	// After selection
	if adjEnd < len(displayLine) {
		result.WriteString(highlightLine(displayLine[adjEnd:], lineIdx, fmStart, fmEnd, codeBlocks))
	}
	return result.String()
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
		if sc > prevCol {
			result.WriteString(highlightLine(line[prevCol:sc], lineIdx, fmStart, fmEnd, codeBlocks))
		}
		matchText := line[sc:ec]
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
	if prevCol < len(line) {
		result.WriteString(highlightLine(line[prevCol:], lineIdx, fmStart, fmEnd, codeBlocks))
	}
	return result.String()
}

// SetSpellPositions updates the misspelled character position map used for
// inline spell check highlights in the editor.
func (e *Editor) SetSpellPositions(pos map[int]map[int]bool) {
	e.spellPositions = pos
}

// hasSpellErrors returns true if the given line has any misspelled characters.
func (e *Editor) hasSpellErrors(lineIdx int) bool {
	if e.spellPositions == nil {
		return false
	}
	_, ok := e.spellPositions[lineIdx]
	return ok
}

// renderWithSpellHighlights renders a line with misspelled words underlined in red.
// It delegates non-misspelled segments to highlightLine for normal syntax coloring.
func (e *Editor) renderWithSpellHighlights(line string, lineIdx, fmStart, fmEnd int, codeBlocks map[int]string, colOffset int) string {
	linePos := e.spellPositions[lineIdx]
	if linePos == nil {
		return highlightLine(line, lineIdx, fmStart, fmEnd, codeBlocks)
	}

	spellStyle := lipgloss.NewStyle().Foreground(red).Underline(true)

	var result strings.Builder
	inSpell := false
	spanStart := 0

	for i := 0; i <= len(line); i++ {
		absCol := colOffset + i
		isMisspelled := i < len(line) && linePos[absCol]

		if i == len(line) || isMisspelled != inSpell {
			// Flush the current segment
			seg := line[spanStart:i]
			if len(seg) > 0 {
				if inSpell {
					result.WriteString(spellStyle.Render(seg))
				} else {
					result.WriteString(highlightLine(seg, lineIdx, fmStart, fmEnd, codeBlocks))
				}
			}
			spanStart = i
			inSpell = isMisspelled
		}
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

	// Use cached frontmatter and code block detection (rebuilt only on content change)
	fmStart, fmEnd := e.viewFMStart, e.viewFMEnd
	codeBlockLines := e.viewCodeBlockCache
	if codeBlockLines == nil {
		codeBlockLines = make(map[int]string)
	}

	// Style for additional multi-cursors (mauve background)
	multiCursorStyle := lipgloss.NewStyle().
		Background(mauve).
		Foreground(base)

	// Style for text selection highlight
	_ = lipgloss.NewStyle().
		Background(surface1).
		Foreground(text)

	// Bracket matching: compute the matching bracket position
	e.updateBracketMatch()
	bracketStyle := lipgloss.NewStyle().
		Background(surface2).
		Foreground(peach).
		Bold(true)

	// Ensure scroll doesn't start inside a folded region
	for e.scroll > 0 && e.isLineFolded(e.scroll) {
		e.scroll--
	}

	// Calculate gutter width once for word wrap calculations
	gutterWidth := 0
	if e.showLineNumbers {
		gutterWidth = 7
	}

	visibleCount := 0
	var lastRenderedLine int
	for i := e.scroll; i < len(e.content) && visibleCount < visibleHeight; i++ {
		// Skip folded (hidden) lines
		if e.isLineFolded(i) {
			continue
		}

		line := e.content[i]
		isActiveLine := (i == e.cursor && e.focused)
		hasMultiCursor := e.focused && e.hasAnyCursorOnLine(i)

		// Determine max display width
		maxWidth := contentWidth - gutterWidth
		if maxWidth < 5 {
			maxWidth = 5
		}

		// Word wrap path: split content line into multiple visual lines
		if e.wordWrap && displayWidth(line) > maxWidth {
			visualLines := e.splitLineForWrap(line, maxWidth)
			// Determine which visual line the cursor is on (for active line)
			cursorVisualLine := 0
			cursorVisualCol := e.col
			if isActiveLine {
				remaining := e.col
				for vi, vl := range visualLines {
					if remaining <= len(vl) {
						cursorVisualLine = vi
						cursorVisualCol = remaining
						break
					}
					remaining -= len(vl)
					if vi < len(visualLines)-1 {
						cursorVisualLine = vi + 1
						cursorVisualCol = remaining
					}
				}
			}

			for vi, vl := range visualLines {
				if visibleCount >= visibleHeight {
					break
				}

				// Line number: only on first visual line; continuation gets wrap indicator
				if e.showLineNumbers {
					if vi == 0 {
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
					} else {
						// Continuation line: blank gutter with wrap indicator
						b.WriteString(DimStyle.Render("   " + WrapIndicator + " "))
						b.WriteString(" ")
					}
				}

				isCursorVisualLine := isActiveLine && vi == cursorVisualLine

				if isCursorVisualLine && !hasMultiCursor {
					// Render this visual line with cursor
					ghostStyle := lipgloss.NewStyle().Foreground(surface2).Italic(true)
					ghostSuffix := ""
					if e.ghostText != "" && vi == len(visualLines)-1 {
						ghostSuffix = ghostStyle.Render(e.ghostText)
					}
					adjCol := cursorVisualCol
					if adjCol < 0 {
						adjCol = 0
					}
					// Calculate segment offset for bracket matching
					segOffset := 0
					for svi := 0; svi < vi; svi++ {
						segOffset += len(visualLines[svi])
					}
					if adjCol <= len(vl) {
						before := vl[:adjCol]
						cursorChar := " "
						if adjCol < len(vl) {
							cursorChar = string(vl[adjCol])
						}
						after := ""
						if adjCol+1 < len(vl) {
							after = vl[adjCol+1:]
						}
						styledBefore := e.highlightLineWithBrackets(before, i, segOffset, fmStart, fmEnd, codeBlockLines, bracketStyle)
						styledAfter := e.highlightLineWithBrackets(after, i, segOffset+adjCol+1, fmStart, fmEnd, codeBlockLines, bracketStyle)
						lineContent := styledBefore + CursorStyle.Render(cursorChar) + styledAfter + ghostSuffix
						if e.highlightCurrentLine {
							lineContent = lipgloss.NewStyle().Background(surface0).Width(maxWidth).Render(lineContent)
						}
						b.WriteString(lineContent)
					} else {
						lineContent := highlightLine(vl, i, fmStart, fmEnd, codeBlockLines) + CursorStyle.Render(" ") + ghostSuffix
						if e.highlightCurrentLine {
							lineContent = lipgloss.NewStyle().Background(surface0).Width(maxWidth).Render(lineContent)
						}
						b.WriteString(lineContent)
					}
				} else if (isActiveLine || hasMultiCursor) && e.hasAnyCursorOnVisualLine(i, vi, maxWidth) {
					b.WriteString(e.renderWrappedLineWithCursors(vl, i, vi, maxWidth, fmStart, fmEnd, codeBlockLines, multiCursorStyle))
				} else {
					// Calculate column offset for this visual line segment
					wrapColOffset := 0
					for wvi := 0; wvi < vi; wvi++ {
						wrapColOffset += len(visualLines[wvi])
					}
					var styledLine string
					if e.hasSpellErrors(i) {
						styledLine = e.renderWithSpellHighlights(vl, i, fmStart, fmEnd, codeBlockLines, wrapColOffset)
					} else {
						styledLine = highlightLine(vl, i, fmStart, fmEnd, codeBlockLines)
					}
					if e.highlightCurrentLine && isActiveLine {
						styledLine = lipgloss.NewStyle().Background(surface0).Width(maxWidth).Render(styledLine)
					}
					b.WriteString(styledLine)
				}

				// Show folded line count only on first visual line
				if vi == 0 && e.foldState != nil {
					if endLine, ok := e.foldState.GetFoldEnd(i); ok {
						count := endLine - i
						b.WriteString(DimStyle.Render(fmt.Sprintf(" ··· %d lines", count)))
					}
				}

				visibleCount++
				lastRenderedLine = i
				if visibleCount < visibleHeight && (vi < len(visualLines)-1 || lastRenderedLine < len(e.content)-1) {
					b.WriteString("\n")
				}
			}
			continue
		}

		// Non-wrap path (original logic)
		visibleCount++
		lastRenderedLine = i

		// Line number (conditional)
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
		}

		// Apply horizontal scroll for active line, truncate others
		displayRunes := []rune(line)
		displayLine := ""
		colOffset := 0
		if isActiveLine && !e.wordWrap {
			// Adjust hscroll to keep cursor visible
			if e.col < e.hscroll {
				e.hscroll = e.col
			} else if e.col >= e.hscroll+maxWidth {
				e.hscroll = e.col - maxWidth + 1
			}
			colOffset = e.hscroll
			if colOffset > 0 && colOffset < len(displayRunes) {
				displayRunes = displayRunes[colOffset:]
			} else if colOffset >= len(displayRunes) {
				displayRunes = nil
			}
		}
		if len(displayRunes) > maxWidth {
			displayRunes = displayRunes[:maxWidth]
		}
		displayLine = string(displayRunes)

		// Check if this line has selection
		inSel, selSC, selEC := e.isLineInSelection(i)

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
				if inSel {
					styledBefore = e.renderLineWithSelectionHighlight(before, i, fmStart, fmEnd, codeBlockLines, selSC, selEC, colOffset, lipgloss.NewStyle().Background(surface1).Foreground(text))
					styledAfter = e.renderLineWithSelectionHighlight(after, i, fmStart, fmEnd, codeBlockLines, selSC, selEC, colOffset+adjCol+1, lipgloss.NewStyle().Background(surface1).Foreground(text))
				} else if len(e.searchMatches) > 0 {
					styledBefore = e.renderWithSearchHighlights(before, i, fmStart, fmEnd, codeBlockLines, colOffset)
					styledAfter = e.renderWithSearchHighlights(after, i, fmStart, fmEnd, codeBlockLines, colOffset+adjCol+1)
				} else {
					styledBefore = e.highlightLineWithBrackets(before, i, colOffset, fmStart, fmEnd, codeBlockLines, bracketStyle)
					styledAfter = e.highlightLineWithBrackets(after, i, colOffset+adjCol+1, fmStart, fmEnd, codeBlockLines, bracketStyle)
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
			b.WriteString(e.renderLineWithCursors(displayLine, i, fmStart, fmEnd, codeBlockLines, multiCursorStyle, maxWidth, bracketStyle))
		} else if inSel {
			b.WriteString(e.renderLineWithSelectionHighlight(displayLine, i, fmStart, fmEnd, codeBlockLines, selSC, selEC, colOffset, lipgloss.NewStyle().Background(surface1).Foreground(text)))
		} else {
			if len(e.searchMatches) > 0 {
				b.WriteString(e.renderWithSearchHighlights(displayLine, i, fmStart, fmEnd, codeBlockLines, 0))
			} else if e.matchBracketValid && i == e.matchBracketLine {
				b.WriteString(e.highlightLineWithBrackets(displayLine, i, 0, fmStart, fmEnd, codeBlockLines, bracketStyle))
			} else if e.hasSpellErrors(i) {
				b.WriteString(e.renderWithSpellHighlights(displayLine, i, fmStart, fmEnd, codeBlockLines, 0))
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
		if e.wordWrap {
			info += "  [wrap]"
		}
		b.WriteString(DimStyle.Render(info))
	}

	return b.String()
}


// displayWidth returns the display width of a string, accounting for
// wide characters (CJK, emoji) that take 2 columns.
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r >= 0x1100 && ((r <= 0x115F) || // Hangul Jamo
			(r >= 0x2E80 && r <= 0x9FFF) || // CJK
			(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility
			(r >= 0xFE30 && r <= 0xFE6F) || // CJK Forms
			(r >= 0xFF01 && r <= 0xFF60) || // Fullwidth
			(r >= 0xFFE0 && r <= 0xFFE6) || // Fullwidth
			(r >= 0x1F000 && r <= 0x1FFFF) || // Emoji
			(r >= 0x20000 && r <= 0x2FFFF)) { // CJK Ext B
			w += 2
		} else {
			w++
		}
	}
	return w
}

// splitLineForWrap splits a content line into multiple visual lines that fit
// within maxWidth. It tries to break at word boundaries (spaces) when possible.
func (e *Editor) splitLineForWrap(line string, maxWidth int) []string {
	if maxWidth < 1 {
		maxWidth = 1
	}
	if displayWidth(line) <= maxWidth {
		return []string{line}
	}

	var result []string
	remaining := line
	for displayWidth(remaining) > maxWidth {
		// Walk runes to find the break point that fits within maxWidth columns
		runes := []rune(remaining)
		breakAt := 0
		w := 0
		for ri, r := range runes {
			rw := 1
			if r >= 0x1100 && ((r <= 0x115F) ||
				(r >= 0x2E80 && r <= 0x9FFF) ||
				(r >= 0xF900 && r <= 0xFAFF) ||
				(r >= 0xFE30 && r <= 0xFE6F) ||
				(r >= 0xFF01 && r <= 0xFF60) ||
				(r >= 0xFFE0 && r <= 0xFFE6) ||
				(r >= 0x1F000 && r <= 0x1FFFF) ||
				(r >= 0x20000 && r <= 0x2FFFF)) {
				rw = 2
			}
			if w+rw > maxWidth {
				breakAt = ri
				break
			}
			w += rw
			breakAt = ri + 1
		}
		if breakAt == 0 {
			breakAt = 1 // always make progress
		}

		// Try to break at a space within the last half of the chunk
		lastSpace := -1
		halfPoint := breakAt / 2
		for j := breakAt - 1; j >= halfPoint; j-- {
			if runes[j] == ' ' {
				lastSpace = j
				break
			}
		}
		if lastSpace > 0 {
			breakAt = lastSpace + 1 // include the space in the current line
		}
		result = append(result, string(runes[:breakAt]))
		remaining = string(runes[breakAt:])
	}
	if len(remaining) > 0 {
		result = append(result, remaining)
	}
	return result
}

// hasAnyCursorOnVisualLine checks if any cursor (main or multi) falls on a
// specific visual line segment of a wrapped content line.
func (e *Editor) hasAnyCursorOnVisualLine(contentLine, visualLineIdx, maxWidth int) bool {
	visualLines := e.splitLineForWrap(e.content[contentLine], maxWidth)
	if visualLineIdx >= len(visualLines) {
		return false
	}

	// cursorOnSegment determines which visual line segment a cursor column falls on
	cursorOnSegment := func(cursorCol int) int {
		segStart := 0
		for vi := 0; vi < len(visualLines); vi++ {
			segEnd := segStart + len([]rune(visualLines[vi]))
			if cursorCol >= segStart && cursorCol < segEnd {
				return vi
			}
			segStart = segEnd
		}
		// Cursor at end of last segment
		return len(visualLines) - 1
	}

	if contentLine == e.cursor && cursorOnSegment(e.col) == visualLineIdx {
		return true
	}
	for _, c := range e.cursors {
		if c.Line == contentLine && cursorOnSegment(c.Col) == visualLineIdx {
			return true
		}
	}
	return false
}

// renderWrappedLineWithCursors renders a visual line segment from a wrapped
// content line, placing cursor highlights at the correct positions.
func (e *Editor) renderWrappedLineWithCursors(visualLine string, contentLine, visualLineIdx, maxWidth, fmStart, fmEnd int, codeBlocks map[int]string, multiCursorStyle lipgloss.Style) string {
	visualLines := e.splitLineForWrap(e.content[contentLine], maxWidth)
	startCol := 0
	for vi := 0; vi < visualLineIdx; vi++ {
		startCol += len([]rune(visualLines[vi]))
	}

	vlRunes := []rune(visualLine)

	type cursorInfo struct {
		col    int
		isMain bool
	}
	var cursorsOnLine []cursorInfo

	if contentLine == e.cursor {
		adjCol := e.col - startCol
		if adjCol >= 0 && adjCol <= len(vlRunes) {
			cursorsOnLine = append(cursorsOnLine, cursorInfo{col: adjCol, isMain: true})
		}
	}
	for _, c := range e.cursors {
		if c.Line == contentLine {
			adjCol := c.Col - startCol
			if adjCol >= 0 && adjCol <= len(vlRunes) {
				cursorsOnLine = append(cursorsOnLine, cursorInfo{col: adjCol, isMain: false})
			}
		}
	}

	sort.Slice(cursorsOnLine, func(i, j int) bool {
		return cursorsOnLine[i].col < cursorsOnLine[j].col
	})

	var result strings.Builder
	prevCol := 0
	for _, ci := range cursorsOnLine {
		col := ci.col
		if col > len(vlRunes) {
			col = len(vlRunes)
		}
		if col < prevCol {
			continue
		}
		if col > prevCol {
			segment := string(vlRunes[prevCol:col])
			result.WriteString(highlightLine(segment, contentLine, fmStart, fmEnd, codeBlocks))
		}
		cursorChar := " "
		if col < len(vlRunes) {
			cursorChar = string(vlRunes[col])
		}
		if ci.isMain {
			result.WriteString(CursorStyle.Render(cursorChar))
		} else {
			result.WriteString(multiCursorStyle.Render(cursorChar))
		}
		if col < len(vlRunes) {
			prevCol = col + 1
		} else {
			prevCol = col
		}
	}
	if prevCol < len(vlRunes) {
		segment := string(vlRunes[prevCol:])
		result.WriteString(highlightLine(segment, contentLine, fmStart, fmEnd, codeBlocks))
	}

	lineContent := result.String()
	if e.highlightCurrentLine && contentLine == e.cursor {
		lineContent = lipgloss.NewStyle().Background(surface0).Width(maxWidth).Render(lineContent)
	}
	return lineContent
}

// highlightLineWithBrackets renders a line segment with syntax highlighting and
// bracket matching highlights. segOffset is the column offset of this segment
// within the original line (used to map bracket positions).
func (e *Editor) highlightLineWithBrackets(segment string, lineIdx, segOffset, fmStart, fmEnd int, codeBlocks map[int]string, bStyle lipgloss.Style) string {
	if !e.matchBracketValid {
		return highlightLine(segment, lineIdx, fmStart, fmEnd, codeBlocks)
	}

	// Determine which positions on this line need bracket highlighting
	var bracketPositions []int
	// Check if the matching bracket is on this line
	if lineIdx == e.matchBracketLine {
		relCol := e.matchBracketCol - segOffset
		if relCol >= 0 && relCol < len(segment) {
			bracketPositions = append(bracketPositions, relCol)
		}
	}
	// Check if the source bracket (under cursor) is on this line
	if lineIdx == e.cursor && e.cursor < len(e.content) {
		cl := e.content[e.cursor]
		srcCol := -1
		if e.col < len(cl) && isBracketChar(cl[e.col]) {
			srcCol = e.col
		} else if e.col > 0 && e.col-1 < len(cl) && isBracketChar(cl[e.col-1]) {
			srcCol = e.col - 1
		}
		if srcCol >= 0 {
			relCol := srcCol - segOffset
			if relCol >= 0 && relCol < len(segment) {
				found := false
				for _, p := range bracketPositions {
					if p == relCol {
						found = true
						break
					}
				}
				if !found {
					bracketPositions = append(bracketPositions, relCol)
				}
			}
		}
	}

	if len(bracketPositions) == 0 {
		return highlightLine(segment, lineIdx, fmStart, fmEnd, codeBlocks)
	}

	// Sort bracket positions
	sort.Ints(bracketPositions)

	// Build the result by splitting at bracket positions
	var result strings.Builder
	prev := 0
	for _, bp := range bracketPositions {
		if bp > prev {
			result.WriteString(highlightLine(segment[prev:bp], lineIdx, fmStart, fmEnd, codeBlocks))
		}
		result.WriteString(bStyle.Render(string(segment[bp])))
		prev = bp + 1
	}
	if prev < len(segment) {
		result.WriteString(highlightLine(segment[prev:], lineIdx, fmStart, fmEnd, codeBlocks))
	}
	return result.String()
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
			e.rebuildBlockCaches()
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
	e.rebuildBlockCaches()
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
	e.rebuildBlockCaches()
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

// leadingWhitespace returns the leading whitespace (spaces and tabs) of a line.
func leadingWhitespace(line string) string {
	for i, r := range line {
		if r != ' ' && r != '\t' {
			return line[:i]
		}
	}
	return line
}
