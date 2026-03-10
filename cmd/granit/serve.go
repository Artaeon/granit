package main

import (
	"fmt"
	"html"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/artaeon/granit/internal/vault"
)

func runServe(args []string) {
	port := "8080"
	vaultPath := "."

	// Parse flags and positional args
	positional := []string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port":
			if i+1 < len(args) {
				port = args[i+1]
				i++
			}
		default:
			if strings.HasPrefix(args[i], "--port=") {
				port = args[i][7:]
			} else if !strings.HasPrefix(args[i], "--") {
				positional = append(positional, args[i])
			}
		}
	}
	if len(positional) >= 1 {
		vaultPath = positional[0]
	}

	absPath, err := filepath.Abs(vaultPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// Load vault
	v, err := vault.NewVault(absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening vault: %v\n", err)
		os.Exit(1)
	}
	if err := v.Scan(); err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning vault: %v\n", err)
		os.Exit(1)
	}

	idx := vault.NewIndex(v)
	idx.Build()

	// Build a lookup map from lowercase title (no extension) to relPath
	// for resolving wikilinks
	titleToPath := buildTitleMap(v)

	mux := http.NewServeMux()

	// Index page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			// Try to serve a note
			serveNote(w, r, v, idx, titleToPath)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(serveIndexPage(v)))
	})

	addr := "localhost:" + port
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Handle Ctrl+C gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down server...")
		_ = server.Close()
	}()

	fmt.Printf("Serving vault at http://%s — press Ctrl+C to stop\n", addr)
	fmt.Printf("Vault: %s (%d notes)\n", absPath, v.NoteCount())

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

// buildTitleMap creates a case-insensitive map from note title to relative path.
// This enables wikilink resolution: [[My Note]] -> notes/My Note.md
func buildTitleMap(v *vault.Vault) map[string]string {
	m := make(map[string]string)
	for _, p := range v.SortedPaths() {
		note := v.GetNote(p)
		if note == nil {
			continue
		}
		key := strings.ToLower(note.Title)
		// First match wins (in case of duplicates)
		if _, exists := m[key]; !exists {
			m[key] = note.RelPath
		}
	}
	return m
}


func serveNote(w http.ResponseWriter, r *http.Request, v *vault.Vault, idx *vault.Index, titleMap map[string]string) {
	urlPath := strings.TrimPrefix(r.URL.Path, "/")

	// Try direct path match (with .md extension)
	relPath := urlPath
	if !strings.HasSuffix(strings.ToLower(relPath), ".md") {
		relPath += ".md"
	}

	// Sanitize the path to prevent directory traversal attacks.
	relPath = filepath.Clean(relPath)
	if filepath.IsAbs(relPath) || strings.HasPrefix(relPath, "..") {
		http.NotFound(w, r)
		return
	}

	note := v.GetNote(relPath)

	// If not found, try case-insensitive title lookup
	if note == nil {
		lookupKey := strings.ToLower(urlPath)
		// Strip .md suffix from lookup key if present
		lookupKey = strings.TrimSuffix(lookupKey, ".md")
		// Try just the filename part (wikilinks often omit folder paths)
		parts := strings.Split(lookupKey, "/")
		baseName := parts[len(parts)-1]

		if mappedPath, ok := titleMap[lookupKey]; ok {
			note = v.GetNote(mappedPath)
		}
		if note == nil {
			if mappedPath, ok := titleMap[baseName]; ok {
				note = v.GetNote(mappedPath)
			}
		}
	}

	if note == nil {
		http.NotFound(w, r)
		return
	}

	content := vault.StripFrontmatter(note.Content)
	backlinks := idx.GetBacklinks(note.RelPath)
	htmlPage := serveNoteHTML(content, note.Title, note.RelPath, note.Links, backlinks, titleMap)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(htmlPage))
}

