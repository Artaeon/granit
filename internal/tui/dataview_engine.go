package tui

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/artaeon/granit/internal/vault"
)

// ═══════════════════════════════════════════════════════════════════════════
// Dataview Engine — executes parsed queries against vault notes
// ═══════════════════════════════════════════════════════════════════════════

// DVResult is a single row returned by the query engine.
type DVResult struct {
	NotePath string            // relative path to the note
	Title    string            // note title
	Fields   map[string]string // field name -> display value
}

// DVTaskResult is a single task item extracted from notes.
type DVTaskResult struct {
	NotePath string // relative path to the source note
	NoteTitle string
	Text      string // task text (without the checkbox prefix)
	Completed bool   // true if [x]
	Line      int    // 0-based line number in the note
}

// DVQueryResult holds the full output of a query execution.
type DVQueryResult struct {
	Mode    DVQueryMode
	Fields  []string       // column headers for TABLE mode
	Rows    []DVResult     // results for TABLE/LIST modes
	Tasks   []DVTaskResult // results for TASK mode
	Total   int            // total matching notes before limit
}

// taskLineRe matches markdown task list items: "- [ ] text" or "- [x] text".
var taskLineRe = regexp.MustCompile(`^(\s*)-\s*\[([ xX])\]\s*(.*)$`)

// ExecuteDVQuery runs a parsed query against the vault and returns results.
func ExecuteDVQuery(query *DVParsedQuery, v *vault.Vault) DVQueryResult {
	result := DVQueryResult{
		Mode:   query.Mode,
		Fields: query.Fields,
	}

	if v == nil {
		return result
	}

	// Collect candidate notes
	candidates := collectCandidates(query, v)

	// Apply WHERE filters
	candidates = filterCandidates(candidates, query.Conditions)

	result.Total = len(candidates)

	// Sort
	sortCandidates(candidates, query.Sort)

	// Apply limit
	if query.Limit > 0 && len(candidates) > query.Limit {
		candidates = candidates[:query.Limit]
	}

	// Build output based on mode
	switch query.Mode {
	case DVModeTable:
		result.Rows = buildTableResults(candidates, query.Fields)
	case DVModeList:
		result.Rows = buildListResults(candidates)
	case DVModeTask:
		result.Tasks = buildTaskResults(candidates)
	}

	return result
}

// collectCandidates gathers notes matching the FROM clause.
func collectCandidates(query *DVParsedQuery, v *vault.Vault) []*vault.Note {
	var candidates []*vault.Note

	for relPath := range v.Notes {
		note := v.GetNote(relPath)
		if note == nil {
			continue
		}

		// FROM folder filter
		if query.Source != "" {
			noteDir := filepath.Dir(note.RelPath)
			folder := query.Source
			if noteDir != folder && !strings.HasPrefix(noteDir, folder+string(filepath.Separator)) {
				continue
			}
		}

		// FROM #tag filter
		if query.SourceTag != "" {
			if !dvNoteHasTag(note, query.SourceTag) {
				continue
			}
		}

		candidates = append(candidates, note)
	}

	return candidates
}

// filterCandidates applies WHERE conditions to the candidate list.
func filterCandidates(candidates []*vault.Note, conditions []DVCondition) []*vault.Note {
	if len(conditions) == 0 {
		return candidates
	}

	var filtered []*vault.Note
	for _, note := range candidates {
		if matchesAllConditions(note, conditions) {
			filtered = append(filtered, note)
		}
	}
	return filtered
}

// matchesAllConditions checks whether a note satisfies all WHERE conditions.
func matchesAllConditions(note *vault.Note, conditions []DVCondition) bool {
	for _, cond := range conditions {
		if !matchCondition(note, cond) {
			return false
		}
	}
	return true
}

// matchCondition evaluates a single condition against a note.
func matchCondition(note *vault.Note, cond DVCondition) bool {
	val := dvGetField(note, cond.Field)
	target := cond.Value
	result := false

	switch cond.Op {
	case "=":
		result = strings.EqualFold(val, target)
	case "!=":
		result = !strings.EqualFold(val, target)
	case "CONTAINS":
		// For tags field, check individual tag membership
		if strings.ToLower(cond.Field) == "tags" {
			result = dvNoteHasTag(note, target)
		} else {
			result = strings.Contains(strings.ToLower(val), strings.ToLower(target))
		}
	case ">":
		result = val > target
	case "<":
		result = val < target
	case ">=":
		result = val >= target
	case "<=":
		result = val <= target
	default:
		result = true
	}

	if cond.Negate {
		return !result
	}
	return result
}

