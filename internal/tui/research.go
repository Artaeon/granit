package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// researchResultMsg carries the output of a Claude Code research run.
type researchResultMsg struct {
	output    string
	err       error
	filesHint []string // files Claude created (parsed from output)
}

// researchPhase tracks the current stage of research.
type researchPhase int

const (
	researchInput researchPhase = iota
	researchRunning
	researchDone
	researchError
)

// ResearchAgent is an overlay that invokes Claude Code CLI to research a topic
// and generate structured notes in the vault.
type ResearchAgent struct {
	active    bool
	width     int
	height    int
	phase     researchPhase
	topic     string
	output    string
	errorMsg  string
	scroll    int
	vaultRoot string

	// Options
	depth  int // 0=quick, 1=standard, 2=deep
	format int // 0=zettelkasten, 1=outline, 2=study

	// Selection
	focusField int // 0=topic, 1=depth, 2=format, 3=run button

	// Created files
	createdFiles []string
	selectedFile int

	// Elapsed time
	startTime time.Time
	elapsed   string
}

// NewResearchAgent creates a new research agent overlay.
func NewResearchAgent() ResearchAgent {
	return ResearchAgent{
		depth:  1,
		format: 0,
	}
}

// IsActive returns whether the overlay is visible.
func (r ResearchAgent) IsActive() bool {
	return r.active
}

// SetSize updates dimensions.
func (r *ResearchAgent) SetSize(w, h int) {
	r.width = w
	r.height = h
}

// Open activates the research overlay.
func (r *ResearchAgent) Open(vaultRoot string) {
	r.active = true
	r.phase = researchInput
	r.topic = ""
	r.output = ""
	r.errorMsg = ""
	r.scroll = 0
	r.vaultRoot = vaultRoot
	r.focusField = 0
	r.createdFiles = nil
	r.selectedFile = 0
	r.elapsed = ""
}

// Close hides the overlay.
func (r *ResearchAgent) Close() {
	r.active = false
}

// GetSelectedFile returns the file the user selected from results.
func (r *ResearchAgent) GetSelectedFile() (string, bool) {
	if r.phase == researchDone && len(r.createdFiles) > 0 && r.selectedFile < len(r.createdFiles) {
		path := r.createdFiles[r.selectedFile]
		return path, true
	}
	return "", false
}

// findClaude locates the claude CLI binary.
func findClaude() string {
	paths := []string{
		"claude",
		"/usr/local/bin/claude",
		"/usr/bin/claude",
	}
	if home, err := exec.Command("sh", "-c", "echo $HOME").Output(); err == nil {
		h := strings.TrimSpace(string(home))
		paths = append([]string{
			filepath.Join(h, ".local", "bin", "claude"),
			filepath.Join(h, ".claude", "local", "claude"),
		}, paths...)
	}
	for _, p := range paths {
		if fullPath, err := exec.LookPath(p); err == nil {
			return fullPath
		}
	}
	return ""
}

// runResearch launches claude code to research a topic and create notes.
func (r *ResearchAgent) runResearch() tea.Cmd {
	topic := r.topic
	vaultRoot := r.vaultRoot
	depth := r.depth
	format := r.format

	return func() tea.Msg {
		claudePath := findClaude()
		if claudePath == "" {
			return researchResultMsg{
				err: fmt.Errorf("claude CLI not found - install Claude Code first"),
			}
		}

		prompt := buildResearchPrompt(topic, vaultRoot, depth, format)

		cmd := exec.Command(claudePath,
			"-p", prompt,
			"--output-format", "text",
			"--allowedTools", "Bash(find:*,ls:*,cat:*) Read Write WebSearch Glob Grep",
			"--add-dir", vaultRoot,
		)

		cmd.Env = append(cmd.Environ(), "CLAUDECODE=")

		output, err := cmd.CombinedOutput()
		if err != nil {
			return researchResultMsg{
				output: string(output),
				err:    fmt.Errorf("claude exited with error: %w", err),
			}
		}

		files := parseCreatedFiles(string(output), vaultRoot)

		return researchResultMsg{
			output:    string(output),
			filesHint: files,
		}
	}
}

