package tui

import (
	"strings"
	"unicode"
)

// VimMode represents the current editing mode in Vim emulation.
type VimMode int

const (
	VimNormal  VimMode = iota
	VimInsert
	VimVisual
	VimCommand // : command line
)

// VimState holds all state for Vim-style modal editing.
type VimState struct {
	mode    VimMode
	enabled bool

	// Normal mode
	count    int    // numeric prefix (e.g. 5j = move down 5)
	pending  string // operator pending (d, c, y)
	register string // yank register content

	// Visual mode
	visualStart int // line where visual selection started
	visualCol   int // col where visual selection started

	// Command mode
	cmdBuf string // : command buffer

	// Search
	searchBuf     string
	searchForward bool

	// Last action for dot repeat
	lastAction string
	lastCount  int
}

// VimResult is returned from HandleKey to tell the editor what to do.
type VimResult struct {
	// Cursor movement
	NewCursor int
	NewCol    int
	CursorSet bool

	// Content changes
	DeleteLine  bool
	DeleteRange [2]int // start, end line (inclusive)
	YankRange   [2]int
	InsertLine  string // insert a new line after cursor
	JoinLine    bool

	// Mode changes
	EnterInsert  bool
	EnterNormal  bool
	EnterVisual  bool
	EnterCommand bool

	// Scroll
	ScrollTo  int
	ScrollSet bool

	// Pass through to editor (for insert mode keys)
	PassThrough bool

	// Status message
	StatusMsg string

	// Undo/Redo
	Undo bool
	Redo bool

	// Clipboard
	PasteBelow bool
	PasteAbove bool
	PasteText  string

	// Folding
	FoldToggle bool
	FoldAll    bool
	UnfoldAll  bool
}

// NewVimState creates a new VimState in Normal mode, disabled by default.
func NewVimState() *VimState {
	return &VimState{
		mode: VimNormal,
	}
}

// SetEnabled enables or disables Vim mode.
func (vs *VimState) SetEnabled(enabled bool) {
	vs.enabled = enabled
	if enabled {
		vs.mode = VimNormal
	}
}

// IsEnabled reports whether Vim mode is active.
func (vs *VimState) IsEnabled() bool {
	return vs.enabled
}

// Mode returns the current Vim mode.
func (vs *VimState) Mode() VimMode {
	return vs.mode
}

// ModeString returns a human-readable label for the current mode.
func (vs *VimState) ModeString() string {
	switch vs.mode {
	case VimNormal:
		return "NORMAL"
	case VimInsert:
		return "INSERT"
	case VimVisual:
		return "VISUAL"
	case VimCommand:
		return "COMMAND"
	default:
		return "NORMAL"
	}
}

// HandleKey processes a single key event and returns a VimResult describing
// what the editor should do. content is the current document lines, cursor is
// the current line index, col is the column, and height is the visible editor
// height (used for page movement).
func (vs *VimState) HandleKey(key string, content []string, cursor, col, height int) VimResult {
	if !vs.enabled {
		return VimResult{PassThrough: true}
	}

	switch vs.mode {
	case VimInsert:
		return vs.handleInsert(key)
	case VimVisual:
		return vs.handleVisual(key, content, cursor, col, height)
	case VimCommand:
		return vs.handleCommand(key, content, cursor)
	default:
		return vs.handleNormal(key, content, cursor, col, height)
	}
}

// ---------------------------------------------------------------------------
// Insert mode
// ---------------------------------------------------------------------------

func (vs *VimState) handleInsert(key string) VimResult {
	if key == "esc" || key == "escape" {
		vs.mode = VimNormal
		return VimResult{EnterNormal: true}
	}
	return VimResult{PassThrough: true}
}

// ---------------------------------------------------------------------------
// Command mode
// ---------------------------------------------------------------------------

func (vs *VimState) handleCommand(key string, content []string, cursor int) VimResult {
	switch key {
	case "esc", "escape":
		vs.mode = VimNormal
		vs.cmdBuf = ""
		return VimResult{EnterNormal: true}
	case "enter":
		return vs.executeCommand(content, cursor)
	case "backspace":
		if len(vs.cmdBuf) > 0 {
			vs.cmdBuf = vs.cmdBuf[:len(vs.cmdBuf)-1]
		}
		if len(vs.cmdBuf) == 0 {
			vs.mode = VimNormal
			return VimResult{EnterNormal: true}
		}
		return VimResult{StatusMsg: ":" + vs.cmdBuf}
	default:
		if len(key) == 1 {
			vs.cmdBuf += key
		}
		return VimResult{StatusMsg: ":" + vs.cmdBuf}
	}
}

