package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/vault"
)

// ═══════════════════════════════════════════════════════════════════════════
// Dataview Overlay — interactive query builder and results viewer
// ═══════════════════════════════════════════════════════════════════════════

// ---------------------------------------------------------------------------
// Query types
// ---------------------------------------------------------------------------

// DataviewQuery describes what to search for and how to present results.
type DataviewQuery struct {
	Source        string        // "vault" or a folder path
	Filters      []queryFilter // frontmatter filters
	SortBy       string        // field name to sort on
	SortDesc     bool          // true = descending
	DisplayFields []string     // frontmatter keys shown in output
	Limit        int           // max results
	ViewType     string        // "table" or "list"
}

// queryFilter is a single predicate applied to every candidate note.
type queryFilter struct {
	Field string // frontmatter key or virtual field
	Op    string // =, !=, contains, >, <, >=, <=
	Value string
}

// dvNoteResult is one row in the result set.
type dvNoteResult struct {
	Path        string
	Title       string
	Frontmatter map[string]string
	Created     time.Time
	Modified    time.Time
	WordCount   int
	Tags        []string
}

// ---------------------------------------------------------------------------
// Overlay
// ---------------------------------------------------------------------------

// DataviewOverlay provides a query-builder UI and a results viewer.
type DataviewOverlay struct {
	active    bool
	width     int
	height    int
	vaultRoot string

	// Phases: 0 = builder, 1 = results.
	phase int

	query   DataviewQuery
	results []dvNoteResult

	// Results navigation
	cursor int
	scroll int

	// Builder navigation
	builderField int // which row is focused (0-6)

	// Filter list
	filterCursor int

	// Adding-filter sub-mode
	addingFilter     bool
	filterFieldBuf   string
	filterOpBuf      string
	filterValueBuf   string
	filterInputField int // 0=field, 1=op, 2=value

	// Consumed-once selected note
	selectedNote string
	hasResult    bool
}

// Operator choices available in the filter operator selector.
var dvFilterOps = []string{"=", "!=", "contains", ">", "<", ">=", "<="}

// Limit presets cycled by left/right in the builder.
var dvLimitPresets = []int{10, 25, 50, 100}

// NewDataviewOverlay returns a zero-value overlay ready to be opened.
func NewDataviewOverlay() DataviewOverlay {
	return DataviewOverlay{
		query: DataviewQuery{
			Source:        "vault",
			Limit:         25,
			ViewType:      "table",
			DisplayFields: []string{"title", "tags", "path"},
			SortBy:        "title",
		},
	}
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Open initialises the overlay with the given vault root directory.
func (d *DataviewOverlay) Open(vaultRoot string) {
	d.active = true
	d.vaultRoot = vaultRoot
	d.phase = 0
	d.cursor = 0
	d.scroll = 0
	d.builderField = 0
	d.filterCursor = 0
	d.addingFilter = false
	d.filterFieldBuf = ""
	d.filterOpBuf = "="
	d.filterValueBuf = ""
	d.filterInputField = 0
	d.selectedNote = ""
	d.hasResult = false
	d.results = nil
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
		if d.phase == 0 {
			return d.updateBuilder(msg)
		}
		return d.updateResults(msg)
	}
	return d, nil
}

// --- builder phase ---------------------------------------------------------

// Builder rows:
//
//	0 = Source
//	1 = Filters
//	2 = Sort By
//	3 = Sort Direction
//	4 = Display Fields
//	5 = View Type
//	6 = Limit
const (
	dvFieldSource     = 0
	dvFieldFilters    = 1
	dvFieldSortBy     = 2
	dvFieldSortDir    = 3
	dvFieldDisplay    = 4
	dvFieldViewType   = 5
	dvFieldLimit      = 6
	dvBuilderRowCount = 7
)

