package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/artaeon/granit/internal/vault"
)

// NoteInfo is the JSON-serializable representation of a vault note.
type NoteInfo struct {
	Path     string   `json:"path"`
	Title    string   `json:"title"`
	Tags     []string `json:"tags"`
	Words    int      `json:"words"`
	Modified string   `json:"modified"`
}

func runListNotes() {
	vaultPath := resolveVaultPath(2)

	// Determine output mode from flags
	jsonOut := hasFlag("--json")
	pathsOut := hasFlag("--paths")
	tagsOut := hasFlag("--tags")
	compact := hasFlag("--compact")

	// Determine vault path: skip flags to find positional arg
	for i := 2; i < len(os.Args); i++ {
		arg := os.Args[i]
		if !strings.HasPrefix(arg, "--") {
			vaultPath = arg
			break
		}
	}

	v, err := vault.NewVault(vaultPath)
	if err != nil {
		exitError("Error opening vault: %v", err)
	}
	if err := v.Scan(); err != nil {
		exitError("Error scanning vault: %v", err)
	}

	if tagsOut {
		listTags(v, jsonOut, compact)
		return
	}

	if pathsOut {
		listPaths(v)
		return
	}

	if jsonOut {
		listJSON(v, compact)
		return
	}

	// Default: pretty table
	listTable(v)
}

func listPaths(v *vault.Vault) {
	for _, p := range v.SortedPaths() {
		fmt.Println(p)
	}
}

func listJSON(v *vault.Vault, compact bool) {
	notes := make([]NoteInfo, 0, v.NoteCount())
	for _, p := range v.SortedPaths() {
		note := v.GetNote(p)
		info := NoteInfo{
			Path:     note.RelPath,
			Title:    note.Title,
			Tags:     extractTags(note),
			Words:    countWords(note.Content),
			Modified: note.ModTime.Format("2006-01-02"),
		}
		notes = append(notes, info)
	}

	var data []byte
	var err error
	if compact {
		data, err = json.Marshal(notes)
	} else {
		data, err = json.MarshalIndent(notes, "", "  ")
	}
	if err != nil {
		exitError("Error marshaling JSON: %v", err)
	}
	fmt.Println(string(data))
}

func listTable(v *vault.Vault) {
	paths := v.SortedPaths()
	if len(paths) == 0 {
		fmt.Println("No notes found.")
		return
	}

	// Calculate column widths
	maxPath := 4 // "PATH"
	maxTitle := 5 // "TITLE"
	for _, p := range paths {
		if len(p) > maxPath {
			maxPath = len(p)
		}
		note := v.GetNote(p)
		if len(note.Title) > maxTitle {
			maxTitle = len(note.Title)
		}
	}
	// Cap widths
	if maxPath > 50 {
		maxPath = 50
	}
	if maxTitle > 40 {
		maxTitle = 40
	}

	header := fmt.Sprintf("%-*s  %-*s  %5s  %s", maxPath, "PATH", maxTitle, "TITLE", "WORDS", "MODIFIED")
	fmt.Println(header)
	fmt.Println(strings.Repeat("─", len(header)))

	for _, p := range paths {
		note := v.GetNote(p)
		words := countWords(note.Content)
		modified := note.ModTime.Format("2006-01-02")

		path := p
		if len(path) > maxPath {
			path = path[:maxPath-3] + "..."
		}
		title := note.Title
		if len(title) > maxTitle {
			title = title[:maxTitle-3] + "..."
		}
		fmt.Printf("%-*s  %-*s  %5d  %s\n", maxPath, path, maxTitle, title, words, modified)
	}

	fmt.Printf("\n%d note(s)\n", len(paths))
}

func listTags(v *vault.Vault, jsonOut bool, compact bool) {
	tagCounts := make(map[string]int)
	for _, p := range v.SortedPaths() {
		note := v.GetNote(p)
		for _, tag := range extractTags(note) {
			tagCounts[tag]++
		}
	}

	// Sort tags alphabetically
	tags := make([]string, 0, len(tagCounts))
	for t := range tagCounts {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	if jsonOut {
		type TagEntry struct {
			Tag   string `json:"tag"`
			Count int    `json:"count"`
		}
		entries := make([]TagEntry, 0, len(tags))
		for _, t := range tags {
			entries = append(entries, TagEntry{Tag: t, Count: tagCounts[t]})
		}
		var data []byte
		var err error
		if compact {
			data, err = json.Marshal(entries)
		} else {
			data, err = json.MarshalIndent(entries, "", "  ")
		}
		if err != nil {
			exitError("Error marshaling JSON: %v", err)
		}
		fmt.Println(string(data))
		return
	}

	// Pretty output
	if len(tags) == 0 {
		fmt.Println("No tags found.")
		return
	}
	fmt.Printf("%-30s  %s\n", "TAG", "COUNT")
	fmt.Println(strings.Repeat("─", 40))
	for _, t := range tags {
		fmt.Printf("%-30s  %d\n", t, tagCounts[t])
	}
	fmt.Printf("\n%d unique tag(s)\n", len(tags))
}

// extractTags pulls tags from a note's frontmatter.
// Supports both []interface{} (parsed YAML arrays) and []string.
func extractTags(note *vault.Note) []string {
	if note.Frontmatter == nil {
		return nil
	}
	raw, ok := note.Frontmatter["tags"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []interface{}:
		tags := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				tags = append(tags, s)
			}
		}
		return tags
	case []string:
		return v
	case string:
		// Comma-separated
		parts := strings.Split(v, ",")
		tags := make([]string, 0, len(parts))
		for _, p := range parts {
			t := strings.TrimSpace(p)
			if t != "" {
				tags = append(tags, t)
			}
		}
		return tags
	}
	return nil
}

// countWords counts the words in text content.
func countWords(content string) int {
	// Strip frontmatter first
	stripped := vault.StripFrontmatter(content)
	return len(strings.Fields(stripped))
}
