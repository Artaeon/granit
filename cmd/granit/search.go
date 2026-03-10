package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type searchFlags struct {
	query         string
	vaultPath     string
	useRegex      bool
	jsonOutput    bool
	caseSensitive bool
	noColor       bool
}

type searchResult struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

func parseSearchFlags(args []string) searchFlags {
	sf := searchFlags{
		vaultPath: ".",
	}

	positional := []string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--regex":
			sf.useRegex = true
		case "--json":
			sf.jsonOutput = true
		case "--case-sensitive":
			sf.caseSensitive = true
		case "--no-color":
			sf.noColor = true
		default:
			positional = append(positional, args[i])
		}
	}

	if len(positional) >= 1 {
		sf.query = positional[0]
	}
	if len(positional) >= 2 {
		sf.vaultPath = positional[1]
	}

	return sf
}

func runSearch(args []string) {
	sf := parseSearchFlags(args)

	if sf.query == "" {
		fmt.Println("Usage: granit search <query> [vault-path] [--regex] [--json] [--case-sensitive] [--no-color]")
		os.Exit(1)
	}

	absPath, err := filepath.Abs(sf.vaultPath)
	if err != nil {
		fmt.Printf("Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// Validate path exists
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		fmt.Printf("Error: %s is not a valid directory\n", absPath)
		os.Exit(1)
	}

	// Compile regex if needed
	var re *regexp.Regexp
	if sf.useRegex {
		pattern := sf.query
		if !sf.caseSensitive {
			pattern = "(?i)" + pattern
		}
		re, err = regexp.Compile(pattern)
		if err != nil {
			fmt.Printf("Error: invalid regex pattern: %v\n", err)
			os.Exit(1)
		}
	}

	var results []searchResult
	matchCount := 0

	// Walk the vault and search files
	err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip hidden directories
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return nil // skip files we can't read
		}
		defer func() { _ = file.Close() }()

		relPath, _ := filepath.Rel(absPath, path)
		scanner := bufio.NewScanner(file)
		lineNum := 0

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			var matched bool
			if sf.useRegex {
				matched = re.MatchString(line)
			} else if sf.caseSensitive {
				matched = strings.Contains(line, sf.query)
			} else {
				matched = strings.Contains(
					strings.ToLower(line),
					strings.ToLower(sf.query),
				)
			}

			if matched {
				matchCount++
				results = append(results, searchResult{
					File:    relPath,
					Line:    lineNum,
					Content: line,
				})
			}
		}

		return nil
	})

	if err != nil {
		fmt.Printf("Error searching vault: %v\n", err)
		os.Exit(1)
	}

	// Output results
	if sf.jsonOutput {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			fmt.Printf("Error marshaling results: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}

	if len(results) == 0 {
		fmt.Printf("No matches found for %q in %s\n", sf.query, absPath)
		return
	}

	// Color codes
	fileColor := "\033[36m"   // cyan
	lineColor := "\033[33m"   // yellow
	matchColor := "\033[1;31m" // bold red
	reset := "\033[0m"

	if sf.noColor {
		fileColor = ""
		lineColor = ""
		matchColor = ""
		reset = ""
	}

	currentFile := ""
	for _, r := range results {
		if r.File != currentFile {
			currentFile = r.File
			fmt.Printf("\n%s%s%s\n", fileColor, r.File, reset)
		}

		displayLine := r.Content
		if !sf.noColor {
			// Highlight matching text
			if sf.useRegex {
				displayLine = re.ReplaceAllStringFunc(displayLine, func(m string) string {
					return matchColor + m + reset
				})
			} else {
				displayLine = highlightMatch(displayLine, sf.query, sf.caseSensitive, matchColor, reset)
			}
		}

		fmt.Printf("  %s%d%s: %s\n", lineColor, r.Line, reset, displayLine)
	}

	fmt.Printf("\n%d match(es) found in %s\n", matchCount, absPath)
}

// highlightMatch highlights all occurrences of query in text with ANSI colors.
func highlightMatch(text, query string, caseSensitive bool, colorStart, colorEnd string) string {
	if query == "" {
		return text
	}

	var result strings.Builder
	lower := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)

	i := 0
	for i < len(text) {
		var idx int
		if caseSensitive {
			idx = strings.Index(text[i:], query)
		} else {
			idx = strings.Index(lower[i:], lowerQuery)
		}

		if idx == -1 {
			result.WriteString(text[i:])
			break
		}

		result.WriteString(text[i : i+idx])
		matchEnd := i + idx + len(query)
		result.WriteString(colorStart)
		result.WriteString(text[i+idx : matchEnd])
		result.WriteString(colorEnd)
		i = matchEnd
	}

	return result.String()
}