func (vs *VimState) executeCommand(content []string, cursor int) VimResult {
	cmd := strings.TrimSpace(vs.cmdBuf)
	vs.cmdBuf = ""
	vs.mode = VimNormal

	switch cmd {
	case "w":
		return VimResult{EnterNormal: true, StatusMsg: "save"}
	case "q":
		return VimResult{EnterNormal: true, StatusMsg: "quit"}
	case "wq":
		return VimResult{EnterNormal: true, StatusMsg: "save_quit"}
	default:
		// Check for :{number} — go to line
		lineNum := parseNumber(cmd)
		if lineNum > 0 {
			target := lineNum - 1 // 1-indexed to 0-indexed
			if target >= len(content) {
				target = len(content) - 1
			}
			return VimResult{
				EnterNormal: true,
				NewCursor:   target,
				NewCol:      0,
				CursorSet:   true,
			}
		}
		return VimResult{EnterNormal: true, StatusMsg: "unknown command: " + cmd}
	}
}

// ---------------------------------------------------------------------------
// Visual mode
// ---------------------------------------------------------------------------

func (vs *VimState) handleVisual(key string, content []string, cursor, col, height int) VimResult {
	maxLine := len(content) - 1
	if maxLine < 0 {
		maxLine = 0
	}

	switch key {
	case "esc", "escape":
		vs.mode = VimNormal
		return VimResult{EnterNormal: true}

	// Movement keys extend the selection
	case "j":
		nc := cursor + 1
		if nc > maxLine {
			nc = maxLine
		}
		return VimResult{NewCursor: nc, NewCol: col, CursorSet: true}
	case "k":
		nc := cursor - 1
		if nc < 0 {
			nc = 0
		}
		return VimResult{NewCursor: nc, NewCol: col, CursorSet: true}
	case "h":
		nc := col - 1
		if nc < 0 {
			nc = 0
		}
		return VimResult{NewCursor: cursor, NewCol: nc, CursorSet: true}
	case "l":
		nc := col + 1
		lineLen := lineLength(content, cursor)
		if nc >= lineLen {
			nc = lineLen - 1
			if nc < 0 {
				nc = 0
			}
		}
		return VimResult{NewCursor: cursor, NewCol: nc, CursorSet: true}
	case "G":
		return VimResult{NewCursor: maxLine, NewCol: col, CursorSet: true}
	case "0":
		return VimResult{NewCursor: cursor, NewCol: 0, CursorSet: true}
	case "$":
		endCol := lineLength(content, cursor) - 1
		if endCol < 0 {
			endCol = 0
		}
		return VimResult{NewCursor: cursor, NewCol: endCol, CursorSet: true}

	// Operations on selection
	case "d", "x":
		startLine := vs.visualStart
		endLine := cursor
		if startLine > endLine {
			startLine, endLine = endLine, startLine
		}
		vs.register = joinRange(content, startLine, endLine)
		vs.mode = VimNormal
		return VimResult{
			EnterNormal: true,
			DeleteRange: [2]int{startLine, endLine},
		}
	case "y":
		startLine := vs.visualStart
		endLine := cursor
		if startLine > endLine {
			startLine, endLine = endLine, startLine
		}
		vs.register = joinRange(content, startLine, endLine)
		vs.mode = VimNormal
		return VimResult{
			EnterNormal: true,
			YankRange:   [2]int{startLine, endLine},
			StatusMsg:   "yanked",
		}
	}

	return VimResult{}
}

// ---------------------------------------------------------------------------
// Normal mode
// ---------------------------------------------------------------------------