func (d DataviewOverlay) updateBuilder(msg tea.KeyMsg) (DataviewOverlay, tea.Cmd) {
	key := msg.String()

	// If we are in the add-filter sub-mode handle it separately.
	if d.addingFilter {
		return d.updateAddFilter(msg)
	}

	switch key {
	case "esc", "q":
		d.active = false
		return d, nil

	case "tab", "down", "j":
		d.builderField = (d.builderField + 1) % dvBuilderRowCount
	case "shift+tab", "up", "k":
		d.builderField = (d.builderField - 1 + dvBuilderRowCount) % dvBuilderRowCount

	case "enter":
		d.executeQuery()
		d.phase = 1
		d.cursor = 0
		d.scroll = 0
		return d, nil

	case "a":
		if d.builderField == dvFieldFilters {
			d.addingFilter = true
			d.filterFieldBuf = ""
			d.filterOpBuf = "="
			d.filterValueBuf = ""
			d.filterInputField = 0
		}

	case "d":
		if d.builderField == dvFieldFilters && len(d.query.Filters) > 0 {
			idx := d.filterCursor
			if idx >= 0 && idx < len(d.query.Filters) {
				d.query.Filters = append(d.query.Filters[:idx], d.query.Filters[idx+1:]...)
				if d.filterCursor >= len(d.query.Filters) && d.filterCursor > 0 {
					d.filterCursor--
				}
			}
		}

	case "left", "h":
		d.builderLeft()
	case "right", "l":
		d.builderRight()

	default:
		d.builderType(msg)
	}

	return d, nil
}

func (d *DataviewOverlay) builderLeft() {
	switch d.builderField {
	case dvFieldSource:
		d.query.Source = "vault"
	case dvFieldFilters:
		if d.filterCursor > 0 {
			d.filterCursor--
		}
	case dvFieldSortDir:
		d.query.SortDesc = !d.query.SortDesc
	case dvFieldViewType:
		if d.query.ViewType == "list" {
			d.query.ViewType = "table"
		} else {
			d.query.ViewType = "list"
		}
	case dvFieldLimit:
		for i, v := range dvLimitPresets {
			if v == d.query.Limit {
				if i > 0 {
					d.query.Limit = dvLimitPresets[i-1]
				}
				return
			}
		}
		d.query.Limit = dvLimitPresets[0]
	}
}

func (d *DataviewOverlay) builderRight() {
	switch d.builderField {
	case dvFieldFilters:
		if d.filterCursor < len(d.query.Filters)-1 {
			d.filterCursor++
		}
	case dvFieldSortDir:
		d.query.SortDesc = !d.query.SortDesc
	case dvFieldViewType:
		if d.query.ViewType == "table" {
			d.query.ViewType = "list"
		} else {
			d.query.ViewType = "table"
		}
	case dvFieldLimit:
		for i, v := range dvLimitPresets {
			if v == d.query.Limit {
				if i < len(dvLimitPresets)-1 {
					d.query.Limit = dvLimitPresets[i+1]
				}
				return
			}
		}
		d.query.Limit = dvLimitPresets[len(dvLimitPresets)-1]
	}
}

func (d *DataviewOverlay) builderType(msg tea.KeyMsg) {
	key := msg.String()
	if len(key) != 1 {
		if key == "backspace" {
			switch d.builderField {
			case dvFieldSource:
				if len(d.query.Source) > 0 {
					d.query.Source = d.query.Source[:len(d.query.Source)-1]
				}
			case dvFieldSortBy:
				if len(d.query.SortBy) > 0 {
					d.query.SortBy = d.query.SortBy[:len(d.query.SortBy)-1]
				}
			case dvFieldDisplay:
				raw := strings.Join(d.query.DisplayFields, ", ")
				if len(raw) > 0 {
					raw = raw[:len(raw)-1]
					d.query.DisplayFields = dvParseCSV(raw)
				}
			}
		}
		return
	}
	ch := key
	switch d.builderField {
	case dvFieldSource:
		d.query.Source += ch
	case dvFieldSortBy:
		d.query.SortBy += ch
	case dvFieldDisplay:
		raw := strings.Join(d.query.DisplayFields, ", ") + ch
		d.query.DisplayFields = dvParseCSV(raw)
	}
}

// --- add filter sub-mode ---------------------------------------------------

