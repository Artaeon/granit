package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/artaeon/granit/internal/vault"
	"github.com/charmbracelet/lipgloss"
)

// DataviewQuery represents a parsed query block from a markdown note.
type DataviewQuery struct {
	From   string   // filter expression: "tags:xxx", "folder:xxx", "all"
	Where  string   // property filter: "status = active"
	Sort   string   // sort field + direction: "modified desc", "title asc"
	Limit  int      // max results
	Fields []string // fields to display: "title", "date", "status", etc.
}

// DataviewResult holds a single note result from a query execution.
type DataviewResult struct {
	Path   string
	Title  string
	Fields map[string]string // field name -> value
}

// Dataview is a pure function component — no state needed.
type Dataview struct{}

// ParseDataviewQuery parses the content of a ```query code block into a
// DataviewQuery. Lines are expected to be key: value pairs.
func ParseDataviewQuery(block string) *DataviewQuery {
	q := &DataviewQuery{
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

// ExecuteDataviewQuery runs a query against the vault notes and returns
// matching results with the requested fields extracted.
func ExecuteDataviewQuery(query *DataviewQuery, notes map[string]*vault.Note) []DataviewResult {
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

	results := make([]DataviewResult, 0, len(candidates))
	for _, note := range candidates {
		r := DataviewResult{
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
		// Tags might be a comma-separated string
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
		// Default: sort by title ascending
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
			// Sort by frontmatter property
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
		// Try frontmatter "date" first, fall back to modified time
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

// RenderDataviewResults renders query results as a styled table.
// If fields is empty, it defaults to ["title", "date"].
func RenderDataviewResults(results []DataviewResult, fields []string, width int) string {
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

	// Constrain total width to available space (accounting for borders and separators)
	// Each column has: "│ " + content + " " = 3 chars overhead, plus final "│" = 1
	overhead := len(fields)*3 + 1
	maxContent := width - overhead - 2 // 2 for left margin
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
		header := capitalize(f)
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
		// Truncate rune-safe by iterating
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

// capitalize returns the string with its first letter uppercased.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
