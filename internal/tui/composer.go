package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// composerResultMsg — carries the AI-generated note back to Update
// ---------------------------------------------------------------------------

type composerResultMsg struct {
	content string
	err     error
}

// ---------------------------------------------------------------------------
// Composer — AI-powered note composer overlay
// ---------------------------------------------------------------------------

const (
	composerModeInput   = 0 // entering a topic / prompt
	composerModePreview = 1 // previewing the generated note
	composerModeTitle   = 2 // editing the filename
)

type Composer struct {
	active bool
	width  int
	height int

	prompt           string // user's topic / instruction
	generatedContent string // AI-generated note content
	loading          bool
	done             bool // generation complete

	ai AIConfig

	existingNotes []string            // vault note names for wikilink suggestions
	noteContents  map[string]string   // vault note contents for context

	mode   int    // composerModeInput / composerModePreview / composerModeTitle
	title  string // filename for the new note
	scroll int    // scroll offset in preview

	// consumed-once result
	resultTitle   string
	resultContent string
	resultReady   bool

	// loading animation
	loadingTick int

	// error message
	errMsg string
}

func NewComposer() Composer {
	return Composer{
		ai: AIConfig{
			Provider:  "ollama",
			Model:     "llama3.2",
			OllamaURL: "http://localhost:11434",
		},
	}
}

// ---------------------------------------------------------------------------
// Overlay interface
// ---------------------------------------------------------------------------

func (c *Composer) IsActive() bool { return c.active }

func (c *Composer) Open() {
	c.active = true
	c.mode = composerModeInput
	c.prompt = ""
	c.generatedContent = ""
	c.loading = false
	c.done = false
	c.title = ""
	c.scroll = 0
	c.resultReady = false
	c.resultTitle = ""
	c.resultContent = ""
	c.loadingTick = 0
	c.errMsg = ""
}

func (c *Composer) Close() {
	c.active = false
}

func (c *Composer) SetSize(width, height int) {
	c.width = width
	c.height = height
}

func (c *Composer) SetExistingNotes(notes []string) {
	c.existingNotes = notes
}

func (c *Composer) SetNoteContents(contents map[string]string) {
	c.noteContents = contents
}

// ---------------------------------------------------------------------------
// GetResult — consumed once after user accepts a generated note
// ---------------------------------------------------------------------------

func (c *Composer) GetResult() (title, content string, ok bool) {
	if !c.resultReady {
		return "", "", false
	}
	t := c.resultTitle
	ct := c.resultContent
	c.resultReady = false
	c.resultTitle = ""
	c.resultContent = ""
	return t, ct, true
}

// ---------------------------------------------------------------------------
// AI generation
// ---------------------------------------------------------------------------

func (c *Composer) buildSystemPrompt() string {
	noteList := ""
	if len(c.existingNotes) > 0 {
		limit := len(c.existingNotes)
		if limit > 50 {
			limit = 50
		}
		noteList = strings.Join(c.existingNotes[:limit], ", ")
	}

	// Find relevant vault context based on the user's topic
	vaultContext := ""
	if len(c.noteContents) > 0 && c.prompt != "" {
		topicWords := strings.Fields(strings.ToLower(c.prompt))
		type scored struct {
			path  string
			score int
		}
		var matches []scored
		for path, content := range c.noteContents {
			lower := strings.ToLower(content) + " " + strings.ToLower(path)
			score := 0
			for _, w := range topicWords {
				if len(w) > 2 {
					score += strings.Count(lower, w)
				}
			}
			if score > 0 {
				matches = append(matches, scored{path, score})
			}
		}
		// Sort by score descending
		for i := 0; i < len(matches); i++ {
			for j := i + 1; j < len(matches); j++ {
				if matches[j].score > matches[i].score {
					matches[i], matches[j] = matches[j], matches[i]
				}
			}
		}
		// Take top 5 relevant notes
		var contextBuf strings.Builder
		contextBuf.WriteString("\n\nRELEVANT EXISTING NOTES (use these for context and wikilinks):\n")
		limit := 5
		if len(matches) < limit {
			limit = len(matches)
		}
		for i := 0; i < limit; i++ {
			preview := c.noteContents[matches[i].path]
			if len(preview) > 300 {
				preview = preview[:300]
			}
			preview = strings.ReplaceAll(preview, "\n", " ")
			contextBuf.WriteString(fmt.Sprintf("- [[%s]]: %s\n", strings.TrimSuffix(matches[i].path, ".md"), preview))
		}
		vaultContext = contextBuf.String()
	}

	sys := "You are a note-taking assistant. Generate a well-structured markdown note about the given topic. Include: " +
		"1) YAML frontmatter with title, date (YYYY-MM-DD), and relevant tags. " +
		"2) Clear headings (##, ###). " +
		"3) Where relevant, include [[wikilinks]] to these existing notes: " + noteList + ". " +
		"4) Use bullet points, numbered lists, and code blocks where appropriate. " +
		"5) Keep it informative but concise. " +
		"6) Build on existing knowledge from the user's vault when relevant." +
		vaultContext

	return sys
}