// serveCSS returns the Catppuccin Mocha-themed CSS for the served pages.
func serveCSS() string {
	return `
    :root {
      --ctp-base: #1e1e2e;
      --ctp-mantle: #181825;
      --ctp-crust: #11111b;
      --ctp-surface0: #313244;
      --ctp-surface1: #45475a;
      --ctp-surface2: #585b70;
      --ctp-overlay0: #6c7086;
      --ctp-text: #cdd6f4;
      --ctp-subtext0: #a6adc8;
      --ctp-subtext1: #bac2de;
      --ctp-blue: #89b4fa;
      --ctp-lavender: #b4befe;
      --ctp-sapphire: #74c7ec;
      --ctp-green: #a6e3a1;
      --ctp-peach: #fab387;
      --ctp-red: #f38ba8;
      --ctp-mauve: #cba6f7;
      --ctp-yellow: #f9e2af;
      --ctp-teal: #94e2d5;
      --ctp-pink: #f5c2e7;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      max-width: 860px;
      margin: 0 auto;
      padding: 2rem 1.5rem;
      line-height: 1.7;
      color: var(--ctp-text);
      background: var(--ctp-base);
    }
    a { color: var(--ctp-blue); text-decoration: none; }
    a:hover { text-decoration: underline; color: var(--ctp-sapphire); }
    h1 { color: var(--ctp-lavender); font-size: 1.8em; margin: 1.2em 0 0.6em;
         border-bottom: 1px solid var(--ctp-surface1); padding-bottom: 0.3em; }
    h2 { color: var(--ctp-mauve); font-size: 1.4em; margin: 1em 0 0.5em;
         border-bottom: 1px solid var(--ctp-surface0); padding-bottom: 0.2em; }
    h3 { color: var(--ctp-pink); font-size: 1.2em; margin: 0.8em 0 0.4em; }
    h4 { color: var(--ctp-peach); font-size: 1.05em; margin: 0.6em 0 0.3em; }
    h5 { color: var(--ctp-yellow); font-size: 1em; margin: 0.5em 0 0.3em; }
    h6 { color: var(--ctp-teal); font-size: 0.95em; margin: 0.5em 0 0.3em; }
    p { margin: 0.6em 0; }
    code {
      background: var(--ctp-surface0);
      color: var(--ctp-green);
      padding: 2px 6px;
      border-radius: 4px;
      font-family: 'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace;
      font-size: 0.88em;
    }
    pre {
      background: var(--ctp-mantle);
      border: 1px solid var(--ctp-surface0);
      padding: 1em 1.2em;
      border-radius: 8px;
      overflow-x: auto;
      margin: 0.8em 0;
    }
    pre code { background: none; padding: 0; color: var(--ctp-text); }
    blockquote {
      border-left: 3px solid var(--ctp-mauve);
      margin: 0.6em 0;
      padding: 0.4em 1em;
      background: var(--ctp-mantle);
      border-radius: 0 6px 6px 0;
      color: var(--ctp-subtext1);
    }
    ul, ol { padding-left: 1.6em; margin: 0.4em 0; }
    li { margin: 0.2em 0; }
    hr { border: none; border-top: 1px solid var(--ctp-surface1); margin: 1.5em 0; }
    .navbar {
      display: flex;
      align-items: center;
      gap: 1em;
      padding: 0.6em 0;
      margin-bottom: 1.5em;
      border-bottom: 1px solid var(--ctp-surface0);
      font-size: 0.9em;
    }
    .navbar a { color: var(--ctp-subtext0); }
    .navbar a:hover { color: var(--ctp-blue); }
    .navbar .brand { color: var(--ctp-mauve); font-weight: 600; font-size: 1em; }
    .navbar .sep { color: var(--ctp-surface2); }
    .breadcrumb { color: var(--ctp-subtext0); font-size: 0.85em; margin-bottom: 0.5em; }
    .backlinks {
      margin-top: 2.5em;
      padding-top: 1em;
      border-top: 1px solid var(--ctp-surface0);
    }
    .backlinks h3 { color: var(--ctp-overlay0); font-size: 0.9em; font-weight: 600;
                     text-transform: uppercase; letter-spacing: 0.05em; }
    .backlinks ul { list-style: none; padding: 0; margin-top: 0.4em; }
    .backlinks li { padding: 0.2em 0; }
    .backlinks a { color: var(--ctp-sapphire); font-size: 0.9em; }
    .tag { display: inline-block; background: var(--ctp-surface0); color: var(--ctp-teal);
           padding: 1px 8px; border-radius: 12px; font-size: 0.8em; margin: 2px; }
    .note-list { list-style: none; padding: 0; }
    .note-list li {
      padding: 0.6em 0.8em;
      border-bottom: 1px solid var(--ctp-surface0);
      display: flex;
      justify-content: space-between;
      align-items: baseline;
    }
    .note-list li:hover { background: var(--ctp-mantle); border-radius: 6px; }
    .note-list .note-path { color: var(--ctp-overlay0); font-size: 0.8em; }
    .note-list .note-title { font-weight: 500; }
    .stats { color: var(--ctp-subtext0); margin-bottom: 1.5em; font-size: 0.9em; }
    .search-box {
      width: 100%;
      padding: 0.6em 1em;
      margin-bottom: 1em;
      border: 1px solid var(--ctp-surface1);
      border-radius: 8px;
      background: var(--ctp-mantle);
      color: var(--ctp-text);
      font-size: 1em;
      outline: none;
    }
    .search-box:focus { border-color: var(--ctp-blue); }
    .search-box::placeholder { color: var(--ctp-overlay0); }
    .folder-group { margin-bottom: 0.5em; }
    .folder-name {
      color: var(--ctp-overlay0);
      font-size: 0.8em;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      padding: 0.8em 0.4em 0.2em;
    }
`
}