// buildResearchPrompt creates the prompt for Claude Code.
func buildResearchPrompt(topic, vaultRoot string, depth, format int) string {
	today := time.Now().Format("2006-01-02")

	safeTopic := strings.ReplaceAll(topic, "/", "-")
	safeTopic = strings.ReplaceAll(safeTopic, "\\", "-")
	if len(safeTopic) > 50 {
		safeTopic = safeTopic[:50]
	}
	folder := filepath.Join(vaultRoot, "Research", fmt.Sprintf("%s %s", safeTopic, today))

	var noteCount string
	switch depth {
	case 0:
		noteCount = "5-8 concise notes"
	case 1:
		noteCount = "10-15 detailed notes"
	case 2:
		noteCount = "15-25 comprehensive notes covering every aspect"
	}

	var formatInstr string
	switch format {
	case 0:
		formatInstr = `Use Zettelkasten-style atomic notes. Each note should cover ONE concept or idea.
Use descriptive titles. Link notes extensively with [[wikilinks]].
Create a hub/MOC (Map of Content) note as _Index.md that links to all other notes.`
	case 1:
		formatInstr = `Use a hierarchical outline structure. Create a main overview note as _Index.md,
then create sub-topic notes organized by category. Use [[wikilinks]] to connect them.
Each note can cover broader topics than Zettelkasten but should still be focused.`
	case 2:
		formatInstr = `Create study-oriented notes optimized for learning. Each note should include:
- Clear explanations with examples
- Key takeaways in bullet points
- Flashcard-ready Q&A pairs formatted as "Q: question" / "A: answer" blocks
- Practice exercises or thought experiments where applicable
Create a hub note as _Index.md with a suggested study order.
Use [[wikilinks]] extensively.`
	}

	return fmt.Sprintf(`You are a research assistant creating structured knowledge notes.

TOPIC: %s

INSTRUCTIONS:
1. Research this topic thoroughly using web search to find current, accurate information.
2. Create %s in the folder: %s
3. Create the folder if it doesn't exist.
4. %s

FORMAT for each note:
- Start with YAML frontmatter: ---\ndate: %s\ntype: research\ntags: [research, <relevant-tags>]\nsource: <url-if-applicable>\n---
- Use Markdown with proper headings (## for sections)
- Include [[wikilinks]] to other notes you create (use just the filename without .md)
- Be thorough, accurate, and cite sources where possible
- Write in a clear, educational style

IMPORTANT:
- Create ALL files using the Write tool
- The _Index.md hub note should be created LAST and link to everything
- Each filename should be descriptive (e.g., "Concept - Neural Networks.md")
- Do NOT create files outside the research folder
- After creating all files, list the files you created

START RESEARCHING NOW.`, topic, noteCount, folder, formatInstr, today)
}

// parseCreatedFiles extracts file paths from Claude's output.
func parseCreatedFiles(output, vaultRoot string) []string {
	var files []string
	seen := make(map[string]bool)

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ".md") || strings.Contains(line, ".md ") || strings.Contains(line, ".md)") {
			parts := strings.Fields(line)
			for _, p := range parts {
				p = strings.Trim(p, "`,*[]()-\"'")
				if strings.HasSuffix(p, ".md") && strings.Contains(p, "/") {
					if strings.HasPrefix(p, vaultRoot) {
						rel, _ := filepath.Rel(vaultRoot, p)
						if rel != "" && !seen[rel] {
							files = append(files, rel)
							seen[rel] = true
						}
					}
				}
			}
		}
	}
	return files
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles key events for the research overlay.
func (r ResearchAgent) Update(msg tea.KeyMsg) (ResearchAgent, tea.Cmd) {
	switch r.phase {
	case researchInput:
		return r.updateInput(msg)
	case researchRunning:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			r.active = false
			return r, nil
		}
		return r, nil
	case researchDone:
		return r.updateDone(msg)
	case researchError:
		if msg.String() == "esc" || msg.String() == "enter" {
			r.phase = researchInput
			return r, nil
		}
		return r, nil
	}
	return r, nil
}

