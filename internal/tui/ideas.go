package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/tasks"
)

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

type IdeaStage string

const (
	IdeaInbox      IdeaStage = "inbox"
	IdeaExploring  IdeaStage = "exploring"
	IdeaValidated  IdeaStage = "validated"
	IdeaInProgress IdeaStage = "in_progress"
	IdeaDone       IdeaStage = "done"
	IdeaParked     IdeaStage = "parked"
)

var ideaStages = []IdeaStage{IdeaInbox, IdeaExploring, IdeaValidated, IdeaInProgress, IdeaDone}

type Idea struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Stage       IdeaStage `json:"stage"`
	Category    string    `json:"category,omitempty"` // startup, business, product, side-project, improvement
	Impact      int       `json:"impact,omitempty"`   // 1-5 potential impact
	Effort      int       `json:"effort,omitempty"`   // 1-5 estimated effort
	Notes       string    `json:"notes,omitempty"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
	ConvertedTo string    `json:"converted_to,omitempty"` // "goal:G001", "task", "project:Name"
}

// Score returns impact/effort ratio (higher = better ROI).
func (idea Idea) Score() float64 {
	if idea.Effort == 0 {
		return 0
	}
	return float64(idea.Impact) / float64(idea.Effort)
}

// ---------------------------------------------------------------------------
// Input modes
// ---------------------------------------------------------------------------

type ideaInputMode int

const (
	ideaInputNone ideaInputMode = iota
	ideaInputTitle
	ideaInputDescription
	ideaInputCategory
	ideaInputNotes
	ideaInputConvert // converting to goal/task/project
	ideaInputHelp
)

// ---------------------------------------------------------------------------
// IdeasBoard overlay
// ---------------------------------------------------------------------------

type IdeasBoard struct {
	OverlayBase
	vaultRoot string
	// taskStore is set by Model when cfg.UseTaskStore is on. When
	// non-nil, "convert idea to task" routes through store.Create
	// (gets a stable ID, sidecar entry, etc.) instead of the raw
	// appendTaskLine path. Nil falls back to legacy behavior.
	taskStore *tasks.TaskStore

	ideas   []Idea
	col     int // current column (0-4 = stages)
	cursor  int // cursor within current column
	scroll  int
	input   ideaInputMode
	inputBuf string

	// Confirm
	confirmMsg    string
	confirmAction func()

	statusMsg   string
	fileChanged bool
	lastSaveErr error // consumed-once via ConsumeSaveError
}

func NewIdeasBoard() IdeasBoard {
	return IdeasBoard{}
}

// ConsumeSaveError returns the most recent saveIdeas error and clears it.
// Returns nil on a nil receiver so hosts can call it defensively.
func (ib *IdeasBoard) ConsumeSaveError() error {
	if ib == nil {
		return nil
	}
	err := ib.lastSaveErr
	ib.lastSaveErr = nil
	return err
}

func (ib *IdeasBoard) Open(vaultRoot string) {
	ib.Activate()
	ib.vaultRoot = vaultRoot
	ib.col = 0
	ib.cursor = 0
	ib.scroll = 0
	ib.input = ideaInputNone
	ib.inputBuf = ""
	ib.statusMsg = ""
	ib.confirmMsg = ""
	ib.loadIdeas()
}

func (ib *IdeasBoard) WasFileChanged() bool {
	if ib.fileChanged {
		ib.fileChanged = false
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Storage
// ---------------------------------------------------------------------------

func (ib *IdeasBoard) ideasPath() string {
	return filepath.Join(ib.vaultRoot, ".granit", "ideas.json")
}

func (ib *IdeasBoard) loadIdeas() {
	ib.ideas = nil
	data, err := os.ReadFile(ib.ideasPath())
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, &ib.ideas); err != nil {
		ib.ideas = []Idea{}
	}
}

func (ib *IdeasBoard) saveIdeas() {
	dir := filepath.Join(ib.vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		ib.lastSaveErr = err
		return
	}
	data, err := json.MarshalIndent(ib.ideas, "", "  ")
	if err != nil {
		ib.lastSaveErr = err
		return
	}
	if err := atomicWriteState(ib.ideasPath(), data); err != nil {
		ib.lastSaveErr = err
		return
	}
	ib.lastSaveErr = nil
}

func (ib *IdeasBoard) nextID() string {
	max := 0
	for _, idea := range ib.ideas {
		if len(idea.ID) > 1 && idea.ID[0] == 'I' {
			n := 0
			_, _ = fmt.Sscanf(idea.ID[1:], "%d", &n)
			if n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("I%03d", max+1)
}

// ---------------------------------------------------------------------------
// Column helpers
// ---------------------------------------------------------------------------

func (ib *IdeasBoard) ideasInStage(stage IdeaStage) []Idea {
	var result []Idea
	for _, idea := range ib.ideas {
		if idea.Stage == stage {
			result = append(result, idea)
		}
	}
	return result
}

func (ib *IdeasBoard) currentStage() IdeaStage {
	if ib.col < 0 || ib.col >= len(ideaStages) {
		return IdeaInbox
	}
	return ideaStages[ib.col]
}

func (ib *IdeasBoard) currentIdeas() []Idea {
	return ib.ideasInStage(ib.currentStage())
}

func (ib *IdeasBoard) findIndex(id string) int {
	for i, idea := range ib.ideas {
		if idea.ID == id {
			return i
		}
	}
	return -1
}

func (ib *IdeasBoard) selectedIdea() *Idea {
	ideas := ib.currentIdeas()
	if ib.cursor < 0 || ib.cursor >= len(ideas) {
		return nil
	}
	idx := ib.findIndex(ideas[ib.cursor].ID)
	if idx < 0 {
		return nil
	}
	return &ib.ideas[idx]
}

// ---------------------------------------------------------------------------
// Operations
// ---------------------------------------------------------------------------

func (ib *IdeasBoard) addIdea(title string) {
	now := time.Now().Format("2006-01-02")
	ib.ideas = append(ib.ideas, Idea{
		ID:        ib.nextID(),
		Title:     title,
		Stage:     IdeaInbox,
		CreatedAt: now,
		UpdatedAt: now,
	})
	ib.saveIdeas()
	ib.statusMsg = "Idea added: " + title
}

func (ib *IdeasBoard) moveRight() {
	idea := ib.selectedIdea()
	if idea == nil {
		return
	}
	// Find next stage
	for i, s := range ideaStages {
		if s == idea.Stage && i < len(ideaStages)-1 {
			idea.Stage = ideaStages[i+1]
			idea.UpdatedAt = time.Now().Format("2006-01-02")
			ib.saveIdeas()
			ib.statusMsg = "Moved to " + string(idea.Stage)
			// Adjust cursor
			ideas := ib.currentIdeas()
			if ib.cursor >= len(ideas) && ib.cursor > 0 {
				ib.cursor--
			}
			return
		}
	}
}

func (ib *IdeasBoard) moveLeft() {
	idea := ib.selectedIdea()
	if idea == nil {
		return
	}
	for i, s := range ideaStages {
		if s == idea.Stage && i > 0 {
			idea.Stage = ideaStages[i-1]
			idea.UpdatedAt = time.Now().Format("2006-01-02")
			ib.saveIdeas()
			ib.statusMsg = "Moved to " + string(idea.Stage)
			ideas := ib.currentIdeas()
			if ib.cursor >= len(ideas) && ib.cursor > 0 {
				ib.cursor--
			}
			return
		}
	}
}

func (ib *IdeasBoard) parkIdea() {
	idea := ib.selectedIdea()
	if idea == nil {
		return
	}
	idea.Stage = IdeaParked
	idea.UpdatedAt = time.Now().Format("2006-01-02")
	ib.saveIdeas()
	ib.statusMsg = "Idea parked"
	ideas := ib.currentIdeas()
	if ib.cursor >= len(ideas) && ib.cursor > 0 {
		ib.cursor--
	}
}

func (ib *IdeasBoard) deleteIdea() {
	idea := ib.selectedIdea()
	if idea == nil {
		return
	}
	idx := ib.findIndex(idea.ID)
	if idx >= 0 {
		ib.ideas = append(ib.ideas[:idx], ib.ideas[idx+1:]...)
		ib.saveIdeas()
		ib.statusMsg = "Idea deleted"
		ideas := ib.currentIdeas()
		if ib.cursor >= len(ideas) && ib.cursor > 0 {
			ib.cursor--
		}
	}
}

func (ib *IdeasBoard) cycleImpact() {
	idea := ib.selectedIdea()
	if idea == nil {
		return
	}
	idea.Impact = idea.Impact%5 + 1
	idea.UpdatedAt = time.Now().Format("2006-01-02")
	ib.saveIdeas()
	ib.statusMsg = fmt.Sprintf("Impact: %d/5", idea.Impact)
}

func (ib *IdeasBoard) cycleEffort() {
	idea := ib.selectedIdea()
	if idea == nil {
		return
	}
	idea.Effort = idea.Effort%5 + 1
	idea.UpdatedAt = time.Now().Format("2006-01-02")
	ib.saveIdeas()
	ib.statusMsg = fmt.Sprintf("Effort: %d/5", idea.Effort)
}

func (ib *IdeasBoard) convertToGoal() {
	idea := ib.selectedIdea()
	if idea == nil {
		return
	}
	// Create goal via goals.json
	gm := &GoalsMode{vaultRoot: ib.vaultRoot}
	gm.loadGoals()
	gm.addGoal(idea.Title, "", idea.Category)
	goalID := ""
	if len(gm.goals) > 0 {
		goalID = gm.goals[len(gm.goals)-1].ID
	}
	idea.ConvertedTo = "goal:" + goalID
	idea.Stage = IdeaInProgress
	idea.UpdatedAt = time.Now().Format("2006-01-02")
	ib.saveIdeas()
	ib.statusMsg = "Converted to goal: " + goalID
	ib.fileChanged = true
}

// SetTaskStore wires the unified TaskStore so converting an idea
// to a task gets a stable ID and sidecar entry. Nil-safe — if the
// store isn't available the convert path falls back to
// appendTaskLine (legacy behavior).
func (ib *IdeasBoard) SetTaskStore(s *tasks.TaskStore) { ib.taskStore = s }

func (ib *IdeasBoard) convertToTask() {
	idea := ib.selectedIdea()
	if idea == nil {
		return
	}
	if ib.taskStore != nil {
		if _, err := ib.taskStore.Create(idea.Title, tasks.CreateOpts{Origin: tasks.OriginManual}); err != nil {
			ib.statusMsg = "Failed to convert: " + err.Error()
			return
		}
	} else if err := appendTaskLine(ib.vaultRoot, "- [ ] "+idea.Title); err != nil {
		ib.statusMsg = "Failed to convert: " + err.Error()
		return
	}
	idea.ConvertedTo = "task"
	idea.Stage = IdeaInProgress
	idea.UpdatedAt = time.Now().Format("2006-01-02")
	ib.saveIdeas()
	ib.statusMsg = "Converted to task"
	ib.fileChanged = true
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (ib IdeasBoard) Update(msg tea.Msg) (IdeasBoard, tea.Cmd) {
	if !ib.active {
		return ib, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		if ib.input != ideaInputNone {
			return ib.updateInput(key)
		}
		return ib.updateNormal(key)
	}
	return ib, nil
}

func (ib IdeasBoard) updateNormal(key string) (IdeasBoard, tea.Cmd) {
	ib.statusMsg = ""

	// Handle confirmation
	if ib.confirmMsg != "" {
		if key == "y" || key == "Y" {
			if ib.confirmAction != nil {
				ib.confirmAction()
			}
		}
		ib.confirmMsg = ""
		ib.confirmAction = nil
		return ib, nil
	}

	switch key {
	case "esc", "q":
		ib.Close()

	// Column navigation
	case "h", "left":
		if ib.col > 0 {
			ib.col--
			ib.cursor = 0
		}
	case "l", "right":
		if ib.col < len(ideaStages)-1 {
			ib.col++
			ib.cursor = 0
		}

	// Item navigation
	case "j", "down":
		ideas := ib.currentIdeas()
		if ib.cursor < len(ideas)-1 {
			ib.cursor++
		}
	case "k", "up":
		if ib.cursor > 0 {
			ib.cursor--
		}

	// Add idea
	case "a":
		ib.input = ideaInputTitle
		ib.inputBuf = ""

	// Move idea through stages
	case ">", "tab":
		ib.moveRight()
	case "<", "shift+tab":
		ib.moveLeft()

	// Park idea
	case "p":
		ib.parkIdea()

	// Delete (with confirmation)
	case "D":
		if idea := ib.selectedIdea(); idea != nil {
			ib.confirmMsg = fmt.Sprintf("Delete \"%s\"? (y/n)", idea.Title)
			ib.confirmAction = func() { ib.deleteIdea() }
		}

	// Edit description
	case "e":
		if idea := ib.selectedIdea(); idea != nil {
			ib.input = ideaInputDescription
			ib.inputBuf = idea.Description
		}

	// Edit notes
	case "n":
		if idea := ib.selectedIdea(); idea != nil {
			ib.input = ideaInputNotes
			ib.inputBuf = idea.Notes
		}

	// Set category
	case "c":
		if ib.selectedIdea() != nil {
			ib.input = ideaInputCategory
		}

	// Cycle impact/effort
	case "i":
		ib.cycleImpact()
	case "E":
		ib.cycleEffort()

	// Convert
	case "G":
		if idea := ib.selectedIdea(); idea != nil {
			ib.input = ideaInputConvert
		}

	// Help
	case "?":
		ib.input = ideaInputHelp
	}

	return ib, nil
}

func (ib IdeasBoard) updateInput(key string) (IdeasBoard, tea.Cmd) {
	switch ib.input {
	case ideaInputHelp:
		ib.input = ideaInputNone
		return ib, nil

	case ideaInputTitle:
		switch key {
		case "esc":
			ib.input = ideaInputNone
		case "enter":
			title := strings.TrimSpace(ib.inputBuf)
			if title != "" {
				ib.addIdea(title)
			}
			ib.input = ideaInputNone
			ib.inputBuf = ""
		case "backspace":
			if len(ib.inputBuf) > 0 {
				ib.inputBuf = TrimLastRune(ib.inputBuf)
			}
		default:
			if len(key) == 1 || key == " " {
				ib.inputBuf += key
			}
		}

	case ideaInputDescription, ideaInputNotes:
		switch key {
		case "esc":
			ib.input = ideaInputNone
		case "enter":
			if idea := ib.selectedIdea(); idea != nil {
				text := strings.TrimSpace(ib.inputBuf)
				if ib.input == ideaInputDescription {
					idea.Description = text
				} else {
					idea.Notes = text
				}
				idea.UpdatedAt = time.Now().Format("2006-01-02")
				ib.saveIdeas()
				ib.statusMsg = "Saved"
			}
			ib.input = ideaInputNone
			ib.inputBuf = ""
		case "backspace":
			if len(ib.inputBuf) > 0 {
				ib.inputBuf = TrimLastRune(ib.inputBuf)
			}
		default:
			if len(key) == 1 || key == " " {
				ib.inputBuf += key
			}
		}

	case ideaInputCategory:
		cats := []string{"startup", "business", "product", "side-project", "improvement"}
		switch key {
		case "esc":
			ib.input = ideaInputNone
		case "1", "2", "3", "4", "5":
			idx := int(key[0] - '1')
			if idx < len(cats) {
				if idea := ib.selectedIdea(); idea != nil {
					idea.Category = cats[idx]
					idea.UpdatedAt = time.Now().Format("2006-01-02")
					ib.saveIdeas()
					ib.statusMsg = "Category: " + cats[idx]
				}
			}
			ib.input = ideaInputNone
		}

	case ideaInputConvert:
		switch key {
		case "esc":
			ib.input = ideaInputNone
		case "1": // Goal
			ib.convertToGoal()
			ib.input = ideaInputNone
		case "2": // Task
			ib.convertToTask()
			ib.input = ideaInputNone
		}
	}

	return ib, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (ib *IdeasBoard) View() string {
	if !ib.active {
		return ""
	}

	width := ib.width * 4 / 5
	if width < 80 {
		width = 80
	}
	if width > 140 {
		width = 140
	}
	innerW := width - 8

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render(IconCanvasChar+" Ideas Board"))

	// Stats
	total := len(ib.ideas)
	parked := len(ib.ideasInStage(IdeaParked))
	active := total - parked
	ds := DimStyle
	ns := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	b.WriteString("  " + ns.Render(fmt.Sprintf("%d", active)) + ds.Render(" active") +
		"  " + ns.Render(fmt.Sprintf("%d", parked)) + ds.Render(" parked"))
	b.WriteString("\n\n")

	// Column headers
	colW := (innerW - 4) / 5
	if colW < 14 {
		colW = 14
	}
	stageNames := map[IdeaStage]string{
		IdeaInbox: "INBOX", IdeaExploring: "EXPLORING", IdeaValidated: "VALIDATED",
		IdeaInProgress: "IN PROGRESS", IdeaDone: "DONE",
	}
	stageColors := map[IdeaStage]lipgloss.Color{
		IdeaInbox: overlay1, IdeaExploring: blue, IdeaValidated: green,
		IdeaInProgress: peach, IdeaDone: teal,
	}

	// Input mode overlay
	if ib.input == ideaInputHelp {
		ib.renderHelp(&b, innerW)
	} else if ib.input != ideaInputNone {
		ib.renderInput(&b, innerW)
	} else {
		// Render columns
		var headers []string
		for ci, stage := range ideaStages {
			count := len(ib.ideasInStage(stage))
			name := stageNames[stage]
			color := stageColors[stage]
			headerStyle := lipgloss.NewStyle().Foreground(color).Bold(true)
			if ci == ib.col {
				headerStyle = headerStyle.Underline(true)
			}
			label := headerStyle.Render(name) + ds.Render(fmt.Sprintf(" %d", count))
			headers = append(headers, PadRight(label, colW))
		}
		b.WriteString(strings.Join(headers, " "))
		b.WriteString("\n")

		// Render rows
		maxRows := ib.height - 12
		if maxRows < 3 {
			maxRows = 3
		}

		for row := 0; row < maxRows; row++ {
			var cells []string
			for ci, stage := range ideaStages {
				ideas := ib.ideasInStage(stage)
				if row >= len(ideas) {
					cells = append(cells, strings.Repeat(" ", colW))
					continue
				}
				idea := ideas[row]
				isCursor := ci == ib.col && row == ib.cursor

				// Build card
				prefix := "  "
				if isCursor {
					prefix = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("> ")
				}

				titleSt := lipgloss.NewStyle().Foreground(text)
				if isCursor {
					titleSt = titleSt.Bold(true)
				}

				title := TruncateDisplay(idea.Title, colW-4)
				card := prefix + titleSt.Render(title)

				// Score badge
				if idea.Impact > 0 && idea.Effort > 0 {
					score := idea.Score()
					scoreColor := overlay0
					if score >= 2.0 {
						scoreColor = green
					} else if score >= 1.0 {
						scoreColor = yellow
					}
					card = PadRight(card, colW-4) + lipgloss.NewStyle().Foreground(scoreColor).Render(fmt.Sprintf("%.1f", score))
				}

				// Category indicator
				if idea.Category != "" && isCursor {
					catColor := sapphire
					card += "\n    " + lipgloss.NewStyle().Foreground(catColor).Render("["+idea.Category+"]")
				}

				cells = append(cells, PadRight(card, colW))
			}
			b.WriteString(strings.Join(cells, " ") + "\n")
		}

		// Selected idea detail
		if idea := ib.selectedIdea(); idea != nil {
			b.WriteString("\n")
			b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)) + "\n")
			detailTitle := lipgloss.NewStyle().Foreground(text).Bold(true).Render(idea.Title)
			b.WriteString("  " + detailTitle)
			if idea.Category != "" {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(sapphire).Render("["+idea.Category+"]"))
			}
			if idea.Impact > 0 {
				b.WriteString("  " + ds.Render(fmt.Sprintf("Impact:%d", idea.Impact)))
			}
			if idea.Effort > 0 {
				b.WriteString("  " + ds.Render(fmt.Sprintf("Effort:%d", idea.Effort)))
			}
			if idea.ConvertedTo != "" {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(green).Render("-> "+idea.ConvertedTo))
			}
			b.WriteString("\n")
			if idea.Description != "" {
				b.WriteString("  " + ds.Render(TruncateDisplay(idea.Description, innerW-4)) + "\n")
			}
			if idea.Notes != "" {
				b.WriteString("  " + ds.Render("\U0001F4DD "+TruncateDisplay(idea.Notes, innerW-6)) + "\n")
			}
		}
	}

	// Confirmation
	if ib.confirmMsg != "" {
		b.WriteString("\n  " + lipgloss.NewStyle().Foreground(red).Bold(true).Render(ib.confirmMsg))
	}

	// Status
	if ib.statusMsg != "" {
		b.WriteString("\n  " + lipgloss.NewStyle().Foreground(green).Render(ib.statusMsg))
	}

	// Help bar
	b.WriteString("\n")
	ks := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	pairs := []string{
		ks.Render("h/l") + ds.Render(":col"), ks.Render("j/k") + ds.Render(":nav"),
		ks.Render("a") + ds.Render(":add"), ks.Render(">/<") + ds.Render(":move"),
		ks.Render("i") + ds.Render(":impact"), ks.Render("E") + ds.Render(":effort"),
		ks.Render("c") + ds.Render(":category"), ks.Render("G") + ds.Render(":convert"),
		ks.Render("e") + ds.Render(":desc"), ks.Render("n") + ds.Render(":note"),
		ks.Render("?") + ds.Render(":help"), ks.Render("Esc") + ds.Render(":close"),
	}
	b.WriteString("  " + strings.Join(pairs, " "))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (ib *IdeasBoard) renderInput(b *strings.Builder, w int) {
	promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	inputStyle := lipgloss.NewStyle().Foreground(text)
	hintStyle := DimStyle

	switch ib.input {
	case ideaInputTitle:
		b.WriteString("\n  " + promptStyle.Render("New Idea: ") + inputStyle.Render(ib.inputBuf+"\u2588") + "\n")
		b.WriteString("  " + hintStyle.Render("Enter to add, Esc to cancel"))
	case ideaInputDescription:
		b.WriteString("\n  " + promptStyle.Render("Description: ") + inputStyle.Render(ib.inputBuf+"\u2588") + "\n")
	case ideaInputNotes:
		b.WriteString("\n  " + promptStyle.Render("Notes: ") + inputStyle.Render(ib.inputBuf+"\u2588") + "\n")
	case ideaInputCategory:
		b.WriteString("\n  " + promptStyle.Render("Category:") + "\n\n")
		ss := lipgloss.NewStyle().Foreground(lavender).Bold(true)
		ds := lipgloss.NewStyle().Foreground(text)
		b.WriteString("  " + ss.Render("1") + ds.Render(" startup       ") + ss.Render("2") + ds.Render(" business") + "\n")
		b.WriteString("  " + ss.Render("3") + ds.Render(" product       ") + ss.Render("4") + ds.Render(" side-project") + "\n")
		b.WriteString("  " + ss.Render("5") + ds.Render(" improvement") + "\n")
	case ideaInputConvert:
		b.WriteString("\n  " + promptStyle.Render("Convert to:") + "\n\n")
		ss := lipgloss.NewStyle().Foreground(lavender).Bold(true)
		ds := lipgloss.NewStyle().Foreground(text)
		b.WriteString("  " + ss.Render("1") + ds.Render(" Goal (with milestones and tracking)") + "\n")
		b.WriteString("  " + ss.Render("2") + ds.Render(" Task (add to Tasks.md)") + "\n")
	}
}

func (ib *IdeasBoard) renderHelp(b *strings.Builder, w int) {
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true).Width(8)
	descStyle := lipgloss.NewStyle().Foreground(text)
	sectionStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)

	b.WriteString("\n" + titleStyle.Render("  Ideas Board Shortcuts") + "\n\n")

	sections := []struct {
		title string
		keys  [][2]string
	}{
		{"Navigation", [][2]string{
			{"h/l", "Move between columns"},
			{"j/k", "Move between ideas in column"},
			{">/<", "Move idea to next/previous stage"},
			{"Tab", "Move idea right (same as >)"},
		}},
		{"Actions", [][2]string{
			{"a", "Add new idea to Inbox"},
			{"e", "Edit description"},
			{"n", "Edit notes"},
			{"c", "Set category (startup/business/product/...)"},
			{"i", "Cycle impact rating (1-5)"},
			{"E", "Cycle effort rating (1-5)"},
			{"p", "Park idea (hide from board)"},
			{"D", "Delete idea permanently"},
			{"G", "Convert to Goal or Task"},
		}},
	}

	for _, sec := range sections {
		b.WriteString("  " + sectionStyle.Render(sec.title) + "\n")
		for _, kv := range sec.keys {
			b.WriteString("    " + keyStyle.Render(kv[0]) + descStyle.Render(kv[1]) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("  " + DimStyle.Render("Press any key to close"))
}
