package main

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"

	"github.com/artaeon/granit/internal/vault"
)

type exportFlags struct {
	vaultPath string
	format    string
	outputDir string
	all       bool
	noteName  string
}

func parseExportFlags(args []string) exportFlags {
	ef := exportFlags{
		vaultPath: ".",
		format:    "html",
	}

	positional := []string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			if i+1 < len(args) {
				ef.format = args[i+1]
				i++
			}
		case "--output":
			if i+1 < len(args) {
				ef.outputDir = args[i+1]
				i++
			}
		case "--all":
			ef.all = true
		case "--note":
			if i+1 < len(args) {
				ef.noteName = args[i+1]
				i++
			}
		default:
			positional = append(positional, args[i])
		}
	}

	if len(positional) >= 1 {
		ef.vaultPath = positional[0]
	}

	return ef
}

func runExport(args []string) {
	ef := parseExportFlags(args)

	// Validate format
	switch ef.format {
	case "html", "text", "json":
		// valid
	default:
		fmt.Printf("Error: unsupported format %q (use html, text, or json)\n", ef.format)
		os.Exit(1)
	}

	if !ef.all && ef.noteName == "" {
		fmt.Println("Usage: granit export [vault-path] --format html|text|json --output <dir> --all|--note <name>")
		fmt.Println()
		fmt.Println("You must specify either --all or --note <name>")
		os.Exit(1)
	}

	absPath, err := filepath.Abs(ef.vaultPath)
	if err != nil {
		fmt.Printf("Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// Determine output directory
	outDir := ef.outputDir
	if outDir == "" {
		outDir = filepath.Join(absPath, "export")
	}
	outDir, err = filepath.Abs(outDir)
	if err != nil {
		fmt.Printf("Error resolving output path: %v\n", err)
		os.Exit(1)
	}

	// Create output directory
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Load vault
	v, err := vault.NewVault(absPath)
	if err != nil {
		fmt.Printf("Error opening vault: %v\n", err)
		os.Exit(1)
	}
	if err := v.Scan(); err != nil {
		fmt.Printf("Error scanning vault: %v\n", err)
		os.Exit(1)
	}

	// Gather notes to export
	var notePaths []string
	if ef.all {
		notePaths = v.SortedPaths()
	} else {
		// Find note by name
		found := false
		for _, p := range v.SortedPaths() {
			note := v.GetNote(p)
			if note.Title == ef.noteName || note.RelPath == ef.noteName || note.RelPath == ef.noteName+".md" {
				notePaths = append(notePaths, p)
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("Error: note %q not found in vault\n", ef.noteName)
			os.Exit(1)
		}
	}

	if len(notePaths) == 0 {
		fmt.Println("No notes to export.")
		return
	}

	// Export
	exported := 0
	switch ef.format {
	case "html":
		exported = exportHTML(v, notePaths, outDir, ef.all)
	case "text":
		exported = exportText(v, notePaths, outDir)
	case "json":
		exported = exportJSON(v, notePaths, outDir)
	}

	fmt.Printf("Exported %d note(s) to %s (format: %s)\n", exported, outDir, ef.format)
}

func exportHTML(v *vault.Vault, paths []string, outDir string, createIndex bool) int {
	exported := 0

	for _, p := range paths {
		note := v.GetNote(p)
		if note == nil {
			continue
		}

		content := vault.StripFrontmatter(note.Content)
		htmlContent := markdownToBasicHTML(content, note.Title)

		// Preserve directory structure
		outPath := filepath.Join(outDir, strings.TrimSuffix(note.RelPath, ".md")+".html")
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			fmt.Printf("  Error creating directory for %s: %v\n", note.RelPath, err)
			continue
		}

		if err := os.WriteFile(outPath, []byte(htmlContent), 0644); err != nil {
			fmt.Printf("  Error writing %s: %v\n", outPath, err)
			continue
		}
		exported++
	}

	// Create index.html if exporting all
	if createIndex && exported > 0 {
		indexContent := generateIndexHTML(v, paths)
		indexPath := filepath.Join(outDir, "index.html")
		if err := os.WriteFile(indexPath, []byte(indexContent), 0644); err != nil {
			fmt.Printf("  Error writing index.html: %v\n", err)
		}
	}

	return exported
}

func exportText(v *vault.Vault, paths []string, outDir string) int {
	exported := 0

	for _, p := range paths {
		note := v.GetNote(p)
		if note == nil {
			continue
		}

		content := vault.StripFrontmatter(note.Content)

		outPath := filepath.Join(outDir, strings.TrimSuffix(note.RelPath, ".md")+".txt")
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			fmt.Printf("  Error creating directory for %s: %v\n", note.RelPath, err)
			continue
		}

		if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
			fmt.Printf("  Error writing %s: %v\n", outPath, err)
			continue
		}
		exported++
	}

	return exported
}

type jsonNote struct {
	Title       string                 `json:"title"`
	Path        string                 `json:"path"`
	Content     string                 `json:"content"`
	Frontmatter map[string]interface{} `json:"frontmatter,omitempty"`
	Links       []string               `json:"links,omitempty"`
}

func exportJSON(v *vault.Vault, paths []string, outDir string) int {
	var notes []jsonNote

	for _, p := range paths {
		note := v.GetNote(p)
		if note == nil {
			continue
		}

		notes = append(notes, jsonNote{
			Title:       note.Title,
			Path:        note.RelPath,
			Content:     vault.StripFrontmatter(note.Content),
			Frontmatter: note.Frontmatter,
			Links:       note.Links,
		})
	}

	data, err := json.MarshalIndent(notes, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}

	outPath := filepath.Join(outDir, "vault.json")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Printf("Error writing %s: %v\n", outPath, err)
		os.Exit(1)
	}

	return len(notes)
}