func (c *Composer) generateNote() tea.Cmd {
	systemPrompt := c.buildSystemPrompt()
	userPrompt := c.prompt
	ai := c.ai

	return func() tea.Msg {
		resp, err := ai.Chat(systemPrompt, userPrompt)
		return composerResultMsg{content: resp, err: err}
	}
}

// ---------------------------------------------------------------------------
// Loading tick
// ---------------------------------------------------------------------------

type composerTickMsg struct{}

func composerTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return composerTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (c Composer) Update(msg tea.Msg) (Composer, tea.Cmd) {
	if !c.active {
		return c, nil
	}

	switch msg := msg.(type) {

	case composerResultMsg:
		c.loading = false
		if msg.err != nil {
			c.errMsg = msg.err.Error()
			c.mode = composerModeInput
			return c, nil
		}
		c.generatedContent = msg.content
		c.done = true
		c.mode = composerModePreview
		c.scroll = 0
		// Derive a default title from the prompt
		c.title = composerTitleFromPrompt(c.prompt)
		return c, nil

	case composerTickMsg:
		if c.loading {
			c.loadingTick++
			return c, composerTickCmd()
		}
		return c, nil

	case tea.KeyMsg:
		switch c.mode {

		// ----- Prompt input mode -----
		case composerModeInput:
			return c.updateInput(msg)

		// ----- Preview mode -----
		case composerModePreview:
			return c.updatePreview(msg)

		// ----- Title edit mode -----
		case composerModeTitle:
			return c.updateTitle(msg)
		}
	}

	return c, nil
}

func (c Composer) updateInput(msg tea.KeyMsg) (Composer, tea.Cmd) {
	switch msg.String() {
	case "esc":
		c.active = false
		return c, nil
	case "enter":
		if strings.TrimSpace(c.prompt) == "" {
			return c, nil
		}
		c.loading = true
		c.loadingTick = 0
		c.errMsg = ""
		return c, tea.Batch(c.generateNote(), composerTickCmd())
	case "backspace":
		if len(c.prompt) > 0 {
			c.prompt = c.prompt[:len(c.prompt)-1]
		}
	case "ctrl+u":
		c.prompt = ""
	default:
		if len(msg.String()) == 1 || msg.Type == tea.KeySpace || msg.Type == tea.KeyRunes {
			c.prompt += msg.String()
		}
	}
	return c, nil
}

func (c Composer) updatePreview(msg tea.KeyMsg) (Composer, tea.Cmd) {
	switch msg.String() {
	case "esc":
		c.mode = composerModeInput
		c.done = false
		c.scroll = 0
		return c, nil
	case "enter":
		// Accept: store the result
		c.resultTitle = c.title
		c.resultContent = c.generatedContent
		c.resultReady = true
		c.active = false
		return c, nil
	case "r":
		// Regenerate
		c.loading = true
		c.loadingTick = 0
		c.done = false
		c.generatedContent = ""
		c.errMsg = ""
		return c, tea.Batch(c.generateNote(), composerTickCmd())
	case "e":
		c.mode = composerModeTitle
		return c, nil
	case "up", "k":
		if c.scroll > 0 {
			c.scroll--
		}
	case "down", "j":
		lines := strings.Count(c.generatedContent, "\n") + 1
		maxScroll := lines - (c.height - 16)
		if maxScroll < 0 {
			maxScroll = 0
		}
		if c.scroll < maxScroll {
			c.scroll++
		}
	}
	return c, nil
}

