package tui

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// kbCardState stores which column a card has been manually assigned to.
type kbCardState struct {
	Source string `json:"source"` // note path
	Line   int    `json:"line"`   // line number
	Column string `json:"column"` // column title
}

// Kanban-local regex patterns (mirrors taskmanager patterns).
var (
	kbDueDateRe    = regexp.MustCompile(`\x{1F4C5}\s*(\d{4}-\d{2}-\d{2})`)
	kbPrioHighestRe = regexp.MustCompile(`\x{1F53A}`)  // 🔺
	kbPrioHighRe   = regexp.MustCompile(`\x{23EB}`)    // ⏫
	kbPrioMedRe    = regexp.MustCompile(`\x{1F53C}`)   // 🔼
	kbPrioLowRe    = regexp.MustCompile(`\x{1F53D}`)   // 🔽
)

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// KanbanCard represents a single task card on the Kanban board.
type KanbanCard struct {
	Text     string
	Source   string // note path where this task lives
	Line     int    // line number in source note
	Done     bool
	Priority int    // 0=none, 1=low, 2=medium, 3=high, 4=highest
	DueDate  string // "2006-01-02" or ""
	Project  string // project name if matched
}

// KanbanColumn represents a column on the Kanban board.
type KanbanColumn struct {
	Title string
	Cards []KanbanCard
}

// Kanban is an overlay component that displays tasks from vault notes as a
// Kanban board with configurable columns.
type Kanban struct {
	OverlayBase
	vaultRoot string

	columns    []KanbanColumn // configurable columns
	colCursor  int            // which column is selected
	cardCursor int            // which card in the column is selected

	allCards   []KanbanCard          // all parsed cards
	columnTags map[string][]string   // column title -> list of tags that route cards there
	savedState []kbCardState         // persisted column assignments

	// Drag state
	dragging bool
	dragCard *KanbanCard

	// Toggle result — consumed by the host on every Update tick so the
	// source file is updated mid-session, not only after the overlay
	// closes.
	pendingToggle  bool
	toggleNotePath string
	toggleLine     int
	toggleNewDone  bool

	// Open-source request — set by 'o'. Host closes the overlay and
	// opens the note in the editor.
	pendingOpen     bool
	openNotePath    string
	openLine        int

	// Delete request — set by 'd'. Host removes the source line atomically.
	pendingDelete    bool
	deleteNotePath   string
	deleteLine       int
	deleteCardText   string

	// Priority cycle request — set by 'p'. Host updates the priority
	// emoji on the source line in place.
	pendingPriority    bool
	priorityNotePath   string
	priorityLine       int
	priorityNewLevel   int // 0..4, what to set on disk

	// WIP limits per column (0 = no limit). Persisted to
	// .granit/kanban-wip.json keyed by column title. Header
	// shows N/limit with a red warning when exceeded.
	wipLimits map[string]int

	// In-board search filter. '/' enters; live-filters cards
	// across all columns by text (case-insensitive substring).
	// Empty = show all.
	searching   bool
	searchQuery string

	// Quick-add prompt for the focused column. 'a' enters;
	// Enter appends "- [ ] <text>" to Tasks.md and tags the
	// new line so it routes to the focused column.
	addingCard bool
	addBuf     string

	lastSaveErr error // consumed-once via ConsumeSaveError
}

// ConsumeSaveError returns the most recent saveState error and clears it.
// Returns nil on a nil receiver so hosts can call it defensively.
func (kb *Kanban) ConsumeSaveError() error {
	if kb == nil {
		return nil
	}
	err := kb.lastSaveErr
	kb.lastSaveErr = nil
	return err
}

// NewKanban creates a new Kanban overlay with the three default columns.
func NewKanban() Kanban {
	return Kanban{
		columns: []KanbanColumn{
			{Title: "Backlog"},
			{Title: "In Progress"},
			{Title: "Done"},
		},
		columnTags: map[string][]string{
			"In Progress": {"#doing", "#wip"},
		},
	}
}

// Configure sets custom column names and tag mappings from config.
func (kb *Kanban) Configure(columns []string, tagMap map[string]string) {
	if len(columns) == 0 {
		return
	}
	kb.columns = make([]KanbanColumn, len(columns))
	for i, name := range columns {
		kb.columns[i] = KanbanColumn{Title: name}
	}
	kb.columnTags = make(map[string][]string)
	for col, tags := range tagMap {
		for _, tag := range strings.Split(tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				kb.columnTags[col] = append(kb.columnTags[col], tag)
			}
		}
	}
}

