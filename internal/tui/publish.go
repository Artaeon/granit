package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// publishResultMsg carries the outcome of an async publish operation.
type publishResultMsg struct {
	pages int
	err   error
}

// publishProgressMsg reports incremental progress during publishing.
type publishProgressMsg struct {
	current int
	total   int
	name    string
}

// ---------------------------------------------------------------------------
// Configuration types
// ---------------------------------------------------------------------------

// publishField identifies which config field is being edited.
type publishField int

const (
	fieldOutputDir publishField = iota
	fieldSiteTitle
	fieldInclude
	fieldExclude
	fieldOptionStart // options start after the text fields
)

// publishOption represents a toggleable option in the publish overlay.
type publishOption struct {
	label   string
	enabled bool
}

// ---------------------------------------------------------------------------
// Publisher overlay
// ---------------------------------------------------------------------------

// Publisher is an overlay that generates a static HTML website from the vault.
type Publisher struct {
	active    bool
	width     int
	height    int
	vaultPath string

	// Configuration fields (editable)
	outputDir       string
	siteTitle       string
	includePatterns string
	excludePatterns string

	// Cursor & editing state
	cursor   int          // index across all rows (fields + options + publish button)
	editing  bool         // true when typing into a text field
	editBuf  string       // current edit buffer
	field    publishField // which field is being edited
	options  []publishOption

	// Progress / result state
	status   string
	progress int
	total    int
	done     bool
	err      error
}

// NewPublisher creates a new Publisher overlay with default configuration.
func NewPublisher() Publisher {
	return Publisher{
		outputDir: "_site",
		siteTitle: "Granit",
		options: []publishOption{
			{label: "Include tags page", enabled: true},
			{label: "Include search (basic)", enabled: true},
			{label: "Include backlinks", enabled: false},
		},
	}
}

func (p *Publisher) IsActive() bool {
	return p.active
}

func (p *Publisher) Open() {
	p.active = true
	p.cursor = 0
	p.editing = false
	p.editBuf = ""
	p.status = ""
	p.progress = 0
	p.total = 0
	p.done = false
	p.err = nil
	if p.outputDir == "" || p.outputDir == "_site" {
		p.outputDir = "_site"
	}
}

func (p *Publisher) Close() {
	p.active = false
	p.editing = false
}

func (p *Publisher) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *Publisher) SetVaultPath(path string) {
	p.vaultPath = path
}

// totalRows returns the number of navigable rows in the config view:
//
//	4 text fields + len(options) + 1 publish button.
func (p *Publisher) totalRows() int {
	return 4 + len(p.options) + 1
}

// publishButtonRow returns the row index of the [Publish] button.
func (p *Publisher) publishButtonRow() int {
	return 4 + len(p.options)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (p Publisher) Update(msg tea.Msg) (Publisher, tea.Cmd) {
	if !p.active {
		return p, nil
	}

	switch msg := msg.(type) {
	case publishResultMsg:
		p.done = true
		p.err = msg.err
		p.total = msg.pages
		if msg.err != nil {
			p.status = "Error: " + msg.err.Error()
		} else {
			p.status = fmt.Sprintf("Published %d pages to %s", msg.pages, p.resolveOutputDir())
		}
		return p, nil

	case publishProgressMsg:
		p.progress = msg.current
		p.total = msg.total
		p.status = fmt.Sprintf("Publishing %d/%d: %s", msg.current, msg.total, msg.name)
		return p, nil

	case tea.KeyMsg:
		// --- Result screen: only Esc closes ---
		if p.done {
			switch msg.String() {
			case "esc", "q":
				p.active = false
				p.done = false
				p.status = ""
			}
			return p, nil
		}

		// --- Publishing in progress: ignore keys ---
		if p.status != "" && !p.done && !p.editing {
			return p, nil
		}

		// --- Editing a text field ---
		if p.editing {
			return p.updateEditing(msg)
		}

		// --- Normal navigation ---
		return p.updateNavigation(msg)
	}
	return p, nil
}

func (p Publisher) updateEditing(msg tea.KeyMsg) (Publisher, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Cancel edit — discard buffer
		p.editing = false
		p.editBuf = ""
		return p, nil
	case "enter":
		// Commit edit
		p.commitEdit()
		p.editing = false
		p.editBuf = ""
		return p, nil
	case "backspace":
		if len(p.editBuf) > 0 {
			p.editBuf = TrimLastRune(p.editBuf)
		}
		return p, nil
	case "ctrl+u":
		p.editBuf = ""
		return p, nil
	default:
		// Only accept printable runes
		for _, r := range msg.Runes {
			p.editBuf += string(r)
		}
		return p, nil
	}
}

