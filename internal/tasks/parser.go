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
	// ASCII shorthand `due:YYYY-MM-DD` is what the web's
	// buildTaskTextLine writes (and what the TUI also emits in
	// inline-task quick-capture). Without this regex, the parser falls
	// back to the daily-note filename for the due-date field — every
	// API-created task with a future date silently appeared "due
	// today" because that was the daily it lived in.
	reDueDateAscii   = regexp.MustCompile(`(?:^|\s)due:(\d{4}-\d{2}-\d{2})(?:\s|$)`)
	rePrioHighest    = regexp.MustCompile(`\x{1F53A}`)                       // 🔺
	rePrioHigh       = regexp.MustCompile(`\x{23EB}`)                        // ⏫
	rePrioMed        = regexp.MustCompile(`\x{1F53C}`)                       // 🔼
	rePrioLow        = regexp.MustCompile(`\x{1F53D}`)                       // 🔽
	// ASCII shorthand `!1` / `!2` / `!3` is what the web's
	// buildTaskTextLine writes (and what the TUI also accepts on
	// inline-task entry). Without this regex, web-created tasks would
	// have `!1` in the markdown line but Priority=0 in the TaskStore —
	// the API would round-trip a priority field of 0 even though the
	// marker was written. Same numeric scale as the emoji branch:
	// !1=Highest=4, !2=High=3, !3=Med=2.
	rePrioBangAscii = regexp.MustCompile(`(?:^|\s)!([1-3])(?:\s|$)`)
	reTagInLine      = regexp.MustCompile(`#([A-Za-z0-9_/-]+)`)
	reScheduledTime  = regexp.MustCompile(`⏰\s*(\d{2}:\d{2}-\d{2}:\d{2})`)
	reDepends        = regexp.MustCompile(`depends:"([^"]+)"|depends:([^\s]+)`)
	reEstimate       = regexp.MustCompile(`~(\d+)(m|h)`)
	reRecurEmoji     = regexp.MustCompile(`\x{1F501}\s*(daily|weekly|monthly|3x-week)`) // 🔁
	reRecurTag       = regexp.MustCompile(`#(daily|weekly|monthly|3x-week)\b`)
	reSnoozeMarker   = regexp.MustCompile(`snooze:(\d{4}-\d{2}-\d{2}T\d{2}:\d{2})`)
	// Goal marker — historically `goal:Gxxx` (TUI-minted IDs) but the
	// web's goals API mints `goal-<timestamp>` shaped IDs too, so the
	// regex accepts both: `goal:` followed by a non-whitespace token
	// of letters / digits / dash / underscore. Strict enough to bound
	// the match (no spaces, no quotes, no punctuation that would let
	// adjacent markdown leak into the captured ID).
	reGoalLink       = regexp.MustCompile(`goal:([A-Za-z0-9_-]+)`)
	// Deadline marker `deadline:<ulid>` (lowercase 26-char Crockford
	// alphabet — what internal/deadlines mints). The shape mirrors
	// the goal: marker so the TUI parser ignores it gracefully if it
	// hasn't learned the new field yet (the marker just becomes
	// inert text from the legacy parser's perspective).
	reDeadlineLink   = regexp.MustCompile(`deadline:([0-9a-z]{26})`)
	reDailyNoteName  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	// Frontmatter opt-out: a note containing `tasks: false` (also
	// `no`, `skip`, `none`) inside its leading YAML block is excluded
	// from the task scanner entirely. This lets users keep `- [ ]`
	// style bullet lists in reading notes / templates / brainstorm
	// pages without those bullets cluttering the global task view.
	// Match is anchored to a line start (`(?m)^`) so a line of body
	// text that happens to contain "tasks: false" can't disable
	// scanning for the whole note.
	reTaskOptOut    = regexp.MustCompile(`(?m)^tasks:\s*(false|no|skip|none)\s*$`)
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
	if hasTaskOptOut(note.Content) {
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
	// ASCII due:YYYY-MM-DD — overrides emoji + filename fallback,
	// because the web's buildTaskTextLine emits this form on
	// create/patch and we want it to round-trip exactly.
	if dm := reDueDateAscii.FindStringSubmatch(taskText); dm != nil {
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
	default:
		// ASCII `!N` shorthand fallback. `!1` → Highest (4), `!2` →
		// High (3), `!3` → Med (2). The mapping inverts the digit so
		// "P1 = highest" matches user intuition AND the parser's
		// existing emoji semantics where bigger number = more urgent.
		if m := rePrioBangAscii.FindStringSubmatch(taskText); m != nil {
			switch m[1] {
			case "1":
				t.Priority = 4
			case "2":
				t.Priority = 3
			case "3":
				t.Priority = 2
			}
		}
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

	if dm := reDeadlineLink.FindStringSubmatch(taskText); dm != nil {
		t.DeadlineID = dm[1]
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

// hasTaskOptOut returns true when the note's leading frontmatter
// contains `tasks: false` (or no/skip/none). Bounded to the
// frontmatter block so the same key inside a code fence or quoted
// body text doesn't accidentally suppress scanning.
//
// The internal/vault package already has a richer ParseFrontmatter,
// but pulling it in here would invert the dependency direction
// (tasks → vault → tasks via the Note conversion at the boundary),
// so we re-detect the block locally with a tiny stdlib-only routine.
func hasTaskOptOut(content string) bool {
	// Frontmatter must start at byte 0. Accept LF and CRLF after the
	// opening "---" so files committed from Windows still match.
	var rest string
	switch {
	case strings.HasPrefix(content, "---\n"):
		rest = content[4:]
	case strings.HasPrefix(content, "---\r\n"):
		rest = content[5:]
	default:
		return false
	}
	// End delimiter is the first line equal to `---` (with optional
	// CR). Look for "\n---" as a cheap match — the strictly correct
	// form would also accept a leading `---` immediately after the
	// opener (an empty block), but no real note hits that case.
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return false
	}
	return reTaskOptOut.MatchString(rest[:end])
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
