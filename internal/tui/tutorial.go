package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/config"
)

// Tutorial provides a multi-step walkthrough overlay for first-time users.
// It shows on first launch when config.TutorialCompleted is false, and can
// be reopened any time from the command palette (CmdShowTutorial).
type Tutorial struct {
	active     bool
	page       int
	width      int
	height     int
	totalPages int
	cfg        *config.Config
	vaultRoot  string
	statusMsg  string
}

// NewTutorial returns a Tutorial in its default (inactive) state.
func NewTutorial(cfg *config.Config) Tutorial {
	return Tutorial{
		totalPages: 7,
		cfg:        cfg,
	}
}

// IsActive reports whether the tutorial overlay is currently visible.
func (t Tutorial) IsActive() bool {
	return t.active
}

// Open activates the tutorial overlay, resetting to the first page.
func (t *Tutorial) Open() {
	t.active = true
	t.page = 0
}

// Close deactivates the tutorial overlay.
func (t *Tutorial) Close() {
	t.active = false
}

// SetSize updates the available terminal dimensions for layout.
func (t *Tutorial) SetSize(w, h int) {
	t.width = w
	t.height = h
}

// tutorialSaveErrMsg is sent when saving the tutorial-completed flag fails.
type tutorialSaveErrMsg struct {
	err error
}

// MarkComplete sets TutorialCompleted in the config and saves it.
// Returns a tea.Cmd that sends tutorialSaveErrMsg on failure, or nil on success.
func (t *Tutorial) MarkComplete() tea.Cmd {
	if t.cfg == nil {
		return nil
	}
	t.cfg.TutorialCompleted = true
	if err := t.cfg.Save(); err != nil {
		return func() tea.Msg {
			return tutorialSaveErrMsg{err: err}
		}
	}
	return nil
}

// Update handles key events for navigating between tutorial pages.
func (t Tutorial) Update(msg tea.Msg) (Tutorial, tea.Cmd) {
	if !t.active {
		return t, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			if t.page > 0 {
				t.page--
			}
		case "right", "l", " ":
			if t.page < t.totalPages-1 {
				t.page++
			}
		case "enter":
			if t.page < t.totalPages-1 {
				t.page++
			} else {
				// Last page — close and mark done
				t.active = false
				return t, t.MarkComplete()
			}
		case "s":
			// Create sample vault notes on last page
			if t.page == t.totalPages-1 && t.vaultRoot != "" {
				if err := createSampleVault(t.vaultRoot); err != nil {
					t.statusMsg = "Error: " + err.Error()
				} else {
					t.statusMsg = "Sample notes created!"
				}
			}
		case "q", "esc":
			t.active = false
			return t, t.MarkComplete()
		}
	}
	return t, nil
}

// View renders the current tutorial page as a centered, bordered overlay.
func (t Tutorial) View() string {
	width := t.width * 2 / 3
	if width < 64 {
		width = 64
	}
	if width > 86 {
		width = 86
	}

	innerWidth := width - 8 // border + padding

	var b strings.Builder

	// Page title and content
	title, content := t.pageContent(innerWidth)

	// ── Header ──────────────────────────────────────────────────────
	pageLabel := lipgloss.NewStyle().
		Foreground(surface2).
		Render(itoa(t.page+1) + "/" + itoa(t.totalPages))

	titleStyle := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true)

	b.WriteString(titleStyle.Render(title))
	titleWidth := lipgloss.Width(title)
	labelWidth := lipgloss.Width(pageLabel)
	gap := innerWidth - titleWidth - labelWidth
	if gap < 1 {
		gap = 1
	}
	b.WriteString(strings.Repeat(" ", gap))
	b.WriteString(pageLabel)
	b.WriteString("\n")

	// Divider
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n\n")

	// Content
	b.WriteString(content)

	// ── Progress dots ───────────────────────────────────────────────
	b.WriteString("\n\n")
	b.WriteString(t.renderProgress(innerWidth))
	b.WriteString("\n")

	// ── Footer ──────────────────────────────────────────────────────
	b.WriteString(t.renderFooter(innerWidth))

	// ── Outer box ───────────────────────────────────────────────────
	box := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 3).
		Width(width).
		Background(mantle)

	rendered := box.Render(b.String())

	return lipgloss.Place(
		t.width,
		t.height,
		lipgloss.Center,
		lipgloss.Center,
		rendered,
	)
}

// ── Progress bar ────────────────────────────────────────────────────────

func (t Tutorial) renderProgress(availWidth int) string {
	barWidth := availWidth - 4
	if barWidth < 10 {
		barWidth = 10
	}

	total := t.totalPages
	if total <= 0 {
		total = 6
	}
	filled := (t.page + 1) * barWidth / total
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled

	filledStyle := lipgloss.NewStyle().Foreground(mauve)
	emptyStyle := lipgloss.NewStyle().Foreground(surface1)

	return "  " +
		filledStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", empty))
}