func (p *Publisher) commitEdit() {
	val := strings.TrimSpace(p.editBuf)
	switch p.field {
	case fieldOutputDir:
		if val != "" {
			p.outputDir = val
		}
	case fieldSiteTitle:
		if val != "" {
			p.siteTitle = val
		}
	case fieldInclude:
		p.includePatterns = val
	case fieldExclude:
		p.excludePatterns = val
	}
}

func (p Publisher) updateNavigation(msg tea.KeyMsg) (Publisher, tea.Cmd) {
	maxRow := p.totalRows() - 1

	switch msg.String() {
	case "esc":
		p.active = false
		return p, nil
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
		}
	case "down", "j":
		if p.cursor < maxRow {
			p.cursor++
		}
	case " ":
		// Toggle option if cursor is on an option row
		optIdx := p.cursor - 4
		if optIdx >= 0 && optIdx < len(p.options) {
			p.options[optIdx].enabled = !p.options[optIdx].enabled
		}
	case "enter":
		if p.cursor < 4 {
			// Start editing a text field
			p.editing = true
			p.field = publishField(p.cursor)
			switch p.field {
			case fieldOutputDir:
				p.editBuf = p.outputDir
			case fieldSiteTitle:
				p.editBuf = p.siteTitle
			case fieldInclude:
				p.editBuf = p.includePatterns
			case fieldExclude:
				p.editBuf = p.excludePatterns
			}
			return p, nil
		}
		if p.cursor == p.publishButtonRow() {
			// Start publishing
			if p.vaultPath == "" {
				p.status = "Error: no vault path set"
				p.done = true
				p.err = fmt.Errorf("no vault path set")
				return p, nil
			}
			p.status = "Publishing..."
			p.progress = 0
			p.total = 0
			return p, p.startPublish()
		}
		// If on an option row, toggle it
		optIdx := p.cursor - 4
		if optIdx >= 0 && optIdx < len(p.options) {
			p.options[optIdx].enabled = !p.options[optIdx].enabled
		}
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Publish command
// ---------------------------------------------------------------------------

func (p *Publisher) resolveOutputDir() string {
	if filepath.IsAbs(p.outputDir) {
		return p.outputDir
	}
	return filepath.Join(p.vaultPath, p.outputDir)
}

func (p *Publisher) startPublish() tea.Cmd {
	vaultPath := p.vaultPath
	outputDir := p.resolveOutputDir()
	siteTitle := p.siteTitle
	includePatterns := splitPatterns(p.includePatterns)
	excludePatterns := splitPatterns(p.excludePatterns)
	includeTags := p.options[0].enabled
	includeSearch := p.options[1].enabled

	return func() tea.Msg {
		pages, err := publishSite(vaultPath, outputDir, siteTitle,
			includePatterns, excludePatterns, includeTags, includeSearch)
		return publishResultMsg{pages: pages, err: err}
	}
}

// splitPatterns splits a comma-separated pattern string into individual globs.
func splitPatterns(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (p Publisher) View() string {
	width := p.width / 2
	if width < 56 {
		width = 56
	}
	if width > 74 {
		width = 74
	}
	innerW := width - 6

	var b strings.Builder

	// Header
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Publish Vault as Website")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n\n")

	if p.done {
		p.viewResult(&b, innerW)
	} else if p.status != "" && !p.editing {
		p.viewProgress(&b, innerW)
	} else {
		p.viewConfig(&b, innerW)
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (p *Publisher) viewResult(b *strings.Builder, innerW int) {
	if p.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(red).Bold(true)
		b.WriteString(errStyle.Render("  " + p.status))
	} else {
		okStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		b.WriteString(okStyle.Render("  " + p.status))
		b.WriteString("\n\n")
		hintStyle := lipgloss.NewStyle().Foreground(text)
		b.WriteString(hintStyle.Render("  Open with: "))
		cmdStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)
		b.WriteString(cmdStyle.Render(fmt.Sprintf("python3 -m http.server -d %s", p.resolveOutputDir())))
	}
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Esc: close"))
}

func (p *Publisher) viewProgress(b *strings.Builder, innerW int) {
	spinStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)
	b.WriteString(spinStyle.Render("  " + p.status))
	b.WriteString("\n\n")

	if p.total > 0 {
		barWidth := innerW - 14
		if barWidth < 10 {
			barWidth = 10
		}
		filled := 0
		if p.total > 0 {
			filled = p.progress * barWidth / p.total
		}
		if filled > barWidth {
			filled = barWidth
		}
		empty := barWidth - filled
		bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("\u2588", filled)) +
			DimStyle.Render(strings.Repeat("\u2591", empty))
		pct := 0
		if p.total > 0 {
			pct = p.progress * 100 / p.total
		}
		b.WriteString(fmt.Sprintf("  %s %3d%%", bar, pct))
	}
}

