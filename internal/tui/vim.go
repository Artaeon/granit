package tui

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

// VimMode represents the current editing mode in Vim emulation.
type VimMode int

const (
	VimNormal  VimMode = iota
	VimInsert
	VimVisual
	VimCommand // : command line
	VimSearch  // / or ? search input
)

// SearchMatch represents a single search match position in the document.
type SearchMatch struct {
	Line     int
	StartCol int
	EndCol   int // exclusive
}

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
	searchBuf      string
	searchForward  bool
	searchMatches  []SearchMatch
	currentMatch   int // index into searchMatches for the current/active match
	searchActive   bool // true when search highlights should be shown

	// Text object pending (i/a prefix after operator)
	textObjPending string // "i" or "a" when waiting for text object char

	// Marks
	marks    map[byte]CursorPos // a-z marks
	prevMark CursorPos          // '' previous position (before last jump)

	// Last action for dot repeat
	lastAction string
	lastCount  int

	// Macro recording and replay
	macros            map[byte][]tea.KeyMsg // stored macros per register (a-z)
	recording         bool                  // whether currently recording
	recordRegister    byte                  // which register is being recorded into
	recordBuffer      []tea.KeyMsg          // current recording buffer
	lastMacroRegister byte                  // for @@ replay
	playingMacro      bool                  // prevent recursive macro execution
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

	// Macro recording/replay
	MacroStart  byte // non-zero means start recording into this register
	MacroStop   bool // stop recording current macro
	MacroReplay byte // non-zero means replay macro from this register

	// Text object operations (inline edit within/across lines)
	TextOp         string // "delete", "change", "yank" — for inline text object ops
	TextOpStartLine int
	TextOpStartCol  int
	TextOpEndLine   int
	TextOpEndCol    int

	// Ex command results
	ExOpenFile    string // :e <file> — filename to open (fuzzy matched by app)
	ExForceQuit   bool   // :q! — quit without saving
	ExSetOption   string // :set option — "number", "nonumber", "wrap", "nowrap"
	ExClearSearch bool   // :noh — clear search highlights
	ExSubstitute  *ExSubResult // :s or :%s substitution result
}

// ExSubResult holds the result of an ex :s substitution command.
type ExSubResult struct {
	NewLines []string // updated content lines
	Count    int      // number of replacements made
}

// vimMacroReplayMsg is a tea.Msg used to replay macro keystrokes one at a time.
type vimMacroReplayMsg struct {
	keys []tea.KeyMsg
	idx  int
}

// NewVimState creates a new VimState in Normal mode, disabled by default.
func NewVimState() *VimState {
	return &VimState{
		mode:   VimNormal,
		macros: make(map[byte][]tea.KeyMsg),
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
	case VimSearch:
		return "SEARCH"
	default:
		return "NORMAL"
	}
}

// RecordingStatus returns a status string shown while macro recording is active.
// Returns "" when not recording, or "recording @a" when recording register 'a'.
func (vs *VimState) RecordingStatus() string {
	if !vs.recording {
		return ""
	}
	return fmt.Sprintf("recording @%c", vs.recordRegister)
}

// IsRecording reports whether a macro is currently being recorded.
func (vs *VimState) IsRecording() bool {
	return vs.recording
}

// IsPlayingMacro reports whether a macro is currently being replayed.
func (vs *VimState) IsPlayingMacro() bool {
	return vs.playingMacro
}

// StartRecording begins recording keystrokes into the given register (a-z).
func (vs *VimState) StartRecording(reg byte) {
	vs.recording = true
	vs.recordRegister = reg
	vs.recordBuffer = nil
}

// StopRecording finishes recording and saves the buffer into the register.
func (vs *VimState) StopRecording() {
	if !vs.recording {
		return
	}
	buf := vs.recordBuffer
	if buf == nil {
		buf = []tea.KeyMsg{}
	}
	vs.macros[vs.recordRegister] = buf
	vs.recording = false
	vs.recordBuffer = nil
}

// RecordKey appends a key message to the current recording buffer.
func (vs *VimState) RecordKey(msg tea.KeyMsg) {
	if vs.recording && !vs.playingMacro {
		vs.recordBuffer = append(vs.recordBuffer, msg)
	}
}

// GetMacro returns the recorded keystrokes for a register, or nil if empty.
func (vs *VimState) GetMacro(reg byte) []tea.KeyMsg {
	keys, ok := vs.macros[reg]
	if !ok {
		return nil
	}
	return keys
}

