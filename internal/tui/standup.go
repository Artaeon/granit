package tui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// standupPhase tracks the current stage of the standup generator.
type standupPhase int

const (
	standupScanning standupPhase = iota
	standupPreview
	standupEdit
	standupSaved
)

// StandupGenerator is an overlay that generates a daily standup note by
// scanning git history, modified files, completed tasks, and recently
// created notes.
type StandupGenerator struct {
	active    bool
	width     int
	height    int
	vaultRoot string

	phase standupPhase

	// Scanned data
	commits       []string
	modifiedFiles []string
	doneTasks     []string
	todayTasks    []string
	newNotes      []string

	// Generated sections
	yesterday string
	today     string
	blockers  string

	// Edit state
	editSection int // 0=yesterday, 1=today, 2=blockers
	editBuf     string

	// Scroll
	scroll int

	scanDone  bool
	saved     bool
	statusMsg string
}

// NewStandupGenerator creates a new StandupGenerator in its default state.
func NewStandupGenerator() StandupGenerator {
	return StandupGenerator{}
}

// IsActive reports whether the standup overlay is visible.
func (s StandupGenerator) IsActive() bool {
	return s.active
}

// Open activates the overlay and scans the vault for standup data.
func (s *StandupGenerator) Open(vaultRoot string) {
	s.active = true
	s.vaultRoot = vaultRoot
	s.phase = standupScanning
	s.scroll = 0
	s.editSection = 0
	s.editBuf = ""
	s.saved = false
	s.statusMsg = ""
	s.commits = nil
	s.modifiedFiles = nil
	s.doneTasks = nil
	s.todayTasks = nil
	s.newNotes = nil
	s.yesterday = ""
	s.today = ""
	s.blockers = ""

	s.scan()
	s.generate()
	s.phase = standupPreview
	s.scanDone = true
}

// Close hides the overlay.
func (s *StandupGenerator) Close() {
	s.active = false
}

// SetSize updates the available dimensions for the overlay.
func (s *StandupGenerator) SetSize(w, h int) {
	s.width = w
	s.height = h
}

// ── Scanning ─────────────────────────────────────────────────────

// scan gathers data from git, modified files, tasks, and new notes.
func (s *StandupGenerator) scan() {
	s.scanGitCommits()
	s.scanModifiedFiles()
	s.scanTasks()
	s.scanNewNotes()
}

// scanGitCommits runs git log --since="yesterday" to find recent commits.
func (s *StandupGenerator) scanGitCommits() {
	cmd := exec.Command("git", "-C", s.vaultRoot, "log",
		"--since=yesterday", "--oneline", "--no-merges")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Not a git repo or no commits — that is fine
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			s.commits = append(s.commits, line)
		}
	}
}

// scanModifiedFiles finds .md files modified in the last 24 hours.
func (s *StandupGenerator) scanModifiedFiles() {
	// Try git diff first
	cmd := exec.Command("git", "-C", s.vaultRoot, "diff", "--name-only", "HEAD~5")
	out, err := cmd.CombinedOutput()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && strings.HasSuffix(line, ".md") {
				s.modifiedFiles = append(s.modifiedFiles, line)
			}
		}
		if len(s.modifiedFiles) > 0 {
			return
		}
	}

	// Fallback: walk the vault and check modification times
	cutoff := time.Now().Add(-24 * time.Hour)
	_ = filepath.Walk(s.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		if info.ModTime().After(cutoff) {
			rel, _ := filepath.Rel(s.vaultRoot, path)
			if rel != "" {
				s.modifiedFiles = append(s.modifiedFiles, rel)
			}
		}
		return nil
	})
}

// scanTasks parses Tasks.md for completed and open tasks.
func (s *StandupGenerator) scanTasks() {
	f, err := os.Open(tasksFilePath(s.vaultRoot))
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().Add(-24 * time.Hour).Format("2006-01-02")

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "- [x]") || strings.HasPrefix(line, "- [X]") {
			// Completed task — include if it mentions today or yesterday
			taskText := strings.TrimSpace(line[5:])
			if strings.Contains(line, today) || strings.Contains(line, yesterday) || !containsDate(line) {
				s.doneTasks = append(s.doneTasks, taskText)
			}
		} else if strings.HasPrefix(line, "- [ ]") {
			// Open task — include if it mentions today or has no date
			taskText := strings.TrimSpace(line[5:])
			if strings.Contains(line, today) || !containsDate(line) {
				s.todayTasks = append(s.todayTasks, taskText)
			}
		}
	}
}

