package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ==================== Kanban ====================

// GetKanban loads the kanban board JSON from .granit/kanban.json.
func (a *GranitApp) GetKanban() (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vaultRoot == "" {
		return "", fmt.Errorf("no vault open")
	}
	fp := filepath.Join(a.vaultRoot, ".granit", "kanban.json")
	data, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return "{}", nil // empty board
		}
		return "", err
	}
	return string(data), nil
}

// SaveKanban persists the kanban board JSON to .granit/kanban.json.
func (a *GranitApp) SaveKanban(data string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vaultRoot == "" {
		return fmt.Errorf("no vault open")
	}
	dir := filepath.Join(a.vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create .granit directory: %w", err)
	}
	fp := filepath.Join(dir, "kanban.json")
	return atomicWriteFile(fp, []byte(data), 0o644)
}

// ==================== Tasks ====================

// TaskItem represents a single task extracted from a vault note.
type TaskItem struct {
	Text             string   `json:"text"`
	Done             bool     `json:"done"`
	NotePath         string   `json:"notePath"`
	LineNum          int      `json:"lineNum"`
	Priority         int      `json:"priority"`         // 0=none, 1=low, 2=med, 3=high, 4=highest
	DueDate          string   `json:"dueDate"`          // "YYYY-MM-DD" or ""
	Tags             []string `json:"tags"`             // e.g. ["work", "urgent"]
	EstimatedMinutes int      `json:"estimatedMinutes"` // from ~30m or ~2h
	ScheduledTime    string   `json:"scheduledTime"`    // "HH:MM-HH:MM" or ""
	Recurrence       string   `json:"recurrence"`       // "daily", "weekly", etc. or ""
	GoalID           string   `json:"goalId"`           // "G001" or ""
	SnoozedUntil     string   `json:"snoozedUntil"`     // "YYYY-MM-DDTHH:MM" or ""
}

// GetAllTasks scans all vault notes for checkbox task lines (- [ ] and - [x])
// and returns them with their source note path, line number, text, and status.
func (a *GranitApp) GetAllTasks() ([]TaskItem, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.getAllTasksInternal()
}

var (
	taskItemRe       = regexp.MustCompile(`^\s*- \[([ xX])\] (.+)`)
	taskDueDateRe    = regexp.MustCompile(`📅\s*(\d{4}-\d{2}-\d{2})`)
	taskPrioHighest  = regexp.MustCompile(`\x{1F53A}`) // 🔺
	taskPrioHigh     = regexp.MustCompile(`\x{23EB}`)  // ⏫
	taskPrioMed      = regexp.MustCompile(`\x{1F53C}`) // 🔼
	taskPrioLow      = regexp.MustCompile(`\x{1F53D}`) // 🔽
	taskTagRe        = regexp.MustCompile(`#([A-Za-z0-9_/-]+)`)
	taskEstimateRe   = regexp.MustCompile(`~(\d+)(m|h)`)
	taskScheduleRe   = regexp.MustCompile(`⏰\s*(\d{2}:\d{2}-\d{2}:\d{2})`)
	taskRecurEmojiRe = regexp.MustCompile(`\x{1F501}\s*(daily|weekly|monthly|3x-week)`)
	taskRecurTagRe   = regexp.MustCompile(`#(daily|weekly|monthly|3x-week)\b`)
	taskGoalIDRe     = regexp.MustCompile(`goal:(G\d{3,})`)
	taskSnoozeRe     = regexp.MustCompile(`snooze:(\d{4}-\d{2}-\d{2}T\d{2}:\d{2})`)
)

// getAllTasksInternal is the lock-free version of GetAllTasks.
// Callers must hold at least a.mu.RLock().
func (a *GranitApp) getAllTasksInternal() ([]TaskItem, error) {
	if a.vault == nil {
		return nil, fmt.Errorf("no vault open")
	}

	var tasks []TaskItem
	for _, p := range a.vault.SortedPaths() {
		// Apply folder exclusions from config.
		excluded := false
		for _, folder := range a.config.TaskExcludeFolders {
			if strings.HasPrefix(p, folder) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		note := a.vault.GetNote(p)
		if note == nil {
			continue
		}
		lines := strings.Split(note.Content, "\n")
		for i, line := range lines {
			m := taskItemRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			done := m[1] != " "
			text := m[2]

			// Extract metadata.
			priority := 0
			if taskPrioHighest.MatchString(text) {
				priority = 4
			} else if taskPrioHigh.MatchString(text) {
				priority = 3
			} else if taskPrioMed.MatchString(text) {
				priority = 2
			} else if taskPrioLow.MatchString(text) {
				priority = 1
			}

			dueDate := ""
			if dm := taskDueDateRe.FindStringSubmatch(text); dm != nil {
				dueDate = dm[1]
			}

			tags := []string{}
			for _, tm := range taskTagRe.FindAllStringSubmatch(text, -1) {
				tags = append(tags, tm[1])
			}

			estimatedMinutes := 0
			if em := taskEstimateRe.FindStringSubmatch(text); em != nil {
				n := 0
				if _, err := fmt.Sscanf(em[1], "%d", &n); err == nil && n > 0 {
					if em[2] == "h" {
						estimatedMinutes = n * 60
					} else {
						estimatedMinutes = n
					}
				}
			}

			scheduledTime := ""
			if sm := taskScheduleRe.FindStringSubmatch(text); sm != nil {
				scheduledTime = sm[1]
			}

			recurrence := ""
			if rm := taskRecurEmojiRe.FindStringSubmatch(text); rm != nil {
				recurrence = rm[1]
			} else if rm := taskRecurTagRe.FindStringSubmatch(text); rm != nil {
				recurrence = rm[1]
			}

			goalID := ""
			if gm := taskGoalIDRe.FindStringSubmatch(text); gm != nil {
				goalID = gm[1]
			}

			snoozedUntil := ""
			if sm := taskSnoozeRe.FindStringSubmatch(text); sm != nil {
				snoozedUntil = sm[1]
			}

			// Apply "tagged" filter from config.
			if a.config.TaskFilterMode == "tagged" && len(a.config.TaskRequiredTags) > 0 {
				hasTag := false
				for _, reqTag := range a.config.TaskRequiredTags {
					for _, taskTag := range tags {
						if strings.EqualFold(taskTag, reqTag) {
							hasTag = true
							break
						}
					}
					if hasTag {
						break
					}
				}
				if !hasTag {
					continue
				}
			}

			if a.config.TaskExcludeDone && done {
				continue
			}

			tasks = append(tasks, TaskItem{
				Text:             text,
				Done:             done,
				NotePath:         p,
				LineNum:          i + 1, // 1-based
				Priority:         priority,
				DueDate:          dueDate,
				Tags:             tags,
				EstimatedMinutes: estimatedMinutes,
				ScheduledTime:    scheduledTime,
				Recurrence:       recurrence,
				GoalID:           goalID,
				SnoozedUntil:     snoozedUntil,
			})
		}
	}
	return tasks, nil
}