// SetPlayingMacro sets the playingMacro flag to prevent recursive execution.
func (vs *VimState) SetPlayingMacro(playing bool) {
	vs.playingMacro = playing
}

// LastMacroRegister returns the register used for the most recent @x replay.
func (vs *VimState) LastMacroRegister() byte {
	return vs.lastMacroRegister
}

// SetLastMacroRegister stores which register was last replayed (for @@).
func (vs *VimState) SetLastMacroRegister(reg byte) {
	vs.lastMacroRegister = reg
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
	case VimSearch:
		return vs.handleSearch(key, content, cursor, col)
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
	case "wq", "x":
		return VimResult{EnterNormal: true, StatusMsg: "save_quit"}
	case "q!":
		return VimResult{EnterNormal: true, ExForceQuit: true}
	case "e":
		return VimResult{EnterNormal: true, StatusMsg: "no file name"}
	case "noh", "nohlsearch":
		vs.searchActive = false
		vs.searchMatches = nil
		vs.currentMatch = -1
		return VimResult{EnterNormal: true, ExClearSearch: true}
	case "set number":
		return VimResult{EnterNormal: true, ExSetOption: "number"}
	case "set nonumber", "set nonu":
		return VimResult{EnterNormal: true, ExSetOption: "nonumber"}
	case "set wrap":
		return VimResult{EnterNormal: true, ExSetOption: "wrap"}
	case "set nowrap":
		return VimResult{EnterNormal: true, ExSetOption: "nowrap"}
	default:
		// :e <file> — open file
		if strings.HasPrefix(cmd, "e ") {
			filename := strings.TrimSpace(cmd[2:])
			if filename != "" {
				return VimResult{EnterNormal: true, ExOpenFile: filename}
			}
			return VimResult{EnterNormal: true, StatusMsg: "no file name"}
		}

		// Substitution: :s/old/new/[g] or :%s/old/new/[g]
		if sub := parseSubstitution(cmd, content, cursor); sub != nil {
			return *sub
		}

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

// parseSubstitution handles :s/old/new/[g] and :%s/old/new/[g] commands.
func parseSubstitution(cmd string, content []string, cursor int) *VimResult {
	allLines := false
	sub := cmd

	// :%s — substitute across entire file
	if strings.HasPrefix(sub, "%s") {
		allLines = true
		sub = sub[1:] // strip %, now starts with "s"
	}

	// Must start with s followed by a delimiter
	if len(sub) < 2 || sub[0] != 's' {
		return nil
	}

	delim := sub[1]
	// Parse s/old/new/[flags] using the delimiter character
	parts := splitSubParts(sub[2:], delim)
	if len(parts) < 2 {
		return nil
	}

	old := parts[0]
	new := parts[1]
	flags := ""
	if len(parts) >= 3 {
		flags = parts[2]
	}

	if old == "" {
		return &VimResult{EnterNormal: true, StatusMsg: "empty search pattern"}
	}

	globalReplace := strings.Contains(flags, "g")

	// Copy content to avoid modifying the original
	newLines := make([]string, len(content))
	copy(newLines, content)

	count := 0
	startLine := cursor
	endLine := cursor
	if allLines {
		startLine = 0
		endLine = len(content) - 1
	}

	for i := startLine; i <= endLine; i++ {
		if globalReplace {
			n := strings.Count(newLines[i], old)
			if n > 0 {
				newLines[i] = strings.ReplaceAll(newLines[i], old, new)
				count += n
			}
		} else {
			idx := strings.Index(newLines[i], old)
			if idx >= 0 {
				newLines[i] = newLines[i][:idx] + new + newLines[i][idx+len(old):]
				count++
			}
		}
	}

	if count == 0 {
		return &VimResult{EnterNormal: true, StatusMsg: "pattern not found: " + old}
	}

	return &VimResult{
		EnterNormal:  true,
		ExSubstitute: &ExSubResult{NewLines: newLines, Count: count},
		StatusMsg:    fmt.Sprintf("%d substitution(s)", count),
	}
}

// splitSubParts splits a substitution body by the delimiter.
// e.g. for "old/new/g" with delim '/', returns ["old", "new", "g"].
func splitSubParts(s string, delim byte) []string {
	var parts []string
	var cur strings.Builder
	escaped := false
	for i := 0; i < len(s); i++ {
		if escaped {
			cur.WriteByte(s[i])
			escaped = false
			continue
		}
		if s[i] == '\\' {
			escaped = true
			continue
		}
		if s[i] == delim {
			parts = append(parts, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(s[i])
	}
	parts = append(parts, cur.String())
	return parts
}

// GetCmdBuffer returns the current command-line buffer for rendering.
// Returns empty string when not in command mode.
func (vs *VimState) GetCmdBuffer() string {
	if vs.mode != VimCommand {
		return ""
	}
	return vs.cmdBuf
}

// ---------------------------------------------------------------------------
// Visual mode
// ---------------------------------------------------------------------------

func (vs *VimState) handleVisual(key string, content []string, cursor, col, height int) VimResult {
	maxLine := len(content) - 1
	if maxLine < 0 {
		maxLine = 0
	}

	// Text object pending in visual mode (v + i/a + object)
	if vs.textObjPending != "" {
		inner := vs.textObjPending == "i"
		vs.textObjPending = ""
		vs.pending = ""

		// Resolve text object to update visual selection bounds
		var rng textObjectRange
		switch key {
		case "w":
			rng = textObjWord(content, cursor, col, inner)
		case "s":
			rng = textObjSentence(content, cursor, col, inner)
		case "p":
			rng = textObjParagraph(content, cursor, inner)
		case "\"":
			rng = textObjQuote(content, cursor, col, '"', inner)
		case "'":
			rng = textObjQuote(content, cursor, col, '\'', inner)
		case "`":
			rng = textObjQuote(content, cursor, col, '`', inner)
		case ")", "(", "b":
			rng = textObjPair(content, cursor, col, '(', ')', inner)
		case "}", "{", "B":
			rng = textObjPair(content, cursor, col, '{', '}', inner)
		case "]", "[":
			rng = textObjPair(content, cursor, col, '[', ']', inner)
		case ">", "<":
			rng = textObjPair(content, cursor, col, '<', '>', inner)
		default:
			return VimResult{}
		}

		if !rng.found {
			return VimResult{}
		}

		// Update visual selection to match text object
		vs.visualStart = rng.startLine
		vs.visualCol = rng.startCol
		endCol := rng.endCol - 1
		if endCol < 0 {
			endCol = 0
		}
		return VimResult{
			NewCursor: rng.endLine,
			NewCol:    endCol,
			CursorSet: true,
		}
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

	// Text object prefixes in visual mode
	case "i", "a":
		vs.textObjPending = key
		vs.pending = "v" // visual text object
		return VimResult{}

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

	// Esc in normal mode clears search highlights
	if key == "esc" || key == "escape" {
		if vs.searchActive {
			vs.searchActive = false
			vs.searchMatches = nil
			vs.currentMatch = -1
			return VimResult{StatusMsg: "search cleared"}
		}
		return VimResult{}
	}

	// --- Numeric prefix accumulation ---
	if isDigit(key) {
		digit := int(key[0] - '0')
		if digit == 0 && vs.count == 0 {
			// "0" with no pending count means go to start of line
			return vs.motionResult(content, cursor, 0, cursor)
		}
		vs.count = vs.count*10 + digit
		if vs.count > 10000 {
			vs.count = 10000
		}
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
		vs.mode = VimSearch
		return VimResult{StatusMsg: "/"}
	case "?":
		vs.searchForward = false
		vs.searchBuf = ""
		vs.mode = VimSearch
		return VimResult{StatusMsg: "?"}
	case "n":
		return vs.nextSearchMatch(content, cursor, col, true)
	case "N":
		return vs.nextSearchMatch(content, cursor, col, false)

	// ---- Repeat ----
	case ".":
		if vs.lastAction != "" {
			return vs.HandleKey(vs.lastAction, content, cursor, col, height)
		}
		return VimResult{}

	// ---- Marks ----
	case "m":
		vs.pending = "m"
		return VimResult{}
	case "'":
		vs.pending = "'"
		return VimResult{}
	case "`":
		vs.pending = "`"
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

	// ---- Macro recording ----
	case "q":
		if vs.recording {
			// Stop recording
			return VimResult{MacroStop: true, StatusMsg: "macro recorded"}
		}
		// Start recording: wait for register letter
		vs.pending = "q"
		return VimResult{}

	// ---- Macro replay ----
	case "@":
		// Wait for register letter or @ for last-used
		vs.pending = "@"
		return VimResult{}

	// ---- Folding (z-prefix) ----
	case "z":
		vs.pending = "z"
		return VimResult{}
	}

	return VimResult{}
}

// ---------------------------------------------------------------------------
// Search mode
// ---------------------------------------------------------------------------

func (vs *VimState) handleSearch(key string, content []string, cursor, col int) VimResult {
	switch key {
	case "esc", "escape":
		vs.mode = VimNormal
		vs.searchBuf = ""
		vs.searchActive = false
		vs.searchMatches = nil
		vs.currentMatch = -1
		return VimResult{EnterNormal: true}
	case "enter":
		vs.mode = VimNormal
		if vs.searchBuf == "" {
			vs.searchActive = false
			vs.searchMatches = nil
			return VimResult{EnterNormal: true}
		}
		vs.computeSearchMatches(content)
		if len(vs.searchMatches) == 0 {
			vs.searchActive = false
			return VimResult{
				EnterNormal: true,
				StatusMsg:   "Pattern not found: " + vs.searchBuf,
			}
		}
		vs.searchActive = true
		return vs.jumpToNearestMatch(content, cursor, col, vs.searchForward)
	case "backspace":
		if len(vs.searchBuf) > 0 {
			vs.searchBuf = vs.searchBuf[:len(vs.searchBuf)-1]
		}
		if len(vs.searchBuf) == 0 {
			prompt := "/"
			if !vs.searchForward {
				prompt = "?"
			}
			return VimResult{StatusMsg: prompt}
		}
		prompt := "/"
		if !vs.searchForward {
			prompt = "?"
		}
		return VimResult{StatusMsg: prompt + vs.searchBuf}
	default:
		if len(key) == 1 {
			vs.searchBuf += key
		}
		prompt := "/"
		if !vs.searchForward {
			prompt = "?"
		}
		return VimResult{StatusMsg: prompt + vs.searchBuf}
	}
}

func (vs *VimState) computeSearchMatches(content []string) {
	vs.searchMatches = nil
	vs.currentMatch = -1
	if vs.searchBuf == "" {
		return
	}
	pattern := strings.ToLower(vs.searchBuf)
	for lineIdx, line := range content {
		lower := strings.ToLower(line)
		offset := 0
		for {
			idx := strings.Index(lower[offset:], pattern)
			if idx < 0 {
				break
			}
			absIdx := offset + idx
			vs.searchMatches = append(vs.searchMatches, SearchMatch{
				Line:     lineIdx,
				StartCol: absIdx,
				EndCol:   absIdx + len(pattern),
			})
			offset = absIdx + 1
			if offset >= len(lower) {
				break
			}
		}
	}
}

func (vs *VimState) jumpToNearestMatch(content []string, cursor, col int, forward bool) VimResult {
	if len(vs.searchMatches) == 0 {
		return VimResult{EnterNormal: true, StatusMsg: "Pattern not found: " + vs.searchBuf}
	}
	bestIdx := -1
	if forward {
		for i, m := range vs.searchMatches {
			if m.Line > cursor || (m.Line == cursor && m.StartCol >= col) {
				bestIdx = i
				break
			}
		}
		if bestIdx == -1 {
			bestIdx = 0
		}
	} else {
		for i := len(vs.searchMatches) - 1; i >= 0; i-- {
			m := vs.searchMatches[i]
			if m.Line < cursor || (m.Line == cursor && m.StartCol <= col) {
				bestIdx = i
				break
			}
		}
		if bestIdx == -1 {
			bestIdx = len(vs.searchMatches) - 1
		}
	}
	vs.currentMatch = bestIdx
	m := vs.searchMatches[bestIdx]
	matchInfo := vimIntToStr(bestIdx+1) + "/" + vimIntToStr(len(vs.searchMatches))
	return VimResult{
		EnterNormal: true,
		NewCursor:   m.Line,
		NewCol:      m.StartCol,
		CursorSet:   true,
		StatusMsg:   "/" + vs.searchBuf + "  [" + matchInfo + "]",
	}
}

func (vs *VimState) nextSearchMatch(content []string, cursor, col int, sameDirection bool) VimResult {
	if !vs.searchActive || len(vs.searchMatches) == 0 {
		if vs.searchBuf != "" {
			vs.computeSearchMatches(content)
			if len(vs.searchMatches) == 0 {
				return VimResult{StatusMsg: "Pattern not found: " + vs.searchBuf}
			}
			vs.searchActive = true
		} else {
			return VimResult{StatusMsg: "No previous search"}
		}
	}
	forward := vs.searchForward
	if !sameDirection {
		forward = !forward
	}
	if forward {
		bestIdx := -1
		for i, m := range vs.searchMatches {
			if m.Line > cursor || (m.Line == cursor && m.StartCol > col) {
				bestIdx = i
				break
			}
		}
		if bestIdx == -1 {
			bestIdx = 0
		}
		vs.currentMatch = bestIdx
	} else {
		bestIdx := -1
		for i := len(vs.searchMatches) - 1; i >= 0; i-- {
			m := vs.searchMatches[i]
			if m.Line < cursor || (m.Line == cursor && m.StartCol < col) {
				bestIdx = i
				break
			}
		}
		if bestIdx == -1 {
			bestIdx = len(vs.searchMatches) - 1
		}
		vs.currentMatch = bestIdx
	}
	match := vs.searchMatches[vs.currentMatch]
	matchInfo := vimIntToStr(vs.currentMatch+1) + "/" + vimIntToStr(len(vs.searchMatches))
	return VimResult{
		NewCursor: match.Line,
		NewCol:    match.StartCol,
		CursorSet: true,
		StatusMsg: "/" + vs.searchBuf + "  [" + matchInfo + "]",
	}
}

func (vs *VimState) GetSearchMatches() []SearchMatch {
	if !vs.searchActive {
		return nil
	}
	return vs.searchMatches
}

func (vs *VimState) GetCurrentMatchIndex() int {
	return vs.currentMatch
}

func (vs *VimState) IsSearchActive() bool {
	return vs.searchActive
}

// ---------------------------------------------------------------------------
// Operator-pending handling (d, c, y, g, z, q, @ + motion)
// ---------------------------------------------------------------------------

func (vs *VimState) handlePending(key string, content []string, cursor, col, count int) VimResult {
	op := vs.pending
	vs.pending = ""

	maxLine := len(content) - 1
	if maxLine < 0 {
		maxLine = 0
	}

	// Text object pending (operator + i/a waiting for object char)
	if vs.textObjPending != "" {
		inner := vs.textObjPending == "i"
		vs.textObjPending = ""
		return vs.handleTextObject(key, inner, op, content, cursor, col, count)
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

	// q-prefix: start macro recording into register a-z
	if op == "q" {
		if len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
			return VimResult{MacroStart: key[0]}
		}
		return VimResult{}
	}

	// @-prefix: replay macro from register a-z, or @@ for last used
	if op == "@" {
		if len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
			return VimResult{MacroReplay: key[0]}
		}
		if key == "@" && vs.lastMacroRegister != 0 {
			return VimResult{MacroReplay: vs.lastMacroRegister}
		}
		return VimResult{}
	}

	// Mark commands
	if op == "m" {
		if len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
			if vs.marks == nil {
				vs.marks = make(map[byte]CursorPos)
			}
			vs.marks[key[0]] = CursorPos{Line: cursor, Col: col}
			return VimResult{StatusMsg: "mark " + key + " set"}
		}
		return VimResult{}
	}

	if op == "'" {
		if key == "'" {
			prev := vs.prevMark
			vs.prevMark = CursorPos{Line: cursor, Col: col}
			target := prev.Line
			if target >= len(content) {
				target = len(content) - 1
			}
			if target < 0 {
				target = 0
			}
			return VimResult{
				NewCursor: target,
				NewCol:    firstNonSpace(content, target),
				CursorSet: true,
			}
		}
		if len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
			if vs.marks == nil {
				return VimResult{StatusMsg: "mark not set"}
			}
			pos, ok := vs.marks[key[0]]
			if !ok {
				return VimResult{StatusMsg: "mark " + key + " not set"}
			}
			vs.prevMark = CursorPos{Line: cursor, Col: col}
			target := pos.Line
			if target >= len(content) {
				target = len(content) - 1
			}
			return VimResult{
				NewCursor: target,
				NewCol:    firstNonSpace(content, target),
				CursorSet: true,
			}
		}
		return VimResult{}
	}

	if op == "`" {
		if key == "`" {
			prev := vs.prevMark
			vs.prevMark = CursorPos{Line: cursor, Col: col}
			target := prev.Line
			if target >= len(content) {
				target = len(content) - 1
			}
			if target < 0 {
				target = 0
			}
			targetCol := prev.Col
			if target >= 0 && target < len(content) {
				lineLen := lineLength(content, target)
				if targetCol >= lineLen && lineLen > 0 {
					targetCol = lineLen - 1
				}
			}
			if targetCol < 0 {
				targetCol = 0
			}
			return VimResult{
				NewCursor: target,
				NewCol:    targetCol,
				CursorSet: true,
			}
		}
		if len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
			if vs.marks == nil {
				return VimResult{StatusMsg: "mark not set"}
			}
			pos, ok := vs.marks[key[0]]
			if !ok {
				return VimResult{StatusMsg: "mark " + key + " not set"}
			}
			vs.prevMark = CursorPos{Line: cursor, Col: col}
			target := pos.Line
			if target >= len(content) {
				target = len(content) - 1
			}
			targetCol := pos.Col
			if target >= 0 && target < len(content) {
				lineLen := lineLength(content, target)
				if targetCol >= lineLen && lineLen > 0 {
					targetCol = lineLen - 1
				}
			}
			if targetCol < 0 {
				targetCol = 0
			}
			return VimResult{
				NewCursor: target,
				NewCol:    targetCol,
				CursorSet: true,
			}
		}
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

	case "i", "a":
		// Text object prefix: di", da(, ci}, ya[, etc.
		if op == "d" || op == "c" || op == "y" {
			vs.textObjPending = key
			vs.pending = op
			return VimResult{}
		}
	}

	// Unknown second key — cancel pending
	return VimResult{}
}

