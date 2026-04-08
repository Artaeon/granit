package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type gitState int

const (
	gitStateStatus gitState = iota
	gitStateLog
	gitStateDiff
)

// gitCmdResultMsg carries the result of an async git command.
type gitCmdResultMsg struct {
	action string // "status", "log", "diff", "commit", "push", "pull"
	output string
	err    error
}

type GitOverlay struct {
	active     bool
	width      int
	height     int
	state      gitState
	cursor     int
	scroll     int
	statusLines []string
	logLines    []string
	diffLines   []string
	commitMsg   string
	commitMode  bool
	errorMsg    string
	vaultRoot   string

	// User identity configuration mode
	userMode      bool // editing user.name / user.email
	userField     int  // 0=name, 1=email
	userName      string
	userEmail     string
}

func NewGitOverlay() GitOverlay {
	return GitOverlay{}
}

func (g *GitOverlay) IsActive() bool {
	return g.active
}

func (g *GitOverlay) Open(vaultRoot string) tea.Cmd {
	g.active = true
	g.vaultRoot = vaultRoot
	g.state = gitStateStatus
	g.cursor = 0
	g.scroll = 0
	g.commitMode = false
	g.commitMsg = ""
	g.userMode = false
	g.errorMsg = ""
	// Load current git user identity from repo config
	if name, err := runGitCmd(vaultRoot, "config", "user.name"); err == nil {
		g.userName = strings.TrimSpace(name)
	}
	if email, err := runGitCmd(vaultRoot, "config", "user.email"); err == nil {
		g.userEmail = strings.TrimSpace(email)
	}
	return g.RefreshAll()
}

func (g *GitOverlay) Close() {
	g.active = false
	g.commitMode = false
	g.commitMsg = ""
}

func (g *GitOverlay) SetSize(width, height int) {
	g.width = width
	g.height = height
}

// refreshStatus runs git status --porcelain synchronously and populates statusLines.
func (g *GitOverlay) refreshStatus() tea.Cmd {
	dir := g.vaultRoot
	return func() tea.Msg {
		out, err := runGitCmd(dir, "status", "--porcelain")
		return gitCmdResultMsg{action: "status", output: out, err: err}
	}
}

// refreshLog runs git log --oneline -20 synchronously.
func (g *GitOverlay) refreshLog() tea.Cmd {
	dir := g.vaultRoot
	return func() tea.Msg {
		out, err := runGitCmd(dir, "log", "--oneline", "-20")
		return gitCmdResultMsg{action: "log", output: out, err: err}
	}
}

// refreshDiff runs git diff synchronously.
func (g *GitOverlay) refreshDiff() tea.Cmd {
	dir := g.vaultRoot
	return func() tea.Msg {
		out, err := runGitCmd(dir, "diff")
		return gitCmdResultMsg{action: "diff", output: out, err: err}
	}
}

// RefreshAll fetches status, log, and diff concurrently.
func (g *GitOverlay) RefreshAll() tea.Cmd {
	return tea.Batch(g.refreshStatus(), g.refreshLog(), g.refreshDiff())
}

func (g GitOverlay) Update(msg tea.Msg) (GitOverlay, tea.Cmd) {
	if !g.active {
		return g, nil
	}

	switch msg := msg.(type) {
	case gitCmdResultMsg:
		return g.handleCmdResult(msg)

	case tea.KeyMsg:
		if g.userMode {
			return g.updateUserMode(msg)
		}
		if g.commitMode {
			return g.updateCommitMode(msg)
		}
		return g.updateNormal(msg)
	}
	return g, nil
}

