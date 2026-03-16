package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ==================== Dataview Query Engine ====================

// RunDataviewQuery parses and executes a simple query against vault notes.
//
// Supported syntax:
//
//	FROM <folder>          — filter by folder
//	WHERE <field> = <val>  — filter by frontmatter field (=, !=, CONTAINS)
//	SORT <field> [ASC|DESC]
//	LIMIT <n>
//
// Examples:
//
//	FROM "projects" WHERE status = "active" SORT date DESC LIMIT 10
//	WHERE tags CONTAINS "meeting" SORT title
//	FROM "daily" SORT date DESC LIMIT 5
func (a *GranitApp) RunDataviewQuery(query string) ([]map[string]interface{}, error) {
	if a.vault == nil {
		return nil, fmt.Errorf("no vault open")
	}

	parsed := parseDVQueryDesktop(query)

	// Collect candidate notes
	type noteEntry struct {
		relPath     string
		title       string
		frontmatter map[string]interface{}
		content     string
		modTime     string
		size        int64
	}

	var candidates []noteEntry

	for _, p := range a.vault.SortedPaths() {
		note := a.vault.GetNote(p)
		if note == nil {
			continue
		}

		// FROM folder filter
		if parsed.source != "" {
			noteDir := filepath.Dir(note.RelPath)
			if noteDir != parsed.source && !strings.HasPrefix(noteDir, parsed.source+"/") {
				continue
			}
		}

		// FROM #tag filter
		if parsed.sourceTag != "" {
			if !dvDesktopNoteHasTag(note.Frontmatter, parsed.sourceTag) {
				continue
			}
		}

		candidates = append(candidates, noteEntry{
			relPath:     note.RelPath,
			title:       note.Title,
			frontmatter: note.Frontmatter,
			content:     note.Content,
			modTime:     note.ModTime.Format("2006-01-02"),
			size:        note.Size,
		})
	}

	// Apply WHERE conditions
	var filtered []noteEntry
	for _, c := range candidates {
		if matchesDVConditions(c.frontmatter, c.title, c.relPath, c.content, c.modTime, parsed.conditions) {
			filtered = append(filtered, c)
		}
	}

	// Sort
	if parsed.sortField != "" {
		sort.SliceStable(filtered, func(i, j int) bool {
			a := dvDesktopGetField(filtered[i].frontmatter, filtered[i].title, filtered[i].relPath, filtered[i].content, filtered[i].modTime, parsed.sortField)
			b := dvDesktopGetField(filtered[j].frontmatter, filtered[j].title, filtered[j].relPath, filtered[j].content, filtered[j].modTime, parsed.sortField)
			if parsed.sortDesc {
				return a > b
			}
			return a < b
		})
	} else {
		// Default sort by title
		sort.SliceStable(filtered, func(i, j int) bool {
			return strings.ToLower(filtered[i].title) < strings.ToLower(filtered[j].title)
		})
	}

	// Limit
	if parsed.limit > 0 && len(filtered) > parsed.limit {
		filtered = filtered[:parsed.limit]
	}

	// Build result rows
	results := make([]map[string]interface{}, 0, len(filtered))
	for _, c := range filtered {
		row := map[string]interface{}{
			"title":   c.title,
			"path":    c.relPath,
			"date":    c.modTime,
			"words":   len(strings.Fields(c.content)),
			"size":    c.size,
		}

		// Include all frontmatter fields
		if c.frontmatter != nil {
			for k, v := range c.frontmatter {
				if _, exists := row[k]; !exists {
					row[k] = v
				}
			}
		}

		// Tags as string
		if tags := dvDesktopTagsString(c.frontmatter); tags != "" {
			row["tags"] = tags
		}

		results = append(results, row)
	}

	return results, nil
}

// dvParsedQueryDesktop is the parsed form of a dataview query.
type dvParsedQueryDesktop struct {
	source     string // folder path filter
	sourceTag  string // tag filter (without #)
	conditions []dvCondDesktop
	sortField  string
	sortDesc   bool
	limit      int
}

type dvCondDesktop struct {
	field string
	op    string
	value string
}