// ---------------------------------------------------------------------------
// Text object handling
// ---------------------------------------------------------------------------

// textObjectRange represents a range found by a text object.
type textObjectRange struct {
	startLine, startCol int
	endLine, endCol     int
	found               bool
}

// handleTextObject processes a text object key (after operator + i/a prefix).
func (vs *VimState) handleTextObject(key string, inner bool, op string, content []string, cursor, col, count int) VimResult {
	var rng textObjectRange

	switch key {
	case "w":
		rng = textObjWord(content, cursor, col, inner)
	case "s":
		rng = textObjSentence(content, cursor, col, inner)
	case "p":
		rng = textObjParagraph(content, cursor, inner)
	case "\"":
		rng = textObjQuote(content, cursor, col, '"', inner)
	case "'":
		rng = textObjQuote(content, cursor, col, '\'', inner)
	case "`":
		rng = textObjQuote(content, cursor, col, '`', inner)
	case ")", "(", "b":
		rng = textObjPair(content, cursor, col, '(', ')', inner)
	case "}", "{", "B":
		rng = textObjPair(content, cursor, col, '{', '}', inner)
	case "]", "[":
		rng = textObjPair(content, cursor, col, '[', ']', inner)
	case ">", "<":
		rng = textObjPair(content, cursor, col, '<', '>', inner)
	default:
		return VimResult{}
	}

	if !rng.found {
		return VimResult{}
	}

	// Extract the text in the range for the register
	vs.register = extractRange(content, rng.startLine, rng.startCol, rng.endLine, rng.endCol)
	prefix := "i"
	if !inner {
		prefix = "a"
	}
	vs.lastAction = op + prefix + key
	vs.lastCount = count

	switch op {
	case "d":
		return VimResult{
			TextOp:          "delete",
			TextOpStartLine: rng.startLine,
			TextOpStartCol:  rng.startCol,
			TextOpEndLine:   rng.endLine,
			TextOpEndCol:    rng.endCol,
			NewCursor:       rng.startLine,
			NewCol:          rng.startCol,
			CursorSet:       true,
		}
	case "c":
		vs.mode = VimInsert
		return VimResult{
			TextOp:          "change",
			TextOpStartLine: rng.startLine,
			TextOpStartCol:  rng.startCol,
			TextOpEndLine:   rng.endLine,
			TextOpEndCol:    rng.endCol,
			NewCursor:       rng.startLine,
			NewCol:          rng.startCol,
			CursorSet:       true,
			EnterInsert:     true,
		}
	case "y":
		return VimResult{
			StatusMsg: "yanked",
		}
	}
	return VimResult{}
}