func serveIndexPage(v *vault.Vault) string {
	var b strings.Builder

	paths := v.SortedPaths()

	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Granit — Vault</title>
  <style>`)
	b.WriteString(serveCSS())
	b.WriteString(`
  </style>
</head>
<body>
  <div class="navbar">
    <span class="brand">Granit</span>
  </div>
  <h1>Vault Index</h1>
  <p class="stats">`)
	b.WriteString(fmt.Sprintf("%d notes", len(paths)))
	b.WriteString(`</p>
  <input type="text" class="search-box" id="filter" placeholder="Filter notes..." autofocus>
  <ul class="note-list" id="noteList">
`)

	// Group by folder
	groups := make(map[string][]*vault.Note)
	var folderOrder []string

	for _, p := range paths {
		note := v.GetNote(p)
		if note == nil {
			continue
		}
		folder := filepath.Dir(note.RelPath)
		if folder == "." {
			folder = ""
		}
		if _, exists := groups[folder]; !exists {
			folderOrder = append(folderOrder, folder)
		}
		groups[folder] = append(groups[folder], note)
	}
	sort.Strings(folderOrder)

	for _, folder := range folderOrder {
		notes := groups[folder]
		if folder != "" {
			b.WriteString(fmt.Sprintf("    <li class=\"folder-name\" data-folder>%s/</li>\n",
				html.EscapeString(folder)))
		}
		for _, note := range notes {
			noteURL := "/" + strings.TrimSuffix(note.RelPath, ".md")
			b.WriteString(fmt.Sprintf("    <li data-title=\"%s\"><a href=\"%s\" class=\"note-title\">%s</a><span class=\"note-path\">%s</span></li>\n",
				html.EscapeString(strings.ToLower(note.Title)),
				html.EscapeString(noteURL),
				html.EscapeString(note.Title),
				html.EscapeString(note.RelPath),
			))
		}
	}

	b.WriteString(`  </ul>
  <script>
    document.getElementById('filter').addEventListener('input', function(e) {
      var q = e.target.value.toLowerCase();
      var items = document.querySelectorAll('#noteList li:not([data-folder])');
      items.forEach(function(li) {
        var t = li.getAttribute('data-title') || '';
        li.style.display = t.indexOf(q) !== -1 ? '' : 'none';
      });
    });
  </script>
</body>
</html>
`)

	return b.String()
}

// serveWikilinkRegex matches [[target]] and [[target|alias]] in already-escaped HTML.
var serveWikilinkRegex = regexp.MustCompile(`\[\[([^\]|]+?)(?:\|([^\]]+?))?\]\]`)

func serveNoteHTML(content, title, relPath string, outLinks, backlinks []string, titleMap map[string]string) string {
	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(` — Granit</title>
  <style>`)
	b.WriteString(serveCSS())
	b.WriteString(`
  </style>
</head>
<body>
  <div class="navbar">
    <a href="/" class="brand">Granit</a>
    <span class="sep">/</span>
    <span>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</span>
  </div>