// Open activates the Kanban overlay.
func (kb *Kanban) Open(vaultRoot string) {
	kb.Activate()
	kb.vaultRoot = vaultRoot
	kb.colCursor = 0
	kb.cardCursor = 0
	kb.dragging = false
	kb.dragCard = nil
	kb.pendingToggle = false
	kb.searching = false
	kb.searchQuery = ""
	kb.addingCard = false
	kb.addBuf = ""
	kb.loadState()
	kb.loadWIPLimits()
}

// Close persists kanban state before deactivating so column assignments
// made during the session survive even if the user doesn't explicitly save.
// Callers should drain ConsumeSaveError afterwards.
func (kb *Kanban) Close() {
	kb.saveState()
	kb.OverlayBase.Close()
}

func (kb *Kanban) statePath() string {
	return filepath.Join(kb.vaultRoot, ".granit", "kanban-state.json")
}

func (kb *Kanban) loadState() {
	kb.savedState = nil
	data, err := os.ReadFile(kb.statePath())
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, &kb.savedState); err != nil {
		log.Printf("granit: kanban-state.json corrupt (%v) — resetting column assignments", err)
	}
}

func (kb *Kanban) saveState() {
	var state []kbCardState
	for _, col := range kb.columns {
		for _, card := range col.Cards {
			state = append(state, kbCardState{
				Source: card.Source,
				Line:   card.Line,
				Column: col.Title,
			})
		}
	}
	dir := filepath.Dir(kb.statePath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		kb.lastSaveErr = err
		return
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		kb.lastSaveErr = err
		return
	}
	if err := atomicWriteState(kb.statePath(), data); err != nil {
		kb.lastSaveErr = err
		return
	}
	kb.lastSaveErr = nil
}

// SetTasks parses all task items from the provided note contents and
// distributes them into the three default columns.
func (kb *Kanban) SetTasks(noteContents map[string]string) {
	kb.allCards = nil

	// Sort note paths for deterministic ordering.
	paths := make([]string, 0, len(noteContents))
	for p := range noteContents {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, path := range paths {
		content := noteContents[path]
		lines := strings.Split(content, "\n")
		for lineIdx, line := range lines {
			trimmed := strings.TrimSpace(line)

			var card KanbanCard
			if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
				card = KanbanCard{
					Text:   strings.TrimSpace(trimmed[6:]),
					Source: path,
					Line:   lineIdx + 1, // 1-based
					Done:   true,
				}
			} else if strings.HasPrefix(trimmed, "- [ ] ") {
				card = KanbanCard{
					Text:   strings.TrimSpace(trimmed[6:]),
					Source: path,
					Line:   lineIdx + 1,
					Done:   false,
				}
			} else {
				continue
			}

			// Parse priority
			if kbPrioHighestRe.MatchString(card.Text) {
				card.Priority = 4
			} else if kbPrioHighRe.MatchString(card.Text) {
				card.Priority = 3
			} else if kbPrioMedRe.MatchString(card.Text) {
				card.Priority = 2
			} else if kbPrioLowRe.MatchString(card.Text) {
				card.Priority = 1
			}

			// Parse due date
			if dm := kbDueDateRe.FindStringSubmatch(card.Text); dm != nil {
				card.DueDate = dm[1]
			}

			kb.allCards = append(kb.allCards, card)
		}
	}

	// Build lookup from saved state for manual column assignments.
	savedCol := make(map[string]string) // "source:line" -> column title
	for _, s := range kb.savedState {
		savedCol[s.Source+":"+kbItoa(s.Line)] = s.Column
	}

	// Distribute cards into columns.
	for i := range kb.columns {
		kb.columns[i].Cards = nil
	}
	lastCol := len(kb.columns) - 1

	// Build column name → index map
	colIndex := make(map[string]int)
	for i, col := range kb.columns {
		colIndex[col.Title] = i
	}

	for _, card := range kb.allCards {
		// 1. Done cards always go to the last column
		if card.Done {
			kb.columns[lastCol].Cards = append(kb.columns[lastCol].Cards, card)
			continue
		}

		// 2. Check saved column assignment (from previous manual moves)
		key := card.Source + ":" + kbItoa(card.Line)
		if colName, ok := savedCol[key]; ok {
			if idx, exists := colIndex[colName]; exists && idx != lastCol {
				kb.columns[idx].Cards = append(kb.columns[idx].Cards, card)
				continue
			}
		}

		// 3. Check tag-based column assignment
		placed := false
		for colIdx, col := range kb.columns {
			if colIdx == 0 || colIdx == lastCol {
				continue
			}
			for _, tag := range kb.columnTags[col.Title] {
				tagLower := strings.ToLower(strings.TrimPrefix(tag, "#"))
				if strings.Contains(strings.ToLower(card.Text), tagLower) {
					kb.columns[colIdx].Cards = append(kb.columns[colIdx].Cards, card)
					placed = true
					break
				}
			}
			if placed {
				break
			}
		}
		if !placed {
			// Default: first column (backlog).
			kb.columns[0].Cards = append(kb.columns[0].Cards, card)
		}
	}

	// Sort cards within each column by priority (highest first).
	for i := range kb.columns {
		sort.SliceStable(kb.columns[i].Cards, func(a, b int) bool {
			return kb.columns[i].Cards[a].Priority > kb.columns[i].Cards[b].Priority
		})
	}

	// Clamp cursors.
	kb.kbClampCursors()
}