func (g GitOverlay) handleCmdResult(msg gitCmdResultMsg) (GitOverlay, tea.Cmd) {
	switch msg.action {
	case "status":
		if msg.err != nil {
			g.errorMsg = "Git error: " + msg.err.Error()
			g.statusLines = nil
		} else {
			g.errorMsg = ""
			g.statusLines = splitNonEmpty(msg.output)
		}
	case "log":
		if msg.err != nil {
			g.logLines = []string{"(error: " + msg.err.Error() + ")"}
		} else {
			g.logLines = splitNonEmpty(msg.output)
		}
	case "diff":
		if msg.err != nil {
			g.diffLines = []string{"(error: " + msg.err.Error() + ")"}
		} else {
			g.diffLines = splitNonEmpty(msg.output)
			if len(g.diffLines) == 0 {
				g.diffLines = []string{"(no unstaged changes)"}
			}
		}
	case "commit":
		if msg.err != nil {
			g.errorMsg = "Commit failed: " + msg.err.Error()
		} else {
			g.errorMsg = ""
			g.commitMode = false
			g.commitMsg = ""
		}
		// Refresh status and log after commit
		return g, tea.Batch(g.refreshStatus(), g.refreshLog())
	case "push":
		if msg.err != nil {
			g.errorMsg = "Push failed: " + msg.err.Error()
		} else {
			g.errorMsg = "Push successful"
		}
		return g, nil
	case "pull":
		if msg.err != nil {
			g.errorMsg = "Pull failed: " + msg.err.Error()
		} else {
			g.errorMsg = "Pull successful"
		}
		return g, tea.Batch(g.refreshStatus(), g.refreshLog())
	}
	return g, nil
}

func (g GitOverlay) updateCommitMode(msg tea.KeyMsg) (GitOverlay, tea.Cmd) {
	switch msg.String() {
	case "esc":
		g.commitMode = false
		g.commitMsg = ""
		return g, nil
	case "enter":
		if g.commitMsg != "" {
			commitMsg := g.commitMsg
			dir := g.vaultRoot
			return g, func() tea.Msg {
				// git add -A && git commit -m "message"
				if _, err := runGitCmd(dir, "add", "-A"); err != nil {
					return gitCmdResultMsg{action: "commit", err: fmt.Errorf("add: %w", err)}
				}
				out, err := runGitCmd(dir, "commit", "-m", commitMsg)
				return gitCmdResultMsg{action: "commit", output: out, err: err}
			}
		}
		return g, nil
	case "backspace":
		if len(g.commitMsg) > 0 {
			runes := []rune(g.commitMsg)
			g.commitMsg = string(runes[:len(runes)-1])
		}
		return g, nil
	default:
		ch := msg.String()
		runes := []rune(ch)
		if len(runes) == 1 && runes[0] >= 32 {
			g.commitMsg += ch
		}
		return g, nil
	}
}

func (g GitOverlay) updateNormal(msg tea.KeyMsg) (GitOverlay, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		g.active = false
		return g, nil

	// View switching
	case "1":
		g.state = gitStateStatus
		g.scroll = 0
		return g, g.refreshStatus()
	case "2":
		g.state = gitStateLog
		g.scroll = 0
		return g, g.refreshLog()
	case "3":
		g.state = gitStateDiff
		g.scroll = 0
		return g, g.refreshDiff()
	case "tab":
		g.state = (g.state + 1) % 3
		g.scroll = 0
		switch g.state {
		case gitStateStatus:
			return g, g.refreshStatus()
		case gitStateLog:
			return g, g.refreshLog()
		case gitStateDiff:
			return g, g.refreshDiff()
		}

	// Scrolling
	case "j", "down":
		var lines []string
		switch g.state {
		case gitStateStatus:
			lines = g.renderStatusLines()
		case gitStateLog:
			lines = g.renderLogLines()
		case gitStateDiff:
			lines = g.renderDiffLines()
		}
		visH := g.height - 12
		if visH < 5 {
			visH = 5
		}
		maxScroll := len(lines) - visH
		if maxScroll < 0 {
			maxScroll = 0
		}
		if g.scroll < maxScroll {
			g.scroll++
		}
		return g, nil
	case "k", "up":
		if g.scroll > 0 {
			g.scroll--
		}
		return g, nil

	// Actions
	case "c":
		g.commitMode = true
		g.commitMsg = ""
		g.errorMsg = ""
		return g, nil
	case "p":
		dir := g.vaultRoot
		return g, func() tea.Msg {
			out, err := runGitCmd(dir, "push")
			return gitCmdResultMsg{action: "push", output: out, err: err}
		}
	case "P":
		dir := g.vaultRoot
		return g, func() tea.Msg {
			out, err := runGitCmd(dir, "pull")
			return gitCmdResultMsg{action: "pull", output: out, err: err}
		}
	case "r":
		// Manual refresh
		return g, g.RefreshAll()
	case "u":
		// Enter user identity config mode
		g.userMode = true
		g.userField = 0
		g.errorMsg = ""
		return g, nil
	}
	return g, nil
}