func (p *Publisher) viewConfig(b *strings.Builder, innerW int) {
	labelStyle := lipgloss.NewStyle().Foreground(text)
	valueStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	editingStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)

	type fieldEntry struct {
		label string
		value string
	}
	fields := []fieldEntry{
		{"Output Dir  ", p.outputDir},
		{"Site Title  ", p.siteTitle},
		{"Include     ", p.includePatterns},
		{"Exclude     ", p.excludePatterns},
	}

	for i, f := range fields {
		prefix := "  "
		if i == p.cursor {
			prefix = "> "
		}

		displayVal := f.value
		if displayVal == "" {
			displayVal = "(none)"
		}

		if p.editing && p.cursor == i {
			// Show edit buffer with cursor
			cursor := lipgloss.NewStyle().Foreground(peach).Render("\u2588")
			line := prefix + labelStyle.Render(f.label) + editingStyle.Render(p.editBuf) + cursor
			b.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Width(innerW).
				Render(line))
		} else if i == p.cursor {
			line := prefix + labelStyle.Render(f.label) + valueStyle.Render(displayVal)
			b.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Width(innerW).
				Render(line))
		} else {
			b.WriteString(prefix + labelStyle.Render(f.label) + DimStyle.Render(displayVal))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(labelStyle.Render("  Options:"))
	b.WriteString("\n")

	for i, opt := range p.options {
		row := i + 4
		checkbox := "[ ]"
		checkStyle := lipgloss.NewStyle().Foreground(overlay0)
		if opt.enabled {
			checkbox = "[x]"
			checkStyle = lipgloss.NewStyle().Foreground(green)
		}

		prefix := "  "
		if row == p.cursor {
			prefix = "> "
		}

		if row == p.cursor {
			line := prefix + checkStyle.Render(checkbox) + " " +
				lipgloss.NewStyle().Foreground(peach).Bold(true).Render(opt.label)
			b.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Width(innerW).
				Render(line))
		} else {
			b.WriteString(prefix + checkStyle.Render(checkbox) + " " + NormalItemStyle.Render(opt.label))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Publish button
	btnRow := p.publishButtonRow()
	if p.cursor == btnRow {
		btnStyle := lipgloss.NewStyle().
			Foreground(mantle).
			Background(green).
			Bold(true).
			Padding(0, 2)
		b.WriteString("  " + btnStyle.Render("  Publish  "))
	} else {
		btnStyle := lipgloss.NewStyle().
			Foreground(green).
			Bold(true)
		b.WriteString("  " + btnStyle.Render("[ Publish ]"))
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")
	if p.editing {
		b.WriteString(DimStyle.Render("  Enter: confirm  Esc: cancel  Ctrl+U: clear"))
	} else {
		b.WriteString(DimStyle.Render("  Enter: edit/publish  Space: toggle  Esc: close"))
	}
}

// ---------------------------------------------------------------------------
// Standalone publish function (can be called without the TUI)
// ---------------------------------------------------------------------------

// PublishVault generates a complete static HTML site from all markdown files in
// the vault. It can be invoked independently of the TUI overlay.
func PublishVault(vaultPath, outputDir, siteTitle string) error {
	if vaultPath == "" {
		return fmt.Errorf("vault path is empty")
	}
	if outputDir == "" {
		outputDir = filepath.Join(vaultPath, "_site")
	} else if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(vaultPath, outputDir)
	}
	if siteTitle == "" {
		siteTitle = "Granit"
	}

	_, err := publishSite(vaultPath, outputDir, siteTitle, nil, nil, true, true)
	return err
}

// ---------------------------------------------------------------------------
// Static site generation engine
// ---------------------------------------------------------------------------

// publishPageInfo holds metadata for a single published page.
type publishPageInfo struct {
	relPath string
	title   string
	tags    []string
}

// publishSite generates the full static website from the vault.
func publishSite(vaultPath, outputDir, siteTitle string,
	includePatterns, excludePatterns []string,
	includeTags, includeSearch bool,
) (int, error) {
	if vaultPath == "" {
		return 0, fmt.Errorf("vault path is empty")
	}

	// Create the output directory.
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return 0, fmt.Errorf("create output dir: %w", err)
	}

	// Write the stylesheet.
	if err := os.WriteFile(filepath.Join(outputDir, "style.css"), []byte(publishCSS), 0644); err != nil {
		return 0, fmt.Errorf("write style.css: %w", err)
	}

	// Collect all markdown files.
	var pages []publishPageInfo
	tagIndex := make(map[string][]string) // tag -> list of page titles

	err := filepath.Walk(vaultPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == "_site" || name == "_export" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		relPath, _ := filepath.Rel(vaultPath, path)

		// Apply include patterns – if set, the file must match at least one.
		if len(includePatterns) > 0 {
			matched := false
			for _, pat := range includePatterns {
				if ok, _ := filepath.Match(pat, filepath.Base(relPath)); ok {
					matched = true
					break
				}
				if ok, _ := filepath.Match(pat, relPath); ok {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}

		// Apply exclude patterns – if matched, skip the file.
		for _, pat := range excludePatterns {
			if ok, _ := filepath.Match(pat, filepath.Base(relPath)); ok {
				return nil
			}
			if ok, _ := filepath.Match(pat, relPath); ok {
				return nil
			}
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		src := string(content)
		fm := publishParseFrontmatter(src)
		body := publishStripFrontmatter(src)

		title := strings.TrimSuffix(filepath.Base(relPath), ".md")
		if fmTitle, ok := fm["title"]; ok {
			if s, ok := fmTitle.(string); ok && s != "" {
				title = s
			}
		}

		// Extract tags from frontmatter.
		var tags []string
		if fmTags, ok := fm["tags"]; ok {
			switch v := fmTags.(type) {
			case []string:
				tags = append(tags, v...)
			case string:
				for _, t := range strings.Split(v, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						tags = append(tags, t)
					}
				}
			}
		}
		// Extract inline #tags.
		for _, word := range strings.Fields(body) {
			if strings.HasPrefix(word, "#") && len(word) > 1 {
				tag := strings.TrimRight(word[1:], ".,;:!?)")
				if tag != "" && !strings.HasPrefix(tag, "#") {
					tags = append(tags, tag)
				}
			}
		}
		tags = publishUniqueStrings(tags)

		// Convert markdown to HTML.
		html := publishMarkdownToHTML(body)

		// Build tag pills.
		var tagHTML string
		if len(tags) > 0 {
			var tagParts []string
			for _, tag := range tags {
				tagParts = append(tagParts, fmt.Sprintf(`<a href="/tags.html#%s" class="tag">#%s</a>`, tag, tag))
			}
			tagHTML = `<div class="tags">` + strings.Join(tagParts, " ") + `</div>`
		}

		articleContent := fmt.Sprintf("<h1>%s</h1>\n%s\n%s", publishHTMLEscape(title), tagHTML, html)
		pageHTML := publishWrapPage(title, siteTitle, articleContent, includeSearch)

		outName := strings.TrimSuffix(relPath, ".md") + ".html"
		outPath := filepath.Join(outputDir, outName)
		if mkErr := os.MkdirAll(filepath.Dir(outPath), 0755); mkErr != nil {
			return nil
		}
		if wErr := os.WriteFile(outPath, []byte(pageHTML), 0644); wErr != nil {
			return nil
		}

		pages = append(pages, publishPageInfo{
			relPath: relPath,
			title:   title,
			tags:    tags,
		})

		for _, tag := range tags {
			tagIndex[tag] = append(tagIndex[tag], title)
		}

		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("walk vault: %w", err)
	}

	// Sort pages by title for the index.
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].title < pages[j].title
	})

	// Generate index.html.
	indexHTML := publishBuildIndex(pages, siteTitle, includeSearch)
	if err := os.WriteFile(filepath.Join(outputDir, "index.html"), []byte(indexHTML), 0644); err != nil {
		return 0, fmt.Errorf("write index.html: %w", err)
	}

	// Generate tags.html.
	if includeTags {
		tagsHTML := publishBuildTagsPage(tagIndex, siteTitle, includeSearch)
		if err := os.WriteFile(filepath.Join(outputDir, "tags.html"), []byte(tagsHTML), 0644); err != nil {
			return 0, fmt.Errorf("write tags.html: %w", err)
		}
	}

	return len(pages), nil
}

