package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type importFlags struct {
	fromFormat string
	sourcePath string
	vaultPath  string
}

func parseImportFlags(args []string) importFlags {
	imf := importFlags{
		vaultPath: ".",
	}

	positional := []string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--from":
			if i+1 < len(args) {
				imf.fromFormat = args[i+1]
				i++
			}
		default:
			positional = append(positional, args[i])
		}
	}

	if len(positional) >= 1 {
		imf.sourcePath = positional[0]
	}
	if len(positional) >= 2 {
		imf.vaultPath = positional[1]
	}

	return imf
}

func runImport(args []string) {
	imf := parseImportFlags(args)

	if imf.fromFormat == "" || imf.sourcePath == "" {
		fmt.Println("Usage: granit import --from obsidian|logseq|notion <source-path> [vault-path]")
		os.Exit(1)
	}

	switch imf.fromFormat {
	case "obsidian":
		importObsidian(imf.sourcePath, imf.vaultPath)
	case "logseq":
		importLogseq(imf.sourcePath, imf.vaultPath)
	case "notion":
		importNotion(imf.sourcePath, imf.vaultPath)
	default:
		fmt.Printf("Error: unsupported format %q (use obsidian, logseq, or notion)\n", imf.fromFormat)
		os.Exit(1)
	}
}

