package tui

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/config"
)

// onboardingDoneFile is the sentinel file written to mark the tutorial as completed.
const onboardingDoneFile = "onboarding_done"

// Onboarding provides a multi-step interactive tutorial overlay for first-time
// users. It walks through the main features of Granit with styled slides,
// keybinding highlights, and a progress indicator. After the user finishes
// (or skips), a sentinel file is written so the tutorial is not shown again.
type Onboarding struct {
	active     bool
	step       int
	width      int
	height     int
	totalSteps int
	skipped    bool
	vaultRoot  string
	statusMsg  string
}

// NewOnboarding returns an Onboarding in its default (inactive) state.
func NewOnboarding() Onboarding {
	return Onboarding{
		totalSteps: 10,
	}
}

// IsActive reports whether the onboarding overlay is currently visible.
func (o Onboarding) IsActive() bool {
	return o.active
}

// Open activates the onboarding overlay, resetting to the first step.
func (o *Onboarding) Open() {
	o.active = true
	o.step = 0
	o.skipped = false
}

// Close deactivates the onboarding overlay.
func (o *Onboarding) Close() {
	o.active = false
}

// SetSize updates the available terminal dimensions for layout.
func (o *Onboarding) SetSize(w, h int) {
	o.width = w
	o.height = h
}

// MarkComplete writes the sentinel file so the tutorial is not shown again.
func (o *Onboarding) MarkComplete() {
	dir := config.ConfigDir()
	_ = os.MkdirAll(dir, 0o700)
	f, err := os.Create(filepath.Join(dir, onboardingDoneFile))
	if err == nil {
		_ = f.Close()
	}
}

// ShouldShow returns true when the onboarding sentinel file does not yet exist,
// meaning the user has never completed (or skipped) the tutorial.
func ShouldShowOnboarding() bool {
	p := filepath.Join(config.ConfigDir(), onboardingDoneFile)
	_, err := os.Stat(p)
	return os.IsNotExist(err)
}

// Update handles key events for navigating between onboarding steps.
func (o Onboarding) Update(msg tea.Msg) (Onboarding, tea.Cmd) {
	if !o.active {
		return o, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			if o.step > 0 {
				o.step--
			}
		case "right", "l", "enter", " ":
			if o.step < o.totalSteps-1 {
				o.step++
			} else {
				// Last step — close and mark done
				o.active = false
				o.MarkComplete()
			}
		case "s":
			if o.step == o.totalSteps-1 && o.vaultRoot != "" {
				if err := createSampleVault(o.vaultRoot); err == nil {
					o.statusMsg = "Sample notes created!"
				} else {
					o.statusMsg = "Error: " + err.Error()
				}
			}
		case "q", "esc":
			o.active = false
			o.skipped = true
			o.MarkComplete()
		case "1":
			o.step = 0
		case "2":
			o.step = 1
		case "3":
			o.step = 2
		case "4":
			o.step = 3
		case "5":
			o.step = 4
		case "6":
			o.step = 5
		case "7":
			o.step = 6
		case "8":
			o.step = 7
		case "9":
			o.step = 8
		case "0":
			o.step = 9
		}
	}
	return o, nil
}

// View renders the current onboarding step as a centered, bordered overlay.
func (o Onboarding) View() string {
	width := o.width * 2 / 3
	if width < 64 {
		width = 64
	}
	if width > 86 {
		width = 86
	}

	innerWidth := width - 8 // account for border + padding

	var b strings.Builder

	// Step title and content
	title, content := o.stepContent(innerWidth)

	// ── Header ──────────────────────────────────────────────────────────
	stepLabel := lipgloss.NewStyle().
		Foreground(surface2).
		Render(itoa(o.step+1) + "/" + itoa(o.totalSteps))

	titleStyle := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true)

	b.WriteString(titleStyle.Render(title))
	// Right-align step counter on same line
	titleWidth := lipgloss.Width(title)
	stepWidth := lipgloss.Width(stepLabel)
	gap := innerWidth - titleWidth - stepWidth
	if gap < 1 {
		gap = 1
	}
	b.WriteString(strings.Repeat(" ", gap))
	b.WriteString(stepLabel)
	b.WriteString("\n")

	// Divider
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n\n")

	// Content
	b.WriteString(content)

	// ── Progress bar ────────────────────────────────────────────────────
	b.WriteString("\n\n")
	b.WriteString(o.renderProgress(innerWidth))
	b.WriteString("\n")

	// ── Footer ──────────────────────────────────────────────────────────
	b.WriteString(o.renderFooter(innerWidth))

	// ── Outer box ───────────────────────────────────────────────────────
	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 3).
		Width(width).
		Background(mantle)

	rendered := box.Render(b.String())

	// Center on screen
	return lipgloss.Place(
		o.width,
		o.height,
		lipgloss.Center,
		lipgloss.Center,
		rendered,
	)
}