// textObjWord finds the word under cursor. inner = word chars only, around = include surrounding whitespace.
func textObjWord(content []string, cursor, col int, inner bool) textObjectRange {
	if cursor >= len(content) {
		return textObjectRange{}
	}
	runes := []rune(content[cursor])
	if len(runes) == 0 {
		return textObjectRange{}
	}
	if col >= len(runes) {
		col = len(runes) - 1
	}
	if col < 0 {
		return textObjectRange{}
	}

	start := col
	end := col

	if isWordChar(runes[col]) {
		// Expand to word boundaries
		for start > 0 && isWordChar(runes[start-1]) {
			start--
		}
		for end < len(runes)-1 && isWordChar(runes[end+1]) {
			end++
		}
		if !inner {
			// Include trailing whitespace
			for end+1 < len(runes) && isSpace(runes[end+1]) {
				end++
			}
			// If no trailing space, include leading whitespace
			if end == col {
				for start > 0 && isSpace(runes[start-1]) {
					start--
				}
			}
		}
	} else if isSpace(runes[col]) {
		// On whitespace: select whitespace block
		for start > 0 && isSpace(runes[start-1]) {
			start--
		}
		for end < len(runes)-1 && isSpace(runes[end+1]) {
			end++
		}
	} else {
		// On punctuation
		for start > 0 && !isWordChar(runes[start-1]) && !isSpace(runes[start-1]) {
			start--
		}
		for end < len(runes)-1 && !isWordChar(runes[end+1]) && !isSpace(runes[end+1]) {
			end++
		}
	}

	return textObjectRange{
		startLine: cursor, startCol: start,
		endLine: cursor, endCol: end + 1,
		found: true,
	}
}

