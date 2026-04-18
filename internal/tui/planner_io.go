package tui

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Planner file I/O — consolidated read/write for Planner/{date}.md files,
// daily note section management, and task schedule annotation.
// ---------------------------------------------------------------------------

// writePlannerBlock appends a single schedule block to the planner file for
// the given date. Creates the file with frontmatter if it doesn't exist.
func writePlannerBlock(vaultRoot, date string, block PlannerBlock) {
	if vaultRoot == "" {
		return
	}
	dir := filepath.Join(vaultRoot, "Planner")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, date+".md")

	line := fmt.Sprintf("- %s-%s | %s | %s\n", block.StartTime, block.EndTime, block.Text, block.BlockType)

	content, err := os.ReadFile(path)
	if err != nil {
		content = []byte("---\ndate: " + date + "\n---\n\n## Schedule\n" + line)
		_ = atomicWriteNote(path, string(content))
		return
	}

	scheduleHeader := "## Schedule\n"
	idx := bytes.Index(content, []byte(scheduleHeader))
	if idx < 0 {
		content = append(content, []byte("\n"+scheduleHeader+line)...)
	} else {
		insertAt := idx + len(scheduleHeader)
		content = append(content[:insertAt], append([]byte(line), content[insertAt:]...)...)
	}
	_ = atomicWriteNote(path, string(content))
}

// writePlannerFocus writes or updates the ## Focus section in the planner file
// for the given date. Replaces any existing Focus section.
func writePlannerFocus(vaultRoot, date, topGoal string, focusItems []string) {
	dir := filepath.Join(vaultRoot, "Planner")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
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
	_ = atomicWriteNote(path, string(content))
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
				BlockType: strings.TrimSpace(strings.ToLower(parts[2])),
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

// updateTaskScheduleInFile annotates matching task lines with a schedule
// marker (⏰ HH:MM-HH:MM). Searches Tasks.md first, then all .md files in
// the vault root. Replaces existing markers.
func updateTaskScheduleInFile(vaultRoot, taskText, startTime, endTime string) {
	scheduleMarkerRe := regexp.MustCompile(`\s*⏰\s*\d{2}:\d{2}-\d{2}:\d{2}`)
	marker := " ⏰ " + startTime + "-" + endTime

	normalise := func(s string) string {
		s = scheduleMarkerRe.ReplaceAllString(s, "")
		return strings.TrimSpace(s)
	}
	needle := normalise(taskText)

	// tryFile attempts to find and annotate the task in a single file.
	tryFile := func(path string) bool {
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "- [") {
				continue
			}
			if idx := strings.Index(trimmed, "] "); idx >= 0 {
				lineTask := normalise(trimmed[idx+2:])
				if lineTask == needle {
					cleaned := scheduleMarkerRe.ReplaceAllString(line, "")
					lines[i] = cleaned + marker
					_ = atomicWriteNote(path, strings.Join(lines, "\n"))
					return true
				}
			}
		}
		return false
	}

	// Try Tasks.md first (most likely location).
	tasksPath := tasksFilePath(vaultRoot)
	if tasksPath != "" && tryFile(tasksPath) {
		return
	}

	// Fall back to scanning all .md files in the vault.
	_ = filepath.Walk(vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}
		if path == tasksPath {
			return nil // already tried
		}
		if tryFile(path) {
			return filepath.SkipAll // found it, stop walking
		}
		return nil
	})
}

// slotToMinutes converts "HH:MM" to minutes from midnight.
func slotToMinutes(s string) int {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0
	}
	h, m := 0, 0
	_, _ = fmt.Sscanf(s, "%d:%d", &h, &m)
	return h*60 + m
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