func (vs *VimState) handleNormal(key string, content []string, cursor, col, height int) VimResult {
	maxLine := len(content) - 1
	if maxLine < 0 {
		maxLine = 0
	}

	// --- Numeric prefix accumulation ---
	if isDigit(key) {
		digit := int(key[0] - '0')
		if digit == 0 && vs.count == 0 {
			// "0" with no pending count means go to start of line
			return vs.motionResult(content, cursor, 0, cursor)
		}
		vs.count = vs.count*10 + digit
		return VimResult{StatusMsg: vimIntToStr(vs.count)}
	}

	// Resolve count (default 1)
	count := vs.count
	if count == 0 {
		count = 1
	}

	// Save for dot repeat later (before clearing)
	actionCount := count

	// Clear count after consuming
	vs.count = 0

	// --- Operator pending mode (d, c, y followed by motion/text-object) ---
	if vs.pending != "" {
		return vs.handlePending(key, content, cursor, col, count)
	}

	switch key {
	// ---- Movement ----
	case "h":
		newCol := col - count
		if newCol < 0 {
			newCol = 0
		}
		return vs.motionResult(content, cursor, newCol, cursor)
	case "j":
		nc := cursor + count
		if nc > maxLine {
			nc = maxLine
		}
		return vs.motionResult(content, nc, col, nc)
	case "k":
		nc := cursor - count
		if nc < 0 {
			nc = 0
		}
		return vs.motionResult(content, nc, col, nc)
	case "l":
		newCol := col + count
		lineLen := lineLength(content, cursor)
		if newCol >= lineLen {
			newCol = lineLen - 1
			if newCol < 0 {
				newCol = 0
			}
		}
		return vs.motionResult(content, cursor, newCol, cursor)

	// ---- Word motions ----
	case "w":
		newCursor, newCol := wordForward(content, cursor, col, count)
		return vs.motionResult(content, newCursor, newCol, newCursor)
	case "b":
		newCursor, newCol := wordBackward(content, cursor, col, count)
		return vs.motionResult(content, newCursor, newCol, newCursor)
	case "e":
		newCursor, newCol := wordEnd(content, cursor, col, count)
		return vs.motionResult(content, newCursor, newCol, newCursor)

	// ---- Line motions ----
	case "$":
		endCol := lineLength(content, cursor) - 1
		if endCol < 0 {
			endCol = 0
		}
		return vs.motionResult(content, cursor, endCol, cursor)
	case "^":
		newCol := firstNonSpace(content, cursor)
		return vs.motionResult(content, cursor, newCol, cursor)

	// ---- File motions ----
	case "G":
		target := maxLine
		if actionCount > 1 || vs.lastRequestedCount(actionCount) {
			// {number}G goes to that line
			target = actionCount - 1
			if target < 0 {
				target = 0
			}
			if target > maxLine {
				target = maxLine
			}
		}
		return vs.motionResult(content, target, 0, target)
	case "g":
		// Accumulate for gg
		vs.pending = "g"
		return VimResult{}

	// ---- Screen motions ----
	case "ctrl+d":
		half := height / 2
		nc := cursor + half*count
		if nc > maxLine {
			nc = maxLine
		}
		return VimResult{
			NewCursor: nc,
			NewCol:    col,
			CursorSet: true,
			ScrollTo:  nc - height/2,
			ScrollSet: true,
		}
	case "ctrl+u":
		half := height / 2
		nc := cursor - half*count
		if nc < 0 {
			nc = 0
		}
		scrollTarget := nc - height/2
		if scrollTarget < 0 {
			scrollTarget = 0
		}
		return VimResult{
			NewCursor: nc,
			NewCol:    col,
			CursorSet: true,
			ScrollTo:  scrollTarget,
			ScrollSet: true,
		}
	case "H":
		// Top of visible screen — we don't know scroll, so just report intent
		return VimResult{NewCursor: 0, NewCol: col, CursorSet: true, StatusMsg: "screen_top"}
	case "M":
		mid := maxLine / 2
		return VimResult{NewCursor: mid, NewCol: col, CursorSet: true, StatusMsg: "screen_middle"}
	case "L":
		return VimResult{NewCursor: maxLine, NewCol: col, CursorSet: true, StatusMsg: "screen_bottom"}

	// ---- Enter insert mode ----
	case "i":
		vs.mode = VimInsert
		vs.lastAction = "i"
		vs.lastCount = actionCount
		return VimResult{EnterInsert: true}
	case "a":
		vs.mode = VimInsert
		newCol := col + 1
		lineLen := lineLength(content, cursor)
		if newCol > lineLen {
			newCol = lineLen
		}
		vs.lastAction = "a"
		vs.lastCount = actionCount
		return VimResult{EnterInsert: true, NewCursor: cursor, NewCol: newCol, CursorSet: true}
	case "I":
		vs.mode = VimInsert
		newCol := firstNonSpace(content, cursor)
		vs.lastAction = "I"
		vs.lastCount = actionCount
		return VimResult{EnterInsert: true, NewCursor: cursor, NewCol: newCol, CursorSet: true}
	case "A":
		vs.mode = VimInsert
		endCol := lineLength(content, cursor)
		vs.lastAction = "A"
		vs.lastCount = actionCount
		return VimResult{EnterInsert: true, NewCursor: cursor, NewCol: endCol, CursorSet: true}
	case "o":
		vs.mode = VimInsert
		vs.lastAction = "o"
		vs.lastCount = actionCount
		return VimResult{EnterInsert: true, InsertLine: "", NewCursor: cursor + 1, NewCol: 0, CursorSet: true}
	case "O":
		vs.mode = VimInsert
		vs.lastAction = "O"
		vs.lastCount = actionCount
		return VimResult{EnterInsert: true, InsertLine: "", NewCursor: cursor, NewCol: 0, CursorSet: true, PasteAbove: true}

	// ---- Delete ----
	case "d":
		vs.pending = "d"
		return VimResult{}
	case "D":
		// Delete from cursor to end of line — equivalent to d$
		if cursor < len(content) {
			vs.register = substringFrom(content[cursor], col)
		}
		vs.lastAction = "D"
		vs.lastCount = actionCount
		return VimResult{StatusMsg: "d$", DeleteRange: [2]int{-1, -1}}
	case "x":
		// Delete character under cursor
		if cursor < len(content) && col < lineLength(content, cursor) {
			vs.register = string([]rune(content[cursor])[col])
		}
		vs.lastAction = "x"
		vs.lastCount = actionCount
		return VimResult{StatusMsg: "delete_char"}

	// ---- Change ----
	case "c":
		vs.pending = "c"
		return VimResult{}
	case "C":
		// Change to end of line — equivalent to c$
		if cursor < len(content) {
			vs.register = substringFrom(content[cursor], col)
		}
		vs.mode = VimInsert
		vs.lastAction = "C"
		vs.lastCount = actionCount
		return VimResult{EnterInsert: true, StatusMsg: "c$", DeleteRange: [2]int{-1, -1}}

	// ---- Yank / Paste ----
	case "y":
		vs.pending = "y"
		return VimResult{}
	case "p":
		vs.lastAction = "p"
		vs.lastCount = actionCount
		return VimResult{PasteBelow: true, PasteText: vs.register}
	case "P":
		vs.lastAction = "P"
		vs.lastCount = actionCount
		return VimResult{PasteAbove: true, PasteText: vs.register}

	// ---- Undo / Redo ----
	case "u":
		return VimResult{Undo: true}
	case "ctrl+r":
		return VimResult{Redo: true}

	// ---- Join ----
	case "J":
		vs.lastAction = "J"
		vs.lastCount = actionCount
		return VimResult{JoinLine: true}

	// ---- Search ----
	case "/":
		vs.searchForward = true
		vs.searchBuf = ""
		return VimResult{StatusMsg: "/"}
	case "?":
		vs.searchForward = false
		vs.searchBuf = ""
		return VimResult{StatusMsg: "?"}
	case "n":
		dir := "next"
		if !vs.searchForward {
			dir = "prev"
		}
		return VimResult{StatusMsg: "search_" + dir + ":" + vs.searchBuf}
	case "N":
		dir := "prev"
		if !vs.searchForward {
			dir = "next"
		}
		return VimResult{StatusMsg: "search_" + dir + ":" + vs.searchBuf}

	// ---- Repeat ----
	case ".":
		if vs.lastAction != "" {
			return vs.HandleKey(vs.lastAction, content, cursor, col, height)
		}
		return VimResult{}

	// ---- Visual mode ----
	case "v", "V":
		vs.mode = VimVisual
		vs.visualStart = cursor
		vs.visualCol = col
		return VimResult{EnterVisual: true}

	// ---- Command mode ----
	case ":":
		vs.mode = VimCommand
		vs.cmdBuf = ""
		return VimResult{EnterCommand: true, StatusMsg: ":"}

	// ---- Folding (z-prefix) ----
	case "z":
		vs.pending = "z"
		return VimResult{}
	}

	return VimResult{}
}

