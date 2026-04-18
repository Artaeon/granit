package tui

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Planner file I/O — consolidated read/write for Planner/{date}.md files,
// daily note section management, and task schedule annotation.
// ---------------------------------------------------------------------------

// writePlannerFocus writes or updates the ## Focus section in the planner file
// for the given date. Replaces any existing Focus section.
func writePlannerFocus(vaultRoot, date, topGoal string, focusItems []string) error {
	dir := filepath.Join(vaultRoot, "Planner")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, date+".md")

	var section strings.Builder
	section.WriteString("## Focus\n")
	if topGoal != "" {
		section.WriteString("- Top goal: " + topGoal + "\n")
	}
	for _, item := range focusItems {
		section.WriteString("- " + item + "\n")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		content = []byte("---\ndate: " + date + "\n---\n\n")
	}
	if idx := bytes.Index(content, []byte("## Focus")); idx >= 0 {
		end := bytes.Index(content[idx+1:], []byte("\n## "))
		if end >= 0 {
			content = append(content[:idx], content[idx+1+end+1:]...)
		} else {
			content = content[:idx]
		}
	}
	content = append(content, []byte("\n"+section.String())...)
	return atomicWriteNote(path, string(content))
}

// readPlannerScheduleBlocks parses the ## Schedule section of a single
// Planner/{date}.md file and returns its blocks. Missing file yields an
// empty slice (not an error) — a new day simply has no schedule yet.
func readPlannerScheduleBlocks(vaultRoot, date string) []PlannerBlock {
	path := filepath.Join(vaultRoot, "Planner", date+".md")
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var blocks []PlannerBlock
	scanner := bufio.NewScanner(f)
	inSchedule := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "## Schedule" {
			inSchedule = true
			continue
		}
		if strings.HasPrefix(line, "## ") {
			inSchedule = false
			continue
		}
		if !inSchedule || !strings.HasPrefix(line, "- ") {
			continue
		}
		if b, ok := parseScheduleBlockLine(line, date); ok {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// parseScheduleBlockLine parses a "- HH:MM-HH:MM | text | type [| done] [| @ref]" line.
// Trailing fields beyond type are order-independent flags:
//
//	"done"                 → block is completed
//	"@notepath:lineNum"    → source-task reference (optional; for matching)
func parseScheduleBlockLine(line, date string) (PlannerBlock, bool) {
	trimmed := strings.TrimPrefix(line, "- ")
	parts := strings.Split(trimmed, " | ")
	if len(parts) < 3 {
		return PlannerBlock{}, false
	}
	timeParts := strings.Split(strings.TrimSpace(parts[0]), "-")
	if len(timeParts) != 2 {
		return PlannerBlock{}, false
	}
	startStr := strings.TrimSpace(timeParts[0])
	endStr := strings.TrimSpace(timeParts[1])
	// Reject malformed times at the parser boundary so they don't become
	// silent midnight blocks downstream.
	if _, ok := parseSlot(startStr); !ok {
		return PlannerBlock{}, false
	}
	if _, ok := parseSlot(endStr); !ok {
		return PlannerBlock{}, false
	}
	b := PlannerBlock{
		Date:      date,
		StartTime: startStr,
		EndTime:   endStr,
		Text:      strings.TrimSpace(parts[1]),
		// Normalise on ingest so downstream switch-on-BlockType is a
		// plain equality check against canonical constants.
		BlockType: NormaliseBlockType(parts[2]),
	}
	for _, extra := range parts[3:] {
		extra = strings.TrimSpace(extra)
		switch {
		case extra == "done":
			b.Done = true
		case strings.HasPrefix(extra, "@"):
			if ref, ok := parseSourceRefSuffix(extra[1:]); ok {
				ref.Text = b.Text
				b.SourceRef = ref
			}
		}
	}
	return b, true
}

// parseSourceRefSuffix parses "notepath:lineNum" into a ScheduleRef. Returns
// false if the shape is wrong.
func parseSourceRefSuffix(s string) (ScheduleRef, bool) {
	// Walk from the end — note paths may contain colons on exotic filesystems,
	// but line numbers are always trailing digits after the last colon.
	i := strings.LastIndex(s, ":")
	if i <= 0 || i == len(s)-1 {
		return ScheduleRef{}, false
	}
	path := s[:i]
	lineStr := s[i+1:]
	var line int
	if _, err := fmt.Sscanf(lineStr, "%d", &line); err != nil || line < 1 {
		return ScheduleRef{}, false
	}
	return ScheduleRef{NotePath: path, LineNum: line}, true
}

// formatScheduleBlockLine renders a PlannerBlock as its "- ..." line form.
func formatScheduleBlockLine(b PlannerBlock) string {
	var suffix strings.Builder
	if b.Done {
		suffix.WriteString(" | done")
	}
	if b.SourceRef.hasLocation() {
		fmt.Fprintf(&suffix, " | @%s:%d", b.SourceRef.NotePath, b.SourceRef.LineNum)
	}
	return fmt.Sprintf("- %s-%s | %s | %s%s", b.StartTime, b.EndTime, b.Text, b.BlockType, suffix.String())
}

// writePlannerScheduleBlocks replaces the ## Schedule section of
// Planner/{date}.md with the given blocks (preserving all other sections).
// Creates the file with frontmatter if it does not exist.
func writePlannerScheduleBlocks(vaultRoot, date string, blocks []PlannerBlock) error {
	if vaultRoot == "" || date == "" {
		return fmt.Errorf("planner: vault root or date empty")
	}
	dir := filepath.Join(vaultRoot, "Planner")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, date+".md")

	var body strings.Builder
	body.WriteString("## Schedule\n")
	for _, b := range blocks {
		body.WriteString(formatScheduleBlockLine(b))
		body.WriteString("\n")
	}

	existing, err := os.ReadFile(path)
	if err != nil {
		// New file: frontmatter + schedule section.
		content := "---\ndate: " + date + "\n---\n\n" + body.String()
		return atomicWriteNote(path, content)
	}
	updated := replaceDailySection(string(existing), body.String(), "## Schedule")
	return atomicWriteNote(path, updated)
}

// loadPlannerBlocks scans the Planner/ directory for schedule files and
// returns all blocks keyed by date string ("YYYY-MM-DD") plus daily focus data.
func loadPlannerBlocks(vaultRoot string) (map[string][]PlannerBlock, map[string]DailyFocus) {
	result := make(map[string][]PlannerBlock)
	focusResult := make(map[string]DailyFocus)
	plannerDir := filepath.Join(vaultRoot, "Planner")
	entries, err := os.ReadDir(plannerDir)
	if err != nil {
		return result, focusResult
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		dateStr := strings.TrimSuffix(e.Name(), ".md")
		if _, parseErr := time.Parse("2006-01-02", dateStr); parseErr != nil {
			continue
		}
		fp := filepath.Join(plannerDir, e.Name())
		f, err := os.Open(fp)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		inSchedule := false
		inFocus := false
		var focusItems []string
		var topGoal string
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "## Schedule" {
				inSchedule = true
				inFocus = false
				continue
			}
			if line == "## Focus" {
				inSchedule = false
				inFocus = true
				continue
			}
			if strings.HasPrefix(line, "## ") {
				inSchedule = false
				inFocus = false
				continue
			}
			if inFocus {
				if strings.HasPrefix(line, "- Top goal: ") {
					topGoal = strings.TrimPrefix(line, "- Top goal: ")
				} else if strings.HasPrefix(line, "- ") {
					focusItems = append(focusItems, strings.TrimPrefix(line, "- "))
				}
				continue
			}
			if !inSchedule || !strings.HasPrefix(line, "- ") {
				continue
			}
			trimmed := strings.TrimPrefix(line, "- ")
			parts := strings.Split(trimmed, " | ")
			if len(parts) < 3 {
				continue
			}
			timeRange := strings.TrimSpace(parts[0])
			timeParts := strings.Split(timeRange, "-")
			if len(timeParts) != 2 {
				continue
			}
			pb := PlannerBlock{
				Date:      dateStr,
				StartTime: strings.TrimSpace(timeParts[0]),
				EndTime:   strings.TrimSpace(timeParts[1]),
				Text:      strings.TrimSpace(parts[1]),
				BlockType: NormaliseBlockType(parts[2]),
			}
			if len(parts) >= 4 && strings.TrimSpace(parts[3]) == "done" {
				pb.Done = true
			}
			result[dateStr] = append(result[dateStr], pb)
		}
		_ = f.Close()
		if topGoal != "" || len(focusItems) > 0 {
			focusResult[dateStr] = DailyFocus{TopGoal: topGoal, FocusItems: focusItems}
		}
	}
	return result, focusResult
}