func (g GitOverlay) updateUserMode(msg tea.KeyMsg) (GitOverlay, tea.Cmd) {
	switch msg.String() {
	case "esc":
		g.userMode = false
		return g, nil
	case "tab":
		g.userField = (g.userField + 1) % 2
		return g, nil
	case "shift+tab":
		g.userField = (g.userField + 1) % 2
		return g, nil
	case "enter":
		// Save user identity to local git config
		dir := g.vaultRoot
		name := g.userName
		email := g.userEmail
		g.userMode = false
		return g, func() tea.Msg {
			var errParts []string
			if name != "" {
				if _, err := runGitCmd(dir, "config", "user.name", name); err != nil {
					errParts = append(errParts, "name: "+err.Error())
				}
			}
			if email != "" {
				if _, err := runGitCmd(dir, "config", "user.email", email); err != nil {
					errParts = append(errParts, "email: "+err.Error())
				}
			}
			if len(errParts) > 0 {
				return gitCmdResultMsg{action: "status", err: fmt.Errorf("config: %s", strings.Join(errParts, "; "))}
			}
			return gitCmdResultMsg{action: "status", output: ""}
		}
	case "backspace":
		if g.userField == 0 && len(g.userName) > 0 {
			runes := []rune(g.userName)
			g.userName = string(runes[:len(runes)-1])
		} else if g.userField == 1 && len(g.userEmail) > 0 {
			runes := []rune(g.userEmail)
			g.userEmail = string(runes[:len(runes)-1])
		}
		return g, nil
	default:
		ch := msg.String()
		runes := []rune(ch)
		if len(runes) == 1 && runes[0] >= 32 {
			if g.userField == 0 {
				g.userName += ch
			} else {
				g.userEmail += ch
			}
		} else if ch == "space" {
			if g.userField == 0 {
				g.userName += " "
			}
		}
		return g, nil
	}
}