// ---------------------------------------------------------------------------
// Operator-pending handling (d, c, y, g + motion)
// ---------------------------------------------------------------------------

func (vs *VimState) handlePending(key string, content []string, cursor, col, count int) VimResult {
	op := vs.pending
	vs.pending = ""

	maxLine := len(content) - 1
	if maxLine < 0 {
		maxLine = 0
	}

	// g-prefix motions
	if op == "g" {
		switch key {
		case "g":
			// gg — go to top, or {count}gg go to line
			target := 0
			if count > 1 {
				target = count - 1
				if target > maxLine {
					target = maxLine
				}
			}
			return vs.motionResult(content, target, 0, target)
		}
		// Unknown g-combo — ignore
		return VimResult{}
	}

	// z-prefix folding commands
	if op == "z" {
		switch key {
		case "a":
			return VimResult{FoldToggle: true}
		case "M":
			return VimResult{FoldAll: true}
		case "R":
			return VimResult{UnfoldAll: true}
		}
		vs.pending = ""
		return VimResult{}
	}

	switch key {
	case "d":
		// dd — delete line(s)
		if op == "d" {
			endLine := cursor + count - 1
			if endLine > maxLine {
				endLine = maxLine
			}
			vs.register = joinRange(content, cursor, endLine)
			vs.lastAction = "dd"
			vs.lastCount = count
			if cursor == endLine {
				return VimResult{DeleteLine: true}
			}
			return VimResult{DeleteRange: [2]int{cursor, endLine}}
		}
	case "c":
		// cc — change line
		if op == "c" {
			endLine := cursor + count - 1
			if endLine > maxLine {
				endLine = maxLine
			}
			vs.register = joinRange(content, cursor, endLine)
			vs.mode = VimInsert
			vs.lastAction = "cc"
			vs.lastCount = count
			return VimResult{
				DeleteRange: [2]int{cursor, endLine},
				EnterInsert: true,
				InsertLine:  "",
			}
		}
	case "y":
		// yy — yank line(s)
		if op == "y" {
			endLine := cursor + count - 1
			if endLine > maxLine {
				endLine = maxLine
			}
			vs.register = joinRange(content, cursor, endLine)
			vs.lastAction = "yy"
			vs.lastCount = count
			return VimResult{
				YankRange: [2]int{cursor, endLine},
				StatusMsg: "yanked",
			}
		}

	case "w":
		// dw, cw, yw — operate on word
		newCursor, newCol := wordForward(content, cursor, col, count)
		switch op {
		case "d":
			vs.lastAction = "dw"
			vs.lastCount = count
			if newCursor == cursor {
				// Same line — delete from col to newCol
				return VimResult{StatusMsg: "delete_word"}
			}
			return VimResult{StatusMsg: "delete_word"}
		case "c":
			vs.mode = VimInsert
			vs.lastAction = "cw"
			vs.lastCount = count
			return VimResult{EnterInsert: true, StatusMsg: "change_word"}
		case "y":
			vs.register = extractWordRange(content, cursor, col, newCursor, newCol)
			vs.lastAction = "yw"
			vs.lastCount = count
			return VimResult{StatusMsg: "yanked"}
		}

	case "$":
		// d$, c$, y$ — operate to end of line
		switch op {
		case "d":
			if cursor < len(content) {
				vs.register = substringFrom(content[cursor], col)
			}
			vs.lastAction = "d$"
			vs.lastCount = count
			return VimResult{StatusMsg: "d$", DeleteRange: [2]int{-1, -1}}
		case "c":
			if cursor < len(content) {
				vs.register = substringFrom(content[cursor], col)
			}
			vs.mode = VimInsert
			vs.lastAction = "c$"
			vs.lastCount = count
			return VimResult{EnterInsert: true, StatusMsg: "c$", DeleteRange: [2]int{-1, -1}}
		case "y":
			if cursor < len(content) {
				vs.register = substringFrom(content[cursor], col)
			}
			vs.lastAction = "y$"
			vs.lastCount = count
			return VimResult{StatusMsg: "yanked"}
		}

	case "j":
		// dj, cj, yj — operate downward
		endLine := cursor + count
		if endLine > maxLine {
			endLine = maxLine
		}
		switch op {
		case "d":
			vs.register = joinRange(content, cursor, endLine)
			vs.lastAction = "dj"
			vs.lastCount = count
			return VimResult{DeleteRange: [2]int{cursor, endLine}}
		case "c":
			vs.register = joinRange(content, cursor, endLine)
			vs.mode = VimInsert
			vs.lastAction = "cj"
			vs.lastCount = count
			return VimResult{DeleteRange: [2]int{cursor, endLine}, EnterInsert: true, InsertLine: ""}
		case "y":
			vs.register = joinRange(content, cursor, endLine)
			vs.lastAction = "yj"
			vs.lastCount = count
			return VimResult{YankRange: [2]int{cursor, endLine}, StatusMsg: "yanked"}
		}

	case "k":
		// dk, ck, yk — operate upward
		startLine := cursor - count
		if startLine < 0 {
			startLine = 0
		}
		switch op {
		case "d":
			vs.register = joinRange(content, startLine, cursor)
			vs.lastAction = "dk"
			vs.lastCount = count
			return VimResult{DeleteRange: [2]int{startLine, cursor}, NewCursor: startLine, CursorSet: true}
		case "c":
			vs.register = joinRange(content, startLine, cursor)
			vs.mode = VimInsert
			vs.lastAction = "ck"
			vs.lastCount = count
			return VimResult{
				DeleteRange: [2]int{startLine, cursor},
				EnterInsert: true,
				InsertLine:  "",
				NewCursor:   startLine,
				CursorSet:   true,
			}
		case "y":
			vs.register = joinRange(content, startLine, cursor)
			vs.lastAction = "yk"
			vs.lastCount = count
			return VimResult{YankRange: [2]int{startLine, cursor}, StatusMsg: "yanked"}
		}

	case "G":
		// dG, cG, yG — operate to end of file
		switch op {
		case "d":
			vs.register = joinRange(content, cursor, maxLine)
			vs.lastAction = "dG"
			vs.lastCount = count
			return VimResult{DeleteRange: [2]int{cursor, maxLine}}
		case "c":
			vs.register = joinRange(content, cursor, maxLine)
			vs.mode = VimInsert
			vs.lastAction = "cG"
			vs.lastCount = count
			return VimResult{DeleteRange: [2]int{cursor, maxLine}, EnterInsert: true, InsertLine: ""}
		case "y":
			vs.register = joinRange(content, cursor, maxLine)
			vs.lastAction = "yG"
			vs.lastCount = count
			return VimResult{YankRange: [2]int{cursor, maxLine}, StatusMsg: "yanked"}
		}

	case "g":
		// dgg, cgg, ygg — operate to start of file
		// Set pending again so we can capture the second g
		vs.pending = op
		return VimResult{StatusMsg: op + "g"}
	}

	// Unknown second key — cancel pending
	return VimResult{}
}