// parseDVQueryDesktop parses a simplified dataview query string.
func parseDVQueryDesktop(raw string) dvParsedQueryDesktop {
	q := dvParsedQueryDesktop{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return q
	}

	tokens := tokenizeDVDesktop(raw)
	pos := 0

	// Skip mode keyword if present (TABLE, LIST, TASK)
	if pos < len(tokens) {
		upper := strings.ToUpper(tokens[pos])
		if upper == "TABLE" || upper == "LIST" || upper == "TASK" {
			pos++
			// Skip field list for TABLE mode
			for pos < len(tokens) {
				u := strings.ToUpper(tokens[pos])
				if u == "FROM" || u == "WHERE" || u == "SORT" || u == "LIMIT" {
					break
				}
				pos++
			}
		}
	}

	// FROM clause
	if pos < len(tokens) && strings.ToUpper(tokens[pos]) == "FROM" {
		pos++
		if pos < len(tokens) {
			src := dvUnquote(tokens[pos])
			if strings.HasPrefix(src, "#") {
				q.sourceTag = src[1:]
			} else {
				q.source = src
			}
			pos++
		}
	}

	// WHERE clause
	if pos < len(tokens) && strings.ToUpper(tokens[pos]) == "WHERE" {
		pos++
		for pos < len(tokens) {
			upper := strings.ToUpper(tokens[pos])
			if upper == "SORT" || upper == "LIMIT" {
				break
			}
			if upper == "AND" {
				pos++
				continue
			}

			// Need field, op, value
			if pos+2 < len(tokens) {
				field := tokens[pos]
				op := strings.ToUpper(tokens[pos+1])
				value := dvUnquote(tokens[pos+2])

				// Normalize operator
				switch op {
				case "=", "!=", ">", "<", ">=", "<=", "CONTAINS":
					q.conditions = append(q.conditions, dvCondDesktop{field: field, op: op, value: value})
					pos += 3
				default:
					pos++
				}
			} else {
				pos++
			}
		}
	}

	// SORT clause
	if pos < len(tokens) && strings.ToUpper(tokens[pos]) == "SORT" {
		pos++
		if pos < len(tokens) {
			q.sortField = tokens[pos]
			pos++
			if pos < len(tokens) {
				dir := strings.ToUpper(tokens[pos])
				if dir == "DESC" {
					q.sortDesc = true
					pos++
				} else if dir == "ASC" {
					pos++
				}
			}
		}
	}

	// LIMIT clause
	if pos < len(tokens) && strings.ToUpper(tokens[pos]) == "LIMIT" {
		pos++
		if pos < len(tokens) {
			if n, err := strconv.Atoi(tokens[pos]); err == nil && n > 0 {
				q.limit = n
			}
		}
	}

	return q
}

// tokenizeDVDesktop splits a query into tokens, respecting quoted strings.
func tokenizeDVDesktop(s string) []string {
	var tokens []string
	runes := []rune(s)
	i := 0

	for i < len(runes) {
		if runes[i] == ' ' || runes[i] == '\t' || runes[i] == ',' {
			i++
			continue
		}

		// Quoted string
		if runes[i] == '"' || runes[i] == '\'' {
			quote := runes[i]
			i++
			start := i
			for i < len(runes) && runes[i] != quote {
				i++
			}
			tokens = append(tokens, string(runes[start:i]))
			if i < len(runes) {
				i++
			}
			continue
		}

		// Multi-char operators
		if i+1 < len(runes) {
			two := string(runes[i : i+2])
			if two == ">=" || two == "<=" || two == "!=" {
				tokens = append(tokens, two)
				i += 2
				continue
			}
		}

		// Single-char operators
		if runes[i] == '=' || runes[i] == '>' || runes[i] == '<' {
			tokens = append(tokens, string(runes[i]))
			i++
			continue
		}

		// Word
		start := i
		for i < len(runes) && runes[i] != ' ' && runes[i] != '\t' &&
			runes[i] != ',' && runes[i] != '"' && runes[i] != '\'' &&
			runes[i] != '=' && runes[i] != '>' && runes[i] != '<' {
			i++
		}
		if i > start {
			tokens = append(tokens, string(runes[start:i]))
		}
	}

	return tokens
}

// dvUnquote removes surrounding quotes from a string.
func dvUnquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// matchesDVConditions checks whether a note matches all WHERE conditions.
func matchesDVConditions(fm map[string]interface{}, title, relPath, content, modTime string, conditions []dvCondDesktop) bool {
	for _, cond := range conditions {
		val := dvDesktopGetField(fm, title, relPath, content, modTime, cond.field)
		target := cond.value

		match := false
		switch cond.op {
		case "=":
			match = strings.EqualFold(val, target)
		case "!=":
			match = !strings.EqualFold(val, target)
		case "CONTAINS":
			if strings.ToLower(cond.field) == "tags" {
				match = dvDesktopNoteHasTag(fm, target)
			} else {
				match = strings.Contains(strings.ToLower(val), strings.ToLower(target))
			}
		case ">":
			match = val > target
		case "<":
			match = val < target
		case ">=":
			match = val >= target
		case "<=":
			match = val <= target
		}

		if !match {
			return false
		}
	}
	return true
}

// dvDesktopGetField extracts a field value from note data.
func dvDesktopGetField(fm map[string]interface{}, title, relPath, content, modTime, field string) string {
	field = strings.ToLower(strings.TrimSpace(field))
	switch field {
	case "title", "name":
		return title
	case "path", "file":
		return relPath
	case "folder":
		return filepath.Dir(relPath)
	case "date":
		if fm != nil {
			if v, ok := fm["date"]; ok {
				if s, ok := v.(string); ok {
					return s
				}
			}
		}
		return modTime
	case "modified":
		return modTime
	case "tags":
		return dvDesktopTagsString(fm)
	case "words", "wordcount":
		return fmt.Sprintf("%d", len(strings.Fields(content)))
	default:
		if fm == nil {
			return ""
		}
		val, ok := fm[field]
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
}

// dvDesktopTagsString returns a comma-separated string of tags.
func dvDesktopTagsString(fm map[string]interface{}) string {
	if fm == nil {
		return ""
	}
	tagsVal, ok := fm["tags"]
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

// dvDesktopNoteHasTag checks whether a note has the given tag.
func dvDesktopNoteHasTag(fm map[string]interface{}, tag string) bool {
	if fm == nil {
		return false
	}
	tagsVal, ok := fm["tags"]
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