func (d DataviewOverlay) updateAddFilter(msg tea.KeyMsg) (DataviewOverlay, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		d.addingFilter = false
		return d, nil

	case "tab":
		d.filterInputField = (d.filterInputField + 1) % 3
		return d, nil

	case "shift+tab":
		d.filterInputField = (d.filterInputField - 1 + 3) % 3
		return d, nil

	case "enter":
		if d.filterFieldBuf != "" {
			d.query.Filters = append(d.query.Filters, queryFilter{
				Field: strings.TrimSpace(d.filterFieldBuf),
				Op:    d.filterOpBuf,
				Value: strings.TrimSpace(d.filterValueBuf),
			})
			d.filterCursor = len(d.query.Filters) - 1
		}
		d.addingFilter = false
		return d, nil

	case "left":
		if d.filterInputField == 1 {
			for i, op := range dvFilterOps {
				if op == d.filterOpBuf && i > 0 {
					d.filterOpBuf = dvFilterOps[i-1]
					break
				}
			}
		}
		return d, nil

	case "right":
		if d.filterInputField == 1 {
			for i, op := range dvFilterOps {
				if op == d.filterOpBuf && i < len(dvFilterOps)-1 {
					d.filterOpBuf = dvFilterOps[i+1]
					break
				}
			}
		}
		return d, nil

	case "backspace":
		switch d.filterInputField {
		case 0:
			if len(d.filterFieldBuf) > 0 {
				d.filterFieldBuf = d.filterFieldBuf[:len(d.filterFieldBuf)-1]
			}
		case 2:
			if len(d.filterValueBuf) > 0 {
				d.filterValueBuf = d.filterValueBuf[:len(d.filterValueBuf)-1]
			}
		}
		return d, nil

	default:
		if len(key) == 1 {
			switch d.filterInputField {
			case 0:
				d.filterFieldBuf += key
			case 2:
				d.filterValueBuf += key
			}
		}
	}

	return d, nil
}

// --- results phase ---------------------------------------------------------

func (d DataviewOverlay) updateResults(msg tea.KeyMsg) (DataviewOverlay, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		d.phase = 0
		return d, nil
	case "q":
		d.active = false
		return d, nil

	case "up", "k":
		if d.cursor > 0 {
			d.cursor--
			if d.cursor < d.scroll {
				d.scroll = d.cursor
			}
		}
	case "down", "j":
		if d.cursor < len(d.results)-1 {
			d.cursor++
			visH := d.visibleRows()
			if d.cursor >= d.scroll+visH {
				d.scroll = d.cursor - visH + 1
			}
		}
	case "enter":
		if len(d.results) > 0 && d.cursor < len(d.results) {
			d.selectedNote = d.results[d.cursor].Path
			d.hasResult = true
			d.active = false
		}
		return d, nil
	}

	return d, nil
}

func (d DataviewOverlay) visibleRows() int {
	h := d.height - 14
	if h < 1 {
		h = 1
	}
	return h
}

// ---------------------------------------------------------------------------
// Query execution
// ---------------------------------------------------------------------------

func (d *DataviewOverlay) executeQuery() {
	d.results = nil

	root := d.vaultRoot
	if d.query.Source != "vault" && d.query.Source != "" {
		candidate := d.query.Source
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(d.vaultRoot, candidate)
		}
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			root = candidate
		}
	}

	var notes []dvNoteResult
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Skip hidden directories.
			if strings.HasPrefix(info.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		content := string(data)

		relPath, _ := filepath.Rel(d.vaultRoot, path)
		if relPath == "" {
			relPath = path
		}

		fm := parseFrontmatter(content)
		title := dvExtractTitle(content, info.Name())
		tags := dvExtractTags(fm, content)
		wc := len(strings.Fields(content))

		nr := dvNoteResult{
			Path:        relPath,
			Title:       title,
			Frontmatter: fm,
			Created:     info.ModTime(),
			Modified:    info.ModTime(),
			WordCount:   wc,
			Tags:        tags,
		}

		// Store virtual fields into frontmatter map for uniform access.
		nr.Frontmatter["title"] = nr.Title
		nr.Frontmatter["path"] = nr.Path
		nr.Frontmatter["words"] = fmt.Sprintf("%d", nr.WordCount)
		nr.Frontmatter["created"] = nr.Created.Format("2006-01-02")
		nr.Frontmatter["modified"] = nr.Modified.Format("2006-01-02")
		nr.Frontmatter["folder"] = filepath.Base(filepath.Dir(path))
		if _, ok := nr.Frontmatter["tags"]; !ok {
			nr.Frontmatter["tags"] = strings.Join(nr.Tags, ", ")
		}

		notes = append(notes, nr)
		return nil
	})

	// Apply filters.
	var filtered []dvNoteResult
	for _, n := range notes {
		if d.matchesAllFilters(n) {
			filtered = append(filtered, n)
		}
	}

	// Sort.
	d.sortResults(filtered)

	// Limit.
	if d.query.Limit > 0 && len(filtered) > d.query.Limit {
		filtered = filtered[:d.query.Limit]
	}

	d.results = filtered
}