// ── Footer ──────────────────────────────────────────────────────────────

func (t Tutorial) renderFooter(availWidth int) string {
	keyStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)

	var parts []string

	if t.page > 0 {
		parts = append(parts, keyStyle.Render("←/h")+dimStyle.Render(" prev"))
	}

	parts = append(parts, dimStyle.Render("page ")+
		keyStyle.Render(itoa(t.page+1))+
		dimStyle.Render("/"+itoa(t.totalPages)))

	if t.page < t.totalPages-1 {
		parts = append(parts, keyStyle.Render("→/l")+dimStyle.Render(" next"))
	} else {
		parts = append(parts, keyStyle.Render("Enter")+dimStyle.Render(" start"))
	}

	parts = append(parts, keyStyle.Render("Esc")+dimStyle.Render(" skip"))

	footer := strings.Join(parts, dimStyle.Render("  ·  "))

	footerWidth := lipgloss.Width(footer)
	pad := (availWidth - footerWidth) / 2
	if pad < 0 {
		pad = 0
	}

	return strings.Repeat(" ", pad) + footer
}

// ── Styling helpers ─────────────────────────────────────────────────────

func tutKey(k string) string {
	return lipgloss.NewStyle().
		Foreground(peach).
		Bold(true).
		Render(k)
}

func tutHighlight(s string) string {
	return lipgloss.NewStyle().
		Foreground(blue).
		Bold(true).
		Render(s)
}

func tutDim(s string) string {
	return lipgloss.NewStyle().
		Foreground(overlay0).
		Render(s)
}

func tutText(s string) string {
	return lipgloss.NewStyle().
		Foreground(text).
		Render(s)
}

func tutSection(s string) string {
	return lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		Render(s)
}

func tutBinding(key, desc string) string {
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

// ── Page content ────────────────────────────────────────────────────────

var tutorialLogo = []string{
	"  ██████╗ ██████╗  █████╗ ███╗   ██╗██╗████████╗",
	" ██╔════╝ ██╔══██╗██╔══██╗████╗  ██║██║╚══██╔══╝",
	" ██║  ███╗██████╔╝███████║██╔██╗ ██║██║   ██║   ",
	" ██║   ██║██╔══██╗██╔══██║██║╚██╗██║██║   ██║   ",
	" ╚██████╔╝██║  ██║██║  ██║██║ ╚████║██║   ██║   ",
	"  ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝╚═╝   ╚═╝",
}

func (t Tutorial) pageContent(w int) (string, string) {
	switch t.page {
	case 0:
		return t.pageWelcome(w)
	case 1:
		return t.pageNavigation(w)
	case 2:
		return t.pageWriting(w)
	case 3:
		return t.pagePowerFeatures(w)
	case 4:
		return t.pageGettingStarted(w)
	case 5:
		return t.pageShortcuts(w)
	case 6:
		return t.pageProductivity(w)
	default:
		return "Granit", ""
	}
}

// ── Page 1: Welcome ─────────────────────────────────────────────────────

func (t Tutorial) pageWelcome(w int) (string, string) {
	var b strings.Builder

	logoStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	for _, line := range tutorialLogo {
		b.WriteString(logoStyle.Render(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString(tutText("Welcome to "))
	b.WriteString(tutHighlight("Granit"))
	b.WriteString(tutText(" — a terminal-native knowledge manager"))
	b.WriteString("\n")
	b.WriteString(tutText("built for speed, privacy, and deep work."))
	b.WriteString("\n\n")

	b.WriteString(tutDim("Your notes are plain Markdown files stored locally."))
	b.WriteString("\n")
	b.WriteString(tutDim("Fully compatible with Obsidian vaults — no lock-in."))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Key Principles"))
	b.WriteString("\n")
	b.WriteString(tutDim("  - Local-first: your data never leaves your machine"))
	b.WriteString("\n")
	b.WriteString(tutDim("  - Plain Markdown: open with any editor, anytime"))
	b.WriteString("\n")
	b.WriteString(tutDim("  - Terminal-native: fast, keyboard-driven workflow"))
	b.WriteString("\n\n")

	b.WriteString(tutDim("Press "))
	b.WriteString(tutKey("Enter"))
	b.WriteString(tutDim(" or "))
	b.WriteString(tutKey("->"))
	b.WriteString(tutDim(" to begin the tour."))

	return "Welcome to Granit!", b.String()
}

// ── Page 2: Navigation ──────────────────────────────────────────────────

func (t Tutorial) pageNavigation(w int) (string, string) {
	var b strings.Builder

	b.WriteString(tutText("Granit has three main panels: "))
	b.WriteString(tutHighlight("Sidebar"))
	b.WriteString(tutText(", "))
	b.WriteString(tutHighlight("Editor"))
	b.WriteString(tutText(", and "))
	b.WriteString(tutHighlight("Backlinks"))
	b.WriteString(tutText("."))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Moving Between Panels"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Tab / Shift+Tab", "Cycle between panels"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Alt+1 / Alt+2 / Alt+3", "Jump to sidebar / editor / backlinks"))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Finding Files"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+J", "Quick switch between recent files"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+P", "Fuzzy search all files"))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Sidebar Search"))
	b.WriteString("\n")
	b.WriteString(tutText("The sidebar supports fuzzy search — just start typing"))
	b.WriteString("\n")
	b.WriteString(tutText("to filter files. Press "))
	b.WriteString(tutKey("Esc"))
	b.WriteString(tutText(" to clear the filter."))

	return "Navigation", b.String()
}

// ── Page 3: Writing ─────────────────────────────────────────────────────

func (t Tutorial) pageWriting(w int) (string, string) {
	var b strings.Builder

	b.WriteString(tutSection("Creating Notes"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+N", "Create from 10+ templates"))
	b.WriteString("\n")
	b.WriteString(tutDim("  Templates: daily note, meeting, project,"))
	b.WriteString("\n")
	b.WriteString(tutDim("  zettelkasten, blank, and more."))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Wikilinks"))
	b.WriteString("\n")
	b.WriteString(tutText("Type "))
	b.WriteString(tutKey("[["))
	b.WriteString(tutText(" to link to another note. An autocomplete"))
	b.WriteString("\n")
	b.WriteString(tutText("popup shows matching notes in your vault."))
	b.WriteString("\n\n")

	wikiStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	exampleBg := lipgloss.NewStyle().
		Foreground(green).
		Background(surface0).
		Padding(0, 1)
	b.WriteString(tutDim("  Example: "))
	b.WriteString(exampleBg.Render(wikiStyle.Render("[[My Note]]")))
	b.WriteString("\n")
	b.WriteString(tutDim("  With heading: "))
	b.WriteString(exampleBg.Render(wikiStyle.Render("[[My Note#Section]]")))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Markdown Support"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+E", "Toggle view/edit mode"))
	b.WriteString("\n")
	b.WriteString(tutDim("  View mode renders headings, bold, lists, code"))
	b.WriteString("\n")
	b.WriteString(tutDim("  blocks, callouts, and embedded images."))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Saving"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+S", "Save current note"))
	b.WriteString("\n")
	b.WriteString(tutDim("  Auto-save can be enabled in settings."))

	return "Writing", b.String()
}