// textObjSentence finds the sentence under cursor, delimited by .!? followed by space or EOL.
func textObjSentence(content []string, cursor, col int, inner bool) textObjectRange {
	if cursor >= len(content) {
		return textObjectRange{}
	}

	// Flatten content to find sentence boundaries
	// Work on the current line for simplicity (matching vim behavior for most cases)
	line := content[cursor]
	runes := []rune(line)
	if len(runes) == 0 {
		return textObjectRange{}
	}
	if col >= len(runes) {
		col = len(runes) - 1
	}

	// Find sentence start: scan backward for .!? followed by space, or start of line
	start := 0
	for i := col; i > 0; i-- {
		if isSentenceEnd(runes[i-1]) {
			// Check if the char after the punctuation is space
			if i < len(runes) && isSpace(runes[i]) {
				start = i
				break
			}
		}
	}

	// Skip leading whitespace for inner
	sentStart := start
	if inner {
		for sentStart < len(runes) && isSpace(runes[sentStart]) {
			sentStart++
		}
	}

	// Find sentence end: scan forward for .!?
	end := len(runes)
	for i := col; i < len(runes); i++ {
		if isSentenceEnd(runes[i]) {
			end = i + 1
			break
		}
	}

	// For around, include trailing whitespace
	sentEnd := end
	if !inner {
		for sentEnd < len(runes) && isSpace(runes[sentEnd]) {
			sentEnd++
		}
	}

	return textObjectRange{
		startLine: cursor, startCol: sentStart,
		endLine: cursor, endCol: sentEnd,
		found: true,
	}
}