func (d *DataviewOverlay) matchesAllFilters(n dvNoteResult) bool {
	for _, f := range d.query.Filters {
		if !d.matchFilter(n, f) {
			return false
		}
	}
	return true
}

func (d *DataviewOverlay) matchFilter(n dvNoteResult, f queryFilter) bool {
	val := dvFieldValue(n, f.Field)
	target := f.Value

	switch f.Op {
	case "=":
		return strings.EqualFold(val, target)
	case "!=":
		return !strings.EqualFold(val, target)
	case "contains":
		return strings.Contains(strings.ToLower(val), strings.ToLower(target))
	case ">":
		return val > target
	case "<":
		return val < target
	case ">=":
		return val >= target
	case "<=":
		return val <= target
	}
	return true
}

func dvFieldValue(n dvNoteResult, field string) string {
	field = strings.ToLower(strings.TrimSpace(field))
	switch field {
	case "title":
		return n.Title
	case "path":
		return n.Path
	case "words":
		return fmt.Sprintf("%d", n.WordCount)
	case "created":
		return n.Created.Format("2006-01-02")
	case "modified":
		return n.Modified.Format("2006-01-02")
	case "folder":
		return filepath.Base(filepath.Dir(n.Path))
	case "tags":
		return strings.Join(n.Tags, ", ")
	default:
		if v, ok := n.Frontmatter[field]; ok {
			return v
		}
	}
	return ""
}

func (d *DataviewOverlay) sortResults(results []dvNoteResult) {
	field := strings.ToLower(strings.TrimSpace(d.query.SortBy))
	if field == "" {
		field = "title"
	}
	desc := d.query.SortDesc

	sort.SliceStable(results, func(i, j int) bool {
		a := dvFieldValue(results[i], field)
		b := dvFieldValue(results[j], field)
		if desc {
			return a > b
		}
		return a < b
	})
}

// ---------------------------------------------------------------------------
// Frontmatter parser
// ---------------------------------------------------------------------------

// parseFrontmatter extracts YAML frontmatter from markdown content.
// It handles simple key: value pairs and tags in both inline and list format.
func parseFrontmatter(content string) map[string]string {
	fm := make(map[string]string)

	lines := strings.Split(content, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return fm
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return fm
	}

	// Track the current key for multi-line list values (e.g. tags).
	currentKey := ""
	var listValues []string

	flushList := func() {
		if currentKey != "" && len(listValues) > 0 {
			fm[currentKey] = strings.Join(listValues, ", ")
		}
		currentKey = ""
		listValues = nil
	}

	kvRe := regexp.MustCompile(`^([A-Za-z0-9_-]+)\s*:\s*(.*)$`)
	listItemRe := regexp.MustCompile(`^\s+-\s+(.+)$`)

	for i := 1; i < endIdx; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		// Check for list item first (indented "- value").
		if m := listItemRe.FindStringSubmatch(line); m != nil {
			if currentKey != "" {
				listValues = append(listValues, strings.TrimSpace(m[1]))
			}
			continue
		}

		// Flush any pending list.
		flushList()

		// Match key: value
		if m := kvRe.FindStringSubmatch(trimmed); m != nil {
			key := strings.ToLower(m[1])
			val := strings.TrimSpace(m[2])

			// Strip surrounding quotes.
			val = strings.Trim(val, "\"'")

			if val == "" {
				// Possibly followed by list items.
				currentKey = key
				listValues = nil
			} else {
				fm[key] = val
			}
		}
	}
	flushList()

	return fm
}

// dvExtractTitle finds the first # heading or falls back to filename.
func dvExtractTitle(content string, filename string) string {
	lines := strings.Split(content, "\n")

	// Skip frontmatter if present.
	start := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				start = i + 1
				break
			}
		}
	}

	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(trimmed[2:])
		}
	}

	return strings.TrimSuffix(filename, ".md")
}

// dvExtractTags collects tags from frontmatter and inline #hashtags.
func dvExtractTags(fm map[string]string, content string) []string {
	seen := make(map[string]bool)
	var tags []string

	// From frontmatter.
	if raw, ok := fm["tags"]; ok && raw != "" {
		for _, t := range strings.Split(raw, ",") {
			t = strings.TrimSpace(t)
			if t != "" && !seen[t] {
				seen[t] = true
				tags = append(tags, t)
			}
		}
	}

	// Inline #tags.
	words := strings.Fields(content)
	for _, w := range words {
		if strings.HasPrefix(w, "#") && len(w) > 1 {
			tag := strings.TrimRight(w[1:], ".,;:!?)")
			if tag != "" && !strings.HasPrefix(tag, "#") && !seen[tag] {
				seen[tag] = true
				tags = append(tags, tag)
			}
		}
	}

	return tags
}