// ── Page 4: Power Features ──────────────────────────────────────────────

func (t Tutorial) pagePowerFeatures(w int) (string, string) {
	var b strings.Builder

	b.WriteString(tutSection("Command Palette"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+X", "Open command palette"))
	b.WriteString("\n")
	b.WriteString(tutDim("  Fuzzy-search 70+ commands. This is the fastest"))
	b.WriteString("\n")
	b.WriteString(tutDim("  way to access every feature in Granit."))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Themes"))
	b.WriteString("\n")
	b.WriteString(tutText("28 built-in themes — change in settings ("))
	b.WriteString(tutKey("Ctrl+,"))
	b.WriteString(tutText(")"))
	b.WriteString("\n")
	themeStyle := lipgloss.NewStyle().Foreground(peach)
	b.WriteString(tutDim("  "))
	b.WriteString(themeStyle.Render("Catppuccin"))
	b.WriteString(tutDim(", "))
	b.WriteString(themeStyle.Render("Tokyo Night"))
	b.WriteString(tutDim(", "))
	b.WriteString(themeStyle.Render("Gruvbox"))
	b.WriteString(tutDim(", "))
	b.WriteString(themeStyle.Render("Nord"))
	b.WriteString(tutDim(", "))
	b.WriteString(themeStyle.Render("Dracula"))
	b.WriteString(tutDim(", ..."))
	b.WriteString("\n\n")

	b.WriteString(tutSection("AI Bots"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+R", "Open AI bot panel"))
	b.WriteString("\n")
	b.WriteString(tutDim("  9 bots: auto-tagger, summarizer, link suggester,"))
	b.WriteString("\n")
	b.WriteString(tutDim("  writing assistant, and more. Works with local"))
	b.WriteString("\n")
	b.WriteString(tutDim("  fallback, Ollama, or OpenAI. All optional."))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Graph View"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+G", "Show note connection graph"))
	b.WriteString("\n")
	b.WriteString(tutDim("  Visualize how your notes link together."))

	return "Power Features", b.String()
}

// ── Page 5: Getting Started ─────────────────────────────────────────────

func (t Tutorial) pageGettingStarted(w int) (string, string) {
	var b strings.Builder

	readyStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	b.WriteString(readyStyle.Render("Ready to start building your knowledge base!"))
	b.WriteString("\n\n")

	b.WriteString(tutSection("First Steps"))
	b.WriteString("\n")
	b.WriteString(tutText("  1. Press "))
	b.WriteString(tutKey("Ctrl+N"))
	b.WriteString(tutText(" to create your first note"))
	b.WriteString("\n")
	b.WriteString(tutText("  2. Start writing in Markdown"))
	b.WriteString("\n")
	b.WriteString(tutText("  3. Use "))
	b.WriteString(tutKey("[["))
	b.WriteString(tutText(" to link ideas together"))
	b.WriteString("\n")
	b.WriteString(tutText("  4. Press "))
	b.WriteString(tutKey("Ctrl+S"))
	b.WriteString(tutText(" to save"))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Useful Tools"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+K", "Task manager across vault"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+L", "Calendar (month/week/agenda)"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+W", "Visual canvas / whiteboard"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+Z", "Focus / zen mode"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+O", "Note heading outline"))
	b.WriteString("\n\n")

	b.WriteString(tutDim("Tip: you can reopen this tutorial any time from"))
	b.WriteString("\n")
	b.WriteString(tutDim("the command palette ("))
	b.WriteString(tutKey("Ctrl+X"))
	b.WriteString(tutDim(") — search "))
	b.WriteString(tutHighlight("\"Show Tutorial\""))
	b.WriteString(tutDim("."))

	return "Getting Started", b.String()
}