// ---------------------------------------------------------------------------
// Page generation helpers
// ---------------------------------------------------------------------------

func publishWrapPage(title, siteTitle, content string, includeSearch bool) string {
	searchBlock := ""
	if includeSearch {
		searchBlock = publishSearchScript()
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s — %s</title>
    <link rel="stylesheet" href="/style.css">
</head>
<body>
    <nav>
        <a href="/index.html">Home</a>
        <a href="/tags.html">Tags</a>
        <span class="title">%s</span>
    </nav>
    <main>
        <article>
            %s
        </article>
    </main>%s
</body>
</html>`, publishHTMLEscape(title), publishHTMLEscape(siteTitle),
		publishHTMLEscape(siteTitle), content, searchBlock)
}

func publishBuildIndex(pages []publishPageInfo, siteTitle string, includeSearch bool) string {
	var body strings.Builder
	body.WriteString("<h1>Notes</h1>\n")

	if includeSearch {
		body.WriteString(`<input type="text" id="search" placeholder="Search notes..." autofocus>`)
		body.WriteString("\n")
	}

	body.WriteString("<ul class=\"note-list\">\n")
	for _, page := range pages {
		href := strings.TrimSuffix(page.relPath, ".md") + ".html"
		body.WriteString(fmt.Sprintf("  <li><a href=\"/%s\">%s</a>", href, publishHTMLEscape(page.title)))
		if len(page.tags) > 0 {
			var tagParts []string
			for _, tag := range page.tags {
				tagParts = append(tagParts, fmt.Sprintf(`<a href="/tags.html#%s" class="tag">#%s</a>`, tag, tag))
			}
			body.WriteString(" " + strings.Join(tagParts, " "))
		}
		body.WriteString("</li>\n")
	}
	body.WriteString("</ul>\n")

	searchBlock := ""
	if includeSearch {
		searchBlock = publishSearchScript()
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Home — %s</title>
    <link rel="stylesheet" href="/style.css">
</head>
<body>
    <nav>
        <a href="/index.html" class="active">Home</a>
        <a href="/tags.html">Tags</a>
        <span class="title">%s</span>
    </nav>
    <main>
        <article>
            %s
        </article>
    </main>%s
</body>
</html>`, publishHTMLEscape(siteTitle), publishHTMLEscape(siteTitle),
		body.String(), searchBlock)
}

func publishBuildTagsPage(tagIndex map[string][]string, siteTitle string, includeSearch bool) string {
	var tagNames []string
	for name := range tagIndex {
		tagNames = append(tagNames, name)
	}
	sort.Strings(tagNames)

	var body strings.Builder
	body.WriteString("<h1>Tags</h1>\n")

	// Tag cloud.
	body.WriteString("<div class=\"tag-cloud\">\n")
	for _, name := range tagNames {
		count := len(tagIndex[name])
		body.WriteString(fmt.Sprintf(`  <a href="#%s" class="tag">#%s <span class="count">(%d)</span></a>`+"\n", name, name, count))
	}
	body.WriteString("</div>\n\n")

	// Sections per tag.
	for _, name := range tagNames {
		notes := tagIndex[name]
		sort.Strings(notes)
		body.WriteString(fmt.Sprintf("<h2 id=\"%s\">#%s</h2>\n", name, name))
		body.WriteString("<ul>\n")
		for _, note := range notes {
			href := note + ".html"
			body.WriteString(fmt.Sprintf("  <li><a href=\"/%s\">%s</a></li>\n", href, publishHTMLEscape(note)))
		}
		body.WriteString("</ul>\n\n")
	}

	searchBlock := ""
	if includeSearch {
		searchBlock = publishSearchScript()
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Tags — %s</title>
    <link rel="stylesheet" href="/style.css">
</head>
<body>
    <nav>
        <a href="/index.html">Home</a>
        <a href="/tags.html" class="active">Tags</a>
        <span class="title">%s</span>
    </nav>
    <main>
        <article>
            %s
        </article>
    </main>%s
</body>
</html>`, publishHTMLEscape(siteTitle), publishHTMLEscape(siteTitle),
		body.String(), searchBlock)
}

func publishSearchScript() string {
	return `
    <script>
    (function() {
        var input = document.getElementById('search');
        if (!input) return;
        input.addEventListener('input', function() {
            var filter = this.value.toLowerCase();
            var items = document.querySelectorAll('.note-list li');
            items.forEach(function(li) {
                var text = li.textContent.toLowerCase();
                li.style.display = text.indexOf(filter) !== -1 ? '' : 'none';
            });
        });
    })();
    </script>`
}

// ---------------------------------------------------------------------------
// Publish-specific markdown converter
// ---------------------------------------------------------------------------

var (
	rePublishWikiLink   = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	rePublishNumbered   = regexp.MustCompile(`^(\s*)\d+\.\s+(.+)$`)
	rePublishHRule      = regexp.MustCompile(`^(---+|\*\*\*+|___+)\s*$`)
)

// publishMarkdownToHTML converts markdown to HTML, handling wikilinks for the
// static site (linking to .html files). It supports headings, bold, italic,
// inline code, code blocks, bullet lists, numbered lists, blockquotes,
// horizontal rules, and paragraphs.
func publishMarkdownToHTML(md string) string {
	lines := strings.Split(md, "\n")
	var out strings.Builder
	inCodeBlock := false
	inUL := false
	inOL := false
	inBlockquote := false

	closeUL := func() {
		if inUL {
			out.WriteString("</ul>\n")
			inUL = false
		}
	}
	closeOL := func() {
		if inOL {
			out.WriteString("</ol>\n")
			inOL = false
		}
	}
	closeBQ := func() {
		if inBlockquote {
			out.WriteString("</blockquote>\n")
			inBlockquote = false
		}
	}
	closeLists := func() {
		closeUL()
		closeOL()
	}
	closeAll := func() {
		closeLists()
		closeBQ()
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// --- Code blocks ---
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inCodeBlock {
				out.WriteString("</code></pre>\n")
				inCodeBlock = false
			} else {
				closeAll()
				lang := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "```"))
				if lang != "" {
					out.WriteString(fmt.Sprintf("<pre><code class=\"language-%s\">", lang))
				} else {
					out.WriteString("<pre><code>")
				}
				inCodeBlock = true
			}
			continue
		}
		if inCodeBlock {
			out.WriteString(publishHTMLEscape(line))
			out.WriteString("\n")
			continue
		}

		trimmed := strings.TrimSpace(line)

		// --- Horizontal rule ---
		if rePublishHRule.MatchString(trimmed) {
			closeAll()
			out.WriteString("<hr>\n")
			continue
		}

		// --- Empty line ---
		if trimmed == "" {
			closeAll()
			out.WriteString("\n")
			continue
		}

		// --- Headings ---
		if m := reHeading.FindStringSubmatch(line); m != nil {
			closeAll()
			level := len(m[1])
			content := publishConvertInline(m[2])
			out.WriteString(fmt.Sprintf("<h%d>%s</h%d>\n", level, content, level))
			continue
		}

		// --- Checkboxes ---
		if m := reCheckboxOn.FindStringSubmatch(line); m != nil {
			closeOL()
			closeBQ()
			if !inUL {
				out.WriteString("<ul class=\"checklist\">\n")
				inUL = true
			}
			out.WriteString(fmt.Sprintf("<li><input type=\"checkbox\" checked disabled> %s</li>\n", publishConvertInline(m[2])))
			continue
		}
		if m := reCheckboxOff.FindStringSubmatch(line); m != nil {
			closeOL()
			closeBQ()
			if !inUL {
				out.WriteString("<ul class=\"checklist\">\n")
				inUL = true
			}
			out.WriteString(fmt.Sprintf("<li><input type=\"checkbox\" disabled> %s</li>\n", publishConvertInline(m[2])))
			continue
		}

		// --- Bullet list items ---
		if m := reListItem.FindStringSubmatch(line); m != nil {
			closeOL()
			closeBQ()
			if !inUL {
				out.WriteString("<ul>\n")
				inUL = true
			}
			out.WriteString(fmt.Sprintf("<li>%s</li>\n", publishConvertInline(m[2])))
			continue
		}

		// --- Numbered list items ---
		if m := rePublishNumbered.FindStringSubmatch(line); m != nil {
			closeUL()
			closeBQ()
			if !inOL {
				out.WriteString("<ol>\n")
				inOL = true
			}
			out.WriteString(fmt.Sprintf("<li>%s</li>\n", publishConvertInline(m[2])))
			continue
		}

		// --- Blockquotes ---
		if m := reBlockquote.FindStringSubmatch(line); m != nil {
			closeLists()
			if !inBlockquote {
				out.WriteString("<blockquote>\n")
				inBlockquote = true
			}
			out.WriteString(publishConvertInline(m[1]))
			out.WriteString("<br>\n")
			continue
		}

		// --- Regular paragraph ---
		closeAll()
		out.WriteString(fmt.Sprintf("<p>%s</p>\n", publishConvertInline(trimmed)))
	}

	// Close any open blocks.
	if inCodeBlock {
		out.WriteString("</code></pre>\n")
	}
	closeAll()

	return out.String()
}