// ---------------------------------------------------------------------------
// Word motion helpers
// ---------------------------------------------------------------------------

// wordForward moves to the start of the next word, count times.
func wordForward(content []string, cursor, col, count int) (int, int) {
	line := cursor
	c := col
	for n := 0; n < count; n++ {
		line, c = nextWord(content, line, c)
	}
	return line, c
}

// wordBackward moves to the start of the previous word, count times.
func wordBackward(content []string, cursor, col, count int) (int, int) {
	line := cursor
	c := col
	for n := 0; n < count; n++ {
		line, c = prevWord(content, line, c)
	}
	return line, c
}

// wordEnd moves to the end of the current/next word, count times.
func wordEnd(content []string, cursor, col, count int) (int, int) {
	line := cursor
	c := col
	for n := 0; n < count; n++ {
		line, c = endOfWord(content, line, c)
	}
	return line, c
}

// nextWord finds the start of the next word from (line, col).
func nextWord(content []string, line, col int) (int, int) {
	if line >= len(content) {
		return line, col
	}
	runes := []rune(content[line])

	// Skip current word characters
	i := col
	for i < len(runes) && isWordChar(runes[i]) {
		i++
	}
	// Skip whitespace / non-word
	for i < len(runes) && !isWordChar(runes[i]) {
		i++
	}
	if i < len(runes) {
		return line, i
	}
	// Move to next line
	if line+1 < len(content) {
		nextLine := line + 1
		runes2 := []rune(content[nextLine])
		j := 0
		for j < len(runes2) && isSpace(runes2[j]) {
			j++
		}
		return nextLine, j
	}
	return line, len(runes)
}