func (r ResearchAgent) updateInput(msg tea.KeyMsg) (ResearchAgent, tea.Cmd) {
	switch msg.String() {
	case "esc":
		r.active = false
		return r, nil
	case "tab":
		r.focusField = (r.focusField + 1) % 4
		return r, nil
	case "shift+tab":
		r.focusField = (r.focusField + 3) % 4
		return r, nil
	case "enter":
		if r.focusField == 3 && r.topic != "" {
			r.phase = researchRunning
			r.startTime = time.Now()
			return r, tea.Batch(r.runResearch(), r.tickElapsed())
		}
		if r.focusField < 3 {
			r.focusField++
		}
		return r, nil
	case "left":
		if r.focusField == 1 && r.depth > 0 {
			r.depth--
		} else if r.focusField == 2 && r.format > 0 {
			r.format--
		}
		return r, nil
	case "right":
		if r.focusField == 1 && r.depth < 2 {
			r.depth++
		} else if r.focusField == 2 && r.format < 2 {
			r.format++
		}
		return r, nil
	case "backspace":
		if r.focusField == 0 && len(r.topic) > 0 {
			r.topic = r.topic[:len(r.topic)-1]
		}
		return r, nil
	default:
		if r.focusField == 0 {
			ch := msg.String()
			if len(ch) == 1 && ch[0] >= 32 {
				r.topic += ch
			} else if ch == "space" {
				r.topic += " "
			}
		}
		return r, nil
	}
}

type researchTickMsg struct{}

func (r *ResearchAgent) tickElapsed() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return researchTickMsg{}
	})
}

func (r ResearchAgent) updateDone(msg tea.KeyMsg) (ResearchAgent, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		r.active = false
		return r, nil
	case "up", "k":
		if r.selectedFile > 0 {
			r.selectedFile--
		}
		return r, nil
	case "down", "j":
		if r.selectedFile < len(r.createdFiles)-1 {
			r.selectedFile++
		}
		return r, nil
	case "enter":
		r.active = false
		return r, nil
	}
	return r, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (r ResearchAgent) overlayWidth() int {
	w := r.width * 2 / 3
	if w < 56 {
		w = 56
	}
	if w > 80 {
		w = 80
	}
	return w
}

// View renders the research overlay.
func (r ResearchAgent) View() string {
	w := r.overlayWidth()
	innerW := w - 6 // padding + border

	var body string
	switch r.phase {
	case researchInput:
		body = r.viewInput(innerW)
	case researchRunning:
		body = r.viewRunning(innerW)
	case researchDone:
		body = r.viewDone(innerW)
	case researchError:
		body = r.viewError(innerW)
	}

	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Background(mantle).
		Padding(1, 2).
		Width(w)

	return border.Render(body)
}

// ---------------------------------------------------------------------------
// Input view
// ---------------------------------------------------------------------------