// SetTaskProjects applies project names from matched tasks to Kanban cards.
// It matches cards to tasks by source path and line number.
func (kb *Kanban) SetTaskProjects(tasks []Task) {
	// Build a lookup map: "notePath:lineNum" -> project name
	projMap := make(map[string]string)
	for _, t := range tasks {
		if t.Project != "" {
			key := t.NotePath + ":" + kbItoa(t.LineNum)
			projMap[key] = t.Project
		}
	}

	for i := range kb.allCards {
		key := kb.allCards[i].Source + ":" + kbItoa(kb.allCards[i].Line)
		if proj, ok := projMap[key]; ok {
			kb.allCards[i].Project = proj
		}
	}

	// Update column cards too.
	for ci := range kb.columns {
		for ci2 := range kb.columns[ci].Cards {
			key := kb.columns[ci].Cards[ci2].Source + ":" + kbItoa(kb.columns[ci].Cards[ci2].Line)
			if proj, ok := projMap[key]; ok {
				kb.columns[ci].Cards[ci2].Project = proj
			}
		}
	}
}

// HasPendingActions reports whether any consumer-pending action is set.
// Used by the host to decide whether to drain Kanban results on a tick
// even when the overlay isn't focused (e.g. immediately after close).
func (kb *Kanban) HasPendingActions() bool {
	return kb.pendingToggle || kb.pendingOpen || kb.pendingDelete || kb.pendingPriority
}

// GetToggleResult returns a pending toggle action, if any.
// The caller should update the source note content accordingly.
func (kb *Kanban) GetToggleResult() (notePath string, line int, newDone bool, ok bool) {
	if !kb.pendingToggle {
		return "", 0, false, false
	}
	kb.pendingToggle = false
	return kb.toggleNotePath, kb.toggleLine, kb.toggleNewDone, true
}

// GetOpenRequest returns a pending "open source note" action, if any.
// Set by pressing 'o' on a card. The host should close the overlay and
// load the note into the editor.
func (kb *Kanban) GetOpenRequest() (notePath string, line int, ok bool) {
	if !kb.pendingOpen {
		return "", 0, false
	}
	kb.pendingOpen = false
	return kb.openNotePath, kb.openLine, true
}

// GetDeleteRequest returns a pending card-deletion action, if any.
// Set by pressing 'd' on a card. The host should remove the line from
// the source note.
func (kb *Kanban) GetDeleteRequest() (notePath string, line int, cardText string, ok bool) {
	if !kb.pendingDelete {
		return "", 0, "", false
	}
	kb.pendingDelete = false
	return kb.deleteNotePath, kb.deleteLine, kb.deleteCardText, true
}

// GetPriorityRequest returns a pending priority-cycle action, if any.
// Set by pressing 'p' on a card. The host should rewrite the source
// line with the new priority emoji (or strip it for level 0).
func (kb *Kanban) GetPriorityRequest() (notePath string, line int, newLevel int, ok bool) {
	if !kb.pendingPriority {
		return "", 0, 0, false
	}
	kb.pendingPriority = false
	return kb.priorityNotePath, kb.priorityLine, kb.priorityNewLevel, true
}

// ---------------------------------------------------------------------------
// Update (value receiver per project convention)
// ---------------------------------------------------------------------------