// prevWord finds the start of the previous word from (line, col).
func prevWord(content []string, line, col int) (int, int) {
	if line >= len(content) {
		return line, col
	}
	runes := []rune(content[line])

	i := col - 1
	if i < 0 {
		// Move to end of previous line
		if line > 0 {
			prevLine := line - 1
			prevRunes := []rune(content[prevLine])
			j := len(prevRunes) - 1
			for j >= 0 && !isWordChar(prevRunes[j]) {
				j--
			}
			for j > 0 && isWordChar(prevRunes[j-1]) {
				j--
			}
			if j < 0 {
				j = 0
			}
			return prevLine, j
		}
		return 0, 0
	}

	// Skip whitespace / non-word going backward
	for i >= 0 && !isWordChar(runes[i]) {
		i--
	}
	// Now on a word char — go to start of this word
	for i > 0 && isWordChar(runes[i-1]) {
		i--
	}
	if i < 0 {
		i = 0
	}
	return line, i
}

// endOfWord finds the end of the current or next word.
func endOfWord(content []string, line, col int) (int, int) {
	if line >= len(content) {
		return line, col
	}
	runes := []rune(content[line])

	i := col + 1
	// Skip non-word chars
	for i < len(runes) && !isWordChar(runes[i]) {
		i++
	}
	// Move to end of this word
	for i < len(runes)-1 && isWordChar(runes[i+1]) {
		i++
	}
	if i < len(runes) {
		return line, i
	}
	// Wrap to next line
	if line+1 < len(content) {
		nextLine := line + 1
		runes2 := []rune(content[nextLine])
		j := 0
		for j < len(runes2) && !isWordChar(runes2[j]) {
			j++
		}
		for j < len(runes2)-1 && isWordChar(runes2[j+1]) {
			j++
		}
		return nextLine, j
	}
	endCol := len(runes) - 1
	if endCol < 0 {
		endCol = 0
	}
	return line, endCol
}

