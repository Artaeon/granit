package tasks

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// NoteContent is the minimal note shape the parser needs. Decoupling
// from internal/vault keeps the package dependency direction clean
// (tasks → atomicio → stdlib only); callers convert their richer
// Note types into these at the boundary.
type NoteContent struct {
	Path    string // relative path within vault, e.g. "Tasks.md", "Daily/2026-04-25.md"
	Content string // raw file contents
}

// Regexes lifted verbatim from the original taskmanager.go parser
// so behavior is identical. Compiled once at package init.
var (
	reTaskLine       = regexp.MustCompile(`^(\s*- \[)([ xX])(\] .+)`)
	reDueDateMarker  = regexp.MustCompile(`\x{1F4C5}\s*(\d{4}-\d{2}-\d{2})`) // 📅
	rePrioHighest    = regexp.MustCompile(`\x{1F53A}`)                       // 🔺
	rePrioHigh       = regexp.MustCompile(`\x{23EB}`)                        // ⏫
	rePrioMed        = regexp.MustCompile(`\x{1F53C}`)                       // 🔼
	rePrioLow        = regexp.MustCompile(`\x{1F53D}`)                       // 🔽
	reTagInLine      = regexp.MustCompile(`#([A-Za-z0-9_/-]+)`)
	reScheduledTime  = regexp.MustCompile(`⏰\s*(\d{2}:\d{2}-\d{2}:\d{2})`)
	reDepends        = regexp.MustCompile(`depends:"([^"]+)"|depends:([^\s]+)`)
	reEstimate       = regexp.MustCompile(`~(\d+)(m|h)`)
	reRecurEmoji     = regexp.MustCompile(`\x{1F501}\s*(daily|weekly|monthly|3x-week)`) // 🔁
	reRecurTag       = regexp.MustCompile(`#(daily|weekly|monthly|3x-week)\b`)
	reSnoozeMarker   = regexp.MustCompile(`snooze:(\d{4}-\d{2}-\d{2}T\d{2}:\d{2})`)
	reGoalLink       = regexp.MustCompile(`goal:(G\d{3,})`)
	reDailyNoteName  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// ParseNotes scans every note for GFM checkbox lines and returns
// one Task per line. Field semantics match the legacy
// tui.ParseAllTasks: due-date inferred from daily-note filenames,
// priority from emoji or shorthand, tags collected from the line,
// indent-based parent linkage within a single file.
//
// The Task.ID, Task.Triage, and other sidecar-only fields are left
// zero — the store fills them in during reconciliation.
func ParseNotes(notes []NoteContent) []Task {
	var out []Task
	for _, note := range notes {
		out = append(out, parseNote(note)...)
	}
	return out
}

func parseNote(note NoteContent) []Task {
	if note.Content == "" {
		return nil
	}
	lines := strings.Split(note.Content, "\n")
	// indent level → LineNum of most recent task at that level (1-based).
	parentStack := make(map[int]int)
	var out []Task

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- [") {
			continue
		}
		m := reTaskLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		done := m[2] == "x" || m[2] == "X"
		taskText := m[3][2:] // strip the "] " prefix that group 3 starts with

		indent := leadingIndentColumns(line)
		indentLevel := indent / 2 // 2 spaces per level (tab counts as 2)

		// Walk up the indent stack to find the nearest enclosing
		// task — that's our parent.
		parentLine := 0
		for lvl := indentLevel - 1; lvl >= 0; lvl-- {
			if pl, ok := parentStack[lvl]; ok {
				parentLine = pl
				break
			}
		}
		parentStack[indentLevel] = i + 1

		t := Task{
			Text:       taskText,
			Done:       done,
			NotePath:   note.Path,
			LineNum:    i + 1,
			Indent:     indentLevel,
			ParentLine: parentLine,
		}
		applyMarkdownExtras(&t, note.Path, taskText)
		out = append(out, t)
	}
	return out
}

// applyMarkdownExtras pulls the structured metadata that the user
// (or granit itself) embeds in a task line: due dates, priority,
// schedule, tags, dependencies, recurrence, snooze, goal, estimate.
//
// Daily-note filenames double as an implicit due date for any task
// they contain — that's the long-standing convention from the
// original parser and stays intact.
func applyMarkdownExtras(t *Task, notePath, taskText string) {
	if t.DueDate == "" {
		base := strings.TrimSuffix(filepath.Base(notePath), ".md")
		if reDailyNoteName.MatchString(base) {
			if _, err := time.Parse("2006-01-02", base); err == nil {
				t.DueDate = base
			}
		}
	}
	if dm := reDueDateMarker.FindStringSubmatch(taskText); dm != nil {
		t.DueDate = dm[1]
	}

	switch {
	case rePrioHighest.MatchString(taskText):
		t.Priority = 4
	case rePrioHigh.MatchString(taskText):
		t.Priority = 3
	case rePrioMed.MatchString(taskText):
		t.Priority = 2
	case rePrioLow.MatchString(taskText):
		t.Priority = 1
	}

	if sm := reScheduledTime.FindStringSubmatch(taskText); sm != nil {
		t.ScheduledTime = sm[1]
	}

	for _, tm := range reTagInLine.FindAllStringSubmatch(taskText, -1) {
		t.Tags = append(t.Tags, tm[1])
	}

	for _, dm := range reDepends.FindAllStringSubmatch(taskText, -1) {
		dep := dm[1] // quoted form
		if dep == "" {
			dep = dm[2] // unquoted form
		}
		if dep != "" {
			t.DependsOn = append(t.DependsOn, dep)
		}
	}

	if rm := reRecurEmoji.FindStringSubmatch(taskText); rm != nil {
		t.Recurrence = rm[1]
	} else if rm := reRecurTag.FindStringSubmatch(taskText); rm != nil {
		t.Recurrence = rm[1]
	}

	if sm := reSnoozeMarker.FindStringSubmatch(taskText); sm != nil {
		t.SnoozedUntil = sm[1]
	}

	if gm := reGoalLink.FindStringSubmatch(taskText); gm != nil {
		t.GoalID = gm[1]
	}

	if em := reEstimate.FindStringSubmatch(taskText); em != nil {
		val := 0
		_, _ = fmt.Sscanf(em[1], "%d", &val)
		if em[2] == "h" {
			val *= 60
		}
		t.EstimatedMinutes = val
	}
}

// leadingIndentColumns counts visual indent columns on a line:
// space = 1, tab = 2. Matches the legacy parser's accounting so
// indent levels round-trip identically.
func leadingIndentColumns(line string) int {
	cols := 0
	for _, ch := range line {
		switch ch {
		case ' ':
			cols++
		case '\t':
			cols += 2
		default:
			return cols
		}
	}
	return cols
}