// containsDate checks whether a line contains a YYYY-MM-DD date pattern.
func containsDate(line string) bool {
	// Simple check: look for a date-like pattern
	for i := 0; i <= len(line)-10; i++ {
		if line[i] >= '0' && line[i] <= '9' &&
			line[i+4] == '-' && line[i+7] == '-' {
			return true
		}
	}
	return false
}

// scanNewNotes finds .md files created in the last 24 hours.
func (s *StandupGenerator) scanNewNotes() {
	cutoff := time.Now().Add(-24 * time.Hour)
	_ = filepath.Walk(s.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" ||
				name == "Standups" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		// Use modification time as a proxy for creation time, since Go's
		// os.FileInfo does not expose birth time portably.
		if info.ModTime().After(cutoff) {
			rel, _ := filepath.Rel(s.vaultRoot, path)
			if rel != "" {
				// Skip files already in modifiedFiles (avoid duplicates
				// by comparing just the base name for readability)
				alreadyListed := false
				for _, mf := range s.modifiedFiles {
					if mf == rel {
						alreadyListed = true
						break
					}
				}
				if !alreadyListed {
					s.newNotes = append(s.newNotes, rel)
				}
			}
		}
		return nil
	})
}

// ── Generation ───────────────────────────────────────────────────

// generate builds the three standup sections from the scanned data.
func (s *StandupGenerator) generate() {
	var yb strings.Builder

	// Yesterday: git commits
	if len(s.commits) > 0 {
		for _, c := range s.commits {
			yb.WriteString("- " + c + "\n")
		}
	}

	// Yesterday: completed tasks
	if len(s.doneTasks) > 0 {
		for _, t := range s.doneTasks {
			yb.WriteString("- " + t + "\n")
		}
	}

	if yb.Len() == 0 {
		yb.WriteString("- (no activity found)\n")
	}

	s.yesterday = strings.TrimRight(yb.String(), "\n")

	// Today: open tasks + recently modified notes
	var tb strings.Builder

	if len(s.todayTasks) > 0 {
		for _, t := range s.todayTasks {
			tb.WriteString("- " + t + "\n")
		}
	}

	if len(s.modifiedFiles) > 0 {
		for _, f := range s.modifiedFiles {
			name := strings.TrimSuffix(filepath.Base(f), ".md")
			tb.WriteString("- Continue working on: " + name + "\n")
		}
	}

	if len(s.newNotes) > 0 {
		for _, n := range s.newNotes {
			name := strings.TrimSuffix(filepath.Base(n), ".md")
			tb.WriteString("- Review new note: " + name + "\n")
		}
	}

	if tb.Len() == 0 {
		tb.WriteString("- (add your plans for today)\n")
	}

	s.today = strings.TrimRight(tb.String(), "\n")

	// Blockers: start empty for user input
	s.blockers = "- None"
}

// ── Save ─────────────────────────────────────────────────────────

// save writes the standup note to Standups/YYYY-MM-DD.md.
func (s *StandupGenerator) save() {
	if s.vaultRoot == "" {
		s.statusMsg = "Error: no vault root set"
		return
	}
	dir := filepath.Join(s.vaultRoot, "Standups")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0o755)
	}

	today := time.Now().Format("2006-01-02")
	filename := filepath.Join(dir, today+".md")

	var content strings.Builder
	content.WriteString("---\n")
	content.WriteString("title: Standup " + today + "\n")
	content.WriteString("date: " + today + "\n")
	content.WriteString("type: standup\n")
	content.WriteString("tags: [standup, daily]\n")
	content.WriteString("---\n\n")
	content.WriteString("# Daily Standup — " + today + "\n\n")
	content.WriteString("## What I did yesterday\n\n")
	content.WriteString(s.yesterday + "\n\n")
	content.WriteString("## What I'm working on today\n\n")
	content.WriteString(s.today + "\n\n")
	content.WriteString("## Blockers\n\n")
	content.WriteString(s.blockers + "\n")

	err := os.WriteFile(filename, []byte(content.String()), 0o644)
	if err != nil {
		s.statusMsg = "Error: " + err.Error()
		return
	}
	s.saved = true
	s.phase = standupSaved
	s.statusMsg = "Saved to Standups/" + today + ".md"
}