func (c Composer) updateTitle(msg tea.KeyMsg) (Composer, tea.Cmd) {
	switch msg.String() {
	case "esc":
		c.mode = composerModePreview
		return c, nil
	case "enter":
		if strings.TrimSpace(c.title) == "" {
			return c, nil
		}
		// Accept with the edited title
		c.resultTitle = c.title
		c.resultContent = c.generatedContent
		c.resultReady = true
		c.active = false
		return c, nil
	case "backspace":
		if len(c.title) > 0 {
			c.title = c.title[:len(c.title)-1]
		}
	case "ctrl+u":
		c.title = ""
	default:
		if len(msg.String()) == 1 || msg.Type == tea.KeySpace || msg.Type == tea.KeyRunes {
			c.title += msg.String()
		}
	}
	return c, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (c Composer) View() string {
	width := c.width / 2
	if width < 60 {
		width = 60
	}
	if width > 90 {
		width = 90
	}

	var b strings.Builder

	switch {
	case c.loading:
		b.WriteString(c.viewLoading(width))
	case c.mode == composerModeInput:
		b.WriteString(c.viewInput(width))
	case c.mode == composerModePreview:
		b.WriteString(c.viewPreview(width))
	case c.mode == composerModeTitle:
		b.WriteString(c.viewTitle(width))
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (c Composer) viewInput(width int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  AI Note Composer")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	b.WriteString(NormalItemStyle.Render("  Enter a topic or instruction for AI to compose a note:"))
	b.WriteString("\n\n")

	// Prompt input line
	promptStyle := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true)
	inputStyle := lipgloss.NewStyle().
		Foreground(text).
		Background(surface0).
		Padding(0, 1).
		Width(width - 12)

	b.WriteString("  " + promptStyle.Render("> "))
	displayPrompt := c.prompt + "\u2588" // block cursor
	b.WriteString(inputStyle.Render(displayPrompt))
	b.WriteString("\n\n")

	if c.errMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(red)
		b.WriteString("  " + errStyle.Render("Error: "+c.errMsg))
		b.WriteString("\n\n")
	}

	// Examples
	b.WriteString(DimStyle.Render("  Examples:"))
	b.WriteString("\n")

	exampleStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
	examples := []string{
		"Machine Learning basics",
		"Meeting notes for project X",
		"Compare React vs Vue",
	}
	for _, ex := range examples {
		b.WriteString("    " + exampleStyle.Render("\""+ex+"\""))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter: generate  Esc: close"))

	return b.String()
}

func (c Composer) viewLoading(width int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  AI Note Composer")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	spinner := []string{"\u280b", "\u2819", "\u2838", "\u2834", "\u2826", "\u2807"}
	frame := spinner[c.loadingTick%len(spinner)]

	loadStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	b.WriteString("  " + loadStyle.Render(frame+" Generating note..."))
	b.WriteString("\n\n")

	promptDisplay := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
	b.WriteString("  " + DimStyle.Render("Topic: ") + promptDisplay.Render(c.prompt))
	b.WriteString("\n\n")

	providerDisplay := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString("  " + providerDisplay.Render("Provider: "+c.ai.Provider+"  Model: "+c.ai.Model))

	return b.String()
}

func (c Composer) viewPreview(width int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		Render("  Note Generated")
	b.WriteString(title)
	b.WriteString("\n")

	filenameStyle := lipgloss.NewStyle().Foreground(peach)
	b.WriteString("  " + DimStyle.Render("File: ") + filenameStyle.Render(c.title+".md"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")

	// Render preview with basic syntax highlighting
	contentLines := strings.Split(c.generatedContent, "\n")

	visibleHeight := c.height - 16
	if visibleHeight < 5 {
		visibleHeight = 5
	}

	start := c.scroll
	if start > len(contentLines) {
		start = len(contentLines)
	}
	end := start + visibleHeight
	if end > len(contentLines) {
		end = len(contentLines)
	}

	contentWidth := width - 8
	if contentWidth < 20 {
		contentWidth = 20
	}

	for i := start; i < end; i++ {
		line := contentLines[i]
		rendered := composerHighlightLine(line, contentWidth)
		b.WriteString("  " + rendered)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	if end < len(contentLines) {
		b.WriteString("\n")
		scrollInfo := lipgloss.NewStyle().Foreground(overlay0)
		remaining := len(contentLines) - end
		b.WriteString("  " + scrollInfo.Render(fmt.Sprintf("... %d more lines (j/k to scroll)", remaining)))
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter: accept  r: regenerate  e: edit title  Esc: back"))

	return b.String()
}

func (c Composer) viewTitle(width int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Edit Note Title")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	b.WriteString(NormalItemStyle.Render("  Enter a filename for the new note:"))
	b.WriteString("\n\n")

	promptStyle := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true)
	inputStyle := lipgloss.NewStyle().
		Foreground(text).
		Background(surface0).
		Padding(0, 1).
		Width(width - 12)

	b.WriteString("  " + promptStyle.Render("> "))
	displayTitle := c.title + "\u2588"
	b.WriteString(inputStyle.Render(displayTitle))
	b.WriteString("\n\n")

	extStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString("  " + extStyle.Render("Extension .md will be added automatically"))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter: confirm  Esc: back to preview"))

	return b.String()
}

// ---------------------------------------------------------------------------
// Syntax highlighting (basic)
// ---------------------------------------------------------------------------

func composerHighlightLine(line string, maxWidth int) string {
	if maxWidth > 0 && len(line) > maxWidth {
		line = line[:maxWidth]
	}

	headingStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	boldStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	wikilinkStyle := lipgloss.NewStyle().Foreground(blue).Underline(true)
	codeStyle := lipgloss.NewStyle().Foreground(yellow)
	frontmatterStyle := lipgloss.NewStyle().Foreground(overlay0)
	normalStyle := lipgloss.NewStyle().Foreground(text)

	trimmed := strings.TrimSpace(line)

	// Frontmatter delimiters
	if trimmed == "---" {
		return frontmatterStyle.Render(line)
	}

	// Headings
	if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") ||
		strings.HasPrefix(trimmed, "### ") || strings.HasPrefix(trimmed, "#### ") {
		return headingStyle.Render(line)
	}

	// Inline code block marker
	if strings.HasPrefix(trimmed, "```") {
		return codeStyle.Render(line)
	}

	// Process inline elements: wikilinks, bold, inline code
	result := composerHighlightInline(line, normalStyle, boldStyle, wikilinkStyle, codeStyle)
	return result
}

func composerHighlightInline(line string, normal, bold, wikilink, code lipgloss.Style) string {
	var b strings.Builder
	i := 0
	n := len(line)

	for i < n {
		// Wikilinks [[...]]
		if i+1 < n && line[i] == '[' && line[i+1] == '[' {
			end := strings.Index(line[i+2:], "]]")
			if end >= 0 {
				linkText := line[i : i+2+end+2]
				b.WriteString(wikilink.Render(linkText))
				i = i + 2 + end + 2
				continue
			}
		}

		// Inline code `...`
		if line[i] == '`' {
			end := strings.Index(line[i+1:], "`")
			if end >= 0 {
				codeText := line[i : i+1+end+1]
				b.WriteString(code.Render(codeText))
				i = i + 1 + end + 1
				continue
			}
		}

		// Bold **...**
		if i+1 < n && line[i] == '*' && line[i+1] == '*' {
			end := strings.Index(line[i+2:], "**")
			if end >= 0 {
				boldText := line[i+2 : i+2+end]
				b.WriteString(bold.Render("**" + boldText + "**"))
				i = i + 2 + end + 2
				continue
			}
		}

		b.WriteString(normal.Render(string(line[i])))
		i++
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func composerTitleFromPrompt(prompt string) string {
	// Derive a reasonable filename from the prompt
	title := strings.TrimSpace(prompt)
	if len(title) > 60 {
		title = title[:60]
	}
	// Replace characters that are problematic in filenames
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "",
		"?", "",
		"\"", "",
		"<", "",
		">", "",
		"|", "",
	)
	title = replacer.Replace(title)
	if title == "" {
		title = "Untitled"
	}
	return title
}