// ── Progress bar ────────────────────────────────────────────────────────────

func (o Onboarding) renderProgress(availWidth int) string {
	barWidth := availWidth - 4
	if barWidth < 10 {
		barWidth = 10
	}

	total := o.totalSteps
	if total <= 0 {
		total = 10
	}
	filled := (o.step + 1) * barWidth / total
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled

	filledStyle := lipgloss.NewStyle().Foreground(mauve)
	emptyStyle := lipgloss.NewStyle().Foreground(surface1)

	bar := "  " +
		filledStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", empty))

	return bar
}

// ── Footer ──────────────────────────────────────────────────────────────────

func (o Onboarding) renderFooter(availWidth int) string {
	keyStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)

	var parts []string

	if o.step > 0 {
		parts = append(parts, keyStyle.Render("←")+dimStyle.Render(" prev"))
	}

	parts = append(parts, dimStyle.Render("step ")+
		keyStyle.Render(itoa(o.step+1))+
		dimStyle.Render("/"+itoa(o.totalSteps)))

	if o.step < o.totalSteps-1 {
		parts = append(parts, keyStyle.Render("→")+dimStyle.Render(" next"))
	} else {
		parts = append(parts, keyStyle.Render("s")+dimStyle.Render(" samples"))
		parts = append(parts, keyStyle.Render("Enter")+dimStyle.Render(" start"))
	}

	parts = append(parts, keyStyle.Render("q")+dimStyle.Render(" skip"))

	footer := strings.Join(parts, dimStyle.Render("  ·  "))

	// Center the footer
	footerWidth := lipgloss.Width(footer)
	pad := (availWidth - footerWidth) / 2
	if pad < 0 {
		pad = 0
	}

	return strings.Repeat(" ", pad) + footer
}

// ── Styling helpers ─────────────────────────────────────────────────────────

// key renders a keybinding in highlighted style.
func onbKey(k string) string {
	return lipgloss.NewStyle().
		Foreground(peach).
		Bold(true).
		Render(k)
}

// onbHighlight renders text in an accent color.
func onbHighlight(s string) string {
	return lipgloss.NewStyle().
		Foreground(blue).
		Bold(true).
		Render(s)
}

// onbDim renders text in a subdued color.
func onbDim(s string) string {
	return lipgloss.NewStyle().
		Foreground(overlay0).
		Render(s)
}

// onbText renders text in the primary text color.
func onbText(s string) string {
	return lipgloss.NewStyle().
		Foreground(text).
		Render(s)
}

// onbSection renders a section heading within a step.
func onbSection(s string) string {
	return lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		Render(s)
}

// onbBinding renders a key-description pair.
func onbBinding(key, desc string) string {
	k := lipgloss.NewStyle().
		Foreground(peach).
		Bold(true).
		Width(20).
		Render("  " + key)
	d := lipgloss.NewStyle().
		Foreground(text).
		Render(desc)
	return k + d
}

// ── Step content ────────────────────────────────────────────────────────────

var onboardingLogo = []string{
	"  ██████╗ ██████╗  █████╗ ███╗   ██╗██╗████████╗",
	" ██╔════╝ ██╔══██╗██╔══██╗████╗  ██║██║╚══██╔══╝",
	" ██║  ███╗██████╔╝███████║██╔██╗ ██║██║   ██║   ",
	" ██║   ██║██╔══██╗██╔══██║██║╚██╗██║██║   ██║   ",
	" ╚██████╔╝██║  ██║██║  ██║██║ ╚████║██║   ██║   ",
	"  ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝╚═╝   ╚═╝",
}