func (g GitOverlay) View() string {
	width := g.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}

	var b strings.Builder

	// Header
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconBotChar + " Git")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n")

	// Tab bar
	tabs := []struct {
		label string
		state gitState
	}{
		{"Status", gitStateStatus},
		{"Log", gitStateLog},
		{"Diff", gitStateDiff},
	}
	var tabParts []string
	for _, t := range tabs {
		if t.state == g.state {
			tabParts = append(tabParts, lipgloss.NewStyle().
				Foreground(mauve).
				Bold(true).
				Render(" ["+t.label+"] "))
		} else {
			tabParts = append(tabParts, DimStyle.Render("  "+t.label+"  "))
		}
	}
	b.WriteString("  " + strings.Join(tabParts, DimStyle.Render("|")))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n")

	// Error display
	if g.errorMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(red).Bold(true)
		b.WriteString("  " + errStyle.Render(g.errorMsg))
		b.WriteString("\n")
	}

	// Content area
	visH := g.height - 12
	if visH < 5 {
		visH = 5
	}

	var lines []string
	switch g.state {
	case gitStateStatus:
		lines = g.renderStatusLines()
	case gitStateLog:
		lines = g.renderLogLines()
	case gitStateDiff:
		lines = g.renderDiffLines()
	}

	// Apply scroll
	maxScroll := len(lines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if g.scroll > maxScroll {
		g.scroll = maxScroll
	}

	end := g.scroll + visH
	if end > len(lines) {
		end = len(lines)
	}

	if len(lines) == 0 {
		b.WriteString(DimStyle.Render("  (empty)"))
		b.WriteString("\n")
	} else {
		for i := g.scroll; i < end; i++ {
			b.WriteString(lines[i])
			b.WriteString("\n")
		}
	}

	// User config mode
	if g.userMode {
		b.WriteString("\n")
		labelStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
		activeStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)
		valueStyle := lipgloss.NewStyle().Foreground(text)

		nameLabel := labelStyle.Render("  Name:  ")
		emailLabel := labelStyle.Render("  Email: ")
		if g.userField == 0 {
			nameLabel = activeStyle.Render("▸ Name:  ")
		}
		if g.userField == 1 {
			emailLabel = activeStyle.Render("▸ Email: ")
		}

		nameCursor := ""
		emailCursor := ""
		if g.userField == 0 {
			nameCursor = DimStyle.Render("_")
		}
		if g.userField == 1 {
			emailCursor = DimStyle.Render("_")
		}

		b.WriteString(nameLabel + valueStyle.Render(g.userName) + nameCursor + "\n")
		b.WriteString(emailLabel + valueStyle.Render(g.userEmail) + emailCursor + "\n")
	}

	// Commit mode input
	if g.commitMode {
		b.WriteString("\n")
		promptStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)
		b.WriteString(promptStyle.Render("  Commit message: "))
		b.WriteString(g.commitMsg + DimStyle.Render("_"))
	}

	// User identity line (when not in user mode)
	if !g.userMode && !g.commitMode && (g.userName != "" || g.userEmail != "") {
		b.WriteString("\n")
		id := "  " + DimStyle.Render("Author: ")
		if g.userName != "" {
			id += DimStyle.Render(g.userName)
		}
		if g.userEmail != "" {
			id += DimStyle.Render(" <" + g.userEmail + ">")
		}
		b.WriteString(id)
	}

	// Footer
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n")
	if g.userMode {
		b.WriteString(DimStyle.Render("  Tab: switch field  Enter: save  Esc: cancel"))
	} else if g.commitMode {
		b.WriteString(DimStyle.Render("  Enter: commit  Esc: cancel"))
	} else {
		b.WriteString(DimStyle.Render("  1/2/3: views  tab: cycle  c: commit  u: author  p: push  P: pull  r: refresh  Esc: close"))
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (g GitOverlay) renderStatusLines() []string {
	if len(g.statusLines) == 0 {
		return []string{DimStyle.Render("  Working tree clean")}
	}
	var out []string
	for _, line := range g.statusLines {
		if len(line) < 3 {
			out = append(out, DimStyle.Render("  "+line))
			continue
		}
		code := line[:2]
		file := strings.TrimSpace(line[2:])

		var icon string
		var iconColor lipgloss.Color
		switch {
		case strings.Contains(code, "M"):
			icon = "M"
			iconColor = yellow
		case strings.Contains(code, "A"):
			icon = "A"
			iconColor = green
		case strings.Contains(code, "D"):
			icon = "D"
			iconColor = red
		case strings.Contains(code, "R"):
			icon = "R"
			iconColor = blue
		case strings.Contains(code, "?"):
			icon = "?"
			iconColor = peach
		default:
			icon = code
			iconColor = text
		}

		statusIcon := lipgloss.NewStyle().
			Foreground(iconColor).
			Bold(true).
			Width(4).
			Render(" " + icon)
		fileName := lipgloss.NewStyle().
			Foreground(text).
			Render(file)
		out = append(out, "  "+statusIcon+" "+fileName)
	}
	return out
}

func (g GitOverlay) renderLogLines() []string {
	if len(g.logLines) == 0 {
		return []string{DimStyle.Render("  No commits")}
	}
	var out []string
	for _, line := range g.logLines {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			hash := lipgloss.NewStyle().
				Foreground(yellow).
				Render(parts[0])
			msg := lipgloss.NewStyle().
				Foreground(text).
				Render(parts[1])
			out = append(out, "  "+hash+" "+msg)
		} else {
			out = append(out, "  "+DimStyle.Render(line))
		}
	}
	return out
}

func (g GitOverlay) renderDiffLines() []string {
	if len(g.diffLines) == 0 {
		return []string{DimStyle.Render("  (no unstaged changes)")}
	}
	var out []string
	for _, line := range g.diffLines {
		var styled string
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			styled = lipgloss.NewStyle().Foreground(blue).Bold(true).Render(line)
		case strings.HasPrefix(line, "@@"):
			styled = lipgloss.NewStyle().Foreground(mauve).Render(line)
		case strings.HasPrefix(line, "+"):
			styled = lipgloss.NewStyle().Foreground(green).Render(line)
		case strings.HasPrefix(line, "-"):
			styled = lipgloss.NewStyle().Foreground(red).Render(line)
		case strings.HasPrefix(line, "diff "):
			styled = lipgloss.NewStyle().Foreground(peach).Bold(true).Render(line)
		default:
			styled = DimStyle.Render(line)
		}
		out = append(out, "  "+styled)
	}
	return out
}

// runGitCmd executes a git command in the given directory and returns its combined output.
// If dir is empty, the process working directory is used.
func runGitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// splitNonEmpty splits a string by newlines and drops empty trailing lines.
func splitNonEmpty(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