func (kb Kanban) Update(msg tea.Msg) (Kanban, tea.Cmd) {
	if !kb.active {
		return kb, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		// Search input mode — '/' opens; live-filter as the
		// user types. Esc clears, Enter commits.
		if kb.searching {
			switch key {
			case "esc":
				kb.searchQuery = ""
				kb.searching = false
			case "enter":
				kb.searching = false
			case "backspace":
				if len(kb.searchQuery) > 0 {
					kb.searchQuery = TrimLastRune(kb.searchQuery)
				}
			default:
				if len(key) == 1 || key == " " {
					kb.searchQuery += key
				}
			}
			kb.kbClampCardCursor()
			return kb, nil
		}

		// Quick-add card prompt — 'a' opens; Enter appends to
		// Tasks.md tagged with the focused column's first
		// routing tag so the new card lands in the column.
		if kb.addingCard {
			switch key {
			case "esc":
				kb.addBuf = ""
				kb.addingCard = false
			case "enter":
				if strings.TrimSpace(kb.addBuf) != "" {
					kb.kbQuickAddCard(kb.addBuf)
				}
				kb.addBuf = ""
				kb.addingCard = false
			case "backspace":
				if len(kb.addBuf) > 0 {
					kb.addBuf = TrimLastRune(kb.addBuf)
				}
			default:
				if len(key) == 1 || key == " " {
					kb.addBuf += key
				}
			}
			return kb, nil
		}

		switch key {
		case "esc", "q":
			// q is the same convention used across granit's nav-only
			// overlays (calendar, kanban-style modals, dashboards). Safe
			// here because Kanban has no free-form text input — typing
			// 'q' anywhere else in the overlay routes to a labelled
			// keybinding (column nav uses h/l, card nav uses j/k).
			kb.active = false
			return kb, nil

		// Quick-add a card to the focused column.
		case "a":
			kb.addingCard = true
			kb.addBuf = ""

		// Search across all columns.
		case "/":
			kb.searching = true

		// Clear search.
		case "c":
			if kb.searchQuery != "" {
				kb.searchQuery = ""
			}

		// WIP limit adjustment for the focused column.
		// '+' increases by 1, '-' decreases (floor 0 = no limit).
		case "+":
			if kb.colCursor >= 0 && kb.colCursor < len(kb.columns) {
				if kb.wipLimits == nil {
					kb.wipLimits = make(map[string]int)
				}
				name := kb.columns[kb.colCursor].Title
				kb.wipLimits[name]++
				kb.saveWIPLimits()
			}
		case "-":
			if kb.colCursor >= 0 && kb.colCursor < len(kb.columns) {
				if kb.wipLimits == nil {
					kb.wipLimits = make(map[string]int)
				}
				name := kb.columns[kb.colCursor].Title
				if kb.wipLimits[name] > 0 {
					kb.wipLimits[name]--
				}
				kb.saveWIPLimits()
			}

		// Column navigation
		case "left", "h":
			if kb.colCursor > 0 {
				kb.colCursor--
				kb.kbClampCardCursor()
			}
		case "right", "l":
			if kb.colCursor < len(kb.columns)-1 {
				kb.colCursor++
				kb.kbClampCardCursor()
			}

		// Card navigation
		case "up", "k":
			if kb.cardCursor > 0 {
				kb.cardCursor--
			}
		case "down", "j":
			if kb.colCursor < 0 || kb.colCursor >= len(kb.columns) {
				break
			}
			col := kb.columns[kb.colCursor]
			if kb.cardCursor < len(col.Cards)-1 {
				kb.cardCursor++
			}

		// Move card to next column
		case "m", "enter":
			kb.kbMoveCardForward()

		// Move card to previous column
		case "M":
			kb.kbMoveCardBackward()

		// Toggle done state
		case "x":
			kb.kbToggleDone()

		// Open the source note in the editor (jump-to-edit). Closes the
		// overlay so the editor takes the foreground.
		case "o":
			if card := kb.currentCard(); card != nil {
				kb.pendingOpen = true
				kb.openNotePath = card.Source
				kb.openLine = card.Line
				kb.active = false
			}

		// Delete the source line for this card.
		case "d":
			if card := kb.currentCard(); card != nil {
				kb.pendingDelete = true
				kb.deleteNotePath = card.Source
				kb.deleteLine = card.Line
				kb.deleteCardText = card.Text
			}

		// Cycle the priority of the current card (none → low → med → high
		// → highest → none). Mutates the in-memory card so the change is
		// immediately visible; the host writes the source line.
		case "p":
			if card := kb.currentCard(); card != nil {
				newLevel := (card.Priority + 1) % 5
				card.Priority = newLevel
				kb.pendingPriority = true
				kb.priorityNotePath = card.Source
				kb.priorityLine = card.Line
				kb.priorityNewLevel = newLevel
			}
		}
	}

	return kb, nil
}

// currentCard returns a pointer to the card under the cursor, or nil.
// Returns the cursor card from the FILTERED slice (when a search is
// active) so the caller acts on what the user actually sees.
func (kb *Kanban) currentCard() *KanbanCard {
	if kb.colCursor < 0 || kb.colCursor >= len(kb.columns) {
		return nil
	}
	cards := kb.visibleCards(kb.colCursor)
	if kb.cardCursor < 0 || kb.cardCursor >= len(cards) {
		return nil
	}
	// Translate the visible-cursor index back to the underlying
	// columns slice so the returned pointer mutates real state.
	target := cards[kb.cardCursor]
	for i := range kb.columns[kb.colCursor].Cards {
		c := &kb.columns[kb.colCursor].Cards[i]
		if c.Source == target.Source && c.Line == target.Line {
			return c
		}
	}
	return nil
}