// textObjParagraph finds the paragraph under cursor, delimited by blank lines.
func textObjParagraph(content []string, cursor int, inner bool) textObjectRange {
	if cursor >= len(content) {
		return textObjectRange{}
	}

	// Find paragraph start (scan up to blank line)
	start := cursor
	for start > 0 && strings.TrimSpace(content[start-1]) != "" {
		start--
	}

	// Find paragraph end (scan down to blank line)
	end := cursor
	for end < len(content)-1 && strings.TrimSpace(content[end+1]) != "" {
		end++
	}

	if inner {
		return textObjectRange{
			startLine: start, startCol: 0,
			endLine: end, endCol: len(content[end]),
			found: true,
		}
	}

	// Around: include trailing blank lines
	for end+1 < len(content) && strings.TrimSpace(content[end+1]) == "" {
		end++
	}

	return textObjectRange{
		startLine: start, startCol: 0,
		endLine: end, endCol: len(content[end]),
		found: true,
	}
}

// textObjQuote finds quoted text on the current line.
func textObjQuote(content []string, cursor, col int, quote rune, inner bool) textObjectRange {
	if cursor >= len(content) {
		return textObjectRange{}
	}
	runes := []rune(content[cursor])
	if len(runes) == 0 {
		return textObjectRange{}
	}
	if col >= len(runes) {
		col = len(runes) - 1
	}

	// Find the opening quote
	openIdx := -1
	closeIdx := -1

	// Strategy: find a pair of quotes that enclose the cursor position.
	// First, find all quote positions on this line.
	var quotePositions []int
	for i, r := range runes {
		if r == quote {
			quotePositions = append(quotePositions, i)
		}
	}

	// Find the pair that encloses cursor
	for i := 0; i+1 < len(quotePositions); i += 2 {
		o := quotePositions[i]
		c := quotePositions[i+1]
		if col >= o && col <= c {
			openIdx = o
			closeIdx = c
			break
		}
	}

	// If not found with simple pairing, try: cursor is on or after a quote,
	// find the next quote
	if openIdx == -1 {
		// Find closest opening quote at or before cursor
		for i := col; i >= 0; i-- {
			if runes[i] == quote {
				openIdx = i
				break
			}
		}
		if openIdx >= 0 {
			// Find closing quote after openIdx
			for i := openIdx + 1; i < len(runes); i++ {
				if runes[i] == quote {
					closeIdx = i
					break
				}
			}
		}
		// If cursor is before any quote, try forward
		if openIdx == -1 || closeIdx == -1 {
			openIdx = -1
			closeIdx = -1
			for i := col; i < len(runes); i++ {
				if runes[i] == quote {
					if openIdx == -1 {
						openIdx = i
					} else {
						closeIdx = i
						break
					}
				}
			}
		}
	}

	if openIdx == -1 || closeIdx == -1 || openIdx >= closeIdx {
		return textObjectRange{}
	}

	if inner {
		return textObjectRange{
			startLine: cursor, startCol: openIdx + 1,
			endLine: cursor, endCol: closeIdx,
			found: true,
		}
	}
	return textObjectRange{
		startLine: cursor, startCol: openIdx,
		endLine: cursor, endCol: closeIdx + 1,
		found: true,
	}
}