// importObsidian copies .md files from an Obsidian vault, converting
// Obsidian-specific syntax where needed.
func importObsidian(srcPath, destPath string) {
	absSrc, err := filepath.Abs(srcPath)
	if err != nil {
		fmt.Printf("Error resolving source path: %v\n", err)
		os.Exit(1)
	}
	absDest, err := filepath.Abs(destPath)
	if err != nil {
		fmt.Printf("Error resolving destination path: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(absDest, 0755); err != nil {
		fmt.Printf("Error creating destination directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Importing from Obsidian vault: %s\n", absSrc)
	fmt.Printf("Destination: %s\n", absDest)
	fmt.Println(strings.Repeat("-", 50))

	imported := 0
	skipped := 0

	err = filepath.Walk(absSrc, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip .obsidian directory and other hidden dirs
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}

		// Only process .md files
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			skipped++
			return nil
		}

		relPath, _ := filepath.Rel(absSrc, path)
		destFile := filepath.Join(absDest, relPath)

		// Create destination directory
		if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
			fmt.Printf("  Error creating dir for %s: %v\n", relPath, err)
			return nil
		}

		// Read source file
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("  Error reading %s: %v\n", relPath, err)
			return nil
		}

		// Convert Obsidian-specific syntax
		converted := convertObsidianSyntax(string(content))

		if err := os.WriteFile(destFile, []byte(converted), 0644); err != nil {
			fmt.Printf("  Error writing %s: %v\n", relPath, err)
			return nil
		}

		imported++
		fmt.Printf("  Imported: %s\n", relPath)
		return nil
	})

	if err != nil {
		fmt.Printf("Error walking source: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("Import complete: %d notes imported, %d non-markdown files skipped\n", imported, skipped)
}

// convertObsidianSyntax converts Obsidian-specific markdown syntax to standard markdown.
func convertObsidianSyntax(content string) string {
	// Convert Obsidian callouts: > [!note] -> > **Note:**
	calloutRe := regexp.MustCompile(`(?m)^> \[!([\w-]+)\]\s*(.*)$`)
	content = calloutRe.ReplaceAllStringFunc(content, func(match string) string {
		sub := calloutRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		calloutType := titleCase(sub[1])
		title := strings.TrimSpace(sub[2])
		if title != "" {
			return fmt.Sprintf("> **%s: %s**", calloutType, title)
		}
		return fmt.Sprintf("> **%s:**", calloutType)
	})

	// Convert Obsidian embeds: ![[note]] -> [[note]] (remove the embed marker)
	embedRe := regexp.MustCompile(`!\[\[([^\]]+)\]\]`)
	content = embedRe.ReplaceAllString(content, "[[$1]]")

	// Obsidian comments: %%comment%% -> <!-- comment -->
	commentRe := regexp.MustCompile(`%%([^%]+)%%`)
	content = commentRe.ReplaceAllString(content, "<!-- $1 -->")

	return content
}

// importLogseq converts Logseq's bullet-based markdown or org-mode to standard markdown.
func importLogseq(srcPath, destPath string) {
	absSrc, err := filepath.Abs(srcPath)
	if err != nil {
		fmt.Printf("Error resolving source path: %v\n", err)
		os.Exit(1)
	}
	absDest, err := filepath.Abs(destPath)
	if err != nil {
		fmt.Printf("Error resolving destination path: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(absDest, 0755); err != nil {
		fmt.Printf("Error creating destination directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Importing from Logseq graph: %s\n", absSrc)
	fmt.Printf("Destination: %s\n", absDest)
	fmt.Println(strings.Repeat("-", 50))

	imported := 0

	// Logseq typically stores pages in pages/ and journals in journals/
	for _, subdir := range []string{"pages", "journals", ""} {
		scanDir := filepath.Join(absSrc, subdir)
		if _, err := os.Stat(scanDir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(scanDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			if info.IsDir() {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".md" && ext != ".org" {
				return nil
			}

			relPath, _ := filepath.Rel(absSrc, path)
			content, err := os.ReadFile(path)
			if err != nil {
				fmt.Printf("  Error reading %s: %v\n", relPath, err)
				return nil
			}

			var converted string
			if ext == ".org" {
				converted = convertOrgMode(string(content))
			} else {
				converted = convertLogseqMarkdown(string(content))
			}

			// Output as .md
			destRelPath := strings.TrimSuffix(relPath, ext) + ".md"
			destFile := filepath.Join(absDest, destRelPath)

			if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
				fmt.Printf("  Error creating dir for %s: %v\n", destRelPath, err)
				return nil
			}

			if err := os.WriteFile(destFile, []byte(converted), 0644); err != nil {
				fmt.Printf("  Error writing %s: %v\n", destRelPath, err)
				return nil
			}

			imported++
			fmt.Printf("  Imported: %s\n", destRelPath)
			return nil
		})
		if err != nil {
			fmt.Printf("  Error scanning %s: %v\n", scanDir, err)
		}
	}

	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("Import complete: %d notes imported\n", imported)
}

// convertLogseqMarkdown removes Logseq's bullet prefix format and converts
// block references / properties.
func convertLogseqMarkdown(content string) string {
	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Remove leading bullet dashes (Logseq's outliner format)
		if strings.HasPrefix(trimmed, "- ") {
			// Preserve indentation level
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			if indent <= 1 {
				// Top-level bullet: remove the dash
				result = append(result, trimmed[2:])
			} else {
				// Nested bullet: keep as list item but reduce indent
				newIndent := strings.Repeat("  ", indent/2)
				result = append(result, newIndent+"- "+trimmed[2:])
			}
			continue
		}

		// Convert Logseq properties (key:: value) to YAML-like format
		if strings.Contains(trimmed, ":: ") {
			parts := strings.SplitN(trimmed, ":: ", 2)
			if len(parts) == 2 {
				result = append(result, fmt.Sprintf("**%s**: %s", parts[0], parts[1]))
				continue
			}
		}

		// Convert block references ((uuid)) -> remove them
		blockRefRe := regexp.MustCompile(`\(\([0-9a-f-]+\)\)`)
		line = blockRefRe.ReplaceAllString(line, "")

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// convertOrgMode converts basic org-mode syntax to markdown.
func convertOrgMode(content string) string {
	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Org headings: * Heading -> # Heading
		if strings.HasPrefix(trimmed, "* ") {
			level := 0
			for _, c := range trimmed {
				if c == '*' {
					level++
				} else {
					break
				}
			}
			heading := strings.TrimLeft(trimmed, "* ")
			result = append(result, strings.Repeat("#", level)+" "+heading)
			continue
		}

		// Org bold: *text* -> **text**
		boldRe := regexp.MustCompile(`\*([^*]+)\*`)
		line = boldRe.ReplaceAllString(line, "**$1**")

		// Org italic: /text/ -> *text*
		italicRe := regexp.MustCompile(`/([^/]+)/`)
		line = italicRe.ReplaceAllString(line, "*$1*")

		// Org code: =text= or ~text~ -> `text`
		codeRe := regexp.MustCompile(`[=~]([^=~]+)[=~]`)
		line = codeRe.ReplaceAllString(line, "`$1`")

		// Org links: [[url][description]] -> [description](url)
		linkRe := regexp.MustCompile(`\[\[([^\]]+)\]\[([^\]]+)\]\]`)
		line = linkRe.ReplaceAllString(line, "[$2]($1)")

		// Org links without description: [[url]] -> [url](url)
		simpleLinkRe := regexp.MustCompile(`\[\[([^\]]+)\]\]`)
		line = simpleLinkRe.ReplaceAllString(line, "[[$1]]") // keep as wikilink

		// Org properties drawer: skip
		if trimmed == ":PROPERTIES:" || trimmed == ":END:" || strings.HasPrefix(trimmed, ":") {
			continue
		}

		// Org block: #+BEGIN_SRC lang -> ```lang
		if strings.HasPrefix(strings.ToUpper(trimmed), "#+BEGIN_SRC") {
			lang := strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(trimmed), "#+BEGIN_SRC"))
			lang = strings.ToLower(lang)
			result = append(result, "```"+lang)
			continue
		}
		if strings.HasPrefix(strings.ToUpper(trimmed), "#+END_SRC") {
			result = append(result, "```")
			continue
		}

		// Skip org metadata lines (#+TITLE, #+DATE, etc.)
		if strings.HasPrefix(trimmed, "#+") {
			continue
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// importNotion handles Notion's export format (nested folders with markdown).
func importNotion(srcPath, destPath string) {
	absSrc, err := filepath.Abs(srcPath)
	if err != nil {
		fmt.Printf("Error resolving source path: %v\n", err)
		os.Exit(1)
	}
	absDest, err := filepath.Abs(destPath)
	if err != nil {
		fmt.Printf("Error resolving destination path: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(absDest, 0755); err != nil {
		fmt.Printf("Error creating destination directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Importing from Notion export: %s\n", absSrc)
	fmt.Printf("Destination: %s\n", absDest)
	fmt.Println(strings.Repeat("-", 50))

	imported := 0

	err = filepath.Walk(absSrc, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".csv" {
			return nil
		}

		// Skip CSV files (Notion database exports)
		if ext == ".csv" {
			return nil
		}

		relPath, _ := filepath.Rel(absSrc, path)

		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("  Error reading %s: %v\n", relPath, err)
			return nil
		}

		converted := convertNotionMarkdown(string(content))

		// Clean up Notion's UUID-suffixed filenames
		cleanPath := cleanNotionPath(relPath)
		destFile := filepath.Join(absDest, cleanPath)

		if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
			fmt.Printf("  Error creating dir for %s: %v\n", cleanPath, err)
			return nil
		}

		if err := os.WriteFile(destFile, []byte(converted), 0644); err != nil {
			fmt.Printf("  Error writing %s: %v\n", cleanPath, err)
			return nil
		}

		imported++
		fmt.Printf("  Imported: %s\n", cleanPath)
		return nil
	})

	if err != nil {
		fmt.Printf("Error walking source: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("Import complete: %d notes imported\n", imported)
}

// cleanNotionPath removes Notion's UUID suffixes from filenames and directories.
// e.g., "My Page abc123def456.md" -> "My Page.md"
func cleanNotionPath(path string) string {
	// Notion appends a space + 32-char hex ID to file/folder names
	notionIDRe := regexp.MustCompile(` [0-9a-f]{32}`)

	parts := strings.Split(path, string(filepath.Separator))
	var clean []string
	for _, part := range parts {
		ext := filepath.Ext(part)
		name := strings.TrimSuffix(part, ext)
		name = notionIDRe.ReplaceAllString(name, "")
		clean = append(clean, name+ext)
	}
	return filepath.Join(clean...)
}

// convertNotionMarkdown cleans up Notion-specific markdown syntax.
func convertNotionMarkdown(content string) string {
	// Convert Notion's callout blocks (they use emoji + blockquote)
	// e.g., > 💡 Some text -> > **Tip:** Some text

	// Convert Notion's toggle blocks (they use <details>)
	detailsRe := regexp.MustCompile(`(?s)<details>\s*<summary>([^<]+)</summary>\s*(.*?)\s*</details>`)
	content = detailsRe.ReplaceAllString(content, "### $1\n\n$2")

	// Convert Notion's table of contents markers
	content = strings.ReplaceAll(content, "[toc]", "")

	// Convert Notion's inline database links
	// Notion uses markdown links with notion.so URLs
	notionLinkRe := regexp.MustCompile(`\[([^\]]+)\]\(https?://(?:www\.)?notion\.so/[^\)]+\)`)
	content = notionLinkRe.ReplaceAllString(content, "[[$1]]")

	return content
}

// titleCase converts a string to title case.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}
