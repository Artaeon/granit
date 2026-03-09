package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/vault"
)

// ═══════════════════════════════════════════════════════════════════════════
// Dataview Overlay — SQL-like query language for querying notes
// ═══════════════════════════════════════════════════════════════════════════

// DataviewOverlay provides a text-based query input with results viewer.
// Users type queries in an Obsidian Dataview-like syntax:
//
//	TABLE title, date, tags FROM "folder" WHERE tags CONTAINS "project"
//	LIST FROM #tag WHERE date >= 2024-01-01 SORT date DESC
//	TASK FROM "projects" WHERE !completed
type DataviewOverlay struct {
	active    bool
	width     int
	height    int
	vault     *vault.Vault

	// Query input
	query     string
	cursorPos int // cursor position within query string

	// Parsed and executed results
	parsed  *DVParsedQuery
	result  DVQueryResult
	hasRun  bool // true after first query execution

	// Results navigation
	cursor int
	scroll int

	// History
	history    []string
	historyIdx int    // -1 = editing; 0..len-1 = browsing
	savedQuery string

	// Consumed-once selected note
	selectedNote string
	hasResult    bool
}

// NewDataviewOverlay returns a zero-value overlay ready to be opened.
func NewDataviewOverlay() DataviewOverlay {
	return DataviewOverlay{
		historyIdx: -1,
	}
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Open initialises the overlay.
func (d *DataviewOverlay) Open(v *vault.Vault) {
	d.active = true
	d.vault = v
	d.query = ""
	d.cursorPos = 0
	d.parsed = nil
	d.result = DVQueryResult{}
	d.hasRun = false
	d.cursor = 0
	d.scroll = 0
	d.selectedNote = ""
	d.hasResult = false
	d.historyIdx = -1
	d.savedQuery = ""
}

// SetSize updates the available rendering dimensions.
func (d *DataviewOverlay) SetSize(w, h int) {
	d.width = w
	d.height = h
}

// IsActive reports whether the overlay is currently shown.
func (d DataviewOverlay) IsActive() bool {
	return d.active
}

// GetSelectedNote returns the chosen note path (consumed once).
func (d *DataviewOverlay) GetSelectedNote() (string, bool) {
	if !d.hasResult {
		return "", false
	}
	path := d.selectedNote
	d.selectedNote = ""
	d.hasResult = false
	return path, true
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles keyboard input and returns the (possibly mutated) overlay.
func (d DataviewOverlay) Update(msg tea.Msg) (DataviewOverlay, tea.Cmd) {
	if !d.active {
		return d, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return d.handleKey(msg)
	}
	return d, nil
}

func (d DataviewOverlay) handleKey(msg tea.KeyMsg) (DataviewOverlay, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		if d.hasRun {
			// Go back to query editing
			d.hasRun = false
			d.cursor = 0
			d.scroll = 0
			return d, nil
		}
		d.active = false
		return d, nil

	case "enter":
		if d.hasRun {
			// Select current result
			d.selectCurrentResult()
			return d, nil
		}
		// Execute query
		d.executeQuery()
		return d, nil

	case "up":
		if d.hasRun {
			d.moveCursorUp()
		} else if len(d.history) > 0 {
			d.browseHistoryUp()
		}
		return d, nil

	case "down":
		if d.hasRun {
			d.moveCursorDown()
		} else if d.historyIdx >= 0 {
			d.browseHistoryDown()
		}
		return d, nil

	case "ctrl+c":
		d.active = false
		return d, nil

	case "backspace":
		if !d.hasRun && len(d.query) > 0 {
			if d.cursorPos > 0 {
				d.query = d.query[:d.cursorPos-1] + d.query[d.cursorPos:]
				d.cursorPos--
			}
			d.historyIdx = -1
		}
		return d, nil

	case "delete":
		if !d.hasRun && d.cursorPos < len(d.query) {
			d.query = d.query[:d.cursorPos] + d.query[d.cursorPos+1:]
			d.historyIdx = -1
		}
		return d, nil

	case "left":
		if !d.hasRun && d.cursorPos > 0 {
			d.cursorPos--
		}
		return d, nil

	case "right":
		if !d.hasRun && d.cursorPos < len(d.query) {
			d.cursorPos++
		}
		return d, nil

	case "home", "ctrl+a":
		if !d.hasRun {
			d.cursorPos = 0
		}
		return d, nil

	case "end", "ctrl+e":
		if !d.hasRun {
			d.cursorPos = len(d.query)
		}
		return d, nil

	case "tab":
		if !d.hasRun {
			d.insertCompletion()
		}
		return d, nil

	default:
		if !d.hasRun && len(key) == 1 && key[0] >= 32 {
			d.query = d.query[:d.cursorPos] + key + d.query[d.cursorPos:]
			d.cursorPos++
			d.historyIdx = -1
		}
		return d, nil
	}
}

func (d *DataviewOverlay) executeQuery() {
	if strings.TrimSpace(d.query) == "" {
		return
	}

	// Save to history
	d.history = dvAppendHistory(d.history, d.query)

	// Parse and execute
	d.parsed = ParseDVQuery(d.query)
	d.result = ExecuteDVQuery(d.parsed, d.vault)
	d.hasRun = true
	d.cursor = 0
	d.scroll = 0
}

func (d *DataviewOverlay) selectCurrentResult() {
	switch d.result.Mode {
	case DVModeTable, DVModeList:
		if len(d.result.Rows) > 0 && d.cursor < len(d.result.Rows) {
			d.selectedNote = d.result.Rows[d.cursor].NotePath
			d.hasResult = true
			d.active = false
		}
	case DVModeTask:
		if len(d.result.Tasks) > 0 && d.cursor < len(d.result.Tasks) {
			d.selectedNote = d.result.Tasks[d.cursor].NotePath
			d.hasResult = true
			d.active = false
		}
	}
}

func (d *DataviewOverlay) moveCursorUp() {
	if d.cursor > 0 {
		d.cursor--
		if d.cursor < d.scroll {
			d.scroll = d.cursor
		}
	}
}

func (d *DataviewOverlay) moveCursorDown() {
	maxIdx := d.resultCount() - 1
	if d.cursor < maxIdx {
		d.cursor++
		visH := d.visibleRows()
		if d.cursor >= d.scroll+visH {
			d.scroll = d.cursor - visH + 1
		}
	}
}

func (d *DataviewOverlay) browseHistoryUp() {
	if d.historyIdx == -1 {
		d.savedQuery = d.query
		d.historyIdx = len(d.history) - 1
	} else if d.historyIdx > 0 {
		d.historyIdx--
	}
	d.query = d.history[d.historyIdx]
	d.cursorPos = len(d.query)
}

func (d *DataviewOverlay) browseHistoryDown() {
	if d.historyIdx < len(d.history)-1 {
		d.historyIdx++
		d.query = d.history[d.historyIdx]
	} else {
		d.historyIdx = -1
		d.query = d.savedQuery
	}
	d.cursorPos = len(d.query)
}

func (d *DataviewOverlay) insertCompletion() {
	upper := strings.ToUpper(strings.TrimSpace(d.query))

	// Offer completions based on what's typed so far
	completions := map[string]string{
		"":      "TABLE ",
		"T":     "TABLE ",
		"TA":    "TABLE ",
		"TAB":   "TABLE ",
		"TABL":  "TABLE ",
		"L":     "LIST ",
		"LI":    "LIST ",
		"LIS":   "LIST ",
		"TAS":   "TASK ",
	}

	if c, ok := completions[upper]; ok {
		d.query = c
		d.cursorPos = len(d.query)
		return
	}

	// After a mode keyword, suggest common continuations
	words := strings.Fields(upper)
	if len(words) > 0 {
		last := words[len(words)-1]
		suffixes := map[string]string{
			"F":   "FROM ",
			"FR":  "FROM ",
			"FRO": "FROM ",
			"W":   "WHERE ",
			"WH":  "WHERE ",
			"WHE": "WHERE ",
			"WHER": "WHERE ",
			"S":   "SORT ",
			"SO":  "SORT ",
			"SOR": "SORT ",
			"C":   "CONTAINS ",
			"CO":  "CONTAINS ",
			"CON": "CONTAINS ",
			"CONT": "CONTAINS ",
			"CONTA": "CONTAINS ",
			"CONTAI": "CONTAINS ",
			"CONTAIN": "CONTAINS ",
		}
		if s, ok := suffixes[last]; ok {
			// Replace the last word
			prefix := d.query[:len(d.query)-len(last)]
			d.query = prefix + s
			d.cursorPos = len(d.query)
		}
	}
}

func (d DataviewOverlay) resultCount() int {
	switch d.result.Mode {
	case DVModeTable, DVModeList:
		return len(d.result.Rows)
	case DVModeTask:
		return len(d.result.Tasks)
	}
	return 0
}

func (d DataviewOverlay) visibleRows() int {
	h := d.height - 16
	if h < 3 {
		h = 3
	}
	return h
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the overlay.
func (d DataviewOverlay) View() string {
	width := d.width * 3 / 4
	if width < 80 {
		width = 80
	}
	if width > 120 {
		width = 120
	}
	innerWidth := width - 6

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render(IconSearchChar + "  Dataview Query"))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	// Query input
	prompt := SearchPromptStyle.Render("  > ")
	queryDisplay := d.query
	if !d.hasRun {
		// Show cursor
		before := ""
		after := ""
		if d.cursorPos <= len(d.query) {
			before = d.query[:d.cursorPos]
			after = d.query[d.cursorPos:]
		}
		cursorChar := lipgloss.NewStyle().Background(text).Foreground(base).Render(" ")
		if d.cursorPos < len(d.query) {
			cursorChar = lipgloss.NewStyle().Background(text).Foreground(base).Render(string(d.query[d.cursorPos]))
			after = d.query[d.cursorPos+1:]
		}
		queryDisplay = before + cursorChar + after
	}
	b.WriteString(prompt + queryDisplay)
	b.WriteString("\n")

	if !d.hasRun && d.query == "" {
		// Show syntax help
		b.WriteString("\n")
		hintStyle := lipgloss.NewStyle().Foreground(overlay0)
		exStyle := lipgloss.NewStyle().Foreground(lavender)
		b.WriteString(hintStyle.Render("  Examples:") + "\n")
		b.WriteString(exStyle.Render("  TABLE title, date, tags FROM \"folder\" WHERE tags CONTAINS \"project\"") + "\n")
		b.WriteString(exStyle.Render("  LIST FROM #tag WHERE date >= 2024-01-01 SORT date DESC") + "\n")
		b.WriteString(exStyle.Render("  TASK FROM \"projects\" WHERE !completed") + "\n")
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("  Syntax:") + "\n")
		kwStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
		b.WriteString("  " + kwStyle.Render("TABLE") + hintStyle.Render(" field1, field2  ") +
			kwStyle.Render("LIST") + hintStyle.Render("  ") +
			kwStyle.Render("TASK") + "\n")
		b.WriteString("  " + kwStyle.Render("FROM") + hintStyle.Render(" \"folder\" or #tag") + "\n")
		b.WriteString("  " + kwStyle.Render("WHERE") + hintStyle.Render(" field ") +
			kwStyle.Render("= != > < >= <= CONTAINS") + hintStyle.Render(" value") + "\n")
		b.WriteString("  " + kwStyle.Render("SORT") + hintStyle.Render(" field ") +
			kwStyle.Render("ASC") + hintStyle.Render("/") + kwStyle.Render("DESC") + "\n")
		b.WriteString("  " + kwStyle.Render("LIMIT") + hintStyle.Render(" number") + "\n")
	}

	if d.hasRun {
		b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
		b.WriteString("\n")
		b.WriteString(d.renderResults(innerWidth))
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	keyStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(overlay0)
	sepStyle := lipgloss.NewStyle().Foreground(surface1)
	sep := sepStyle.Render(" | ")

	if d.hasRun {
		b.WriteString("  " +
			keyStyle.Render("↑↓") + descStyle.Render(" navigate") + sep +
			keyStyle.Render("Enter") + descStyle.Render(" open note") + sep +
			keyStyle.Render("Esc") + descStyle.Render(" edit query") + sep +
			keyStyle.Render("Ctrl+C") + descStyle.Render(" close"))
	} else {
		b.WriteString("  " +
			keyStyle.Render("Enter") + descStyle.Render(" run query") + sep +
			keyStyle.Render("Tab") + descStyle.Render(" complete") + sep +
			keyStyle.Render("↑↓") + descStyle.Render(" history") + sep +
			keyStyle.Render("Esc") + descStyle.Render(" close"))
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// renderResults renders the query results based on mode.
func (d DataviewOverlay) renderResults(innerWidth int) string {
	var b strings.Builder

	// Result count header
	modeLabel := "TABLE"
	switch d.result.Mode {
	case DVModeList:
		modeLabel = "LIST"
	case DVModeTask:
		modeLabel = "TASK"
	}

	countStyle := lipgloss.NewStyle().Foreground(overlay0)
	modeStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	b.WriteString("  " + modeStyle.Render(modeLabel))
	count := d.resultCount()
	b.WriteString(countStyle.Render(fmt.Sprintf("  %d result", count)))
	if count != 1 {
		b.WriteString(countStyle.Render("s"))
	}
	if d.result.Total > count {
		b.WriteString(countStyle.Render(fmt.Sprintf(" (of %d)", d.result.Total)))
	}
	b.WriteString("\n\n")

	if count == 0 {
		b.WriteString(DimStyle.Render("  No matching notes found."))
		b.WriteString("\n")
		return b.String()
	}

	switch d.result.Mode {
	case DVModeTable:
		b.WriteString(d.renderTableView(innerWidth))
	case DVModeList:
		b.WriteString(d.renderListView(innerWidth))
	case DVModeTask:
		b.WriteString(d.renderTaskView(innerWidth))
	}

	return b.String()
}

// renderTableView draws an ASCII table of results.
func (d DataviewOverlay) renderTableView(innerWidth int) string {
	fields := d.result.Fields
	if len(fields) == 0 {
		fields = []string{"title", "path"}
	}

	numCols := len(fields)
	colWidth := (innerWidth - numCols - 3) / numCols
	if colWidth < 8 {
		colWidth = 8
	}

	var b strings.Builder

	borderSt := lipgloss.NewStyle().Foreground(surface1)

	// Top border
	b.WriteString("  " + borderSt.Render("┌"))
	for i := 0; i < numCols; i++ {
		b.WriteString(borderSt.Render(strings.Repeat("─", colWidth)))
		if i < numCols-1 {
			b.WriteString(borderSt.Render("┬"))
		}
	}
	b.WriteString(borderSt.Render("┐") + "\n")

	// Header row
	headerStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	b.WriteString("  " + borderSt.Render("│"))
	for _, f := range fields {
		label := dvTruncate(dvCapitalize(f), colWidth-2)
		padded := dvPadField(label, colWidth-2)
		b.WriteString(" " + headerStyle.Render(padded) + " " + borderSt.Render("│"))
	}
	b.WriteString("\n")

	// Header separator
	b.WriteString("  " + borderSt.Render("├"))
	for i := 0; i < numCols; i++ {
		b.WriteString(borderSt.Render(strings.Repeat("─", colWidth)))
		if i < numCols-1 {
			b.WriteString(borderSt.Render("┼"))
		}
	}
	b.WriteString(borderSt.Render("┤") + "\n")

	// Data rows
	visH := d.visibleRows()
	end := d.scroll + visH
	if end > len(d.result.Rows) {
		end = len(d.result.Rows)
	}

	for idx := d.scroll; idx < end; idx++ {
		row := d.result.Rows[idx]
		isSelected := idx == d.cursor

		rowStyle := lipgloss.NewStyle().Foreground(text)
		if isSelected {
			rowStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
		}

		prefix := "  " + borderSt.Render("│")
		if isSelected {
			prefix = lipgloss.NewStyle().Foreground(mauve).Render("▸ ") + borderSt.Render("│")
		}
		b.WriteString(prefix)

		for _, f := range fields {
			val := row.Fields[f]
			if val == "" {
				val = "-"
			}
			truncated := dvTruncate(val, colWidth-2)
			padded := dvPadField(truncated, colWidth-2)
			b.WriteString(" " + rowStyle.Render(padded) + " " + borderSt.Render("│"))
		}
		b.WriteString("\n")
	}

	// Bottom border
	b.WriteString("  " + borderSt.Render("└"))
	for i := 0; i < numCols; i++ {
		b.WriteString(borderSt.Render(strings.Repeat("─", colWidth)))
		if i < numCols-1 {
			b.WriteString(borderSt.Render("┴"))
		}
	}
	b.WriteString(borderSt.Render("┘"))

	return b.String()
}

// renderListView draws a bullet list of note names as wikilinks.
func (d DataviewOverlay) renderListView(innerWidth int) string {
	var b strings.Builder

	visH := d.visibleRows()
	end := d.scroll + visH
	if end > len(d.result.Rows) {
		end = len(d.result.Rows)
	}

	for idx := d.scroll; idx < end; idx++ {
		row := d.result.Rows[idx]
		isSelected := idx == d.cursor

		titleStyle := lipgloss.NewStyle().Foreground(blue)
		pathStyle := lipgloss.NewStyle().Foreground(overlay0)
		bullet := DimStyle.Render("  - ")
		if isSelected {
			titleStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
			pathStyle = lipgloss.NewStyle().Foreground(lavender)
			bullet = lipgloss.NewStyle().Foreground(mauve).Render("  ▸ ")
		}

		name := strings.TrimSuffix(filepath.Base(row.NotePath), ".md")
		link := titleStyle.Render("[[" + name + "]]")
		path := pathStyle.Render("  " + row.NotePath)

		line := bullet + link + path
		b.WriteString(dvTruncate(line, innerWidth) + "\n")
	}

	return b.String()
}

// renderTaskView draws a checkbox list grouped by source note.
func (d DataviewOverlay) renderTaskView(innerWidth int) string {
	var b strings.Builder

	visH := d.visibleRows()
	end := d.scroll + visH
	if end > len(d.result.Tasks) {
		end = len(d.result.Tasks)
	}

	lastNote := ""
	for idx := d.scroll; idx < end; idx++ {
		task := d.result.Tasks[idx]
		isSelected := idx == d.cursor

		// Group header
		if task.NotePath != lastNote {
			if lastNote != "" {
				b.WriteString("\n")
			}
			noteStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
			b.WriteString("  " + noteStyle.Render(task.NoteTitle) + "\n")
			lastNote = task.NotePath
		}

		checkStyle := lipgloss.NewStyle().Foreground(green)
		textStyle := lipgloss.NewStyle().Foreground(text)
		prefix := "    "
		if isSelected {
			textStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
			prefix = lipgloss.NewStyle().Foreground(mauve).Render("  ▸ ")
		}

		checkbox := checkStyle.Render("[ ]")
		if task.Completed {
			checkbox = checkStyle.Render("[x]")
			if !isSelected {
				textStyle = lipgloss.NewStyle().Foreground(overlay0).Strikethrough(true)
			}
		}

		taskText := dvTruncate(task.Text, innerWidth-10)
		b.WriteString(prefix + checkbox + " " + textStyle.Render(taskText) + "\n")
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// dvAppendHistory adds a query to the history, deduplicating.
func dvAppendHistory(history []string, query string) []string {
	// Remove duplicates
	var filtered []string
	for _, h := range history {
		if h != query {
			filtered = append(filtered, h)
		}
	}
	filtered = append(filtered, query)
	// Keep last 50 entries
	if len(filtered) > 50 {
		filtered = filtered[len(filtered)-50:]
	}
	return filtered
}

// dvTruncate shortens a string to fit within maxLen runes.
func dvTruncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

// dvPadField pads a string to exactly width runes with trailing spaces.
func dvPadField(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return string(runes[:width])
	}
	return s + strings.Repeat(" ", width-len(runes))
}

// dvCapitalize returns the string with its first letter uppercased.
func dvCapitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ═══════════════════════════════════════════════════════════════════════════
// Inline Dataview — query engine used by the markdown renderer to evaluate
// ```query code blocks directly inside notes.
// ═══════════════════════════════════════════════════════════════════════════

// InlineDataviewQuery represents a parsed query block from a markdown note.
type InlineDataviewQuery struct {
	From   string   // filter expression: "tags:xxx", "folder:xxx", "all"
	Where  string   // property filter: "status = active"
	Sort   string   // sort field + direction: "modified desc", "title asc"
	Limit  int      // max results
	Fields []string // fields to display: "title", "date", "status", etc.
}

// InlineDataviewResult holds a single note result from an inline query.
type InlineDataviewResult struct {
	Path   string
	Title  string
	Fields map[string]string // field name -> value
}

// ParseDataviewQuery parses the content of a ```query code block into an
// InlineDataviewQuery. Lines are expected to be key: value pairs.
func ParseDataviewQuery(block string) *InlineDataviewQuery {
	q := &InlineDataviewQuery{
		From:  "all",
		Limit: 20,
	}

	lines := strings.Split(strings.TrimSpace(block), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch strings.ToLower(key) {
		case "from":
			q.From = value
		case "where":
			q.Where = value
		case "sort":
			q.Sort = value
		case "limit":
			if n, err := strconv.Atoi(value); err == nil && n > 0 {
				q.Limit = n
			}
		case "fields":
			raw := strings.Split(value, ",")
			for _, f := range raw {
				f = strings.TrimSpace(f)
				if f != "" {
					q.Fields = append(q.Fields, f)
				}
			}
		case "tags":
			// Shorthand: tags: xxx -> from: tags:xxx
			q.From = "tags:" + value
		case "recent":
			// Shorthand: recent: N -> sort: modified desc + limit: N
			q.Sort = "modified desc"
			if n, err := strconv.Atoi(value); err == nil && n > 0 {
				q.Limit = n
			}
		}
	}

	return q
}

// ExecuteDataviewQuery runs an inline query against the vault notes and
// returns matching results with the requested fields extracted.
func ExecuteDataviewQuery(query *InlineDataviewQuery, notes map[string]*vault.Note) []InlineDataviewResult {
	// 1. Start with all notes
	candidates := make([]*vault.Note, 0, len(notes))
	for _, note := range notes {
		candidates = append(candidates, note)
	}

	// 2. Apply "from" filter
	candidates = applyFromFilter(candidates, query.From)

	// 3. Apply "where" filter
	candidates = applyWhereFilter(candidates, query.Where)

	// 4. Sort
	applySortOrder(candidates, query.Sort)

	// 5. Apply limit
	if query.Limit > 0 && len(candidates) > query.Limit {
		candidates = candidates[:query.Limit]
	}

	// 6. Extract fields
	fields := query.Fields
	if len(fields) == 0 {
		fields = []string{"title", "date"}
	}

	results := make([]InlineDataviewResult, 0, len(candidates))
	for _, note := range candidates {
		r := InlineDataviewResult{
			Path:   note.RelPath,
			Title:  note.Title,
			Fields: make(map[string]string),
		}
		for _, field := range fields {
			r.Fields[field] = extractField(note, field)
		}
		results = append(results, r)
	}

	return results
}

// applyFromFilter filters notes based on the "from" expression.
func applyFromFilter(notes []*vault.Note, from string) []*vault.Note {
	if from == "" || from == "all" {
		return notes
	}

	if strings.HasPrefix(from, "tags:") {
		tag := strings.TrimPrefix(from, "tags:")
		tag = strings.TrimSpace(tag)
		var filtered []*vault.Note
		for _, note := range notes {
			if noteHasTag(note, tag) {
				filtered = append(filtered, note)
			}
		}
		return filtered
	}

	if strings.HasPrefix(from, "folder:") {
		folder := strings.TrimPrefix(from, "folder:")
		folder = strings.TrimSpace(folder)
		var filtered []*vault.Note
		for _, note := range notes {
			noteDir := filepath.Dir(note.RelPath)
			// Match if the note is in the folder or a subfolder
			if noteDir == folder || strings.HasPrefix(noteDir, folder+string(filepath.Separator)) {
				filtered = append(filtered, note)
			}
		}
		return filtered
	}

	return notes
}

// noteHasTag checks whether a note's frontmatter contains the given tag.
func noteHasTag(note *vault.Note, tag string) bool {
	if note.Frontmatter == nil {
		return false
	}

	tagsVal, ok := note.Frontmatter["tags"]
	if !ok {
		return false
	}

	switch v := tagsVal.(type) {
	case []string:
		for _, t := range v {
			if strings.EqualFold(strings.TrimSpace(t), tag) {
				return true
			}
		}
	case string:
		for _, t := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(t), tag) {
				return true
			}
		}
	}

	return false
}

// applyWhereFilter filters notes by a frontmatter property condition.
// Supports "key = value" and "key != value".
func applyWhereFilter(notes []*vault.Note, where string) []*vault.Note {
	if where == "" {
		return notes
	}

	var key, value string
	negate := false

	if idx := strings.Index(where, "!="); idx >= 0 {
		key = strings.TrimSpace(where[:idx])
		value = strings.TrimSpace(where[idx+2:])
		negate = true
	} else if idx := strings.Index(where, "="); idx >= 0 {
		key = strings.TrimSpace(where[:idx])
		value = strings.TrimSpace(where[idx+1:])
	} else {
		return notes
	}

	var filtered []*vault.Note
	for _, note := range notes {
		fmVal := getFrontmatterString(note, key)
		match := strings.EqualFold(fmVal, value)
		if negate {
			match = !match
		}
		if match {
			filtered = append(filtered, note)
		}
	}
	return filtered
}

// applySortOrder sorts notes in place based on the sort expression.
// Format: "field [asc|desc]". Default direction is asc.
func applySortOrder(notes []*vault.Note, sortExpr string) {
	if sortExpr == "" {
		sort.Slice(notes, func(i, j int) bool {
			return strings.ToLower(notes[i].Title) < strings.ToLower(notes[j].Title)
		})
		return
	}

	parts := strings.Fields(sortExpr)
	field := parts[0]
	desc := false
	if len(parts) > 1 && strings.ToLower(parts[1]) == "desc" {
		desc = true
	}

	sort.Slice(notes, func(i, j int) bool {
		var less bool
		switch strings.ToLower(field) {
		case "modified":
			less = notes[i].ModTime.Before(notes[j].ModTime)
		case "title":
			less = strings.ToLower(notes[i].Title) < strings.ToLower(notes[j].Title)
		case "name":
			nameI := filepath.Base(notes[i].RelPath)
			nameJ := filepath.Base(notes[j].RelPath)
			less = strings.ToLower(nameI) < strings.ToLower(nameJ)
		default:
			vi := getFrontmatterString(notes[i], field)
			vj := getFrontmatterString(notes[j], field)
			less = strings.ToLower(vi) < strings.ToLower(vj)
		}
		if desc {
			return !less
		}
		return less
	})
}

// extractField extracts a display value for the given field from a note.
func extractField(note *vault.Note, field string) string {
	switch strings.ToLower(field) {
	case "title":
		return note.Title
	case "path":
		return note.RelPath
	case "modified":
		return note.ModTime.Format("2006-01-02")
	case "date":
		if v := getFrontmatterString(note, "date"); v != "" {
			return v
		}
		return note.ModTime.Format("2006-01-02")
	case "tags":
		if note.Frontmatter == nil {
			return "-"
		}
		tagsVal, ok := note.Frontmatter["tags"]
		if !ok {
			return "-"
		}
		switch v := tagsVal.(type) {
		case []string:
			return strings.Join(v, ", ")
		case string:
			return v
		}
		return "-"
	default:
		v := getFrontmatterString(note, field)
		if v == "" {
			return "-"
		}
		return v
	}
}

// getFrontmatterString returns a frontmatter value as a string, or "" if
// the key is missing or the note has no frontmatter.
func getFrontmatterString(note *vault.Note, key string) string {
	if note.Frontmatter == nil {
		return ""
	}
	val, ok := note.Frontmatter[key]
	if !ok {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case []string:
		return strings.Join(v, ", ")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// RenderDataviewResults renders inline query results as a styled table.
// If fields is empty, it defaults to ["title", "date"].
func RenderDataviewResults(results []InlineDataviewResult, fields []string, width int) string {
	if len(fields) == 0 {
		fields = []string{"title", "date"}
	}

	if len(results) == 0 {
		return DimStyle.Render("  No matching notes found.")
	}

	// Calculate column widths
	colWidths := make([]int, len(fields))
	for i, f := range fields {
		colWidths[i] = len(f)
	}
	for _, r := range results {
		for i, f := range fields {
			val := r.Fields[f]
			if len(val) > colWidths[i] {
				colWidths[i] = len(val)
			}
		}
	}

	// Constrain total width to available space
	overhead := len(fields)*3 + 1
	maxContent := width - overhead - 2
	if maxContent < len(fields)*3 {
		maxContent = len(fields) * 3
	}

	totalContent := 0
	for _, w := range colWidths {
		totalContent += w
	}

	// Shrink columns proportionally if they exceed available space
	if totalContent > maxContent && totalContent > 0 {
		for i := range colWidths {
			colWidths[i] = colWidths[i] * maxContent / totalContent
			if colWidths[i] < 3 {
				colWidths[i] = 3
			}
		}
	}

	borderStyle := lipgloss.NewStyle().Foreground(surface0)
	headerTextStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	cellStyle := lipgloss.NewStyle().Foreground(text)

	var b strings.Builder

	// Helper to build a horizontal rule line
	buildRule := func(left, mid, right, fill string) string {
		var s strings.Builder
		s.WriteString(left)
		for i, w := range colWidths {
			s.WriteString(strings.Repeat(fill, w+2))
			if i < len(colWidths)-1 {
				s.WriteString(mid)
			}
		}
		s.WriteString(right)
		return s.String()
	}

	// Top border
	b.WriteString("  ")
	b.WriteString(borderStyle.Render(buildRule("┌", "┬", "┐", "─")))
	b.WriteString("\n")

	// Header row
	b.WriteString("  ")
	b.WriteString(borderStyle.Render("│"))
	for i, f := range fields {
		header := dvCapitalize(f)
		header = padOrTruncate(header, colWidths[i])
		b.WriteString(" ")
		b.WriteString(headerTextStyle.Render(header))
		b.WriteString(" ")
		b.WriteString(borderStyle.Render("│"))
	}
	b.WriteString("\n")

	// Header separator
	b.WriteString("  ")
	b.WriteString(borderStyle.Render(buildRule("├", "┼", "┤", "─")))
	b.WriteString("\n")

	// Data rows
	for _, r := range results {
		b.WriteString("  ")
		b.WriteString(borderStyle.Render("│"))
		for i, f := range fields {
			val := r.Fields[f]
			if val == "" || val == "-" {
				val = padOrTruncate("-", colWidths[i])
				b.WriteString(" ")
				b.WriteString(DimStyle.Render(val))
				b.WriteString(" ")
			} else {
				val = padOrTruncate(val, colWidths[i])
				b.WriteString(" ")
				b.WriteString(cellStyle.Render(val))
				b.WriteString(" ")
			}
			b.WriteString(borderStyle.Render("│"))
		}
		b.WriteString("\n")
	}

	// Bottom border
	b.WriteString("  ")
	b.WriteString(borderStyle.Render(buildRule("└", "┴", "┘", "─")))

	return b.String()
}

// padOrTruncate pads a string with spaces to the given width, or truncates
// it with an ellipsis if it exceeds the width.
func padOrTruncate(s string, width int) string {
	visWidth := lipgloss.Width(s)
	if visWidth > width {
		if width <= 3 {
			return s[:width]
		}
		truncated := ""
		for _, r := range s {
			if lipgloss.Width(truncated+string(r)+"...") > width {
				break
			}
			truncated += string(r)
		}
		return truncated + "..."
	}
	return s + strings.Repeat(" ", width-visWidth)
}