// sortCandidates sorts notes based on the SORT clause.
func sortCandidates(candidates []*vault.Note, s *DVSort) {
	if s == nil {
		// Default sort by title
		sort.SliceStable(candidates, func(i, j int) bool {
			return strings.ToLower(candidates[i].Title) < strings.ToLower(candidates[j].Title)
		})
		return
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		a := dvGetField(candidates[i], s.Field)
		b := dvGetField(candidates[j], s.Field)
		if s.Desc {
			return a > b
		}
		return a < b
	})
}

// buildTableResults creates table rows with the requested fields.
func buildTableResults(candidates []*vault.Note, fields []string) []DVResult {
	if len(fields) == 0 {
		fields = []string{"title", "path"}
	}

	results := make([]DVResult, 0, len(candidates))
	for _, note := range candidates {
		r := DVResult{
			NotePath: note.RelPath,
			Title:    note.Title,
			Fields:   make(map[string]string),
		}
		for _, f := range fields {
			r.Fields[f] = dvGetField(note, f)
		}
		results = append(results, r)
	}
	return results
}

// buildListResults creates list entries (title + path).
func buildListResults(candidates []*vault.Note) []DVResult {
	results := make([]DVResult, 0, len(candidates))
	for _, note := range candidates {
		r := DVResult{
			NotePath: note.RelPath,
			Title:    note.Title,
			Fields: map[string]string{
				"title": note.Title,
				"path":  note.RelPath,
			},
		}
		results = append(results, r)
	}
	return results
}

// buildTaskResults extracts task items from matching notes.
func buildTaskResults(candidates []*vault.Note) []DVTaskResult {
	var tasks []DVTaskResult
	for _, note := range candidates {
		lines := strings.Split(note.Content, "\n")
		for lineIdx, line := range lines {
			m := taskLineRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			completed := m[2] == "x" || m[2] == "X"
			tasks = append(tasks, DVTaskResult{
				NotePath:  note.RelPath,
				NoteTitle: note.Title,
				Text:      strings.TrimSpace(m[3]),
				Completed: completed,
				Line:      lineIdx,
			})
		}
	}
	return tasks
}

// dvGetField extracts a field value from a note for display or comparison.
func dvGetField(note *vault.Note, field string) string {
	field = strings.ToLower(strings.TrimSpace(field))
	switch field {
	case "title", "name":
		return note.Title
	case "path", "file":
		return note.RelPath
	case "folder":
		return filepath.Dir(note.RelPath)
	case "date":
		if v := dvFrontmatterStr(note, "date"); v != "" {
			return v
		}
		return note.ModTime.Format("2006-01-02")
	case "modified":
		return note.ModTime.Format("2006-01-02")
	case "created":
		return note.ModTime.Format("2006-01-02")
	case "tags":
		return dvTagsString(note)
	case "words", "wordcount":
		return fmt.Sprintf("%d", len(strings.Fields(note.Content)))
	case "size":
		return fmt.Sprintf("%d", note.Size)
	default:
		return dvFrontmatterStr(note, field)
	}
}

// dvFrontmatterStr returns a frontmatter value as a string.
func dvFrontmatterStr(note *vault.Note, key string) string {
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
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, fmt.Sprintf("%v", item))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// dvTagsString returns a comma-separated string of all tags for a note.
func dvTagsString(note *vault.Note) string {
	if note.Frontmatter == nil {
		return ""
	}
	tagsVal, ok := note.Frontmatter["tags"]
	if !ok {
		return ""
	}
	switch v := tagsVal.(type) {
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, fmt.Sprintf("%v", item))
		}
		return strings.Join(parts, ", ")
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

// dvNoteHasTag checks whether a note has the given tag.
func dvNoteHasTag(note *vault.Note, tag string) bool {
	if note.Frontmatter == nil {
		return false
	}
	tagsVal, ok := note.Frontmatter["tags"]
	if !ok {
		return false
	}
	tag = strings.ToLower(strings.TrimSpace(tag))
	switch v := tagsVal.(type) {
	case []interface{}:
		for _, item := range v {
			if strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", item)), tag) {
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