// visibleCards returns the cards for column ci, narrowed by the
// active search query. Empty query returns all cards in order.
func (kb Kanban) visibleCards(ci int) []KanbanCard {
	if ci < 0 || ci >= len(kb.columns) {
		return nil
	}
	cards := kb.columns[ci].Cards
	if kb.searchQuery == "" {
		return cards
	}
	q := strings.ToLower(kb.searchQuery)
	out := make([]KanbanCard, 0, len(cards))
	for _, c := range cards {
		if strings.Contains(strings.ToLower(c.Text), q) {
			out = append(out, c)
		}
	}
	return out
}

// loadWIPLimits restores per-column WIP limits from disk.
func (kb *Kanban) loadWIPLimits() {
	kb.wipLimits = make(map[string]int)
	if kb.vaultRoot == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(kb.vaultRoot, ".granit", "kanban-wip.json"))
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &kb.wipLimits)
	if kb.wipLimits == nil {
		kb.wipLimits = make(map[string]int)
	}
}

// saveWIPLimits persists the column → limit map. Sidecar so we
// don't have to extend the kanban-state schema; missing file
// silently means "no limits set anywhere".
func (kb *Kanban) saveWIPLimits() {
	if kb.vaultRoot == "" {
		return
	}
	dir := filepath.Join(kb.vaultRoot, ".granit")
	_ = os.MkdirAll(dir, 0o755)
	data, err := json.MarshalIndent(kb.wipLimits, "", "  ")
	if err != nil {
		return
	}
	if err := atomicWriteNote(filepath.Join(dir, "kanban-wip.json"), string(data)); err != nil {
		kb.lastSaveErr = err
	}
}

// kbQuickAddCard appends a fresh task to Tasks.md tagged so it
// routes to the focused column on next reparse. Routing tag is
// the first tag mapped to the column in columnTags, or the
// column title (lowercased, spaces → '-') as a fallback when
// no explicit tag exists.
func (kb *Kanban) kbQuickAddCard(text string) {
	if kb.vaultRoot == "" || kb.colCursor < 0 || kb.colCursor >= len(kb.columns) {
		return
	}
	col := kb.columns[kb.colCursor]
	tag := ""
	if tags, ok := kb.columnTags[col.Title]; ok && len(tags) > 0 {
		tag = tags[0]
	} else {
		tag = strings.ToLower(strings.ReplaceAll(col.Title, " ", "-"))
	}
	body := strings.TrimSpace(text)
	if !strings.Contains(body, "#"+tag) {
		body += " #" + tag
	}
	if err := appendTaskLine(kb.vaultRoot, "- [ ] "+body); err != nil {
		kb.lastSaveErr = err
		return
	}
	// Caller drains kb.allCards / kb.columns via the host's
	// next refreshComponents pass — file change triggers it.
}

// ---------------------------------------------------------------------------
// View (value receiver per project convention)
// ---------------------------------------------------------------------------