// publishConvertInline handles inline formatting and wikilinks for publishing.
func publishConvertInline(s string) string {
	s = publishHTMLEscape(s)
	s = reBold.ReplaceAllString(s, "<strong>$1</strong>")
	s = regexp.MustCompile(`\*([^*]+?)\*`).ReplaceAllString(s, "<em>$1</em>")
	s = reInlineCode.ReplaceAllString(s, "<code>$1</code>")

	// Wikilinks: [[target]] or [[target|display]]
	s = rePublishWikiLink.ReplaceAllStringFunc(s, func(match string) string {
		inner := rePublishWikiLink.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		linkTarget := inner[1]
		displayText := linkTarget
		if idx := strings.Index(linkTarget, "|"); idx >= 0 {
			displayText = linkTarget[idx+1:]
			linkTarget = linkTarget[:idx]
		}
		return fmt.Sprintf(`<a href="/%s.html">%s</a>`, linkTarget, displayText)
	})

	// Standard markdown links.
	s = reMdLink.ReplaceAllString(s, `<a href="$2">$1</a>`)
	return s
}

// publishHTMLEscape escapes HTML special characters for the published site.
func publishHTMLEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// ---------------------------------------------------------------------------
// Frontmatter helpers (self-contained to avoid import cycles)
// ---------------------------------------------------------------------------

