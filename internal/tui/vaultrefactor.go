package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type vaultRefactorResultMsg struct {
	plan string
	err  error
}

type vaultRefactorTickMsg struct{}

func vaultRefactorTickCmd() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return vaultRefactorTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// RefactorAction represents a single proposed change
// ---------------------------------------------------------------------------

type RefactorAction struct {
	Kind    string // "move", "rename", "tag", "link"
	OldPath string
	NewPath string
	Tags    []string
	Links   []string
	Reason  string
}

// ---------------------------------------------------------------------------
// VaultRefactor overlay
// ---------------------------------------------------------------------------

const (
	refactorStateConfirm  = 0
	refactorStateLoading  = 1
	refactorStateResults  = 2
)

type VaultRefactor struct {
	OverlayBase

	state       int
	plan        string   // raw AI output
	planLines   []string // split for display
	scroll       int
	loadingTick  int
	loadingStart time.Time

	// vault data
	notes    map[string]string
	tags     map[string][]string
	allPaths []string

	// AI config
	ai AIConfig

	// result to be consumed by app
	resultPlan  string
	resultReady bool
}

func NewVaultRefactor() VaultRefactor {
	return VaultRefactor{}
}

func (vr *VaultRefactor) Open() {
	vr.Activate()
	vr.state = refactorStateConfirm
	vr.plan = ""
	vr.planLines = nil
	vr.scroll = 0
	vr.loadingTick = 0
	vr.loadingStart = time.Time{}
	vr.resultReady = false
	vr.resultPlan = ""
}

func (vr *VaultRefactor) SetVaultData(notes map[string]string, tags map[string][]string, allPaths []string) {
	vr.notes = notes
	vr.tags = tags
	vr.allPaths = allPaths
}

func (vr *VaultRefactor) GetResult() (string, bool) {
	if !vr.resultReady {
		return "", false
	}
	r := vr.resultPlan
	vr.resultReady = false
	vr.resultPlan = ""
	return r, true
}

// ---------------------------------------------------------------------------
// Prompt
// ---------------------------------------------------------------------------