// dvParseCSV splits a comma-separated string into trimmed non-empty tokens.
func dvParseCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the overlay (builder or results depending on phase).
func (d DataviewOverlay) View() string {
	width := d.width * 2 / 3
	if width < 70 {
		width = 70
	}
	if width > 110 {
		width = 110
	}
	innerWidth := width - 6

	if d.phase == 0 {
		return d.viewBuilder(width, innerWidth)
	}
	return d.viewResults(width, innerWidth)
}

// --- builder view ----------------------------------------------------------

func (d DataviewOverlay) viewBuilder(width, innerWidth int) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render(IconSearchChar + "  Dataview Query Builder"))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n\n")

	labelStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true).Width(18)
	valueStyle := lipgloss.NewStyle().Foreground(text)
	activeLabel := lipgloss.NewStyle().Foreground(mauve).Bold(true).Width(18)
	activeValue := lipgloss.NewStyle().Foreground(peach).Bold(true)
	hintStyle := lipgloss.NewStyle().Foreground(overlay0)

	row := func(idx int, label, value, hint string) {
		ls := labelStyle
		vs := valueStyle
		prefix := "  "
		if idx == d.builderField {
			ls = activeLabel
			vs = activeValue
			prefix = lipgloss.NewStyle().Foreground(mauve).Render("▸ ")
		}
		line := prefix + ls.Render(label) + vs.Render(value)
		if hint != "" && idx == d.builderField {
			line += "  " + hintStyle.Render(hint)
		}
		b.WriteString(line + "\n")
	}

	// 0: Source
	srcDisplay := d.query.Source
	if srcDisplay == "vault" || srcDisplay == "" {
		srcDisplay = "Entire Vault"
	}
	row(dvFieldSource, "Source:", srcDisplay, "type path or 'vault'")

	b.WriteString("\n")

	// 1: Filters
	if d.builderField == dvFieldFilters {
		b.WriteString(lipgloss.NewStyle().Foreground(mauve).Render("▸ "))
		b.WriteString(activeLabel.Render("Filters:"))
	} else {
		b.WriteString("  ")
		b.WriteString(labelStyle.Render("Filters:"))
	}
	if len(d.query.Filters) == 0 {
		b.WriteString(hintStyle.Render("(none)"))
	}
	b.WriteString("\n")

	for i, f := range d.query.Filters {
		prefix := "    "
		fStyle := lipgloss.NewStyle().Foreground(text)
		if d.builderField == dvFieldFilters && i == d.filterCursor {
			fStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
			prefix = "   " + lipgloss.NewStyle().Foreground(mauve).Render("›") + " "
		}
		opStyle := lipgloss.NewStyle().Foreground(sapphire)
		line := prefix + fStyle.Render(f.Field) + " " + opStyle.Render(f.Op) + " " + fStyle.Render(f.Value)
		b.WriteString(line + "\n")
	}

	if d.addingFilter {
		b.WriteString(d.viewAddFilter(innerWidth))
	} else if d.builderField == dvFieldFilters {
		b.WriteString(hintStyle.Render("    a: add filter  d: delete") + "\n")
	}

	b.WriteString("\n")

	// 2: Sort By
	sortDisplay := d.query.SortBy
	if sortDisplay == "" {
		sortDisplay = "title"
	}
	row(dvFieldSortBy, "Sort By:", sortDisplay, "type field name")

	// 3: Sort Direction
	dirDisplay := "ascending"
	if d.query.SortDesc {
		dirDisplay = "descending"
	}
	row(dvFieldSortDir, "Sort Direction:", dirDisplay, "left/right to toggle")

	b.WriteString("\n")

	// 4: Display Fields
	dfDisplay := strings.Join(d.query.DisplayFields, ", ")
	if dfDisplay == "" {
		dfDisplay = "title, tags, path"
	}
	row(dvFieldDisplay, "Display Fields:", dfDisplay, "comma-separated")

	// 5: View Type
	row(dvFieldViewType, "View Type:", d.query.ViewType, "left/right to toggle")

	// 6: Limit
	row(dvFieldLimit, "Limit:", fmt.Sprintf("%d", d.query.Limit), "left/right to adjust")

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	keyStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(overlay0)
	sepStyle := lipgloss.NewStyle().Foreground(surface1)
	sep := sepStyle.Render(" | ")

	b.WriteString("  " +
		keyStyle.Render("Tab") + descStyle.Render(" navigate") + sep +
		keyStyle.Render("Enter") + descStyle.Render(" run query") + sep +
		keyStyle.Render("Esc") + descStyle.Render(" close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (d DataviewOverlay) viewAddFilter(innerWidth int) string {
	var b strings.Builder

	boxStyle := lipgloss.NewStyle().
		Foreground(text).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(surface1).
		Padding(0, 1).
		Width(innerWidth - 6).
		MarginLeft(4)

	labelStyle := lipgloss.NewStyle().Foreground(lavender).Width(10)
	activeStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(text)

	var inner strings.Builder
	inner.WriteString(lipgloss.NewStyle().Foreground(green).Bold(true).Render("New Filter") + "\n")

	// Field name
	vs := normalStyle
	if d.filterInputField == 0 {
		vs = activeStyle
	}
	fieldDisplay := d.filterFieldBuf
	if d.filterInputField == 0 {
		fieldDisplay += "│"
	}
	if fieldDisplay == "" && d.filterInputField != 0 {
		fieldDisplay = "(empty)"
	}
	inner.WriteString(labelStyle.Render("Field:") + vs.Render(fieldDisplay) + "\n")

	// Operator
	vs = normalStyle
	if d.filterInputField == 1 {
		vs = activeStyle
	}
	opDisplay := d.filterOpBuf
	if d.filterInputField == 1 {
		opDisplay = "◂ " + opDisplay + " ▸"
	}
	inner.WriteString(labelStyle.Render("Operator:") + vs.Render(opDisplay) + "\n")

	// Value
	vs = normalStyle
	if d.filterInputField == 2 {
		vs = activeStyle
	}
	valDisplay := d.filterValueBuf
	if d.filterInputField == 2 {
		valDisplay += "│"
	}
	if valDisplay == "" && d.filterInputField != 2 {
		valDisplay = "(empty)"
	}
	inner.WriteString(labelStyle.Render("Value:") + vs.Render(valDisplay) + "\n")

	hintStyle := lipgloss.NewStyle().Foreground(overlay0)
	inner.WriteString(hintStyle.Render("Tab: next field  Enter: add  Esc: cancel"))

	b.WriteString(boxStyle.Render(inner.String()))
	b.WriteString("\n")

	return b.String()
}

// --- results view ----------------------------------------------------------

func (d DataviewOverlay) viewResults(width, innerWidth int) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render(IconGraphChar + "  Query Results"))

	countStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(countStyle.Render(fmt.Sprintf("  %d notes", len(d.results))))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	if len(d.results) == 0 {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  No matching notes found."))
		b.WriteString("\n")
	} else if d.query.ViewType == "table" {
		b.WriteString(d.renderTable(innerWidth))
	} else {
		b.WriteString(d.renderList(innerWidth))
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	keyStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(overlay0)
	sepStyle := lipgloss.NewStyle().Foreground(surface1)
	sep := sepStyle.Render(" | ")

	b.WriteString("  " +
		keyStyle.Render("j/k") + descStyle.Render(" navigate") + sep +
		keyStyle.Render("Enter") + descStyle.Render(" open") + sep +
		keyStyle.Render("Esc") + descStyle.Render(" back") + sep +
		keyStyle.Render("q") + descStyle.Render(" close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// renderTable draws an ASCII-bordered table of results.
func (d DataviewOverlay) renderTable(innerWidth int) string {
	fields := d.query.DisplayFields
	if len(fields) == 0 {
		fields = []string{"title", "tags", "path"}
	}

	numCols := len(fields)
	colWidth := (innerWidth - numCols - 1) / numCols
	if colWidth < 8 {
		colWidth = 8
	}

	var b strings.Builder

	// Top border
	b.WriteString("  " + dvCorner("tl"))
	for i := 0; i < numCols; i++ {
		b.WriteString(strings.Repeat("─", colWidth))
		if i < numCols-1 {
			b.WriteString(dvCorner("tc"))
		}
	}
	b.WriteString(dvCorner("tr") + "\n")

	// Header row
	headerStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	b.WriteString("  │")
	for _, f := range fields {
		label := dvTruncate(dvCapitalize(f), colWidth-2)
		padded := dvPadField(label, colWidth-2)
		b.WriteString(" " + headerStyle.Render(padded) + " │")
	}
	b.WriteString("\n")

	// Header separator
	b.WriteString("  " + dvCorner("ml"))
	for i := 0; i < numCols; i++ {
		b.WriteString(strings.Repeat("─", colWidth))
		if i < numCols-1 {
			b.WriteString(dvCorner("mc"))
		}
	}
	b.WriteString(dvCorner("mr") + "\n")

	// Data rows
	visH := d.visibleRows()
	end := d.scroll + visH
	if end > len(d.results) {
		end = len(d.results)
	}

	for idx := d.scroll; idx < end; idx++ {
		nr := d.results[idx]
		isSelected := idx == d.cursor

		rowStyle := lipgloss.NewStyle().Foreground(text)
		if isSelected {
			rowStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
		}

		prefix := "  │"
		if isSelected {
			prefix = lipgloss.NewStyle().Foreground(mauve).Render("▸ ") + "│"
		}
		b.WriteString(prefix)

		for _, f := range fields {
			val := dvFieldValue(nr, f)
			truncated := dvTruncate(val, colWidth-2)
			padded := dvPadField(truncated, colWidth-2)
			b.WriteString(" " + rowStyle.Render(padded) + " │")
		}
		b.WriteString("\n")
	}

	// Bottom border
	b.WriteString("  " + dvCorner("bl"))
	for i := 0; i < numCols; i++ {
		b.WriteString(strings.Repeat("─", colWidth))
		if i < numCols-1 {
			b.WriteString(dvCorner("bc"))
		}
	}
	b.WriteString(dvCorner("br"))

	return b.String()
}

// renderList draws one note per row with inline field values.
func (d DataviewOverlay) renderList(innerWidth int) string {
	fields := d.query.DisplayFields
	if len(fields) == 0 {
		fields = []string{"title", "tags", "path"}
	}

	var b strings.Builder

	visH := d.visibleRows()
	end := d.scroll + visH
	if end > len(d.results) {
		end = len(d.results)
	}

	for idx := d.scroll; idx < end; idx++ {
		nr := d.results[idx]
		isSelected := idx == d.cursor

		titleStyle := lipgloss.NewStyle().Foreground(text)
		metaStyle := lipgloss.NewStyle().Foreground(overlay0)
		if isSelected {
			titleStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
			metaStyle = lipgloss.NewStyle().Foreground(lavender)
		}

		prefix := "  "
		if isSelected {
			prefix = lipgloss.NewStyle().Foreground(mauve).Render("▸ ")
		}

		// Title line
		titleVal := dvFieldValue(nr, "title")
		b.WriteString(prefix + titleStyle.Render(dvTruncate(titleVal, innerWidth-4)) + "\n")

		// Meta line: remaining fields
		var meta []string
		for _, f := range fields {
			if strings.ToLower(f) == "title" {
				continue
			}
			val := dvFieldValue(nr, f)
			if val != "" {
				meta = append(meta, f+": "+val)
			}
		}
		if len(meta) > 0 {
			metaLine := strings.Join(meta, "  ")
			b.WriteString("    " + metaStyle.Render(dvTruncate(metaLine, innerWidth-6)) + "\n")
		}

		if idx < end-1 {
			b.WriteString(lipgloss.NewStyle().Foreground(surface0).
				Render("  "+strings.Repeat("·", innerWidth-4)) + "\n")
		}
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Table drawing helpers
// ---------------------------------------------------------------------------

// dvCorner returns a styled box-drawing character for the named position.
func dvCorner(pos string) string {
	s := lipgloss.NewStyle().Foreground(surface1)
	switch pos {
	case "tl":
		return s.Render("┌")
	case "tr":
		return s.Render("┐")
	case "bl":
		return s.Render("└")
	case "br":
		return s.Render("┘")
	case "tc":
		return s.Render("┬")
	case "bc":
		return s.Render("┴")
	case "ml":
		return s.Render("├")
	case "mr":
		return s.Render("┤")
	case "mc":
		return s.Render("┼")
	}
	return s.Render("│")
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