// replaceDailySection replaces an existing markdown section (identified by its
// ## heading prefix) in content, or appends it if no such section exists.
// The heading is matched as a line prefix at a line boundary.
func replaceDailySection(existing, newSection, heading string) string {
	idx := -1
	for i := 0; i < len(existing); {
		pos := strings.Index(existing[i:], heading)
		if pos < 0 {
			break
		}
		pos += i
		if pos > 0 && existing[pos-1] != '\n' {
			i = pos + 1
			continue
		}
		afterIdx := pos + len(heading)
		if afterIdx >= len(existing) {
			idx = pos
			break
		}
		ch := existing[afterIdx]
		if ch == '\n' || ch == '\r' || ch == ' ' {
			idx = pos
			break
		}
		i = pos + 1
	}
	if idx < 0 {
		return strings.TrimRight(existing, "\n") + "\n\n" + newSection
	}
	rest := existing[idx+len(heading):]
	end := strings.Index(rest, "\n## ")
	if end >= 0 {
		return strings.TrimRight(existing[:idx], "\n") + "\n\n" + newSection + "\n" + strings.TrimLeft(rest[end+1:], "\n")
	}
	return strings.TrimRight(existing[:idx], "\n") + "\n\n" + newSection
}

// parseSlot strictly parses "HH:MM" into total minutes-from-midnight.
// Returns ok=false for anything that isn't two non-negative ints
// separated by a single colon, with hours 0..23 and minutes 0..59.
// Used at parser boundaries (parseScheduleBlockLine) to reject malformed
// schedule lines before they enter the system as silent midnights.
func parseSlot(s string) (int, bool) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, false
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, false
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// slotToMinutes converts "HH:MM" to minutes from midnight, returning 0
// for malformed input. Lenient — meant for in-tree callers that already
// hold parser-validated strings. New code that handles user/external
// input should use parseSlot directly.
func slotToMinutes(s string) int {
	n, _ := parseSlot(s)
	return n
}