func publishParseFrontmatter(content string) map[string]interface{} {
	fm := make(map[string]interface{})
	if !strings.HasPrefix(content, "---") {
		return fm
	}
	end := strings.Index(content[3:], "---")
	if end == -1 {
		return fm
	}
	block := content[3 : 3+end]
	lines := strings.Split(strings.TrimSpace(block), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
				inner := value[1 : len(value)-1]
				items := strings.Split(inner, ",")
				trimmed := make([]string, 0, len(items))
				for _, item := range items {
					trimmed = append(trimmed, strings.TrimSpace(item))
				}
				fm[key] = trimmed
			} else {
				fm[key] = value
			}
		}
	}
	return fm
}

func publishStripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	end := strings.Index(content[3:], "---")
	if end == -1 {
		return content
	}
	return strings.TrimSpace(content[3+end+3:])
}

// publishUniqueStrings removes duplicate strings from a slice.
func publishUniqueStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	var result []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Embedded CSS stylesheet for the published site
// ---------------------------------------------------------------------------

const publishCSS = `/* Granit — published vault stylesheet */
*,
*::before,
*::after {
    box-sizing: border-box;
}

:root {
    --bg: #1e1e2e;
    --bg-secondary: #181825;
    --surface: #313244;
    --text: #cdd6f4;
    --text-dim: #a6adc8;
    --heading: #cba6f7;
    --link: #89b4fa;
    --link-hover: #b4befe;
    --accent: #a6e3a1;
    --tag-bg: #89b4fa;
    --tag-text: #11111b;
    --code-bg: #313244;
    --border: #45475a;
    --blockquote: #cba6f7;
}

@media (prefers-color-scheme: light) {
    :root {
        --bg: #eff1f5;
        --bg-secondary: #e6e9ef;
        --surface: #ccd0da;
        --text: #4c4f69;
        --text-dim: #6c6f85;
        --heading: #8839ef;
        --link: #1e66f5;
        --link-hover: #7287fd;
        --accent: #40a02b;
        --tag-bg: #1e66f5;
        --tag-text: #eff1f5;
        --code-bg: #e6e9ef;
        --border: #bcc0cc;
        --blockquote: #8839ef;
    }
}

html {
    scroll-behavior: smooth;
}

body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
        Oxygen, Ubuntu, Cantarell, "Fira Sans", "Droid Sans",
        "Helvetica Neue", sans-serif;
    max-width: 720px;
    margin: 0 auto;
    padding: 0 1.5rem 4rem;
    line-height: 1.7;
    color: var(--text);
    background: var(--bg);
    -webkit-font-smoothing: antialiased;
}

/* Navigation */
nav {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 1rem 0;
    margin-bottom: 2rem;
    border-bottom: 1px solid var(--border);
}

nav a {
    color: var(--link);
    text-decoration: none;
    font-weight: 500;
    padding: 0.25rem 0.5rem;
    border-radius: 4px;
    transition: background 0.2s, color 0.2s;
}

nav a:hover,
nav a.active {
    background: var(--surface);
    color: var(--link-hover);
}

nav .title {
    margin-left: auto;
    font-weight: 700;
    color: var(--heading);
    font-size: 0.9rem;
    letter-spacing: 0.05em;
    text-transform: uppercase;
}

/* Headings */
h1, h2, h3, h4, h5, h6 {
    color: var(--heading);
    margin-top: 2rem;
    margin-bottom: 0.5rem;
    line-height: 1.3;
}

h1 { font-size: 1.8rem; margin-top: 0; }
h2 { font-size: 1.4rem; }
h3 { font-size: 1.2rem; }

/* Links */
a {
    color: var(--link);
    text-decoration: underline;
    text-decoration-thickness: 1px;
    text-underline-offset: 2px;
    transition: color 0.2s;
}

a:hover {
    color: var(--link-hover);
}

/* Paragraphs */
p {
    margin: 0.8rem 0;
}

/* Code */
code {
    font-family: "JetBrains Mono", "Fira Code", "SF Mono", Consolas,
        "Liberation Mono", Menlo, monospace;
    background: var(--code-bg);
    padding: 0.15em 0.4em;
    border-radius: 4px;
    font-size: 0.88em;
}

pre {
    background: var(--code-bg);
    padding: 1rem 1.25rem;
    border-radius: 8px;
    overflow-x: auto;
    border: 1px solid var(--border);
    margin: 1.2rem 0;
}

pre code {
    background: none;
    padding: 0;
    font-size: 0.85em;
    line-height: 1.6;
}

/* Blockquotes */
blockquote {
    border-left: 3px solid var(--blockquote);
    margin: 1rem 0;
    padding: 0.5rem 0 0.5rem 1.25rem;
    color: var(--text-dim);
    font-style: italic;
}

blockquote p {
    margin: 0.3rem 0;
}

/* Lists */
ul, ol {
    padding-left: 1.5rem;
    margin: 0.8rem 0;
}

li {
    margin: 0.3rem 0;
}

ul.checklist {
    list-style: none;
    padding-left: 0.5rem;
}

ul.checklist li {
    margin: 0.4rem 0;
}

ul.checklist input[type="checkbox"] {
    margin-right: 0.5rem;
    accent-color: var(--accent);
}

/* Note list (index page) */
.note-list {
    list-style: none;
    padding-left: 0;
}

.note-list li {
    padding: 0.5rem 0;
    border-bottom: 1px solid var(--border);
}

.note-list li:last-child {
    border-bottom: none;
}

.note-list a {
    font-weight: 500;
}

/* Tags */
.tag {
    display: inline-block;
    background: var(--tag-bg);
    color: var(--tag-text);
    padding: 0.1em 0.5em;
    border-radius: 4px;
    font-size: 0.8em;
    font-weight: 600;
    text-decoration: none;
    margin: 0.1em 0.15em;
    transition: opacity 0.2s;
}

.tag:hover {
    opacity: 0.85;
    color: var(--tag-text);
}

.tag .count {
    font-weight: 400;
    opacity: 0.8;
}

.tags {
    margin: 0.8rem 0;
}

.tag-cloud {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    margin: 1rem 0 2rem;
}

/* Tables */
table {
    width: 100%;
    border-collapse: collapse;
    margin: 1.2rem 0;
}

th, td {
    text-align: left;
    padding: 0.6rem 0.8rem;
    border: 1px solid var(--border);
}

th {
    background: var(--surface);
    font-weight: 600;
}

tr:nth-child(even) {
    background: var(--bg-secondary);
}

/* Images */
img {
    max-width: 100%;
    height: auto;
    border-radius: 8px;
    margin: 1rem 0;
}

/* Horizontal rule */
hr {
    border: none;
    border-top: 1px solid var(--border);
    margin: 2rem 0;
}

/* Search input */
#search {
    width: 100%;
    padding: 0.6rem 1rem;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface);
    color: var(--text);
    font-size: 1rem;
    margin-bottom: 1.5rem;
    outline: none;
    transition: border-color 0.2s;
}

#search:focus {
    border-color: var(--link);
}

#search::placeholder {
    color: var(--text-dim);
}

/* Responsive adjustments */
@media (max-width: 600px) {
    body {
        padding: 0 1rem 3rem;
    }

    h1 { font-size: 1.5rem; }
    h2 { font-size: 1.2rem; }

    pre {
        padding: 0.75rem;
        font-size: 0.82em;
    }

    nav {
        flex-wrap: wrap;
        gap: 0.5rem;
    }
}
`