`)

	// Show breadcrumb for nested notes
	dir := filepath.Dir(relPath)
	if dir != "." && dir != "" {
		b.WriteString(fmt.Sprintf("  <div class=\"breadcrumb\">%s</div>\n", html.EscapeString(dir+"/")))
	}

	// Render markdown body
	b.WriteString(serveRenderMarkdown(content, titleMap))

	// Backlinks section
	if len(backlinks) > 0 {
		b.WriteString("  <div class=\"backlinks\">\n")
		b.WriteString("    <h3>Backlinks</h3>\n")
		b.WriteString("    <ul>\n")
		for _, bl := range backlinks {
			linkTitle := strings.TrimSuffix(bl, ".md")
			linkTitle = strings.TrimSuffix(filepath.Base(linkTitle), filepath.Ext(filepath.Base(linkTitle)))
			linkURL := "/" + strings.TrimSuffix(bl, ".md")
			b.WriteString(fmt.Sprintf("      <li><a href=\"%s\">%s</a></li>\n",
				html.EscapeString(linkURL), html.EscapeString(linkTitle)))
		}
		b.WriteString("    </ul>\n")
		b.WriteString("  </div>\n")
	}

	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// serveRenderMarkdown converts markdown content to HTML with the serve theme.
func serveRenderMarkdown(content string, titleMap map[string]string) string {
	var b strings.Builder

	lines := strings.Split(content, "\n")
	inCodeBlock := false
	inList := false
	listType := "" // "ul" or "ol"

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Code blocks
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				b.WriteString("</code></pre>\n")
				inCodeBlock = false
			} else {
				lang := strings.TrimPrefix(trimmed, "```")
				if lang != "" {
					b.WriteString(fmt.Sprintf("<pre><code data-lang=\"%s\">", html.EscapeString(lang)))
				} else {
					b.WriteString("<pre><code>")
				}
				inCodeBlock = true
			}
			continue
		}
		if inCodeBlock {
			b.WriteString(html.EscapeString(line) + "\n")
			continue
		}

		// Determine if this line is a list item
		isUnordered := strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")
		isOrdered := len(trimmed) > 2 && trimmed[0] >= '1' && trimmed[0] <= '9' && strings.Contains(trimmed[:min(4, len(trimmed))], ". ")

		// Close list if we're leaving a list context
		if inList && !isUnordered && !isOrdered && trimmed != "" {
			b.WriteString(fmt.Sprintf("</%s>\n", listType))
			inList = false
		}

		// Empty line
		if trimmed == "" {
			if inList {
				b.WriteString(fmt.Sprintf("</%s>\n", listType))
				inList = false
			}
			continue
		}

		// Headings
		if strings.HasPrefix(trimmed, "###### ") {
			b.WriteString("<h6>" + serveInline(trimmed[7:], titleMap) + "</h6>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "##### ") {
			b.WriteString("<h5>" + serveInline(trimmed[6:], titleMap) + "</h5>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "#### ") {
			b.WriteString("<h4>" + serveInline(trimmed[5:], titleMap) + "</h4>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			b.WriteString("<h3>" + serveInline(trimmed[4:], titleMap) + "</h3>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			b.WriteString("<h2>" + serveInline(trimmed[3:], titleMap) + "</h2>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			b.WriteString("<h1>" + serveInline(trimmed[2:], titleMap) + "</h1>\n")
			continue
		}

		// Horizontal rule
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			b.WriteString("<hr>\n")
			continue
		}

		// Blockquote
		if strings.HasPrefix(trimmed, "> ") {
			b.WriteString("<blockquote><p>" + serveInline(trimmed[2:], titleMap) + "</p></blockquote>\n")
			continue
		}

		// Unordered list
		if isUnordered {
			if !inList {
				b.WriteString("<ul>\n")
				inList = true
				listType = "ul"
			}
			item := trimmed[2:]
			if strings.HasPrefix(item, "[ ] ") {
				item = `<input type="checkbox" disabled> ` + item[4:]
			} else if strings.HasPrefix(item, "[x] ") || strings.HasPrefix(item, "[X] ") {
				item = `<input type="checkbox" disabled checked> ` + item[4:]
			}
			b.WriteString("  <li>" + serveInline(item, titleMap) + "</li>\n")
			continue
		}

		// Ordered list
		if isOrdered {
			if !inList {
				b.WriteString("<ol>\n")
				inList = true
				listType = "ol"
			}
			dotIdx := strings.Index(trimmed, ". ")
			if dotIdx != -1 {
				item := trimmed[dotIdx+2:]
				b.WriteString("  <li>" + serveInline(item, titleMap) + "</li>\n")
			}
			continue
		}

		// Regular paragraph
		b.WriteString("<p>" + serveInline(trimmed, titleMap) + "</p>\n")
	}

	if inList {
		b.WriteString(fmt.Sprintf("</%s>\n", listType))
	}
	if inCodeBlock {
		b.WriteString("</code></pre>\n")
	}

	return b.String()
}

// serveInline converts inline markdown (bold, italic, code, wikilinks) to HTML.
func serveInline(text string, titleMap map[string]string) string {
	// First, handle inline code spans so their content isn't processed further.
	// We'll use a placeholder approach.
	type codeSpan struct {
		placeholder string
		html        string
	}
	var codeSpans []codeSpan
	codeIdx := 0

	// Extract inline code
	result := text
	for {
		start := strings.Index(result, "`")
		if start == -1 {
			break
		}
		end := strings.Index(result[start+1:], "`")
		if end == -1 {
			break
		}
		end += start + 1
		code := result[start+1 : end]
		placeholder := fmt.Sprintf("\x00CODE%d\x00", codeIdx)
		codeIdx++
		cs := codeSpan{
			placeholder: placeholder,
			html:        "<code>" + html.EscapeString(code) + "</code>",
		}
		codeSpans = append(codeSpans, cs)
		result = result[:start] + placeholder + result[end+1:]
	}

	// HTML-escape the remaining text
	result = html.EscapeString(result)

	// Convert wikilinks [[target|alias]] or [[target]]
	result = serveWikilinkRegex.ReplaceAllStringFunc(result, func(match string) string {
		subs := serveWikilinkRegex.FindStringSubmatch(match)
		if len(subs) < 2 {
			return match
		}
		target := subs[1]
		display := target
		if len(subs) >= 3 && subs[2] != "" {
			display = subs[2]
		}

		// Resolve the link URL
		linkURL := "/" + target
		lookupKey := strings.ToLower(target)
		if mappedPath, ok := titleMap[lookupKey]; ok {
			linkURL = "/" + strings.TrimSuffix(mappedPath, ".md")
		}

		return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(linkURL), html.EscapeString(display))
	})

	// Bold: **text** or __text__
	result = regexp.MustCompile(`\*\*(.+?)\*\*`).ReplaceAllString(result, "<strong>$1</strong>")
	result = regexp.MustCompile(`__(.+?)__`).ReplaceAllString(result, "<strong>$1</strong>")

	// Italic: *text* or _text_ (but not inside words for underscore)
	result = regexp.MustCompile(`\*(.+?)\*`).ReplaceAllString(result, "<em>$1</em>")
	result = regexp.MustCompile(`(?:^|[\s(])_(.+?)_(?:[\s).,!?;:]|$)`).ReplaceAllString(result, " <em>$1</em> ")

	// Strikethrough: ~~text~~
	result = regexp.MustCompile(`~~(.+?)~~`).ReplaceAllString(result, "<del>$1</del>")

	// Highlight: ==text==
	result = regexp.MustCompile(`==(.+?)==`).ReplaceAllString(result, `<mark style="background:var(--ctp-yellow);color:var(--ctp-crust);padding:1px 3px;border-radius:3px">$1</mark>`)

	// Tags: #tag
	result = regexp.MustCompile(`(?:^|\s)#([a-zA-Z0-9_/-]+)`).ReplaceAllString(result, ` <span class="tag">#$1</span>`)

	// Restore code spans
	for _, cs := range codeSpans {
		// The placeholder was HTML-escaped, so we need to find the escaped version
		escapedPlaceholder := html.EscapeString(cs.placeholder)
		result = strings.ReplaceAll(result, escapedPlaceholder, cs.html)
	}

	return result
}