func (kb Kanban) View() string {
	// Determine board dimensions. Tab mode fills the editor pane
	// so columns get real estate to breathe (8 columns × ~16 char
	// minimum is the visual floor we cared about). Overlay mode
	// keeps the historical 70–130 clamp so the centered popup
	// stays a reasonable size on huge terminals.
	var boardWidth, boardHeight int
	if kb.IsTabMode() {
		boardWidth = kb.width
		if boardWidth < 70 {
			boardWidth = 70
		}
		boardHeight = kb.height
		if boardHeight < 16 {
			boardHeight = 16
		}
	} else {
		boardWidth = kb.width * 3 / 4
		if boardWidth < 70 {
			boardWidth = 70
		}
		if boardWidth > 130 {
			boardWidth = 130
		}
		boardHeight = kb.height * 3 / 4
		if boardHeight < 16 {
			boardHeight = 16
		}
	}

	// Inner content width (subtract border + padding: 2 border + 4 padding = 6)
	innerWidth := boardWidth - 6
	if innerWidth < 60 {
		innerWidth = 60
	}

	numCols := len(kb.columns)
	// Column widths — divide inner width evenly, accounting for dividers
	dividerWidth := 1 // "│" with padding handled by columns
	totalDividers := (numCols - 1) * dividerWidth
	colWidth := (innerWidth - totalDividers) / numCols
	if colWidth < 18 {
		colWidth = 18
	}

	// Visible card slots per column
	visibleCards := (boardHeight - 10) / 2 // each card takes 2 lines
	if visibleCards < 2 {
		visibleCards = 2
	}

	// Column colors — cycle through a palette for any number of columns
	colColorPalette := []lipgloss.Color{blue, yellow, green, peach, mauve, teal, lavender}
	colIconPalette := []string{"○", "◉", "●", "◆", "★", "▸", "◇"}

	// Render each column to a slice of lines
	type colLines struct {
		lines []string
	}
	allCols := make([]colLines, numCols)
	maxHeight := 0
	for ci, col := range kb.columns {
		cc := colColorPalette[ci%len(colColorPalette)]
		ci2 := colIconPalette[ci%len(colIconPalette)]
		rendered := kb.kbRenderColumn(ci, col, colWidth, visibleCards, cc, ci2)
		lines := strings.Split(rendered, "\n")
		allCols[ci] = colLines{lines: lines}
		if len(lines) > maxHeight {
			maxHeight = len(lines)
		}
	}

	// Normalize all columns to the same height
	colStyle := lipgloss.NewStyle().Width(colWidth)
	for ci := range allCols {
		for len(allCols[ci].lines) < maxHeight {
			allCols[ci].lines = append(allCols[ci].lines, colStyle.Render(""))
		}
	}

	// Build the board line by line for precise alignment
	divStyle := lipgloss.NewStyle().Foreground(surface1)
	var boardLines []string
	for row := 0; row < maxHeight; row++ {
		var line strings.Builder
		for ci := 0; ci < numCols; ci++ {
			if ci > 0 {
				line.WriteString(divStyle.Render("│"))
			}
			line.WriteString(allCols[ci].lines[row])
		}
		boardLines = append(boardLines, line.String())
	}
	board := strings.Join(boardLines, "\n")

	// Build final content
	var b strings.Builder

	// Title bar
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render("  Kanban Board"))

	// Stats
	totalCards := len(kb.allCards)
	doneCount := 0
	for _, c := range kb.allCards {
		if c.Done {
			doneCount++
		}
	}
	statsStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(statsStyle.Render("  " + kbItoa(totalCards) + " tasks, " + kbItoa(doneCount) + " done"))

	// Progress indicator
	if totalCards > 0 {
		pct := doneCount * 100 / totalCards
		pctStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		b.WriteString(pctStyle.Render("  " + kbItoa(pct) + "%"))
	}
	b.WriteString("\n")

	// Top rule
	ruleStyle := lipgloss.NewStyle().Foreground(surface1)
	b.WriteString(ruleStyle.Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n\n")

	// Board
	b.WriteString(board)
	b.WriteString("\n\n")

	// Bottom rule
	b.WriteString(ruleStyle.Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n")

	// Active search query — show as a chip when a query is
	// committed (search bar closed), or as a live-typing bar
	// when the user is mid-search.
	promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	if kb.searching {
		b.WriteString("  " + promptStyle.Render("/ Search: ") +
			lipgloss.NewStyle().Foreground(text).Render(kb.searchQuery+"█") + "\n")
	} else if kb.searchQuery != "" {
		b.WriteString("  " + DimStyle.Render("/ search: "+kb.searchQuery+"  (c to clear)") + "\n")
	}

	// Quick-add prompt for the focused column.
	if kb.addingCard {
		colName := ""
		if kb.colCursor < len(kb.columns) {
			colName = kb.columns[kb.colCursor].Title
		}
		b.WriteString("  " + promptStyle.Render("+ Add to "+colName+": ") +
			lipgloss.NewStyle().Foreground(text).Render(kb.addBuf+"█") + "\n")
	}

	// Footer with keybinds
	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"←→", "column"}, {"↑↓", "card"}, {"m/M", "move ↔"},
		{"a", "add"}, {"x", "toggle"}, {"o", "open"}, {"p", "priority"}, {"d", "delete"},
		{"+/-", "WIP"}, {"/", "search"}, {"c", "clear"}, {"Esc", "close"},
	}))

	if kb.IsTabMode() {
		return b.String()
	}
	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(boardWidth)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// Internal helpers (kb-prefixed to avoid collisions)
// ---------------------------------------------------------------------------