// ── Page 6: Keyboard Shortcuts ──────────────────────────────────────────

func (t Tutorial) pageShortcuts(w int) (string, string) {
	var b strings.Builder

	b.WriteString(tutText("Quick reference of the top shortcuts:"))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Essential"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+N", "New note from template"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+S", "Save"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+E", "Toggle view/edit mode"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+X", "Command palette"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+P", "Fuzzy find files"))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Navigation"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Tab", "Cycle panels"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+J", "Quick switch files"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+F", "Find in file"))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Tools"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+R", "AI bots"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+G", "Graph view"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+K", "Task manager"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+,", "Settings"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Alt+?", "Full help / all shortcuts"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+Q", "Quit"))
	b.WriteString("\n\n")

	b.WriteString(tutText("Press "))
	b.WriteString(tutKey("Enter"))
	b.WriteString(tutText(" to close and start using Granit."))

	return "Keyboard Shortcuts", b.String()
}

// ── Page 7: Productivity ───────────────────────────────────────────────

func (t Tutorial) pageProductivity(_ int) (string, string) {
	var b strings.Builder

	b.WriteString(tutText("Granit includes a full productivity suite for"))
	b.WriteString("\n")
	b.WriteString(tutText("managing tasks, goals, habits, and your daily plan."))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Task Manager") + tutDim("  (Ctrl+K)"))
	b.WriteString("\n")
	b.WriteString(tutText("  6 views: Plan (with mini timeline), Upcoming, All, Done, Calendar, Kanban"))
	b.WriteString("\n")
	b.WriteString(tutText("  Subtasks, dependencies, time estimation, snooze, templates, quick-edit"))
	b.WriteString("\n")
	b.WriteString(tutText("  Quick-add: ") + tutKey("Ctrl+T") + tutText(" with @date !priority #tag ~estimate"))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Goal Manager") + tutDim("  (command palette > Goals)"))
	b.WriteString("\n")
	b.WriteString(tutText("  Track long-term goals with milestones and progress"))
	b.WriteString("\n")
	b.WriteString(tutText("  Recurring reviews (weekly/monthly/quarterly)"))
	b.WriteString("\n")
	b.WriteString(tutText("  Link milestones to tasks, assign colors, set due dates"))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Daily Planner") + tutDim("  (command palette > Daily Planner)"))
	b.WriteString("\n")
	b.WriteString(tutText("  Time-blocked schedule from 6am to 10pm"))
	b.WriteString("\n")
	b.WriteString(tutText("  Copy plan to clipboard (") + tutKey("c") + tutText(") or export as markdown (") + tutKey("S") + tutText(")"))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Search Everything") + tutDim("  (Ctrl+/)"))
	b.WriteString("\n")
	b.WriteString(tutText("  Fuzzy search across notes, tasks, goals, and habits"))
	b.WriteString("\n\n")

	b.WriteString(tutSection("Also available:"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+L", "Calendar with 6 views (½hr grid, event editing)"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+X", "Command palette (80+ commands)"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+G", "Note graph"))
	b.WriteString("\n")
	b.WriteString(tutBinding("Ctrl+R", "AI bots"))
	b.WriteString("\n\n")

	b.WriteString(tutText("Press "))
	b.WriteString(tutKey("Enter"))
	b.WriteString(tutText(" to close and start using Granit."))

	return "Productivity Suite", b.String()
}