// ── Update ───────────────────────────────────────────────────────

func (s StandupGenerator) Update(msg tea.Msg) (StandupGenerator, tea.Cmd) {
	if !s.active {
		return s, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch s.phase {
		case standupPreview:
			return s.updatePreview(msg)
		case standupEdit:
			return s.updateEdit(msg)
		case standupSaved:
			return s.updateSaved(msg)
		}
	}
	return s, nil
}

func (s StandupGenerator) updatePreview(msg tea.KeyMsg) (StandupGenerator, tea.Cmd) {
	switch msg.String() {
	case "esc":
		s.active = false
	case "e":
		s.phase = standupEdit
		s.editSection = 0
		s.loadEditBuf()
	case "s", "enter":
		s.save()
	case "j", "down":
		s.scroll++
	case "k", "up":
		if s.scroll > 0 {
			s.scroll--
		}
	}
	return s, nil
}

func (s StandupGenerator) updateEdit(msg tea.KeyMsg) (StandupGenerator, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Save current edit buffer back and return to preview
		s.commitEditBuf()
		s.phase = standupPreview
		s.scroll = 0
	case "tab":
		s.commitEditBuf()
		s.editSection = (s.editSection + 1) % 3
		s.loadEditBuf()
	case "shift+tab":
		s.commitEditBuf()
		s.editSection = (s.editSection + 2) % 3
		s.loadEditBuf()
	case "enter":
		s.editBuf += "\n"
	case "backspace":
		if len(s.editBuf) > 0 {
			s.editBuf = TrimLastRune(s.editBuf)
		}
	default:
		if len(msg.String()) == 1 || msg.String() == " " {
			s.editBuf += msg.String()
		}
	}
	return s, nil
}

func (s StandupGenerator) updateSaved(msg tea.KeyMsg) (StandupGenerator, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "q":
		s.active = false
	}
	return s, nil
}

// loadEditBuf loads the current section content into the edit buffer.
func (s *StandupGenerator) loadEditBuf() {
	switch s.editSection {
	case 0:
		s.editBuf = s.yesterday
	case 1:
		s.editBuf = s.today
	case 2:
		s.editBuf = s.blockers
	}
}

// commitEditBuf writes the edit buffer back to the appropriate section.
func (s *StandupGenerator) commitEditBuf() {
	switch s.editSection {
	case 0:
		s.yesterday = s.editBuf
	case 1:
		s.today = s.editBuf
	case 2:
		s.blockers = s.editBuf
	}
}

// ── View ─────────────────────────────────────────────────────────

func (s StandupGenerator) View() string {
	width := s.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 90 {
		width = 90
	}

	switch s.phase {
	case standupScanning:
		return s.viewScanning(width)
	case standupPreview:
		return s.viewPreview(width)
	case standupEdit:
		return s.viewEdit(width)
	case standupSaved:
		return s.viewSaved(width)
	}
	return ""
}

