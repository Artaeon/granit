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
	depth       int    // 0=quick, 1=standard, 2=deep
	format      int    // 0=zettelkasten, 1=outline, 2=study
	depthLabels []string
	formatLabels []string

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
		depth:        1,
		format:       0,
		depthLabels:  []string{"Quick (5-8 notes)", "Standard (10-15 notes)", "Deep Dive (15-25 notes)"},
		formatLabels: []string{"Zettelkasten (atomic notes)", "Outline (hierarchical)", "Study Guide (with flashcards)"},
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
	// Check common locations
	paths := []string{
		"claude",
		"/usr/local/bin/claude",
		"/usr/bin/claude",
	}
	// Check home local bin
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
				err: fmt.Errorf("claude CLI not found — install Claude Code first: https://docs.anthropic.com/en/docs/claude-code"),
			}
		}

		// Build the research prompt
		prompt := buildResearchPrompt(topic, vaultRoot, depth, format)

		// Run claude in non-interactive mode
		cmd := exec.Command(claudePath,
			"-p", prompt,
			"--output-format", "text",
			"--allowedTools", "Bash(find:*,ls:*,cat:*) Read Write WebSearch Glob Grep",
			"--add-dir", vaultRoot,
		)

		// Unset CLAUDECODE env var to allow nested execution
		cmd.Env = append(cmd.Environ(), "CLAUDECODE=")

		output, err := cmd.CombinedOutput()
		if err != nil {
			return researchResultMsg{
				output: string(output),
				err:    fmt.Errorf("claude exited with error: %w", err),
			}
		}

		// Parse created files from output
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

	// Determine folder
	safeTopic := strings.ReplaceAll(topic, "/", "-")
	safeTopic = strings.ReplaceAll(safeTopic, "\\", "-")
	if len(safeTopic) > 50 {
		safeTopic = safeTopic[:50]
	}
	folder := filepath.Join(vaultRoot, "Research", fmt.Sprintf("%s %s", safeTopic, today))

	// Depth instructions
	var noteCount string
	switch depth {
	case 0:
		noteCount = "5-8 concise notes"
	case 1:
		noteCount = "10-15 detailed notes"
	case 2:
		noteCount = "15-25 comprehensive notes covering every aspect"
	}

	// Format instructions
	var formatInstr string
	switch format {
	case 0: // Zettelkasten
		formatInstr = `Use Zettelkasten-style atomic notes. Each note should cover ONE concept or idea.
Use descriptive titles. Link notes extensively with [[wikilinks]].
Create a hub/MOC (Map of Content) note as _Index.md that links to all other notes.`
	case 1: // Outline
		formatInstr = `Use a hierarchical outline structure. Create a main overview note as _Index.md,
then create sub-topic notes organized by category. Use [[wikilinks]] to connect them.
Each note can cover broader topics than Zettelkasten but should still be focused.`
	case 2: // Study Guide
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
		// Look for markdown file references
		if strings.HasSuffix(line, ".md") || strings.Contains(line, ".md ") || strings.Contains(line, ".md)") {
			// Try to extract path
			parts := strings.Fields(line)
			for _, p := range parts {
				p = strings.Trim(p, "`,*[]()-\"'")
				if strings.HasSuffix(p, ".md") && strings.Contains(p, "/") {
					// Make relative to vault
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

// Update handles key events for the research overlay.
func (r ResearchAgent) Update(msg tea.KeyMsg) (ResearchAgent, tea.Cmd) {
	switch r.phase {
	case researchInput:
		return r.updateInput(msg)
	case researchRunning:
		// No interaction while running
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
			// Launch research
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
		// Signal to open the selected file
		r.active = false
		return r, nil
	case "pgup":
		r.scroll -= 10
		if r.scroll < 0 {
			r.scroll = 0
		}
		return r, nil
	case "pgdown":
		r.scroll += 10
		return r, nil
	}
	return r, nil
}

// View renders the research overlay.
func (r ResearchAgent) View() string {
	w := r.width * 3 / 4
	if w > 90 {
		w = 90
	}
	if w < 50 {
		w = 50
	}
	h := r.height * 3 / 4
	if h > 35 {
		h = 35
	}
	if h < 15 {
		h = 15
	}

	var content string
	switch r.phase {
	case researchInput:
		content = r.viewInput(w, h)
	case researchRunning:
		content = r.viewRunning(w, h)
	case researchDone:
		content = r.viewDone(w, h)
	case researchError:
		content = r.viewError(w, h)
	}

	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Background(mantle).
		Padding(1, 2).
		Width(w).
		Height(h)

	return border.Render(content)
}

func (r ResearchAgent) viewInput(w, h int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Deep Dive — AI Research Agent")
	b.WriteString(title + "\n")
	b.WriteString(DimStyle.Render("  Powered by Claude Code") + "\n")
	b.WriteString(ThemeSeparator + "\n\n")

	// Topic field
	topicLabel := "  Topic:"
	if r.focusField == 0 {
		topicLabel = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  Topic:")
	}
	b.WriteString(topicLabel + "\n")
	topicBox := r.topic
	if r.focusField == 0 {
		topicBox += "█"
	}
	if topicBox == "" && r.focusField != 0 {
		topicBox = DimStyle.Render("(enter research topic)")
	}
	inputStyle := lipgloss.NewStyle().
		Foreground(text).
		Background(surface0).
		Padding(0, 1).
		Width(w - 8)
	b.WriteString("  " + inputStyle.Render(topicBox) + "\n\n")

	// Depth selector
	depthLabel := "  Depth:"
	if r.focusField == 1 {
		depthLabel = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  Depth:")
	}
	b.WriteString(depthLabel + "\n  ")
	for i, label := range r.depthLabels {
		if i == r.depth {
			b.WriteString(lipgloss.NewStyle().
				Foreground(mantle).
				Background(mauve).
				Padding(0, 1).
				Render(label))
		} else {
			b.WriteString(lipgloss.NewStyle().
				Foreground(overlay0).
				Padding(0, 1).
				Render(label))
		}
		if i < len(r.depthLabels)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteString("\n\n")

	// Format selector
	formatLabel := "  Format:"
	if r.focusField == 2 {
		formatLabel = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  Format:")
	}
	b.WriteString(formatLabel + "\n  ")
	for i, label := range r.formatLabels {
		if i == r.format {
			b.WriteString(lipgloss.NewStyle().
				Foreground(mantle).
				Background(peach).
				Padding(0, 1).
				Render(label))
		} else {
			b.WriteString(lipgloss.NewStyle().
				Foreground(overlay0).
				Padding(0, 1).
				Render(label))
		}
		if i < len(r.formatLabels)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteString("\n\n")

	// Run button
	if r.focusField == 3 && r.topic != "" {
		b.WriteString("  " + lipgloss.NewStyle().
			Foreground(mantle).
			Background(green).
			Bold(true).
			Padding(0, 3).
			Render("  Start Research  "))
	} else if r.topic != "" {
		b.WriteString("  " + lipgloss.NewStyle().
			Foreground(text).
			Background(surface0).
			Padding(0, 3).
			Render("  Start Research  "))
	} else {
		b.WriteString("  " + DimStyle.Render("  Enter a topic to begin  "))
	}
	b.WriteString("\n\n")

	// Help
	b.WriteString(DimStyle.Render("  Tab: switch fields  ←→: change option  Enter: confirm  Esc: close"))

	return b.String()
}

func (r ResearchAgent) viewRunning(w, h int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Researching...")
	b.WriteString(title + "\n")
	b.WriteString(ThemeSeparator + "\n\n")

	b.WriteString(lipgloss.NewStyle().Foreground(peach).Bold(true).
		Render("  Topic: ") + r.topic + "\n\n")

	// Spinner animation
	frames := []string{"  ", " ", " ", " "}
	idx := int(time.Since(r.startTime).Seconds()) % len(frames)
	spinner := lipgloss.NewStyle().Foreground(green).Render(frames[idx])

	b.WriteString(spinner + " Claude Code is researching and creating notes...\n\n")

	if r.elapsed != "" {
		b.WriteString(DimStyle.Render("  Elapsed: " + r.elapsed) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  This may take 1-3 minutes depending on depth."))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Claude will search the web and create structured notes."))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  Esc: cancel"))

	return b.String()
}

func (r ResearchAgent) viewDone(w, h int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		Render("  Research Complete!")
	b.WriteString(title + "\n")
	b.WriteString(ThemeSeparator + "\n\n")

	b.WriteString(lipgloss.NewStyle().Foreground(peach).Bold(true).
		Render("  Topic: ") + r.topic + "\n")
	if r.elapsed != "" {
		b.WriteString(DimStyle.Render("  Time: " + r.elapsed) + "\n")
	}
	b.WriteString("\n")

	if len(r.createdFiles) > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(mauve).Bold(true).
			Render(fmt.Sprintf("  Created %d notes:", len(r.createdFiles))) + "\n\n")

		maxVisible := h - 12
		if maxVisible < 5 {
			maxVisible = 5
		}

		start := 0
		if r.selectedFile >= maxVisible {
			start = r.selectedFile - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(r.createdFiles) {
			end = len(r.createdFiles)
		}

		for i := start; i < end; i++ {
			name := filepath.Base(r.createdFiles[i])
			name = strings.TrimSuffix(name, ".md")

			if i == r.selectedFile {
				b.WriteString("  " + ThemeAccentBar + " ")
				b.WriteString(lipgloss.NewStyle().Foreground(peach).Bold(true).Render(name))
			} else {
				b.WriteString("    ")
				b.WriteString(lipgloss.NewStyle().Foreground(text).Render(name))
			}
			b.WriteString("\n")
		}
	} else {
		b.WriteString(DimStyle.Render("  Notes were created in your vault.") + "\n")
		b.WriteString(DimStyle.Render("  Refresh the vault to see them.") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter: open note  j/k: navigate  Esc: close"))

	return b.String()
}

func (r ResearchAgent) viewError(w, h int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(red).
		Bold(true).
		Render("  Research Failed")
	b.WriteString(title + "\n")
	b.WriteString(ThemeSeparator + "\n\n")

	b.WriteString(lipgloss.NewStyle().Foreground(red).Render("  " + r.errorMsg) + "\n\n")

	if r.output != "" {
		b.WriteString(DimStyle.Render("  Output:") + "\n")
		lines := strings.Split(r.output, "\n")
		maxLines := h - 10
		if maxLines < 3 {
			maxLines = 3
		}
		for i, line := range lines {
			if i >= maxLines {
				b.WriteString(DimStyle.Render(fmt.Sprintf("  ... (%d more lines)", len(lines)-i)) + "\n")
				break
			}
			if len(line) > w-6 {
				line = line[:w-6]
			}
			b.WriteString(DimStyle.Render("  "+line) + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter/Esc: back to input"))

	return b.String()
}