// textObjPair finds paired delimiters (parentheses, braces, brackets, angle brackets).
// This searches across lines for matching pairs.
func textObjPair(content []string, cursor, col int, open, close rune, inner bool) textObjectRange {
	if cursor >= len(content) {
		return textObjectRange{}
	}

	// Find the opening delimiter by scanning backward
	openLine, openCol := findMatchingOpen(content, cursor, col, open, close)
	if openLine == -1 {
		return textObjectRange{}
	}

	// Find the closing delimiter by scanning forward from the opening
	closeLine, closeCol := findMatchingClose(content, openLine, openCol, open, close)
	if closeLine == -1 {
		return textObjectRange{}
	}

	if inner {
		// Inner: content between delimiters (excluding delimiters)
		startCol := openCol + 1
		startLine := openLine
		endCol := closeCol
		endLine := closeLine
		return textObjectRange{
			startLine: startLine, startCol: startCol,
			endLine: endLine, endCol: endCol,
			found: true,
		}
	}
	// Around: include the delimiters
	return textObjectRange{
		startLine: openLine, startCol: openCol,
		endLine: closeLine, endCol: closeCol + 1,
		found: true,
	}
}

// findMatchingOpen scans backward from (line, col) to find the matching opening delimiter.
func findMatchingOpen(content []string, line, col int, open, close rune) (int, int) {
	depth := 0
	runes := []rune(content[line])

	// First check if we're ON an open delimiter
	if col < len(runes) && runes[col] == open {
		return line, col
	}

	// Scan backward from current position
	for l := line; l >= 0; l-- {
		runes = []rune(content[l])
		startCol := len(runes) - 1
		if l == line {
			startCol = col
		}
		for c := startCol; c >= 0; c-- {
			ch := runes[c]
			if ch == close {
				depth++
			} else if ch == open {
				if depth == 0 {
					return l, c
				}
				depth--
			}
		}
	}
	return -1, -1
}