func (o Onboarding) stepContent(w int) (string, string) {
	switch o.step {
	case 0:
		return o.stepWelcome(w)
	case 1:
		return o.stepNavigation(w)
	case 2:
		return o.stepCreatingNotes(w)
	case 3:
		return o.stepEditing(w)
	case 4:
		return o.stepVimMode(w)
	case 5:
		return o.stepTaskManager(w)
	case 6:
		return o.stepAIFeatures(w)
	case 7:
		return o.stepCommandPalette(w)
	case 8:
		return o.stepCustomization(w)
	case 9:
		return o.stepGetStarted(w)
	default:
		return "Granit", ""
	}
}

// ── Step 1: Welcome ─────────────────────────────────────────────────────────

func (o Onboarding) stepWelcome(w int) (string, string) {
	var b strings.Builder

	// ASCII logo
	logoStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	for _, line := range onboardingLogo {
		b.WriteString(logoStyle.Render(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString(onbText("Welcome to "))
	b.WriteString(onbHighlight("Granit"))
	b.WriteString(onbText(" — a terminal-native knowledge manager"))
	b.WriteString("\n")
	b.WriteString(onbText("built for speed, privacy, and deep work."))
	b.WriteString("\n\n")

	b.WriteString(onbDim("Your notes are plain Markdown files stored locally."))
	b.WriteString("\n")
	b.WriteString(onbDim("Fully compatible with Obsidian vaults, no lock-in."))
	b.WriteString("\n\n")

	b.WriteString(onbSection("What you'll learn:"))
	b.WriteString("\n")
	b.WriteString(onbDim("  1. Navigation and panels"))
	b.WriteString("\n")
	b.WriteString(onbDim("  2. Creating and linking notes"))
	b.WriteString("\n")
	b.WriteString(onbDim("  3. Editing, vim mode, and multi-cursor"))
	b.WriteString("\n")
	b.WriteString(onbDim("  4. Tasks, AI, and the command palette"))
	b.WriteString("\n")
	b.WriteString(onbDim("  5. Themes, layouts, and customization"))
	b.WriteString("\n\n")

	b.WriteString(onbDim("Press "))
	b.WriteString(onbKey("Enter"))
	b.WriteString(onbDim(" or "))
	b.WriteString(onbKey("->"))
	b.WriteString(onbDim(" to begin the tour."))

	return "Welcome to Granit!", b.String()
}

// ── Step 2: Navigation ──────────────────────────────────────────────────────

func (o Onboarding) stepNavigation(w int) (string, string) {
	var b strings.Builder

	b.WriteString(onbText("Granit has three main panels: "))
	b.WriteString(onbHighlight("Sidebar"))
	b.WriteString(onbText(", "))
	b.WriteString(onbHighlight("Editor"))
	b.WriteString(onbText(", and "))
	b.WriteString(onbHighlight("Backlinks"))
	b.WriteString(onbText("."))
	b.WriteString("\n\n")

	b.WriteString(onbSection("Panel Focus"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Tab / Shift+Tab", "Cycle between panels"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Alt+1", "Jump to sidebar"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Alt+2", "Jump to editor"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Alt+3", "Jump to backlinks"))
	b.WriteString("\n\n")

	b.WriteString(onbSection("Quick File Access"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+P", "Fuzzy search all files"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+J", "Quick switch recent files"))
	b.WriteString("\n")
	b.WriteString(onbBinding("j / k / arrows", "Navigate within panels"))
	b.WriteString("\n\n")

	b.WriteString(onbDim("The sidebar supports fuzzy search — just start typing"))
	b.WriteString("\n")
	b.WriteString(onbDim("to filter files. Press "))
	b.WriteString(onbKey("Esc"))
	b.WriteString(onbDim(" to clear the filter."))

	return "Navigation", b.String()
}

// ── Step 3: Creating Notes ──────────────────────────────────────────────────

func (o Onboarding) stepCreatingNotes(w int) (string, string) {
	var b strings.Builder

	b.WriteString(onbSection("New Notes"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+N", "Create from 10+ templates"))
	b.WriteString("\n")
	b.WriteString(onbDim("  Templates include: daily note, meeting,"))
	b.WriteString("\n")
	b.WriteString(onbDim("  project, zettelkasten, blank, and more."))
	b.WriteString("\n\n")

	b.WriteString(onbSection("Wikilinks"))
	b.WriteString("\n")
	b.WriteString(onbText("Type "))
	b.WriteString(onbKey("[["))
	b.WriteString(onbText(" to link to another note. An autocomplete"))
	b.WriteString("\n")
	b.WriteString(onbText("popup will show matching notes in your vault."))
	b.WriteString("\n\n")

	wikiStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	exampleStyle := lipgloss.NewStyle().
		Foreground(green).
		Background(surface0).
		Padding(0, 1)
	b.WriteString(onbDim("  Example: "))
	b.WriteString(exampleStyle.Render(wikiStyle.Render("[[My Note]]")))
	b.WriteString("\n")
	b.WriteString(onbDim("  With alias: "))
	b.WriteString(exampleStyle.Render(wikiStyle.Render("[[My Note|display text]]")))
	b.WriteString("\n\n")

	b.WriteString(onbSection("Backlinks"))
	b.WriteString("\n")
	b.WriteString(onbText("The backlinks panel ("))
	b.WriteString(onbKey("Alt+3"))
	b.WriteString(onbText(") shows which notes link"))
	b.WriteString("\n")
	b.WriteString(onbText("to the current one — great for discovering"))
	b.WriteString("\n")
	b.WriteString(onbText("connections in your knowledge graph."))

	return "Creating Notes", b.String()
}

// ── Step 4: Editing ─────────────────────────────────────────────────────────

func (o Onboarding) stepEditing(w int) (string, string) {
	var b strings.Builder

	b.WriteString(onbSection("View / Edit Mode"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+E", "Toggle view/edit mode"))
	b.WriteString("\n")
	b.WriteString(onbDim("  View mode renders Markdown beautifully."))
	b.WriteString("\n")
	b.WriteString(onbDim("  Edit mode gives you a full-featured editor."))
	b.WriteString("\n\n")

	b.WriteString(onbSection("Undo / Redo"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+U", "Undo last change"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+Y", "Redo"))
	b.WriteString("\n")
	b.WriteString(onbDim("  (Not Ctrl+Z — that opens Focus Mode!)"))
	b.WriteString("\n\n")

	b.WriteString(onbSection("Multi-Cursor"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+D", "Select word, add next match"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+Shift+Up", "Add cursor above"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+Shift+Down", "Add cursor below"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Esc", "Clear extra cursors"))
	b.WriteString("\n\n")

	b.WriteString(onbSection("Other Essentials"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+S", "Save note"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+F", "Find in file"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+H", "Find and replace"))

	return "Editing", b.String()
}

// ── Step 5: Vim Mode ────────────────────────────────────────────────────────

func (o Onboarding) stepVimMode(w int) (string, string) {
	var b strings.Builder

	b.WriteString(onbText("Granit includes optional "))
	b.WriteString(onbHighlight("Vim keybindings"))
	b.WriteString(onbText(" for"))
	b.WriteString("\n")
	b.WriteString(onbText("power users who prefer modal editing."))
	b.WriteString("\n\n")

	b.WriteString(onbSection("How to Enable"))
	b.WriteString("\n")
	b.WriteString(onbText("  1. Open settings with "))
	b.WriteString(onbKey("Ctrl+,"))
	b.WriteString("\n")
	b.WriteString(onbText("  2. Find "))
	b.WriteString(onbHighlight("\"Vim Mode\""))
	b.WriteString(onbText(" and toggle it on"))
	b.WriteString("\n")
	b.WriteString(onbText("  3. Or use the command palette: "))
	b.WriteString(onbKey("Ctrl+X"))
	b.WriteString("\n")
	b.WriteString(onbText("     and search "))
	b.WriteString(onbHighlight("\"Toggle Vim Mode\""))
	b.WriteString("\n\n")

	b.WriteString(onbSection("Supported Motions"))
	b.WriteString("\n")
	b.WriteString(onbBinding("h/j/k/l", "Movement in normal mode"))
	b.WriteString("\n")
	b.WriteString(onbBinding("i / a / o", "Enter insert mode"))
	b.WriteString("\n")
	b.WriteString(onbBinding("w / b / e", "Word navigation"))
	b.WriteString("\n")
	b.WriteString(onbBinding("dd / yy / p", "Delete/yank/paste lines"))
	b.WriteString("\n")
	b.WriteString(onbBinding("/ + pattern", "Search within file"))
	b.WriteString("\n\n")

	b.WriteString(onbDim("Vim mode works alongside all Granit overlays"))
	b.WriteString("\n")
	b.WriteString(onbDim("and shortcuts. The mode indicator shows in the"))
	b.WriteString("\n")
	b.WriteString(onbDim("status bar: NORMAL, INSERT, or VISUAL."))

	return "Vim Mode", b.String()
}

// ── Step 6: Task Manager ────────────────────────────────────────────────────

func (o Onboarding) stepTaskManager(w int) (string, string) {
	var b strings.Builder

	b.WriteString(onbText("Granit has a built-in task management system"))
	b.WriteString("\n")
	b.WriteString(onbText("that parses tasks from your Markdown notes."))
	b.WriteString("\n\n")

	b.WriteString(onbSection("Access Tasks"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+K", "Open task manager"))
	b.WriteString("\n\n")

	b.WriteString(onbSection("Task Syntax"))
	b.WriteString("\n")
	exStyle := lipgloss.NewStyle().
		Foreground(green).
		Background(surface0).
		Padding(0, 1)
	b.WriteString(onbDim("  Standard checkboxes: "))
	b.WriteString(exStyle.Render("- [ ] Buy groceries"))
	b.WriteString("\n")
	b.WriteString(onbDim("  With priority:       "))
	b.WriteString(exStyle.Render("- [ ] !!1 Urgent task"))
	b.WriteString("\n")
	b.WriteString(onbDim("  With due date:       "))
	b.WriteString(exStyle.Render("- [ ] @due(2026-03-10)"))
	b.WriteString("\n\n")

	b.WriteString(onbSection("Kanban Board"))
	b.WriteString("\n")
	b.WriteString(onbText("Open the command palette ("))
	b.WriteString(onbKey("Ctrl+X"))
	b.WriteString(onbText(") and search"))
	b.WriteString("\n")
	b.WriteString(onbHighlight("\"Kanban\""))
	b.WriteString(onbText(" for a visual board view with columns"))
	b.WriteString("\n")
	b.WriteString(onbText("for To Do, In Progress, and Done."))
	b.WriteString("\n\n")

	b.WriteString(onbDim("Tasks with due dates today are shown in the status bar."))

	return "Task Manager", b.String()
}

// ── Step 7: AI Features ─────────────────────────────────────────────────────

func (o Onboarding) stepAIFeatures(w int) (string, string) {
	var b strings.Builder

	b.WriteString(onbText("Granit includes 9 AI-powered bots that analyze"))
	b.WriteString("\n")
	b.WriteString(onbText("your notes and help you write. All AI is "))
	b.WriteString(onbHighlight("optional"))
	b.WriteString(onbText("."))
	b.WriteString("\n\n")

	b.WriteString(onbSection("AI Bots"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+R", "Open bot panel"))
	b.WriteString("\n")
	b.WriteString(onbDim("  Auto-Tagger, Link Suggester, Summarizer,"))
	b.WriteString("\n")
	b.WriteString(onbDim("  Question Bot, Writing Assistant, Title"))
	b.WriteString("\n")
	b.WriteString(onbDim("  Suggester, Action Items, MOC Generator,"))
	b.WriteString("\n")
	b.WriteString(onbDim("  and Daily Digest."))
	b.WriteString("\n\n")

	b.WriteString(onbSection("AI Chat"))
	b.WriteString("\n")
	b.WriteString(onbText("Use the command palette ("))
	b.WriteString(onbKey("Ctrl+X"))
	b.WriteString(onbText(") to open"))
	b.WriteString("\n")
	b.WriteString(onbHighlight("\"AI Chat\""))
	b.WriteString(onbText(" — ask questions about your vault."))
	b.WriteString("\n\n")

	b.WriteString(onbSection("Setup"))
	b.WriteString("\n")
	b.WriteString(onbText("Three providers: "))
	b.WriteString(onbHighlight("local"))
	b.WriteString(onbText(" (no setup), "))
	b.WriteString(onbHighlight("Ollama"))
	b.WriteString(onbText(","))
	b.WriteString("\n")
	b.WriteString(onbText("or "))
	b.WriteString(onbHighlight("OpenAI"))
	b.WriteString(onbText(". Configure in Settings ("))
	b.WriteString(onbKey("Ctrl+,"))
	b.WriteString(onbText(")."))
	b.WriteString("\n")
	b.WriteString(onbDim("  The settings panel includes a one-click"))
	b.WriteString("\n")
	b.WriteString(onbDim("  Ollama setup wizard to install and pull a model."))

	return "AI Features", b.String()
}

// ── Step 8: Command Palette ─────────────────────────────────────────────────

func (o Onboarding) stepCommandPalette(w int) (string, string) {
	var b strings.Builder

	b.WriteString(onbText("The command palette is the fastest way to access"))
	b.WriteString("\n")
	b.WriteString(onbText("every feature in Granit."))
	b.WriteString("\n\n")

	b.WriteString(onbBinding("Ctrl+X", "Open the command palette"))
	b.WriteString("\n\n")

	b.WriteString(onbText("Start typing to fuzzy-search 70+ commands:"))
	b.WriteString("\n\n")

	b.WriteString(onbSection("Popular Commands"))
	b.WriteString("\n")

	cmdStyle := lipgloss.NewStyle().
		Foreground(text).
		Background(surface0).
		Padding(0, 1)
	dimDescStyle := lipgloss.NewStyle().Foreground(overlay0)

	commands := []struct{ name, desc string }{
		{"Git: Status & Commit", "Version control from within Granit"},
		{"Export Current Note", "HTML, text, or PDF export"},
		{"Vault Statistics", "Charts and insights about your vault"},
		{"Canvas", "Visual 2D whiteboard for notes"},
		{"Flashcards", "Spaced repetition from your notes"},
		{"Publish Site", "Export vault as a static HTML site"},
		{"Similar Notes", "Find related notes via TF-IDF"},
	}

	for _, c := range commands {
		b.WriteString("  ")
		b.WriteString(cmdStyle.Render(c.name))
		b.WriteString(" ")
		b.WriteString(dimDescStyle.Render(c.desc))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(onbDim("Tip: You can also reopen this tutorial from the"))
	b.WriteString("\n")
	b.WriteString(onbDim("command palette by searching "))
	b.WriteString(onbHighlight("\"Show Tutorial\""))
	b.WriteString(onbDim("."))

	return "Command Palette", b.String()
}

// ── Step 9: Customization ───────────────────────────────────────────────────

func (o Onboarding) stepCustomization(w int) (string, string) {
	var b strings.Builder

	b.WriteString(onbSection("Settings"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+,", "Open settings panel"))
	b.WriteString("\n")
	b.WriteString(onbDim("  30+ configurable options for editor,"))
	b.WriteString("\n")
	b.WriteString(onbDim("  appearance, behavior, and AI."))
	b.WriteString("\n\n")

	b.WriteString(onbSection("Themes"))
	b.WriteString("\n")
	b.WriteString(onbText("35 built-in themes including:"))
	b.WriteString("\n")

	themeStyle := lipgloss.NewStyle().Foreground(peach)
	themes := []string{
		"Catppuccin Mocha", "Tokyo Night", "Gruvbox",
		"Nord", "Dracula", "Rose Pine",
		"Synthwave 84", "Solarized", "One Dark",
	}
	b.WriteString(onbDim("  "))
	for i, t := range themes {
		b.WriteString(themeStyle.Render(t))
		if i < len(themes)-1 {
			b.WriteString(onbDim(", "))
		}
		if i == 2 || i == 5 {
			b.WriteString("\n")
			b.WriteString(onbDim("  "))
		}
	}
	b.WriteString("\n\n")

	b.WriteString(onbSection("Layouts"))
	b.WriteString("\n")
	b.WriteString(onbText("8 panel layouts to suit your workflow:"))
	b.WriteString("\n")

	layoutStyle := lipgloss.NewStyle().Foreground(blue)
	layouts := []string{
		"Default (3-panel)", "Writer (2-panel)",
		"Minimal (editor only)", "Reading",
		"Dashboard", "Zen", "Taskboard", "Research",
	}
	for _, l := range layouts {
		b.WriteString("  ")
		b.WriteString(layoutStyle.Render("  " + l))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(onbSection("Icon Themes"))
	b.WriteString("\n")
	b.WriteString(onbDim("  Unicode (default), Nerd Font, Emoji, or ASCII"))

	return "Customization", b.String()
}

// ── Step 10: Get Started ────────────────────────────────────────────────────

func (o Onboarding) stepGetStarted(w int) (string, string) {
	var b strings.Builder

	readyStyle := lipgloss.NewStyle().
		Foreground(green).
		Bold(true)
	b.WriteString(readyStyle.Render("You're ready to go!"))
	b.WriteString("\n\n")

	b.WriteString(onbText("Here's a quick reference card to keep handy:"))
	b.WriteString("\n\n")

	b.WriteString(onbSection("Essential Shortcuts"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+P", "Fuzzy find files"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+N", "New note from template"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+S", "Save"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+E", "Toggle view/edit mode"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+X", "Command palette"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+K", "Task manager"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+R", "AI bots"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+,", "Settings"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Alt+?", "Full help / shortcuts"))
	b.WriteString("\n")
	b.WriteString(onbBinding("Ctrl+Z", "Focus / zen mode"))
	b.WriteString("\n\n")

	b.WriteString(onbDim("You can always reopen this tutorial from the"))
	b.WriteString("\n")
	b.WriteString(onbDim("command palette ("))
	b.WriteString(onbKey("Ctrl+X"))
	b.WriteString(onbDim(") by searching "))
	b.WriteString(onbHighlight("\"Show Tutorial\""))
	b.WriteString(onbDim("."))
	b.WriteString("\n\n")

	b.WriteString(onbText("Press "))
	b.WriteString(onbKey("s"))
	b.WriteString(onbText(" to create sample notes in your vault."))
	b.WriteString("\n")
	b.WriteString(onbText("Press "))
	b.WriteString(onbKey("Enter"))
	b.WriteString(onbText(" to start using Granit."))

	if o.statusMsg != "" {
		b.WriteString("\n\n")
		statusStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		b.WriteString(statusStyle.Render(o.statusMsg))
	}

	return "Get Started", b.String()
}

// ── Sample vault creation ───────────────────────────────────────────────────

// createSampleVault writes a set of starter notes into vaultRoot, skipping any
// file that already exists so it never overwrites user content.
func createSampleVault(vaultRoot string) error {
	samples := map[string]string{
		"Welcome.md": `# Welcome to Granit

Your personal knowledge manager. Start by creating notes with ` + "`Ctrl+N`" + `.

## Quick Tips
- Link notes with ` + "`[[Note Name]]`" + `
- Add tasks with ` + "`- [ ] Task description`" + `
- Open command palette with ` + "`Ctrl+X`" + `
- Toggle view mode with ` + "`Ctrl+E`" + `
- Search with ` + "`Ctrl+K`" + `

## Next Steps
- [ ] Create your first note
- [ ] Link two notes together
- [ ] Try the daily planner
- [ ] Set up your habits
`,
		"Tasks Example.md": "# Example Tasks\n\n" +
			"- [ ] \U0001F53A High priority task \U0001F4C5 2026-04-01\n" +
			"- [ ] \u23EB Review project goals #project\n" +
			"- [ ] \U0001F53C Read chapter 3 #reading\n" +
			"- [ ] \U0001F53D Organize bookmarks #low\n" +
			"- [x] Completed task example\n",
		"Project Example.md": `# My First Project

A sample project to get started with Granit's project tracking.

## Goals
- [ ] Define project scope
- [ ] Set up milestones
- [ ] Track progress daily

## Notes
Use ` + "`Ctrl+X`" + ` → "Projects" to manage projects with goals and milestones.
`,
	}

	for name, content := range samples {
		p := filepath.Join(vaultRoot, name)
		if _, err := os.Stat(p); err == nil {
			continue // already exists, skip
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}