// markdownToBasicHTML converts markdown content to a simple HTML page.
func markdownToBasicHTML(content, title string) string {
	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>` + html.EscapeString(title) + `</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
           max-width: 800px; margin: 40px auto; padding: 0 20px; line-height: 1.6;
           color: #333; background: #fff; }
    h1 { border-bottom: 2px solid #eee; padding-bottom: 0.3em; }
    h2 { border-bottom: 1px solid #eee; padding-bottom: 0.2em; }
    code { background: #f4f4f4; padding: 2px 6px; border-radius: 3px; font-size: 0.9em; }
    pre { background: #f4f4f4; padding: 16px; border-radius: 6px; overflow-x: auto; }
    pre code { background: none; padding: 0; }
    blockquote { border-left: 4px solid #ddd; margin: 0; padding: 0 16px; color: #666; }
    a { color: #0366d6; text-decoration: none; }
    a:hover { text-decoration: underline; }
    ul, ol { padding-left: 2em; }
    hr { border: none; border-top: 1px solid #eee; margin: 2em 0; }
  </style>
</head>
<body>
`)

	// Simple markdown-to-HTML line-by-line conversion
	lines := strings.Split(content, "\n")
	inCodeBlock := false
	inList := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Code blocks
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				b.WriteString("</code></pre>\n")
				inCodeBlock = false
			} else {
				b.WriteString("<pre><code>")
				inCodeBlock = true
			}
			continue
		}
		if inCodeBlock {
			b.WriteString(html.EscapeString(line) + "\n")
			continue
		}

		// Close list if needed
		if inList && !strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "* ") && trimmed != "" {
			b.WriteString("</ul>\n")
			inList = false
		}

		// Empty line
		if trimmed == "" {
			if inList {
				b.WriteString("</ul>\n")
				inList = false
			}
			continue
		}

		// Headings
		if strings.HasPrefix(trimmed, "# ") {
			b.WriteString("<h1>" + html.EscapeString(trimmed[2:]) + "</h1>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			b.WriteString("<h2>" + html.EscapeString(trimmed[3:]) + "</h2>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			b.WriteString("<h3>" + html.EscapeString(trimmed[4:]) + "</h3>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "#### ") {
			b.WriteString("<h4>" + html.EscapeString(trimmed[5:]) + "</h4>\n")
			continue
		}

		// Horizontal rule
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			b.WriteString("<hr>\n")
			continue
		}

		// Blockquote
		if strings.HasPrefix(trimmed, "> ") {
			b.WriteString("<blockquote><p>" + html.EscapeString(trimmed[2:]) + "</p></blockquote>\n")
			continue
		}

		// Unordered list
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				b.WriteString("<ul>\n")
				inList = true
			}
			// Handle checkboxes
			item := trimmed[2:]
			if strings.HasPrefix(item, "[ ] ") {
				item = "&#9744; " + item[4:]
			} else if strings.HasPrefix(item, "[x] ") || strings.HasPrefix(item, "[X] ") {
				item = "&#9745; " + item[4:]
			}
			b.WriteString("  <li>" + html.EscapeString(item) + "</li>\n")
			continue
		}

		// Regular paragraph
		b.WriteString("<p>" + convertInlineMarkdown(trimmed) + "</p>\n")
	}

	if inList {
		b.WriteString("</ul>\n")
	}
	if inCodeBlock {
		b.WriteString("</code></pre>\n")
	}

	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// convertInlineMarkdown handles bold, italic, code, and wikilinks.
func convertInlineMarkdown(text string) string {
	escaped := html.EscapeString(text)

	// Convert wikilinks [[target|alias]] or [[target]]
	for {
		start := strings.Index(escaped, "[[")
		if start == -1 {
			break
		}
		end := strings.Index(escaped[start:], "]]")
		if end == -1 {
			break
		}
		end += start
		inner := escaped[start+2 : end]
		display := inner
		if pipeIdx := strings.Index(inner, "|"); pipeIdx != -1 {
			display = inner[pipeIdx+1:]
			inner = inner[:pipeIdx]
		}
		link := strings.TrimSuffix(inner, ".md") + ".html"
		escaped = escaped[:start] + `<a href="` + link + `">` + display + `</a>` + escaped[end+2:]
	}

	return escaped
}

// generateIndexHTML creates an index page listing all exported notes.
func generateIndexHTML(v *vault.Vault, paths []string) string {
	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Vault Index</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
           max-width: 800px; margin: 40px auto; padding: 0 20px; line-height: 1.6;
           color: #333; background: #fff; }
    h1 { border-bottom: 2px solid #eee; padding-bottom: 0.3em; }
    a { color: #0366d6; text-decoration: none; }
    a:hover { text-decoration: underline; }
    .note-list { list-style: none; padding: 0; }
    .note-list li { padding: 8px 0; border-bottom: 1px solid #f0f0f0; }
    .note-path { color: #999; font-size: 0.85em; margin-left: 8px; }
    .stats { color: #666; margin-bottom: 2em; }
  </style>
</head>
<body>
  <h1>Vault Index</h1>
  <p class="stats">` + fmt.Sprintf("%d notes", len(paths)) + `</p>
  <ul class="note-list">
`)

	for _, p := range paths {
		note := v.GetNote(p)
		if note == nil {
			continue
		}
		htmlPath := strings.TrimSuffix(note.RelPath, ".md") + ".html"
		b.WriteString(fmt.Sprintf("    <li><a href=\"%s\">%s</a><span class=\"note-path\">%s</span></li>\n",
			html.EscapeString(htmlPath),
			html.EscapeString(note.Title),
			html.EscapeString(note.RelPath),
		))
	}

	b.WriteString(`  </ul>
</body>
</html>
`)

	return b.String()
}
