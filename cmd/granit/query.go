package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/artaeon/granit/internal/vault"
)

// QueryResult is the JSON-serializable representation of a query match.
type QueryResult struct {
	Path     string                 `json:"path"`
	Title    string                 `json:"title"`
	Tags     []string               `json:"tags,omitempty"`
	Modified string                 `json:"modified"`
	Meta     map[string]interface{} `json:"meta,omitempty"`
}

// queryFilter represents a single parsed filter like tag:project or folder:Research.
type queryFilter struct {
	kind  string // "tag", "folder", "title", "status", or any frontmatter key
	value string
}

func runQuery() {
	args := getPositionalArgs(2)
	if len(args) == 0 {
		exitError("Usage: granit query '<expression>' [vault-path]\n\nFilters:\n  tag:<value>      Match notes with a specific tag\n  folder:<value>   Match notes in a folder (prefix match)\n  title:<value>    Match note title (case-insensitive substring)\n  <key>:<value>    Match any frontmatter key\n\nOperators: AND (default), OR\n\nExamples:\n  granit query 'tag:project AND status:active'\n  granit query 'folder:Research OR tag:important'\n  granit query 'title:meeting'")
	}

	expression := args[0]
	vaultPath := "."
	if len(args) > 1 {
		vaultPath = args[len(args)-1]
	}
	if v := getFlagValue("--vault"); v != "" {
		vaultPath = v
	}

	jsonOut := hasFlag("--json")
	tableOut := hasFlag("--table")
	compact := hasFlag("--compact")

	v, err := vault.NewVault(vaultPath)
	if err != nil {
		exitError("Error opening vault: %v", err)
	}
	if err := v.Scan(); err != nil {
		exitError("Error scanning vault: %v", err)
	}

	// Parse the expression into filter groups
	filterGroups, useOR := parseExpression(expression)

	// Evaluate each note
	var results []QueryResult
	for _, p := range v.SortedPaths() {
		note := v.GetNote(p)
		if matchesQuery(note, filterGroups, useOR) {
			tags := extractTags(note)
			meta := make(map[string]interface{})
			for k, val := range note.Frontmatter {
				if k != "tags" {
					meta[k] = val
				}
			}
			results = append(results, QueryResult{
				Path:     note.RelPath,
				Title:    note.Title,
				Tags:     tags,
				Modified: note.ModTime.Format("2006-01-02"),
				Meta:     meta,
			})
		}
	}

	if jsonOut {
		var data []byte
		if compact {
			data, err = json.Marshal(results)
		} else {
			data, err = json.MarshalIndent(results, "", "  ")
		}
		if err != nil {
			exitError("Error marshaling JSON: %v", err)
		}
		fmt.Println(string(data))
		return
	}

	if tableOut {
		printQueryTable(results)
		return
	}

	// Default: paths only (for piping)
	if len(results) == 0 {
		fmt.Fprintf(os.Stderr, "No notes match the query.\n")
		os.Exit(1)
	}
	for _, r := range results {
		fmt.Println(r.Path)
	}
	fmt.Fprintf(os.Stderr, "\n%d note(s) matched\n", len(results))
}

// parseExpression splits "tag:project AND status:active" into filter groups.
// Returns the filters and whether OR mode is used.
func parseExpression(expr string) ([]queryFilter, bool) {
	useOR := false
	// Normalize
	expr = strings.TrimSpace(expr)

	// Detect OR mode
	var parts []string
	if strings.Contains(strings.ToUpper(expr), " OR ") {
		useOR = true
		parts = splitOnOperator(expr, "OR")
	} else {
		// Default: AND (also handles explicit AND)
		parts = splitOnOperator(expr, "AND")
	}

	var filters []queryFilter
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		colonIdx := strings.Index(part, ":")
		if colonIdx > 0 {
			kind := strings.ToLower(part[:colonIdx])
			value := strings.TrimSpace(part[colonIdx+1:])
			filters = append(filters, queryFilter{kind: kind, value: value})
		}
	}
	return filters, useOR
}

// splitOnOperator splits an expression on AND or OR (case-insensitive).
func splitOnOperator(expr, op string) []string {
	upper := strings.ToUpper(expr)
	opPadded := " " + op + " "
	var result []string
	for {
		idx := strings.Index(upper, opPadded)
		if idx < 0 {
			result = append(result, expr)
			break
		}
		result = append(result, expr[:idx])
		expr = expr[idx+len(opPadded):]
		upper = upper[idx+len(opPadded):]
	}
	return result
}

// matchesQuery checks if a note matches the filter set.
func matchesQuery(note *vault.Note, filters []queryFilter, useOR bool) bool {
	if len(filters) == 0 {
		return true
	}

	for _, f := range filters {
		matched := matchFilter(note, f)
		if useOR && matched {
			return true
		}
		if !useOR && !matched {
			return false
		}
	}

	// AND mode: all matched; OR mode: none matched
	return !useOR
}

func matchFilter(note *vault.Note, f queryFilter) bool {
	switch f.kind {
	case "tag":
		tags := extractTags(note)
		for _, t := range tags {
			if strings.EqualFold(t, f.value) {
				return true
			}
		}
		return false

	case "folder":
		dir := filepath.Dir(note.RelPath)
		return strings.HasPrefix(strings.ToLower(dir), strings.ToLower(f.value))

	case "title":
		return strings.Contains(strings.ToLower(note.Title), strings.ToLower(f.value))

	default:
		// Generic frontmatter key match
		if note.Frontmatter == nil {
			return false
		}
		val, ok := note.Frontmatter[f.kind]
		if !ok {
			return false
		}
		switch v := val.(type) {
		case string:
			return strings.EqualFold(v, f.value)
		case []string:
			for _, s := range v {
				if strings.EqualFold(s, f.value) {
					return true
				}
			}
			return false
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok && strings.EqualFold(s, f.value) {
					return true
				}
			}
			return false
		default:
			return fmt.Sprintf("%v", v) == f.value
		}
	}
}

func printQueryTable(results []QueryResult) {
	if len(results) == 0 {
		fmt.Println("No notes match the query.")
		return
	}

	// Calculate column widths
	maxPath := 4
	maxTitle := 5
	maxTags := 4
	for _, r := range results {
		if len(r.Path) > maxPath {
			maxPath = len(r.Path)
		}
		if len(r.Title) > maxTitle {
			maxTitle = len(r.Title)
		}
		tagStr := strings.Join(r.Tags, ", ")
		if len(tagStr) > maxTags {
			maxTags = len(tagStr)
		}
	}
	if maxPath > 45 {
		maxPath = 45
	}
	if maxTitle > 30 {
		maxTitle = 30
	}
	if maxTags > 30 {
		maxTags = 30
	}

	// Header
	header := fmt.Sprintf("%-*s  %-*s  %-*s  %s", maxPath, "PATH", maxTitle, "TITLE", maxTags, "TAGS", "MODIFIED")
	fmt.Println(header)
	fmt.Println(strings.Repeat("─", len(header)))

	for _, r := range results {
		path := r.Path
		if len(path) > maxPath {
			path = path[:maxPath-3] + "..."
		}
		title := r.Title
		if len(title) > maxTitle {
			title = title[:maxTitle-3] + "..."
		}
		tagStr := strings.Join(r.Tags, ", ")
		if len(tagStr) > maxTags {
			tagStr = tagStr[:maxTags-3] + "..."
		}
		fmt.Printf("%-*s  %-*s  %-*s  %s\n", maxPath, path, maxTitle, title, maxTags, tagStr, r.Modified)
	}

	fmt.Printf("\n%d note(s) matched\n", len(results))
}