func (vr *VaultRefactor) buildPrompt() string {
	var sb strings.Builder

	sb.WriteString("You are a knowledge management expert. Analyze this vault of notes and propose a complete reorganization plan.\n\n")
	sb.WriteString("CURRENT VAULT STRUCTURE:\n")

	// Group by directory
	dirs := make(map[string][]string)
	for _, p := range vr.allPaths {
		dir := filepath.Dir(p)
		dirs[dir] = append(dirs[dir], filepath.Base(p))
	}
	sortedDirs := make([]string, 0, len(dirs))
	for d := range dirs {
		sortedDirs = append(sortedDirs, d)
	}
	sort.Strings(sortedDirs)

	for _, d := range sortedDirs {
		if d == "." {
			sb.WriteString("\n/ (root):\n")
		} else {
			sb.WriteString(fmt.Sprintf("\n%s/:\n", d))
		}
		for _, f := range dirs[d] {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}

	// Note summaries (first 150 chars each, up to 40 notes)
	sb.WriteString("\nNOTE SUMMARIES:\n")
	count := 0
	for _, p := range vr.allPaths {
		if count >= 40 {
			break
		}
		content := vr.notes[p]
		preview := strings.ReplaceAll(content, "\n", " ")
		if len(preview) > 150 {
			preview = preview[:150]
		}
		sb.WriteString(fmt.Sprintf("- %s: %s\n", strings.TrimSuffix(p, ".md"), preview))
		count++
	}

	// Existing tags
	if len(vr.tags) > 0 {
		sb.WriteString("\nEXISTING TAGS:\n")
		tagNames := make([]string, 0, len(vr.tags))
		for t := range vr.tags {
			tagNames = append(tagNames, t)
		}
		sort.Strings(tagNames)
		for _, t := range tagNames {
			sb.WriteString(fmt.Sprintf("- #%s (%d notes)\n", t, len(vr.tags[t])))
		}
	}

	sb.WriteString(`
TASK: Propose a vault reorganization. For each note, output ONE line in this exact format:

MOVE: old-path.md -> folder/new-name.md | TAGS: tag1, tag2 | LINKS: [[Note1]], [[Note2]] | REASON: brief explanation

Rules:
1. Group related notes into logical folders (e.g., Projects/, Daily/, Reference/, Ideas/)
2. Improve file names to be clear and descriptive (use kebab-case)
3. Suggest 2-4 relevant tags per note
4. Suggest wikilinks between related notes
5. Keep daily notes in a Daily/ folder with their date names
6. Don't over-nest - max 2 levels of folders
7. Output ONLY the MOVE lines, one per note. No other text.
`)

	return sb.String()
}

// ---------------------------------------------------------------------------
// AI calls
// ---------------------------------------------------------------------------

func (vr *VaultRefactor) startRefactor() tea.Cmd {
	systemPrompt := "You are a knowledge management expert that helps organize note vaults. Be precise and follow the output format exactly."
	userPrompt := vr.buildPrompt()
	ai := vr.ai

	return func() tea.Msg {
		resp, err := ai.Chat(systemPrompt, userPrompt)
		return vaultRefactorResultMsg{plan: resp, err: err}
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (vr VaultRefactor) Update(msg tea.Msg) (VaultRefactor, tea.Cmd) {
	if !vr.active {
		return vr, nil
	}

	switch msg := msg.(type) {
	case vaultRefactorResultMsg:
		if msg.err != nil {
			vr.state = refactorStateResults
			vr.planLines = []string{
				lipgloss.NewStyle().Foreground(red).Render("  Error: " + msg.err.Error()),
				"",
				DimStyle.Render("  Make sure your AI provider is configured in Settings (Ctrl+,)"),
			}
			return vr, nil
		}
		vr.state = refactorStateResults
		vr.plan = msg.plan
		vr.planLines = vr.formatPlan(msg.plan)
		vr.scroll = 0
		return vr, nil

	case vaultRefactorTickMsg:
		if vr.state == refactorStateLoading {
			vr.loadingTick++
			return vr, vaultRefactorTickCmd()
		}
		return vr, nil

	case tea.KeyMsg:
		switch vr.state {
		case refactorStateConfirm:
			switch msg.String() {
			case "esc":
				vr.active = false
			case "enter", "y":
				vr.state = refactorStateLoading
				vr.loadingTick = 0
				vr.loadingStart = time.Now()
				return vr, tea.Batch(vr.startRefactor(), vaultRefactorTickCmd())
			case "n", "q":
				vr.active = false
			}

		case refactorStateLoading:
			if msg.String() == "esc" {
				vr.active = false
			}

		case refactorStateResults:
			switch msg.String() {
			case "esc", "q":
				vr.active = false
			case "up", "k":
				if vr.scroll > 0 {
					vr.scroll--
				}
			case "down", "j":
				vr.scroll++
			case "enter":
				// Accept the plan — pass it to the app for execution
				vr.resultPlan = vr.plan
				vr.resultReady = true
				vr.active = false
			}
		}
	}
	return vr, nil
}

// ---------------------------------------------------------------------------
// Format plan for display
// ---------------------------------------------------------------------------

func (vr *VaultRefactor) formatPlan(raw string) []string {
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  Proposed Vault Reorganization"))
	lines = append(lines, "")

	moveIcon := lipgloss.NewStyle().Foreground(peach).Render("  → ")
	tagIcon := lipgloss.NewStyle().Foreground(blue).Render(" " + IconTagChar + " ")
	linkIcon := lipgloss.NewStyle().Foreground(green).Render(" " + IconLinkChar + " ")

	for _, rawLine := range strings.Split(raw, "\n") {
		rawLine = strings.TrimSpace(rawLine)
		if rawLine == "" {
			continue
		}

		// Parse MOVE: old -> new | TAGS: ... | LINKS: ... | REASON: ...
		if strings.HasPrefix(rawLine, "MOVE:") {
			parts := strings.SplitN(rawLine[5:], "|", 4)
			if len(parts) >= 1 {
				movePart := strings.TrimSpace(parts[0])
				arrow := strings.SplitN(movePart, "->", 2)
				if len(arrow) == 2 {
					oldName := strings.TrimSpace(arrow[0])
					newName := strings.TrimSpace(arrow[1])
					lines = append(lines, moveIcon+
						lipgloss.NewStyle().Foreground(overlay0).Render(oldName)+
						lipgloss.NewStyle().Foreground(text).Render("  →  ")+
						lipgloss.NewStyle().Foreground(peach).Bold(true).Render(newName))
				} else {
					lines = append(lines, moveIcon+lipgloss.NewStyle().Foreground(text).Render(movePart))
				}
			}

			for _, part := range parts[1:] {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "TAGS:") {
					lines = append(lines, tagIcon+lipgloss.NewStyle().Foreground(blue).Render(strings.TrimSpace(part[5:])))
				} else if strings.HasPrefix(part, "LINKS:") {
					lines = append(lines, linkIcon+lipgloss.NewStyle().Foreground(green).Render(strings.TrimSpace(part[6:])))
				} else if strings.HasPrefix(part, "REASON:") {
					lines = append(lines, DimStyle.Render("    "+strings.TrimSpace(part[7:])))
				}
			}
			lines = append(lines, "")
		} else {
			// Any non-MOVE line, show as-is dimmed
			lines = append(lines, DimStyle.Render("  "+rawLine))
		}
	}

	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("  Press Enter to apply changes, Esc to cancel"))

	return lines
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (vr VaultRefactor) View() string {
	panelWidth := vr.width * 3 / 4
	if panelWidth < 60 {
		panelWidth = 60
	}
	if panelWidth > 120 {
		panelWidth = 120
	}
	innerWidth := panelWidth - 6

	var b strings.Builder

	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  " + IconBotChar + " AI Vault Refactor")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat(ThemeSeparator, innerWidth)))
	b.WriteString("\n")

	switch vr.state {
	case refactorStateConfirm:
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(text).Render("  AI will analyze your entire vault and propose:"))
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(peach).Render("    → Folder structure") + DimStyle.Render(" — group related notes"))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(peach).Render("    → Better file names") + DimStyle.Render(" — clear, descriptive titles"))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(peach).Render("    → Tags") + DimStyle.Render(" — consistent categorization"))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(peach).Render("    → Wikilinks") + DimStyle.Render(" — connect related notes"))
		b.WriteString("\n\n")
		noteCount := len(vr.allPaths)
		b.WriteString(DimStyle.Render(fmt.Sprintf("  %d notes will be analyzed.", noteCount)))
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(green).Bold(true).Render("  Press Enter to start, Esc to cancel"))

	case refactorStateLoading:
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spinner := lipgloss.NewStyle().Foreground(mauve).Render(frames[vr.loadingTick%len(frames)])
		b.WriteString("\n")
		elapsed := time.Since(vr.loadingStart).Truncate(time.Second)
		b.WriteString("  " + spinner + lipgloss.NewStyle().Foreground(text).Render(" Analyzing vault structure...") + DimStyle.Render(fmt.Sprintf(" %s", elapsed)))
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  This may take a moment for large vaults."))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Press Esc to cancel."))

	case refactorStateResults:
		visH := vr.height - 10
		if visH < 5 {
			visH = 5
		}
		maxScroll := len(vr.planLines) - visH
		if maxScroll < 0 {
			maxScroll = 0
		}
		if vr.scroll > maxScroll {
			vr.scroll = maxScroll
		}
		end := vr.scroll + visH
		if end > len(vr.planLines) {
			end = len(vr.planLines)
		}
		for i := vr.scroll; i < end; i++ {
			b.WriteString(vr.planLines[i])
			b.WriteString("\n")
		}
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(panelWidth).
		Background(mantle)

	return border.Render(b.String())
}
