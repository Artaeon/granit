package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/artaeon/granit/internal/vault"
)

// SearchResult represents a single search hit for JSON output.
type SearchResult struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Context string `json:"context"`
}

func runSearch() {
	// Collect positional args (the query terms) and vault path
	args := getPositionalArgs(2)
	if len(args) == 0 {
		exitError("Usage: granit search <query> [vault-path]\n  --json     JSON output\n  --regex    enable regex mode\n  --compact  compact JSON")
	}

	query := args[0]
	vaultPath := "."
	if len(args) > 1 {
		vaultPath = args[len(args)-1]
	}
	// Allow --vault flag override
	if v := getFlagValue("--vault"); v != "" {
		vaultPath = v
	}

	jsonOut := hasFlag("--json")
	useRegex := hasFlag("--regex")
	compact := hasFlag("--compact")

	v, err := vault.NewVault(vaultPath)
	if err != nil {
		exitError("Error opening vault: %v", err)
	}
	if err := v.Scan(); err != nil {
		exitError("Error scanning vault: %v", err)
	}

	var results []SearchResult
	var re *regexp.Regexp
	if useRegex {
		re, err = regexp.Compile(query)
		if err != nil {
			exitError("Invalid regex: %v", err)
		}
	}

	for _, p := range v.SortedPaths() {
		note := v.GetNote(p)
		lines := strings.Split(note.Content, "\n")
		for lineNum, line := range lines {
			var matched bool
			var col int
			if useRegex {
				loc := re.FindStringIndex(line)
				if loc != nil {
					matched = true
					col = loc[0]
				}
			} else {
				idx := strings.Index(strings.ToLower(line), strings.ToLower(query))
				if idx >= 0 {
					matched = true
					col = idx
				}
			}
			if matched {
				results = append(results, SearchResult{
					Path:    note.RelPath,
					Line:    lineNum + 1,
					Column:  col + 1,
					Context: strings.TrimSpace(line),
				})
			}
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

	// Default: grep-like colored output
	if len(results) == 0 {
		fmt.Fprintf(os.Stderr, "No matches found for %q\n", query)
		os.Exit(1)
	}

	for _, r := range results {
		// file:line:col: context
		fmt.Printf("\033[35m%s\033[0m:\033[32m%d\033[0m:\033[32m%d\033[0m: %s\n",
			r.Path, r.Line, r.Column, highlightMatch(r.Context, query, useRegex, re))
	}
	fmt.Fprintf(os.Stderr, "\n%d match(es) found\n", len(results))
}

// highlightMatch wraps matching text in ANSI bold red.
func highlightMatch(line, query string, useRegex bool, re *regexp.Regexp) string {
	if useRegex && re != nil {
		return re.ReplaceAllStringFunc(line, func(m string) string {
			return "\033[1;31m" + m + "\033[0m"
		})
	}
	// Case-insensitive literal highlighting
	lower := strings.ToLower(line)
	lowerQ := strings.ToLower(query)
	var result strings.Builder
	pos := 0
	for {
		idx := strings.Index(lower[pos:], lowerQ)
		if idx < 0 {
			result.WriteString(line[pos:])
			break
		}
		result.WriteString(line[pos : pos+idx])
		result.WriteString("\033[1;31m")
		result.WriteString(line[pos+idx : pos+idx+len(query)])
		result.WriteString("\033[0m")
		pos += idx + len(query)
	}
	return result.String()
}