func (s StandupGenerator) viewScanning(width int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconOutlineChar + " Daily Standup Generator")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(yellow).Render("  Scanning vault..."))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (s StandupGenerator) viewPreview(width int) string {
	var b strings.Builder

	today := time.Now().Format("2006-01-02")
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconOutlineChar + " Daily Standup — " + today)
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")

	// Build content lines
	var lines []string

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	contentStyle := lipgloss.NewStyle().Foreground(text)
	commitStyle := lipgloss.NewStyle().Foreground(green)
	taskStyle := lipgloss.NewStyle().Foreground(peach)
	noteStyle := lipgloss.NewStyle().Foreground(teal)

	// Yesterday section
	lines = append(lines, "")
	lines = append(lines, sectionStyle.Render("  What I did yesterday"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("\u2500", 30)))
	for _, line := range strings.Split(s.yesterday, "\n") {
		if strings.HasPrefix(line, "- ") {
			// Color based on whether it looks like a commit or a task
			trimmed := line[2:]
			if len(s.commits) > 0 && isCommitLine(trimmed, s.commits) {
				lines = append(lines, "  "+commitStyle.Render(line))
			} else {
				lines = append(lines, "  "+taskStyle.Render(line))
			}
		} else {
			lines = append(lines, "  "+contentStyle.Render(line))
		}
	}

	// Today section
	lines = append(lines, "")
	lines = append(lines, sectionStyle.Render("  What I'm working on today"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("\u2500", 30)))
	for _, line := range strings.Split(s.today, "\n") {
		if strings.Contains(line, "Continue working on:") {
			lines = append(lines, "  "+noteStyle.Render(line))
		} else if strings.Contains(line, "Review new note:") {
			lines = append(lines, "  "+noteStyle.Render(line))
		} else {
			lines = append(lines, "  "+taskStyle.Render(line))
		}
	}

	// Blockers section
	lines = append(lines, "")
	lines = append(lines, sectionStyle.Render("  Blockers"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("\u2500", 30)))
	for _, line := range strings.Split(s.blockers, "\n") {
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(red).Render(line))
	}

	// Data sources summary
	lines = append(lines, "")
	lines = append(lines, DimStyle.Render(fmt.Sprintf("  Sources: %d commits, %d modified files, %d tasks, %d new notes",
		len(s.commits), len(s.modifiedFiles),
		len(s.doneTasks)+len(s.todayTasks), len(s.newNotes))))

	// Scroll the content
	visH := s.height - 14
	if visH < 10 {
		visH = 10
	}
	maxScroll := len(lines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := s.scroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + visH
	if end > len(lines) {
		end = len(lines)
	}

	for i := scroll; i < end; i++ {
		b.WriteString(lines[i])
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  e: edit  s/Enter: save  j/k: scroll  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (s StandupGenerator) viewEdit(width int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconEditChar + " Edit Standup")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")

	sectionNames := []string{"What I did yesterday", "What I'm working on today", "Blockers"}
	sectionColors := []lipgloss.Color{green, blue, red}
	sections := []string{s.yesterday, s.today, s.blockers}

	var lines []string

	for i, name := range sectionNames {
		lines = append(lines, "")

		headerStyle := lipgloss.NewStyle().Foreground(sectionColors[i]).Bold(true)
		if i == s.editSection {
			headerStyle = headerStyle.Background(surface0)
		}
		indicator := "  "
		if i == s.editSection {
			indicator = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("> ")
		}
		lines = append(lines, indicator+headerStyle.Render(name))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("\u2500", 30)))

		content := sections[i]
		if i == s.editSection {
			content = s.editBuf
		}

		for _, line := range strings.Split(content, "\n") {
			if i == s.editSection {
				lines = append(lines, "  "+lipgloss.NewStyle().
					Foreground(text).
					Background(surface0).
					Render(line))
			} else {
				lines = append(lines, "  "+DimStyle.Render(line))
			}
		}

		// Show cursor in active section
		if i == s.editSection {
			lines = append(lines, "  "+lipgloss.NewStyle().
				Background(text).
				Foreground(mantle).
				Render(" "))
		}
	}

	// Scroll the content
	visH := s.height - 14
	if visH < 10 {
		visH = 10
	}
	maxScroll := len(lines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := s.scroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + visH
	if end > len(lines) {
		end = len(lines)
	}

	for i := scroll; i < end; i++ {
		b.WriteString(lines[i])
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Tab: next section  Shift+Tab: prev  Esc: back to preview"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (s StandupGenerator) viewSaved(width int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconSaveChar + " Standup Saved")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	checkStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	b.WriteString(checkStyle.Render("  Standup saved successfully!"))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(text).Render("  " + s.statusMsg))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter/Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// ── Helpers ──────────────────────────────────────────────────────

// isCommitLine checks whether a line text matches any of the known commits.
func isCommitLine(text string, commits []string) bool {
	for _, c := range commits {
		if strings.Contains(c, text) || text == c {
			return true
		}
	}
	return false
}