// ---------------------------------------------------------------------------
// Utility helpers
// ---------------------------------------------------------------------------

func (vs *VimState) motionResult(content []string, newCursor, newCol, line int) VimResult {
	// Clamp col to line length
	lineLen := lineLength(content, newCursor)
	if newCol >= lineLen && lineLen > 0 {
		newCol = lineLen - 1
	}
	if newCol < 0 {
		newCol = 0
	}
	return VimResult{
		NewCursor: newCursor,
		NewCol:    newCol,
		CursorSet: true,
	}
}

// lastRequestedCount returns true if the user typed an explicit count before
// the key. We detect this by checking if actionCount != 1 OR if vs.count was
// explicitly set. Since we already resolved it, we just check count > 1.
func (vs *VimState) lastRequestedCount(count int) bool {
	// This is called after count resolution. If count > 1, user typed a
	// number. We can't distinguish "1G" from plain "G" easily, but treating
	// plain G as "go to last line" and 1G as "go to line 1" is standard.
	return count > 1
}

func lineLength(content []string, line int) int {
	if line < 0 || line >= len(content) {
		return 0
	}
	return len([]rune(content[line]))
}

func firstNonSpace(content []string, line int) int {
	if line < 0 || line >= len(content) {
		return 0
	}
	runes := []rune(content[line])
	for i, r := range runes {
		if !unicode.IsSpace(r) {
			return i
		}
	}
	return 0
}

func joinRange(content []string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end >= len(content) {
		end = len(content) - 1
	}
	if start > end {
		return ""
	}
	return strings.Join(content[start:end+1], "\n")
}

func substringFrom(line string, col int) string {
	runes := []rune(line)
	if col >= len(runes) {
		return ""
	}
	return string(runes[col:])
}

func extractWordRange(content []string, startLine, startCol, endLine, endCol int) string {
	if startLine == endLine && startLine < len(content) {
		runes := []rune(content[startLine])
		if startCol >= len(runes) {
			return ""
		}
		end := endCol
		if end > len(runes) {
			end = len(runes)
		}
		return string(runes[startCol:end])
	}
	// Multi-line: just join the lines
	return joinRange(content, startLine, endLine)
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func isSpace(r rune) bool {
	return unicode.IsSpace(r)
}

func isDigit(key string) bool {
	return len(key) == 1 && key[0] >= '0' && key[0] <= '9'
}

// parseNumber parses a decimal integer from a string, returning 0 on failure.
func parseNumber(s string) int {
	if len(s) == 0 {
		return 0
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// vimIntToStr converts a non-negative integer to its string representation.
// (intToStr already exists in settings.go, so we use a separate name here.)
func vimIntToStr(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	// Reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