// kbRenderColumn renders a single Kanban column as a slice of fixed-width lines.
func (kb Kanban) kbRenderColumn(colIdx int, col KanbanColumn, width, visibleCards int, colColor lipgloss.Color, icon string) string {
	lineStyle := lipgloss.NewStyle().Width(width)
	isActiveCol := colIdx == kb.colCursor

	var lines []string

	// Column header
	titleStyle := lipgloss.NewStyle().Foreground(colColor).Bold(true)
	countStyle := lipgloss.NewStyle().Foreground(surface2)

	indicator := "  "
	if isActiveCol {
		indicator = lipgloss.NewStyle().Foreground(colColor).Bold(true).Render("> ")
	}

	// WIP-aware count: "5/3" with red ⚠ when over limit.
	countText := kbItoa(len(col.Cards))
	countRendered := countStyle.Render(" " + countText)
	if limit := kb.wipLimits[col.Title]; limit > 0 {
		countText = kbItoa(len(col.Cards)) + "/" + kbItoa(limit)
		if len(col.Cards) > limit {
			countRendered = " " + lipgloss.NewStyle().Foreground(red).Bold(true).
				Render("⚠ "+countText)
		} else {
			countRendered = countStyle.Render(" " + countText)
		}
	}
	headerContent := indicator +
		lipgloss.NewStyle().Foreground(colColor).Render(icon) + " " +
		titleStyle.Render(col.Title) + countRendered
	lines = append(lines, lineStyle.Render(headerContent))

	// Column underline
	underColor := surface1
	if isActiveCol {
		underColor = colColor
	}
	underline := "  " + lipgloss.NewStyle().Foreground(underColor).Render(strings.Repeat("-", width-4))
	lines = append(lines, lineStyle.Render(underline))

	// Search-aware card list: when a query is active, only
	// matching cards render (and the cursor walks the
	// filtered slice via currentCard()).
	visible := kb.visibleCards(colIdx)
	if len(visible) == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(surface2).Italic(true)
		emptyMsg := "No tasks"
		if kb.searchQuery != "" {
			emptyMsg = "No matches"
		}
		lines = append(lines, lineStyle.Render("  "+emptyStyle.Render(emptyMsg)))
		// Pad remaining
		for i := 1; i < visibleCards*2; i++ {
			lines = append(lines, lineStyle.Render(""))
		}
	} else {
		scrollOffset := 0
		if isActiveCol && kb.cardCursor >= visibleCards {
			scrollOffset = kb.cardCursor - visibleCards + 1
		}
		if scrollOffset > len(visible) {
			scrollOffset = len(visible)
		}

		end := scrollOffset + visibleCards
		if end > len(visible) {
			end = len(visible)
		}

		linesUsed := 0
		for i := scrollOffset; i < end; i++ {
			card := visible[i]
			isSelected := isActiveCol && i == kb.cardCursor

			prioIcon := kbPriorityIcon(card.Priority)
			cardText := kbTruncate(card.Text, width-8)
			sourceName := kbBaseName(card.Source)
			metaParts := []string{sourceName}
			if card.Project != "" {
				metaParts = append(metaParts, card.Project)
			}
			if card.DueDate != "" {
				metaParts = append(metaParts, card.DueDate)
			}
			sourceStr := kbTruncate(strings.Join(metaParts, " | "), width-8)

			if isSelected {
				checkIcon := "o"
				checkColor := colColor
				if card.Done {
					checkIcon = "x"
					checkColor = green
				}

				checkSt := lipgloss.NewStyle().Background(surface0).Foreground(checkColor).Bold(true)
				textSt := lipgloss.NewStyle().Background(surface0).Foreground(text).Bold(true)
				srcSt := lipgloss.NewStyle().Background(surface0).Foreground(overlay0)
				bgStyle := lipgloss.NewStyle().Background(surface0).Width(width)

				line1 := " " + checkSt.Render(checkIcon) + prioIcon + " " + textSt.Render(cardText)
				line2 := "   " + srcSt.Render(sourceStr)

				lines = append(lines, bgStyle.Render(line1))
				lines = append(lines, bgStyle.Render(line2))
			} else {
				checkIcon := "o"
				checkColor := surface2
				if card.Done {
					checkIcon = "x"
					checkColor = green
				}

				checkSt := lipgloss.NewStyle().Foreground(checkColor)
				textSt := lipgloss.NewStyle().Foreground(text)
				srcSt := lipgloss.NewStyle().Foreground(surface2)

				line1 := " " + checkSt.Render(checkIcon) + prioIcon + " " + textSt.Render(cardText)
				line2 := "   " + srcSt.Render(sourceStr)

				lines = append(lines, lineStyle.Render(line1))
				lines = append(lines, lineStyle.Render(line2))
			}
			linesUsed += 2
		}

		// Scroll indicator
		if len(col.Cards) > visibleCards {
			remaining := len(col.Cards) - end
			if remaining > 0 {
				moreStyle := lipgloss.NewStyle().Foreground(surface2).Italic(true)
				lines = append(lines, lineStyle.Render(" "+moreStyle.Render("+"+kbItoa(remaining)+" more")))
				linesUsed++
			}
		}

		// Pad remaining vertical space
		for linesUsed < visibleCards*2 {
			lines = append(lines, lineStyle.Render(""))
			linesUsed++
		}
	}

	return strings.Join(lines, "\n")
}