func (r ResearchAgent) viewInput(innerW int) string {
	var b strings.Builder

	// Title
	b.WriteString(lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render("  Deep Dive Research"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Powered by Claude Code"))
	b.WriteString("\n\n")

	// ── Topic ──
	label := r.fieldLabel("Topic", 0)
	b.WriteString(label + "\n")

	topicText := r.topic
	if r.focusField == 0 {
		topicText += "█"
	}
	if topicText == "" {
		topicText = DimStyle.Render("Type a research topic...")
	}
	inputW := innerW - 4
	if inputW < 20 {
		inputW = 20
	}
	inputBox := lipgloss.NewStyle().
		Background(surface0).
		Foreground(text).
		Width(inputW).
		Padding(0, 1).
		Render(topicText)
	b.WriteString("  " + inputBox + "\n\n")

	// ── Depth ──
	depthLabels := []string{"Quick", "Standard", "Deep"}
	depthDescs := []string{"5-8 notes", "10-15 notes", "15-25 notes"}
	b.WriteString(r.fieldLabel("Depth", 1) + "\n")
	b.WriteString(r.renderRadio(depthLabels, depthDescs, r.depth, r.focusField == 1, innerW))
	b.WriteString("\n\n")

	// ── Format ──
	fmtLabels := []string{"Zettelkasten", "Outline", "Study Guide"}
	fmtDescs := []string{"atomic notes", "hierarchical", "with flashcards"}
	b.WriteString(r.fieldLabel("Format", 2) + "\n")
	b.WriteString(r.renderRadio(fmtLabels, fmtDescs, r.format, r.focusField == 2, innerW))
	b.WriteString("\n\n")

	// ── Button ──
	if r.topic != "" {
		btnColor := surface0
		btnFg := text
		if r.focusField == 3 {
			btnColor = green
			btnFg = mantle
		}
		btn := lipgloss.NewStyle().
			Background(btnColor).
			Foreground(btnFg).
			Bold(r.focusField == 3).
			Padding(0, 3).
			Render(" Start Research ")
		b.WriteString("  " + btn)
	} else {
		b.WriteString("  " + DimStyle.Render("Enter a topic to begin"))
	}
	b.WriteString("\n\n")

	// ── Help ──
	b.WriteString(DimStyle.Render("  Tab switch  ←→ option  Enter confirm  Esc close"))

	return b.String()
}

// fieldLabel returns a styled label, highlighted when focused.
func (r ResearchAgent) fieldLabel(name string, idx int) string {
	style := DimStyle
	if r.focusField == idx {
		style = lipgloss.NewStyle().Foreground(mauve).Bold(true)
	}
	return style.Render("  " + name)
}

// renderRadio renders a horizontal radio-button selector.
func (r ResearchAgent) renderRadio(labels, descs []string, selected int, focused bool, maxW int) string {
	var parts []string
	for i, label := range labels {
		desc := ""
		if i < len(descs) {
			desc = " " + descs[i]
		}
		full := label + desc

		if i == selected {
			bg := mauve
			if focused {
				bg = peach
			}
			parts = append(parts, lipgloss.NewStyle().
				Foreground(mantle).
				Background(bg).
				Padding(0, 1).
				Render(full))
		} else {
			parts = append(parts, lipgloss.NewStyle().
				Foreground(overlay0).
				Padding(0, 1).
				Render(full))
		}
	}
	return "  " + strings.Join(parts, " ")
}

// ---------------------------------------------------------------------------
// Running view
// ---------------------------------------------------------------------------

func (r ResearchAgent) viewRunning(innerW int) string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render("  Researching..."))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n\n")

	// Topic
	b.WriteString(lipgloss.NewStyle().Foreground(peach).Bold(true).
		Render("  " + r.topic))
	b.WriteString("\n\n")

	// Animated dots
	dots := int(time.Since(r.startTime).Seconds()) % 4
	dotStr := strings.Repeat(".", dots+1)
	spinner := lipgloss.NewStyle().Foreground(green).Bold(true).Render(dotStr)
	b.WriteString("  " + lipgloss.NewStyle().Foreground(lavender).Render("Claude is working") + spinner)
	b.WriteString("\n\n")

	// Progress info
	if r.elapsed != "" {
		b.WriteString("  " + DimStyle.Render("Elapsed: "+r.elapsed))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Description
	infoStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(infoStyle.Render("  Searching the web and creating notes."))
	b.WriteString("\n")
	b.WriteString(infoStyle.Render("  This takes 1-3 min depending on depth."))
	b.WriteString("\n\n")

	b.WriteString(DimStyle.Render("  Esc cancel"))

	return b.String()
}

// ---------------------------------------------------------------------------
// Done view
// ---------------------------------------------------------------------------

func (r ResearchAgent) viewDone(innerW int) string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Foreground(green).Bold(true).
		Render("  Research Complete"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n\n")

	// Summary
	b.WriteString(lipgloss.NewStyle().Foreground(peach).Bold(true).
		Render("  " + r.topic))
	b.WriteString("\n")
	if r.elapsed != "" {
		b.WriteString(DimStyle.Render("  Completed in " + r.elapsed))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if len(r.createdFiles) > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lavender).
			Render(fmt.Sprintf("  %d notes created:", len(r.createdFiles))))
		b.WriteString("\n\n")

		maxVisible := r.height/2 - 10
		if maxVisible < 5 {
			maxVisible = 5
		}
		if maxVisible > len(r.createdFiles) {
			maxVisible = len(r.createdFiles)
		}

		start := 0
		if r.selectedFile >= start+maxVisible {
			start = r.selectedFile - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(r.createdFiles) {
			end = len(r.createdFiles)
		}

		nameStyle := lipgloss.NewStyle().Foreground(text)
		selectedStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
		pathStyle := lipgloss.NewStyle().Foreground(overlay0)

		for i := start; i < end; i++ {
			name := strings.TrimSuffix(filepath.Base(r.createdFiles[i]), ".md")
			dir := filepath.Dir(r.createdFiles[i])

			// Truncate long names
			maxName := innerW - 8
			if maxName < 20 {
				maxName = 20
			}
			if len(name) > maxName {
				name = name[:maxName-1] + "…"
			}

			if i == r.selectedFile {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(peach).Render(ThemeAccentBar) + " ")
				b.WriteString(selectedStyle.Render(name))
				b.WriteString("\n")
				// Show path for selected item
				b.WriteString("      " + pathStyle.Render(dir))
				b.WriteString("\n")
			} else {
				b.WriteString("    ")
				b.WriteString(nameStyle.Render(name))
				b.WriteString("\n")
			}
		}

		if len(r.createdFiles) > maxVisible {
			b.WriteString(DimStyle.Render(fmt.Sprintf("\n  %d/%d shown", maxVisible, len(r.createdFiles))))
			b.WriteString("\n")
		}
	} else {
		b.WriteString(DimStyle.Render("  Notes created in your vault."))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Refresh to see them."))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter open  j/k navigate  Esc close"))

	return b.String()
}