// fmtTimeSlot formats minutes-from-midnight as "HH:MM".
func fmtTimeSlot(mins int) string {
	return fmt.Sprintf("%02d:%02d", mins/60, mins%60)
}

// gatherTodayEvents converts loaded CalendarEvents into PlannerEvents for today,
// handling multi-day events that span into today.
func gatherTodayEvents(calEvents []CalendarEvent) []PlannerEvent {
	today := time.Now().Format("2006-01-02")
	todayT, _ := time.Parse("2006-01-02", today)
	var events []PlannerEvent
	for _, ev := range calEvents {
		evDate := ev.Date.Format("2006-01-02")
		isToday := evDate == today
		if !isToday && !ev.EndDate.IsZero() {
			startD := time.Date(ev.Date.Year(), ev.Date.Month(), ev.Date.Day(), 0, 0, 0, 0, time.Local)
			endD := time.Date(ev.EndDate.Year(), ev.EndDate.Month(), ev.EndDate.Day(), 0, 0, 0, 0, time.Local)
			if !todayT.Before(startD) && !todayT.After(endD) {
				isToday = true
			}
		}
		if !isToday {
			continue
		}
		dur := 60
		if !ev.EndDate.IsZero() {
			dur = int(ev.EndDate.Sub(ev.Date).Minutes())
			if dur <= 0 {
				dur = 60
			}
		}
		timeStr := ""
		if !ev.AllDay {
			timeStr = ev.Date.Format("15:04")
		}
		events = append(events, PlannerEvent{
			Title:    ev.Title,
			Time:     timeStr,
			Duration: dur,
		})
	}
	return events
}