// kbPriorityIcon returns a colored priority dot for kanban cards.
func kbPriorityIcon(priority int) string {
	switch priority {
	case 4:
		return lipgloss.NewStyle().Foreground(red).Render("\u25cf")
	case 3:
		return lipgloss.NewStyle().Foreground(peach).Render("\u25cf")
	case 2:
		return lipgloss.NewStyle().Foreground(yellow).Render("\u25cf")
	case 1:
		return lipgloss.NewStyle().Foreground(blue).Render("\u25cf")
	default:
		return ""
	}
}


// kbTruncate truncates s to maxLen display width, using unicode-safe measurement.
func kbTruncate(s string, maxLen int) string {
	return TruncateDisplay(s, maxLen)
}

// kbBaseName returns the file name from a path, without the .md extension.
func kbBaseName(path string) string {
	idx := strings.LastIndex(path, "/")
	name := path
	if idx >= 0 {
		name = path[idx+1:]
	}
	idx = strings.LastIndex(name, "\\")
	if idx >= 0 {
		name = name[idx+1:]
	}
	if strings.HasSuffix(strings.ToLower(name), ".md") {
		name = name[:len(name)-3]
	}
	return name
}

// kbItoa converts an int to a decimal string without importing strconv.
func kbItoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

// kbClampCursors ensures both colCursor and cardCursor are within bounds.
func (kb *Kanban) kbClampCursors() {
	if len(kb.columns) == 0 {
		kb.colCursor = 0
		kb.cardCursor = 0
		return
	}
	if kb.colCursor < 0 {
		kb.colCursor = 0
	}
	if kb.colCursor >= len(kb.columns) {
		kb.colCursor = len(kb.columns) - 1
	}
	kb.kbClampCardCursor()
}

// kbClampCardCursor ensures cardCursor is within bounds for the current column.
func (kb *Kanban) kbClampCardCursor() {
	if kb.colCursor < 0 || kb.colCursor >= len(kb.columns) {
		kb.cardCursor = 0
		return
	}
	col := kb.columns[kb.colCursor]
	if kb.cardCursor >= len(col.Cards) {
		kb.cardCursor = len(col.Cards) - 1
	}
	if kb.cardCursor < 0 {
		kb.cardCursor = 0
	}
}

// kbMoveCardForward moves the selected card from the current column to the next one.
func (kb *Kanban) kbMoveCardForward() {
	if kb.colCursor >= len(kb.columns)-1 {
		return
	}
	col := &kb.columns[kb.colCursor]
	if len(col.Cards) == 0 || kb.cardCursor >= len(col.Cards) {
		return
	}

	card := col.Cards[kb.cardCursor]
	col.Cards = append(col.Cards[:kb.cardCursor], col.Cards[kb.cardCursor+1:]...)
	next := &kb.columns[kb.colCursor+1]
	next.Cards = append(next.Cards, card)

	if kb.cardCursor >= len(col.Cards) && kb.cardCursor > 0 {
		kb.cardCursor--
	}
}

// kbMoveCardBackward moves the selected card from the current column to the previous one.
func (kb *Kanban) kbMoveCardBackward() {
	if kb.colCursor <= 0 {
		return
	}
	col := &kb.columns[kb.colCursor]
	if len(col.Cards) == 0 || kb.cardCursor >= len(col.Cards) {
		return
	}

	card := col.Cards[kb.cardCursor]
	col.Cards = append(col.Cards[:kb.cardCursor], col.Cards[kb.cardCursor+1:]...)
	prev := &kb.columns[kb.colCursor-1]
	prev.Cards = append(prev.Cards, card)

	if kb.cardCursor >= len(col.Cards) && kb.cardCursor > 0 {
		kb.cardCursor--
	}
}

// kbToggleDone toggles the Done state of the selected card and records the
// toggle result so the host can update the source note.
func (kb *Kanban) kbToggleDone() {
	if kb.colCursor < 0 || kb.colCursor >= len(kb.columns) {
		return
	}
	col := &kb.columns[kb.colCursor]
	if kb.cardCursor < 0 || kb.cardCursor >= len(col.Cards) {
		return
	}

	card := &col.Cards[kb.cardCursor]
	card.Done = !card.Done

	kb.pendingToggle = true
	kb.toggleNotePath = card.Source
	kb.toggleLine = card.Line
	kb.toggleNewDone = card.Done
}