// ---------------------------------------------------------------------------
// Error view
// ---------------------------------------------------------------------------

func (r ResearchAgent) viewError(innerW int) string {
	var b strings.Builder

	b.WriteString(lipgloss.NewStyle().Foreground(red).Bold(true).
		Render("  Research Failed"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n\n")

	// Error message — wrap to innerW
	errMsg := r.errorMsg
	if len(errMsg) > innerW-4 {
		// Simple word wrap
		words := strings.Fields(errMsg)
		var lines []string
		cur := ""
		for _, w := range words {
			if len(cur)+len(w)+1 > innerW-4 {
				lines = append(lines, cur)
				cur = w
			} else {
				if cur != "" {
					cur += " "
				}
				cur += w
			}
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		for _, l := range lines {
			b.WriteString(lipgloss.NewStyle().Foreground(red).Render("  " + l))
			b.WriteString("\n")
		}
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(red).Render("  " + errMsg))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if r.output != "" {
		b.WriteString(DimStyle.Render("  Output:"))
		b.WriteString("\n")
		lines := strings.Split(r.output, "\n")
		maxLines := 8
		for i, line := range lines {
			if i >= maxLines {
				b.WriteString(DimStyle.Render(fmt.Sprintf("  ... %d more lines", len(lines)-i)))
				b.WriteString("\n")
				break
			}
			if len(line) > innerW-4 {
				line = line[:innerW-4]
			}
			b.WriteString(DimStyle.Render("  " + line))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter/Esc back"))

	return b.String()
}