// findMatchingClose scans forward from (line, col) to find the matching closing delimiter.
func findMatchingClose(content []string, line, col int, open, close rune) (int, int) {
	depth := 0
	for l := line; l < len(content); l++ {
		runes := []rune(content[l])
		startCol := 0
		if l == line {
			startCol = col + 1
		}
		for c := startCol; c < len(runes); c++ {
			ch := runes[c]
			if ch == open {
				depth++
			} else if ch == close {
				if depth == 0 {
					return l, c
				}
				depth--
			}
		}
	}
	return -1, -1
}

// isSentenceEnd returns true if r is a sentence-ending punctuation character.
func isSentenceEnd(r rune) bool {
	return r == '.' || r == '!' || r == '?'
}

// extractRange extracts text from content between start and end positions.
// endCol is exclusive.
func extractRange(content []string, startLine, startCol, endLine, endCol int) string {
	if startLine == endLine {
		runes := []rune(content[startLine])
		if startCol >= len(runes) {
			return ""
		}
		ec := endCol
		if ec > len(runes) {
			ec = len(runes)
		}
		return string(runes[startCol:ec])
	}
	var parts []string
	// First line from startCol
	runes := []rune(content[startLine])
	if startCol < len(runes) {
		parts = append(parts, string(runes[startCol:]))
	}
	// Middle lines in full
	for l := startLine + 1; l < endLine; l++ {
		parts = append(parts, content[l])
	}
	// Last line up to endCol
	if endLine < len(content) {
		runes = []rune(content[endLine])
		ec := endCol
		if ec > len(runes) {
			ec = len(runes)
		}
		parts = append(parts, string(runes[:ec]))
	}
	return strings.Join(parts, "\n")
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
